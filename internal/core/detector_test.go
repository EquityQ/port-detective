package core

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProjectPathsMatchUsesPlatformCaseRules(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Clean(dir)
	variant := filepath.Clean(dir + "_different")
	if runtime.GOOS == "windows" {
		variant = filepath.Clean(strings.ToUpper(dir))
		if !projectPathsMatch(canonical, variant) {
			t.Fatalf("Windows path comparison should ignore case: %q vs %q", canonical, variant)
		}
		return
	}
	if projectPathsMatch(canonical, variant) {
		t.Fatalf("Unix path comparison should reject different paths: %q vs %q", canonical, variant)
	}
}

func TestProjectPathsMatchUnicodePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "项目-服务")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("create Unicode project directory: %v", err)
	}
	if !projectPathsMatch(path, path) {
		t.Fatalf("identical Unicode project paths should match: %q", path)
	}
}

var testHelperPID = int32(os.Getpid())

// listenLoopback listens on an ephemeral port assigned by the OS on IPv4 loopback.
func listenLoopback(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("无法创建本地临时监听端口：%v", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		t.Fatalf("监听地址类型异常：%T", ln.Addr())
	}
	return ln, addr.Port
}

// freeLoopbackPort returns a port that is known to be free.
func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	ln, port := listenLoopback(t)
	if err := ln.Close(); err != nil {
		t.Fatalf("关闭临时监听端口失败：%v", err)
	}
	return port
}

// TestInspectPortOpenAndIdentifyProcess opens an ephemeral port and verifies that
// InspectPort reports it as open and associates process information correctly.
func TestInspectPortOpenAndIdentifyProcess(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	ctx := context.Background()
	diag, err := InspectPort(ctx, port)
	if err != nil {
		t.Fatalf("InspectPort(%d) 返回意外错误：%v", port, err)
	}
	if diag == nil {
		t.Fatalf("InspectPort(%d) 返回了 nil 结果", port)
	}

	if !diag.IsOpen {
		t.Fatalf("端口 %d 正在监听，但 IsOpen = false", port)
	}
	if diag.Port != port {
		t.Errorf("Port = %d，期望 %d", diag.Port, port)
	}
	if diag.Protocol != ProtocolTCP {
		t.Errorf("Protocol = %q，期望 %q", diag.Protocol, ProtocolTCP)
	}
	if diag.PID != testHelperPID {
		t.Errorf("PID = %d，期望 %d（测试进程自身）", diag.PID, testHelperPID)
	}
	if diag.ProcessName == "" {
		t.Errorf("进程名 ProcessName 为空，未能补全进程详情")
	}
	if diag.Status != StatusOccupied && diag.Status != StatusPermissionRequired {
		t.Errorf("状态异常：%q", diag.Status)
	}
	t.Logf("端口 %d -> PID=%d ProcessName=%q MemoryMB=%.2f CPU=%.2f%% Status=%s",
		diag.Port, diag.PID, diag.ProcessName, diag.MemoryMB, diag.CPUPercent, diag.Status)
}

// TestInspectPortIPv6Listener verifies loopback probing on IPv6-capable hosts.
func TestInspectPortIPv6Listener(t *testing.T) {
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback is unavailable on this host: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected IPv6 listener address type: %T", ln.Addr())
	}
	probe, err := net.DialTimeout("tcp6", net.JoinHostPort("::1", strconv.Itoa(addr.Port)), time.Second)
	if err != nil {
		t.Skipf("IPv6 loopback listener is not reachable on this host: %v", err)
	}
	_ = probe.Close()

	diag, err := InspectPort(context.Background(), addr.Port)
	if err != nil {
		t.Fatalf("InspectPort(%d) returned an unexpected error: %v", addr.Port, err)
	}
	if !diag.IsOpen {
		t.Fatalf("IPv6 listener on port %d was not detected", addr.Port)
	}
}

