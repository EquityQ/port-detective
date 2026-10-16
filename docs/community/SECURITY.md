# Security Policy

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

## Supported Versions

Security fixes are applied to the latest published release and the active development
line. Please upgrade before reporting an issue against an older release.

| Version | Security fixes |
| --- | --- |
| Active development line (unreleased) | Supported |
| `v0.1.0-assignment` and older | Best effort only |

## Reporting a Vulnerability

Please use a [private GitHub Security Advisory](https://github.com/EquityQ/port-detective/security/advisories/new)
to report a vulnerability. Do not open a public issue or include exploit details
in a pull request.

Include the affected version, operating system and architecture, installation
method, reproduction steps, expected impact, and any proof of concept that can be
shared safely. Remove credentials, tokens, private paths, and other sensitive data
before submitting.

The maintainers will acknowledge a report within 5 business days and will use the
advisory to coordinate validation, a fix, and public disclosure. Response times are
best effort for this community-maintained project.

The repository does not currently publish a signed v1.1.0 artifact. For deployed
automation, use a reviewed commit or a future release with verified checksums and
provenance.

## Scope and Safety Notes

`port-detective` can inspect process metadata and can terminate a process when the
user explicitly requests it. Test termination behavior only against a disposable
local process. Never include production credentials or host data in a report.

## 中文摘要

安全问题请通过 [GitHub 私密 Security Advisory](https://github.com/EquityQ/port-detective/security/advisories/new) 报告，不要在公开 Issue 或 Pull Request 中披露漏洞细节。请提供受影响版本、操作系统和架构、安装方式、复现步骤及影响范围，并删除凭据、令牌和私有路径。

项目目前支持最新正式版本和活跃开发线。由于当前 `v1.1.0` 尚未正式发布，生产自动化应使用经过审核的提交或带校验和与来源证明的未来正式版本。涉及终止进程的测试只能使用临时本地进程。
