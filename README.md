# port-detective

Cross-platform CLI, MCP server, and Python SDK for inspecting local TCP ports,
identifying owning processes, and cleaning project-scoped listeners safely.

> **Release status:** The repository is on the v1.1 development line. No v1.1.0
> binary, PyPI package, or npm package has been published yet. Use a reviewed source
> commit until a release is available.

## Quick Start

Requirements: Go 1.23+ for the CLI, Python 3.9+ for the Python SDK, and Node.js 22+
for the Node.js client.

```bash
git clone https://github.com/EquityQ/port-detective.git
cd port-detective
go build -o port-detective .
```

```bash
port-detective check 3000
port-detective scan --ports 3000,5173,8080
port-detective kill 3000 --dry-run
port-detective clean --dry-run
```

Use `--json` with `check`, `scan`, `kill`, or `clean` for machine-readable output.
Destructive JSON commands require an explicit `--yes` flag:

```bash
port-detective kill 3000 --json --yes
port-detective clean --json --yes
```

Run the test suite with:

```bash
go test -race ./...
```

## MCP

Start the Stdio server:

```bash
port-detective mcp
```

Example client configuration:

```json
{
  "mcpServers": {
    "port-detective": {
      "command": "port-detective",
      "args": ["mcp"]
    }
  }
}
```

The server exposes `inspect_port`, `scan_dev_ports`, `free_port`, and
`clean_current_project`. Read-only tools return diagnostics; destructive tools use
`dry_run=true` by default. MCP stdout is reserved for JSON-RPC.

## Python SDK

The SDK wraps the CLI and does not bundle a Go binary:

```bash
# After the v1.1.0 release is published:
python -m pip install port-detective

# Or install the SDK from this checkout:
python -m pip install ./python
```

Keep the SDK and CLI on the same reviewed commit or release. Set
`PORT_DETECTIVE_BIN` when the executable is not on `PATH`. The tag workflow uses
PyPI Trusted Publishing and verifies the installed package before creating the
matching GitHub Release. The v1.1.0 package is not published yet.

```python
from port_detective import inspect_port, scan_ports

diagnostic = inspect_port(3000)
occupied = scan_ports([3000, 8080])
```

See the [Python SDK guide](python/README.md) for synchronous and asyncio APIs.

## Node.js Client

The npm package provides synchronous, Promise, and TypeScript APIs. Published releases
install the matching CLI binary for Windows, macOS, and Linux on x64 or arm64:

```bash
npm install port-detective
```

Source checkouts do not contain generated platform packages, so local development still
requires a built CLI on `PATH` or `PORT_DETECTIVE_BIN`. Process actions default to dry-run;
set `dryRun: false` only after reviewing the target. The package is not published to npm
yet. See the [Node.js client guide](npm/port-detective/README.md).

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