// TestInspectPortClosed verifies that a free port has IsOpen false and status "free".
func TestInspectPortClosed(t *testing.T) {
	port := freeLoopbackPort(t)

	diag, err := InspectPort(context.Background(), port)
	if err != nil {
		t.Fatalf("InspectPort(%d) 返回意外错误：%v", port, err)
	}
	if diag.IsOpen {
		t.Fatalf("端口 %d 已关闭，但 IsOpen = true", port)
	}
	if diag.Status != StatusFree {
		t.Errorf("空闲端口 Status = %q，期望 %q", diag.Status, StatusFree)
	}
	if diag.PID != 0 {
		t.Errorf("空闲端口的 PID 应为 0，实际为 %d", diag.PID)
	}
}

// TestInspectPortRejectsInvalidPort covers invalid port argument validation.
func TestInspectPortRejectsInvalidPort(t *testing.T) {
	ctx := context.Background()
	for _, port := range []int{0, -1, 65536, 100000} {
		if _, err := InspectPort(ctx, port); err == nil {
			t.Errorf("InspectPort(%d) 期望返回错误，实际返回 nil", port)
		}
	}
}

// TestInspectPortContextCancel verifies that cancellation is returned to the caller
// instead of being reported as an ordinary free-port diagnostic.
func TestInspectPortContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	diag, err := InspectPort(ctx, freeLoopbackPort(t))
	if diag != nil {
		t.Fatalf("取消的检查不应返回诊断报告：%+v", diag)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("期望返回 context.Canceled，实际为 %v", err)
	}
}

// TestScanPortsConcurrentDetectsAllListeners opens temporary listeners and verifies
// that concurrent scanning returns all occupied ports in ascending order.
func TestScanPortsConcurrentDetectsAllListeners(t *testing.T) {
	const count = 3
	listeners := make([]net.Listener, count)
	ports := make([]int, count)
	for i := 0; i < count; i++ {
		ln, p := listenLoopback(t)
		listeners[i] = ln
		ports[i] = p
	}
	t.Cleanup(func() {
		for _, ln := range listeners {
			_ = ln.Close()
		}
	})

	// Include one free port.
	freePort := freeLoopbackPort(t)
	testPorts := append([]int{freePort}, ports...)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results, err := ScanPortsConcurrent(ctx, testPorts)
	if err != nil {
		t.Fatalf("ScanPortsConcurrent 返回错误：%v", err)
	}

	// Results must be sorted in ascending order.
	if !sort.SliceIsSorted(results, func(i, j int) bool { return results[i].Port < results[j].Port }) {
		t.Errorf("扫描结果未按端口号升序排序：%+v", results)
	}

	// Verify that every test listener is included.
	found := make(map[int]bool)
	for _, r := range results {
		found[r.Port] = true
		if !r.IsOpen {
			t.Errorf("结果中包含非开启端口：%d", r.Port)
		}
	}
	for _, p := range ports {
		if !found[p] {
			t.Errorf("端口 %d 处于监听，但并发扫描未返回", p)
		}
	}
	if found[freePort] {
		t.Errorf("空闲端口 %d 不应出现在扫描结果中", freePort)
	}
}

// TestScanPortsConcurrentContextCancel verifies prompt exit after context cancellation.
func TestScanPortsConcurrentContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := ScanPortsConcurrent(ctx, DefaultDevPorts)
	if err == nil {
		t.Fatal("上下文已取消，期望返回错误，实际返回 nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("错误未包装 context.Canceled：%v", err)
	}
}

func TestScanPortsConcurrentRejectsInvalidPort(t *testing.T) {
	for _, port := range []int{0, -1, 65536, 100000} {
		results, err := ScanPortsConcurrent(context.Background(), []int{port})
		if err == nil {
			t.Errorf("ScanPortsConcurrent(%d) expected an error", port)
		}
		if results == nil {
			t.Errorf("ScanPortsConcurrent(%d) should return an empty result slice", port)
		}
	}
}

