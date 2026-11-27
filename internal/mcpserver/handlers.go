package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/EquityQ/port-detective/internal/core"
)

func handleInspectPort(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	diag, err := core.InspectPortProtocol(ctx, port, proto)
	if err != nil {
		return mcp.NewToolResultErrorf("inspect port %d failed: %v", port, err), nil
	}

	return jsonResult(diag)
}

func handleScanDevPorts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	summary, err := core.ScanPortsProtocolSummary(ctx, targetPorts, proto)
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
		return previewFreePort(ctx, port, proto)
	}
	return executeFreePort(ctx, request, port, proto)
}

// previewFreePort inspects the port and returns a signed preview token that binds
// the confirmed process identity to a later execution.
func previewFreePort(ctx context.Context, port int, protocol string) (*mcp.CallToolResult, error) {
	diag, err := core.InspectPortProtocol(ctx, port, protocol)
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
func executeFreePort(ctx context.Context, request mcp.CallToolRequest, port int, protocol string) (*mcp.CallToolResult, error) {
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
	diag, msg, err := core.KillPortTarget(ctx, targets[0], false)
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
		return previewCleanCurrentProject(ctx, cwd, ports, scope)
	}
	return executeCleanCurrentProject(ctx, request, cwd, ports, scope)
}

// previewCleanCurrentProject discovers targets, revalidates them, and binds them
// into a preview token. The `cleaned` field keeps its historical dry-run meaning
// so existing clients are unaffected; `targets` and `preview_id` are additive.
func previewCleanCurrentProject(ctx context.Context, cwd string, ports []int, scope string) (*mcp.CallToolResult, error) {
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
