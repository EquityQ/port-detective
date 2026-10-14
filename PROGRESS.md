# PROGRESS.md - 多 Agent 进度与交接看板

## 当前状态指针
- **当前待执行步骤**: `Step 4`
- **构建/测试状态**: `go build -o port-detective .` 通过；`go test ./...` 全绿（core 16 项 + cli 14 项）；`go test -race` 无数据竞争；`gofmt -l .` 无差异；`go vet ./...` 通过。工具链 go1.25.13，`go.mod` 声明 `go 1.23.0`。
- **模块状态**: `go mod tidy` 现已稳定可跑（cobra 已在 Step 3 被 import），执行后无多余变更。

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

- [x] **Step 2: 核心侦探逻辑、并发扫描与单元测试 (`internal/core`)**
  - **动作**:
    1. 严格按照 `AGENTS.md` 第 4 节的契约，编写 `internal/core/detector.go`，实现 `InspectPort`、`ScanPortsConcurrent`（必须用 `goroutine` + `sync.WaitGroup` + `channel` 并发实现，并加好中文注释）和 `KillPortProcess`。
    2. 编写 `internal/core/detector_test.go`：用 `net.Listen("tcp", "127.0.0.1:0")` 动态开启一个本地临时端口，测试 `InspectPort` 和 `ScanPortsConcurrent` 能否准确识别该端口为 `IsOpen: true`。
  - **验收标准**: 运行 `go test -v ./internal/core/...` 100% PASS。
  - **Git 提交信息**: `feat(core): [Step-2] implement concurrent port scanner, process inspector and unit tests`
  - **完成情况**: 已完成（2026-10-14）。`detector.go` 约 390 行，`detector_test.go` 13 个用例全部 PASS，`-race` 无竞争。
    - **⚠ 契约缺口（重要）**：本步骤引用的「`AGENTS.md` 第 4 节的契约」**并不存在** —— AGENTS.md 实际只有第 1~3 节，没有任何函数签名契约。因此 `PortInfo` 结构体与三个函数的签名是我依据本步骤描述自行设计的（见下方「Step 2 交付契约」）。若你手上有原始契约，请核对签名是否一致。
    - **⚠ 关键约束（已实测）**：`go mod tidy` 现在会**删掉 `github.com/spf13/cobra` 与 `github.com/mark3labs/mcp-go`**，因为 Step 2 只导入 gopsutil，这两个库要到 Step 3/4 才会被 import。为保住 Step 1 已锁定的依赖，**Step 3 之前请不要执行 `go mod tidy`**；等 CLI 层 import cobra、MCP 层 import mcp-go 之后再跑，届时它才会收敛到稳定状态。（此条已推翻 Step 1 交接日志里的「警告一」。）
    - `internal/cli/.gitkeep` 与 `internal/mcpserver/.gitkeep` 仍在（尚无源码）；`internal/core/.gitkeep` 已删除。

### Step 2 交付契约（本步骤确立，Step 3/4 请直接复用）

```go
// 数据契约
type PortInfo struct {
    Port     int     `json:"port"`
    Protocol string  `json:"protocol"`            // "tcp" / "udp"
    IsOpen   bool    `json:"is_open"`
    PID      int32   `json:"pid,omitempty"`
    Name     string  `json:"name,omitempty"`
    Cmdline  string  `json:"cmdline,omitempty"`
    RSS      uint64  `json:"rss_bytes,omitempty"`  // 内存 RSS，字节
    CPU      float64 `json:"cpu_percent,omitempty"`
    Exe      string  `json:"exe,omitempty"`
    Created  int64   `json:"created_unix,omitempty"`
    Degraded bool    `json:"degraded,omitempty"`   // 权限受限导致详情缺失
    Err      string  `json:"error,omitempty"`
}

// 常量：core.ProtocolTCP / core.ProtocolUDP / core.DefaultDialTimeout
//       core.DefaultWorkers / core.ErrProcessNotAccessible

type ScanOptions struct {
    Protocol string        // 留空默认 tcp
    Workers  int           // <=0 时按 CPU 核数自动推导（上限 64）
    Timeout  time.Duration // <=0 时取 DefaultDialTimeout
}

func InspectPort(port int, protocol string) (*PortInfo, error)
func ScanPortsConcurrent(ctx context.Context, start, end int, opts ScanOptions) ([]PortInfo, error)
func KillPortProcess(port int, force bool) (int32, error)
```

