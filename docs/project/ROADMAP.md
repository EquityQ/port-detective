# Roadmap

This roadmap tracks the remaining work needed for a dependable public release.

## Release readiness

- Create the signed `v1.1.0` CLI and matching Python SDK only after all release gates pass.
- Retain checksums, SBOM, provenance, signatures, and MCP acceptance evidence.
- Keep CLI and SDK versions synchronized.
- Publish supported platforms and known permission limits in the release notes.

## Cross-environment confidence

- Keep the real CLI/Python integration matrix required on Windows, macOS, and Linux.
- Keep deterministic protected-process and PID-reuse guards; run the opt-in restricted-environment
  diagnostic when WSL, container, or sandbox evidence is available.
- Keep IPv4/IPv6 loopback, Unicode path, and PID 0/1 guard coverage.
- Keep `govulncheck`, `pip-audit`, and API drift checks green.

## Packaging and quality

- Configure the PyPI Trusted Publisher before the first release.
- Decide whether to configure npm Trusted Publishing and publish the matching Node.js client.
- Raise coverage thresholds only with targeted regression tests.

## Later

- Track compatibility and benchmark evidence by OS, architecture, and permission level.
- Establish named project ownership and review support expectations against actual activity.

Priorities may change as evidence arrives. Security fixes and data-loss prevention take
precedence over feature work.
