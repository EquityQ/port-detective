package core

import (
	"context"
	"errors"
	"net"
	"os"
	"sort"
	"sync"
	"testing"
	"time"
)

var testHelperPID = int32(os.Getpid())

// listenLoopback 在 IPv4 回环地址上监听一个由操作系统分配的临时端口。
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

// freeLoopbackPort 取得一个确定空闲的端口号。
func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	ln, port := listenLoopback(t)
	if err := ln.Close(); err != nil {
		t.Fatalf("关闭临时监听端口失败：%v", err)
	}
	return port
}

// TestInspectPortOpenAndIdentifyProcess 是 Step 2 的核心验收用例：
// 动态开启临时端口，验证 InspectPort 能够识别为 IsOpen: true，且正确关联进程信息。
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

// TestInspectPortClosed 验证空闲端口被判定为 IsOpen: false 且 Status: "free"。
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

// TestInspectPortRejectsInvalidPort 覆盖非法端口号参数校验。
func TestInspectPortRejectsInvalidPort(t *testing.T) {
	ctx := context.Background()
	for _, port := range []int{0, -1, 65536, 100000} {
		if _, err := InspectPort(ctx, port); err == nil {
			t.Errorf("InspectPort(%d) 期望返回错误，实际返回 nil", port)
		}
	}
}

// TestScanPortsConcurrentDetectsAllListeners 是 Step 2 核心验收用例：
// 开启若干临时端口，验证并发扫描端口切片能够按升序返回所有被占用端口。
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

	// 混入一个空闲端口
	freePort := freeLoopbackPort(t)
	testPorts := append([]int{freePort}, ports...)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results, err := ScanPortsConcurrent(ctx, testPorts)
	if err != nil {
		t.Fatalf("ScanPortsConcurrent 返回错误：%v", err)
	}

	// 结果必须升序排列
	if !sort.SliceIsSorted(results, func(i, j int) bool { return results[i].Port < results[j].Port }) {
		t.Errorf("扫描结果未按端口号升序排序：%+v", results)
	}

	// 验证包含所有测试监听端口
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

// TestScanPortsConcurrentContextCancel 验证 Context 取消可快速优雅退出。
func TestScanPortsConcurrentContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := ScanPortsConcurrent(ctx, DefaultDevPorts)
	if err == nil {
		t.Fatal("上下文已取消，期望返回错误，实际返回 nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("错误未包装 context.Canceled：%v", err)
	}
}

// TestKillPortProcessDryRun 验证 KillPortProcess 的 dryRun 模式与安全红线。
func TestKillPortProcessDryRun(t *testing.T) {
	ctx := context.Background()

	// 1. 测试空闲端口
	idle := freeLoopbackPort(t)
	_, _, err := KillPortProcess(ctx, idle, true)
	if err == nil {
		t.Errorf("空闲端口 %d 执行 KillPortProcess 期望报错，实际返回 nil", idle)
	}

	// 2. 测试自身监听端口 dryRun 预演成功
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

	// 确认端口依然活着（dryRun 绝不真杀）
	if !checkTCPOpen(port, DefaultDialTimeout) {
		t.Errorf("dry-run 不应结束进程，但端口 %d 已无法连接", port)
	}
}

// TestKillPortProcessRejectsInvalidPort 覆盖非法端口输入。
func TestKillPortProcessRejectsInvalidPort(t *testing.T) {
	ctx := context.Background()
	if _, _, err := KillPortProcess(ctx, 0, true); err == nil {
		t.Error("端口 0 非法，期望返回错误，实际返回 nil")
	}
}

// TestScanPortsConcurrentIsRaceFree 通过多协程并发扫描验证竞态安全性。
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

// TestRegisterProjectDetectorHook 验证指纹探测器 hook 注入与诊断报告集成
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

// TestCleanCurrentProjectPorts 验证项目级一键清理能力
func TestCleanCurrentProjectPorts(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	testDir := t.TempDir()

	// 保存原本的 DefaultDevPorts 并在测试结束后恢复
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

	// 1. 匹配的目录，dry-run 预演
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

	// 2. 不匹配的目录，应返回空列表
	cleanedEmpty, err := CleanCurrentProjectPorts(context.Background(), "/non-existent-dir-12345", true)
	if err != nil {
		t.Fatalf("CleanCurrentProjectPorts 失败: %v", err)
	}
	if len(cleanedEmpty) != 0 {
		t.Errorf("非匹配目录期望返回 0 个端口，实际为 %d", len(cleanedEmpty))
	}
}
