package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	gnet "github.com/shirou/gopsutil/v3/net"

	"github.com/EquityQ/port-detective/internal/core"
	"github.com/EquityQ/port-detective/internal/fingerprint"
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
	t.Setenv(core.PreviewKeyFileEnv, filepath.Join(t.TempDir(), "preview-key.json"))
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

// TestKillRejectsBadPreviewIDOnFreePort proves token validation runs before live
// probing, so a forged token fails with exit status 6 even on an idle port.
func TestKillRejectsBadPreviewIDOnFreePort(t *testing.T) {
	port := freeLoopbackPortCLI(t)
	_, errOut, status := runWithStderr(t, "kill", strconv.Itoa(port), "--json", "--yes", "--preview-id", "forged-token-value")
	if status != 6 {
		t.Fatalf("status = %d, want 6 (preview_rejected)\nstderr: %s", status, errOut)
	}
	if !strings.Contains(errOut, "preview_rejected") {
		t.Fatalf("stderr should report preview_rejected: %s", errOut)
	}
}

// TestCleanRejectsBadPreviewIDWithoutListeners proves clean validates --preview-id
// before discovering live listeners, so a forged token is never silently ignored.
func TestCleanRejectsBadPreviewIDWithoutListeners(t *testing.T) {
	dir := t.TempDir()
	writeCLIManifest(t, dir)

	_, errOut, status := runWithStderr(t, "clean", "--project", dir, "--json", "--yes", "--preview-id", "forged-token-value")
	if status != 6 {
		t.Fatalf("status = %d, want 6 (preview_rejected)\nstderr: %s", status, errOut)
	}
	if !strings.Contains(errOut, "preview_rejected") {
		t.Fatalf("stderr should report preview_rejected: %s", errOut)
	}
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
	t.Cleanup(func() { core.RegisterProjectDetector(fingerprint.DetectProject) })

	out, _, err := captureRun(t, "clean", "--project", dir, "--ports", strconv.Itoa(port), "--json", "--dry-run", "--yes")
	if err != nil {
		t.Fatalf("clean --json --dry-run failed: %v", err)
	}
	var diags []core.PortDiagnostic
	if err := json.Unmarshal([]byte(out), &diags); err != nil {
		t.Fatalf("clean --json must stay a JSON array: %v\nraw: %s", err, out)
	}
}

func TestCleanJSONEnvelopeEmitsPreviewTokenAndRedeems(t *testing.T) {
	t.Setenv(core.PreviewKeyFileEnv, filepath.Join(t.TempDir(), "preview-key.json"))
	dir := t.TempDir()
	writeCLIManifest(t, dir)

	// --envelope without --json must fail with exit status 2.
	_, errOut, status := runWithStderr(t, "clean", "--project", dir, "--envelope", "--dry-run")
	if status != 2 {
		t.Fatalf("status = %d, want 2 (--envelope requires --json)\nstderr: %s", status, errOut)
	}

	// Empty project scope with --envelope returns a structured object without a preview token.
	idlePort := freeLoopbackPortCLI(t)
	emptyOut, _, emptyStatus := runWithStderr(t, "clean", "--project", dir, "--ports", strconv.Itoa(idlePort), "--json", "--envelope", "--dry-run")
	if emptyStatus != 0 {
		t.Fatalf("empty clean --envelope failed with status %d", emptyStatus)
	}
	var emptyPayload cleanEnvelope
	if err := json.Unmarshal([]byte(emptyOut), &emptyPayload); err != nil {
		t.Fatalf("empty clean envelope invalid JSON: %v\nraw: %s", err, emptyOut)
	}
	if emptyPayload.Count != 0 || emptyPayload.PreviewID != "" || emptyPayload.Cleaned == nil {
		t.Fatalf("unexpected empty clean envelope: %+v", emptyPayload)
	}

	ln, port := listenLoopbackCLI(t)
	t.Cleanup(func() { _ = ln.Close() })
	core.RegisterProjectDetector(func(context.Context, int32) *core.ProjectInfo {
		return &core.ProjectInfo{Name: "cli-preview", Path: dir}
	})
	t.Cleanup(func() { core.RegisterProjectDetector(fingerprint.DetectProject) })

	out, _, err := captureRun(t, "clean", "--project", dir, "--ports", strconv.Itoa(port), "--json", "--envelope", "--dry-run")
	if err != nil {
		t.Fatalf("clean --json --envelope --dry-run failed: %v", err)
	}
	var payload cleanEnvelope
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("clean envelope invalid JSON: %v\nraw: %s", err, out)
	}
	if !payload.DryRun || payload.Count != 1 || len(payload.Cleaned) != 1 || payload.PreviewID == "" || payload.PreviewExpiresAtUnix <= 0 {
		t.Fatalf("unexpected clean envelope: %+v", payload)
	}

	redeemedOut, _, err := captureRun(t, "clean", "--project", dir, "--ports", strconv.Itoa(port), "--json", "--envelope", "--dry-run", "--preview-id", payload.PreviewID)
	if err != nil {
		t.Fatalf("redeeming clean envelope preview_id failed: %v", err)
	}
	var redeemed cleanEnvelope
	if err := json.Unmarshal([]byte(redeemedOut), &redeemed); err != nil {
		t.Fatal(err)
	}
	if redeemed.Count != 1 || redeemed.Cleaned[0].Port != port {
		t.Fatalf("unexpected redeemed clean envelope: %+v", redeemed)
	}
}

