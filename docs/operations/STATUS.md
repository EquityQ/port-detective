# Project Status

`port-detective` is at the v1.1 feature-complete development milestone. The latest Git
tag is `v0.1.0-assignment`; no signed or published `v1.1.0` release exists.

Current capabilities:

- Concurrent TCP port inspection with process health diagnostics.
- Node.js, Python, and Go project fingerprinting.
- Dry-run-aware cleanup through the CLI and MCP tools.
- MCP Stdio server, Python 3.9+ SDK, and zero-dependency Node.js 22+ client.
- Bash, Zsh, Fish, and PowerShell completion.
- A safe listener-based CLI demo that never terminates an external process.
- Focused CLI modules, generated API inventory checks, and cross-platform CI.

Go, Python, and Node.js CI cover Windows, macOS, and Linux. The Node.js package is
available from the checkout but is not published to npm. Process metadata can be
unavailable for protected or system-owned processes; the CLI reports that limitation
instead of treating it as proof that a port is free.

See [platform support](PLATFORM_SUPPORT.md), [contributing](../community/CONTRIBUTING.md),
[security](../community/SECURITY.md), [quality review](../project/QUALITY_REVIEW.md), and
the [roadmap](../project/ROADMAP.md) for current constraints and next steps.
