# CLI and MCP API Reference

Revision: **v1.5.0**, released 2026-11-27. Published CLI and SDK packages use
the same major/minor compatibility contract.

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
| `--protocol` | Transport protocol for `check`, `scan`, and `kill`: `tcp` (default) or `udp`. |
| `--preset` | Select `web`, `python`, `database`, or opt-in `project` ports for `scan`; cannot be combined with `--ports`. |
| `--project` | Existing project directory for `clean` or `scan --preset project`; defaults to the current directory. |
| `--envelope` | Wrap `clean --json` output in an object containing `cwd`, `scope`, `dry_run`, `count`, `cleaned`, and `preview_id`. |
| `--watch` | Repeat `check` until interrupted. |
| `--interval` | Positive poll duration for `check --watch` and `wait`. |
| `--state` | Target state for `wait`: `open` or `free`. |
| `--timeout` | Positive maximum wait duration. |
| `--from` / `--to` | Inclusive candidate range for `find-free`, within `1-65535`. |
| `--preview-id` | Opaque token from an earlier preview (`kill --dry-run`, `clean --dry-run`, or an MCP `dry_run=true` call). Binds the action to the exact process snapshot that was previewed. CLI tokens use a user-scoped key file (rotated every 24h, overridable via `PORT_DETECTIVE_PREVIEW_KEY_FILE`) so previews work across CLI invocations; MCP keeps an in-memory per-server key. |

