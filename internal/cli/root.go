// Package cli 实现 port-detective 的终端交互层（Cobra）。
//
// 设计原则：
//   - 视觉克制：只使用分组标题、对齐表格与细线分隔，不引入边框艺术字或动画；
//   - 尊重环境：遵循 NO_COLOR 约定，输出重定向或非 TTY 时自动关闭颜色；
//   - 机器友好：所有子命令均支持 --json，输出为纯净 JSON，不含任何装饰。
//
// 本包只做「取数 + 呈现」，全部探测能力来自 internal/core。
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"port-detective/internal/core"
	"port-detective/internal/mcpserver"
)

// ============================ 输出样式常量 ============================

const (
	ruleLine            = "─"
	maxProcessNameWidth = 26
	degradedMarker      = "<no-access>"
	keyWidth            = 10
)

// ============================ 颜色（遵循 NO_COLOR 约定） ============================

type palette struct {
	reset  string
	title  string
	head   string
	dim    string
	accent string
	warn   string
	key    string
}

func newPalette(out io.Writer) palette {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return palette{}
	}
	if f, ok := out.(*os.File); ok {
		if info, err := f.Stat(); err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return palette{}
		}
	} else {
		return palette{}
	}
	return palette{
		reset:  "\x1b[0m",
		title:  "\x1b[38;5;39m",  // 青蓝色
		head:   "\x1b[38;5;245m", // 中性灰
		dim:    "\x1b[38;5;244m", // 次要信息灰
		accent: "\x1b[38;5;81m",  // 浅青
		warn:   "\x1b[38;5;179m", // 琥珀色
		key:    "\x1b[38;5;250m", // 浅灰
	}
}

func (p palette) paint(code, s string) string {
	if code == "" {
		return s
	}
	return code + s + p.reset
}

// ============================ 渲染工具 ============================

func rule(width int) string {
	if width <= 0 {
		width = 60
	}
	return strings.Repeat(ruleLine, width)
}

func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%.0fms", float64(d.Microseconds())/1000)
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

func sectionTitle(w io.Writer, p palette, title string, width int) {
	fmt.Fprintf(w, "%s\n", p.paint(p.title, title))
	fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
}

func field(w io.Writer, p palette, key string, keyWidth int, value string) {
	fmt.Fprintf(w, "  %s %s %s\n",
		p.paint(p.key, padRight(key, keyWidth)),
		p.paint(p.dim, ":"),
		value)
}

func hint(w io.Writer, p palette, text string) {
	fmt.Fprintf(w, "  %s %s\n", p.paint(p.warn, "!"), p.paint(p.dim, text))
}

func stdout(cmd *cobra.Command) io.Writer {
	if out := cmd.OutOrStdout(); out != nil {
		return out
	}
	return os.Stdout
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// ============================ 对齐渲染引擎 ============================

func displayWidth(s string) int {
	width := 0
	for _, r := range s {
		switch {
		case r == 0:
		case r < 0x20 || (r >= 0x7f && r < 0xa0):
		case isWideRune(r):
			width += 2
		default:
			width++
		}
	}
	return width
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F:
		return true
	case r >= 0x2E80 && r <= 0x303E:
		return true
	case r >= 0x3041 && r <= 0x33FF:
		return true
	case r >= 0x3400 && r <= 0x4DBF:
		return true
	case r >= 0x4E00 && r <= 0x9FFF:
		return true
	case r >= 0xA000 && r <= 0xA4CF:
		return true
	case r >= 0xAC00 && r <= 0xD7A3:
		return true
	case r >= 0xF900 && r <= 0xFAFF:
		return true
	case r >= 0xFE10 && r <= 0xFE19:
		return true
	case r >= 0xFE30 && r <= 0xFE6F:
		return true
	case r >= 0xFF00 && r <= 0xFF60:
		return true
	case r >= 0xFFE0 && r <= 0xFFE6:
		return true
	case r >= 0x1F300 && r <= 0x1F64F:
		return true
	case r >= 0x1F900 && r <= 0x1F9FF:
		return true
	case r >= 0x20000 && r <= 0x3FFFD:
		return true
	}
	return false
}

