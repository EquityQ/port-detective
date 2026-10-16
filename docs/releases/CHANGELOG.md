# Changelog

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

All notable changes are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/).

## [Unreleased]

The repository currently contains the v1.1 feature-complete development line. A
`v1.1.0` release is planned after the release-readiness gates in
[QUALITY_REVIEW.md](../project/QUALITY_REVIEW.md) pass.

### Added

- Community security policy, platform support policy, status page, issue forms, and
  pull request template.
- Quality review and staged open-source maturity roadmap.
- Release artifact verification, SPDX SBOM generation, Cosign signatures, and
  GitHub provenance attestation workflow.
- Atomic release publication: artifacts are built, verified, signed, signature-
  verified, and attested before the GitHub Release is created.
- Immutable GitHub Action references and Dependabot update configuration for Go,
  Python, and workflow dependencies.
- PyPI Trusted Publishing for the Python wheel and source distribution, followed
  by a retrying post-publish installation and real CLI integration check.
- Dependency-free Node.js client with synchronous and Promise APIs, TypeScript
  declarations, dry-run process-action defaults, and a three-OS real CLI matrix.
- Reviewed coverage thresholds for Go packages and Python/Node.js SDKs, with Go
  and Python coverage reports retained by CI.

### Changed

- Documentation now distinguishes source builds from published release artifacts.
- CLI, MCP, and Python SDK examples are checked against the current implementation.

## [0.1.0-assignment]

The original course assignment baseline. See the repository history for the detailed
implementation record.

## 中文摘要

本文件遵循 [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) 和语义化版本规范。

### [未发布]

当前仓库是 v1.1 功能完成开发线，正式 `v1.1.0` 将在发布门禁通过后创建。本阶段新增社区安全策略、平台支持政策、项目状态、Issue 表单、质量评估、开源成熟度路线图，以及发布制品校验、SPDX SBOM、Cosign 签名、签名验真、GitHub 来源证明和 PyPI Trusted Publishing 工作流；发布会在所有门禁通过后最后创建。新增零依赖 Node.js 客户端、TypeScript 类型、同步和 Promise API、dry-run 安全默认值及三平台真实 CLI 集成。CI 已为 Go、Python 和 Node.js 建立分层覆盖率门槛，并保留 Go/Python 覆盖率报告。工作流动作已固定到不可变提交，Dependabot 负责依赖更新；发布后会从 PyPI 安装相同版本并对匹配 CLI 做真实验证，同时校正源码构建、发布产物和各 SDK 示例说明。

### [0.1.0-assignment]

课程作业基线版本，详细实现记录请查看 Git 历史。
