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
		t.Fatalf("无法创建本地临时监听端口：%v", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		t.Fatalf("监听地址类型异常：%T", ln.Addr())
	}
	return ln, addr.Port
}

func freeLoopbackPort(t *testing.T) int {
	t.Helper()

	ln, port := listenLoopback(t)
	if err := ln.Close(); err != nil {
		t.Fatalf("关闭临时监听端口失败：%v", err)
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
		t.Fatalf("工具处理函数返回 Go 错误（应改为返回 CallToolResult 错误）：%v", err)
	}
	if res == nil {
		t.Fatal("工具处理函数返回 nil 结果")
	}
	return res
}

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

// TestDescribeListsAllTools verifies that all expected tools are registered.
func TestDescribeListsAllTools(t *testing.T) {
	info := Describe()
	if info.Name != "port-detective" {
		t.Errorf("服务端名称 = %q，期望 %q", info.Name, "port-detective")
	}

	want := map[string]bool{
		"inspect_port":          false,
		"scan_dev_ports":        false,
		"free_port":             false,
		"clean_current_project": false,
	}
	for _, tool := range info.Tools {
		if _, ok := want[tool.Name]; !ok {
			t.Errorf("出现了未约定的工具 %q", tool.Name)
			continue
		}
		want[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("工具 %q 缺少描述", tool.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("缺少约定工具 %q", name)
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
			t.Fatalf("未找到工具 %q", name)
		}
		if !tool.ReadOnly {
			t.Errorf("工具 %q 应为只读（ReadOnly=true），实际为 false", name)
		}
		if tool.Destructive {
			t.Errorf("工具 %q 不应标为破坏性操作（Destructive=false），实际为 true", name)
		}
	}

	for _, name := range []string{"free_port", "clean_current_project"} {
		tool, ok := lookup[name]
		if !ok {
			t.Fatalf("未找到工具 %q", name)
		}
		if tool.ReadOnly {
			t.Errorf("%s 会终止进程，不应标为只读（期望 ReadOnly=false）", name)
		}
		if !tool.Destructive {
			t.Errorf("%s 会终止进程，必须如实标为破坏性操作（期望 Destructive=true）", name)
		}
	}
}

// TestNewServerRegistersWithoutPanic verifies that server construction does not panic.
func TestNewServerRegistersWithoutPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewServer 发生 panic：%v", r)
		}
	}()
	s := NewServer()
	if s == nil {
		t.Fatal("NewServer 返回了 nil")
	}
}

// TestInspectPortToolFindsOwnListener verifies that inspect_port detects a local listener.
func TestInspectPortToolFindsOwnListener(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	res := callTool(t, handleInspectPort, map[string]any{"port": float64(port)})
	if res.IsError {
		t.Fatalf("inspect_port 返回错误：%s", resultText(t, res))
	}

	var diag core.PortDiagnostic
	if err := json.Unmarshal([]byte(resultText(t, res)), &diag); err != nil {
		t.Fatalf("返回内容不是合法 JSON：%v\n原始内容：%s", err, resultText(t, res))
	}
	if !diag.IsOpen {
		t.Errorf("端口 %d 正在监听，但 inspect_port 返回 is_open=false", port)
	}
	if diag.Port != port {
		t.Errorf("端口号 = %d，期望 %d", diag.Port, port)
	}
}

// TestInspectPortToolValidatesInput verifies input validation.
func TestInspectPortToolValidatesInput(t *testing.T) {
	res := callTool(t, handleInspectPort, map[string]any{})
	if !res.IsError {
		t.Error("缺少 port 参数时应返回错误结果")
	}

	res = callTool(t, handleInspectPort, map[string]any{"port": float64(70000)})
	if !res.IsError {
		t.Error("越界端口应返回错误结果")
	}
}

// TestScanDevPortsToolDefaultScope verifies the default scan uses DefaultDevPorts only.
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

	var diags []core.PortDiagnostic
	if err := json.Unmarshal([]byte(text), &diags); err != nil {
		t.Fatalf("返回内容不是合法 JSON 数组：%v", err)
	}

	allowed := make(map[int]bool, len(core.DefaultDevPorts))
	for _, p := range core.DefaultDevPorts {
		allowed[p] = true
	}
	for _, d := range diags {
		if !allowed[d.Port] {
			t.Errorf("默认扫描返回了清单外的端口 %d（清单应仅含 DefaultDevPorts）", d.Port)
		}
	}
	t.Logf("开发常用端口命中 %d 个", len(diags))
}

