# Changelog

All notable changes follow [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

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

These changes are available from source and have not been published as a new release.

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
