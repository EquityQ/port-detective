# Changelog

All notable changes follow [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

The repository contains the v1.1 feature-complete development line. A `v1.1.0`
release is planned after the release gates in [QUALITY_REVIEW.md](../project/QUALITY_REVIEW.md)
pass.

### Added

- Security, conduct, contribution, platform, status, issue, and pull request guidance.
- Release verification, SPDX SBOM, Cosign signatures, provenance, and PyPI Trusted Publishing workflow.
- Atomic publication that verifies and signs artifacts before creating a GitHub Release.
- Immutable GitHub Action references and Dependabot configuration.
- Dependency-free Node.js client with sync/Promise APIs and TypeScript declarations.
- Reviewed coverage thresholds for Go, Python, and Node.js packages.
- CLI and MCP API reference with a CI drift checker.
- IPv6, Unicode-path, and protected-PID regression coverage.
- Bash, Zsh, Fish, and PowerShell completion plus a safe listener-based demo.

### Changed

- Public documentation is concise and English-only.
- CLI, MCP, Python, and Node.js examples are checked against the current implementation.
- Generated API inventories are produced in CI instead of committed as derived files.

## [0.1.0-assignment]

The original course baseline. See the repository history for the detailed implementation record.
