package core

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gnet "github.com/shirou/gopsutil/v3/net"
)

func TestParsePortsListRangesAndValidation(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want []int
	}{
		{"", []int{}},
		{" 8082,8080-8082, 3000,3000 ", []int{3000, 8080, 8081, 8082}},
		{"1,65535", []int{1, 65535}},
		{"10 - 10", []int{10}},
	} {
		got, err := ParsePortsList(tc.raw)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParsePortsList(%q) = %v, %v; want %v", tc.raw, got, err, tc.want)
		}
	}
	for _, raw := range []string{"0", "65536", "1,,2", "1,", "-1", "10-1", "1-2-3", "abc", "1.2", "+80", "1-", "1-9999999999999999999999"} {
		if _, err := ParsePortsList(raw); err == nil {
			t.Errorf("invalid input %q was accepted", raw)
		}
	}
}

func TestResolveProjectPathNearestManifest(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "services", "api", "src")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveProjectPath(child)
	if err != nil || !projectPathsMatch(got, root) {
		t.Fatalf("nested directory resolved to %q, %v; want %q", got, err, root)
	}
	nearest := filepath.Dir(child)
	if err := os.WriteFile(filepath.Join(nearest, "package.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = ResolveProjectPath(child)
	if err != nil || !projectPathsMatch(got, nearest) {
		t.Fatalf("nearest manifest resolved to %q, %v; want %q", got, err, nearest)
	}
	if _, err := ResolveProjectPath(filepath.Join(root, "absent")); err == nil {
		t.Fatal("missing directory was accepted")
	}
	if _, err := ResolveProjectPath(filepath.Join(root, "go.mod")); err == nil {
		t.Fatal("file path was accepted as a project directory")
	}
}

func projectTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module preview-test"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInspectSpecificInterfaceListener(t *testing.T) {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Skipf("local interfaces unavailable: %v", err)
	}
	var listener net.Listener
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err != nil || ip.IsLoopback() || ip.To4() == nil {
			continue
		}
		listener, err = net.Listen("tcp4", net.JoinHostPort(ip.String(), "0"))
		if err == nil {
			break
		}
	}
	if listener == nil {
		t.Skip("no bindable non-loopback IPv4 interface")
	}
	t.Cleanup(func() { _ = listener.Close() })
	port := listener.Addr().(*net.TCPAddr).Port
	if checkTCPOpen(port, DefaultDialTimeout) {
		t.Skip("port also has a reachable loopback binding")
	}
	diag, err := InspectPort(context.Background(), port)
	if err != nil || !diag.IsOpen || diag.PID != testHelperPID {
		t.Fatalf("specific-interface listener missed: %+v, %v", diag, err)
	}
	results, err := ScanPortsConcurrent(context.Background(), []int{port, port, port})
	if err != nil || len(results) != 1 || results[0].Port != port {
		t.Fatalf("scan should return the interface-bound port once: %+v, %v", results, err)
	}
	if err := waitForPortRelease(context.Background(), port, 30*time.Millisecond); err == nil {
		t.Fatal("non-loopback listener was incorrectly reported as released")
	}
	_ = listener.Close()
	if err := waitForPortRelease(context.Background(), port, time.Second); err != nil {
		t.Fatalf("closed listener release check failed: %v", err)
	}
}

func TestListenerIndexRejectsAmbiguousOwners(t *testing.T) {
	connections := []gnet.ConnectionStat{
		{Type: sockStream, Status: "LISTEN", Laddr: gnet.Addr{IP: "127.0.0.1", Port: 3000}, Pid: 50},
		{Type: sockStream, Status: "LISTEN", Laddr: gnet.Addr{IP: "192.0.2.1", Port: 3000}, Pid: 51},
		{Type: sockStream, Status: "LISTEN", Laddr: gnet.Addr{Port: 4000}, Pid: 50},
		{Type: sockStream, Status: "LISTEN", Laddr: gnet.Addr{Port: 4000}, Pid: 50},
		{Type: sockStream, Status: "LISTEN", Laddr: gnet.Addr{Port: 5000}},
		{Type: sockStream, Status: "ESTABLISHED", Laddr: gnet.Addr{Port: 6000}, Pid: 50},
	}
	index := indexTCPListeners(connections)
	if len(index) != 3 || index[connKey{3000, sockStream}] != -1 || index[connKey{4000, sockStream}] != 50 {
		t.Fatalf("listener ownership index is incorrect: %v", index)
	}
	diag := inspectPortInternal(context.Background(), 3000, index)
	if !diag.IsOpen || !diag.ambiguousOwner || diag.Status != StatusPermissionRequired {
		t.Fatalf("ambiguous binding must remain occupied and protected: %+v", diag)
	}
	if _, _, err := KillPortTarget(context.Background(), *diag, false); err == nil || !strings.Contains(err.Error(), "multiple or unknown owners") {
		t.Fatalf("ambiguous owner termination was not rejected: %v", err)
	}
	unknown := inspectPortInternal(context.Background(), 5000, index)
	if !unknown.IsOpen || unknown.PID != 0 || unknown.Status != StatusPermissionRequired {
		t.Fatalf("hidden owner listener was discarded: %+v", unknown)
	}
}

