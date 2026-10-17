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
- Add integration fixtures for protected processes, PID reuse, WSL, and containers.
- IPv4 and IPv6 loopback, Unicode project-path, and PID 0/1 guard checks now run in
  the core suite; the IPv6 fixture skips when host policy blocks IPv6 loopback.
- Keep the scheduled `govulncheck` and `pip-audit` jobs green as dependencies change.
- Keep the versioned [CLI/MCP API reference](API_REFERENCE.md) synchronized with
  implementation and acceptance tests; CI now fails when documented command, flag,
  tool, or argument names drift.

## Next: Packaging and measurable quality

- The tag workflow is configured to publish the Python wheel and source distribution
  with PyPI Trusted Publishing, then install the exact version from PyPI and exercise
  it against the matching CLI. Configure the `pypi` environment and publisher before
  creating the first release.
- A zero-dependency npm client with sync/Promise APIs, TypeScript declarations,
  dry-run defaults, and Windows/macOS/Linux real CLI integration is available under
  `npm/port-detective`. Decide whether to configure npm Trusted Publishing for the
  matching CLI release; until then, install the package from a checkout.
- Go, Python, and Node.js CI now enforce reviewed package-level coverage thresholds;
  Go and Python HTML reports are published as workflow artifacts. Raise thresholds
  alongside meaningful regression tests without weakening platform, race, MCP, or
  dependency security gates.

## Later: Community Scale

- Establish a named maintainer roster and review documented response goals and
  release cadence against actual community activity.
- Track compatibility reports and benchmarks by operating system, architecture,
  and permission level.

The troubleshooting guide, four-shell completion walkthroughs, compatibility report
form, safe reproducible CLI demo, and focused CLI implementation files are available
from the documentation index and source tree.

Priorities may change as compatibility reports arrive. Security fixes and data-loss
prevention always take precedence over feature work.

## 中文摘要

### 当前：发布准备

- 审查自动化发布清单后，发布签名的 `v1.1.0` CLI 和匹配版本的 Python SDK。
- 在发布记录中保留校验和、SBOM、来源证明、签名和 MCP 验收证据。
- 保持 CLI 与 SDK 版本同步，并公开支持平台和权限限制。

### 下一步：跨环境可信度

- 已增加真实 CLI/Python SDK 集成工作流，在 Windows、macOS 和 Linux 上使用 Python 3.9/3.13 构建并调用匹配二进制；应将该矩阵作为发布候选版本的必需证据。
- 增加受保护进程、PID 复用、WSL 和容器集成夹具；IPv4/IPv6 回环、Unicode 路径及 PID 0/1 保护测试已加入核心测试，IPv6 会按宿主能力跳过。
- 保持 `govulncheck`、`pip-audit` 定时检查和版本化 CLI/MCP 参考。

### 打包与质量门槛

- Tag 工作流已配置为通过 PyPI Trusted Publishing 发布 Python SDK，并从 PyPI 安装相同版本对匹配 CLI 做真实验证；首次发布前仍需配置 `pypi` 环境和 Trusted Publisher。
- `npm/port-detective` 已提供零依赖客户端、同步/Promise API、TypeScript 类型、dry-run 默认值及 Windows/macOS/Linux 真实 CLI 集成。可在后续为匹配 CLI 版本配置 npm Trusted Publishing；目前从源码仓库安装。
- Go、Python 和 Node.js CI 已执行经审查的分层覆盖率门槛；Go 与 Python 覆盖率报告作为工作流产物保留。后续应通过有针对性的回归测试逐步提高门槛，并保持平台集成、race、MCP 和依赖安全门禁。

### 后续：社区规模化

- 建立具名维护者名单，并根据实际社区活动复核已记录的响应目标和发布节奏。
- 按系统、架构和权限记录兼容性报告与基准数据。
- 核心终止与项目清理逻辑、CLI 根命令、输出渲染、检查扫描、清理、MCP 和端口解析已按职责拆分。发布流程已改为先校验、签名验真和生成来源证明，最后创建 GitHub Release；GitHub Actions 已固定到不可变提交并由 Dependabot 跟踪更新。
- 故障排查、四种 Shell 补全指南、兼容性报告表单和安全可复现 CLI 演示已提供。