| Command | Behavior and result |
| --- | --- |
| `check <port>` | Inspect TCP or UDP port `1-65535`; returns one `PortDiagnostic` (including `bind_addresses`), or repeated diagnostics with `--watch`. |
| `scan` | Scan `DefaultDevPorts`, `--ports`, or `--preset` over TCP (default) or UDP (`--protocol udp`); returns only occupied ports as a sorted `PortDiagnostic[]`. |
| `kill <port>` | CLI asks for confirmation by default; `--dry-run` previews, `--yes` executes without a prompt. Destructive JSON requires `--yes`. `--dry-run` emits a `preview_id` (in both JSON and terminal output); pass it back with `--preview-id` across CLI invocations to bind execution to the exact previewed process. |
| `clean` | Discover all visible TCP listeners and match the resolved project. `--ports` limits discovery; an explicitly empty `--ports` is rejected rather than widened. Show targets before confirmation; `--dry-run` previews (emitting a `preview_id` in terminal mode). Destructive JSON requires `--yes`. Accepts `--preview-id` to validate and act only on a previously previewed target set. |
| `wait <port>` | Poll until `--state` matches or `--timeout` expires; return the matching diagnostic. An open TCP port does not prove application readiness, and `--state free` never accepts an `undetermined` degraded result. |
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
`package.json`, `go.mod`, `pyproject.toml`, `requirements.txt`, `Cargo.toml`,
`pom.xml`, `build.gradle`, `build.gradle.kts`, `Gemfile`, `composer.json`,
`deno.json`, `deno.jsonc`, `bunfig.toml`, or `*.csproj` ancestor,
or keeps the selected directory if no manifest is found. It does not include unrelated
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
| `5` | `doctor` self-check completed with limitations; read its hints instead of assuming full visibility. |
| `6` | A supplied `--preview-id` was rejected: malformed, expired, issued by another process, or issued for a different selection. |
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
  },
  "bind_addresses": ["127.0.0.1"],
  "listener_table_missing": true,
  "post_action_status": "released"
}
```

`status` is normally `occupied`, `free`, `permission_required`, or `undetermined`.
Process fields can be empty or zero when the operating system denies access.
`cpu_percent` is an instantaneous single-sample OS snapshot (it does not sleep to
compute a multi-interval delta, so some platforms may report `0` on the first sample).
`bind_addresses` lists unique local IP addresses observed in the OS socket table
(for example `["127.0.0.1"]` or `["0.0.0.0", "::"]`).

`listener_table_missing` appears only when the local TCP listener table could not
be read. In that state a closed loopback probe is reported as `undetermined`, never
`free`, because a listener bound only to another interface stays invisible to that
probe. Never treat a degraded diagnostic as proof that a port is available. `scan`
and `scan_dev_ports` keep returning only occupied ports as a JSON array; a degraded
scan adds a warning instead of changing that array shape.

`post_action_status` appears on the diagnostics returned by destructive actions and
records the verified outcome: `released`, `release_unverified`, or `not_attempted`.
The rest of such a diagnostic remains the pre-action observation, which is why
`kill --json` and `free_port` also name it `pre_action_diagnostic`; the legacy
`diagnostic` key is kept as a deprecated alias with the same value. A successful
kill therefore reports `is_open: true` for the state it observed *before* acting,
with `post_action_status: "released"` describing the outcome.

System TCP listener tables cover bindings to specific local interfaces; loopback
probes provide a fallback when the table cannot be read. A fallback cannot prove
that an interface-only listener is absent. Default project discovery fails
explicitly if its listener table is unavailable.

## MCP tools

Start the server with `port-detective mcp`. Stdio stdout is reserved for
JSON-RPC; diagnostics belong on stderr.

| Tool | Input | Result and safety |
| --- | --- | --- |
| `inspect_port` | Required `port` number `1-65535`; optional `protocol` (`"tcp"` default or `"udp"`) | Read-only diagnostic object including `bind_addresses`. |
| `scan_dev_ports` | Optional `ports` string such as `"3000-3005,8080"`, or `preset` (`"web"`, `"python"`, `"database"`, `"project"`) with optional `project_path`; optional `protocol` (`"tcp"` default or `"udp"`) | Read-only occupied-port array; omitting `ports` and `preset` scans `DefaultDevPorts`. `ports` and `preset` are mutually exclusive; a provided-but-blank value is an error. |
| `free_port` | Required `port`; optional `dry_run` (default `true`), `preview_id`, `protocol` (`"tcp"` default or `"udp"`) | `dry_run=true` previews and returns a `preview_id` plus the process snapshot. Setting `dry_run=false` requires that `preview_id`, bound to the exact port, process identity, and protocol. |
| `clean_current_project` | Optional `project_path`, `ports`, `dry_run` (default `true`), `preview_id` | Match listeners to the selected project; defaults to the server working directory project. Returns effective `cwd`, `scope`, `dry_run`, `count`, `targets`, and `cleaned`; partial failures set `isError` and `error`. Omitting `ports` discovers all visible listeners; a provided-but-blank value is an error. `dry_run=false` requires the `preview_id` from a prior preview. |
| `find_free_port` | Optional `from` (default `3000`), `to` (default `3100`) | Read-only loopback TCP bindability check returning a `FreePortResult` snapshot (`reserved: false`). |
| `wait_for_port` | Required `port`; optional `state` (`"open"` default or `"free"`), `timeout_ms` (default `30000`), `interval_ms` (default `250`), `protocol` (`"tcp"` default or `"udp"`) | Read-only polling until the target port reaches the requested state or the timeout expires. |
| `doctor` | Optional `project_path` | Read-only environment and temporary-listener visibility report (`DoctorReport`). |

The frozen core API exposes `PortDiagnostic`, `DefaultDevPorts`,
`InspectPort`, `ScanPortsConcurrent`, and `KillPortProcess`. Additive helpers
(`InspectPortProtocol`, `ScanPortsProtocolSummary`, `KillPortProcessProtocol`,
`ResolvePresetPorts`, `FindFreePort`, `WaitForPort`, `WaitForPortProtocol`,
`RunDoctor`, `core.Inspector`, `IssueCLIPreviewToken`, `RestoreCLIPreviewToken`,
`RestoreCLIPreviewTokenProtocol`, `RestorePreviewTokenProtocol`, and
`SupportedProjectManifests`) extend the core without altering any frozen signature
or JSON field name. The CLI and MCP layers consume these contracts without changing
field names or safety rules.

Tool descriptions, annotations, and cross-language `PortDiagnostic` schema parity
across Go, Python (`models.py`), and TypeScript (`index.d.ts`) are verified by
`scripts/quality/verify_api_reference.py` and the acceptance scripts in `scripts/mcp/`.
SDKs consume the CLI JSON contract instead of reimplementing process inspection.

## Compatibility

Within the v1.5 development line, fields may be added. SDK 1.5.x accepts CLI 1.5.x,
including 1.5.0-dev, and rejects CLI 1.4.x. Changes to field names, command
flags, tool names, or safety defaults require updates to this page, SDK models, acceptance
tests, and the changelog.

## Breaking changes in v1.3.0

Two changes require caller updates; both replace silently unsafe behavior with an
explicit failure.

- MCP `free_port` and `clean_current_project` reject `dry_run=false` unless it is
  accompanied by the `preview_id` returned from a prior `dry_run=true` call. Preview
  first, then execute with the token.
- The Python and Node.js SDKs raise `ValueError` / `TypeError` for an explicitly empty
  port selection (`ports=[]` / `[]`). Omit the argument, or pass `None` / `undefined`, to
  use the documented default.

The CLI is unaffected: `kill --json --yes` still executes without a token, and
`--preview-id` is an optional hardening path.
