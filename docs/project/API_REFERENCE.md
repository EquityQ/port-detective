# CLI and MCP API Reference

API revision / 接口修订：**v1.1 development line** · CLI build default: `1.1.0-dev`

This is the concise public contract for the CLI JSON interface and MCP tools. Keep
the CLI, Python SDK, and Node.js client on the same reviewed commit or release.

这是 CLI JSON 接口和 MCP 工具的简明公开契约。CLI、Python SDK 和 Node.js 客户端应使用同一个经过审核的提交或版本。

## CLI

Global flags apply before or after a subcommand:

| Flag | Meaning |
| --- | --- |
| `--json` | Emit machine-readable JSON for `check`, `scan`, `kill`, or `clean`. |
| `--lang en\|zh` | Select human-readable output language; JSON is language-neutral. |
| `--version` | Print the build version. |

全局参数可放在子命令前或后：`--json` 输出机器可读 JSON，`--lang en|zh` 选择终端语言，`--version` 输出构建版本。

### Commands

| Command | Arguments and flags | JSON result |
| --- | --- | --- |
| `check <port>` | TCP port `1-65535` | One `PortDiagnostic` object. |
| `scan` | Optional `--ports 3000,8080`; defaults to `DefaultDevPorts` | `PortDiagnostic[]`, sorted by port. |
| `kill <port>` | `--dry-run` previews; `--yes` skips interactive confirmation | `{port, dry_run, diagnostic, message}`. |
| `clean` | Operates on the current project; `--dry-run` previews; `--yes` skips confirmation | `PortDiagnostic[]`. |
| `mcp` | Starts the Stdio MCP server | JSON-RPC only on stdout. |
| `completion <shell>` | `bash`, `zsh`, `fish`, or `powershell` | Writes the generated shell script to stdout. |

`kill` and `clean` perform real termination only when dry-run is disabled. A
destructive JSON invocation also requires `--yes` (`-y`). Process identity and
listener ownership are revalidated immediately before termination.

`kill` 和 `clean` 只有在关闭预演模式时才会真实终止进程。破坏性 JSON 调用还必须显式添加 `--yes`（`-y`）。终止前会重新校验进程身份和监听归属。

`completion <shell>` 支持 Bash、Zsh、Fish 和 PowerShell，直接向 stdout 输出补全脚本，不输出 JSON。安装示例见[Shell 补全指南](../operations/SHELL_COMPLETION.md)。

### `PortDiagnostic`

```json
{
  "port": 8080,
  "is_open": true,
  "protocol": "tcp",
  "pid": 1234,
  "process_name": "node",
  "cmdline": "node server.js",
  "memory_mb": 42.5,
  "cpu_percent": 1.2,
  "status": "occupied",
  "project": {
    "name": "example-app",
    "framework": "Express",
    "path": "/workspace/example-app",
    "version": "1.0.0"
  }
}
```

`status` is normally `occupied`, `free`, or `permission_required`. Process fields
can be empty or zero when the operating system denies access.

## MCP tools

The server is started with `port-detective mcp`. Stdio stdout is reserved for
JSON-RPC; diagnostics belong on stderr.

服务通过 `port-detective mcp` 启动。Stdio 的 stdout 仅用于 JSON-RPC，诊断信息写入 stderr。

| Tool | Input | Safety and result |
| --- | --- | --- |
| `inspect_port` | Required `port` number `1-65535` | Read-only; returns one diagnostic object. |
| `scan_dev_ports` | Optional `ports` string such as `"3000,8080"` | Read-only; defaults to `DefaultDevPorts`; returns an array. |
| `free_port` | Required `port`; optional `dry_run` boolean, default `true` | Destructive when false; returns `port`, `dry_run`, `diagnostic`, and `message`. |
| `clean_current_project` | Optional `dry_run` boolean, default `true` | Destructive when false; scopes listeners to the current working directory. |

All tool descriptions and annotations are verified by the MCP acceptance tests in
`scripts/mcp/`. The Python and Node.js SDKs consume the CLI JSON contract rather
than opening a second process-inspection implementation.

所有工具的描述和注解由 `scripts/mcp/` 下的 MCP 验收测试校验。Python 和 Node.js SDK 消费 CLI JSON 契约，不重复实现进程探测逻辑。

## Compatibility policy / 兼容策略

Fields are additive within the v1.1 development line. A release that changes
field names, command flags, tool names, or safety defaults must update this page,
the SDK models, acceptance tests, and the changelog together.

v1.1 开发线内允许增加字段。若发布版本修改字段名、命令参数、工具名或安全默认值，必须同步更新本页、SDK 模型、验收测试和变更记录。
