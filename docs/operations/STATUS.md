# Project Status

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

`port-detective` is currently at the v1.1 feature-complete development milestone.
The repository's latest Git tag is `v0.1.0-assignment`; a signed and published
`v1.1.0` release has not been cut yet. The current development line includes:

- Concurrent TCP port inspection with process health diagnostics.
- Project and framework fingerprinting for Node.js, Python, and Go services.
- Dry-run-aware process cleanup through the CLI and MCP tools.
- A bilingual CLI, MCP Stdio server, and Python 3.9+ SDK.
- Cross-platform release archives for Windows, macOS, and Linux on `amd64` and
  `arm64`.

The project is in active maintenance. Current priorities are signed release artifacts,
provenance/SBOM metadata, cross-platform Python validation, compatibility reports,
and careful security fixes. Process
metadata can still be unavailable for protected or system-owned processes; this is
reported as a permission-limited diagnostic rather than treated as proof that a
port is free.

See [Platform Support](PLATFORM_SUPPORT.md), [Contributing](../community/CONTRIBUTING.md), and
[Security Policy](../community/SECURITY.md) before opening a report. The detailed gap analysis is
in [QUALITY_REVIEW.md](../project/QUALITY_REVIEW.md), with execution order in [ROADMAP.md](../project/ROADMAP.md).

## 中文摘要

项目当前处于 v1.1 功能完成开发阶段，最新 Git 标签仍是 `v0.1.0-assignment`，签名并正式发布的 `v1.1.0` 尚未创建。现有能力包括并发端口诊断、Node/Python/Go 项目指纹、CLI/MCP 清理功能、双语 CLI、MCP Stdio 服务和 Python 3.9+ SDK。

当前重点是签名发布包、SBOM 和来源证明、Python 跨平台验证、兼容性报告及安全修复。受保护进程的元数据可能不可用，工具会将其标记为权限受限诊断，不会据此判断端口空闲。
