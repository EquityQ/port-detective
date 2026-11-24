# Project Status

`port-detective` v1.4.0 was released on 2026-11-24 through GitHub, PyPI, and npm
Trusted Publishing. The release includes signed Go archives and Python distributions,
six platform npm packages, an SPDX SBOM, checksums, and GitHub provenance attestations.

Capabilities in v1.4.0:

- Concurrent TCP and UDP port inspection with process health diagnostics and local
  `bind_addresses` visibility.
- Multi-language project fingerprinting for Node.js, Python, Go, Rust, Java/Kotlin,
  Ruby, and PHP, plus explicit dependency injection via `core.Inspector`.
- Destructive actions bound to a previewed process snapshot: dry runs return a signed,
  short-lived `preview_id`, CLI persists a user-scoped 24h-rotated signing key across
  invocations (`PORT_DETECTIVE_PREVIEW_KEY_FILE`), and MCP keeps strict per-process
  memory isolation.
- Dry-run-aware cleanup through the CLI and MCP tools with fail-closed empty selection.
- MCP Stdio server (`inspect_port`, `scan_dev_ports`, `free_port`, `clean_current_project`
  with optional `protocol` on read-only tools), Python 3.9+ SDK (with PEP 561 `py.typed`),
  and minimal-dependency Node.js 22+ client with full workflow parity (`wait`, `find-free`,
  `doctor`, `protocol`, `preset`, `preview_id`).
- Python SDK first-use CLI bootstrap with SHA-256 verification and a user-cache opt-out.
- Bash, Zsh, Fish, and PowerShell completion and a safe listener-based CLI demo.
- Modularized Go, Python, Node.js, and release-script packages with cross-language
  schema parity, `mypy`, `tsc`, and raised coverage gates.

Version 1.4.0 builds on v1.3.0's safety hardening with zero breaking API changes. SDK
1.4.x requires CLI 1.4.x so callers using new flags and JSON fields (`--protocol udp`,
`bind_addresses`, `wait`, `find-free`, `doctor`, `--preview-id`) fail fast rather than
hitting an older binary. All capabilities are live in the published GitHub Release, PyPI,
and npm packages.

Local validation, hosted CI, release signing, and registry clean-install checks passed
for v1.4.0. The [release workflow](https://github.com/EquityQ/port-detective/actions/runs/45324780732)
ran Go/Python/MCP gates and verified checksums, the strict six-platform manifest, Cosign
signatures, SBOM, and provenance before publishing. The
[npm publication workflow](https://github.com/EquityQ/port-detective/actions/runs/45325148398)
published the client and all six platform packages and passed clean-install integration.

Go, Python, and Node.js CI cover Windows, macOS, and Linux. Process metadata can be
unavailable for protected or system-owned processes; the CLI reports that limitation
instead of treating it as proof that a port is free.

See [platform support](PLATFORM_SUPPORT.md), [contributing](../community/CONTRIBUTING.md),
[security](../community/SECURITY.md), [quality review](../project/QUALITY_REVIEW.md), and
the [roadmap](../project/ROADMAP.md) for current constraints and next steps.
