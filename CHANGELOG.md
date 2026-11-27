# Changelog

All notable changes follow [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Self-update command `port-detective update` (`internal/updater` pure Go stdlib package)
  supporting `--check`, `--dry-run`, `--yes`, `--force`, `--to-version`, and `--timeout`,
  with SHA-256 verification against `checksums.txt`, safe archive extraction, staged binary
  verification (`--version`), npm/Python-SDK managed-install detection, and cross-platform
  atomic executable replacement.

## [1.5.0] - 2026-11-27

This minor release closes the structured cleanup preview contract (`clean --json --envelope`
and SDK `CleanResult` helpers), extends termination and preview verification to optional UDP
ports, adds read-only workflow tools to the MCP server (`find_free_port`, `wait_for_port`,
`doctor`, and `scan_dev_ports` presets), expands monorepo/runtime fingerprinting (Deno, Bun,
.NET, and 8-level traversal), completes `core.Inspector` dependency injection across CLI and
MCP, and migrates release signing to Cosign v3 Sigstore bundles. All changes are backward
compatible with v1.4.x APIs; SDK `1.5.x` requires CLI `1.5.x` to protect the new flags,
options, and JSON fields.

### Added

- Structured cleanup envelope support: CLI `clean --json --envelope` (`CleanResult` with
  `cwd`, `scope`, `dry_run`, `count`, `cleaned`, `preview_id`, `preview_expires_at_unix`,
  `message`, `error`), plus `clean_project_result` / `async_clean_project_result` in Python
  and `cleanProjectResult` / `asyncCleanProjectResult` in Node.js (`CleanResult` model/type).
  Plain `clean --json` and `clean_project` / `cleanProject` preserve their array responses.
- Optional UDP termination (`--protocol udp`) on CLI `kill` and `protocol` (`"tcp"` / `"udp"`)
  on MCP `free_port`, Python `free_port` / `async_free_port`, and Node.js `freePort` /
  `asyncFreePort`, backed by protocol-bound preview token verification (`RestorePreviewTokenProtocol`,
  `RestoreCLIPreviewTokenProtocol`) and bounded UDP release polling (`KillPortProcessProtocol`).
- Expanded MCP workflow tools: added read-only `find_free_port`, `wait_for_port`, and
  `doctor` tools plus `preset` and `project_path` filtering on `scan_dev_ports` (7 tools total).
- Expanded fingerprinting & monorepo support: increased upward manifest traversal to 8 levels
  within the 50ms budget, added Node.js framework detectors (Fastify, Hono, Elysia, Astro,
  SvelteKit), and added Deno (`deno.json`, `deno.jsonc`), Bun (`bunfig.toml`), and .NET
  (`*.csproj`) manifest and cmdline detectors.
- Sigstore bundle completeness verification (`--require-bundles`, `--sbom`) in
  `scripts/release/verify_artifacts.py` and MCP/`mypy`/`tsc`/`--strict` gates in
  `scripts/quality/run_checks.py`.

### Changed

- Extracted pure workflow helpers (`ResolvePresetPorts`, `ReadProjectPresetPorts`,
  `FindFreePort`, `CheckPortBindings`, `WaitForPort`, `WaitForPortProtocol`, `RunDoctor`)
  into `internal/core/workflows.go` and delegated CLI commands to them.
- Completed explicit dependency injection via `*core.Inspector` (`NewRootCommandWithInspector`,
  `ExecuteWithInspector`, `NewServerWithInspector`, `RunStdioServerWithInspector`), removing
  blank `_ ".../fingerprint"` imports across `main.go`, `internal/cli`, and `internal/mcpserver`.
- Migrated release signing and verification in `.github/workflows/release.yml` from detached
  `.sig`/`.pem` files to `.sigstore.json` Sigstore bundles (`--bundle` with
  `sigstore/cosign-installer` `v4.1.4` / Cosign `v3.1.5`), and removed the Dependabot
  major-update hold on `sigstore/cosign-installer`.
- Updated Go and GitHub Actions dependencies: `github.com/mark3labs/mcp-go` (`v1.6.0`),
  `actions/setup-python` (`v7.1.0`), `actions/setup-go` (`v7.1.0`),
  `pypa/gh-action-pypi-publish` (`v1.14.3`), `goreleaser/goreleaser-action` (`v7.3.0`),
  and `sigstore/cosign-installer` (`v4.1.4`).

### Fixed

- CLI `check`/`scan` UDP footers now suggest `--protocol udp` for follow-up release hints,
  Python `clean_project` passes an explicit `path` via `--project`, and `run_checks.py
  --skip-go --skip-python --skip-node` no longer requires a `go` binary on `PATH`.
- Translated residual non-English comment in `internal/core/termination.go`.

### Security

- Preview tokens bind destructive `kill` and `free_port` operations to both exact listener
  identity and normalized transport protocol (`tcp` vs `udp`).

## [1.4.0] - 2026-11-24

This minor release adds cross-process CLI preview token persistence, local bind-address
reporting, optional UDP inspection and concurrent scanning, expanded multi-language
project fingerprinting, explicit dependency injection (`core.Inspector`), full Python
and Node.js SDK workflow parity, and modularized internals with stricter CI gates. All
changes are backward compatible with v1.3.x APIs; SDK `1.4.x` requires CLI `1.4.x` to
protect the new flags and JSON fields.

### Added

- Cross-process CLI preview signing keystore (`PORT_DETECTIVE_PREVIEW_KEY_FILE`, defaulting
  to a `0600` user-cache key rotated every 24h) so `kill --dry-run` and `clean --dry-run`
  tokens can be redeemed across separate CLI invocations while MCP keeps strict per-process
  memory isolation.
- Local `bind_addresses` reporting on `PortDiagnostic` (Go, CLI `check`, MCP, Python SDK,
  and TypeScript definitions).
- Optional UDP inspection and concurrent scanning (`--protocol udp` on CLI `check`/`scan`,
  `protocol="udp"` on MCP `inspect_port`/`scan_dev_ports` and both SDKs, plus
  `core.InspectPortProtocol` / `core.ScanPortsProtocolSummary`), with honest `undetermined`
  degradation when the UDP socket table is unreadable.
- Explicit dependency injection via `core.Inspector` (`core.NewInspector`) without mutating
  global detector state.
- Multi-language project fingerprinting and root resolution for Rust (`Cargo.toml`: Axum,
  Actix Web, Rocket, Warp, Tonic), Java/Kotlin (`pom.xml`, `build.gradle`,
  `build.gradle.kts`: Spring Boot, Quarkus, Micronaut, Ktor, Vert.x), Ruby (`Gemfile`:
  Rails, Sinatra, Hanami, Puma), and PHP (`composer.json`: Laravel, Symfony, Slim,
  WordPress), plus `core.SupportedProjectManifests()`.
- Full Python SDK and Node.js client workflow parity: `wait_for_port` / `waitForPort`,
  `find_free_port` / `findFreePort`, `doctor` / `async_doctor` / `asyncDoctor` (parsing
  degraded exit-5 JSON reports cleanly), `protocol`, `preset`, `preview_id` / `previewId`,
  `FreePortResult`, `DoctorReport`, and PEP 561 `py.typed` packaging for Python.
- Unified cross-platform local verification runner (`scripts/quality/run_checks.py`),
  `scripts/README.md`, cross-language `PortDiagnostic` / TypeScript export drift checks in
  `scripts/quality/verify_api_reference.py`, Python `mypy` CI check, and TypeScript
  `tsc --noEmit` CI check.

### Changed

- Relocated the shared SDK test fixture from `npm/port-detective/test/fixtures/cli.go` to
  `testdata/sdk-cli/main.go`, decoupling Python tests/CI from `npm/` and keeping it out of
  `go test ./...`.
- Modularized `internal/mcpserver` (`server.go`, `tools.go`, `handlers.go`, `args.go`),
  `internal/fingerprint` (`analyzer.go`, `manifest.go`, `detectors.go`), `internal/cli`
  (`kill.go`, `clean.go`, `confirm.go`, `terminal.go`, `output.go`), `npm/port-detective`
  (`lib/{errors,validation,runner}.js`, shared `platform.resolveBundledBinary()`), Python
  SDK (`errors.py`, shared command/parsing helpers), and release scripts
  (`scripts/release/release_common.py`).
- Raised CI coverage thresholds to Go 75/65/75/75%, Python 70/90/70%, and Node.js
  75/70/80%, and enabled `--require-platform-manifest` in `npm-publish.yml`.

### Fixed

- CLI `clean --preview-id` and `kill --preview-id` now validate tokens before live discovery
  or probing, preventing forged tokens from returning `[]` on empty scopes or double-prompting
  in interactive mode.
- CLI `wait --state free` no longer treats an `undetermined` / `listener_table_missing`
  diagnostic as proof that a port is free.
- Protected core test hooks (`listTCPConnections`, `previewKey`) with `sync.RWMutex` and
  restored `fingerprint.DetectProject` instead of `nil` in CLI/MCP preview tests.
- Fixed stale `SDK 1.2.x` compatibility strings in `README.md`.

## [1.3.0] - 2026-11-03

This release hardens destructive operations, makes degraded results honest, and closes
release-gate gaps. Two changes are breaking; they turn silently unsafe behavior into
explicit failures, so they are a minor bump rather than a patch.

### Breaking

- **MCP `free_port` and `clean_current_project` require a preview before executing.**
  Calling either tool with `dry_run=false` without the `preview_id` returned by a prior
  dry run now returns a tool error. A dry run and a later execution used to be two
  independent probes, so a port whose owner changed in between could be terminated on a
  process the caller never saw. Automation that previously executed directly must now
  preview first and pass the token back.
- **The Python and Node.js SDKs reject an explicitly empty port selection.**
  `scan_ports(ports=[])`, `clean_project(ports=[])`, `scanPorts([])`, and
  `cleanProject({ports: []})` now raise `ValueError` / `TypeError` instead of silently
  falling back to the default ports or every project listener. Omit the argument (or pass
  `None` / `undefined`) to keep the documented default.

The CLI is not affected by the first breaking change: `kill --json --yes` still works
without a token, and `--preview-id` is an optional hardening path.

### Added

- Signed, short-lived preview tokens that pin the port, PID, and process creation time of
  the snapshot a caller previewed. Tokens are HMAC-signed with a per-process key, scoped
  (a kill preview cannot authorize a project cleanup), bound to an exact port selection,
  and rejected when expired, tampered with, or issued by another process. Execution
  restores the pinned snapshot through the existing revalidation path, so live drift fails
  the action instead of redirecting it to a new owner.
- `--preview-id` for `kill` and `clean`, and `preview_id` in the JSON preview payload.
- `preview_id` on MCP dry runs, required by `dry_run=false`.
- `listener_table_missing` and the `undetermined` status, so an unreadable listener table
  is never presented as proof that a port is free. Degraded scans report an incomplete
  result instead of implying the unscanned world is empty.
- `doctor` exits with status `5` when its self-check is degraded.

### Changed

- `kill --json` and `free_port` name their snapshot `pre_action_diagnostic` and report the
  verified outcome separately as `post_action_status`. The legacy `diagnostic` key remains
  as a deprecated alias with the same value.
- `doctor` no longer prints an unqualified success conclusion when hints were recorded.

### Fixed

- An explicitly empty port selection is rejected instead of widening scope, across the CLI,
  MCP tools, and both SDKs.
- Project path comparison uses filesystem identity before string comparison, so a
  case-sensitive directory on a case-insensitive volume is matched correctly.
- Fingerprint detection bounds manifest reads: cancellation is checked between chunks and
  manifests above a size cap are rejected rather than read, so a slow or oversized manifest
  can no longer outrun the detection budget.
- The release verifier requires the exact six-platform archive manifest, a checksum entry
  for every archive, and the platform executable inside each archive.
- `TestKillDryRunCLI` uses a dynamically reserved port instead of a hardcoded one.
- Removed the CLI's "clean orphan dev processes" claim, which it cannot verify.

## [1.2.0] - 2026-10-22

Released 2026-10-22 through GitHub, PyPI, and npm Trusted Publishing.

### Added

- Port list ranges and deduplication for CLI and MCP scans.
- Explicit web, Python, database, and opt-in project scan presets.
- `clean --project` / `--ports` and MCP `project_path` / `ports` selection.
- `wait`, `check --watch`, `find-free`, and `doctor` workflows.
- SDK compatibility checks for external CLI binaries and Python install/doctor entry points.

### Fixed

- Interface-specific TCP listeners are included in inspection and scans.
- Project cleanup discovers listeners beyond the ten default development ports.
- Cleanup shows its selected targets before confirmation and retains their process identities.
- Interactive kill/clean confirmations exit promptly when interrupted.
- MCP acceptance waits for live replies and verifies successful tool results.
- Quick-start commands work with current-directory executables and explain actual safety defaults.

### Changed

- CLI, Python, and npm release metadata target 1.2.0; SDK 1.2.x requires a CLI
  with the same major and minor version.

The matching CLI and SDK packages are available from the linked release and registries.

## [1.1.1] - 2026-10-17

The patch release improves installation without a separately installed CLI and adds a
direct npm command entry point.

### Added

- First-use Python CLI bootstrap with SHA-256 verification and versioned user caching.
- `npx --no-install port-detective ...` support for the published npm package.

### Fixed

- Python runtime cache validation now rejects tampered or empty cached binaries.

## [1.1.0] - 2026-10-17

The first public release includes the CLI, MCP server, Python SDK, Node.js client, and
platform-specific npm CLI packages. GitHub, PyPI, and npm publication completed through
Trusted Publishing after checksum, signature, SBOM, provenance, and integration checks.

### Security

- Release archives, Python distributions, checksums, and SBOM are signed with keyless Cosign.
- GitHub build provenance attestations are attached to release artifacts.

### Added

- Security, conduct, contribution, platform, status, issue, and pull request guidance.
- Release verification, SPDX SBOM, Cosign signatures, provenance, and PyPI Trusted Publishing workflow.
- Atomic publication that verifies and signs artifacts before creating a GitHub Release.
- Immutable GitHub Action references and Dependabot configuration.
- Dependency-free Node.js client with sync/Promise APIs and TypeScript declarations.
- Reviewed coverage thresholds for Go, Python, and Node.js packages.
- CLI and MCP API reference with a CI drift checker.
- IPv6, Unicode-path, and protected-PID regression coverage.
- PID-reuse and restricted-environment diagnostic checks with safe capability skips.
- Community guidance for compatibility, benchmark, and maintenance submissions.
- Platform-specific npm CLI packages with automatic binary resolution and provenance publishing workflow.
- Bash, Zsh, Fish, and PowerShell completion plus a safe listener-based demo.

### Changed

- Public documentation is concise and English-only.
- CLI, MCP, Python, and Node.js examples are checked against the current implementation.
- Generated API inventories are produced in CI instead of committed as derived files.

## [0.1.0-assignment]

The original course baseline. See the repository history for the detailed implementation record.
