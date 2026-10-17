# Project Status

`port-detective` is at the v1.1 feature-complete development milestone. The repository
has no public `v1.1.0` tag or GitHub Release yet. The seven npm packages currently
expose only the `1.1.0-bootstrap.0` staging version; the matching PyPI project has not
been published.

Current capabilities:

- Concurrent TCP port inspection with process health diagnostics.
- Node.js, Python, and Go project fingerprinting.
- Dry-run-aware cleanup through the CLI and MCP tools.
- MCP Stdio server, Python 3.9+ SDK, and minimal-dependency Node.js 22+ client with platform-package staging.
- Bash, Zsh, Fish, and PowerShell completion.
- A safe listener-based CLI demo that never terminates an external process.
- Focused CLI modules, generated API inventory checks, and cross-platform CI.

Go, Python, and Node.js CI cover Windows, macOS, and Linux. The Node.js package and
platform binary workflow are ready for the first formal npm release. Process metadata can be
unavailable for protected or system-owned processes; the CLI reports that limitation
instead of treating it as proof that a port is free.

See [platform support](PLATFORM_SUPPORT.md), [contributing](../community/CONTRIBUTING.md),
[security](../community/SECURITY.md), [quality review](../project/QUALITY_REVIEW.md), and
the [roadmap](../project/ROADMAP.md) for current constraints and next steps.
