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
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/EquityQ/port-detective/internal/core"
	_ "github.com/EquityQ/port-detective/internal/fingerprint"
	"github.com/EquityQ/port-detective/internal/version"
)

const (
	serverName = "port-detective"
)

// NewServer creates an MCP server with all tools registered.
func NewServer() *server.MCPServer {
	s := server.NewMCPServer(
		serverName,
		version.Value,
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
			panic(fmt.Sprintf("MCP tool %q has no registered handler", tool.Name))
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
	info := ServerInfo{Name: serverName, Version: version.Value}

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
		mcp.WithTitleAnnotation("Inspect port"),
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
		mcp.WithTitleAnnotation("Scan ports"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("ports",
			mcp.Description("Optional ports and inclusive ranges, e.g. '3000-3005,8080'. Duplicates are removed. If omitted, scans DefaultDevPorts."),
		),
	)
}

// 3. free_port
func freePortTool() mcp.Tool {
	return mcp.NewTool("free_port",
		mcp.WithDescription(
			"Terminate the local process occupying the specified port. Set dry_run=false to actually kill the process.",
		),
		mcp.WithTitleAnnotation("Free port"),
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
			"Discover local TCP listeners belonging to the selected project, including non-default ports. Defaults to the server working directory project. Set dry_run=false to actually kill processes.",
		),
		mcp.WithTitleAnnotation("Clean current project"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("project_path",
			mcp.Description("Existing project directory on the MCP server host. Prefer an absolute path. Subdirectories resolve to the nearest project manifest; defaults to the server working directory."),
		),
		mcp.WithString("ports",
			mcp.Description("Optional port list or ranges, e.g. '3000-3005,8080'. If omitted, discovers all visible local TCP listeners before matching the project."),
		),
		mcp.WithBoolean("dry_run",
			mcp.Description("Whether to run in dry-run mode without actually terminating the processes. Defaults to true for safety."),
			mcp.DefaultBool(true),
		),
	)
}

// ============================ Tool Handlers ============================

func handleInspectPort(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	port, err := requirePort(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	diag, err := core.InspectPort(ctx, port)
	if err != nil {
		return mcp.NewToolResultErrorf("inspect port %d failed: %v", port, err), nil
	}

	return jsonResult(diag)
}

func handleScanDevPorts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	portsStr, err := optionalString(request, "ports")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	targetPorts := core.DefaultDevPorts
	if strings.TrimSpace(portsStr) != "" {
		targetPorts, err = core.ParsePortsList(portsStr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	results, err := core.ScanPortsConcurrent(ctx, targetPorts)
	if err != nil {
		return mcp.NewToolResultErrorf("scan failed: %v", err), nil
	}
	return jsonResult(results)
}

func handleFreePort(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	port, err := requirePort(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	dryRun, err := optionalDryRun(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	diag, msg, err := core.KillPortProcess(ctx, port, dryRun)
	if err != nil {
		return mcp.NewToolResultErrorf("free port %d failed: %v", port, err), nil
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
	projectPath, err := optionalString(request, "project_path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	portsStr, err := optionalString(request, "ports")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	dryRun, err := optionalDryRun(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	cwd, err := core.ResolveProjectPath(projectPath)
	if err != nil {
		return mcp.NewToolResultErrorf("invalid project_path: %v", err), nil
	}
	var ports []int
	scope := "all visible local TCP listeners"
	if strings.TrimSpace(portsStr) != "" {
		ports, err = core.ParsePortsList(portsStr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		scope = "specified TCP ports"
	}
	targets, err := core.DiscoverProjectPorts(ctx, cwd, ports)
	if err != nil {
		return mcp.NewToolResultErrorf("project discovery failed: %v", err), nil
	}
	cleaned, cleanErr := core.CleanProjectTargets(ctx, cwd, targets, dryRun)
	result := map[string]any{
		"cwd":     cwd,
		"scope":   scope,
		"dry_run": dryRun,
		"count":   len(cleaned),
		"cleaned": cleaned,
	}
	if cleanErr != nil {
		result["error"] = cleanErr.Error()
	}
	response, err := jsonResult(result)
	if err != nil {
		return nil, err
	}
	response.IsError = cleanErr != nil
	return response, nil
}

// ============================ Shared Helpers ============================

func requirePort(request mcp.CallToolRequest) (int, error) {
	value := request.GetArguments()["port"]
	var port float64
	switch v := value.(type) {
	case int:
		port = float64(v)
	case float64:
		port = v
	default:
		return 0, fmt.Errorf("port is required and must be an integer number from 1 to 65535")
	}
	if math.IsNaN(port) || port < 1 || port > 65535 || math.Trunc(port) != port {
		return 0, fmt.Errorf("port must be an integer number from 1 to 65535")
	}
	return int(port), nil
}

func optionalString(request mcp.CallToolRequest, name string) (string, error) {
	value, found := request.GetArguments()[name]
	if !found {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", name)
	}
	return text, nil
}

func optionalDryRun(request mcp.CallToolRequest) (bool, error) {
	value, found := request.GetArguments()["dry_run"]
	if !found {
		return true, nil
	}
	dryRun, ok := value.(bool)
	if !ok {
		return true, fmt.Errorf("dry_run must be a boolean; omit it for a safe preview")
	}
	return dryRun, nil
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	payload, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultErrorf("result serialization failed: %v", err), nil
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
