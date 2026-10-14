# PROGRESS.md - 多 Agent 进度与交接看板

## 当前状态指针
- **当前待执行步骤**: `Step 2`
- **构建/测试状态**: `go build ./...` 通过（暂无 Go 源文件）；`go mod verify` 全部通过；`gofmt -l .` 无差异。工具链 go1.25.13，`go.mod` 声明 `go 1.23.0`。

---

## 任务路线图 (每次仅允许完成 1 个 Step 并 Commit)

- [x] **Step 1: Go Module 初始化与依赖安装**
  - **动作**:
    1. 初始化 `go mod init port-detective`。
    2. 创建 `.gitignore`（忽略二进制产物 `port-detective`、`*.exe`、`.DS_Store`、`.idea/`、`.vscode/`）。
    3. 创建基础目录 `internal/core`、`internal/cli`、`internal/mcpserver`。
    4. 拉取依赖：`go get github.com/shirou/gopsutil/v3 github.com/spf13/cobra github.com/mark3labs/mcp-go` 并执行 `go mod tidy`。
  - **验收标准**: `go.mod` 与 `go.sum` 生成无误，`git status` 干净。
  - **Git 提交信息**: `chore: [Step-1] init go module and project directory structure`
  - **完成情况**: 已完成（2026-10-14）。
    - 仓库：`git init`（分支 `master`，user `EquityQ <3176811516@qq.com>`）。
    - 依赖版本（均已锁定）：`gopsutil/v3 v3.24.5`、`cobra v1.10.2`、`mcp-go v0.48.0`；`go.sum` 共 69 行，`go mod verify` 输出 `all modules verified`。
    - **关键取舍**：`mcp-go` 当时最新版为 `v1.2.1`，但其 `go.mod` 要求 `go >= 1.25.5`；`v0.49.0` 起同样跳到 `1.25.5`。为守住 AGENTS.md「Go 1.22+」的兼容底线，特选定 **`v0.48.0`**（仅要求 `go 1.23.0`），并使用 Go 1.25.13 工具链完成构建验证。
    - **注意（给下一位 Agent）**：当前仓库还没有任何 `.go` 源文件，因此**此时执行 `go mod tidy` 会把 `require` 块与 `go.sum` 全部剪空**（`warning: "all" matched no packages`），这是 Go 1.17+ 模块图裁剪的正常行为。Step 1 的 `require` 块与 `go.sum` 是用 `go get` + `go mod download all` 固化的；**请等到 Step 2/3 写入真实源码后再跑 `go mod tidy`**，届时它会自动把依赖归位并去掉多余条目。
    - `internal/*` 下暂以 `.gitkeep` 占位，以便 Git 记录空目录；写入真实源码后请删除对应的 `.gitkeep`。

- [ ] **Step 2: 核心侦探逻辑、并发扫描与单元测试 (`internal/core`)**
  - **动作**:
    1. 严格按照 `AGENTS.md` 第 4 节的契约，编写 `internal/core/detector.go`，实现 `InspectPort`、`ScanPortsConcurrent`（必须用 `goroutine` + `sync.WaitGroup` + `channel` 并发实现，并加好中文注释）和 `KillPortProcess`。
    2. 编写 `internal/core/detector_test.go`：用 `net.Listen("tcp", "127.0.0.1:0")` 动态开启一个本地临时端口，测试 `InspectPort` 和 `ScanPortsConcurrent` 能否准确识别该端口为 `IsOpen: true`。
  - **验收标准**: 运行 `go test -v ./internal/core/...` 100% PASS。
  - **Git 提交信息**: `feat(core): [Step-2] implement concurrent port scanner, process inspector and unit tests`

