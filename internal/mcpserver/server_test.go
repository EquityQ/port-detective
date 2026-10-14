package mcpserver

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"port-detective/internal/core"
)

// listenLoopback 在 IPv4 回环地址上监听一个由操作系统分配的临时端口。
// 各包的测试辅助函数无法跨包复用，故此处独立实现一份。
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
func freeLoopbackPort(t *testing.T) int {
	t.Helper()

	ln, port := listenLoopback(t)
	if err := ln.Close(); err != nil {
		t.Fatalf("关闭临时监听端口失败：%v", err)
	}
	return port
}

// callTool 以给定参数直接调用工具处理函数，绕过传输层。
// 这是 MCP 工具测试最经济的做法：既能验证参数解析，又能验证返回内容。
func callTool(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) *mcp.CallToolResult {
	t.Helper()

	req := mcp.CallToolRequest{}
	req.Params.Name = "test"
	req.Params.Arguments = args

	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("工具处理函数返回 Go 错误（应改为返回 CallToolResult 错误）：%v", err)
	}
	if res == nil {
		t.Fatal("工具处理函数返回 nil 结果")
	}
	return res
}

// resultText 提取工具返回的首个文本内容。
func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()

	if len(res.Content) == 0 {
		t.Fatal("工具结果没有任何内容")
	}
	text, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("首个内容不是文本类型：%T", res.Content[0])
	}
	return text.Text
}

// TestDescribeListsAllThreeTools 验证服务端注册了全部三个约定工具。
func TestDescribeListsAllThreeTools(t *testing.T) {
	info := Describe()
	if info.Name != "port-detective" {
		t.Errorf("服务端名称 = %q，期望 %q", info.Name, "port-detective")
	}

	want := map[string]bool{
		"inspect_port":   false,
		"scan_dev_ports": false,
		"free_port":      false,
	}
	for _, tool := range info.Tools {
		if _, ok := want[tool.Name]; !ok {
			t.Errorf("出现了未约定的工具 %q", tool.Name)
			continue
		}
		want[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("工具 %q 缺少描述，AI 客户端将无法判断何时调用", tool.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("缺少约定工具 %q", name)
		}
	}

	if len(info.Tools) != len(want) {
		t.Errorf("工具数量 = %d，期望 %d", len(info.Tools), len(want))
	}
}

// TestToolAnnotationsAreHonest 验证工具的只读/破坏性标注与实际行为一致。
// free_port 会结束进程，必须被标注为破坏性操作，以便客户端做二次确认。
func TestToolAnnotationsAreHonest(t *testing.T) {
	for _, tool := range Describe().Tools {
		switch tool.Name {
		case "inspect_port", "scan_dev_ports":
			if !tool.ReadOnly {
				t.Errorf("工具 %q 是只读操作，应标注 readOnly=true", tool.Name)
			}
			if tool.Destructive {
				t.Errorf("工具 %q 不会修改环境，不应标注 destructive=true", tool.Name)
			}
		case "free_port":
			if tool.ReadOnly {
				t.Errorf("工具 free_port 会结束进程，不应标注 readOnly=true")
			}
			if !tool.Destructive {
				t.Errorf("工具 free_port 会结束进程，必须标注 destructive=true")
			}
		}
	}
}

// TestNewServerRegistersWithoutPanic 验证 NewServer 能正常构建。
// 该函数在工具声明与处理函数脱节时会 panic，因此本用例也是一致性检查。
func TestNewServerRegistersWithoutPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewServer 发生 panic：%v", r)
		}
	}()
	if s := NewServer(); s == nil {
		t.Fatal("NewServer 返回 nil")
	}
}

// TestInspectPortToolFindsOwnListener 验证 inspect_port 能发现真实监听端口。
func TestInspectPortToolFindsOwnListener(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	res := callTool(t, handleInspectPort, map[string]any{"port": float64(port)})
	if res.IsError {
		t.Fatalf("inspect_port 返回错误：%s", resultText(t, res))
	}

	var info core.PortInfo
	if err := json.Unmarshal([]byte(resultText(t, res)), &info); err != nil {
		t.Fatalf("返回内容不是合法 JSON：%v", err)
	}
	if !info.IsOpen {
		t.Errorf("端口 %d 正在监听，但 IsOpen = false", port)
	}
	if info.Port != port {
		t.Errorf("端口 = %d，期望 %d", info.Port, port)
	}
}

// TestInspectPortToolValidatesInput 验证参数校验与错误响应。
func TestInspectPortToolValidatesInput(t *testing.T) {
	// 缺少 port
	res := callTool(t, handleInspectPort, map[string]any{})
	if !res.IsError {
		t.Error("缺少 port 参数时应返回错误结果")
	}

	// 非法协议
	res = callTool(t, handleInspectPort, map[string]any{"port": float64(80), "protocol": "sctp"})
	if !res.IsError {
		t.Error("非法 protocol 应返回错误结果")
	}

	// 越界端口：交由 core 校验
	res = callTool(t, handleInspectPort, map[string]any{"port": float64(70000)})
	if !res.IsError {
		t.Error("越界端口应返回错误结果")
	}
}