// TestScanDevPortsToolExplicitPorts verifies the explicit ports argument.
func TestScanDevPortsToolExplicitPorts(t *testing.T) {
	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	res := callTool(t, handleScanDevPorts, map[string]any{
		"ports": fmt.Sprintf("%d", port),
	})
	if res.IsError {
		t.Fatalf("scan_dev_ports 返回错误：%s", resultText(t, res))
	}

	var diags []core.PortDiagnostic
	if err := json.Unmarshal([]byte(resultText(t, res)), &diags); err != nil {
		t.Fatalf("返回内容不是合法 JSON 数组：%v", err)
	}
	if len(diags) != 1 || diags[0].Port != port {
		t.Errorf("扫描结果异常：%+v（期望命中端口 %d）", diags, port)
	}
}

// TestScanDevPortsToolRejectsInvalidPort verifies invalid port arguments are rejected.
func TestScanDevPortsToolRejectsInvalidPort(t *testing.T) {
	res := callTool(t, handleScanDevPorts, map[string]any{"ports": "abc"})
	if !res.IsError {
		t.Error("ports 包含非数字时应返回错误结果")
	}

	res = callTool(t, handleScanDevPorts, map[string]any{"ports": "70000"})
	if !res.IsError {
		t.Error("ports 越界时应返回错误结果")
	}
}

// TestFreePortToolOnIdlePort verifies free_port returns an error for an idle port.
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

// TestFreePortToolRequiresPort verifies the required port argument.
func TestFreePortToolRequiresPort(t *testing.T) {
	res := callTool(t, handleFreePort, map[string]any{})
	if !res.IsError {
		t.Error("缺少 port 参数时应返回错误结果")
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
		t.Fatalf("free_port dry_run 期望成功，实际报错: %s", resultText(t, res))
	}

	var resp struct {
		Port       int                 `json:"port"`
		DryRun     bool                `json:"dry_run"`
		Diagnostic core.PortDiagnostic `json:"diagnostic"`
		Message    string              `json:"message"`
	}
	if err := json.Unmarshal([]byte(resultText(t, res)), &resp); err != nil {
		t.Fatalf("解析响应 JSON 失败: %v", err)
	}
	if !resp.DryRun {
		t.Errorf("响应中的 dry_run 应为 true")
	}
	if resp.Port != port {
		t.Errorf("响应端口 %d 不匹配 %d", resp.Port, port)
	}
	if !strings.Contains(resp.Message, "预演") {
		t.Errorf("消息应包含预演提示，实际为: %s", resp.Message)
	}
}

// TestDevPortsAreUniqueAndSorted ensures the development port list has ten valid unique ports.
func TestDevPortsAreUniqueAndSorted(t *testing.T) {
	if len(core.DefaultDevPorts) != 10 {
		t.Errorf("DefaultDevPorts 长度应为 10，实际为 %d", len(core.DefaultDevPorts))
	}
	seen := make(map[int]bool, len(core.DefaultDevPorts))
	for _, p := range core.DefaultDevPorts {
		if p < 1 || p > 65535 {
			t.Errorf("开发端口清单含非法端口 %d", p)
		}
		if seen[p] {
			t.Errorf("开发端口清单含重复端口 %d", p)
		}
		seen[p] = true
	}
}

// TestCleanCurrentProjectTool verifies dry-run behavior for clean_current_project.
func TestCleanCurrentProjectTool(t *testing.T) {
	res := callTool(t, handleCleanCurrentProject, map[string]any{"dry_run": true})
	if res.IsError {
		t.Fatalf("clean_current_project 工具返回错误: %s", resultText(t, res))
	}

	var resp struct {
		Cwd     string                `json:"cwd"`
		DryRun  bool                  `json:"dry_run"`
		Count   int                   `json:"count"`
		Cleaned []core.PortDiagnostic `json:"cleaned"`
	}
	if err := json.Unmarshal([]byte(resultText(t, res)), &resp); err != nil {
		t.Fatalf("解析响应 JSON 失败: %v", err)
	}
	if !resp.DryRun {
		t.Errorf("响应中的 dry_run 应为 true")
	}
	if resp.Cwd == "" {
		t.Errorf("响应中的 cwd 不应为空")
	}
}
