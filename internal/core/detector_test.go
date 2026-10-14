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

// testHelperPID 是当前测试进程的 PID。
// 测试自己监听端口，因此系统连接表里该端口归属的进程必然是测试进程本身，
// 这让我们可以断言 PID 的准确性，而不只是断言「拿到了某个 PID」。
var testHelperPID = int32(os.Getpid())

// listenLoopback 在 IPv4 回环地址上监听一个由操作系统分配的临时端口。
// 返回已就绪的 listener 与其端口号，由调用方负责 Close。
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

// freeLoopbackPort 取得一个「当前确定空闲」的临时端口。
//
// 做法是先让操作系统分配一个端口再立刻关闭：该端口随即回到空闲池，
// 因此在紧随其后的断言窗口内不会有进程监听它。
func freeLoopbackPort(t *testing.T) int {
	t.Helper()

	ln, port := listenLoopback(t)
	if err := ln.Close(); err != nil {
		t.Fatalf("关闭临时监听端口失败：%v", err)
	}
	return port
}

// portBounds 返回覆盖 ports 中全部端口号的最小闭区间，
// 用于把扫描范围收窄到测试真正关心的几个端口，避免全量扫描拖慢测试。
func portBounds(ports map[int]bool) (int, int) {
	start, end := 1<<31-1, 0
	for port := range ports {
		if port < start {
			start = port
		}
		if port > end {
			end = port
		}
	}
	if start > end {
		return 1, 1
	}
	return start, end
}

// serveLoopbackListeners 一次性开启 count 个临时监听端口，
// 返回端口号 -> listener 的映射，并注册 t.Cleanup 自动关闭。
// cleanup 的作用是让 listener 的生命周期覆盖整个测试函数体，
// 这样 defer 与 t.Cleanup 的差别不会影响断言结果。
func serveLoopbackListeners(t *testing.T, count int) map[int]net.Listener {
	t.Helper()

	bound := make(map[int]net.Listener, count)
	for i := 0; i < count; i++ {
		ln, port := listenLoopback(t)
		bound[port] = ln
	}
	t.Cleanup(func() {
		for _, ln := range bound {
			_ = ln.Close()
		}
	})
	return bound
}

// listenerPorts 把 listener 映射转换为只关心端口号的集合。
func listenerPorts(bound map[int]net.Listener) map[int]bool {
	ports := make(map[int]bool, len(bound))
	for port := range bound {
		ports[port] = true
	}
	return ports
}

// TestInspectPortOpenAndIdentifyProcess 是 Step 2 的核心验收用例：
// 用 net.Listen("tcp","127.0.0.1:0") 动态开启临时端口，
// 验证 InspectPort 能准确识别 IsOpen: true，并正确反查到归属进程。
func TestInspectPortOpenAndIdentifyProcess(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	info, err := InspectPort(port, ProtocolTCP)
	if err != nil {
		t.Fatalf("InspectPort(%d) 返回意外错误：%v", port, err)
	}
	if info == nil {
		t.Fatalf("InspectPort(%d) 返回了 nil 结果", port)
	}

	if !info.IsOpen {
		t.Fatalf("端口 %d 正在监听，但 IsOpen = false（期望 true）", port)
	}
	if info.Port != port {
		t.Errorf("Port = %d，期望 %d", info.Port, port)
	}
	if info.Protocol != ProtocolTCP {
		t.Errorf("Protocol = %q，期望 %q", info.Protocol, ProtocolTCP)
	}

	// 进程反查：端口由本测试进程监听，PID 必须等于本进程。
	if info.PID != testHelperPID {
		t.Errorf("PID = %d，期望 %d（测试进程自身）", info.PID, testHelperPID)
	}
	if info.Name == "" {
		t.Errorf("进程名 Name 为空，未能补全进程详情")
	}
	t.Logf("端口 %d -> PID=%d Name=%q RSS=%d bytes CPU=%.2f%%",
		info.Port, info.PID, info.Name, info.RSS, info.CPU)
}

// TestInspectPortClosed 验证空闲端口被判定为 IsOpen: false。
func TestInspectPortClosed(t *testing.T) {
	port := freeLoopbackPort(t)

	info, err := InspectPort(port, ProtocolTCP)
	if err != nil {
		t.Fatalf("InspectPort(%d) 返回意外错误：%v", port, err)
	}
	if info.IsOpen {
		t.Fatalf("端口 %d 已关闭，但 IsOpen = true（期望 false）", port)
	}
	if info.PID != 0 {
		t.Errorf("空闲端口的 PID 应为 0，实际为 %d", info.PID)
	}
}

