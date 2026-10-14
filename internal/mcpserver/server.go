// Package mcpserver 把 internal/core 的端口探测能力封装为 MCP（Model Context
// Protocol）Tool，并以 Stdio 传输方式对外提供服务，供 AI Agent 直接调用。
//
// 关键约束：Stdio 传输下 **标准输出专供协议通信**。本包内任何位置都不得向
// os.Stdout 写入日志或提示，否则会污染 JSON-RPC 报文流导致客户端解析失败。
// 需要输出诊断信息时一律使用 os.Stderr。
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"port-detective/internal/core"
)

const (
	// serverName / serverVersion 是向 MCP 客户端自报的身份信息。
	serverName    = "port-detective"
	serverVersion = "0.1.0"

	// defaultScanWorkers 是 MCP 扫描工具默认使用的并发协程数。
	// 取值偏保守：MCP 调用通常来自 AI Agent 的即时交互，不宜占满 CPU。
	defaultScanWorkers = 64
)

// devPorts 是「开发常用端口」清单，用于 scan_dev_ports 的默认扫描范围。
//
// 依据是各类本地开发服务的默认监听端口：前端 dev server、后端框架、
// 数据库、消息队列、容器与常见中间件。仅覆盖高频端口，保证默认调用足够快。
var devPorts = []int{
	3000, 3001, 3002, 3306, 4200, 5000, 5001, 5173, 5174, 5432,
	5672, 6379, 7001, 8000, 8001, 8080, 8081, 8082, 8086, 8443,
	8888, 9000, 9090, 9200, 9300, 11211, 27017, 15672,
}

// NewServer 构建并返回已注册全部工具的 MCP Server。
//
// 与 RunStdioServer 分离是为了便于单元测试：测试可直接检查已注册的工具
// 清单与描述，也可以在进程内直接调用工具处理函数，无需启动真实传输层。
func NewServer() *server.MCPServer {
	s := server.NewMCPServer(
		serverName,
		serverVersion,
		// 单个工具内部发生 panic 时自动恢复为错误响应，避免整个服务端崩溃。
		server.WithRecovery(),
	)

	// 工具声明与处理函数在此一一绑定；声明本身由 serverTools 统一提供，
	// 以保证注册与 Describe 自检两处永远一致。
	handlers := map[string]server.ToolHandlerFunc{
		"inspect_port":   handleInspectPort,
		"scan_dev_ports": handleScanDevPorts,
		"free_port":      handleFreePort,
	}
	for _, tool := range serverTools() {
		handler, ok := handlers[tool.Name]
		if !ok {
			// 声明与处理函数脱节属编程错误，尽早暴露而不是静默跳过。
			panic(fmt.Sprintf("MCP 工具 %q 缺少对应的处理函数", tool.Name))
		}
		s.AddTool(tool, handler)
	}

	return s
}

// RunStdioServer 以 Stdio 传输启动 MCP 服务端，阻塞直到上下文取消或输入流关闭。
//
// 注意：参数固定为 os.Stdin / os.Stdout —— 这是 MCP Stdio 协议的约定通道。
func RunStdioServer(ctx context.Context) error {
	stdio := server.NewStdioServer(NewServer())
	return stdio.Listen(ctx, os.Stdin, os.Stdout)
}

// ToolSummary 描述一个已注册工具的身份信息。
type ToolSummary struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ReadOnly    bool   `json:"read_only"`
	Destructive bool   `json:"destructive"`
}

// ServerInfo 描述 MCP 服务端的能力清单，供 `port-detective mcp --json` 自检使用。
type ServerInfo struct {
	Name    string        `json:"name"`
	Version string        `json:"version"`
	Tools   []ToolSummary `json:"tools"`
}

// Describe 返回服务端的能力清单（不启动传输层）。
//
// 之所以单独提供，是因为 Stdio 模式下 stdout 不能用于人类可读输出；
// 该清单让使用者（与测试）能在不进入协议循环的前提下确认注册了哪些工具。
func Describe() ServerInfo {
	info := ServerInfo{Name: serverName, Version: serverVersion}

	for _, tool := range serverTools() {
		summary := ToolSummary{
			Name:        tool.Name,
			Title:       tool.Annotations.Title,
			Description: tool.Description,
		}
		if tool.Annotations.ReadOnlyHint != nil {
			summary.ReadOnly = *tool.Annotations.ReadOnlyHint
		}
		if tool.Annotations.DestructiveHint != nil {
			summary.Destructive = *tool.Annotations.DestructiveHint
		}
		info.Tools = append(info.Tools, summary)
	}
	return info
}

// serverTools 返回三个工具的声明，供注册与自检共用，避免两处定义漂移。
func serverTools() []mcp.Tool {
	return []mcp.Tool{
		inspectPortTool(),
		scanDevPortsTool(),
		freePortTool(),
	}
}

// ============================ 工具定义 ============================

