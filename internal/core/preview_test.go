package core

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	gnet "github.com/shirou/gopsutil/v3/net"
)

// actionablePreview builds a synthetic, fully confirmed snapshot. Real
// inspections are covered elsewhere; these tests focus on the token contract.
func actionablePreview(port int, pid int32, created int64) PortDiagnostic {
	return PortDiagnostic{
		Port:           port,
		IsOpen:         true,
		Protocol:       ProtocolTCP,
		PID:            pid,
		Status:         StatusOccupied,
		ProcessName:    "node",
		processCreated: created,
	}
}

func TestPreviewTokenRoundTripPinsIdentity(t *testing.T) {
	original := actionablePreview(3000, 4242, 1700000000000)
	token, expiresAt, err := IssueKillPreview(original, time.Minute)
	if err != nil {
		t.Fatalf("issue preview token: %v", err)
	}
	if token == "" {
		t.Fatal("preview token must not be empty")
	}
	if expiresAt <= time.Now().Unix() {
		t.Fatalf("expiry %d should be in the future", expiresAt)
	}

	targets, restoredExpiry, err := RestorePreviewToken(PreviewScopeKill, token, []int{3000})
	if err != nil {
		t.Fatalf("restore preview token: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("restored %d targets, want 1", len(targets))
	}
	got := targets[0]
	if got.Port != original.Port || got.PID != original.PID {
		t.Fatalf("restored identity = port %d pid %d, want port %d pid %d", got.Port, got.PID, original.Port, original.PID)
	}
	if got.processCreated != original.processCreated {
		t.Fatalf("restored creation time = %d, want %d", got.processCreated, original.processCreated)
	}
	if restoredExpiry != expiresAt {
		t.Fatalf("restored expiry %d != issued expiry %d", restoredExpiry, expiresAt)
	}
}

func TestPreviewTokenRejectsTampering(t *testing.T) {
	target := actionablePreview(3000, 4242, 1700000000000)
	token, _, err := IssueKillPreview(target, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// Flip one payload character: the HMAC must reject it rather than decoding a
	// forged target with a different PID.
	bodyEnd := strings.Index(token, ".")
	if bodyEnd <= 0 {
		t.Fatalf("token has no payload separator: %q", token)
	}
	tampered := []byte(token)
	if tampered[0] == 'A' {
		tampered[0] = 'B'
	} else {
		tampered[0] = 'A'
	}
	if _, _, err := RestorePreviewToken(PreviewScopeKill, string(tampered), nil); !errors.Is(err, ErrPreviewTokenInvalid) {
		t.Fatalf("tampered token error = %v, want ErrPreviewTokenInvalid", err)
	}
}

func TestPreviewTokenRejectsForeignProcessSignature(t *testing.T) {
	target := actionablePreview(3000, 4242, 1700000000000)
	token, _, err := IssueKillPreview(target, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// A token minted under a different key stands in for "another process" or a
	// previous server run: it must never be replayable here.
	restoreKey := setPreviewSigningKeyForTesting([]byte("a-completely-different-signing-key"))
	defer restoreKey()
	if _, _, err := RestorePreviewToken(PreviewScopeKill, token, nil); !errors.Is(err, ErrPreviewTokenInvalid) {
		t.Fatalf("foreign-key token error = %v, want ErrPreviewTokenInvalid", err)
	}
}

func TestPreviewTokenExpires(t *testing.T) {
	target := actionablePreview(3000, 4242, 1700000000000)
	// Issue with the smallest meaningful lifetime, then wait it out.
	token, expiresAt, err := IssueKillPreview(target, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got := expiresAt - time.Now().Unix(); got > 2 {
		t.Fatalf("unexpectedly long lifetime: %d seconds", got)
	}
	// Rewriting the payload is impossible without the key, so wait for real expiry.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Unix() < expiresAt {
		if time.Now().After(deadline) {
			t.Fatal("token did not reach its expiry in time")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, _, err := RestorePreviewToken(PreviewScopeKill, token, []int{3000}); !errors.Is(err, ErrPreviewTokenExpired) {
		t.Fatalf("expired token error = %v, want ErrPreviewTokenExpired", err)
	}
}

func TestPreviewTokenRejectsScopeMismatch(t *testing.T) {
	target := actionablePreview(3000, 4242, 1700000000000)
	token, _, err := IssueKillPreview(target, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// A single-port kill preview must not authorize a project-wide cleanup.
	if _, _, err := RestorePreviewToken(PreviewScopeClean, token, nil); !errors.Is(err, ErrPreviewTokenInvalid) {
		t.Fatalf("scope mismatch error = %v, want ErrPreviewTokenInvalid", err)
	}
}

func TestPreviewTokenRequiresExactSelection(t *testing.T) {
	target := actionablePreview(3000, 4242, 1700000000000)
	token, _, err := IssueKillPreview(target, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		port int
	}{
		{name: "different port", port: 3001},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := RestorePreviewToken(PreviewScopeKill, token, []int{tc.port}); !errors.Is(err, ErrPreviewTokenInvalid) {
				t.Fatalf("selection mismatch error = %v, want ErrPreviewTokenInvalid", err)
			}
		})
	}

	// An explicit empty selection can never be authorized.
	if _, _, err := RestorePreviewToken(PreviewScopeKill, token, []int{}); !errors.Is(err, ErrPreviewTokenInvalid) {
		t.Fatalf("empty selection error = %v, want ErrPreviewTokenInvalid", err)
	}
}

func TestIssuePreviewTokenRejectsUnsafeTargets(t *testing.T) {
	base := actionablePreview(3000, 4242, 1700000000000)
	cases := []struct {
		name   string
		mutate func(*PortDiagnostic)
	}{
		{name: "free port", mutate: func(d *PortDiagnostic) { d.IsOpen = false }},
		{name: "ambiguous owner", mutate: func(d *PortDiagnostic) { d.ambiguousOwner = true }},
		{name: "protected pid", mutate: func(d *PortDiagnostic) { d.PID = 1 }},
		{name: "negative pid", mutate: func(d *PortDiagnostic) { d.PID = -1 }},
		{name: "unknown pid", mutate: func(d *PortDiagnostic) { d.PID = 0 }},
		{name: "permission required", mutate: func(d *PortDiagnostic) { d.Status = StatusPermissionRequired }},
		{name: "missing creation time", mutate: func(d *PortDiagnostic) { d.processCreated = 0 }},
		{name: "invalid port", mutate: func(d *PortDiagnostic) { d.Port = 70000 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := base
			tc.mutate(&target)
			if _, _, err := IssueKillPreview(target, time.Minute); err == nil {
				t.Fatalf("%s must not be previewable", tc.name)
			}
		})
	}

	if _, _, err := IssuePreviewToken(PreviewScopeKill, nil, time.Minute); err == nil {
		t.Fatal("an empty target list must not produce a token")
	}
	if _, _, err := IssuePreviewToken("bogus-scope", []PortDiagnostic{base}, time.Minute); err == nil {
		t.Fatal("an unknown scope must be rejected")
	}
}

func TestPreviewTokenDuplicatePortsRejected(t *testing.T) {
	target := actionablePreview(3000, 4242, 1700000000000)
	if _, _, err := IssuePreviewToken(PreviewScopeClean, []PortDiagnostic{target, target}, time.Minute); err == nil {
		t.Fatal("duplicate ports must not be silently collapsed into a token")
	}
}

func TestRestorePreviewTokenRejectsMalformedInput(t *testing.T) {
	for _, token := range []string{"", "no-separator", ".leading", "trailing.", "a.b.c"} {
		if _, _, err := RestorePreviewToken(PreviewScopeKill, token, nil); !errors.Is(err, ErrPreviewTokenInvalid) {
			t.Fatalf("token %q error = %v, want ErrPreviewTokenInvalid", token, err)
		}
	}
}

// TestKillPortTargetRejectsStalePreview is the end-to-end safety property: a
// token issued for a process that has since been replaced must fail the action
// rather than terminate the replacement.
func TestKillPortTargetRejectsStalePreview(t *testing.T) {
	ln, port := listenLoopback(t)
	ctx := context.Background()
	preview, err := InspectPort(ctx, port)
	if err != nil {
		t.Fatalf("inspect preview port: %v", err)
	}
	if preview.PID != int32(os.Getpid()) {
		t.Skipf("test process does not own the listener (pid %d)", preview.PID)
	}
	token, _, err := IssueKillPreview(*preview, time.Minute)
	if err != nil {
		t.Fatalf("issue preview token: %v", err)
	}
	_ = ln.Close()

	targets, _, err := RestorePreviewToken(PreviewScopeKill, token, []int{port})
	if err != nil {
		t.Fatalf("restore preview token: %v", err)
	}
	// The port is now free, so the action must fail instead of "helpfully" acting
	// on whatever claims the port next.
	if _, _, err := KillPortTarget(ctx, targets[0], true); err == nil {
		t.Fatal("a preview whose port was released must not be actionable")
	}
}

// TestDegradedInspectionsNeverClaimFree proves the P1 degraded-status contract.
func TestDegradedInspectionsNeverClaimFree(t *testing.T) {
	restore := SetConnectionsListerForTesting(func(context.Context) ([]gnet.ConnectionStat, error) {
		return nil, errors.New("simulated listener table unavailable")
	})
	defer restore()

	port := freeLoopbackPort(t)
	diag, err := InspectPort(context.Background(), port)
	if err != nil {
		t.Fatalf("InspectPort should degrade rather than fail: %v", err)
	}
	if !diag.ListenerTableMissing {
		t.Fatal("degraded inspection must set ListenerTableMissing")
	}
	if diag.Status != StatusUndetermined {
		t.Fatalf("degraded closed probe status = %q, want %q", diag.Status, StatusUndetermined)
	}
	if diag.Status == StatusFree {
		t.Fatal("a degraded probe must never report a port as free")
	}
	if diag.PID != 0 {
		t.Fatalf("degraded inspection must not invent an owner, got pid %d", diag.PID)
	}
}

// TestDegradedScanReportsIncompleteResult proves scans surface degradation while
// keeping the stable JSON array shape for existing consumers.
func TestDegradedScanReportsIncompleteResult(t *testing.T) {
	restore := SetConnectionsListerForTesting(func(context.Context) ([]gnet.ConnectionStat, error) {
		return nil, errors.New("simulated listener table unavailable")
	})
	defer restore()

	port := freeLoopbackPort(t)
	summary, err := ScanPortsConcurrentSummary(context.Background(), []int{port})
	if err != nil {
		t.Fatalf("degraded scan should not fail: %v", err)
	}
	if !summary.Degraded {
		t.Fatal("degraded scan must report Degraded")
	}
	if summary.Warning == "" {
		t.Fatal("degraded scan must carry a warning")
	}
	// ScanPortsConcurrent keeps its historical signature and shape.
	results, err := ScanPortsConcurrent(context.Background(), []int{port})
	if err != nil {
		t.Fatalf("ScanPortsConcurrent should still succeed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("a closed port must not appear in scan results: %+v", results)
	}
}

// TestDegradedScanStillReportsReachableListeners keeps genuinely open ports
// visible even when the listener table is unreadable.
func TestDegradedScanStillReportsReachableListeners(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })
	restore := SetConnectionsListerForTesting(func(context.Context) ([]gnet.ConnectionStat, error) {
		return nil, errors.New("simulated listener table unavailable")
	})
	defer restore()

	summary, err := ScanPortsConcurrentSummary(context.Background(), []int{port})
	if err != nil {
		t.Fatalf("degraded scan failed: %v", err)
	}
	if len(summary.Diagnostics) != 1 || summary.Diagnostics[0].Port != port {
		t.Fatalf("a reachable listener must still be reported: %+v", summary.Diagnostics)
	}
	if summary.Diagnostics[0].Status != StatusPermissionRequired {
		t.Fatalf("owner must stay unknown while degraded, got %q", summary.Diagnostics[0].Status)
	}
	if !summary.Degraded || summary.Warning == "" {
		t.Fatal("a partially known result must still be marked degraded")
	}
}