// TestInspectPortRejectsInvalidPort 覆盖非法端口号的参数校验。
func TestInspectPortRejectsInvalidPort(t *testing.T) {
	for _, port := range []int{0, -1, 65536, 100000} {
		if _, err := InspectPort(port, ProtocolTCP); err == nil {
			t.Errorf("InspectPort(%d) 期望返回错误，实际返回 nil", port)
		}
	}
}

// TestScanPortsConcurrentDetectsAllListeners 是 Step 2 的另一个核心验收用例：
// 同时开启若干临时端口，验证并发扫描能把它们全部识别为 IsOpen: true。
func TestScanPortsConcurrentDetectsAllListeners(t *testing.T) {
	const wantListeners = 3

	bound := serveLoopbackListeners(t, wantListeners)
	expected := listenerPorts(bound)
	start, end := portBounds(expected)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results, err := ScanPortsConcurrent(ctx, start, end, ScanOptions{Protocol: ProtocolTCP, Workers: 8})
	if err != nil {
		t.Fatalf("ScanPortsConcurrent(%d, %d) 返回错误：%v", start, end, err)
	}

	// 扫描结果必须包含全部预期监听端口。
	found := make(map[int]PortInfo, len(results))
	for _, info := range results {
		found[info.Port] = info
	}
	for port := range expected {
		info, ok := found[port]
		if !ok {
			t.Errorf("端口 %d 正在监听，但并发扫描未识别到它", port)
			continue
		}
		if !info.IsOpen {
			t.Errorf("端口 %d 的扫描结果 IsOpen = false，期望 true", port)
		}
		if info.PID != testHelperPID {
			t.Errorf("端口 %d 的 PID = %d，期望 %d", port, info.PID, testHelperPID)
		}
	}
	t.Logf("并发扫描 [%d, %d] 命中 %d/%d 个临时监听端口", start, end, len(found), len(expected))
}

// TestScanPortsConcurrentSkipsClosedPorts 验证并发扫描不会把空闲端口误报为占用。
func TestScanPortsConcurrentSkipsClosedPorts(t *testing.T) {
	// 连续取 5 个临时端口并确保它们都已释放，得到一段确定空闲的窗口。
	free := make([]int, 0, 5)
	for i := 0; i < 5; i++ {
		free = append(free, freeLoopbackPort(t))
	}
	sort.Ints(free)

	start, end := free[0], free[len(free)-1]
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results, err := ScanPortsConcurrent(ctx, start, end, ScanOptions{Protocol: ProtocolTCP, Workers: 4})
	if err != nil {
		t.Fatalf("ScanPortsConcurrent(%d, %d) 返回错误：%v", start, end, err)
	}

	// 端口可能被系统内的其他程序偶然占用，因此只对「确实空闲」的端口断言。
	closed := make(map[int]bool, len(free))
	for _, p := range free {
		closed[p] = true
	}
	for _, info := range results {
		if closed[info.Port] {
			t.Errorf("端口 %d 空闲，却被扫描报告为占用", info.Port)
		}
	}
}

// TestScanPortsConcurrentReturnsSortedResults 验证并发结果经过排序，输出稳定可复现。
func TestScanPortsConcurrentReturnsSortedResults(t *testing.T) {
	const wantListeners = 4

	bound := serveLoopbackListeners(t, wantListeners)
	expected := listenerPorts(bound)
	start, end := portBounds(expected)

	results, err := ScanPortsConcurrent(context.Background(), start, end, ScanOptions{Protocol: ProtocolTCP})
	if err != nil {
		t.Fatalf("ScanPortsConcurrent 返回错误：%v", err)
	}
	if !sort.SliceIsSorted(results, func(i, j int) bool { return results[i].Port < results[j].Port }) {
		t.Errorf("扫描结果未按端口号升序排序：%+v", results)
	}
}

// TestScanPortsConcurrentRejectsInvalidRange 覆盖区间参数校验。
func TestScanPortsConcurrentRejectsInvalidRange(t *testing.T) {
	if _, err := ScanPortsConcurrent(context.Background(), 9000, 8000, ScanOptions{}); err == nil {
		t.Error("起始端口大于结束端口时期望返回错误，实际返回 nil")
	}
}

// TestScanPortsConcurrentHandlesZeroWorkers 确认 Workers=0 能自动推导协程数而不 panic。
func TestScanPortsConcurrentHandlesZeroWorkers(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	results, err := ScanPortsConcurrent(context.Background(), port, port, ScanOptions{Workers: 0})
	if err != nil {
		t.Fatalf("ScanPortsConcurrent 返回错误：%v", err)
	}
	if len(results) != 1 || results[0].Port != port {
		t.Errorf("单端口扫描结果异常：%+v（期望恰好命中端口 %d）", results, port)
	}
}

