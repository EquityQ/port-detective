# Platform Support

`port-detective` is built with Go and uses `gopsutil` plus the standard library for
network and process inspection. Release archives target the following platforms:

| Platform | Architectures | Support level |
| --- | --- | --- |
| Windows 10/11 | `amd64`, `arm64` | Tier 1 |
| macOS 13+ | `amd64`, `arm64` | Tier 1 |
| Ubuntu 22.04+ | `amd64`, `arm64` | Tier 1 |

The source is expected to work on other recent Linux distributions and supported Go
platforms, but those combinations are not part of the release validation matrix.
Go builds require Go 1.23 or newer. The Python SDK supports Python 3.9 or newer and
requires a separately installed CLI from the same release.

## Platform-specific behavior

- Process ownership and command-line metadata may be incomplete without elevated
  permissions. The CLI reports this as a degraded diagnostic instead of failing the
  complete scan.
- Windows services, macOS protected processes, containers, WSL, and sandboxed
  environments can hide process details or deny termination.
- Port probing covers local TCP listeners. Firewall rules, namespace isolation, and
  IPv4/IPv6 binding choices can make a port appear different across environments.
- `clean` and `kill` are local process operations. Review `--dry-run` output and
  grant elevated permissions only when the target is understood.

Please include the OS version, architecture, Go/Python version, CLI version, and
whether elevated permissions or a container were involved when reporting a
platform-specific issue.