func TestIsTCPListenerFiltersConnectionStates(t *testing.T) {
	for _, test := range []struct {
		status string
		want   bool
	}{
		{status: "LISTEN", want: true},
		{status: "listen", want: true},
		{status: "ESTABLISHED", want: false},
		{status: "TIME_WAIT", want: false},
		{status: "", want: false},
	} {
		if got := isTCPListener(test.status); got != test.want {
			t.Errorf("isTCPListener(%q) = %v, want %v", test.status, got, test.want)
		}
	}
}

// TestKillPortProcessDryRun verifies dry-run behavior and process safety guards.
func TestKillPortProcessDryRun(t *testing.T) {
	ctx := context.Background()

	// Test a free port.
	idle := freeLoopbackPort(t)
	_, _, err := KillPortProcess(ctx, idle, true)
	if err == nil {
		t.Errorf("空闲端口 %d 执行 KillPortProcess 期望报错，实际返回 nil", idle)
	}

	// Verify dry-run succeeds for a listener owned by this test process.
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	diag, msg, err := KillPortProcess(ctx, port, true)
	if err != nil {
		t.Fatalf("dry-run 预演失败：%v", err)
	}
	if diag == nil || !diag.IsOpen {
		t.Fatalf("预演返回的诊断报告异常：%+v", diag)
	}
	if msg == "" {
		t.Errorf("预演应返回明确的提示消息")
	}
	t.Logf("dry-run 预演消息: %s", msg)

	// Ensure the listener is still alive; dry-run must never terminate it.
	if !checkTCPOpen(port, DefaultDialTimeout) {
		t.Errorf("dry-run 不应结束进程，但端口 %d 已无法连接", port)
	}
}

// TestKillPortProcessRejectsInvalidPort covers invalid port inputs.
func TestKillPortProcessRejectsInvalidPort(t *testing.T) {
	ctx := context.Background()
	if _, _, err := KillPortProcess(ctx, 0, true); err == nil {
		t.Error("端口 0 非法，期望返回错误，实际返回 nil")
	}
}

// TestKillPortProcessCanceledContext verifies that cancellation prevents both
// process lookup and termination.
func TestKillPortProcessCanceledContext(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := KillPortProcess(ctx, port, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if !checkTCPOpen(port, DefaultDialTimeout) {
		t.Fatal("canceled termination must leave the listener alive")
	}
}

// TestScanPortsConcurrentIsRaceFree verifies concurrent scans are race-free.
func TestScanPortsConcurrentIsRaceFree(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = ScanPortsConcurrent(ctx, []int{port, 80, 443})
		}()
	}
	wg.Wait()
}

// TestRegisterProjectDetectorHook verifies detector hook registration and report integration.
func TestRegisterProjectDetectorHook(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	dummyInfo := &ProjectInfo{
		Name:      "test-app",
		Framework: "Gin",
		Path:      "/tmp/test-app",
		Version:   "1.0.0",
	}

	RegisterProjectDetector(func(ctx context.Context, pid int32) *ProjectInfo {
		if pid == testHelperPID {
			return dummyInfo
		}
		return nil
	})
	defer RegisterProjectDetector(nil)

	diag, err := InspectPort(context.Background(), port)
	if err != nil {
		t.Fatalf("InspectPort 失败: %v", err)
	}
	if diag.Project == nil {
		t.Fatal("期望 diag.Project 不为空")
	}
	if diag.Project.Framework != "Gin" || diag.Project.Name != "test-app" {
		t.Errorf("Project 信息不符合预期: %+v", diag.Project)
	}
}