// inspectPortTool 声明 inspect_port 工具：体检单个端口。
func inspectPortTool() mcp.Tool {
	return mcp.NewTool("inspect_port",
		mcp.WithDescription(
			"体检本机指定端口：判断是否被占用，并返回占用进程的名称、PID、常驻内存、CPU 占用与启动命令。"+
				"当用户询问「某个端口被谁占用」时使用本工具。"),
		mcp.WithTitleAnnotation("端口体检"),
		// 注意：mcp-go 的 NewTool 把 DestructiveHint/OpenWorldHint 默认为 true，
		// 只读工具必须显式覆盖，否则客户端会把它当成破坏性操作要求二次确认。
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("protocol",
			mcp.Description("探测协议，tcp 或 udp，默认 tcp"),
			mcp.Enum(core.ProtocolTCP, core.ProtocolUDP),
			mcp.DefaultString(core.ProtocolTCP),
		),
		mcp.WithNumber("port",
			mcp.Required(),
			mcp.Description("要体检的端口号，取值 1-65535"),
			mcp.Min(1),
			mcp.Max(65535),
		),
	)
}

// scanDevPortsTool 声明 scan_dev_ports 工具：扫描被占用的端口。
func scanDevPortsTool() mcp.Tool {
	return mcp.NewTool("scan_dev_ports",
		mcp.WithDescription(fmt.Sprintf(
			"扫描本机被占用的端口，返回端口、PID、进程名、内存与 CPU 等体检结果。"+
				"不传参数时只扫描开发常用端口（如 3000/5173/8080/5432/6379 等，共 %d 个），速度快；"+
				"需要扫描指定区间时传入 start_port 与 end_port。"+
				"当用户询问「哪些端口被占用」「本机开了哪些服务」时使用本工具。", len(devPorts))),
		mcp.WithTitleAnnotation("端口扫描"),
		// 同 inspect_port：只读且不改变环境，需显式覆盖 mcp-go 的默认值。
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("protocol",
			mcp.Description("探测协议，tcp 或 udp，默认 tcp"),
			mcp.Enum(core.ProtocolTCP, core.ProtocolUDP),
			mcp.DefaultString(core.ProtocolTCP),
		),
		mcp.WithNumber("start_port",
			mcp.Description("扫描起始端口，需与 end_port 成对出现；留空则扫描开发常用端口"),
			mcp.Min(1),
			mcp.Max(65535),
		),
		mcp.WithNumber("end_port",
			mcp.Description("扫描结束端口，需与 start_port 成对出现；留空则扫描开发常用端口"),
			mcp.Min(1),
			mcp.Max(65535),
		),
		mcp.WithNumber("workers",
			mcp.Description(fmt.Sprintf("并发协程数，默认 %d", defaultScanWorkers)),
			mcp.Min(1),
			mcp.Max(512),
		),
	)
}

// freePortTool 声明 free_port 工具：结束占用端口的进程以释放端口。
func freePortTool() mcp.Tool {
	return mcp.NewTool("free_port",
		mcp.WithDescription(
			"结束占用指定端口的进程，从而释放该端口。属于破坏性操作，调用前请先向用户确认目标进程。"+
				"默认请求进程优雅退出，force 为 true 时强制终止。"+
				"当用户明确要求「释放某个端口」「杀掉占用端口的进程」时使用本工具。"),
		mcp.WithTitleAnnotation("释放端口"),
		// 该工具会结束其他进程，如实声明为破坏性操作，供客户端做二次确认。
		// 这是唯一使用「破坏性」语义的工具，其余两个已显式覆盖为 false。
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithNumber("port",
			mcp.Required(),
			mcp.Description("要释放的端口号，取值 1-65535"),
			mcp.Min(1),
			mcp.Max(65535),
		),
		mcp.WithBoolean("force",
			mcp.Description("是否强制终止进程（默认 false，即请求优雅退出）"),
			mcp.DefaultBool(false),
		),
	)
}

// ============================ 工具处理函数 ============================

// handleInspectPort 处理 inspect_port 调用。
func handleInspectPort(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	port, err := request.RequireInt("port")
	if err != nil {
		return mcp.NewToolResultError("缺少必填参数 port（端口号，1-65535）"), nil
	}

	protocol := request.GetString("protocol", core.ProtocolTCP)
	if protocol != core.ProtocolTCP && protocol != core.ProtocolUDP {
		return mcp.NewToolResultErrorf("参数 protocol 只能是 %q 或 %q，收到 %q",
			core.ProtocolTCP, core.ProtocolUDP, protocol), nil
	}

	info, err := core.InspectPort(port, protocol)
	if err != nil {
		return mcp.NewToolResultErrorf("体检端口 %d 失败：%v", port, err), nil
	}

	return jsonResult(info)
}

