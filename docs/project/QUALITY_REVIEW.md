# Open Source Quality Review

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

**Review date:** 2026-10-15
**Review scope:** Go core, CLI, MCP server, Python SDK, CI/release automation, documentation, and community files.

## Current Assessment

`port-detective` is a strong feature-complete v1.1 development line with a clear
architecture and unusually good safety coverage for a local process tool. It is
ready for careful public use from source or a locally built binary. It is not yet a
fully mature release project because the v1.1.0 tag and published artifact trail do
not exist yet, and supply-chain metadata and cross-platform SDK validation are still
limited.

**Current score: 78/100**

| Area | Score | Evidence | Remaining gap |
| --- | ---: | --- | --- |
| Product usefulness | 18/20 | CLI, MCP, project cleanup, fingerprinting, Python SDK | More real-world compatibility reports |
| Architecture | 17/20 | Core/CLI/MCP separation, worker pool, protocol-safe stdout | Large legacy modules should be split when contracts allow |
| Correctness and safety | 17/20 | 46 Go tests, race tests, PID identity revalidation, dry-run | More OS-specific termination fixtures |
| Developer experience | 11/15 | Bilingual README, examples, issue forms, CI | API reference and a reproducible demo environment |
| Release and supply chain | 8/15 | Pinned GoReleaser, checksums, matrix Go CI | Signed artifacts, SBOM, provenance, actual release tag |
| Community operations | 7/10 | Security, support, status, contribution and PR guidance | Code of Conduct, support triage and maintainer ownership |

## Strengths

- The core package does not depend on Cobra or MCP, keeping business logic testable.
- Destructive operations have dry-run defaults at the MCP and SDK layers, PID guards,
  process identity checks, and a second ownership check immediately before termination.
- MCP Stdio keeps stdout reserved for JSON-RPC. Handshake, tool schema, tool calls,
  and stdout purity are covered by executable acceptance scripts.
- Go CI covers Windows, macOS, and Linux with Go 1.23 and 1.25. Python tests cover
  the synchronous and asynchronous wrapper APIs.
- The project explains permission-limited process metadata instead of presenting
  incomplete data as certainty.

## Priority Improvements

### P0: Before Calling v1.1.0 Released

1. Create and validate the `v1.1.0` tag only after the release workflow succeeds.
2. Publish checksums together with signed archives and a generated SBOM.
3. Run the MCP acceptance scripts against the release binary and attach the output
   to the release notes.
4. Verify the Python wheel installation from the exact matching CLI release.

### P1: Next Development Cycle

1. Add release provenance (for example, GitHub artifact attestations) and dependency
   vulnerability scanning (`govulncheck` and a Python dependency audit).
2. Add Python CI on Windows and macOS, then document the tested SDK matrix separately
   from the Go CLI matrix.
3. Add integration fixtures for protected processes, PID reuse, IPv4/IPv6 listeners,
   WSL/containers, and non-English paths.
4. Generate a versioned CLI/MCP schema reference from the implementation or tests so
   tool arguments cannot drift from the README.

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

当前质量基线为 **78/100**。项目已经具备清晰的 core/CLI/MCP 分层、并发扫描、进程安全校验、MCP 协议验收、跨平台 Go CI 和 Python SDK，但仍属于 v1.1 开发线，尚未达到成熟发布项目的完整闭环。

发布前必须完成：正式 `v1.1.0` 标签、签名归档、校验和、SBOM、来源证明、MCP 发布包验收，以及与 CLI 版本匹配的 Python wheel 验证。下一阶段应补充 Python 在 Windows/macOS 的 CI、权限和 PID 复用集成夹具、依赖漏洞扫描、版本化 CLI/MCP API 参考。
