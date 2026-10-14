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
```

**编码红线**：
1. `internal/core` 必须保持纯洁，**绝对不能**导入 `cobra` 或 `mcp-go`。
2. 在 `mcp` 模式下，`os.Stdout` 被 JSON-RPC 通信独占，任何日志或报错必须输出到 `os.Stderr`，严禁在 `core` 或 `mcpserver` 中使用 `fmt.Println`。
3. 关键并发逻辑需附带清晰的中文注释（方便课程作业答辩）。

---

## 4. 冻结的核心接口契约 (`internal/core/detector.go`)

后续所有 Agent 必须严格遵守以下数据结构与函数签名，**严禁私自修改字段名或函数签名**：

```go
package core

import "context"

// PortDiagnostic 单个端口与所属进程的体检报告
type PortDiagnostic struct {
	Port        int     `json:"port"`
	IsOpen      bool    `json:"is_open"`
	Protocol    string  `json:"protocol"`     // "tcp"
	PID         int32   `json:"pid"`          // 占用进程 PID（未查到则为 0）
	ProcessName string  `json:"process_name"` // 进程名称，如 "node", "python", "go"
	Cmdline     string  `json:"cmdline"`      // 启动命令行参数
	MemoryMB    float64 `json:"memory_mb"`    // 进程物理内存占用 (MB)
	CPUPercent  float64 `json:"cpu_percent"`  // 进程 CPU 占用率 (%)
	Status      string  `json:"status"`       // 诊断摘要，如 "occupied", "free", "permission_required"
}

// DefaultDevPorts 默认高频开发端口列表（用于并发扫描演示）
var DefaultDevPorts = []int{3000, 3306, 5173, 5432, 6379, 8000, 8080, 8888, 9000, 27017}

// 1. 侦探单端口：查询指定端口是否开启，并通过 gopsutil 关联进程体检数据
func InspectPort(ctx context.Context, port int) (*PortDiagnostic, error)

// 2. 并发扫描多端口（作业核心得分点）：必须使用 goroutine + sync.WaitGroup + channel 并发探测端口列表，并按端口号升序返回结果
func ScanPortsConcurrent(ctx context.Context, ports []int) ([]PortDiagnostic, error)

// 3. 释放端口（安全终止进程）：根据端口找到占用 PID，若 dryRun 为 true 仅返回将要终止的进程信息而不真杀；禁止终止 PID <= 1 的系统进程
func KillPortProcess(ctx context.Context, port int, dryRun bool) (*PortDiagnostic, string, error)
```

---

## 5. CLI 命令与 MCP Tools 规格定义

### A. 终端 CLI 子命令 (`internal/cli/root.go`)

支持全局标志 `--json`（以格式化 JSON 输出，默认输出对齐的终端表格）：
1. `port-detective check <port>`：检查单个端口详情（调用 `InspectPort`）。
2. `port-detective scan [--ports 3000,8080,5432]`：并发扫描指定端口列表，不传参则默认并发扫描 `DefaultDevPorts`（调用 `ScanPortsConcurrent`）。
3. `port-detective kill <port> [--dry-run]`：终止占用该端口的进程（调用 `KillPortProcess`）。
4. `port-detective mcp`：启动 MCP Stdio Server（调用 `mcpserver.RunStdioServer()`）。

### B. MCP AI 工具集 (`internal/mcpserver/server.go`)

注册以下 3 个 MCP Tool，供 AI Agent 直接连接调用：
1. `inspect_port`
   - 参数：`port` (number, 必填, 端口号 1-65535)
   - 描述：`Check if a local port is occupied and get the owning process health metrics (PID, name, memory MB, CPU%, command line).`
2. `scan_dev_ports`
   - 参数：`ports` (string, 选填, 逗号分隔的端口号如 "3000,8080"，为空则扫描常见开发端口)
   - 描述：`Concurrently scan multiple development ports using Go goroutines and return all occupied ports with process diagnostics.`
3. `free_port`
   - 参数：`port` (number, 必填), `dry_run` (boolean, 选填, 默认为 true 以保证安全)
   - 描述：`Terminate the local process occupying the specified port. Set dry_run=false to actually kill the process.`

---

## 6. 多 Agent 步进工作流与 Git 铁律（Read -> Code -> Test -> Update -> Commit）

每次任何 AI Agent 接手任务时，**必须且只能执行 `PROGRESS.md` 中第一个未勾选 `[ ]` 的 Step**：
1. **读取状态**：先读 `AGENTS.md` 和 `PROGRESS.md`，运行 `git status` 和 `git log -n 3 --oneline`。
2. **编码实现**：仅编写当前 Step 范围内的文件，绝不越界写下一步的代码。
3. **强制验证**：运行 `go build ./...` 和 `go test -v ./...`，必须全部通过且无语法错误。
4. **更新看板**：在 `PROGRESS.md` 中将当前 Step 标记为 `[x]`，并在底部的 `Handoff Log` 追加本步完成情况与注意事项。
5. **Git 提交并停机**：使用规范的 Commit Message 提交代码，格式见 `PROGRESS.md`。提交后**立即停止工作**，等待用户指令或下一个 Agent 接手。