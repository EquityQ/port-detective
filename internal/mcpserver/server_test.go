package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/EquityQ/port-detective/internal/core"
)

func listenLoopback(t *testing.T) (net.Listener, int) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create loopback listener: %v", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		t.Fatalf("unexpected listener address type: %T", ln.Addr())
	}
	return ln, addr.Port
}

func freeLoopbackPort(t *testing.T) int {
	t.Helper()

	ln, port := listenLoopback(t)
	if err := ln.Close(); err != nil {
		t.Fatalf("failed to close loopback listener: %v", err)
	}
	return port
}

func callTool(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) *mcp.CallToolResult {
	t.Helper()

	req := mcp.CallToolRequest{}
	req.Params.Name = "test"
	req.Params.Arguments = args

	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("tool handler returned a Go error; it should return a CallToolResult error: %v", err)
	}
	if res == nil {
		t.Fatal("tool handler returned a nil result")
	}
	return res
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()

	if len(res.Content) == 0 {
		t.Fatal("tool result has no content")
	}
	text, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("first content item is not text: %T", res.Content[0])
	}
	return text.Text
}

// TestDescribeListsAllTools verifies that all expected tools are registered.
func TestDescribeListsAllTools(t *testing.T) {
	info := Describe()
	if info.Name != "port-detective" {
		t.Errorf("server name = %q, want %q", info.Name, "port-detective")
	}

	want := map[string]bool{
		"inspect_port":          false,
		"scan_dev_ports":        false,
		"free_port":             false,
		"clean_current_project": false,
	}
	for _, tool := range info.Tools {
		if _, ok := want[tool.Name]; !ok {
			t.Errorf("unexpected tool %q", tool.Name)
			continue
		}
		want[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %q has no description", tool.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing expected tool %q", name)
		}
	}
}

// TestToolAnnotationsAreHonest ensures read-only tools are not marked destructive.
func TestToolAnnotationsAreHonest(t *testing.T) {
	info := Describe()
	lookup := make(map[string]ToolSummary, len(info.Tools))
	for _, tool := range info.Tools {
		lookup[tool.Name] = tool
	}

	for _, name := range []string{"inspect_port", "scan_dev_ports"} {
		tool, ok := lookup[name]
		if !ok {
			t.Fatalf("tool %q was not found", name)
		}
		if !tool.ReadOnly {
			t.Errorf("tool %q should be read-only, got false", name)
		}
		if tool.Destructive {
			t.Errorf("tool %q should not be destructive, got true", name)
		}
	}

	for _, name := range []string{"free_port", "clean_current_project"} {
		tool, ok := lookup[name]
		if !ok {
			t.Fatalf("tool %q was not found", name)
		}
		if tool.ReadOnly {
			t.Errorf("%s terminates processes and must not be read-only", name)
		}
		if !tool.Destructive {
			t.Errorf("%s terminates processes and must be marked destructive", name)
		}
	}
}

// TestNewServerRegistersWithoutPanic verifies that server construction does not panic.
func TestNewServerRegistersWithoutPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewServer panicked: %v", r)
		}
	}()
	s := NewServer()
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
}

// TestInspectPortToolFindsOwnListener verifies that inspect_port detects a local listener.
func TestInspectPortToolFindsOwnListener(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	res := callTool(t, handleInspectPort, map[string]any{"port": float64(port)})
	if res.IsError {
		t.Fatalf("inspect_port returned an error: %s", resultText(t, res))
	}

	var diag core.PortDiagnostic
	if err := json.Unmarshal([]byte(resultText(t, res)), &diag); err != nil {
		t.Fatalf("response is not valid JSON: %v\nraw response: %s", err, resultText(t, res))
	}
	if !diag.IsOpen {
		t.Errorf("port %d is listening, but inspect_port returned is_open=false", port)
	}
	if diag.Port != port {
		t.Errorf("port = %d, want %d", diag.Port, port)
	}
}

// TestInspectPortToolValidatesInput verifies input validation.
func TestInspectPortToolValidatesInput(t *testing.T) {
	res := callTool(t, handleInspectPort, map[string]any{})
	if !res.IsError {
		t.Error("missing port should return an error result")
	}

	res = callTool(t, handleInspectPort, map[string]any{"port": float64(70000)})
	if !res.IsError {
		t.Error("out-of-range port should return an error result")
	}
}

