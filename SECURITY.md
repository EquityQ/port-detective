# Security Policy

## Supported Versions

Security fixes are applied to the latest published release and the active development
line. Please upgrade before reporting an issue against an older release.

| Version | Security fixes |
| --- | --- |
| `v1.1.x` / active development line | Supported |
| `v0.1.0-assignment` and older | Best effort only |

## Reporting a Vulnerability

Please use a [private GitHub Security Advisory](https://github.com/EquityQ/port-detective/security/advisories/new)
to report a vulnerability. Do not open a public issue or include exploit details
in a pull request.

Include the affected version, operating system and architecture, installation
method, reproduction steps, expected impact, and any proof of concept that can be
shared safely. Remove credentials, tokens, private paths, and other sensitive data
before submitting.

The maintainers will acknowledge a report within 5 business days and will use the
advisory to coordinate validation, a fix, and public disclosure. Response times are
best effort for this community-maintained project.

The repository does not currently publish a signed v1.1.0 artifact. For deployed
automation, use a reviewed commit or a future release with verified checksums and
provenance.

## Scope and Safety Notes

`port-detective` can inspect process metadata and can terminate a process when the
user explicitly requests it. Test termination behavior only against a disposable
local process. Never include production credentials or host data in a report.
