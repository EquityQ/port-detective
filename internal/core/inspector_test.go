package core

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	gnet "github.com/shirou/gopsutil/v3/net"
)

func TestInspectorInjectsDetectorWithoutMutatingGlobal(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	globalCalled := false
	previous := currentProjectDetector()
	RegisterProjectDetector(func(context.Context, int32) *ProjectInfo {
		globalCalled = true
		return &ProjectInfo{Name: "global-proj", Framework: "Global"}
	})
	t.Cleanup(func() { RegisterProjectDetector(previous) })

	inspector := NewInspector(func(context.Context, int32) *ProjectInfo {
		return &ProjectInfo{Name: "injected-proj", Framework: "Rust"}
	})
	diag, err := inspector.InspectPort(context.Background(), port)
	if err != nil {
		t.Fatalf("Inspector.InspectPort failed: %v", err)
	}
	if !diag.IsOpen || diag.PID != int32(os.Getpid()) {
		t.Fatalf("unexpected diagnostic: %+v", diag)
	}
	if globalCalled {
		t.Fatal("Inspector should not invoke the global project detector")
	}
	if diag.Project == nil || diag.Project.Name != "injected-proj" {
		t.Fatalf("expected injected project info, got %+v", diag.Project)
	}
	if len(diag.BindAddresses) == 0 {
		t.Fatalf("expected non-empty BindAddresses for live listener: %+v", diag)
	}

	scanned, err := inspector.ScanPortsConcurrent(context.Background(), []int{port})
	if err != nil || len(scanned) != 1 || scanned[0].Project == nil || scanned[0].Project.Name != "injected-proj" {
		t.Fatalf("Inspector.ScanPortsConcurrent returned %+v, %v", scanned, err)
	}

	projDir := projectTestDir(t)
	scopedInspector := NewInspector(func(_ context.Context, pid int32) *ProjectInfo {
		if pid == int32(os.Getpid()) {
			return &ProjectInfo{Name: "di-proj", Path: projDir}
		}
		return nil
	})
	discovered, err := scopedInspector.DiscoverProjectPorts(context.Background(), projDir, []int{port})
	if err != nil || len(discovered) != 1 || discovered[0].Port != port {
		t.Fatalf("Inspector.DiscoverProjectPorts failed: %+v, %v", discovered, err)
	}
	cleaned, err := scopedInspector.CleanCurrentProjectPorts(context.Background(), projDir, true)
	if err != nil || len(cleaned) != 1 {
		t.Fatalf("Inspector.CleanCurrentProjectPorts failed: %+v, %v", cleaned, err)
	}
	killedDiag, _, err := scopedInspector.KillPortProcess(context.Background(), port, true)
	if err != nil || killedDiag == nil || killedDiag.Project == nil || killedDiag.Project.Name != "di-proj" {
		t.Fatalf("Inspector.KillPortProcess failed: %+v, %v", killedDiag, err)
	}
	waitedDiag, err := scopedInspector.WaitForPort(context.Background(), port, "open", time.Second, 20*time.Millisecond)
	if err != nil || waitedDiag.Project == nil || waitedDiag.Project.Name != "di-proj" {
		t.Fatalf("Inspector.WaitForPort failed: %+v, %v", waitedDiag, err)
	}
	docReport, err := scopedInspector.RunDoctor(context.Background(), "", projDir, "1.5.0")
	if err != nil || docReport.SelfCheck == nil {
		t.Fatalf("Inspector.RunDoctor failed: %+v, %v", docReport, err)
	}
	if globalCalled {
		t.Fatal("Inspector workflow and cleanup methods must never invoke the global detector")
	}
}

func TestInspectAndScanUDP(t *testing.T) {
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp4 failed: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	port := pc.LocalAddr().(*net.UDPAddr).Port

	diag, err := InspectPortProtocol(context.Background(), port, ProtocolUDP)
	if err != nil {
		t.Fatalf("InspectPortProtocol(udp) failed: %v", err)
	}
	if !diag.IsOpen || diag.Protocol != ProtocolUDP {
		t.Fatalf("expected open UDP diagnostic, got %+v", diag)
	}
	if diag.PID != int32(os.Getpid()) {
		t.Logf("UDP socket PID = %d (host may hide or restrict UDP PID)", diag.PID)
	}

	summary, err := ScanPortsProtocolSummary(context.Background(), []int{port}, ProtocolUDP)
	if err != nil {
		t.Fatalf("ScanPortsProtocolSummary(udp) failed: %v", err)
	}
	if len(summary.Diagnostics) != 1 || summary.Diagnostics[0].Port != port || summary.Diagnostics[0].Protocol != ProtocolUDP {
		t.Fatalf("unexpected UDP scan result: %+v", summary)
	}

	if _, err := InspectPortProtocol(context.Background(), port, "sctp"); err == nil {
		t.Fatal("unsupported protocol must be rejected")
	}

	if err := waitForPortReleaseProtocol(context.Background(), port, ProtocolUDP, 30*time.Millisecond); err == nil {
		t.Fatal("open UDP socket must not be reported as released")
	}

	if diag.PID == int32(os.Getpid()) && diag.Status == StatusOccupied {
		dryDiag, msg, err := KillPortProcessProtocol(context.Background(), port, ProtocolUDP, true)
		if err != nil || dryDiag == nil || dryDiag.Protocol != ProtocolUDP || msg == "" {
			t.Fatalf("KillPortProcessProtocol(udp, dryRun) failed: diag=%+v msg=%q err=%v", dryDiag, msg, err)
		}
	}
	if _, _, err := KillPortProcessProtocol(context.Background(), port, "sctp", true); err == nil {
		t.Fatal("KillPortProcessProtocol must reject unsupported protocols")
	}

	_ = pc.Close()
	if err := waitForPortReleaseProtocol(context.Background(), port, ProtocolUDP, time.Second); err != nil {
		t.Fatalf("closed UDP socket release check failed: %v", err)
	}
}

func TestDegradedUDPReportsUndetermined(t *testing.T) {
	restore := SetUDPConnectionsListerForTesting(func(context.Context) ([]gnet.ConnectionStat, error) {
		return nil, errors.New("simulated udp table failure")
	})
	defer restore()

	diag, err := InspectPortProtocol(context.Background(), 5353, ProtocolUDP)
	if err != nil {
		t.Fatalf("degraded UDP inspection should degrade rather than fail: %v", err)
	}
	if !diag.ListenerTableMissing || diag.Status != StatusUndetermined || diag.Protocol != ProtocolUDP {
		t.Fatalf("unexpected degraded UDP diagnostic: %+v", diag)
	}

	summary, err := ScanPortsProtocolSummary(context.Background(), []int{5353}, ProtocolUDP)
	if err != nil {
		t.Fatalf("degraded UDP scan failed: %v", err)
	}
	if !summary.Degraded || summary.Warning == "" {
		t.Fatalf("expected degraded UDP scan warning, got %+v", summary)
	}
}
