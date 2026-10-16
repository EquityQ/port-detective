# Roadmap

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

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

## 中文摘要

### 当前：发布准备

- 发布签名的 `v1.1.0` CLI 和匹配版本的 Python SDK。
- 附带校验和、SBOM、来源证明和 MCP 验收证据。
- 保持 CLI 与 SDK 版本同步，并公开支持平台和权限限制。

### 下一步：跨环境可信度

- 将 Python CI 扩展到 Windows 和 macOS。
- 增加受保护进程、PID 复用、IPv4/IPv6、WSL 和容器集成夹具。
- 增加依赖漏洞扫描、定时测试和版本化 CLI/MCP 参考。

### 后续：社区规模化

- 明确维护者、响应目标和发布节奏。
- 增加故障排查、Shell 补全和可复现演示。
- 按系统、架构和权限记录兼容性与基准数据。
