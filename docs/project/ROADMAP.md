# Roadmap

This roadmap tracks post-release improvements for the next development cycle.

## Implemented in source, awaiting release

- Read all local TCP bindings and deduplicate scan inputs.
- Discover project listeners beyond the default development ports; support an
  explicit project, port ranges, and preview-before-confirmation cleanup.
- Select a project explicitly through MCP and report the effective path/scope.
- Add port waiting, continuous inspection, free-port suggestions, and doctor diagnostics.
- Offer web, Python, database, and opt-in project scan presets without changing default scans.
- Check external CLI versions in SDKs and provide Python install/doctor commands.
- Prefer a one-command npm quick start and document CLI versus SDK/MCP safety defaults.

Validate these changes against the required platform matrix before publishing
the next release. The published v1.1.1 packages do not contain them yet.

## Release readiness

- v1.1.1 release completed through GitHub, PyPI, and npm Trusted Publishing.
- Retain checksums, SBOM, provenance, signatures, and MCP acceptance evidence.
- Keep CLI and SDK versions synchronized.
- Keep supported platforms and known permission limits current in release notes.

## Cross-environment confidence

- Keep the real CLI/Python integration matrix required on Windows, macOS, and Linux.
- Keep deterministic protected-process and PID-reuse guards; run the opt-in restricted-environment
  diagnostic when WSL, container, or sandbox evidence is available.
- Keep IPv4/IPv6 loopback, Unicode path, and PID 0/1 guard coverage.
- Keep `govulncheck`, `pip-audit`, and API drift checks green.

## Packaging and quality

- Keep PyPI and npm Trusted Publishing configured and verify clean installs on future releases.
- Raise coverage thresholds only with targeted regression tests.

## Later

- Track compatibility and benchmark evidence by OS, architecture, and permission level.
- Establish named project ownership and review support expectations against actual activity.

Priorities may change as evidence arrives. Security fixes and data-loss prevention take
precedence over feature work.