func padRight(s string, width int) string {
	if gap := width - displayWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

func padLeft(s string, width int) string {
	if gap := width - displayWidth(s); gap > 0 {
		return strings.Repeat(" ", gap) + s
	}
	return s
}

type alignment int

const (
	alignLeft alignment = iota
	alignRight
)

type tableColumn struct {
	header string
	align  alignment
}

func renderTable(w io.Writer, p palette, columns []tableColumn, rows [][]string) {
	if len(columns) == 0 {
		return
	}

	colWidths := make([]int, len(columns))
	for i, col := range columns {
		colWidths[i] = displayWidth(col.header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) {
				clean := stripANSI(cell)
				if w := displayWidth(clean); w > colWidths[i] {
					colWidths[i] = w
				}
			}
		}
	}

	// 打印表头
	var headParts []string
	for i, col := range columns {
		formatted := padCell(col.header, colWidths[i], col.align)
		headParts = append(headParts, p.paint(p.head, formatted))
	}
	fmt.Fprintf(w, "  %s\n", strings.Join(headParts, "   "))

	// 打印数据行
	for _, row := range rows {
		var rowParts []string
		for i, col := range columns {
			var cell string
			if i < len(row) {
				cell = row[i]
			}
			clean := stripANSI(cell)
			visibleLen := displayWidth(clean)
			gap := colWidths[i] - visibleLen
			if gap < 0 {
				gap = 0
			}

			var formatted string
			if col.align == alignRight {
				formatted = strings.Repeat(" ", gap) + cell
			} else {
				formatted = cell + strings.Repeat(" ", gap)
			}
			rowParts = append(rowParts, formatted)
		}
		line := strings.TrimRight("  "+strings.Join(rowParts, "   "), " ")
		fmt.Fprintf(w, "%s\n", line)
	}
}

func padCell(s string, width int, align alignment) string {
	if align == alignRight {
		return padLeft(s, width)
	}
	return padRight(s, width)
}

func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b[") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	inEsc := false
	for i := 0; i < len(s); i++ {
		if !inEsc && s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			inEsc = true
			i++
			continue
		}
		if inEsc {
			if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
				inEsc = false
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

type portRow struct {
	port    string
	pid     string
	process string
	rss     string
	cpu     string
	status  string
}

func buildRow(info core.PortDiagnostic) portRow {
	process := info.ProcessName
	if process == "" {
		if info.Status == core.StatusPermissionRequired {
			process = degradedMarker
		} else {
			process = "-"
		}
	}

	pid := "-"
	if info.PID > 0 {
		pid = strconv.Itoa(int(info.PID))
	}

	rss := "-"
	if info.MemoryMB > 0 {
		rss = fmt.Sprintf("%.1f MB", info.MemoryMB)
	}

	cpu := "-"
	if info.CPUPercent > 0 {
		cpu = fmt.Sprintf("%.1f%%", info.CPUPercent)
	}

	return portRow{
		port:    strconv.Itoa(info.Port),
		pid:     pid,
		process: truncate(process, maxProcessNameWidth),
		rss:     rss,
		cpu:     cpu,
		status:  info.Status,
	}
}

func renderPortTable(w io.Writer, p palette, infos []core.PortDiagnostic) {
	columns := []tableColumn{
		{header: "PORT", align: alignRight},
		{header: "PID", align: alignRight},
		{header: "PROCESS", align: alignLeft},
		{header: "RSS", align: alignRight},
		{header: "CPU", align: alignRight},
		{header: "STATUS", align: alignLeft},
	}

	rows := make([][]string, 0, len(infos))
	for _, info := range infos {
		row := buildRow(info)
		process := row.process
		if strings.HasPrefix(process, "<") {
			process = p.paint(p.warn, process)
		}

		rows = append(rows, []string{
			p.paint(p.accent, row.port),
			row.pid,
			process,
			row.rss,
			row.cpu,
			p.paint(p.dim, row.status),
		})
	}
	renderTable(w, p, columns, rows)
}

func summaryLine(w io.Writer, p palette, parts ...string) {
	fmt.Fprintf(w, "  %s\n", p.paint(p.dim, strings.Join(parts, "  ·  ")))
}

func degradedNote(w io.Writer, p palette, infos []core.PortDiagnostic) {
	degraded := 0
	for _, info := range infos {
		if info.Status == core.StatusPermissionRequired {
			degraded++
		}
	}
	if degraded == 0 {
		return
	}
	hint(w, p, fmt.Sprintf("其中 %d 个端口的归属进程属系统保护进程，当前权限无法读取详情。", degraded))
	fmt.Fprintf(w, "    %s\n", p.paint(p.dim, "以管理员（Windows）或 root（Linux/macOS）身份运行可查看完整信息。"))
}

// ============================ 命令构建 ============================

type options struct {
	jsonOutput bool
}

func NewRootCommand() *cobra.Command {
	opts := &options{}

	root := &cobra.Command{
		Use:   "port-detective",
		Short: "本地端口占用侦探与进程体检工具",
		Long: strings.Join([]string{
			"port-detective —— 本地端口占用侦探与进程体检 CLI",
			"",
			"一句话定位端口归属：谁占了我的端口、它占了多少内存、能不能安全释放。",
			"同时原生支持 MCP 协议，可作为 AI Agent 的端口探测工具直接调用。",
		}, "\n"),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		newCheckCommand(opts),
		newScanCommand(opts),
		newKillCommand(opts),
		newMCPCommand(opts),
	)
	return root
}

func Execute() {
	root := NewRootCommand()
	if err := root.Execute(); err != nil {
		p := newPalette(os.Stderr)
		fmt.Fprintf(os.Stderr, "%s %v\n", p.paint(p.warn, "错误:"), err)
		os.Exit(1)
	}
}

// 1. check 子命令：体检单端口
func newCheckCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check <端口号>",
		Short: "体检指定端口：占用状态、归属进程与资源开销",
		Long:  "体检单个端口。除占用状态外，还会给出归属进程的名称、PID、常驻内存与 CPU 占用。",
		Example: strings.Join([]string{
			"  port-detective check 8080",
			"  port-detective check 5432 --json",
		}, "\n"),
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("需要且仅需要一个端口号，例如：port-detective check 8080")
			}
			if _, err := parsePort(args[0]); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			port, _ := parsePort(args[0])
			out := stdout(cmd)

			start := time.Now()
			info, err := core.InspectPort(cmd.Context(), port)
			if err != nil {
				return err
			}
			elapsed := time.Since(start)

			if opts.jsonOutput {
				return writeJSON(out, info)
			}
			renderCheck(out, newPalette(out), info, elapsed)
			return nil
		},
	}

	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "以 JSON 格式输出，便于脚本消费")
	return cmd
}

