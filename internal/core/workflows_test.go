package core

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	gnet "github.com/shirou/gopsutil/v3/net"
)

func TestResolvePresetPortsAndProjectValidation(t *testing.T) {
	for _, name := range []string{"web", "python", "database"} {
		ports, scope, err := ResolvePresetPorts(name, "")
		if err != nil || len(ports) == 0 || scope != "preset "+name {
			t.Fatalf("ResolvePresetPorts(%q) = %v, %q, %v", name, ports, scope, err)
		}
	}
	if _, _, err := ResolvePresetPorts("invalid", ""); err == nil {
		t.Fatal("invalid preset must be rejected")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module preset-demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".port-detective.json"), []byte(`{"ports":"8081,3000-3002"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ports, scope, err := ResolvePresetPorts("project", dir)
	if err != nil || len(ports) != 4 || ports[0] != 3000 || ports[3] != 8081 {
		t.Fatalf("ResolvePresetPorts(project) = %v, %q, %v", ports, scope, err)
	}
}

func TestFindFreePortAndWaitForPortCore(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	if _, err := FindFreePort(context.Background(), 4000, 3000); err == nil {
		t.Fatal("inverted range must be rejected")
	}
	if _, err := FindFreePort(context.Background(), port, port); !errors.Is(err, ErrNoFreePort) {
		t.Fatalf("occupied single-port FindFreePort err = %v, want ErrNoFreePort", err)
	}

	diag, err := WaitForPortProtocol(context.Background(), port, ProtocolTCP, "open", time.Second, 20*time.Millisecond)
	if err != nil || !diag.IsOpen || diag.Port != port {
		t.Fatalf("WaitForPort open failed: %+v, %v", diag, err)
	}
	if _, err := WaitForPort(context.Background(), port, "free", 60*time.Millisecond, 15*time.Millisecond); !errors.Is(err, ErrWaitTimeout) {
		t.Fatalf("expected ErrWaitTimeout for open port waiting free, got %v", err)
	}

	// Degraded table must never satisfy state="free".
	idle := freeLoopbackPort(t)
	restore := SetConnectionsListerForTesting(func(context.Context) ([]gnet.ConnectionStat, error) {
		return nil, os.ErrPermission
	})
	defer restore()
	if _, err := WaitForPort(context.Background(), idle, "free", 60*time.Millisecond, 15*time.Millisecond); !errors.Is(err, ErrWaitTimeout) {
		t.Fatalf("expected ErrWaitTimeout on degraded table, got %v", err)
	}
}

func TestRunDoctorCore(t *testing.T) {
	report, err := RunDoctor(context.Background(), "", "", "1.6.0")
	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}
	if report.Version != "1.6.0" || report.Cwd == "" || report.SelfCheck == nil {
		t.Fatalf("unexpected RunDoctor report: %+v", report)
	}
	if report.SelfCheck.Cmdline != "" || report.SelfCheck.Project != nil {
		t.Fatalf("RunDoctor must strip sensitive self_check fields: %+v", report.SelfCheck)
	}

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
	if _, err := WaitForPort(context.Background(), ln.Addr().(*net.TCPAddr).Port, "bad", time.Second, time.Millisecond); err == nil {
		t.Fatal("invalid wait state must fail")
	}
	if _, err := WaitForPort(context.Background(), 3000, "open", 0, time.Millisecond); err == nil {
		t.Fatal("zero timeout must fail")
	}
	_ = strconv.Itoa(3000)
}
