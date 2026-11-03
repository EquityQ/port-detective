# Changelog

All notable changes follow [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Preview-bound destructive actions. `free_port` and `clean_current_project` now return a
  `preview_id` from a dry run; executing requires that token, so an action can only target
  the process snapshot that was actually previewed. The CLI exposes the same contract through
  `kill --json --dry-run` and `--preview-id` on `kill` and `clean`. The token is HMAC-signed
  with a per-process key and expires quickly, so a token from another process or an earlier
  server run is rejected. This closes a window where a preview and a later execution were
  two independent probes that could observe different owners of the same port.
- `doctor` exits with status `5` when its self-check is degraded, so automation no longer
  reads a failed self-check as success.
- `listener_table_missing` and the `undetermined` status make an unreadable listener table
  explicit. A degraded inspection or scan no longer presents a closed loopback probe as
  proof that a port is free.

### Changed

- `kill --json` and `free_port` name their snapshot `pre_action_diagnostic` and report the
  verified outcome separately as `post_action_status`. The legacy `diagnostic` key remains
  as a deprecated alias with the same value.
- `doctor` no longer prints an unqualified success conclusion when hints were recorded.

### Fixed

- An explicitly empty port selection is rejected instead of widening scope into the default
  development ports or every project listener. This applies to the CLI, MCP tools, and the
  Python and Node.js SDKs (`ports=[]` raises; omitting the argument keeps the default).
- Project path comparison uses filesystem identity before string comparison, so a
  case-sensitive directory on a case-insensitive volume is matched correctly.
- Fingerprint detection now bounds manifest reads: cancellation is checked between chunks,
  and manifests above a size cap are rejected rather than read, so a slow or oversized
  manifest can no longer outrun the detection budget.
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
