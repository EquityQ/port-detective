# Troubleshooting

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

## Common issues

| Symptom | Check |
| --- | --- |
| Port is shown as free, but a service is reachable | Confirm the port is a local TCP listener; check IPv4/IPv6 binding, WSL/container namespaces, and which machine runs the CLI. |
| `permission_required` or empty process fields | The listener is visible but the OS restricts process metadata. Retry with appropriate privileges only when you understand the impact. |
| `kill` or `clean` refuses to continue | Start with `--dry-run`; confirm the target and project path. JSON termination requires explicit `--yes`. |
| Python or Node.js SDK cannot find the CLI | Build/install the matching CLI separately, add it to `PATH`, or set `PORT_DETECTIVE_BIN`. |
| MCP client fails to parse server output | Launch `port-detective mcp` directly and keep stdout connected to the MCP client. Do not pipe logs or shell banners into stdout. |
| Command is not found after installation | Confirm the executable directory is on `PATH`, restart the shell, and check `port-detective --version`. |

## Diagnostic commands

```sh
port-detective --version
port-detective check 8080 --json
port-detective scan --ports 3000,5173,8080 --json
go version
python --version
node --version
```

Remove process command lines and private paths before sharing output. For a platform-specific problem, file a compatibility report using the [report guide](COMPATIBILITY_REPORT.md).

## 常见问题

| 现象 | 检查方法 |
| --- | --- |
| 显示端口空闲，但服务可以访问 | 确认目标是本机 TCP 监听；检查 IPv4/IPv6 绑定、WSL/容器网络命名空间，以及 CLI 所在主机。 |
| 状态为 `permission_required` 或进程字段为空 | 系统可见监听端口，但限制读取进程元数据。仅在理解影响后再使用适当权限重试。 |
| `kill` 或 `clean` 拒绝执行 | 先使用 `--dry-run`，核对目标和项目路径。JSON 终止操作必须显式添加 `--yes`。 |
| Python/Node.js SDK 找不到 CLI | 单独构建或安装匹配版本 CLI，将其加入 `PATH`，或设置 `PORT_DETECTIVE_BIN`。 |
| MCP 客户端无法解析服务输出 | 直接运行 `port-detective mcp`，保持 stdout 连接 MCP 客户端；不要把日志或 shell 提示符混入 stdout。 |
| 安装后提示找不到命令 | 确认可执行文件目录在 `PATH` 中，重启 Shell，并检查 `port-detective --version`。 |

分享输出前请删除进程命令行和私有路径。平台相关问题请按[兼容性报告指南](COMPATIBILITY_REPORT.md)提交。