func TestKillProtocolUDPDryRunAndMismatchRejection(t *testing.T) {
	t.Setenv(core.PreviewKeyFileEnv, filepath.Join(t.TempDir(), "preview-key.json"))
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp4 failed: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	port := pc.LocalAddr().(*net.UDPAddr).Port

	checkOut, _, err := captureRun(t, "check", strconv.Itoa(port), "--protocol", "udp")
	if err != nil {
		t.Fatalf("check --protocol udp failed: %v", err)
	}
	if strings.Contains(checkOut, "Release:") && !strings.Contains(checkOut, "--protocol udp") {
		t.Fatalf("UDP check release hint must mention --protocol udp:\n%s", checkOut)
	}
	scanOut, _, err := captureRun(t, "scan", "--ports", strconv.Itoa(port), "--protocol", "udp")
	if err != nil {
		t.Fatalf("scan --protocol udp failed: %v", err)
	}
	if !strings.Contains(scanOut, "--protocol udp") {
		t.Fatalf("UDP scan footer must mention --protocol udp:\n%s", scanOut)
	}

	diag, err := core.InspectPortProtocol(context.Background(), port, core.ProtocolUDP)
	if err != nil {
		t.Fatal(err)
	}
	if diag.Status != core.StatusOccupied || diag.PID != int32(os.Getpid()) {
		t.Skip("host restricts UDP process metadata for current process")
	}

	out, _, err := captureRun(t, "kill", strconv.Itoa(port), "--protocol", "udp", "--json", "--dry-run")
	if err != nil {
		t.Fatalf("kill --protocol udp --json --dry-run failed: %v", err)
	}
	var payload struct {
		Port      int    `json:"port"`
		Protocol  string `json:"protocol"`
		DryRun    bool   `json:"dry_run"`
		PreviewID string `json:"preview_id"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Protocol != core.ProtocolUDP || payload.PreviewID == "" {
		t.Fatalf("unexpected UDP kill preview payload: %+v", payload)
	}

	// Redeeming a UDP preview token with --protocol tcp must fail with exit status 6.
	_, errOut, status := runWithStderr(t, "kill", strconv.Itoa(port), "--protocol", "tcp", "--json", "--yes", "--preview-id", payload.PreviewID)
	if status != 6 {
		t.Fatalf("status = %d, want 6 for protocol mismatch\nstderr: %s", status, errOut)
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

// TestWaitFreeDoesNotTreatUndeterminedAsFree proves wait --state free will not
// declare a port free when the local listener table cannot be read.
func TestWaitFreeDoesNotTreatUndeterminedAsFree(t *testing.T) {
	port := freeLoopbackPortCLI(t)
	restore := core.SetConnectionsListerForTesting(func(context.Context) ([]gnet.ConnectionStat, error) {
		return nil, os.ErrPermission
	})
	defer restore()

	_, errOut, status := runWithStderr(t, "wait", strconv.Itoa(port), "--state", "free", "--timeout", "120ms", "--interval", "20ms", "--json")
	if status != 3 {
		t.Fatalf("wait --state free with unreadable listener table exited %d, want 3 (timeout)\nstderr: %s", status, errOut)
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

// TestCLIPreviewHelperProcess runs a single CLI invocation inside a fresh OS
// process for TestKillPreviewTokenWorksAcrossCLIProcesses.
func TestCLIPreviewHelperProcess(t *testing.T) {
	if os.Getenv("PORT_DETECTIVE_CLI_HELPER") != "1" {
		return
	}
	sep := -1
	for i, arg := range os.Args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep < 0 {
		os.Exit(2)
	}
	root := NewRootCommand()
	root.SetArgs(os.Args[sep+1:])
	os.Exit(executeCommand(root))
}

// TestKillPreviewTokenWorksAcrossCLIProcesses proves that a preview_id minted
// by one CLI process can be redeemed by a separate CLI process against the
// same live listener, without ever killing the test process.
func TestKillPreviewTokenWorksAcrossCLIProcesses(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "preview-key.json")
	ln, port := listenLoopbackCLI(t)
	t.Cleanup(func() { _ = ln.Close() })

	runSubprocess := func(args ...string) (string, string, int) {
		cmdArgs := append([]string{"-test.run=^TestCLIPreviewHelperProcess$", "--"}, args...)
		cmd := exec.Command(os.Args[0], cmdArgs...)
		cmd.Env = append(os.Environ(),
			"PORT_DETECTIVE_CLI_HELPER=1",
			core.PreviewKeyFileEnv+"="+keyFile,
		)
		var out, errOut bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errOut
		err := cmd.Run()
		code := 0
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				code = exitErr.ExitCode()
			} else {
				t.Fatalf("subprocess exec failed: %v", err)
			}
		}
		return out.String(), errOut.String(), code
	}

	// Process 1: preview the live listener and mint preview_id.
	out1, err1, code1 := runSubprocess("kill", strconv.Itoa(port), "--json", "--dry-run")
	if code1 != 0 {
		t.Fatalf("process 1 preview failed (code %d): %s", code1, err1)
	}
	var preview struct {
		PreviewID string `json:"preview_id"`
	}
	if err := json.Unmarshal([]byte(out1), &preview); err != nil || preview.PreviewID == "" {
		t.Fatalf("process 1 did not emit preview_id: %v\nraw: %s", err, out1)
	}

	// Process 2: redeem the preview_id in a fresh OS process (using --dry-run
	// so the test process itself is never terminated, while still exercising the
	// full RestoreCLIPreviewToken + KillPortTarget revalidation path).
	out2, err2, code2 := runSubprocess("kill", strconv.Itoa(port), "--json", "--dry-run", "--preview-id", preview.PreviewID)
	if code2 != 0 {
		t.Fatalf("process 2 rejected cross-process preview_id (code %d): %s\nstdout: %s", code2, err2, out2)
	}

	// Process 3: closing the listener must cause drift rejection when redeeming.
	_ = ln.Close()
	_, err3, code3 := runSubprocess("kill", strconv.Itoa(port), "--json", "--dry-run", "--preview-id", preview.PreviewID)
	if code3 == 0 {
		t.Fatalf("process 3 accepted a preview token after the port was released: %s", err3)
	}
}

func TestNewRootCommandWithInspectorIsolatesGlobalDetector(t *testing.T) {
	ln, port := listenLoopbackCLI(t)
	t.Cleanup(func() { _ = ln.Close() })

	globalCalled := false
	core.RegisterProjectDetector(func(context.Context, int32) *core.ProjectInfo {
		globalCalled = true
		return &core.ProjectInfo{Name: "global", Framework: "Global"}
	})
	t.Cleanup(func() { core.RegisterProjectDetector(fingerprint.DetectProject) })

	root := NewRootCommandWithInspector(core.NewInspector(func(context.Context, int32) *core.ProjectInfo {
		return &core.ProjectInfo{Name: "cli-injected", Framework: "Hono"}
	}))
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"check", strconv.Itoa(port), "--json"})
	if code := executeCommand(root); code != 0 {
		t.Fatalf("check failed (%d): %s", code, errOut.String())
	}
	if globalCalled {
		t.Fatal("injected CLI inspector must not call global detector")
	}
	var diag core.PortDiagnostic
	if err := json.Unmarshal(out.Bytes(), &diag); err != nil {
		t.Fatal(err)
	}
	if diag.Project == nil || diag.Project.Name != "cli-injected" {
		t.Fatalf("expected injected project metadata, got %+v", diag.Project)
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
