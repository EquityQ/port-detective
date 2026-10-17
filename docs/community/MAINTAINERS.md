# Maintainer Operations

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

No individual maintainer roster is published yet. Until one is established, ownership is assigned by responsibility: repository maintainers handle releases and project direction; security reports follow [SECURITY.md](SECURITY.md); platform-specific reports are triaged using the [compatibility report guide](../operations/COMPATIBILITY_REPORT.md).

## Response goals

These are planning targets, not guaranteed service levels:

| Work item | Initial response goal | Follow-up |
| --- | --- | --- |
| Security report | Acknowledge within 3 business days through the private channel | Triage privately; coordinate a fix and disclosure. |
| Reproducible bug or compatibility report | Acknowledge within 7 calendar days | Confirm scope and request only missing reproduction details. |
| Pull request | First review within 14 calendar days | Explain requested changes or the reason for deferral. |
| Feature request | Triage within 30 calendar days | Accept, defer, or close with a short rationale. |

If capacity is limited, maintainers should update the issue status instead of allowing targets to imply guaranteed support.

## Release cadence

- Plan a feature release approximately every quarter when changes and validation are ready.
- Publish security and compatibility fixes as needed; do not wait for the feature cadence.
- Create a release only after the [release checklist](../releases/RELEASE_CHECKLIST.md) passes and matching CLI/SDK artifacts are verified.
- Record supported environments, known limitations, and compatibility evidence in the release notes.

## 维护者协作

项目尚未公开个人维护者名单。名单建立前按职责分配：仓库维护者负责发布和项目方向；安全报告按[安全策略](SECURITY.md)私密处理；平台问题按[兼容性报告指南](../operations/COMPATIBILITY_REPORT.md)分流。

### 响应目标

以下为规划目标，不构成服务时限承诺：

| 事项 | 首次响应目标 | 后续处理 |
| --- | --- | --- |
| 安全报告 | 通过私密渠道在 3 个工作日内确认收到 | 私密分级，协调修复与披露。 |
| 可复现缺陷或兼容性报告 | 7 个自然日内确认收到 | 确认影响范围，仅补充询问缺少的复现信息。 |
| Pull Request | 14 个自然日内首次审查 | 说明修改要求或延期原因。 |
| 功能建议 | 30 个自然日内分流 | 简要说明接受、延期或关闭理由。 |

若维护资源不足，应更新 Issue 状态，避免让目标被理解为保证支持。

### 发布节奏

- 条件成熟时约每季度规划一次功能版本。
- 安全和兼容性修复按需发布，不必等待功能版本周期。
- 只有[发布清单](../releases/RELEASE_CHECKLIST.md)通过且 CLI/SDK 制品匹配校验后才创建正式版本。
- 发布说明记录支持环境、已知限制和兼容性证据。
