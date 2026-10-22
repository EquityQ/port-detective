# CLI and MCP API Reference

Revision: **Unreleased after v1.1.1**. Published packages still provide v1.1.1;
build this checkout for the new commands and arguments below. Published CLI builds
use the release tag; source builds use a development version.

Keep the CLI, Python SDK, and Node.js client on the same reviewed commit or release.
The implementation inventory is generated during CI; this page is the stable behavior
contract.

## CLI

The global `--json` flag may appear before or after a subcommand. Other flags
belong to the commands listed below:

| Flag | Meaning |
| --- | --- |
| `--json` | Emit JSON; `check --watch` emits one compact diagnostic per line (NDJSON). |
| `--version` | Print the build version. |
| `--dry-run` | Preview process actions without terminating a process. |
| `--yes` | Skip interactive confirmation for destructive actions. |
| `--ports` | Port list or inclusive ranges for `scan` / `clean`, e.g. `3000-3005,8080`; sorted and deduplicated. |
| `--preset` | Select `web`, `python`, `database`, or opt-in `project` ports for `scan`; cannot be combined with `--ports`. |
| `--project` | Existing project directory for `clean` or `scan --preset project`; defaults to the current directory. |
| `--watch` | Repeat `check` until interrupted. |
| `--interval` | Positive poll duration for `check --watch` and `wait`. |
| `--state` | Target state for `wait`: `open` or `free`. |
| `--timeout` | Positive maximum wait duration. |
| `--from` / `--to` | Inclusive candidate range for `find-free`, within `1-65535`. |

| Command | Behavior and result |
| --- | --- |
| `check <port>` | Inspect TCP port `1-65535`; returns one `PortDiagnostic`, or repeated diagnostics with `--watch`. |
| `scan` | Scan `DefaultDevPorts`, `--ports`, or `--preset`; returns only occupied ports as a sorted `PortDiagnostic[]`. |
| `kill <port>` | CLI asks for confirmation by default; `--dry-run` previews, `--yes` executes without a prompt. Destructive JSON requires `--yes`. |
| `clean` | Discover all visible TCP listeners and match the resolved project. `--ports` limits discovery. Show targets before confirmation; `--dry-run` previews. Destructive JSON requires `--yes`. |
| `wait <port>` | Poll until `--state` matches or `--timeout` expires; return the matching diagnostic. An open TCP port does not prove application readiness. |
| `find-free` | Suggest a bindable TCP loopback port in the candidate range; the result is a snapshot, not a reservation. |
| `doctor` | Report local CLI version, binary location, OS/architecture, working directory, and temporary-listener process visibility with troubleshooting hints. |
| `mcp` | Start the Stdio MCP server with JSON-RPC only on stdout. |
| `completion <shell>` | Write a Bash, Zsh, Fish, or PowerShell completion script to stdout. |

Defaults: `wait` uses `--state open --timeout 30s --interval 250ms`;
`check --watch` uses `--interval 1s`; `find-free` searches `3000-3100`.

For project presets, create `.port-detective.json` at the resolved project root:

```json
{
  "ports": "3000-3005,8081"
}
```

Load it explicitly with `scan --preset project --project /path/to/project`.
Plain `scan` keeps the ten `DefaultDevPorts`; no configuration is loaded implicitly.

Process identity and listener ownership are revalidated immediately before termination.
The confirmed target snapshot is retained; a new owner is never substituted after
confirmation. Project cleanup groups listeners by process so a multi-port process
is terminated once. Project selection resolves an existing directory to its nearest
`package.json`, `go.mod`, `pyproject.toml`, or `requirements.txt` ancestor, or keeps
the selected directory if no manifest is found. It does not include unrelated
nested projects. Listener discovery is limited to the current host/network namespace.
PID 0 and 1 are never eligible targets. Permission-limited metadata is reported as a
degraded diagnostic rather than treated as proof that a port is free.

## JSON result

Success results go to stdout. With `--json`, execution errors go to stderr as
`{"error":{"code":"timeout","message":"...","exit_status":3}}`,
leaving successful arrays and NDJSON streams parseable. Project cleanup can emit
a partial result array before reporting its aggregate error; check the exit status.

| Exit status | Meaning |
| --- | --- |
| `0` | Completed, including an interactive action declined by the user. |
| `1` | Operation failed. |
| `2` | Invalid arguments or missing required destructive JSON confirmation. |
| `3` | Wait deadline expired. |
| `4` | No candidate loopback port is available. |
| `130` | Context canceled, such as an interrupted watch. |

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
`scan_dev_ports` always return only occupied ports as a JSON array, including an
empty array. System TCP listener tables cover bindings to specific local interfaces;
loopback probes provide a fallback when the table cannot be read. A fallback cannot
prove that an interface-only listener is absent. Default project discovery fails
explicitly if its listener table is unavailable.

## MCP tools

Start the server with `port-detective mcp`. Stdio stdout is reserved for
JSON-RPC; diagnostics belong on stderr.

| Tool | Input | Result and safety |
| --- | --- | --- |
| `inspect_port` | Required `port` number `1-65535` | Read-only diagnostic object. |
| `scan_dev_ports` | Optional `ports` string such as `"3000-3005,8080"` | Read-only occupied-port array; defaults to `DefaultDevPorts`. |
| `free_port` | Required `port`; optional `dry_run`, default `true` | Destructive when false; returns diagnostic and message. |
| `clean_current_project` | Optional `project_path`, `ports`, and `dry_run` (default `true`) | Match listeners to the selected project; defaults to the server working directory project. Returns effective `cwd`, `scope`, `dry_run`, `count`, and `cleaned`; partial failures set `isError` and `error`. |

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
