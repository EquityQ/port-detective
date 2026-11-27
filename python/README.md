# port-detective Python SDK

A lightweight subprocess client for the `port-detective` CLI. It provides synchronous
and `asyncio` APIs for port diagnostics, project fingerprinting, and safe cleanup on
Windows, macOS, and Linux.

The `v1.4.0` SDK adds CLI workflow parity (`wait_for_port`, `find_free_port`, `doctor`,
UDP `protocol`, `preset`, `preview_id`, `bind_addresses`), `FreePortResult` and
`DoctorReport` models, PEP 561 `py.typed` metadata, and keeps fail-closed port selection.
Major/minor compatibility protects the shared API contract and does not add new commands
to an older CLI.

An explicitly empty port selection is rejected: `scan_ports(ports=[])` and
`clean_project(ports=[])` raise `ValueError` instead of silently scanning the default
ports or every project listener. Omit the argument, or pass `None`, to use the documented
default.

## Install

Install the published SDK from PyPI:

```bash
python -m pip install port-detective==1.4.0
```

For source development, install this checkout and build the matching CLI from the
repository root:

```bash
python -m pip install ./python
go build -o port-detective .
```

Set `PORT_DETECTIVE_BIN` to the absolute path of this local executable. On Windows,
build with `go build -o port-detective.exe .`. Automatic bootstrap can use the
published GitHub Release, or the local CLI can be selected with `PORT_DETECTIVE_BIN`.

The SDK checks an explicit `binary_path`, `PORT_DETECTIVE_BIN`, `PATH`, and nearby
checkout binaries, in that order. External CLIs must have the same major and minor
version as the SDK: SDK `1.4.x` accepts CLI `1.4.x`, including `1.4.0-dev`, and
rejects CLI `1.3.x`. An
unrecognized or incompatible version raises `PortDetectiveVersionError` with the
resolved executable and repair instructions. Successful checks are cached until the
executable's file identity changes. If no CLI is available, the first API call
downloads the matching release archive from GitHub, verifies its SHA-256 checksum,
and caches the binary in
the user cache directory. This requires network access only once. Set
`PORT_DETECTIVE_AUTO_DOWNLOAD=0` for offline or locked-down environments, or set
`PORT_DETECTIVE_CACHE_DIR` to choose the cache location. You can also download it
explicitly with `from port_detective import download_cli`.

Prepare the matching CLI before the first request or diagnose installation and
permissions:

```bash
python -m port_detective install
python -m port_detective doctor
python -m port_detective doctor --json
```

`install` prints preparation status to stderr and the cached executable path to
stdout. `doctor` reports the resolved executable on stderr and passes the CLI's
read-only diagnostic output through unchanged. Installation needs GitHub access;
Python's standard `HTTPS_PROXY`, `HTTP_PROXY`, and `NO_PROXY` settings apply.

For offline use, run `install --destination /path/to/cache` on an online computer
with the same OS and architecture. Transfer that cache directory, set
`PORT_DETECTIVE_CACHE_DIR` to it, and leave automatic downloads enabled so cache
resolution can reuse the verified file without network access. Alternatively set
`PORT_DETECTIVE_BIN` to the transferred executable and
`PORT_DETECTIVE_AUTO_DOWNLOAD=0` to prevent downloads entirely. Installing a cached
release does not override an incompatible CLI already selected by an explicit
path, environment variable, or `PATH`; update that selection when needed.

The package still requires Python 3.9 or newer. Source checkouts can build the CLI
locally with Go, while unsupported platforms must provide a compatible CLI through
`PORT_DETECTIVE_BIN`.

## Quickstart