func renderCheck(w io.Writer, p palette, info *core.PortDiagnostic, elapsed time.Duration) {
	const width = 58

	state := p.paint(p.warn, "已占用")
	if !info.IsOpen {
		state = p.paint(p.title, "空闲")
	}
	sectionTitle(w, p, fmt.Sprintf("端口体检  TCP/%s",
		p.paint(p.accent, strconv.Itoa(info.Port))), width)

	fmt.Fprintf(w, "  %s\n", p.paint(p.head, "连接状态"))
	field(w, p, "状态", keyWidth, state)
	field(w, p, "协议", keyWidth, strings.ToUpper(info.Protocol))
	field(w, p, "诊断摘要", keyWidth, info.Status)

	if !info.IsOpen {
		field(w, p, "结论", keyWidth, "该端口当前无人监听，可以直接使用。")
		fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
		summaryLine(w, p, fmt.Sprintf("探测耗时 %s", humanDuration(elapsed)))
		return
	}

	fmt.Fprintf(w, "\n  %s\n", p.paint(p.head, "归属进程"))
	if info.ProcessName != "" {
		field(w, p, "进程名", keyWidth, info.ProcessName)
	}
	if info.PID > 0 {
		field(w, p, "PID", keyWidth, p.paint(p.accent, strconv.Itoa(int(info.PID))))
	}
	if info.Cmdline != "" {
		field(w, p, "启动命令", keyWidth, truncate(info.Cmdline, 96))
	}
	if info.MemoryMB > 0 {
		field(w, p, "常驻内存", keyWidth, fmt.Sprintf("%.1f MB", info.MemoryMB))
	}
	if info.CPUPercent > 0 {
		field(w, p, "CPU 占用", keyWidth, fmt.Sprintf("%.1f%%", info.CPUPercent))
	}

	fmt.Fprintf(w, "\n")
	if info.Status == core.StatusPermissionRequired {
		hint(w, p, "该端口归属系统保护进程，当前权限无法读取其完整详情。")
		fmt.Fprintf(w, "    %s\n", p.paint(p.dim, "以管理员（Windows）或 root（Linux/macOS）身份运行可查看完整信息。"))
	}

	fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
	parts := []string{fmt.Sprintf("探测耗时 %s", humanDuration(elapsed))}
	if info.PID > 0 {
		parts = append(parts, fmt.Sprintf("释放命令  port-detective kill %d", info.Port))
	}
	summaryLine(w, p, parts...)
}

