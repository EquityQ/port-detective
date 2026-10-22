# port-detective

Cross-platform CLI, MCP server, and Python/Node.js SDKs for inspecting local TCP ports,
identifying owning processes, and cleaning project-scoped listeners safely.

> **Latest release:** `v1.2.0` is published on [GitHub](https://github.com/EquityQ/port-detective/releases/tag/v1.2.0),
> [PyPI](https://pypi.org/project/port-detective/1.2.0/), and [npm](https://www.npmjs.com/package/port-detective/v/1.2.0).

## Quick Start

Requirements: Go 1.25.5+ only when building the CLI from source, Python 3.9+ for the
Python SDK, and Node.js 22+ for the Node.js client and npm-based MCP setup.

Inspect a port immediately; no Go installation is needed:

```bash
npx --yes --package=port-detective@1.2.0 port-detective check 3000
```

Install the CLI for repeated use:

```bash
npm install -g port-detective@1.2.0
```

```bash
port-detective check 3000
port-detective scan --ports 3000,5173,8080
port-detective kill 3000 --dry-run
port-detective clean --dry-run
```

Use `--json` for machine-readable output. CLI `kill` and `clean` ask for
confirmation by default; `--dry-run` previews without acting. SDK and MCP process
actions default to dry-run.
Destructive JSON commands require an explicit `--yes` flag:

```bash
port-detective kill 3000 --json --yes
port-detective clean --json --yes
```

To build the CLI from source with Go 1.25.5 or newer:

```bash
git clone https://github.com/EquityQ/port-detective.git
cd port-detective
go build -o port-detective .
./port-detective check 3000
```

On Windows, build with `go build -o port-detective.exe .` and run
`.\port-detective.exe check 3000`. The current directory is usually absent from PATH.
Run the test suite with:

```bash
go test -race ./...
```

## Development Workflows

These examples are available in the published v1.2.0 CLI. They assume it is on
PATH; otherwise use the local executable path shown above.

```bash
port-detective scan --ports 3000-3005,8080
port-detective scan --preset web
port-detective scan --preset project --project /path/to/project
port-detective clean --project /path/to/project --dry-run
port-detective clean --project /path/to/project --ports 3000-3010,8081 --dry-run
port-detective wait 3000 --state open --timeout 30s
port-detective check 3000 --watch --interval 1s
port-detective find-free --from 3000 --to 3100
port-detective doctor
```

Cleanup discovers all visible TCP listeners, matches the resolved project directory,
and shows the target list before confirmation. Running from a subdirectory resolves
to the nearest supported project manifest. Wait observes port state rather than
application readiness; a suggested free port is a snapshot, not a reservation.
See the [API reference](docs/project/API_REFERENCE.md) for JSON and error contracts.

## Set Up with an AI Agent

The MCP server runs on the same computer as your AI client. The published npm
package starts it without Go or a local build. Node.js 22+ and network access are
needed the first time the package is downloaded. Python installs the SDK, not the
MCP server.

Add a `port-detective` server to your AI client's MCP configuration. Client config
formats and locations vary; use the matching client instructions and adapt this
standard `mcpServers` example:

macOS and Linux:

```json
{
  "mcpServers": {
    "port-detective": {
      "command": "npx",
      "args": ["--yes", "--package=port-detective@1.2.0", "port-detective", "mcp"]
    }
  }
}
```

Windows:

```json
{
  "mcpServers": {
    "port-detective": {
      "command": "cmd.exe",
      "args": ["/d", "/s", "/c", "npx --yes --package=port-detective@1.2.0 port-detective mcp"]
    }
  }
}
```

If you want your AI agent to do the setup, send it this request and name your MCP
client if it is not obvious:

> Set up the port-detective MCP server for this AI client. Detect my operating
> system and client, use the official client-specific configuration format, and
> use the published port-detective v1.2.0 npm package. Make the smallest
> appropriate config change, then tell me what you
> changed and how to reload the client. After I reconnect, verify the read-only
> `inspect_port` and `scan_dev_ports` tools. Show a dry-run preview before any
> process action, and ask me before using `dry_run=false`.

Reload or reconnect the AI client after saving its configuration. The server
provides `inspect_port`, `scan_dev_ports`, `free_port`, and
`clean_current_project`. Supply `project_path` and optional `ports` to select a
project explicitly from a global MCP server. Its preview returns the effective
path and scope.
Process actions default to dry-run. Review the target and get user confirmation
before terminating a process. MCP stdout is reserved for JSON-RPC.

Check `port-detective --version` when switching from a source build.

## Python SDK

Install the published Python SDK from PyPI:

```bash
python -m pip install port-detective==1.2.0
```

The SDK first checks `PORT_DETECTIVE_BIN`, `PATH`, and local checkout binaries. If no
CLI is found, the first API call downloads the matching GitHub Release archive,
verifies its SHA-256 checksum, and caches it under the user cache directory. This
needs network access once. For offline use, select a local CLI with
`PORT_DETECTIVE_BIN`. SDK `1.2.x` accepts
CLI `1.2.x`; a `1.1.x` CLI must be upgraded or replaced. Set
`PORT_DETECTIVE_AUTO_DOWNLOAD=0` for offline use and
provide a CLI with `PORT_DETECTIVE_BIN`, or set `PORT_DETECTIVE_CACHE_DIR` to choose
the cache location.

```python
from port_detective import inspect_port, scan_ports

diagnostic = inspect_port(3000)
occupied = scan_ports([3000, 8080])
```

See the [Python SDK guide](python/README.md) for synchronous and asyncio APIs.

## Node.js Client

The npm package provides synchronous, Promise, and TypeScript APIs and installs the
matching CLI binary for Windows, macOS, and Linux on x64 or arm64:

```bash
npm install port-detective@1.2.0
npx --no-install port-detective check 3000
```

The published npm package includes the matching CLI binary, so Go and a local CLI build
are unnecessary on supported platforms. Source checkouts require a built CLI or
`PORT_DETECTIVE_BIN`. SDK `1.2.x` accepts CLI `1.2.x`.

Process actions default to dry-run;
set `dryRun: false` only after reviewing the target. See the [Node.js client guide](npm/port-detective/README.md).

## Safety and Platform Notes

- Cleanup matches the detected process project path; it does not determine
  whether a process is orphaned.
- Review `--dry-run` output before using `--yes` or `dry_run=false`.
- Process metadata and termination can be limited by permissions, services,
  containers, WSL, and sandboxing.
- The CLI release matrix targets Windows, macOS, and Ubuntu on amd64 and arm64.
  Python SDK CI covers Ubuntu, macOS, and Windows; see the [platform policy](docs/operations/PLATFORM_SUPPORT.md).

## Documentation

- [Documentation index](docs/README.md)
- [Node.js client guide](npm/port-detective/README.md)
- [Contributing](docs/community/CONTRIBUTING.md)
- [Community submissions](docs/community/COMMUNITY_SUBMISSIONS.md)
- [Security policy](docs/community/SECURITY.md)
- [Project status](docs/operations/STATUS.md)
- [Quality review](docs/project/QUALITY_REVIEW.md)
- [Roadmap](docs/project/ROADMAP.md)
- [CLI and MCP API reference](docs/project/API_REFERENCE.md)
- [Troubleshooting](docs/operations/TROUBLESHOOTING.md) · [Shell completion](docs/operations/SHELL_COMPLETION.md)
- [Safe CLI demo](scripts/demo/README.md)
- [Changelog](docs/releases/CHANGELOG.md)
- [Release checklist](docs/releases/RELEASE_CHECKLIST.md)

## License

MIT. See [LICENSE](LICENSE).
