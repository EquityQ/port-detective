# Roadmap

This roadmap describes the work required to move `port-detective` from a strong
feature-complete development line to a dependable reference-quality open-source
project.

## Now: Release Readiness

- Publish the first signed `v1.1.0` CLI and matching Python SDK release.
- Attach checksums, SBOM, provenance, and acceptance-test evidence to the release.
- Keep the CLI and SDK version numbers synchronized.
- Publish a short release note with supported platforms and known permission limits.

## Next: Confidence Across Environments

- Expand Python CI to Windows and macOS.
- Add integration fixtures for protected processes, PID reuse, IPv4/IPv6, WSL, and
  containers.
- Add automated dependency vulnerability checks and scheduled test runs.
- Generate CLI and MCP reference tables from tests or machine-readable schemas.

## Later: Community Scale

- Establish a maintainer roster, response targets, and a predictable release cadence.
- Add a troubleshooting guide, shell completion walkthroughs, and a reproducible demo.
- Track benchmarks and compatibility reports by operating system, architecture, and
  permission level.
- Split oversized implementation files while preserving the documented contracts.

Priorities may change as compatibility reports arrive. Security fixes and data-loss
prevention always take precedence over feature work.
