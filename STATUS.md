# Project Status

`port-detective` is currently at the v1.1 feature-complete milestone. The latest
release line includes:

- Concurrent TCP port inspection with process health diagnostics.
- Project and framework fingerprinting for Node.js, Python, and Go services.
- Dry-run-aware process cleanup through the CLI and MCP tools.
- A bilingual CLI, MCP Stdio server, and Python 3.9+ SDK.
- Cross-platform release archives for Windows, macOS, and Linux on `amd64` and
  `arm64`.

The project is in active maintenance. Current priorities are release automation,
documentation quality, compatibility reports, and careful security fixes. Process
metadata can still be unavailable for protected or system-owned processes; this is
reported as a permission-limited diagnostic rather than treated as proof that a
port is free.

See [Platform Support](PLATFORM_SUPPORT.md), [Contributing](CONTRIBUTING.md), and
[Security Policy](SECURITY.md) before opening a report.
