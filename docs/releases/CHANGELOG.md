# Changelog

All notable changes follow [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

No additional changes are recorded after v1.3.0.

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