// handleScanDevPorts 处理 scan_dev_ports 调用。
func handleScanDevPorts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	protocol := request.GetString("protocol", core.ProtocolTCP)
	if protocol != core.ProtocolTCP && protocol != core.ProtocolUDP {
		return mcp.NewToolResultErrorf("参数 protocol 只能是 %q 或 %q，收到 %q",
			core.ProtocolTCP, core.ProtocolUDP, protocol), nil
	}

	workers := request.GetInt("workers", defaultScanWorkers)

	// 两个区间参数必须成对出现：只给一个属于调用方语义不明，直接报错。
	_, hasStart := request.GetArguments()["start_port"]
	_, hasEnd := request.GetArguments()["end_port"]
	if hasStart != hasEnd {
		return mcp.NewToolResultError("start_port 与 end_port 必须成对提供；两者都留空则扫描开发常用端口"), nil
	}

	start, end := 0, 0
	if hasStart {
		start = request.GetInt("start_port", 0)
		end = request.GetInt("end_port", 0)
		if start < 1 || end > 65535 || start > end {
			return mcp.NewToolResultErrorf("端口区间非法：start_port=%d, end_port=%d（需满足 1 <= start <= end <= 65535）",
				start, end), nil
		}
	}

	infos, err := scanPorts(ctx, start, end, protocol, workers)
	if err != nil {
		return mcp.NewToolResultErrorf("扫描端口失败：%v", err), nil
	}

	// 扫描结果可能很长，但 MCP 工具应返回完整数据；仅在为空时给出明确结论。
	if len(infos) == 0 {
		return mcp.NewToolResultText("扫描完成：未发现被占用的端口。"), nil
	}
	return jsonResult(infos)
}

// scanPorts 按区间扫描；区间为空（0, 0）时回退到开发常用端口清单。
//
// 实现上统一走 core.ScanPortsConcurrent（goroutine + WaitGroup + channel），
// 因此「默认端口清单」同样享受并发加速。
func scanPorts(ctx context.Context, start, end int, protocol string, workers int) ([]core.PortInfo, error) {
	opts := core.ScanOptions{Protocol: protocol, Workers: workers}

	if start > 0 && end > 0 {
		return core.ScanPortsConcurrent(ctx, start, end, opts)
	}

	// 开发常用端口是稀疏清单，合并为最小闭区间后交由并发引擎扫描，
	// 再按清单过滤，避免暴露清单以外的端口。
	low, high := devPorts[0], devPorts[0]
	for _, p := range devPorts {
		if p < low {
			low = p
		}
		if p > high {
			high = p
		}
	}

	scanned, err := core.ScanPortsConcurrent(ctx, low, high, opts)
	if err != nil {
		return nil, err
	}

	wanted := make(map[int]bool, len(devPorts))
	for _, p := range devPorts {
		wanted[p] = true
	}
	filtered := make([]core.PortInfo, 0, len(scanned))
	for _, info := range scanned {
		if wanted[info.Port] {
			filtered = append(filtered, info)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Port < filtered[j].Port })
	return filtered, nil
}

// handleFreePort 处理 free_port 调用。
func handleFreePort(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	port, err := request.RequireInt("port")
	if err != nil {
		return mcp.NewToolResultError("缺少必填参数 port（端口号，1-65535）"), nil
	}
	force := request.GetBool("force", false)

	// 先体检拿到目标进程信息，便于在响应里说明「结束了谁」。
	info, err := core.InspectPort(port, core.ProtocolTCP)
	if err != nil {
		return mcp.NewToolResultErrorf("体检端口 %d 失败：%v", port, err), nil
	}
	if !info.IsOpen {
		return mcp.NewToolResultErrorf("端口 %d 当前未被占用，无需释放", port), nil
	}

	pid, err := core.KillPortProcess(port, force)
	if err != nil {
		// 区分「权限不足」与「其他失败」，便于 AI 向用户给出可操作的提示。
		if errors.Is(err, core.ErrProcessNotAccessible) {
			return mcp.NewToolResultErrorf(
				"释放端口 %d 失败：%v。请提示用户以管理员（Windows）或 root（Linux/macOS）身份重试。",
				port, err), nil
		}
		return mcp.NewToolResultErrorf("释放端口 %d 失败：%v", port, err), nil
	}

	result := map[string]any{
		"port":     port,
		"pid":      pid,
		"force":    force,
		"released": true,
		"process":  info.Name,
	}
	return jsonResult(result)
}

// ============================ 公共辅助 ============================

// jsonResult 把任意结果序列化为缩进 JSON 文本后包装为工具响应。
//
// 选择返回 JSON 文本而非纯自然语言，是为了让 AI 客户端能可靠地解析字段，
// 同时人类阅读时也足够清晰。
func jsonResult(v any) (*mcp.CallToolResult, error) {
	payload, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultErrorf("结果序列化失败：%v", err), nil
	}
	return mcp.NewToolResultText(string(payload)), nil
}

// WaitForSignal 返回一个在收到中断信号（Ctrl+C / SIGTERM）时被取消的上下文。
// CLI 层用它实现优雅退出。
func WaitForSignal(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		defer signal.Stop(sigCh)
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}
