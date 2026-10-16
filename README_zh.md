# port-detective

[English README](README.md)

面向本机 TCP 端口诊断的跨平台 CLI、MCP 服务端和 Python SDK，支持进程信息、项目指纹和项目范围端口清理。

> **发布状态：** 当前处于 v1.1 开发主线，尚未发布 v1.1.0 二进制包、PyPI 包或 npm 包。正式版本发布前请使用经过审核的源码提交。

## 快速开始

要求：CLI 需要 Go 1.23+；Python SDK 需要 Python 3.9+；Node.js 客户端需要 Node.js 22+。

```bash
git clone https://github.com/EquityQ/port-detective.git
cd port-detective
go build -o port-detective .
```

```bash
port-detective check 3000
port-detective scan --ports 3000,5173,8080
port-detective kill 3000 --dry-run
port-detective clean --dry-run
```

`check`、`scan`、`kill` 和 `clean` 支持 `--json`。JSON 模式不会绕过安全确认，真实终止必须显式添加 `--yes`：

```bash
port-detective kill 3000 --json --yes
port-detective clean --json --yes
```

运行测试：

```bash
go test -race ./...
```

## MCP

启动 Stdio 服务：

```bash
port-detective mcp
```

客户端配置示例：

```json
{
  "mcpServers": {
    "port-detective": {
      "command": "port-detective",
      "args": ["mcp"]
    }
  }
}
```

服务提供 `inspect_port`、`scan_dev_ports`、`free_port` 和 `clean_current_project`。只读工具返回诊断信息，破坏性工具默认 `dry_run=true`；MCP stdout 专用于 JSON-RPC。

## Python SDK

SDK 通过子进程调用 CLI，不内置 Go 二进制：

```bash
# v1.1.0 发布后：
python -m pip install port-detective

# 或从当前源码安装 SDK：
python -m pip install ./python
```

SDK 与 CLI 应使用同一个经过审核的提交或版本。可执行文件不在 `PATH` 时，设置 `PORT_DETECTIVE_BIN`。Tag 工作流使用 PyPI Trusted Publishing，并在创建对应 GitHub Release 前验证已安装的包；v1.1.0 目前尚未发布。

```python
from port_detective import inspect_port, scan_ports

diagnostic = inspect_port(3000)
occupied = scan_ports([3000, 8080])
```

同步和 asyncio 接口详见 [Python SDK 指南](python/README.md)。

## Node.js 客户端

零依赖 npm 包通过子进程调用 CLI，不内置 CLI 二进制：

```bash
npm install ./npm/port-detective
```

提供同步和 Promise API。释放或清理操作默认 dry-run；确认目标后才设置 `dryRun: false`。
详见 [Node.js 客户端指南](npm/port-detective/README.md)。

## 安全与平台说明

- 清理操作匹配进程的项目路径，不判断进程是否真正成为孤儿进程。
- 使用 `--yes` 或 `dry_run=false` 前，请先查看 `--dry-run` 结果。
- 权限、系统服务、容器、WSL 和沙箱可能限制进程信息读取或终止。
- CLI 发布矩阵覆盖 Windows、macOS、Ubuntu 的 amd64 和 arm64；Python SDK CI 覆盖 Ubuntu、macOS 和 Windows，详见[平台支持政策](docs/operations/PLATFORM_SUPPORT.md)。

## 文档

- [文档中心](docs/README.md)
- [Node.js 客户端指南](npm/port-detective/README.md)
- [贡献指南](docs/community/CONTRIBUTING.md)
- [安全策略](docs/community/SECURITY.md)
- [项目状态](docs/operations/STATUS.md)
- [质量评估](docs/project/QUALITY_REVIEW.md)
- [路线图](docs/project/ROADMAP.md)
- [变更记录](docs/releases/CHANGELOG.md)
- [发布验收清单](docs/releases/RELEASE_CHECKLIST.md)

## 许可证

MIT，见 [LICENSE](LICENSE)。