```python
from port_detective import (
    inspect_port,
    scan_ports,
    free_port,
    clean_project,
    wait_for_port,
    find_free_port,
    doctor,
)

diagnostic = inspect_port(8080)
udp_diag = inspect_port(5353, protocol="udp")
occupied = scan_ports([3000, 5173, 8080])
web_ports = scan_ports(preset="web")
preview = free_port(3000, dry_run=True)
project_listeners = clean_project(dry_run=True)
selected = clean_project(path="/workspace/my-app", ports=[3000, 8080])
ready = wait_for_port(3000, state="open", wait_timeout=10.0)
free = find_free_port(port_from=3000, port_to=3100)
env_report = doctor()

print(diagnostic.status, diagnostic.bind_addresses, len(occupied), preview.preview_id, free.port, env_report.status)
```

All calls accept an optional timeout in seconds, shared by CLI version checking and
command execution. A first-use download uses its own bounded network timeout for
synchronous calls; async calls include discovery in the requested timeout.
Process actions default to dry-run; set `dry_run=False` only after reviewing the target,
and optionally pass `preview_id=preview.preview_id` to bind execution to the exact
previewed snapshot. Workflow APIs (`wait_for_port`, `find_free_port`, `doctor`,
`protocol="udp"`, `preset`, `preview_id`, and `bind_addresses`) are included in `v1.4.0`
together with async counterparts (`async_wait_for_port`,
`async_find_free_port`, `async_doctor`) and PEP 561 `py.typed` typing metadata.
On `main` (unreleased after `v1.4.0`), `clean_project_result` / `async_clean_project_result`
return a structured `CleanResult` envelope including `preview_id`, and `free_port` /
`async_free_port` accept `protocol="udp"`.

`scan_ports` returns occupied ports with diagnostics in ascending port order. An
omitted list or `None` scans the default development ports; an explicitly empty list
raises `ValueError` instead. Project cleanup discovers the project's TCP listeners,
including ports outside the default list. An optional `ports` list restricts cleanup
to those ports; `ports=[]` likewise raises `ValueError` rather than silently keeping
discovery enabled, so an empty selection can never widen the scope. The `path`
directory scopes cleanup without changing the Python
process's working directory.
An existing directory resolves to its nearest supported project manifest
(`package.json`, `go.mod`, `pyproject.toml`, `requirements.txt`, `Cargo.toml`,
`pom.xml`, `build.gradle`, `build.gradle.kts`, `Gemfile`, `composer.json`,
and on `main` also `deno.json`, `deno.jsonc`, `bunfig.toml`, and `*.csproj`), or
retains its own path when no manifest exists. Selecting a subdirectory can therefore
select its parent project; unrelated nested projects are excluded.

### Async API

```python
import asyncio
from port_detective import async_inspect_port, async_scan_ports

async def main():
    diagnostic, ports = await asyncio.gather(
        async_inspect_port(3000),
        async_scan_ports(),
    )
    print(diagnostic.status, len(ports))

asyncio.run(main())
```

Async discovery performs version checks with asynchronous child processes and keeps
first-use download work off the event loop. Timeout and cancellation stop and reap
the version or command child. A download already running in a worker thread may
finish populating the cache after cancellation; it never terminates a target process.

## Models

`PortDiagnostic` includes `port`, `is_open`, `protocol`, `pid`,
`process_name`, `cmdline`, `memory_mb`, `cpu_percent`, `status`, `bind_addresses`,
`listener_table_missing`, `post_action_status`, and optional `project` metadata.
`ProjectInfo` includes `name`, `framework`, `path`, and `version`.
`KillResult` exposes `port`, `dry_run`, `message`, `pre_action_diagnostic`,
`diagnostic`, `post_action_status`, `preview_id`, `preview_expires_at_unix`, and `protocol`.
`CleanResult` (on `main`) exposes `cwd`, `scope`, `dry_run`, `count`, `cleaned`,
`preview_id`, `preview_expires_at_unix`, `message`, and `error`.
`FreePortResult` and `DoctorReport` model `find_free_port` and `doctor` outputs.

See the [API reference](../docs/project/API_REFERENCE.md), [platform support policy](../docs/operations/PLATFORM_SUPPORT.md),
and [quality review](../docs/project/QUALITY_REVIEW.md) for release constraints.

## License

MIT.
