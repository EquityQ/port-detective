// Package mcpserver exposes internal/core port inspection capabilities as MCP
// (Model Context Protocol) tools over Stdio for AI agents.
//
// With Stdio transport, standard output is reserved for protocol messages. Never
// write logs or status messages to os.Stdout; send diagnostics to os.Stderr.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"port-detective/internal/core"
	_ "port-detective/internal/fingerprint"
)

const (
	serverName    = "port-detective"
	serverVersion = "1.1.0"
)

// NewServer creates an MCP server with all tools registered.
func NewServer() *server.MCPServer {
	s := server.NewMCPServer(
		serverName,
		serverVersion,
		server.WithRecovery(),
	)

	handlers := map[string]server.ToolHandlerFunc{
		"inspect_port":          handleInspectPort,
		"scan_dev_ports":        handleScanDevPorts,
		"free_port":             handleFreePort,
		"clean_current_project": handleCleanCurrentProject,
	}
	for _, tool := range serverTools() {
		handler, ok := handlers[tool.Name]
		if !ok {
			panic(fmt.Sprintf("MCP 工具 %q 缺少对应的处理函数", tool.Name))
		}
		s.AddTool(tool, handler)
	}

	return s
}

// RunStdioServer starts the MCP server over Stdio and blocks until canceled or input closes.
func RunStdioServer(ctx context.Context) error {
	stdio := server.NewStdioServer(NewServer())
	return stdio.Listen(ctx, os.Stdin, os.Stdout)
}

// ToolSummary describes a registered tool.
type ToolSummary struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ReadOnly    bool   `json:"read_only"`
	Destructive bool   `json:"destructive"`
}

// ServerInfo describes the MCP server capabilities for self-inspection.
type ServerInfo struct {
	Name    string        `json:"name"`
	Version string        `json:"version"`
	Tools   []ToolSummary `json:"tools"`
}

// Describe returns the server capabilities without starting the transport.
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

func serverTools() []mcp.Tool {
	return []mcp.Tool{
		inspectPortTool(),
		scanDevPortsTool(),
		freePortTool(),
		cleanCurrentProjectTool(),
	}
}

// ============================ Tool Definitions ============================

// 1. inspect_port
func inspectPortTool() mcp.Tool {
	return mcp.NewTool("inspect_port",
		mcp.WithDescription(
			"Check if a local port is occupied and get the owning process health metrics (PID, name, memory MB, CPU%, command line).",
		),
		mcp.WithTitleAnnotation("端口体检"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithNumber("port",
			mcp.Required(),
			mcp.Description("Local port number to inspect (1-65535)"),
			mcp.Min(1),
			mcp.Max(65535),
		),
	)
}

// 2. scan_dev_ports
func scanDevPortsTool() mcp.Tool {
	return mcp.NewTool("scan_dev_ports",
		mcp.WithDescription(
			"Concurrently scan multiple development ports using Go goroutines and return all occupied ports with process diagnostics.",
		),
		mcp.WithTitleAnnotation("端口扫描"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("ports",
			mcp.Description("Optional comma-separated list of ports to scan, e.g. '3000,8080'. If omitted, scans DefaultDevPorts."),
		),
	)
}

// 3. free_port
func freePortTool() mcp.Tool {
	return mcp.NewTool("free_port",
		mcp.WithDescription(
			"Terminate the local process occupying the specified port. Set dry_run=false to actually kill the process.",
		),
		mcp.WithTitleAnnotation("释放端口"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithNumber("port",
			mcp.Required(),
			mcp.Description("Port number occupied by the target process to terminate (1-65535)"),
			mcp.Min(1),
			mcp.Max(65535),
		),
		mcp.WithBoolean("dry_run",
			mcp.Description("Whether to run in dry-run mode without actually terminating the process. Defaults to true for safety."),
			mcp.DefaultBool(true),
		),
	)
}

// 4. clean_current_project
func cleanCurrentProjectTool() mcp.Tool {
	return mcp.NewTool("clean_current_project",
		mcp.WithDescription(
			"Concurrently scan development ports and terminate orphan processes belonging to the current working directory project. Set dry_run=false to actually kill processes.",
		),
		mcp.WithTitleAnnotation("清理当前项目孤儿端口"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithBoolean("dry_run",
			mcp.Description("Whether to run in dry-run mode without actually terminating the processes. Defaults to true for safety."),
			mcp.DefaultBool(true),
		),
	)
}

// ============================ Tool Handlers ============================

func handleInspectPort(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	port, err := request.RequireInt("port")
	if err != nil {
		return mcp.NewToolResultError("缺少必填参数 port（端口号，1-65535）"), nil
	}

	diag, err := core.InspectPort(ctx, port)
	if err != nil {
		return mcp.NewToolResultErrorf("体检端口 %d 失败：%v", port, err), nil
	}

	return jsonResult(diag)
}

func handleScanDevPorts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	portsStr := request.GetString("ports", "")
	var targetPorts []int

	if portsStr != "" {
		parts := strings.Split(portsStr, ",")
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed == "" {
				continue
			}
			num, err := strconv.Atoi(trimmed)
			if err != nil || num < 1 || num > 65535 {
				return mcp.NewToolResultErrorf("非法端口号 %q：需为 1-65535 之间的整数", trimmed), nil
			}
			targetPorts = append(targetPorts, num)
		}
		if len(targetPorts) == 0 {
			targetPorts = core.DefaultDevPorts
		}
	} else {
		targetPorts = core.DefaultDevPorts
	}

	results, err := core.ScanPortsConcurrent(ctx, targetPorts)
	if err != nil {
		return mcp.NewToolResultErrorf("扫描端口失败：%v", err), nil
	}

	if len(results) == 0 {
		return mcp.NewToolResultText("扫描完成：未发现被占用的端口。"), nil
	}
	return jsonResult(results)
}

func handleFreePort(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	port, err := request.RequireInt("port")
	if err != nil {
		return mcp.NewToolResultError("缺少必填参数 port（端口号，1-65535）"), nil
	}

	dryRun := request.GetBool("dry_run", true)

	diag, msg, err := core.KillPortProcess(ctx, port, dryRun)
	if err != nil {
		return mcp.NewToolResultErrorf("释放端口 %d 失败：%v", port, err), nil
	}

	result := map[string]any{
		"port":       port,
		"dry_run":    dryRun,
		"diagnostic": diag,
		"message":    msg,
	}
	return jsonResult(result)
}

func handleCleanCurrentProject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dryRun := request.GetBool("dry_run", true)

	cwd, err := os.Getwd()
	if err != nil {
		return mcp.NewToolResultErrorf("无法获取当前工作目录：%v", err), nil
	}

	cleaned, err := core.CleanCurrentProjectPorts(ctx, cwd, dryRun)
	if err != nil {
		return mcp.NewToolResultErrorf("清理当前项目端口失败：%v", err), nil
	}

	result := map[string]any{
		"cwd":     cwd,
		"dry_run": dryRun,
		"count":   len(cleaned),
		"cleaned": cleaned,
	}
	return jsonResult(result)
}

// ============================ Shared Helpers ============================

func jsonResult(v any) (*mcp.CallToolResult, error) {
	payload, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultErrorf("结果序列化失败：%v", err), nil
	}
	return mcp.NewToolResultText(string(payload)), nil
}

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
