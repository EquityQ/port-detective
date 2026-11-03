package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	gnet "github.com/shirou/gopsutil/v3/net"

	"github.com/EquityQ/port-detective/internal/core"
)

// runWithStderr executes the root command and reports the exit status mapping a
// real process would use, so degraded paths can be asserted end to end.
func runWithStderr(t *testing.T, args ...string) (string, string, int) {
	t.Helper()

	root := NewRootCommand()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	status := executeCommand(root)
	return out.String(), errOut.String(), status
}

// TestKillJSONDryRunEmitsPreviewToken proves the CLI exposes the same two-phase
// safety contract as MCP: preview first, then act on the token.
func TestKillJSONDryRunEmitsPreviewToken(t *testing.T) {
	ln, port := listenLoopbackCLI(t)
	t.Cleanup(func() { _ = ln.Close() })

	out, _, err := captureRun(t, "kill", strconv.Itoa(port), "--json", "--dry-run")
	if err != nil {
		t.Fatalf("kill --json --dry-run failed: %v", err)
	}
	var payload struct {
		Port                 int                  `json:"port"`
		DryRun               bool                 `json:"dry_run"`
		PreviewID            string               `json:"preview_id"`
		PreActionDiagnostic  *core.PortDiagnostic `json:"pre_action_diagnostic"`
		Diagnostic           *core.PortDiagnostic `json:"diagnostic"`
		PreviewExpiresAtUnix int64                `json:"preview_expires_at_unix"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("kill --json output is not valid JSON: %v\nraw: %s", err, out)
	}
	if !payload.DryRun {
		t.Fatal("dry_run should be true")
	}
	if payload.PreviewID == "" {
		t.Fatalf("a JSON dry run must emit a preview_id: %s", out)
	}
	if payload.PreviewExpiresAtUnix <= 0 {
		t.Fatal("a preview must report an expiry")
	}
	if payload.PreActionDiagnostic == nil || payload.Diagnostic == nil {
		t.Fatal("both the truthful and deprecated diagnostic fields must be present")
	}
	if payload.PreActionDiagnostic.PID != payload.Diagnostic.PID {
		t.Fatal("the deprecated alias must match pre_action_diagnostic")
	}
	// The pre-action snapshot must remain a true pre-action observation.
	if !payload.PreActionDiagnostic.IsOpen {
		t.Fatal("the pre-action snapshot should still describe the port as open")
	}
	if payload.PreActionDiagnostic.PostActionStatus != core.PostActionNotAttempted {
		t.Fatalf("dry run post_action_status = %q, want %q",
			payload.PreActionDiagnostic.PostActionStatus, core.PostActionNotAttempted)
	}
}

// TestKillRejectsBadPreviewID proves a forged token cannot authorize a kill.
func TestKillRejectsBadPreviewID(t *testing.T) {
	ln, port := listenLoopbackCLI(t)
	t.Cleanup(func() { _ = ln.Close() })

	_, _, err := captureRun(t, "kill", strconv.Itoa(port), "--json", "--yes", "--preview-id", "forged-token-value")
	if err == nil {
		t.Fatal("a forged preview id must be rejected")
	}
	if !strings.Contains(err.Error(), "preview id rejected") {
		t.Fatalf("unexpected error: %v", err)
	}
	// The listener must be untouched.
	conn, dialErr := net.Dial("tcp", ln.Addr().String())
	if dialErr != nil {
		t.Fatalf("a rejected kill must leave the listener alive: %v", dialErr)
	}
	_ = conn.Close()
}

// TestCleanRejectsEmptyPortsFlag proves an explicit empty selection fails closed.
func TestCleanRejectsEmptyPortsFlag(t *testing.T) {
	for _, value := range []string{"", "  "} {
		_, _, err := captureRun(t, "clean", "--dry-run", "--ports", value)
		if err == nil {
			t.Fatalf("--ports %q must be rejected instead of widening scope", value)
		}
	}
}

// TestCleanJSONDryRunStaysArray keeps the documented stable shape for clean.
func TestCleanJSONDryRunStaysArray(t *testing.T) {
	dir := t.TempDir()
	writeCLIManifest(t, dir)
	ln, port := listenLoopbackCLI(t)
	t.Cleanup(func() { _ = ln.Close() })
	core.RegisterProjectDetector(func(context.Context, int32) *core.ProjectInfo {
		return &core.ProjectInfo{Name: "cli-preview", Path: dir}
	})
	t.Cleanup(func() { core.RegisterProjectDetector(nil) })

	out, _, err := captureRun(t, "clean", "--project", dir, "--ports", strconv.Itoa(port), "--json", "--dry-run", "--yes")
	if err != nil {
		t.Fatalf("clean --json --dry-run failed: %v", err)
	}
	var diags []core.PortDiagnostic
	if err := json.Unmarshal([]byte(out), &diags); err != nil {
		t.Fatalf("clean --json must stay a JSON array: %v\nraw: %s", err, out)
	}
}

// TestCheckJSONDegradedIsNotFree proves a degraded inspection never claims "free".
func TestCheckJSONDegradedIsNotFree(t *testing.T) {
	port := freeLoopbackPortCLI(t)
	restore := core.SetConnectionsListerForTesting(func(context.Context) ([]gnet.ConnectionStat, error) {
		return nil, os.ErrPermission
	})
	defer restore()

	out, _, err := captureRun(t, "check", strconv.Itoa(port), "--json")
	if err != nil {
		t.Fatalf("check --json should degrade rather than fail: %v", err)
	}
	var diag core.PortDiagnostic
	if err := json.Unmarshal([]byte(out), &diag); err != nil {
		t.Fatalf("check --json output invalid: %v\nraw: %s", err, out)
	}
	if diag.Status == core.StatusFree {
		t.Fatal("a degraded inspection must never report status=free")
	}
	if !diag.ListenerTableMissing {
		t.Fatal("a degraded inspection must set listener_table_missing")
	}
	if diag.Status != core.StatusUndetermined {
		t.Fatalf("status = %q, want %q", diag.Status, core.StatusUndetermined)
	}
}

// TestCheckHumanDegradedDoesNotPromiseReady guards the wording regression: the
// terminal output must not tell the user a port is free when it cannot know.
func TestCheckHumanDegradedDoesNotPromiseReady(t *testing.T) {
	port := freeLoopbackPortCLI(t)
	restore := core.SetConnectionsListerForTesting(func(context.Context) ([]gnet.ConnectionStat, error) {
		return nil, os.ErrPermission
	})
	defer restore()

	out, _, err := captureRun(t, "check", strconv.Itoa(port))
	if err != nil {
		t.Fatalf("check should degrade rather than fail: %v", err)
	}
	if strings.Contains(out, "ready for use") {
		t.Fatalf("degraded output must not promise the port is ready:\n%s", out)
	}
	if !strings.Contains(out, "Inconclusive") {
		t.Fatalf("degraded output should say the conclusion is inconclusive:\n%s", out)
	}
}

// TestDoctorDegradedReportsNonZeroExit proves a failed self-check is not reported
// as success to automation.
func TestDoctorDegradedReportsNonZeroExit(t *testing.T) {
	restore := core.SetConnectionsListerForTesting(func(context.Context) ([]gnet.ConnectionStat, error) {
		return nil, os.ErrPermission
	})
	defer restore()

	out, _, status := runWithStderr(t, "doctor", "--json")
	if status == 0 {
		t.Fatalf("a degraded doctor must not exit 0\noutput: %s", out)
	}
	var report struct {
		Status string   `json:"status"`
		Hints  []string `json:"hints"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("doctor --json output invalid: %v\nraw: %s", err, out)
	}
	if report.Status != "degraded" {
		t.Fatalf("doctor status = %q, want degraded", report.Status)
	}
}

// TestRootHelpDoesNotClaimOrphanCleanup removes an over-broad capability promise.
func TestRootHelpDoesNotClaimOrphanCleanup(t *testing.T) {
	out, _, err := captureRun(t, "--help")
	if err != nil {
		t.Fatalf("root help failed: %v", err)
	}
	if strings.Contains(strings.ToLower(out), "orphan") {
		t.Fatalf("the CLI must not promise orphan-process cleanup it cannot detect:\n%s", out)
	}
}

func listenLoopbackCLI(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create loopback listener: %v", err)
	}
	return ln, ln.Addr().(*net.TCPAddr).Port
}

func freeLoopbackPortCLI(t *testing.T) int {
	t.Helper()
	ln, port := listenLoopbackCLI(t)
	if err := ln.Close(); err != nil {
		t.Fatalf("failed to close loopback listener: %v", err)
	}
	return port
}

func writeCLIManifest(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"cli-preview"}`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