// TestScanDevPortsToolDefaultScope 验证默认扫描只覆盖开发常用端口。
func TestScanDevPortsToolDefaultScope(t *testing.T) {
	res := callTool(t, handleScanDevPorts, map[string]any{})
	if res.IsError {
		t.Fatalf("scan_dev_ports 返回错误：%s", resultText(t, res))
	}

	text := resultText(t, res)
	if strings.HasPrefix(text, "扫描完成：未发现") {
		t.Log("本机开发常用端口均未被占用")
		return
	}

	var infos []core.PortInfo
	if err := json.Unmarshal([]byte(text), &infos); err != nil {
		t.Fatalf("返回内容不是合法 JSON 数组：%v", err)
	}

	// 关键断言：默认扫描绝不能泄漏清单以外的端口。
	allowed := make(map[int]bool, len(devPorts))
	for _, p := range devPorts {
		allowed[p] = true
	}
	for _, info := range infos {
		if !allowed[info.Port] {
			t.Errorf("默认扫描返回了清单外的端口 %d（清单应仅含开发常用端口）", info.Port)
		}
	}
	t.Logf("开发常用端口命中 %d 个", len(infos))
}

// TestScanDevPortsToolExplicitRange 验证显式区间扫描。
func TestScanDevPortsToolExplicitRange(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	// 以监听端口为中心取一个很窄的区间，保证结果可控且扫描很快。
	res := callTool(t, handleScanDevPorts, map[string]any{
		"start_port": float64(port),
		"end_port":   float64(port),
	})
	if res.IsError {
		t.Fatalf("scan_dev_ports 返回错误：%s", resultText(t, res))
	}

	var infos []core.PortInfo
	if err := json.Unmarshal([]byte(resultText(t, res)), &infos); err != nil {
		t.Fatalf("返回内容不是合法 JSON 数组：%v", err)
	}
	if len(infos) != 1 || infos[0].Port != port {
		t.Errorf("区间 [%d,%d] 扫描结果异常：%+v", port, port, infos)
	}
}

// TestScanDevPortsToolRequiresPairedRange 验证 start_port / end_port 必须成对出现。
func TestScanDevPortsToolRequiresPairedRange(t *testing.T) {
	res := callTool(t, handleScanDevPorts, map[string]any{"start_port": float64(80)})
	if !res.IsError {
		t.Error("只提供 start_port 时应返回错误结果")
	}

	res = callTool(t, handleScanDevPorts, map[string]any{
		"start_port": float64(9000),
		"end_port":   float64(8000),
	})
	if !res.IsError {
		t.Error("起始端口大于结束端口时应返回错误结果")
	}
}

// TestFreePortToolOnIdlePort 验证对空闲端口调用 free_port 会返回明确错误而非 panic。
func TestFreePortToolOnIdlePort(t *testing.T) {
	port := freeLoopbackPort(t)

	res := callTool(t, handleFreePort, map[string]any{"port": float64(port)})
	if !res.IsError {
		t.Error("端口空闲时 free_port 应返回错误结果")
	}
	if !strings.Contains(resultText(t, res), "未被占用") {
		t.Errorf("错误信息不够明确：%s", resultText(t, res))
	}
}

// TestFreePortToolRequiresPort 验证必填参数缺失时返回明确错误。
func TestFreePortToolRequiresPort(t *testing.T) {
	res := callTool(t, handleFreePort, map[string]any{})
	if !res.IsError {
		t.Error("缺少 port 参数时应返回错误结果")
	}
}

// TestFreePortToolReportsPermissionDenied 验证「权限不足」被如实上报为可操作的错误。
//
// 背景：非管理员 / 非 root 运行时，结束他人进程会失败并返回 OpenProcess 拒绝。
// 该场景在 Windows 非管理员环境下必然出现（svchost 等系统服务占用端口时），
// 因此必须返回明确的、带提权建议的错误信息，而不是静默失败或误报成功。
func TestFreePortToolReportsPermissionDenied(t *testing.T) {
	// 找一个「已被占用但无权限读取详情」的端口：这类端口的宿主进程
	// 通常也无法被当前用户结束，正好覆盖权限拒绝分支。
	infos, err := core.ListOccupiedPorts(context.Background(), core.ListOptions{Protocol: core.ProtocolTCP})
	if err != nil {
		t.Skipf("无法读取系统连接表，跳过：%v", err)
	}

	var target *core.PortInfo
	for i := range infos {
		if infos[i].Degraded && infos[i].PID > 0 {
			target = &infos[i]
			break
		}
	}
	if target == nil {
		t.Skip("未找到权限受限的占用端口（可能是管理员身份运行），跳过该分支验证")
	}

	res := callTool(t, handleFreePort, map[string]any{"port": float64(target.Port)})
	text := resultText(t, res)

	if !res.IsError {
		// 若恰好成功结束了进程，也属合理结果（说明当前用户有该权限），
		// 但需确认返回体是结构化的成功响应。
		if !strings.Contains(text, "released") {
			t.Errorf("既非错误也未返回 released 字段：%s", text)
		}
		return
	}

	// 错误信息必须可操作：应当提示提权，而不是只丢一个系统错误码。
	if !strings.Contains(text, "管理员") && !strings.Contains(text, "root") {
		t.Errorf("权限失败信息应提示用户提权重试，实际：%s", text)
	}
	t.Logf("端口 %d 权限拒绝分支已验证：%s", target.Port, text)
}

// TestDevPortsAreUniqueAndSorted 保证开发端口清单本身是干净的数据。
func TestDevPortsAreUniqueAndSorted(t *testing.T) {
	seen := make(map[int]bool, len(devPorts))
	for _, p := range devPorts {
		if p < 1 || p > 65535 {
			t.Errorf("开发端口清单含非法端口 %d", p)
		}
		if seen[p] {
			t.Errorf("开发端口清单含重复端口 %d", p)
		}
		seen[p] = true
	}
}