- [ ] **Step 3: Cobra 命令行交互层实现 (`internal/cli` & `main.go`)**
  - **动作**:
    1. 编写 `internal/cli/root.go`，实现 `check`、`scan`、`kill` 三个子命令，支持人类友好的对齐表格输出（使用标准库 `text/tabwriter`）以及 `--json` 标志输出。
    2. 预留 `mcp` 子命令入口（若 Step 4 尚未实现，可先打印提示或调空函数占位）。
    3. 编写 `main.go` 入口文件。
  - **验收标准**: 运行 `go build -o port-detective .` 成功，执行 `./port-detective scan` 能正常输出本机端口扫描表格。
  - **Git 提交信息**: `feat(cli): [Step-3] add cobra CLI commands (check, scan, kill) with table and JSON output`

- [ ] **Step 4: MCP AI 协议服务端接入 (`internal/mcpserver`)**
  - **动作**:
    1. 编写 `internal/mcpserver/server.go`，使用 `mark3labs/mcp-go` 注册 `inspect_port`、`scan_dev_ports`、`free_port` 三个 MCP Tool，内部直接调用 `internal/core` 的方法，返回格式化 JSON 文本。
    2. 在 `internal/cli/root.go` 中将 `port-detective mcp` 子命令正式绑定到 `mcpserver.RunStdioServer()`。
  - **验收标准**: `go build ./...` 与 `go test ./...` 全部通过，确保 `mcp` 启动路径上没有任何污染 `os.Stdout` 的打印语句。
  - **Git 提交信息**: `feat(mcp): [Step-4] implement MCP stdio server and register AI port detective tools`

- [ ] **Step 5: 课程作业文档、架构图与验收报告 (`README.md`)**
  - **动作**:
    1. 编写高质量中文 `README.md`，必须包含：
       - 项目背景与痛点介绍
       - 系统三层架构图（使用 Mermaid 语法绘制：CLI / MCP 双模接入 -> Core 并发引擎 -> OS 底层探测）
       - Go 语言并发设计亮点解析（展示 `ScanPortsConcurrent` 的协程池设计思路，方便直接写入课程报告）
       - CLI 终端运行示例与输出截图文本
       - AI 客户端（Claude Code / Cursor 等）连接本项目的 MCP JSON 配置示例
  - **验收标准**: 文档排版清晰，全项目 `go test -v ./...` 绿灯通过，删除临时编译二进制文件保持仓库整洁。
  - **Git 提交信息**: `docs: [Step-5] add comprehensive assignment README with Mermaid architecture and MCP config`

---

## Handoff Log (跨 Agent 交接日志)
> 规则：每位完成当前 Step 的 Agent 必须在此处留下一行交接记录（说明已完成内容及下一任 Agent 注意事项）。

- **[Init]**: 项目规范 `AGENTS.md` 与进度表 `PROGRESS.md` 已就绪，等待第一位 Agent 执行 `Step 1`。
- **[Step-1 done]**: 已初始化 Go module `port-detective`（`go.mod` 声明 `go 1.23.0`），锁定三大依赖 `gopsutil/v3 v3.24.5` / `cobra v1.10.2` / `mcp-go v0.48.0`，生成 `.gitignore` 与 `internal/core|cli|mcpserver` 目录，`go mod verify` 全部通过。
  - **警告一（最重要）**：仓库当前**没有任何 `.go` 文件**，此时跑 `go mod tidy` 会清空 `require` 与 `go.sum`。请在 Step 2 写入 `detector.go` 之后再执行 `go mod tidy`。
  - **警告二**：`mcp-go` 不要升级到 `v0.49.0` 及以上（会要求 `go >= 1.25.5`，破坏「Go 1.22+」兼容目标）。
  - **警告三**：`internal/*/.gitkeep` 是空目录占位符，Step 2/3 写入真实源码后请删除。
  - **环境说明**：本机原先未安装 Go，已部署便携工具链至 `F:\AI Workspace\.tools\go`（go1.25.13）。该目录在工作区之外，不影响仓库；构建命令前置 `$env:PATH = "F:\AI Workspace\.tools\go\bin;$env:PATH"` 即可使用。另需 `HTTPS_PROXY=http://127.0.0.1:10808` 才能访问 Go 模块代理。
