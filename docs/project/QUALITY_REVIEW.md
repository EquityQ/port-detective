# Open Source Quality Review

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

**Review date:** 2026-10-16
**Review scope:** Go core, CLI, MCP server, Python and Node.js SDKs, CI/release automation, documentation, and community files.

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
| Developer experience | 14/15 | Bilingual READMEs, issue forms, three-OS Go/Python/Node.js CI, a typed dependency-free Node.js package, wheel smoke checks, real CLI integration matrices, a versioned CLI/MCP API reference with CI drift detection, and a post-publish PyPI install check | Published npm package, generated reference output, and a reproducible demo environment |
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
- The Node.js client provides sync and Promise APIs, TypeScript declarations, safe
  process-action defaults, timeout and abort handling, and a real CLI matrix on
  Windows, macOS, and Linux with Node.js 22 and 24.
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
2. Configure the PyPI Trusted Publisher and create the first release so the
   post-publish installation check has public artifact evidence.
3. Decide whether to configure npm Trusted Publishing and publish the Node.js client
   with the matching CLI release.

### P2: Maturity and Adoption

1. Split the largest Go modules into cohesive files without changing the public
   contracts documented in `AGENTS.md`.
2. Add shell completion examples, a troubleshooting guide, and a short recorded demo.
3. Track compatibility reports and benchmark results by OS, architecture, and
   permission level instead of presenting one environment as universal.
4. Define maintainer ownership, response targets, and a regular release cadence.

## Quality Gates

Every change should pass `go build ./...`, `go test -race ./...`, `go vet ./...`,
`gofmt`, the API reference drift check, and the Python and Node.js test suites when applicable. Changes to MCP or CLI behavior
must also run the scripts under `scripts/mcp/`. Release changes must validate the
GoReleaser configuration, archive contents, checksums, and Python wheel smoke test.

CI enforces Go statement coverage of 60% for core, 45% for CLI, 70% for MCP, and
70% for fingerprinting. Python requires 60% for `core.py`, 85% for `models.py`,
and 65% overall. Node.js requires 45% line, 65% branch, and 55% function coverage.
Go and Python HTML reports are retained as workflow artifacts.

See [ROADMAP.md](ROADMAP.md) for the staged implementation plan and
[STATUS.md](../operations/STATUS.md) for the current release state.

## 中文摘要

当前质量基线为 **89/100**。CI 已为 Go core/CLI/MCP/fingerprint（60%/45%/70%/70%）、Python core/models/总计（60%/85%/65%）和 Node.js 行/分支/函数（45%/65%/55%）建立分层覆盖率门槛，并保留 Go 与 Python 报告。真实 CLI/Python/Node.js 集成工作流覆盖 Windows、macOS、Linux；Node.js 客户端提供零依赖同步/Promise API 和类型声明，但 npm registry 发布尚未配置。已补充版本化 CLI/MCP 接口参考；仍需实际创建 `v1.1.0` Tag、保留公开制品证据、决定 npm 发布方式，并让参考文档未来由实现或测试自动校验。
