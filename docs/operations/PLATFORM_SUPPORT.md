# Platform Support

`port-detective` uses Go, `gopsutil`, and the standard library for local network and
process inspection. CI validates Windows, macOS, and Linux. Release builds target:

| Platform | Architectures | Support level |
| --- | --- | --- |
| Windows 10/11 | `amd64`, `arm64` | CLI Tier 1 |
| macOS 13+ | `amd64`, `arm64` | CLI Tier 1 |
| Ubuntu 22.04+ | `amd64`, `arm64` | CLI Tier 1 |

Go builds require Go 1.25.5+. The Python SDK requires Python 3.9+ and the Node.js client
requires Node.js 22+. Published Node.js packages install a matching CLI for the listed
platforms. The Python SDK downloads and verifies a matching CLI on first use when no
local binary is available; source checkouts and unsupported platforms still require a
compatible CLI through `PORT_DETECTIVE_BIN`.

## Known limits

- Protected processes and services may hide metadata or deny termination.
- Containers, WSL, sandboxes, firewall rules, and network namespaces can change results.
- Source builds read local TCP listener tables, including bindings to specific
  interfaces. If table access fails, inspection falls back to IPv4/IPv6 loopback
  probes; that fallback cannot rule out interface-only listeners. Full project
  discovery fails explicitly when it cannot read the listener table.
- `find-free` tests loopback binding and releases its test sockets before returning;
  another process may claim the suggestion before your application binds it.
- Review `--dry-run` output before using `--yes` or `dry_run=false`.

When reporting a platform issue, include OS, architecture, runtime versions, CLI version,
and whether elevated permissions or a container were involved.
