# Contributing to port-detective

Thank you for your interest in contributing to **port-detective**! We welcome community contributions, bug reports, feature suggestions, and documentation improvements.

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
- Go 1.22+ (tested up to Go 1.25.x)
- Python 3.8+ (for running end-to-end MCP verification scripts)

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