func TestScanDeduplicatesAndCancelsDuringInspection(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })
	results, err := ScanPortsConcurrent(context.Background(), []int{port, port, port})
	if err != nil || len(results) != 1 {
		t.Fatalf("repeated port results = %+v, %v", results, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	var once sync.Once
	RegisterProjectDetector(func(ctx context.Context, pid int32) *ProjectInfo {
		if pid == testHelperPID {
			once.Do(func() { close(started) })
			<-ctx.Done()
		}
		return nil
	})
	defer RegisterProjectDetector(nil)
	done := make(chan error, 1)
	go func() {
		_, err := ScanPortsConcurrent(ctx, []int{port})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("scan did not begin process inspection")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("in-flight cancellation = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled scan did not release workers")
	}
}

func TestDiscoverAndCleanProjectPreviewSnapshot(t *testing.T) {
	first, firstPort := listenLoopback(t)
	t.Cleanup(func() { _ = first.Close() })
	project := projectTestDir(t)
	for _, defaultPort := range DefaultDevPorts {
		if firstPort == defaultPort {
			t.Skip("ephemeral listener unexpectedly uses a default development port")
		}
	}
	RegisterProjectDetector(func(_ context.Context, pid int32) *ProjectInfo {
		if pid == testHelperPID {
			return &ProjectInfo{Name: "snapshot", Path: project}
		}
		return nil
	})
	defer RegisterProjectDetector(nil)
	preview, err := DiscoverProjectPorts(context.Background(), project, nil)
	if err != nil || len(preview) != 1 || preview[0].Port != firstPort {
		t.Fatalf("non-default project listener was not discovered: %+v, %v", preview, err)
	}
	second, secondPort := listenLoopback(t)
	t.Cleanup(func() { _ = second.Close() })
	cleaned, err := CleanProjectTargets(context.Background(), project, preview, true)
	if err != nil || len(cleaned) != 1 || cleaned[0].Port != firstPort {
		t.Fatalf("cleanup must use only the original preview: %+v, %v", cleaned, err)
	}
	subset, err := DiscoverProjectPorts(context.Background(), project, []int{secondPort, secondPort})
	if err != nil || len(subset) != 1 || subset[0].Port != secondPort {
		t.Fatalf("explicit subset discovery = %+v, %v", subset, err)
	}
	empty, err := DiscoverProjectPorts(context.Background(), project, []int{})
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("explicit empty selection = %+v, %v", empty, err)
	}
	all, err := CleanCurrentProjectPorts(context.Background(), project, true)
	if err != nil || len(all) != 2 {
		t.Fatalf("same-process multi-port preview = %+v, %v", all, err)
	}
	wrong := preview[0]
	wrong.processCreated++
	if _, _, err := KillPortTarget(context.Background(), wrong, true); err == nil || !strings.Contains(err.Error(), "was reused") {
		t.Fatalf("snapshot identity change was accepted: %v", err)
	}
	if _, err := CleanProjectTargets(context.Background(), projectTestDir(t), preview, true); err == nil {
		t.Fatal("preview for a different project was accepted")
	}
	if _, err := CleanProjectTargets(context.Background(), project, []PortDiagnostic{preview[0], wrong}, true); err == nil {
		t.Fatal("conflicting snapshot identities were accepted")
	}
	for _, port := range []int{firstPort, secondPort} {
		conn, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
		if err != nil {
			t.Fatalf("preview/rejected cleanup changed listener %d: %v", port, err)
		}
		_ = conn.Close()
	}
}
