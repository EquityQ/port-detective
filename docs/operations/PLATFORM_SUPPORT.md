# Platform Support

`port-detective` uses Go, `gopsutil`, and the standard library for local network and
process inspection. CI validates Windows, macOS, and Linux. Release builds target:

| Platform | Architectures | Support level |
| --- | --- | --- |
| Windows 10/11 | `amd64`, `arm64` | CLI Tier 1 |
| macOS 13+ | `amd64`, `arm64` | CLI Tier 1 |
| Ubuntu 22.04+ | `amd64`, `arm64` | CLI Tier 1 |

Go builds require Go 1.23+. The Python SDK requires Python 3.9+ and the Node.js client
requires Node.js 22+. Both SDKs use a separately installed CLI from the same release.

## Known limits

- Protected processes and services may hide metadata or deny termination.
- Containers, WSL, sandboxes, firewall rules, and network namespaces can change results.
- Port probing covers local TCP listeners and may differ for IPv4 and IPv6 bindings.
- Review `--dry-run` output before using `--yes` or `dry_run=false`.

When reporting a platform issue, include OS, architecture, runtime versions, CLI version,
and whether elevated permissions or a container were involved.
