package mcpserver

import "github.com/mark3labs/mcp-go/mcp"

func serverTools() []mcp.Tool {
	return []mcp.Tool{
		inspectPortTool(),
		scanDevPortsTool(),
		freePortTool(),
		cleanCurrentProjectTool(),
	}
}

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
		mcp.WithString("protocol",
			mcp.Description("Optional transport protocol: 'tcp' (default) or 'udp'."),
		),
	)
}

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
		mcp.WithString("protocol",
			mcp.Description("Optional transport protocol: 'tcp' (default) or 'udp'."),
		),
	)
}

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
