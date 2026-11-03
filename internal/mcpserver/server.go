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
			"Preview or terminate the local process occupying a port. "+
				"dry_run=true (the default) returns a short-lived preview_id plus the exact process snapshot; "+
				"pass that preview_id back with dry_run=false to terminate exactly the previewed process. "+
				"Execution fails if the port owner changed since the preview was issued.",
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
			mcp.Description("Whether to preview without terminating. Defaults to true. Setting dry_run=false requires the preview_id returned by a prior dry run of this server."),
			mcp.DefaultBool(true),
		),
		mcp.WithString("preview_id",
			mcp.Description("Opaque preview token returned by a previous dry_run=true call. Required when dry_run=false so the action is bound to the process snapshot you actually previewed."),
		),
	)
}

// 4. clean_current_project
func cleanCurrentProjectTool() mcp.Tool {
	return mcp.NewTool("clean_current_project",
		mcp.WithDescription(
			"Discover local TCP listeners belonging to the selected project, including non-default ports. "+
				"Defaults to the server working directory project. dry_run=true (the default) returns a preview_id plus the exact target snapshot; "+
				"pass that preview_id back with dry_run=false to terminate exactly the previewed processes. "+
				"An explicitly empty ports value is rejected rather than widened to all listeners.",
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
			mcp.Description("Optional non-empty port list or ranges, e.g. '3000-3005,8080'. Omit the argument to discover all visible local TCP listeners; an empty or blank value is an error."),
		),
		mcp.WithBoolean("dry_run",
			mcp.Description("Whether to preview without terminating. Defaults to true. Setting dry_run=false requires the preview_id returned by a prior dry run of this server."),
			mcp.DefaultBool(true),
		),
		mcp.WithString("preview_id",
			mcp.Description("Opaque preview token returned by a previous dry_run=true call. Required when dry_run=false so the action is bound to the targets you actually previewed."),
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
	// Presence decides the meaning: an explicitly blank ports value is an error,
	// while an omitted argument means the documented default development ports.
	provided, portsStr, err := optionalStringPresent(request, "ports")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	targetPorts := core.DefaultDevPorts
	if provided {
		if strings.TrimSpace(portsStr) == "" {
			return mcp.NewToolResultError(
				"ports was provided but is empty; omit the argument to scan DefaultDevPorts, " +
					"or provide at least one port"), nil
		}
		targetPorts, err = core.ParsePortsList(portsStr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if len(targetPorts) == 0 {
			return mcp.NewToolResultError("ports resolved to an empty selection; omit the argument to scan DefaultDevPorts"), nil
		}
	}

	summary, err := core.ScanPortsConcurrentSummary(ctx, targetPorts)
	if err != nil {
		return mcp.NewToolResultErrorf("scan failed: %v", err), nil
	}
	// Keep the stable JSON array shape as the first content item so existing
	// clients keep parsing it, and append a warning when the result is incomplete.
	response, jsonErr := jsonResult(summary.Diagnostics)
	if jsonErr != nil {
		return nil, jsonErr
	}
	if summary.Degraded {
		response.Content = append(response.Content, mcp.TextContent{
			Type: "text",
			Text: "incomplete result: " + summary.Warning,
		})
	}
	return response, nil
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

	if dryRun {
		return previewFreePort(ctx, port)
	}
	return executeFreePort(ctx, request, port)
}

// previewFreePort inspects the port and returns a signed preview token that binds
// the confirmed process identity to a later execution.
func previewFreePort(ctx context.Context, port int) (*mcp.CallToolResult, error) {
	diag, err := core.InspectPort(ctx, port)
	if err != nil {
		return mcp.NewToolResultErrorf("free port %d preview failed: %v", port, err), nil
	}
	if !diag.IsOpen {
		return mcp.NewToolResultErrorf("port %d is currently free; no process to terminate", port), nil
	}
	token, expiresAt, err := core.IssueKillPreview(*diag, core.DefaultPreviewTTL)
	if err != nil {
		return mcp.NewToolResultErrorf("cannot preview port %d safely: %v", port, err), nil
	}
	// No action ran, so the outcome is explicitly "not attempted" rather than absent.
	diag.PostActionStatus = core.PostActionNotAttempted

	return jsonResult(map[string]any{
		"port":                    port,
		"dry_run":                 true,
		"preview_id":              token,
		"preview_expires_at_unix": expiresAt,
		"pre_action_diagnostic":   diag,
		"diagnostic":              diag, // Deprecated alias kept for older clients.
		"message": fmt.Sprintf(
			"dry run: would terminate process %q (PID: %d) occupying port %d; pass preview_id with dry_run=false within %s to execute",
			diag.ProcessName, diag.PID, port, core.DefaultPreviewTTL),
	})
}

// executeFreePort terminates exactly the process recorded in the preview token.
func executeFreePort(ctx context.Context, request mcp.CallToolRequest, port int) (*mcp.CallToolResult, error) {
	token, err := optionalString(request, "preview_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if strings.TrimSpace(token) == "" {
		return mcp.NewToolResultError(
			"dry_run=false requires the preview_id returned by a dry_run=true call; " +
				"this binds the action to the process you actually previewed"), nil
	}

	targets, expiresAt, err := core.RestorePreviewToken(core.PreviewScopeKill, token, []int{port})
	if err != nil {
		return mcp.NewToolResultErrorf("preview_id rejected for port %d: %v", port, err), nil
	}
	// The token proves what was previewed; core still revalidates the live port and
	// fails on any drift instead of redirecting the action to a new owner.
	diag, msg, err := core.KillPortTarget(ctx, targets[0], false)
	if err != nil {
		return mcp.NewToolResultErrorf("free port %d failed: %v", port, err), nil
	}

	return jsonResult(map[string]any{
		"port":                    port,
		"dry_run":                 false,
		"preview_expires_at_unix": expiresAt,
		"pre_action_diagnostic":   diag,
		"diagnostic":              diag, // Deprecated alias kept for older clients.
		"post_action_status":      diag.PostActionStatus,
		"message":                 msg,
	})
}

func handleCleanCurrentProject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectPath, err := optionalString(request, "project_path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	// Presence, not blankness, decides the meaning of "ports": an explicitly
	// supplied blank value must fail instead of widening scope to every listener.
	portsProvided, portsStr, err := optionalStringPresent(request, "ports")
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
	if portsProvided {
		if strings.TrimSpace(portsStr) == "" {
			return mcp.NewToolResultError(
				"ports was provided but is empty; omit the argument to discover all listeners, " +
					"or provide at least one port. Refusing to widen an empty selection"), nil
		}
		ports, err = core.ParsePortsList(portsStr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if len(ports) == 0 {
			return mcp.NewToolResultError(
				"ports resolved to an empty selection; omit the argument to discover all listeners"), nil
		}
		scope = "specified TCP ports"
	}
	if dryRun {
		return previewCleanCurrentProject(ctx, request, cwd, ports, scope)
	}
	return executeCleanCurrentProject(ctx, request, cwd, ports, scope)
}

// previewCleanCurrentProject discovers targets, revalidates them, and binds them
// into a preview token. The `cleaned` field keeps its historical dry-run meaning
// so existing clients are unaffected; `targets` and `preview_id` are additive.
func previewCleanCurrentProject(ctx context.Context, request mcp.CallToolRequest, cwd string, ports []int, scope string) (*mcp.CallToolResult, error) {
	targets, err := core.DiscoverProjectPorts(ctx, cwd, ports)
	if err != nil {
		return mcp.NewToolResultErrorf("project discovery failed: %v", err), nil
	}
	// Dry-run through the normal path so the preview validates exactly what an
	// execution would act on, and reports the same partial failures it always has.
	cleaned, previewErr := core.CleanProjectTargets(ctx, cwd, targets, true)
	result := map[string]any{
		"cwd":     cwd,
		"scope":   scope,
		"dry_run": true,
		"count":   len(cleaned),
		"targets": targets,
		"cleaned": cleaned,
	}
	if len(targets) == 0 {
		result["message"] = "no project-scoped listeners found in this scope; nothing to preview"
	} else if token, expiresAt, tokenErr := core.IssueCleanPreview(targets, core.DefaultPreviewTTL); tokenErr != nil {
		result["message"] = "preview is not executable: " + tokenErr.Error()
	} else {
		result["preview_id"] = token
		result["preview_expires_at_unix"] = expiresAt
		result["message"] = fmt.Sprintf(
			"dry run: would terminate %d project listener(s); pass preview_id with dry_run=false within %s to execute",
			len(targets), core.DefaultPreviewTTL)
	}
	if previewErr != nil {
		result["error"] = previewErr.Error()
	}
	response, jsonErr := jsonResult(result)
	if jsonErr != nil {
		return nil, jsonErr
	}
	response.IsError = previewErr != nil
	return response, nil
}

// executeCleanCurrentProject acts only on the targets pinned by the preview token.
func executeCleanCurrentProject(ctx context.Context, request mcp.CallToolRequest, cwd string, ports []int, scope string) (*mcp.CallToolResult, error) {
	token, err := optionalString(request, "preview_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if strings.TrimSpace(token) == "" {
		return mcp.NewToolResultError(
			"dry_run=false requires the preview_id returned by a dry_run=true call; " +
				"this binds the action to the targets you actually previewed"), nil
	}
	var expected []int
	if ports != nil {
		expected = ports
	}
	targets, expiresAt, err := core.RestorePreviewToken(core.PreviewScopeClean, token, expected)
	if err != nil {
		return mcp.NewToolResultErrorf("preview_id rejected for project cleanup: %v", err), nil
	}
	cleaned, cleanErr := core.CleanProjectTargets(ctx, cwd, targets, false)
	result := map[string]any{
		"cwd":                     cwd,
		"scope":                   scope,
		"dry_run":                 false,
		"preview_expires_at_unix": expiresAt,
		"count":                   len(cleaned),
		"cleaned":                 cleaned,
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

// optionalStringPresent distinguishes "argument omitted" from "argument supplied
// but empty". Destructive scope decisions must never treat those as equivalent:
// an explicitly empty selection is rejected instead of widening to everything.
func optionalStringPresent(request mcp.CallToolRequest, name string) (bool, string, error) {
	value, found := request.GetArguments()[name]
	if !found {
		return false, "", nil
	}
	text, ok := value.(string)
	if !ok {
		return true, "", fmt.Errorf("%s must be a string", name)
	}
	return true, text, nil
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