// TestRegisterProjectDetectorPanicIsRecovered ensures a third-party detector
// cannot crash a port inspection when its own implementation panics.
func TestRegisterProjectDetectorPanicIsRecovered(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	RegisterProjectDetector(func(context.Context, int32) *ProjectInfo {
		panic("detector failure")
	})
	defer RegisterProjectDetector(nil)

	diag, err := InspectPort(context.Background(), port)
	if err != nil {
		t.Fatalf("InspectPort 失败: %v", err)
	}
	if diag.Project != nil {
		t.Fatalf("panic 的项目探测器不应返回项目结果: %+v", diag.Project)
	}
}

// TestCleanCurrentProjectPorts verifies project-scoped port cleanup.
func TestCleanCurrentProjectPorts(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	testDir := t.TempDir()

	// Save and restore DefaultDevPorts.
	origPorts := DefaultDevPorts
	DefaultDevPorts = []int{port}
	defer func() { DefaultDevPorts = origPorts }()

	RegisterProjectDetector(func(ctx context.Context, pid int32) *ProjectInfo {
		if pid == testHelperPID {
			return &ProjectInfo{
				Name:      "clean-demo",
				Framework: "Next.js",
				Path:      testDir,
			}
		}
		return nil
	})
	defer RegisterProjectDetector(nil)

	// Preview cleanup for a matching project directory.
	cleaned, err := CleanCurrentProjectPorts(context.Background(), testDir, true)
	if err != nil {
		t.Fatalf("CleanCurrentProjectPorts dry-run 失败: %v", err)
	}
	if len(cleaned) != 1 {
		t.Fatalf("期望预演清理 1 个端口，实际清理 %d 个", len(cleaned))
	}
	if cleaned[0].Port != port {
		t.Errorf("清理端口为 %d，期望 %d", cleaned[0].Port, port)
	}

	// A non-matching directory should return an empty list.
	cleanedEmpty, err := CleanCurrentProjectPorts(context.Background(), "/non-existent-dir-12345", true)
	if err != nil {
		t.Fatalf("CleanCurrentProjectPorts 失败: %v", err)
	}
	if len(cleanedEmpty) != 0 {
		t.Errorf("非匹配目录期望返回 0 个端口，实际为 %d", len(cleanedEmpty))
	}
}

// TestCleanCurrentProjectPortsRevalidatesProjectPath ensures a process that
// changes project identity between scan and cleanup is never terminated.
func TestCleanCurrentProjectPortsRevalidatesProjectPath(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	testDir := t.TempDir()
	otherDir := t.TempDir()
	origPorts := DefaultDevPorts
	DefaultDevPorts = []int{port}
	defer func() { DefaultDevPorts = origPorts }()

	var calls atomic.Int32
	RegisterProjectDetector(func(context.Context, int32) *ProjectInfo {
		if calls.Add(1) == 1 {
			return &ProjectInfo{Name: "clean-demo", Path: testDir}
		}
		return &ProjectInfo{Name: "moved-demo", Path: otherDir}
	})
	defer RegisterProjectDetector(nil)

	cleaned, err := CleanCurrentProjectPorts(context.Background(), testDir, true)
	if err == nil {
		t.Fatal("项目路径变化时应返回聚合清理错误")
	}
	if len(cleaned) != 1 || !strings.HasPrefix(cleaned[0].Status, "kill_failed:") {
		t.Fatalf("项目路径变化时应保留失败诊断而不执行清理: %+v", cleaned)
	}
	if !checkTCPOpen(port, DefaultDialTimeout) {
		t.Fatal("项目路径变化时监听端口必须保持开启")
	}
}

func TestCleanProjectTargetRejectsSystemPIDs(t *testing.T) {
	for _, pid := range []int32{0, 1} {
		t.Run(strconv.Itoa(int(pid)), func(t *testing.T) {
			target := PortDiagnostic{
				Port:        8080,
				IsOpen:      true,
				PID:         pid,
				Status:      StatusOccupied,
				ProcessName: "system",
			}
			if _, _, err := cleanProjectTarget(context.Background(), target, t.TempDir(), false); err == nil {
				t.Fatalf("system PID %d must be rejected", pid)
			}
		})
	}
}
