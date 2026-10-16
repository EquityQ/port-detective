# Platform Support

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

`port-detective` is built with Go and uses `gopsutil` plus the standard library for
network and process inspection. The Go and Python CI matrices validate Windows, macOS,
and Linux. The release configuration
targets the following CLI platforms, but no v1.1.0 archives are published yet:

| Platform | Architectures | Support level |
| --- | --- | --- |
| Windows 10/11 | `amd64`, `arm64` | CLI Tier 1 |
| macOS 13+ | `amd64`, `arm64` | CLI Tier 1 |
| Ubuntu 22.04+ | `amd64`, `arm64` | CLI Tier 1 |

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

See [QUALITY_REVIEW.md](../project/QUALITY_REVIEW.md) for the remaining integration
fixtures and release work. Python CI validates SDK logic and wheel installation on all
three operating systems; its subprocess calls are mocked, so it does not replace
end-to-end checks against a platform-specific CLI binary.

## 中文摘要

正式验证矩阵覆盖 Windows 10/11、macOS 13+ 和 Ubuntu 22.04+，CLI 提供 `amd64` 与 `arm64` 构建；Go 要求 1.23+，Python SDK 要求 3.9+，且必须使用同一版本的 CLI。Go 和 Python CI 均覆盖三类操作系统；Python CI 主要验证 SDK 逻辑和 wheel 安装，子进程调用使用 mock，真实 CLI 集成仍需单独夹具。

受保护进程、Windows 服务、容器、WSL 和沙箱环境可能无法读取完整进程信息或拒绝终止操作。端口探测面向本机 TCP 监听，防火墙、网络命名空间及 IPv4/IPv6 绑定方式可能导致结果差异。报告平台问题时，请附操作系统、架构、Go/Python 和 CLI 版本，以及是否使用提权或容器。
