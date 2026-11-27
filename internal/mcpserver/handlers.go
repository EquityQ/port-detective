package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/EquityQ/port-detective/internal/core"
	"github.com/EquityQ/port-detective/internal/version"
)

type mcpHandlers struct {
	inspector *core.Inspector
}

func handleInspectPort(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return (&mcpHandlers{}).inspectPort(ctx, request)
}

func handleScanDevPorts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return (&mcpHandlers{}).scanDevPorts(ctx, request)
}

func handleFreePort(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return (&mcpHandlers{}).freePort(ctx, request)
}

func handleCleanCurrentProject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return (&mcpHandlers{}).cleanCurrentProject(ctx, request)
}

func handleFindFreePort(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return (&mcpHandlers{}).findFreePort(ctx, request)
}

func handleWaitForPort(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return (&mcpHandlers{}).waitForPort(ctx, request)
}

func handleDoctor(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return (&mcpHandlers{}).doctor(ctx, request)
}

func (h *mcpHandlers) inspectPort(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	port, err := requirePort(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	rawProto, err := optionalString(request, "protocol")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	proto, err := core.NormalizeProtocol(rawProto)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	diag, err := h.inspector.InspectPortProtocol(ctx, port, proto)
	if err != nil {
		return mcp.NewToolResultErrorf("inspect port %d failed: %v", port, err), nil
	}

	return jsonResult(diag)
}

func (h *mcpHandlers) scanDevPorts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawProto, err := optionalString(request, "protocol")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	proto, err := core.NormalizeProtocol(rawProto)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	// Presence decides the meaning: an explicitly blank ports value is an error,
	// while an omitted argument means the documented default development ports.
	provided, portsStr, err := optionalStringPresent(request, "ports")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	presetProvided, preset, err := optionalStringPresent(request, "preset")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	projectProvided, projectPath, err := optionalStringPresent(request, "project_path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if provided && presetProvided {
		return mcp.NewToolResultError("ports and preset cannot be used together"), nil
	}
	if projectProvided && (!presetProvided || preset != "project") {
		return mcp.NewToolResultError("project_path requires preset='project'"), nil
	}

	targetPorts := core.DefaultDevPorts
	switch {
	case presetProvided:
		if strings.TrimSpace(preset) == "" {
			return mcp.NewToolResultError("preset was provided but is empty; expected web, python, database, or project"), nil
		}
		if projectProvided && strings.TrimSpace(projectPath) == "" {
			return mcp.NewToolResultError("project_path was provided but is empty"), nil
		}
		targetPorts, _, err = core.ResolvePresetPorts(preset, projectPath)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	case provided:
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

	summary, err := h.inspector.ScanPortsProtocolSummary(ctx, targetPorts, proto)
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

func (h *mcpHandlers) freePort(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	port, err := requirePort(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	rawProto, err := optionalString(request, "protocol")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	proto, err := core.NormalizeProtocol(rawProto)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	dryRun, err := optionalDryRun(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if dryRun {
		return h.previewFreePort(ctx, port, proto)
	}
	return h.executeFreePort(ctx, request, port, proto)
}

// previewFreePort inspects the port and returns a signed preview token that binds
// the confirmed process identity to a later execution.
func (h *mcpHandlers) previewFreePort(ctx context.Context, port int, protocol string) (*mcp.CallToolResult, error) {
	diag, err := h.inspector.InspectPortProtocol(ctx, port, protocol)
	if err != nil {
		return mcp.NewToolResultErrorf("free port %d preview failed: %v", port, err), nil
	}
	if !diag.IsOpen {
		return mcp.NewToolResultErrorf("port %d (%s) is currently free; no process to terminate", port, protocol), nil
	}
	token, expiresAt, err := core.IssueKillPreview(*diag, core.DefaultPreviewTTL)
	if err != nil {
		return mcp.NewToolResultErrorf("cannot preview port %d safely: %v", port, err), nil
	}
	// No action ran, so the outcome is explicitly "not attempted" rather than absent.
	diag.PostActionStatus = core.PostActionNotAttempted

	return jsonResult(map[string]any{
		"port":                    port,
		"protocol":                protocol,
		"dry_run":                 true,
		"preview_id":              token,
		"preview_expires_at_unix": expiresAt,
		"pre_action_diagnostic":   diag,
		"diagnostic":              diag, // Deprecated alias kept for older clients.
		"message": fmt.Sprintf(
			"dry run: would terminate process %q (PID: %d) occupying %s port %d; pass preview_id with dry_run=false within %s to execute",
			diag.ProcessName, diag.PID, strings.ToUpper(protocol), port, core.DefaultPreviewTTL),
	})
}

// executeFreePort terminates exactly the process recorded in the preview token.
func (h *mcpHandlers) executeFreePort(ctx context.Context, request mcp.CallToolRequest, port int, protocol string) (*mcp.CallToolResult, error) {
	token, err := optionalString(request, "preview_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if strings.TrimSpace(token) == "" {
		return mcp.NewToolResultError(
			"dry_run=false requires the preview_id returned by a dry_run=true call; " +
				"this binds the action to the process you actually previewed"), nil
	}

	targets, expiresAt, err := core.RestorePreviewTokenProtocol(core.PreviewScopeKill, token, []int{port}, protocol)
	if err != nil {
		return mcp.NewToolResultErrorf("preview_id rejected for port %d: %v", port, err), nil
	}
	// The token proves what was previewed; core still revalidates the live port and
	// fails on any drift instead of redirecting the action to a new owner.
	diag, msg, err := h.inspector.KillPortTarget(ctx, targets[0], false)
	if err != nil {
		return mcp.NewToolResultErrorf("free port %d failed: %v", port, err), nil
	}

	return jsonResult(map[string]any{
		"port":                    port,
		"protocol":                protocol,
		"dry_run":                 false,
		"preview_expires_at_unix": expiresAt,
		"pre_action_diagnostic":   diag,
		"diagnostic":              diag, // Deprecated alias kept for older clients.
		"post_action_status":      diag.PostActionStatus,
		"message":                 msg,
	})
}

func (h *mcpHandlers) cleanCurrentProject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		return h.previewCleanCurrentProject(ctx, cwd, ports, scope)
	}
	return h.executeCleanCurrentProject(ctx, request, cwd, ports, scope)
}

// previewCleanCurrentProject discovers targets, revalidates them, and binds them
// into a preview token. The `cleaned` field keeps its historical dry-run meaning
// so existing clients are unaffected; `targets` and `preview_id` are additive.
func (h *mcpHandlers) previewCleanCurrentProject(ctx context.Context, cwd string, ports []int, scope string) (*mcp.CallToolResult, error) {
	targets, err := h.inspector.DiscoverProjectPorts(ctx, cwd, ports)
	if err != nil {
		return mcp.NewToolResultErrorf("project discovery failed: %v", err), nil
	}
	// Dry-run through the normal path so the preview validates exactly what an
	// execution would act on, and reports the same partial failures it always has.
	cleaned, previewErr := h.inspector.CleanProjectTargets(ctx, cwd, targets, true)
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
func (h *mcpHandlers) executeCleanCurrentProject(ctx context.Context, request mcp.CallToolRequest, cwd string, ports []int, scope string) (*mcp.CallToolResult, error) {
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
	cleaned, cleanErr := h.inspector.CleanProjectTargets(ctx, cwd, targets, false)
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

func (h *mcpHandlers) findFreePort(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	from, err := optionalInt(request, "from", 3000, 1, 65535)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	to, err := optionalInt(request, "to", 3100, 1, 65535)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if from > to {
		return mcp.NewToolResultError("expected 1 <= from <= to <= 65535"), nil
	}
	result, err := h.inspector.FindFreePort(ctx, from, to)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return jsonResult(result)
}

func (h *mcpHandlers) waitForPort(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	port, err := requirePort(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	state, err := optionalString(request, "state")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if state == "" {
		state = "open"
	}
	timeoutMs, err := optionalInt(request, "timeout_ms", 30000, 1, 300000)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	intervalMs, err := optionalInt(request, "interval_ms", 250, 10, 60000)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	rawProto, err := optionalString(request, "protocol")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	diag, err := h.inspector.WaitForPortProtocol(
		ctx,
		port,
		rawProto,
		state,
		time.Duration(timeoutMs)*time.Millisecond,
		time.Duration(intervalMs)*time.Millisecond,
	)
	if err != nil {
		return mcp.NewToolResultErrorf("wait_for_port %d failed: %v", port, err), nil
	}
	return jsonResult(diag)
}

func (h *mcpHandlers) doctor(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectProvided, projectPath, err := optionalStringPresent(request, "project_path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	cwd := ""
	if projectProvided {
		if strings.TrimSpace(projectPath) == "" {
			return mcp.NewToolResultError("project_path was provided but is empty"), nil
		}
		cwd, err = core.ResolveProjectPath(projectPath)
		if err != nil {
			return mcp.NewToolResultErrorf("invalid project_path: %v", err), nil
		}
	}
	report, err := h.inspector.RunDoctor(ctx, "", cwd, version.Value)
	if err != nil {
		return mcp.NewToolResultErrorf("doctor failed: %v", err), nil
	}
	return jsonResult(report)
}
