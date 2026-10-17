# Generated API Inventory / 自动生成的 API 清单

Generated from the CLI, core, and MCP implementation. See
[API_REFERENCE.md](API_REFERENCE.md) for behavior, result shapes, and safety rules.

根据 CLI、core 和 MCP 实现生成。行为、结果结构和安全规则见
[API_REFERENCE.md](API_REFERENCE.md)。

## CLI commands / CLI 命令

| Command / 命令 | Syntax / 用法 | Source / 源文件 |
| --- | --- | --- |
| `check` | `check <端口号>` | `internal/cli/inspect.go` |
| `clean` | `clean` | `internal/cli/cleanup.go` |
| `completion` | `completion <bash\|zsh\|fish\|powershell>` | `internal/cli/root.go` |
| `kill` | `kill <端口号>` | `internal/cli/cleanup.go` |
| `mcp` | `mcp` | `internal/cli/mcp.go` |
| `scan` | `scan` | `internal/cli/inspect.go` |

## CLI flags / CLI 参数

| Flag / 参数 | Source / 源文件 |
| --- | --- |
| `--dry-run` | `internal/cli/cleanup.go` |
| `--json` | `internal/cli/root.go` |
| `--lang` | `internal/cli/root.go` |
| `--ports` | `internal/cli/inspect.go` |
| `--yes` | `internal/cli/cleanup.go` |

## MCP tools / MCP 工具

| Tool / 工具 | Arguments / 参数 | Source / 源文件 |
| --- | --- | --- |
| `clean_current_project` | `dry_run` | `internal/mcpserver/server.go` |
| `free_port` | `dry_run`, `port` | `internal/mcpserver/server.go` |
| `inspect_port` | `port` | `internal/mcpserver/server.go` |
| `scan_dev_ports` | `ports` | `internal/mcpserver/server.go` |

## Core API / 核心 API

| Kind / 类型 | Names / 名称 |
| --- | --- |
| Types | `PortDiagnostic` |
| Values | `DefaultDevPorts` |
| Functions | `InspectPort`, `ScanPortsConcurrent`, `KillPortProcess` |
