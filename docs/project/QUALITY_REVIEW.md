# Open Source Quality Review

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

**Review date:** 2026-10-16
**Review scope:** Go core, CLI, MCP server, Python SDK, CI/release automation, documentation, and community files.

## Current Assessment

`port-detective` is a strong feature-complete v1.1 development line with a clear
architecture and unusually good safety coverage for a local process tool. It is
ready for careful public use from source or a locally built binary. It is not yet a
fully mature release project because the v1.1.0 tag and published artifact trail do
not exist yet. A real CLI/Python integration matrix now covers Windows, macOS, and
Linux, while its first hosted run remains release evidence rather than a local
assumption.

**Current score: 89/100**

| Area | Score | Evidence | Remaining gap |
| --- | ---: | --- | --- |
| Product usefulness | 18/20 | CLI, MCP, project cleanup, fingerprinting, Python SDK | More real-world compatibility reports |
| Architecture | 17/20 | Core/CLI/MCP separation, worker pool, protocol-safe stdout | Large legacy modules should be split when contracts allow |
| Correctness and safety | 19/20 | 59 Go tests, race tests, PID identity revalidation, listener-only indexing, stable scan output, dry-run and JSON confirmation guard | More OS-specific termination fixtures |
| Developer experience | 14/15 | Bilingual README, examples, issue forms, three-OS Go/Python CI, wheel smoke checks, a real CLI/SDK integration matrix, and a post-publish PyPI install check | API reference and a reproducible demo environment |
| Release and supply chain | 14/15 | Atomic pre-publication verification, immutable Action SHAs, Dependabot, checksum/archive verification, signed artifacts, SPDX SBOM, GitHub provenance, configured PyPI Trusted Publishing, post-publish installation verification, three-OS CI, `govulncheck`, and `pip-audit` | Actual release tag and published artifact trail |
| Community operations | 7/10 | Security, Code of Conduct, status, contribution and PR guidance | Support triage and maintainer ownership |

## Strengths

- The core package does not depend on Cobra or MCP, keeping business logic testable.
- Destructive operations have dry-run defaults at the MCP and SDK layers, PID guards,
  process identity checks, and a second ownership check immediately before termination.
- MCP Stdio keeps stdout reserved for JSON-RPC. Handshake, tool schema, tool calls,
  and stdout purity are covered by executable acceptance scripts.
- Go and Python CI cover Windows, macOS, and Linux with Go 1.23/1.25 and Python
  3.9/3.13. Python tests cover the synchronous and asynchronous wrapper APIs, and
  each OS installs a built wheel.
- The integration workflow builds the matching CLI on all three operating systems
  and verifies synchronous and asyncio SDK calls against a real temporary listener.
- The tag workflow is configured to verify and sign all assets, check Cosign
  signatures, emit provenance attestations, publish Python distributions through
  PyPI Trusted Publishing, install that version from PyPI, and check it against the
  matching CLI before creating the GitHub Release. GitHub Actions are pinned to
  immutable commits and Dependabot tracks updates; the first hosted release remains
  the evidence for the external publishing configuration.
- Scan input validation, TCP listener filtering, stable empty MCP arrays, and
  aggregate project-cleanup failures are covered by focused regression tests.
- The project explains permission-limited process metadata instead of presenting
  incomplete data as certainty.

## Priority Improvements

### P0: Before Calling v1.1.0 Released

1. Create and validate the `v1.1.0` tag only after the release workflow succeeds.
2. Review the generated checksums, signatures, SBOM, and provenance attestation.
3. Run the MCP acceptance scripts against the release binary and attach the output
   to the release notes.
4. Verify the Python wheel installation from the exact matching CLI release.

### P1: Next Development Cycle

1. Keep the cross-platform CLI/SDK integration matrix required and add fixtures for
   protected processes, PID reuse, IPv4/IPv6 listeners, WSL/containers, and
   non-English paths.
2. Generate a versioned CLI/MCP schema reference from the implementation or tests so
   tool arguments cannot drift from the README.
3. Configure the PyPI Trusted Publisher and create the first release so the
   post-publish installation check has public artifact evidence.

### P2: Maturity and Adoption

1. Split the largest Go modules into cohesive files without changing the public
   contracts documented in `AGENTS.md`.
2. Add shell completion examples, a troubleshooting guide, and a short recorded demo.
3. Track compatibility reports and benchmark results by OS, architecture, and
   permission level instead of presenting one environment as universal.
4. Define maintainer ownership, response targets, and a regular release cadence.

## Quality Gates

Every change should pass `go build ./...`, `go test -race ./...`, `go vet ./...`,
`gofmt`, and the Python test suite when applicable. Changes to MCP or CLI behavior
must also run the scripts under `scripts/mcp/`. Release changes must validate the
GoReleaser configuration, archive contents, checksums, and Python wheel smoke test.

See [ROADMAP.md](ROADMAP.md) for the staged implementation plan and
[STATUS.md](../operations/STATUS.md) for the current release state.

## 中文摘要

当前质量基线为 **89/100**。真实 CLI/Python 集成工作流已覆盖 Windows、macOS、Linux 及 Python 3.9/3.13；发布工作流现在会先完成制品校验、签名验真和来源证明，通过 PyPI Trusted Publishing 发布 wheel/sdist，从 PyPI 安装相同版本并对匹配 CLI 做真实验证，最后创建 GitHub Release。所有 GitHub Actions 已固定到不可变提交并由 Dependabot 跟踪。仍需实际创建 `v1.1.0` Tag、保留公开制品证据，并补充版本化 CLI/MCP API 参考。
