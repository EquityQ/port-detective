package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	gnet "github.com/shirou/gopsutil/v3/net"

	"github.com/EquityQ/port-detective/internal/core"
)

// errSimulatedTableFailure stands in for a host that refuses to expose its TCP
// listener table, so the degraded path is covered deterministically.
var errSimulatedTableFailure = errors.New("simulated listener table unavailable")

// writeManifest makes a directory recognizable as a project root.
func writeManifest(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"preview-flow"}`), 0o600); err != nil {
		t.Fatalf("write project manifest: %v", err)
	}
}

// projectDetectorFor pins every process to a test project directory.
func projectDetectorFor(dir string) core.ProjectDetectorFunc {
	return func(context.Context, int32) *core.ProjectInfo {
		return &core.ProjectInfo{Name: "preview-flow", Path: dir}
	}
}

// waitForFree polls until a port stops listening, so the drift test is not racing
// the operating system's socket teardown.
func waitForFree(t *testing.T, ctx context.Context, port int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if err := ctx.Err(); err != nil {
			t.Fatalf("wait for free port: %v", err)
		}
		diag, err := core.InspectPort(ctx, port)
		if err != nil {
			t.Fatalf("inspect while waiting for free port: %v", err)
		}
		if !diag.IsOpen {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("port %d did not become free", port)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// freePortPayload mirrors the free_port response contract.
type freePortPayload struct {
	Port                 int                  `json:"port"`
	DryRun               bool                 `json:"dry_run"`
	PreviewID            string               `json:"preview_id"`
	PreviewExpiresAtUnix int64                `json:"preview_expires_at_unix"`
	PreActionDiagnostic  *core.PortDiagnostic `json:"pre_action_diagnostic"`
	Diagnostic           *core.PortDiagnostic `json:"diagnostic"`
	PostActionStatus     string               `json:"post_action_status"`
	Message              string               `json:"message"`
}

func decodeFreePort(t *testing.T, res *mcp.CallToolResult) freePortPayload {
	t.Helper()
	var payload freePortPayload
	if err := json.Unmarshal([]byte(resultText(t, res)), &payload); err != nil {
		t.Fatalf("free_port response is not valid JSON: %v\nraw: %s", err, resultText(t, res))
	}
	return payload
}

// TestFreePortPreviewIssuesToken proves a dry run returns a usable preview token
// plus the truthful pre-action snapshot.
func TestFreePortPreviewIssuesToken(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	res := callTool(t, handleFreePort, map[string]any{"port": float64(port)})
	if res.IsError {
		t.Fatalf("free_port preview failed: %s", resultText(t, res))
	}
	payload := decodeFreePort(t, res)
	if !payload.DryRun {
		t.Fatal("free_port must default to a dry run")
	}
	if payload.PreviewID == "" {
		t.Fatal("a successful preview must return a preview_id")
	}
	if payload.PreviewExpiresAtUnix <= 0 {
		t.Fatal("a preview must report its expiry")
	}
	if payload.PreActionDiagnostic == nil || !payload.PreActionDiagnostic.IsOpen {
		t.Fatalf("pre-action diagnostic must describe the occupied port: %+v", payload.PreActionDiagnostic)
	}
	// The deprecated alias must keep matching the truthful field.
	if payload.Diagnostic == nil || payload.Diagnostic.PID != payload.PreActionDiagnostic.PID {
		t.Fatal("the deprecated diagnostic alias must match pre_action_diagnostic")
	}
	if !strings.Contains(payload.Message, "preview") {
		t.Fatalf("preview message should explain the next step: %q", payload.Message)
	}
}

// TestFreePortExecutionRequiresPreviewToken proves dry_run=false cannot be used
// as a blind destructive shortcut.
func TestFreePortExecutionRequiresPreviewToken(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	res := callTool(t, handleFreePort, map[string]any{
		"port":    float64(port),
		"dry_run": false,
	})
	if !res.IsError {
		t.Fatal("destructive execution without a preview_id must fail")
	}
	if !strings.Contains(resultText(t, res), "preview_id") {
		t.Fatalf("error should name the missing preview_id: %s", resultText(t, res))
	}
	// The listener must still be alive: the refusal happened before any action.
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("a refused action must leave the listener alive: %v", err)
	}
	_ = conn.Close()
}

// TestFreePortRejectsForgedAndReusedTokens covers token tampering and cross-port
// reuse without terminating anything.
func TestFreePortRejectsForgedAndReusedTokens(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })
	other, otherPort := listenLoopback(t)
	t.Cleanup(func() { _ = other.Close() })

	preview := callTool(t, handleFreePort, map[string]any{"port": float64(port)})
	if preview.IsError {
		t.Fatalf("preview failed: %s", resultText(t, preview))
	}
	token := decodeFreePort(t, preview).PreviewID

	// A token for port A must not authorize port B.
	res := callTool(t, handleFreePort, map[string]any{
		"port":       float64(otherPort),
		"dry_run":    false,
		"preview_id": token,
	})
	if !res.IsError {
		t.Fatal("a preview token must not authorize a different port")
	}

	// A tampered token must be rejected outright.
	tampered := "x" + token[1:]
	res = callTool(t, handleFreePort, map[string]any{
		"port":       float64(port),
		"dry_run":    false,
		"preview_id": tampered,
	})
	if !res.IsError {
		t.Fatal("a tampered preview token must be rejected")
	}
}

// TestFreePortPreviewOnIdlePortFails keeps the existing "nothing to kill" behavior.
func TestFreePortPreviewOnIdlePortFails(t *testing.T) {
	port := freeLoopbackPort(t)
	res := callTool(t, handleFreePort, map[string]any{"port": float64(port)})
	if !res.IsError {
		t.Fatal("previewing a free port must fail")
	}
	if !strings.Contains(resultText(t, res), "currently free") {
		t.Fatalf("unexpected message: %s", resultText(t, res))
	}
}

// TestScanToolRejectsBlankPorts proves presence is distinguished from blankness:
// an explicitly empty ports value must not widen into the default port list.
func TestScanToolRejectsBlankPorts(t *testing.T) {
	for _, ports := range []string{"", "   ", ","} {
		res := callTool(t, handleScanDevPorts, map[string]any{"ports": ports})
		if !res.IsError {
			t.Fatalf("blank ports %q must be rejected, not treated as omitted", ports)
		}
	}
}

// TestScanToolOmittedPortsUsesDefaultScope keeps the documented omitted-argument default.
func TestScanToolOmittedPortsUsesDefaultScope(t *testing.T) {
	res := callTool(t, handleScanDevPorts, map[string]any{})
	if res.IsError {
		t.Fatalf("omitted ports must keep the default scope: %s", resultText(t, res))
	}
	var diags []core.PortDiagnostic
	if err := json.Unmarshal([]byte(resultText(t, res)), &diags); err != nil {
		t.Fatalf("default scan must stay a JSON array: %v", err)
	}
	allowed := make(map[int]bool, len(core.DefaultDevPorts))
	for _, p := range core.DefaultDevPorts {
		allowed[p] = true
	}
	for _, d := range diags {
		if !allowed[d.Port] {
			t.Fatalf("default scan returned port %d outside DefaultDevPorts", d.Port)
		}
	}
}

// TestCleanToolRejectsBlankPorts proves the same fail-closed rule for cleanup,
// which is the most dangerous scope to widen accidentally.
func TestCleanToolRejectsBlankPorts(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })
	_ = port

	for _, ports := range []string{"", "   "} {
		res := callTool(t, handleCleanCurrentProject, map[string]any{"ports": ports})
		if !res.IsError {
			t.Fatalf("blank cleanup ports %q must be rejected, not widened to all listeners", ports)
		}
	}
}

// TestCleanToolExecutionRequiresPreviewToken mirrors the free_port rule.
func TestCleanToolExecutionRequiresPreviewToken(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	res := callTool(t, handleCleanCurrentProject, map[string]any{
		"ports":   strconv.Itoa(port),
		"dry_run": false,
	})
	if !res.IsError {
		t.Fatal("cleanup without a preview_id must fail")
	}
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("a refused cleanup must leave the listener alive: %v", err)
	}
	_ = conn.Close()
}

// TestCleanPreviewAndExecutionBindTargets walks the full preview -> execute
// handshake on a port that is deliberately free afterwards, proving drift fails.
func TestCleanPreviewRejectsDriftedTargets(t *testing.T) {
	ln, port := listenLoopback(t)
	projectDir := t.TempDir()
	writeManifest(t, projectDir)
	core.RegisterProjectDetector(projectDetectorFor(projectDir))
	t.Cleanup(func() { core.RegisterProjectDetector(nil) })
	t.Cleanup(func() { _ = ln.Close() })

	preview := callTool(t, handleCleanCurrentProject, map[string]any{
		"project_path": projectDir,
		"ports":        strconv.Itoa(port),
	})
	if preview.IsError {
		t.Fatalf("cleanup preview failed: %s", resultText(t, preview))
	}
	var payload struct {
		PreviewID string                `json:"preview_id"`
		Count     int                   `json:"count"`
		Targets   []core.PortDiagnostic `json:"targets"`
		Cleaned   []core.PortDiagnostic `json:"cleaned"`
	}
	if err := json.Unmarshal([]byte(resultText(t, preview)), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.PreviewID == "" {
		t.Fatalf("a non-empty preview must return a token: %s", resultText(t, preview))
	}
	if payload.Count == 0 || len(payload.Targets) == 0 {
		t.Fatalf("preview should report its targets: %+v", payload)
	}

	// Release the port between preview and execution: the action must fail
	// instead of terminating whatever takes the port next.
	_ = ln.Close()
	waitForFree(t, context.Background(), port)

	res := callTool(t, handleCleanCurrentProject, map[string]any{
		"project_path": projectDir,
		"ports":        strconv.Itoa(port),
		"dry_run":      false,
		"preview_id":   payload.PreviewID,
	})
	if !res.IsError {
		t.Fatalf("execution against a released port must fail: %s", resultText(t, res))
	}
}

// TestCleanPreviewEmptyScopeReturnsNoToken keeps an empty preview honest.
func TestCleanPreviewEmptyScopeReturnsNoToken(t *testing.T) {
	projectDir := t.TempDir()
	writeManifest(t, projectDir)
	core.RegisterProjectDetector(projectDetectorFor(projectDir))
	t.Cleanup(func() { core.RegisterProjectDetector(nil) })

	res := callTool(t, handleCleanCurrentProject, map[string]any{
		"project_path": projectDir,
		"ports":        "1",
		"dry_run":      true,
	})
	if res.IsError {
		t.Fatalf("empty preview should not be an error: %s", resultText(t, res))
	}
	var payload struct {
		PreviewID string `json:"preview_id"`
		Count     int    `json:"count"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal([]byte(resultText(t, res)), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count != 0 {
		t.Skipf("host has a listener on port 1 that belongs to this project: %+v", payload)
	}
	if payload.PreviewID != "" {
		t.Fatal("an empty preview must not issue an executable token")
	}
}

// TestScanToolReportsDegradedNotice proves the incomplete-result warning is
// surfaced without breaking the JSON array contract.
func TestScanToolReportsDegradedNotice(t *testing.T) {
	restore := core.SetConnectionsListerForTesting(func(context.Context) ([]gnet.ConnectionStat, error) {
		return nil, errSimulatedTableFailure
	})
	defer restore()

	res := callTool(t, handleScanDevPorts, map[string]any{"ports": "1"})
	if res.IsError {
		t.Fatalf("a degraded scan should still return data: %s", resultText(t, res))
	}
	var diags []core.PortDiagnostic
	if err := json.Unmarshal([]byte(resultText(t, res)), &diags); err != nil {
		t.Fatalf("first content item must stay a JSON array: %v", err)
	}
	if len(res.Content) < 2 {
		t.Fatalf("a degraded scan must append a warning item, got %d item(s)", len(res.Content))
	}
	notice, ok := res.Content[1].(mcp.TextContent)
	if !ok {
		t.Fatalf("second content item is not text: %T", res.Content[1])
	}
	if !strings.Contains(notice.Text, "incomplete result") {
		t.Fatalf("degraded notice should say the result is incomplete: %q", notice.Text)
	}
}
