# Roadmap

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

This roadmap describes the work required to move `port-detective` from a strong
feature-complete development line to a dependable reference-quality open-source
project.

## Now: Release Readiness

- Review the automated release checklist, then publish the first signed `v1.1.0` CLI and matching Python SDK release. The workflow now creates the GitHub Release only after all artifact gates pass.
- Retain checksums, SBOM, provenance, signatures, and acceptance-test evidence with the release.
- Keep the CLI and SDK version numbers synchronized.
- Publish a short release note with supported platforms and known permission limits.

## Next: Confidence Across Environments

- The real CLI/Python SDK integration workflow now builds and exercises the matching
  binary on Windows, macOS, and Linux with Python 3.9 and 3.13. Keep the matrix
  required and retain its listener-based evidence for every release candidate.
- Add integration fixtures for protected processes, PID reuse, IPv4/IPv6, WSL, and
  containers.
- Keep the scheduled `govulncheck` and `pip-audit` jobs green as dependencies change.
- Generate CLI and MCP reference tables from tests or machine-readable schemas.

## Next: Packaging and measurable quality

- The tag workflow is configured to publish the Python wheel and source distribution
  with PyPI Trusted Publishing, then install the exact version from PyPI and exercise
  it against the matching CLI. Configure the `pypi` environment and publisher before
  creating the first release.
- A zero-dependency npm client with sync/Promise APIs, TypeScript declarations,
  dry-run defaults, and Windows/macOS/Linux real CLI integration is available under
  `npm/port-detective`. Decide whether to configure npm Trusted Publishing for the
  matching CLI release; until then, install the package from a checkout.
- Publish coverage reports and enforce reviewed minimum thresholds without weakening
  the cross-platform, race, MCP, or dependency security gates.

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

- 审查自动化发布清单后，发布签名的 `v1.1.0` CLI 和匹配版本的 Python SDK。
- 在发布记录中保留校验和、SBOM、来源证明、签名和 MCP 验收证据。
- 保持 CLI 与 SDK 版本同步，并公开支持平台和权限限制。

### 下一步：跨环境可信度

- 已增加真实 CLI/Python SDK 集成工作流，在 Windows、macOS 和 Linux 上使用 Python 3.9/3.13 构建并调用匹配二进制；应将该矩阵作为发布候选版本的必需证据。
- 增加受保护进程、PID 复用、IPv4/IPv6、WSL 和容器集成夹具。
- 保持 `govulncheck`、`pip-audit` 定时检查和版本化 CLI/MCP 参考。

### 打包与质量门槛

- Tag 工作流已配置为通过 PyPI Trusted Publishing 发布 Python SDK，并从 PyPI 安装相同版本对匹配 CLI 做真实验证；首次发布前仍需配置 `pypi` 环境和 Trusted Publisher。
- `npm/port-detective` 已提供零依赖客户端、同步/Promise API、TypeScript 类型、dry-run 默认值及 Windows/macOS/Linux 真实 CLI 集成。可在后续为匹配 CLI 版本配置 npm Trusted Publishing；目前从源码仓库安装。各语言覆盖率门槛仍待建立。

### 后续：社区规模化

- 明确维护者、响应目标和发布节奏。
- 增加故障排查、Shell 补全和可复现演示。
- 按系统、架构和权限记录兼容性与基准数据。
- 发布流程已改为先校验、签名验真和生成来源证明，最后创建 GitHub Release；GitHub Actions 已固定到不可变提交并由 Dependabot 跟踪更新。