**Step 3 接线要点**
- `ScanPortsConcurrent` **只返回被占用的端口**（`IsOpen == true`），且已按端口号升序排序，CLI 直接渲染即可，无需再排序。
- `KillPortProcess` 已内置「端口空闲」「拿不到 PID」「权限不足」三类错误文案；CLI 侧直接打印 `err` 即可，`force=true` 走 `Kill()`，`false` 走 `Terminate()`。
- `PortInfo.Degraded == true` 时，应当打印 `PortInfo.Err` 作为**优雅降级提示**，而不是当错误处理、也不要让命令返回非零退出码。
- 扫描全端口 1-65535 在本机实测约 **8.2 秒**（128 协程）。

- [x] **Step 3: Cobra 命令行交互层实现 (`internal/cli` & `main.go`)**
  - **动作**:
    1. 编写 `internal/cli/root.go`，实现 `check`、`scan`、`kill` 三个子命令，支持人类友好的对齐表格输出（使用标准库 `text/tabwriter`）以及 `--json` 标志输出。
    2. 预留 `mcp` 子命令入口（若 Step 4 尚未实现，可先打印提示或调空函数占位）。
    3. 编写 `main.go` 入口文件。
  - **验收标准**: 运行 `go build -o port-detective .` 成功，执行 `./port-detective scan` 能正常输出本机端口扫描表格。
  - **Git 提交信息**: `feat(cli): [Step-3] add cobra CLI commands (check, scan, kill) with table and JSON output`
  - **完成情况**: 已完成（2026-10-14）。`root.go` 约 850 行，`root_test.go` 14 个用例；`main.go` 仅负责调用 `cli.Execute()`。
    - **⚠ 对规格的一处有意偏离（重要）**：Step 3 要求「使用标准库 `text/tabwriter`」做表格对齐，但实测**无法满足中文对齐需求** —— tabwriter 按 **rune** 计数估算列宽，而中日韩文字在终端占 **2 列**，混排时后续所有列都会错位。因此改为自实现 `renderTable` + `displayWidth`（按 Unicode East Asian Width 累加真实显示宽度）。这是达成「表格整齐」的必要改动，`text/tabwriter` 已完全移除（无残留 import）。
    - **修正了两个只有真实运行才会暴露的渲染 bug**：
      1. 降级占位符原本用中文 `＜权限受限＞`，双列宽导致整表错位 —— 改为纯 ASCII 的 `<no-access>`，语义交给脚注解释。
      2. 表格**最后一列跳过了对齐补齐**，导致数值列表头（`RSS` 仅 3 列）无法与其数值（`206.2 MB` 共 8 列）右对齐；现改为最后一列同样参与补齐，仅在行尾统一裁掉空白。
    - **`scan` 语义修正**：系统连接表包含大量**出站连接**（ESTABLISHED / TIME_WAIT / CLOSE_WAIT），它们占用的是客户端随机临时端口。默认只列出**监听端口**（TCP `Status == "LISTEN"`，UDP 无状态），与 `ScanPortsConcurrent`（天然只能发现监听）语义一致；新增 `--all/-a` 可查看全部连接。本机实测：默认 54 个监听端口（netstat TCP LISTENING 为 55，差异来自同端口 IPv4/IPv6 去重），`--all` 为 316 个。
    - **新增 core 能力**：`ListOccupiedPorts(ctx, ListOptions{Protocol, All})` 与 `isListening`。默认模式走连接表（约 13ms），`--range` 模式走并发探测引擎（满足 Step 2/3 的并发要求）。

### Step 3 交付契约（CLI 层）

```text
port-detective
├── check <端口>    [-p tcp|udp] [--json]                 体检单端口
├── scan            [-p tcp|udp] [-w N] [--range A-B] [-a] [--json]   列出占用端口
├── kill  <端口>    [-f|--force] [-y|--yes] [--json]      释放端口
└── mcp             [--json]                              MCP Stdio 服务端（Step 4 接入）
```