// TestScanPortsConcurrentContextCancel 验证上下文取消能被及时响应。
func TestScanPortsConcurrentContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	results, err := ScanPortsConcurrent(ctx, 1, 1024, ScanOptions{Workers: 4})
	if err == nil {
		t.Fatal("上下文已取消，期望返回错误，实际返回 nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("错误应可 unwrap 为 context.Canceled，实际为：%v", err)
	}
	t.Logf("取消后仍回收了 %d 条结果，错误：%v", len(results), err)
}

// TestPortInfoDegradedContract 锁定「优雅降级」字段的自洽性：
// 只要 Degraded 为 true，就必须给出可读的 Err 说明原因。
//
// 背景：Windows 非管理员环境下，svchost.exe 这类 SYSTEM 进程可以被成功打开，
// 却在读取 Name 时返回 "Access is denied"。此时若只把 Name 留空而不标记降级，
// 上层就会显示一个空进程名却无从解释。本用例确保该场景被如实上报。
func TestPortInfoDegradedContract(t *testing.T) {
	// 先验证自洽性：空闲端口与自身监听端口都不应处于降级状态。
	idle := freeLoopbackPort(t)
	idleInfo, err := InspectPort(idle, ProtocolTCP)
	if err != nil {
		t.Fatalf("InspectPort(%d) 返回错误：%v", idle, err)
	}
	if idleInfo.Degraded {
		t.Errorf("空闲端口 %d 不应被标记为降级：Err=%q", idle, idleInfo.Err)
	}

	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	ownInfo, err := InspectPort(port, ProtocolTCP)
	if err != nil {
		t.Fatalf("InspectPort(%d) 返回错误：%v", port, err)
	}
	if ownInfo.Degraded {
		t.Errorf("本进程监听的端口 %d 不应降级，却得到 Err=%q", port, ownInfo.Err)
	}
	if ownInfo.Err != "" {
		t.Errorf("本进程监听的端口 %d 不应有错误信息，却得到 Err=%q", port, ownInfo.Err)
	}

	// 再主动寻找一个「确实无法读取详情」的端口来验证降级路径。
	// 以非管理员身份运行时，系统级服务端口（如 135）必然命中该分支。
	index, indexErr := buildProcIndex(context.Background())
	if indexErr != nil {
		t.Skipf("无法读取系统连接表，跳过降级路径验证：%v", indexErr)
	}

	checked := 0
	for key, pid := range index {
		if key.sockTyp != sockStream {
			continue
		}
		info, err := InspectPort(int(key.port), ProtocolTCP)
		if err != nil || !info.IsOpen {
			continue
		}
		checked++
		if info.Degraded {
			if info.Err == "" {
				t.Fatalf("端口 %d 已标记降级但缺少原因说明（PID=%d）", key.port, pid)
			}
			t.Logf("端口 %d 命中降级路径：PID=%d Err=%q", key.port, info.PID, info.Err)
			return
		}
		if checked >= 40 {
			break
		}
	}

	// 以管理员/root 身份运行时所有进程详情均可读，此时不存在降级样本。
	t.Skipf("已检查 %d 个被占用端口，均无权限受限情况（通常是管理员身份运行），跳过降级路径验证", checked)
}

// TestKillPortProcessOnIdlePort 验证对空闲端口调用释放会得到明确错误而非 panic。
func TestKillPortProcessOnIdlePort(t *testing.T) {
	port := freeLoopbackPort(t)

	pid, err := KillPortProcess(port, true)
	if err == nil {
		t.Fatalf("端口 %d 空闲，期望返回错误，实际返回 nil（pid=%d）", port, pid)
	}
	if pid != 0 {
		t.Errorf("空闲端口不应返回 PID，实际为 %d", pid)
	}
}

// TestKillPortProcessRejectsInvalidPort 覆盖非法端口输入。
func TestKillPortProcessRejectsInvalidPort(t *testing.T) {
	if _, err := KillPortProcess(0, true); err == nil {
		t.Error("端口 0 非法，期望返回错误，实际返回 nil")
	}
}

// TestScanPortsConcurrentIsRaceFree 通过多轮并发扫描暴露数据竞争，
// 用于配合 `go test -race` 验证并发实现的安全性。
func TestScanPortsConcurrentIsRaceFree(t *testing.T) {
	const (
		wantListeners = 3
		rounds        = 3
	)

	bound := serveLoopbackListeners(t, wantListeners)
	expected := listenerPorts(bound)
	start, end := portBounds(expected)

	ctx := context.Background()
	var wg sync.WaitGroup
	for round := 0; round < rounds; round++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := ScanPortsConcurrent(ctx, start, end, ScanOptions{Workers: 4})
			if err != nil {
				t.Errorf("并发轮次扫描失败：%v", err)
				return
			}
			if len(results) < len(expected) {
				t.Errorf("并发轮次命中 %d 个端口，少于预期的 %d 个", len(results), len(expected))
			}
		}()
	}
	wg.Wait()
}