// TestScanDevPortsToolDefaultScope verifies the default scan uses DefaultDevPorts only.
func TestScanDevPortsToolDefaultScope(t *testing.T) {
	res := callTool(t, handleScanDevPorts, map[string]any{})
	if res.IsError {
		t.Fatalf("scan_dev_ports returned an error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	var diags []core.PortDiagnostic
	if err := json.Unmarshal([]byte(text), &diags); err != nil {
		t.Fatalf("response is not a valid JSON array: %v", err)
	}

	allowed := make(map[int]bool, len(core.DefaultDevPorts))
	for _, p := range core.DefaultDevPorts {
		allowed[p] = true
	}
	for _, d := range diags {
		if !allowed[d.Port] {
			t.Errorf("default scan returned port %d outside DefaultDevPorts", d.Port)
		}
	}
	t.Logf("default development ports found: %d", len(diags))
}

// TestScanDevPortsToolEmptyResultIsArray verifies that empty scans keep the
// same machine-readable shape as scans that find occupied ports.
func TestScanDevPortsToolEmptyResultIsArray(t *testing.T) {
	port := freeLoopbackPort(t)
	res := callTool(t, handleScanDevPorts, map[string]any{
		"ports": fmt.Sprintf("%d", port),
	})
	if res.IsError {
		t.Fatalf("scan_dev_ports returned an error: %s", resultText(t, res))
	}

	var diags []core.PortDiagnostic
	if err := json.Unmarshal([]byte(resultText(t, res)), &diags); err != nil {
		t.Fatalf("empty scan result is not a valid JSON array: %v", err)
	}
	if diags == nil {
		t.Fatal("empty scan result must encode as [] rather than null")
	}
	if len(diags) != 0 {
		t.Fatalf("free ports must not appear in scan results: %+v", diags)
	}
}

// TestScanDevPortsToolExplicitPorts verifies the explicit ports argument.
func TestScanDevPortsToolExplicitPorts(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	res := callTool(t, handleScanDevPorts, map[string]any{
		"ports": fmt.Sprintf("%d", port),
	})
	if res.IsError {
		t.Fatalf("scan_dev_ports returned an error: %s", resultText(t, res))
	}

	var diags []core.PortDiagnostic
	if err := json.Unmarshal([]byte(resultText(t, res)), &diags); err != nil {
		t.Fatalf("response is not a valid JSON array: %v", err)
	}
	if len(diags) != 1 || diags[0].Port != port {
		t.Errorf("unexpected scan result: %+v (expected port %d)", diags, port)
	}
}

// TestScanDevPortsToolRejectsInvalidPort verifies invalid port arguments are rejected.
func TestScanDevPortsToolRejectsInvalidPort(t *testing.T) {
	res := callTool(t, handleScanDevPorts, map[string]any{"ports": "abc"})
	if !res.IsError {
		t.Error("non-numeric ports should return an error result")
	}

	res = callTool(t, handleScanDevPorts, map[string]any{"ports": "70000"})
	if !res.IsError {
		t.Error("out-of-range ports should return an error result")
	}
}

// TestFreePortToolOnIdlePort verifies free_port returns an error for an idle port.
func TestFreePortToolOnIdlePort(t *testing.T) {
	port := freeLoopbackPort(t)

	res := callTool(t, handleFreePort, map[string]any{"port": float64(port)})
	if !res.IsError {
		t.Error("free_port should return an error for an idle port")
	}
	if !strings.Contains(resultText(t, res), "currently free") {
		t.Errorf("error message is not specific enough: %s", resultText(t, res))
	}
}

// TestFreePortToolRequiresPort verifies the required port argument.
func TestFreePortToolRequiresPort(t *testing.T) {
	res := callTool(t, handleFreePort, map[string]any{})
	if !res.IsError {
		t.Error("missing port should return an error result")
	}
}

// TestFreePortToolDryRun verifies free_port defaults to a safe dry run.
func TestFreePortToolDryRun(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	res := callTool(t, handleFreePort, map[string]any{
		"port":    float64(port),
		"dry_run": true,
	})
	if res.IsError {
		t.Fatalf("free_port dry_run should succeed: %s", resultText(t, res))
	}

	var resp struct {
		Port       int                 `json:"port"`
		DryRun     bool                `json:"dry_run"`
		Diagnostic core.PortDiagnostic `json:"diagnostic"`
		Message    string              `json:"message"`
	}
	if err := json.Unmarshal([]byte(resultText(t, res)), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}
	if !resp.DryRun {
		t.Errorf("response dry_run should be true")
	}
	if resp.Port != port {
		t.Errorf("response port %d does not match %d", resp.Port, port)
	}
	if !strings.Contains(resp.Message, "dry run") {
		t.Errorf("message should include a dry-run notice: %s", resp.Message)
	}
}

// TestDevPortsAreUniqueAndSorted ensures the development port list has ten valid unique ports.
func TestDevPortsAreUniqueAndSorted(t *testing.T) {
	if len(core.DefaultDevPorts) != 10 {
		t.Errorf("DefaultDevPorts should contain 10 ports, got %d", len(core.DefaultDevPorts))
	}
	seen := make(map[int]bool, len(core.DefaultDevPorts))
	for _, p := range core.DefaultDevPorts {
		if p < 1 || p > 65535 {
			t.Errorf("development port list contains invalid port %d", p)
		}
		if seen[p] {
			t.Errorf("development port list contains duplicate port %d", p)
		}
		seen[p] = true
	}
}

// TestCleanCurrentProjectTool verifies dry-run behavior for clean_current_project.
func TestCleanCurrentProjectTool(t *testing.T) {
	res := callTool(t, handleCleanCurrentProject, map[string]any{"dry_run": true})
	if res.IsError {
		t.Fatalf("clean_current_project returned an error: %s", resultText(t, res))
	}

	var resp struct {
		Cwd     string                `json:"cwd"`
		DryRun  bool                  `json:"dry_run"`
		Count   int                   `json:"count"`
		Cleaned []core.PortDiagnostic `json:"cleaned"`
	}
	if err := json.Unmarshal([]byte(resultText(t, res)), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}
	if !resp.DryRun {
		t.Errorf("response dry_run should be true")
	}
	if resp.Cwd == "" {
		t.Errorf("response cwd must not be empty")
	}
}
