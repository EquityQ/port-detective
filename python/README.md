# port-detective Python SDK

A lightweight subprocess client for the `port-detective` CLI. It provides synchronous
and `asyncio` APIs for port diagnostics, project fingerprinting, and safe cleanup on
Windows, macOS, and Linux.

This checkout is the `v1.2.0` release candidate; PyPI and GitHub publication is pending.
Version checks, module entry points, and cleanup port selection require the v1.2.0 SDK
and matching CLI. Install both from source as shown below until publication is verified.
`v1.1.1` remains the latest verified public release. Major/minor compatibility
protects the shared API contract and does not add new commands to an older CLI.

## Install

After v1.2.0 publication is verified, install the exact SDK version:

```bash
python -m pip install port-detective==1.2.0
```

To use the v1.2.0 candidate now, run both commands from the repository root:

```bash
python -m pip install ./python
go build -o port-detective .
```

Set `PORT_DETECTIVE_BIN` to the absolute path of this local executable. On Windows,
build with `go build -o port-detective.exe .`. Automatic bootstrap and
`python -m port_detective install` require the matching GitHub Release to be published.

The SDK checks an explicit `binary_path`, `PORT_DETECTIVE_BIN`, `PATH`, and nearby
checkout binaries, in that order. External CLIs must have the same major and minor
version as the SDK: SDK `1.2.x` accepts CLI `1.2.x`, including `1.2.0-dev`, and
rejects CLI `1.1.x`. An
unrecognized or incompatible version raises `PortDetectiveVersionError` with the
resolved executable and repair instructions. Successful checks are cached until the
executable's file identity changes. If no CLI is available, the first API call
downloads the matching release archive from GitHub, verifies its SHA-256 checksum,
and caches the binary in
the user cache directory. This requires network access only once. Set
`PORT_DETECTIVE_AUTO_DOWNLOAD=0` for offline or locked-down environments, or set
`PORT_DETECTIVE_CACHE_DIR` to choose the cache location. You can also download it
explicitly with `from port_detective import download_cli`.

After publication, prepare the matching CLI before the first request or diagnose
installation and permissions. During the candidate phase, `doctor` can use the
local executable selected with `PORT_DETECTIVE_BIN`:

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
from port_detective import inspect_port, scan_ports, free_port, clean_project

diagnostic = inspect_port(8080)
occupied = scan_ports([3000, 5173, 8080])
preview = free_port(3000, dry_run=True)
project_listeners = clean_project(dry_run=True)
selected = clean_project(path="/workspace/my-app", ports=[3000, 8080])

print(diagnostic.status, len(occupied), preview.message, len(project_listeners))
```

All calls accept an optional timeout in seconds, shared by CLI version checking and
command execution. A first-use download uses its own bounded network timeout for
synchronous calls; async calls include discovery in the requested timeout.
Process actions default to dry-run; set `dry_run=False` only after reviewing the target.

`scan_ports` returns occupied ports with diagnostics in ascending port order. An
omitted list or `[]` scans the default development ports; it is not an empty scan.
Project cleanup discovers the project's TCP listeners, including ports outside the
default list. An optional `ports` list restricts cleanup to those ports; `[]` keeps
discovery enabled. The `path` directory scopes cleanup without changing the Python
process's working directory.
An existing directory resolves to its nearest `package.json`, `go.mod`,
`pyproject.toml`, or `requirements.txt` ancestor, or retains its own path when no
manifest exists. Selecting a subdirectory can therefore select its parent project;
unrelated nested projects are excluded.

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
`process_name`, `cmdline`, `memory_mb`, `cpu_percent`, `status`, and optional
project metadata. `ProjectInfo` includes `name`, `framework`, `path`, and
`version`.

See the [API reference](../docs/project/API_REFERENCE.md), [platform support policy](../docs/operations/PLATFORM_SUPPORT.md),
and [quality review](../docs/project/QUALITY_REVIEW.md) for release constraints.

## License

MIT.
