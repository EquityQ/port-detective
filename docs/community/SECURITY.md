# Security Policy

## Supported Versions

Security fixes target the latest published release and the active development line.
Upgrade before reporting an issue against an older release.

| Version | Security fixes |
| --- | --- |
| `v1.2.0` | Supported |
| Active development line | Supported |
| `v0.1.0-assignment` and older | Best effort |

## Reporting a Vulnerability

Use a [private GitHub Security Advisory](https://github.com/EquityQ/port-detective/security/advisories/new).
Do not open a public issue or include exploit details in a pull request. Include the
affected version, OS and architecture, installation method, reproduction steps, and
impact. Remove credentials, tokens, and private paths.

The repository publishes signed `v1.2.0` artifacts. Verify checksums, signatures, and
provenance before using a release in deployed automation.

## Safety

The tool can inspect process metadata and terminate a process after explicit user
confirmation. Test termination only against disposable local processes.
