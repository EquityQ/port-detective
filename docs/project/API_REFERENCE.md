# CLI and MCP API Reference

Revision: **v1.1.1**. Published CLI builds use the release tag; source builds use a
development version until the next release.

Keep the CLI, Python SDK, and Node.js client on the same reviewed commit or release.
The implementation inventory is generated during CI; this page is the stable behavior
contract.

## CLI

Global flags may appear before or after a subcommand:

| Flag | Meaning |
| --- | --- |
| `--json` | Emit machine-readable JSON for `check`, `scan`, `kill`, or `clean`. |
| `--version` | Print the build version. |
| `--dry-run` | Preview process actions without terminating a process. |
| `--yes` | Skip interactive confirmation for destructive actions. |
| `--ports` | Provide a comma-separated scan list. |

| Command | Behavior and result |
| --- | --- |
| `check <port>` | Inspect TCP port `1-65535`; returns one `PortDiagnostic`. |
| `scan` | Scan `DefaultDevPorts` or `--ports`; returns a sorted `PortDiagnostic[]`. |
| `kill <port>` | Preview by default; real termination requires disabled dry-run and `--yes` for JSON. |
| `clean` | Operate on listeners belonging to the current project; preview by default. |
| `mcp` | Start the Stdio MCP server with JSON-RPC only on stdout. |
| `completion <shell>` | Write a Bash, Zsh, Fish, or PowerShell completion script to stdout. |

Process identity and listener ownership are revalidated immediately before termination.
PID 0 and 1 are never eligible targets. Permission-limited metadata is reported as a
degraded diagnostic rather than treated as proof that a port is free.

## JSON result

`PortDiagnostic` is the shared result model:

```json
{
  "port": 8080,
  "is_open": true,
  "protocol": "tcp",
  "pid": 1234,
  "process_name": "node",
  "cmdline": "node server.js",
  "memory_mb": 42.5,
  "cpu_percent": 1.2,
  "status": "occupied",
  "project": {
    "name": "example-app",
    "framework": "Express",
    "path": "/workspace/example-app",
    "version": "1.0.0"
  }
}
```

`status` is normally `occupied`, `free`, or `permission_required`. Process
fields can be empty or zero when the operating system denies access. `scan` and
`scan_dev_ports` always return a JSON array, including an empty array.

## MCP tools

Start the server with `port-detective mcp`. Stdio stdout is reserved for
JSON-RPC; diagnostics belong on stderr.

| Tool | Input | Result and safety |
| --- | --- | --- |
| `inspect_port` | Required `port` number `1-65535` | Read-only diagnostic object. |
| `scan_dev_ports` | Optional `ports` string such as `"3000,8080"` | Read-only array; defaults to `DefaultDevPorts`. |
| `free_port` | Required `port`; optional `dry_run`, default `true` | Destructive when false; returns diagnostic and message. |
| `clean_current_project` | Optional `dry_run`, default `true` | Destructive when false; scopes listeners to the current working directory. |

The frozen core API exposes PortDiagnostic, DefaultDevPorts,
InspectPort, ScanPortsConcurrent, and KillPortProcess. The CLI and MCP
layers consume these contracts without changing field names or safety rules.

Tool descriptions and annotations are checked by the acceptance scripts in
`scripts/mcp/`. SDKs consume the CLI JSON contract instead of reimplementing process
inspection.

## Compatibility

Within the v1.1 development line, fields may be added. Changes to field names, command
flags, tool names, or safety defaults require updates to this page, SDK models, acceptance
tests, and the changelog.
