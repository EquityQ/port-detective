# Platform Support

`port-detective` uses Go, `gopsutil`, and the standard library for local network and
process inspection. CI validates Windows, macOS, and Linux. Release builds target:

| Platform | Architectures | Support level |
| --- | --- | --- |
| Windows 10/11 | `amd64`, `arm64` | CLI Tier 1 |
| macOS 13+ | `amd64`, `arm64` | CLI Tier 1 |
| Ubuntu 22.04+ | `amd64`, `arm64` | CLI Tier 1 |

Go builds require Go 1.25+. The Python SDK requires Python 3.9+ and the Node.js client
requires Node.js 22+. Published Node.js packages install a matching CLI for the listed
platforms. The Python SDK downloads and verifies a matching CLI on first use when no
local binary is available; source checkouts and unsupported platforms still require a
compatible CLI through `PORT_DETECTIVE_BIN`.

## Known limits

- Protected processes and services may hide metadata or deny termination.
- Containers, WSL, sandboxes, firewall rules, and network namespaces can change results.
- Port probing covers local TCP listeners and may differ for IPv4 and IPv6 bindings.
- Review `--dry-run` output before using `--yes` or `dry_run=false`.

When reporting a platform issue, include OS, architecture, runtime versions, CLI version,
and whether elevated permissions or a container were involved.
