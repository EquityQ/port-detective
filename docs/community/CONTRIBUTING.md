# Contributing to port-detective

Thank you for your interest in contributing to **port-detective**! We welcome community contributions, bug reports, feature suggestions, and documentation improvements.

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

Read the [Code of Conduct](CODE_OF_CONDUCT.md), [platform support policy](../operations/PLATFORM_SUPPORT.md),
and [current quality review](../project/QUALITY_REVIEW.md) before making a change. For
environment-specific failures, see the [troubleshooting guide](../operations/TROUBLESHOOTING.md)
and [compatibility report process](../operations/COMPATIBILITY_REPORT.md).

---

## 🧭 Architecture Principles & Ground Rules

To keep `port-detective` lightweight, robust, and AI-native across Windows, macOS, and Linux, please adhere to these non-negotiable architectural boundaries:

1. **`internal/core` Purity**:
   - `internal/core` must **never** import `github.com/spf13/cobra` or `github.com/mark3labs/mcp-go`.
   - Core functions must remain pure business logic and unit-testable without CLI or MCP dependencies.
2. **MCP Stdout Zero-Pollution**:
   - In MCP mode, `os.Stdout` is strictly reserved for JSON-RPC message framing.
   - Never print debugging logs or errors to `os.Stdout`. All diagnostic output must go to `os.Stderr`.
3. **Cross-Platform Compatibility**:
   - Do not invoke external shell commands (like `netstat`, `lsof`, or `kill`). Rely solely on `gopsutil/v3` and the standard library `net` package.
   - Always gracefully handle permission limitations (e.g. SYSTEM processes on Windows) using the `Status: "permission_required"` degradation pattern.

---

## 🛠️ Local Development & Testing

### Prerequisites
- Go 1.23+ (tested with Go 1.23.x and 1.25.x)
- Python 3.9+ (for the SDK and end-to-end MCP verification scripts)

### Run Tests
```bash
# Run all unit tests
go test -v ./...

# Run race condition detector
go test -race ./...

# Verify code formatting and linting
go vet ./...
gofmt -l .
```

When Python files change, also run:
```bash
python -m unittest discover -s python/tests -v
```

### End-to-End MCP Verification
Before submitting a PR, ensure that the MCP Stdio server and protocol purity remain intact:
```bash
# Build temporary binary
go build -o port-detective .

# Verify handshake, tools schema, and stdout purity
python scripts/mcp/verify_handshake.py
python scripts/mcp/verify_stdout_purity.py
python scripts/mcp/verify_tools.py
```

---

## 📝 Commit Message Guidelines

We strictly follow the **[Conventional Commits](https://www.conventionalcommits.org/)** specification:

```text
<type>(<scope>): <short description>
```

### Common Types:
- `feat`: A new feature or capability.
- `fix`: A bug fix.
- `docs`: Documentation changes only.
- `refactor`: Code change that neither fixes a bug nor adds a feature.
- `test`: Adding missing tests or correcting existing tests.
- `chore`: Changes to build process, CI/CD, or auxiliary tools.

### Example:
```text
feat(core): add UDP port diagnostic support
fix(cli): correct column alignment for wide CJK characters
docs(readme): add Windsurf MCP configuration guide
```

---

## 🚀 Submitting a Pull Request

1. **Fork** the repository and create your branch from `main` (e.g. `feature/add-foo` or `fix/bar-error`).
2. Make your changes adhering to the architecture principles.
3. Ensure all tests and lint checks pass locally (`go test -race ./...`).
4. Commit your changes with a conventional commit message.
5. Push to your fork and submit a Pull Request to `main`.
6. Our CI workflow will automatically verify cross-platform builds and race conditions on Ubuntu, macOS, and Windows.

Documentation, CI, and release changes should explain their compatibility impact.
Do not claim a release is available until its tag, checksums, and artifacts have been
published successfully.

## Community and Support

- Review the [platform support policy](../operations/PLATFORM_SUPPORT.md) before filing a platform-specific report.
- Use the repository's issue forms for reproducible bugs and focused feature requests.
- Report security vulnerabilities privately through [SECURITY.md](SECURITY.md), never in a public issue.
- Check [STATUS.md](../operations/STATUS.md) for the current maintenance focus and known limitations.
- See [maintainer operations](MAINTAINERS.md) for response goals and release planning; these are targets, not service guarantees.
- Use [QUALITY_REVIEW.md](../project/QUALITY_REVIEW.md) and [ROADMAP.md](../project/ROADMAP.md) to understand
  current priorities before proposing a broad refactor.

## 中文摘要

欢迎提交代码、问题、功能建议和文档改进。贡献前请阅读[行为准则](CODE_OF_CONDUCT.md)、[平台支持政策](../operations/PLATFORM_SUPPORT.md)和[质量评估](../project/QUALITY_REVIEW.md)。平台问题可先查阅[故障排查](../operations/TROUBLESHOOTING.md)和[兼容性报告指南](../operations/COMPATIBILITY_REPORT.md)；维护者响应目标见[维护者协作](MAINTAINERS.md)，这些目标不构成服务承诺。核心约束是：`internal/core` 不依赖 Cobra 或 MCP；MCP 模式下 stdout 只能输出 JSON-RPC；跨平台逻辑不得调用 `netstat`、`lsof` 或 `kill` 等外部命令。

本地验证至少运行 `go test -race ./...`、`go vet ./...`、`gofmt -l .`；Python 变更还需运行 `python -m unittest discover -s python/tests -v`。提交信息遵循 Conventional Commits，文档、CI 和发布变更需说明兼容性影响。
