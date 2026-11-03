# Project Status

`port-detective` v1.3.0 was released on 2026-11-03 through GitHub, PyPI, and npm
Trusted Publishing. The release includes signed Go archives and Python distributions,
six platform npm packages, an SPDX SBOM, checksums, and GitHub provenance attestations.

Capabilities in v1.3.0:

- Concurrent TCP port inspection with process health diagnostics.
- Node.js, Python, and Go project fingerprinting.
- Destructive actions bound to a previewed process snapshot: a dry run returns a signed,
  short-lived `preview_id`, and executing requires it, so a port whose owner changed in
  between can never terminate a process the caller did not see.
- Dry-run-aware cleanup through the CLI and MCP tools.
- MCP Stdio server, Python 3.9+ SDK, and minimal-dependency Node.js 22+ client with platform-package staging.
- Python SDK first-use CLI bootstrap with SHA-256 verification and a user-cache opt-out.
- Bash, Zsh, Fish, and PowerShell completion.
- A safe listener-based CLI demo that never terminates an external process.
- Focused CLI modules, generated API inventory checks, and cross-platform CI.

Version 1.3.0 hardens destructive operations and reporting. MCP `free_port` and
`clean_current_project` require a preview token before executing, and both SDKs reject an
explicitly empty port selection instead of widening it to the default ports or every
project listener; both changes are documented as breaking. A listener table that cannot be
read is now reported as `listener_table_missing` with an `undetermined` status rather than
as a free port, and degraded scans report an incomplete result. `doctor` exits non-zero
when its self-check is degraded. SDK 1.3.x requires CLI 1.3.x. These capabilities are
available in the published CLI and SDK packages.

Local validation, hosted CI, release signing, and registry clean-install checks
passed for v1.3.0. The release workflow ran Go/Python/MCP gates and verified
checksums, Cosign signatures, SBOM, and provenance before publishing. PyPI and npm
clean-install integrations passed.

Go, Python, and Node.js CI cover Windows, macOS, and Linux. The Node.js package and
platform binary workflow are published with the formal npm release. Process metadata can be
unavailable for protected or system-owned processes; the CLI reports that limitation
instead of treating it as proof that a port is free.

See [platform support](PLATFORM_SUPPORT.md), [contributing](../community/CONTRIBUTING.md),
[security](../community/SECURITY.md), [quality review](../project/QUALITY_REVIEW.md), and
the [roadmap](../project/ROADMAP.md) for current constraints and next steps.