// 2. scan 子命令：并发扫描端口列表，默认扫描 DefaultDevPorts
func newScanCommand(opts *options) *cobra.Command {
	var portsStr string

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "并发扫描并列出本机被占用的端口",
		Long: strings.Join([]string{
			"并发扫描本机被占用的端口。",
			"",
			"默认使用 goroutine 协程池并发扫描 DefaultDevPorts 高频开发端口（3000, 3306, 5173, 5432, 6379, 8000, 8080, 8888, 9000, 27017）；",
			"亦可通过 --ports 指定自定义端口列表（如 --ports 3000,8080,5432）。",
		}, "\n"),
		Example: strings.Join([]string{
			"  port-detective scan",
			"  port-detective scan --ports 3000,8080,5432",
			"  port-detective scan --json",
		}, "\n"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := stdout(cmd)

			var targetPorts []int
			if portsStr != "" {
				parsed, err := parsePortsList(portsStr)
				if err != nil {
					return err
				}
				targetPorts = parsed
			} else {
				targetPorts = core.DefaultDevPorts
			}

			begin := time.Now()
			infos, err := core.ScanPortsConcurrent(cmd.Context(), targetPorts)
			elapsed := time.Since(begin)
			if err != nil {
				return err
			}

			if opts.jsonOutput {
				return writeJSON(out, infos)
			}
			engine := fmt.Sprintf("并发扫描 (%d 个目标端口)", len(targetPorts))
			renderScan(out, newPalette(out), infos, elapsed, engine)
			return nil
		},
	}

	cmd.Flags().StringVar(&portsStr, "ports", "", "指定要并发扫描的端口列表（逗号分隔，如 3000,8080,5432）")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "以 JSON 格式输出，便于脚本消费")
	return cmd
}

func renderScan(w io.Writer, p palette, infos []core.PortDiagnostic, elapsed time.Duration, engine string) {
	const width = 64
	sectionTitle(w, p, "端口扫描  TCP", width)

	if len(infos) == 0 {
		fmt.Fprintf(w, "  %s\n\n", p.paint(p.dim, "未发现被占用的端口。"))
		summaryLine(w, p, fmt.Sprintf("引擎 %s", engine), fmt.Sprintf("耗时 %s", humanDuration(elapsed)))
		fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
		return
	}

	fmt.Fprintf(w, "\n")
	renderPortTable(w, p, infos)
	fmt.Fprintf(w, "\n")

	summaryLine(w, p,
		fmt.Sprintf("共 %d 个端口被占用", len(infos)),
		fmt.Sprintf("引擎 %s", engine),
		fmt.Sprintf("耗时 %s", humanDuration(elapsed)))
	degradedNote(w, p, infos)

	fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
	fmt.Fprintf(w, "%s\n", p.paint(p.dim, "查看详情  port-detective check <端口>      释放端口  port-detective kill <端口>"))
}

