# Project Status

`port-detective` v1.5.0 was released on 2026-11-27 through GitHub, PyPI, and npm
Trusted Publishing. The release includes signed Go archives and Python distributions,
six platform npm packages, an SPDX SBOM, checksums, `.sigstore.json` Sigstore bundles,
and GitHub provenance attestations.

Capabilities in v1.5.0:

- Concurrent TCP and UDP port inspection with process health diagnostics, local
  `bind_addresses` visibility, and pure core workflow helpers (`ResolvePresetPorts`,
  `FindFreePort`, `WaitForPortProtocol`, `RunDoctor`).
- Multi-language monorepo project fingerprinting (8-level ancestor traversal) for
  Node.js (including Fastify, Hono, Elysia, Astro, SvelteKit), Python, Go, Rust,
  Java/Kotlin, Ruby, PHP, Deno (`deno.json`/`deno.jsonc`), Bun (`bunfig.toml`), and .NET
  (`*.csproj`), plus complete `core.Inspector` dependency injection across core, CLI,
  and MCP.
- Destructive actions bound to a previewed process snapshot and normalized protocol:
  `kill --protocol tcp|udp`, MCP `free_port` `protocol`, CLI `clean --json --envelope`
  (`CleanResult`), Python `clean_project_result` / `async_clean_project_result`, and
  Node.js `cleanProjectResult` / `asyncCleanProjectResult`. CLI persists a user-scoped
  24h-rotated signing key across invocations (`PORT_DETECTIVE_PREVIEW_KEY_FILE`), while
  MCP keeps strict per-process memory isolation.
- Dry-run-aware cleanup through CLI, MCP, Python, and Node.js with fail-closed empty
  selection.
- MCP Stdio server exposing seven tools (`inspect_port`, `scan_dev_ports` with
  `preset`/`project_path`, `free_port`, `clean_current_project`, `find_free_port`,
  `wait_for_port`, `doctor`), Python 3.9+ SDK (with PEP 561 `py.typed`), and
  minimal-dependency Node.js 22+ client with full CLI workflow parity.
- Python SDK first-use CLI bootstrap with SHA-256 verification and a user-cache opt-out.
- Bash, Zsh, Fish, and PowerShell completion and a safe listener-based CLI demo.
- Modularized Go, Python, Node.js, and release-script packages with cross-language
  schema parity, `mypy`, `tsc`, raised coverage gates, unified `run_checks.py`, and
  Sigstore bundle completeness verification (`--require-bundles --sbom`).

Version 1.5.0 builds on v1.4.0 with zero breaking API changes. SDK 1.5.x requires CLI
1.5.x so callers using new flags, options, and JSON fields (`clean --envelope`,
`kill --protocol udp`, MCP `find_free_port`/`wait_for_port`/`doctor`, Deno/Bun/.NET
discovery) fail fast rather than hitting an older binary. All capabilities are live in the
published GitHub Release, PyPI, and npm packages.

Local validation, hosted CI, release signing, and registry clean-install checks passed
for v1.5.0. The [release workflow](https://github.com/EquityQ/port-detective/actions/runs/45951344497)
ran Go/Python/MCP gates and verified checksums, the strict six-platform manifest, Cosign
Sigstore bundles (`cosign-installer` v4.1.4 / Cosign v3.1.5), SBOM, and provenance before
publishing. The
[npm publication workflow](https://github.com/EquityQ/port-detective/actions/runs/45951693084)
published the client and all six platform packages and passed clean-install integration.

Go, Python, and Node.js CI cover Windows, macOS, and Linux. Process metadata can be
unavailable for protected or system-owned processes; the CLI reports that limitation
instead of treating it as proof that a port is free.

See [platform support](PLATFORM_SUPPORT.md), [contributing](../community/CONTRIBUTING.md),
[security](../community/SECURITY.md), [quality review](../project/QUALITY_REVIEW.md), and
the [roadmap](../project/ROADMAP.md) for current constraints and next steps.