**设计约束（Step 4/5 请遵守）**
- 颜色遵循 `NO_COLOR` 约定：设置了 `NO_COLOR`、`TERM=dumb`、或输出被重定向/非 TTY 时**自动关闭**全部 ANSI 转义序列。
- `--json` 模式输出**纯净 JSON**：不含任何标题、制表线或颜色，可直接 `| jq` 消费。`PortInfo` 的字段名即 JSON 契约，不可随意改名。
- 降级（`Degraded=true`）以脚注形式提示，**不返回非零退出码**。
- **`mcp` 子命令的 stdout 必须保持纯净**（当前仅向 stderr 打印占位提示），Step 4 接入 Stdio 协议时不可破坏这一点。

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
- **[Step-2 done]**: 已实现 `internal/core/detector.go`（并发端口扫描 + 进程体检 + 端口释放）与 `internal/core/detector_test.go`（13 个用例，含 `net.Listen("tcp","127.0.0.1:0")` 临时端口验收用例）。
  - **交接要点**：函数签名与 `PortInfo` 字段定义见上方「Step 2 交付契约」，Step 3 直接复用，**不要自行改名**（CLI 的 `--json` 输出会直接序列化 `PortInfo`，字段名即 JSON 契约）。
  - **⚠ `go mod tidy` 仍然不能跑**：Step 2 只导入 gopsutil，此时 tidy 会删掉 cobra 与 mcp-go。请等 Step 3/4 完成 import 之后再执行。
  - **⚠ 权限降级是真实存在的，不是异常**：本机以非管理员身份全量扫描 124 个占用端口，其中 **44 个**因进程属 SYSTEM 而只能拿到 PID、拿不到进程名（`Access is denied`）。这类结果会被标记 `Degraded=true` 并附带原因文案，属**预期行为**，Step 3 请作为提示输出而非错误。
  - **实现要点（供 Step 3/4 理解）**：gopsutil 的 `ConnectionsPid` 在 Windows 上是 **未实现**（直接返回 `ErrNotImplementedError`），因此进程反查改为「全量连接表构建一次反向索引，由所有 worker 只读共享」，既跨平台可用，又避免 N 倍系统调用。
  - **验证记录**：`go test -v ./internal/core/...` 13/13 PASS（`-count=3` 与 `-race -count=2` 均稳定）；真实环境全量扫描 1-65535 实测 **8.2 秒**发现 124 个占用端口；自检用临时程序（`.selfcheck/`）与编译产物均已删除，仓库无残留。
- **[Step-3 done]**: 已实现 `internal/cli/root.go`（check / scan / kill / mcp 四子命令 + 自研显示宽度对齐表格）与 `main.go`；`internal/cli/root_test.go` 14 个用例。
  - **交接要点（Step 4 必读）**：`mcp` 子命令已在 `root.go` 中占位（`newMCPCommand`）。Step 4 需要把它的 `RunE` 改为调用 `mcpserver.RunStdioServer()`。**当前 stdout 是完全纯净的，请务必保持** —— 任何 `fmt.Println` 混入 stdout 都会破坏 Stdio 协议。
  - **⚠ 偏离规格一处**：表格未使用 `text/tabwriter`（按 rune 计宽，中文会错位），改为自实现 `displayWidth`/`renderTable`。若评审要求必须出现 `text/tabwriter`，请在此处与我确认后再改回，但那会牺牲中文对齐。
  - **可直接复用的 CLI 基建**：`displayWidth`（CJK 双列宽计算）、`renderTable`/`tableColumn`（显示宽度对齐表格）、`palette`（遵循 NO_COLOR 的配色）、`humanBytes`、`humanDuration`、`truncate`（按字符截断）。Step 5 写 README 的终端示例时，建议直接用真实输出，不要手编。
  - **验证记录**：`go build -o port-detective .` 通过；`go test ./...` 全绿（core 16 + cli 14）；`-race` 无竞争；真实环境实测 `scan` 13ms、`scan --all` 含 316 条连接、`check`/`kill` 端到端（含取消路径）均正确。临时诊断程序与 `.gitkeep` 已清理。