// 3. kill 子命令：释放指定端口占用，支持 --dry-run
func newKillCommand(opts *options) *cobra.Command {
	var (
		dryRun bool
		yes    bool
	)

	cmd := &cobra.Command{
		Use:   "kill <端口号>",
		Short: "释放被指定端口占用的进程",
		Long: strings.Join([]string{
			"结束占用指定端口的进程，从而释放该端口。",
			"",
			"支持 --dry-run 安全预演模式，仅报告将要终止的进程而不做实际操作；",
			"默认模式下会先展示目标进程详情并请求交互确认，加 -y 可跳过确认。",
		}, "\n"),
		Example: strings.Join([]string{
			"  port-detective kill 8080",
			"  port-detective kill 8080 --dry-run",
			"  port-detective kill 8080 -y",
		}, "\n"),
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("需要且仅需要一个端口号，例如：port-detective kill 8080")
			}
			if _, err := parsePort(args[0]); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			port, _ := parsePort(args[0])
			out := stdout(cmd)
			p := newPalette(out)

			// 预先体检以展示目标进程
			diag, err := core.InspectPort(cmd.Context(), port)
			if err != nil {
				return err
			}
			if !diag.IsOpen {
				return fmt.Errorf("端口 %d 当前未被占用，无需释放", port)
			}

			if !opts.jsonOutput {
				title := fmt.Sprintf("释放端口  %s", p.paint(p.accent, strconv.Itoa(port)))
				if dryRun {
					title += " (预演模式)"
				}
				sectionTitle(out, p, title, 58)
				fmt.Fprintf(out, "\n  %s\n", p.paint(p.head, "目标进程信息"))
				if diag.ProcessName != "" {
					field(out, p, "进程名", 8, diag.ProcessName)
				}
				field(out, p, "PID", 8, strconv.Itoa(int(diag.PID)))
				if diag.Cmdline != "" {
					field(out, p, "启动命令", 8, truncate(diag.Cmdline, 96))
				}
				if diag.MemoryMB > 0 {
					field(out, p, "物理内存", 8, fmt.Sprintf("%.1f MB", diag.MemoryMB))
				}
				fmt.Fprintf(out, "\n")
			}

			if !dryRun && !yes && !opts.jsonOutput {
				fmt.Fprintf(out, "  确认结束该进程？%s ", p.paint(p.dim, "[y/N]:"))
				if !confirm(cmd.InOrStdin()) {
					fmt.Fprintf(out, "  %s\n", p.paint(p.dim, "已取消，未对进程做任何操作。"))
					return nil
				}
			}

			resultDiag, msg, err := core.KillPortProcess(cmd.Context(), port, dryRun)
			if err != nil {
				return err
			}

			if opts.jsonOutput {
				return writeJSON(out, map[string]any{
					"port":       port,
					"dry_run":    dryRun,
					"diagnostic": resultDiag,
					"message":    msg,
				})
			}

			statusText := "完成"
			if dryRun {
				statusText = "预演"
			}
			fmt.Fprintf(out, "  %s %s\n", p.paint(p.title, statusText), msg)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "安全预演模式，仅返回将被终止的进程而不执行实际终止")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "跳过交互确认，直接执行终止")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "以 JSON 格式输出，便于脚本消费")
	return cmd
}

func confirm(in io.Reader) bool {
	var answer string
	if _, err := fmt.Fscanln(in, &answer); err != nil {
		return false
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return strings.HasPrefix(answer, "y")
}

// 4. mcp 子命令：启动 MCP Stdio 服务端
func newMCPCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "以 MCP 协议 Stdio 服务端模式启动，供 AI Agent 调用",
		Long: strings.Join([]string{
			"以 MCP（Model Context Protocol）Stdio 服务端模式启动。",
			"",
			"启动后通过标准输入输出与 AI 客户端通信，注册以下工具：",
			"  inspect_port     体检单个端口，返回占用进程全景体检报告",
			"  scan_dev_ports   并发扫描开发端口（默认覆盖 DefaultDevPorts）",
			"  free_port        释放端口占用进程（默认 dry_run=true 安全预演）",
			"",
			"注意：Stdio 模式下标准输出专供协议通信，诊断信息一律写入标准错误。",
		}, "\n"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.jsonOutput {
				return writeJSON(stdout(cmd), mcpserver.Describe())
			}

			perr := newPalette(os.Stderr)
			fmt.Fprintf(os.Stderr, "%s port-detective 已启动（MCP Stdio），等待客户端连接…\n",
				perr.paint(perr.dim, "·"))
			fmt.Fprintf(os.Stderr, "  已注册工具：inspect_port / scan_dev_ports / free_port\n")

			ctx, cancel := mcpserver.WaitForSignal(cmd.Context())
			defer cancel()

			if err := mcpserver.RunStdioServer(ctx); err != nil {
				return fmt.Errorf("MCP 服务端异常退出：%w", err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "仅打印服务端能力清单后退出（自检用）")
	return cmd
}

// ============================ 参数解析 ============================

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("端口号 %q 不是合法数字", s)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("端口号 %d 超出有效范围 1-65535", port)
	}
	return port, nil
}

func parsePortsList(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	ports := make([]int, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		port, err := parsePort(trimmed)
		if err != nil {
			return nil, err
		}
		ports = append(ports, port)
	}
	if len(ports) == 0 {
		return nil, fmt.Errorf("未指定任何有效端口号")
	}
	return ports, nil
}
