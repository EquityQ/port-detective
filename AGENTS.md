# AGENTS.md - `port-detective` 多 Agent 协同开发宪章

## 1. 项目概述与作业目标
本项目 `port-detective` 是一个基于 Go 语言开发的轻量级、跨平台“本地端口占用侦探与进程体检 CLI 工具”，同时原生支持 **MCP (Model Context Protocol)** 协议供 AI Agent 直接调用。
- **核心目标**：代码精简规范（总代码量约 400~500 行）、突出 Go 语言并发编程特性 (`goroutine` + `channel` + `WaitGroup`)、架构分层清晰、包含单元测试、Git 提交记录工整。
- **Go Module 名称**：`port-detective` (Go 1.22+)

---

## 2. 强制技术栈与第三方依赖（严禁随意更换）
为保证跨平台（Windows / macOS / Linux）编译与运行 100% 成功，仅允许引入以下 3 个轻量级库：
1. **系统进程与端口探测**：`github.com/shirou/gopsutil/v3`（使用 `net` 和 `process` 子包获取端口对应的 PID、进程名、内存 RSS、CPU 占用及启动命令；若无权限获取系统级映射，需优雅降级提示）。
2. **CLI 命令行框架**：`github.com/spf13/cobra`
3. **MCP 协议服务端**：`github.com/mark3labs/mcp-go`（使用 `mcp` 与 `server` 包实现 Stdio 传输协议）。

---

## 3. 严格目录结构与模块化边界
```text
port-detective/
├── AGENTS.md                  # 本开发宪章（只读，严禁删除）
├── PROGRESS.md                # 多 Agent 状态交接看板（每完成一步必须更新）
├── main.go                    # 唯一入口，仅负责调用 cli.Execute()
├── internal/
│   ├── core/
│   │   ├── detector.go        # 核心业务逻辑与并发端口扫描实现（严禁依赖 cobra 或 mcp-go）
│   │   └── detector_test.go   # 核心逻辑单元测试（使用 Go 标准库 net.Listen 模拟临时端口测试）
│   ├── cli/
│   │   └── root.go            # Cobra 终端命令实现（check / scan / kill / mcp）
│   └── mcpserver/
│       └── server.go          # MCP Stdio Server 实现，将 core 能力封装为 AI Tools
├── go.mod
└── README.md                  # 课程作业说明文档、架构图与演示指南