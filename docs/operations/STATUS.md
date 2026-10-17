# Project Status

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

`port-detective` is currently at the v1.1 feature-complete development milestone.
The repository's latest Git tag is `v0.1.0-assignment`; a signed and published
`v1.1.0` release has not been cut yet. The current development line includes:

- Concurrent TCP port inspection with process health diagnostics.
- Project and framework fingerprinting for Node.js, Python, and Go services.
- Dry-run-aware process cleanup through the CLI and MCP tools.
- A bilingual CLI, MCP Stdio server, Python 3.9+ SDK, and zero-dependency Node.js 22+ client.
- Bash, Zsh, Fish, and PowerShell completion scripts, plus a listener-based CLI demo that never terminates an external process.
- CLI setup, inspection/scanning, cleanup, MCP, port parsing, and terminal rendering are separated into focused package files.
- GoReleaser configuration targeting Windows, macOS, and Linux on `amd64` and
  `arm64`; the release workflow builds artifacts without publishing, verifies
  versions/checksums, generates an SPDX SBOM, signs and verifies release files,
  emits GitHub provenance attestations, and is configured to publish Python
  distributions through PyPI Trusted Publishing, verify the public installation
  against the matching CLI, and create the GitHub Release only at the end. No v1.1.0
  archives or PyPI/npm packages have been published yet.

The project is in active maintenance. Current priorities are the first tagged release,
end-to-end compatibility reports, and careful security fixes. Go, Python, and Node.js
CI cover Windows, macOS, and Linux. The standard Python matrix uses mocked CLI
subprocesses for fast SDK feedback, while separate Python and Node.js integration
matrices build and exercise the real CLI on all three operating systems. The Node.js
package can be installed from the checkout; it has not been published to npm. Process
metadata can still be unavailable for protected or system-owned processes; this is
reported as a permission-limited diagnostic rather than treated as proof that a
port is free.

See [Platform Support](PLATFORM_SUPPORT.md), [Contributing](../community/CONTRIBUTING.md), and
[Security Policy](../community/SECURITY.md) before opening a report. The detailed gap analysis is
in [QUALITY_REVIEW.md](../project/QUALITY_REVIEW.md), with execution order in [ROADMAP.md](../project/ROADMAP.md).

## 中文摘要

项目当前处于 v1.1 功能完成开发阶段，最新 Git 标签仍是 `v0.1.0-assignment`，签名并正式发布的 `v1.1.0` 尚未创建。现有能力包括并发端口诊断、Node/Python/Go 项目指纹、CLI/MCP 清理功能、双语 CLI、MCP Stdio 服务、Python 3.9+ SDK 和零依赖 Node.js 22+ 客户端；CLI 已按根命令、检查扫描、清理、MCP、端口解析和输出渲染职责拆分。

当前 CLI 提供 Bash、Zsh、Fish 和 PowerShell 补全，并附有仅操作自身临时监听器的可复现演示。故障排查、兼容性报告和维护者协作入口已整理在文档中心。当前重点是首个正式 Tag、真实 CLI 兼容性报告及安全修复。发布工作流已配置为先构建并校验版本、归档和校验和，生成 SBOM、签名并验证签名、发布来源证明，通过 PyPI Trusted Publishing 发布 Python 分发包，从 PyPI 安装相同版本并对匹配 CLI 做真实验证，最后才创建 GitHub Release。Go、Python 和 Node.js CI 已覆盖 Windows、macOS 和 Linux；Node.js 包可从源码仓库安装，尚未发布到 npm。受保护进程的元数据可能不可用，工具会将其标记为权限受限诊断，不会据此判断端口空闲。
