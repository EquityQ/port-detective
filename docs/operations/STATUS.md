# Project Status

`port-detective` v1.1.1 is publicly released on GitHub, PyPI, and npm. The release
contains signed Go archives, Python distributions, six platform npm packages, an SPDX
SBOM, checksums, and GitHub provenance attestations.

Current capabilities:

- Concurrent TCP port inspection with process health diagnostics.
- Node.js, Python, and Go project fingerprinting.
- Dry-run-aware cleanup through the CLI and MCP tools.
- MCP Stdio server, Python 3.9+ SDK, and minimal-dependency Node.js 22+ client with platform-package staging.
- Python SDK first-use CLI bootstrap with SHA-256 verification and a user-cache opt-out.
- Bash, Zsh, Fish, and PowerShell completion.
- A safe listener-based CLI demo that never terminates an external process.
- Focused CLI modules, generated API inventory checks, and cross-platform CI.

Unreleased source changes after v1.1.1 add interface-aware listener inspection,
all-listener project cleanup with an explicit project/port selection and target
preview, MCP `project_path`, deduplicated port ranges, `wait`, `check --watch`,
explicit scan presets, `find-free`, and `doctor`. SDK updates validate external CLI compatibility and
provide Python install/doctor entry points. These changes require a local source
build until a new release is published.

Go, Python, and Node.js CI cover Windows, macOS, and Linux. The Node.js package and
platform binary workflow are published with the formal npm release. Process metadata can be
unavailable for protected or system-owned processes; the CLI reports that limitation
instead of treating it as proof that a port is free.

See [platform support](PLATFORM_SUPPORT.md), [contributing](../community/CONTRIBUTING.md),
[security](../community/SECURITY.md), [quality review](../project/QUALITY_REVIEW.md), and
the [roadmap](../project/ROADMAP.md) for current constraints and next steps.
