package mcpserver

import "github.com/mark3labs/mcp-go/mcp"

func serverTools() []mcp.Tool {
	return []mcp.Tool{
		inspectPortTool(),
		scanDevPortsTool(),
		freePortTool(),
		cleanCurrentProjectTool(),
		findFreePortTool(),
		waitForPortTool(),
		doctorTool(),
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
			mcp.Description("Optional ports and inclusive ranges, e.g. '3000-3005,8080'. Duplicates are removed. If omitted (along with preset), scans DefaultDevPorts."),
		),
		mcp.WithString("preset",
			mcp.Description("Optional port preset: 'web', 'python', 'database', or 'project'. Cannot be combined with ports."),
		),
		mcp.WithString("project_path",
			mcp.Description("Optional project directory when preset='project'; defaults to the server working directory."),
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
		mcp.WithString("protocol",
			mcp.Description("Optional transport protocol: 'tcp' (default) or 'udp'."),
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

func findFreePortTool() mcp.Tool {
	return mcp.NewTool("find_free_port",
		mcp.WithDescription(
			"Find the first TCP port currently bindable on IPv4 and supported IPv6 loopback interfaces. "+
				"The test listener is closed before returning, so the result is a snapshot rather than a reservation.",
		),
		mcp.WithTitleAnnotation("Find free port"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithNumber("from",
			mcp.Description("First candidate port (inclusive, 1-65535). Defaults to 3000."),
			mcp.Min(1),
			mcp.Max(65535),
		),
		mcp.WithNumber("to",
			mcp.Description("Last candidate port (inclusive, 1-65535). Defaults to 3100."),
			mcp.Min(1),
			mcp.Max(65535),
		),
	)
}

func waitForPortTool() mcp.Tool {
	return mcp.NewTool("wait_for_port",
		mcp.WithDescription(
			"Wait until a local port reaches the requested occupancy state ('open' or 'free') or the timeout expires. "+
				"An open port does not prove application readiness, and a degraded listener table never satisfies state='free'.",
		),
		mcp.WithTitleAnnotation("Wait for port"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithNumber("port",
			mcp.Required(),
			mcp.Description("Local port number to observe (1-65535)"),
			mcp.Min(1),
			mcp.Max(65535),
		),
		mcp.WithString("state",
			mcp.Description("Desired state: 'open' (default) or 'free'."),
		),
		mcp.WithNumber("timeout_ms",
			mcp.Description("Maximum wait duration in milliseconds (1-300000). Defaults to 30000."),
			mcp.Min(1),
			mcp.Max(300000),
		),
		mcp.WithNumber("interval_ms",
			mcp.Description("Polling interval in milliseconds (10-60000). Defaults to 250."),
			mcp.Min(10),
			mcp.Max(60000),
		),
		mcp.WithString("protocol",
			mcp.Description("Optional transport protocol: 'tcp' (default) or 'udp'."),
		),
	)
}

func doctorTool() mcp.Tool {
	return mcp.NewTool("doctor",
		mcp.WithDescription(
			"Diagnose CLI version, runtime platform, resolved project root, and local listener visibility using a temporary loopback socket.",
		),
		mcp.WithTitleAnnotation("Doctor diagnostics"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("project_path",
			mcp.Description("Optional existing directory to resolve as the project context; defaults to the server working directory."),
		),
	)
}
