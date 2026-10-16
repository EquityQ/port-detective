// Package cli implements the terminal interface for port-detective with Cobra.
//
// Design principles:
//   - Keep output restrained with section titles, aligned tables, and simple separators.
//   - Honor NO_COLOR and disable colors when output is redirected or not a TTY.
//   - Support clean, undecorated JSON output for every subcommand.
//
// This package handles data retrieval and presentation; probes live in internal/core.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"port-detective/internal/core"
	_ "port-detective/internal/fingerprint"
	"port-detective/internal/mcpserver"
	"port-detective/internal/version"
)

// ============================ Output Style Constants ============================

const (
	ruleLine            = "─"
	maxProcessNameWidth = 26
	degradedMarker      = "<no-access>"
	keyWidth            = 10
)

// ============================ Color (NO_COLOR Convention) ============================

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
		title:  "\x1b[38;5;39m",  // Cyan-blue.
		head:   "\x1b[38;5;245m", // Neutral gray.
		dim:    "\x1b[38;5;244m", // Secondary gray.
		accent: "\x1b[38;5;81m",  // Light cyan.
		warn:   "\x1b[38;5;179m", // Amber.
		key:    "\x1b[38;5;250m", // Light gray.
	}
}

func (p palette) paint(code, s string) string {
	if code == "" {
		return s
	}
	return code + s + p.reset
}

// ============================ Rendering Helpers ============================

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

// ============================ Aligned Table Renderer ============================

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

	// Print the table header.
	var headParts []string
	for i, col := range columns {
		formatted := padCell(col.header, colWidths[i], col.align)
		headParts = append(headParts, p.paint(p.head, formatted))
	}
	fmt.Fprintf(w, "  %s\n", strings.Join(headParts, "   "))

	// Print data rows.
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
	port      string
	pid       string
	process   string
	framework string
	project   string
	rss       string
	cpu       string
	status    string
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

	framework := "-"
	project := "-"
	if info.Project != nil {
		if info.Project.Framework != "" {
			framework = info.Project.Framework
		}
		if info.Project.Name != "" {
			project = info.Project.Name
		} else if info.Project.Path != "" {
			project = filepath.Base(info.Project.Path)
		}
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
		port:      strconv.Itoa(info.Port),
		pid:       pid,
		process:   truncate(process, maxProcessNameWidth),
		framework: truncate(framework, 14),
		project:   truncate(project, 18),
		rss:       rss,
		cpu:       cpu,
		status:    info.Status,
	}
}

func renderPortTable(w io.Writer, p palette, infos []core.PortDiagnostic, lang string) {
	headers := []string{"PORT", "PID", "PROCESS", "FRAMEWORK", "PROJECT", "RSS", "CPU", "STATUS"}
	if isZh(lang) {
		headers = []string{"端口", "PID", "进程", "开发框架", "所属项目", "内存", "CPU", "状态"}
	}

	columns := []tableColumn{
		{header: headers[0], align: alignRight},
		{header: headers[1], align: alignRight},
		{header: headers[2], align: alignLeft},
		{header: headers[3], align: alignLeft},
		{header: headers[4], align: alignLeft},
		{header: headers[5], align: alignRight},
		{header: headers[6], align: alignRight},
		{header: headers[7], align: alignLeft},
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
			row.framework,
			row.project,
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

func degradedNote(w io.Writer, p palette, infos []core.PortDiagnostic, lang string) {
	degraded := 0
	for _, info := range infos {
		if info.Status == core.StatusPermissionRequired {
			degraded++
		}
	}
	if degraded == 0 {
		return
	}
	if isZh(lang) {
		hint(w, p, fmt.Sprintf("其中 %d 个端口的归属进程属系统保护进程，当前权限无法读取详情。", degraded))
		fmt.Fprintf(w, "    %s\n", p.paint(p.dim, "以管理员（Windows）或 root（Linux/macOS）身份运行可查看完整信息。"))
	} else {
		hint(w, p, fmt.Sprintf("%d port(s) belong to protected processes inaccessible with current privileges.", degraded))
		fmt.Fprintf(w, "    %s\n", p.paint(p.dim, "Run as Administrator (Windows) or root (Linux/macOS) for full details."))
	}
}

// ============================ Command Construction ============================

type options struct {
	jsonOutput bool
	lang       string
}

func defaultLanguage() string {
	env := strings.ToLower(os.Getenv("LANG") + " " + os.Getenv("LC_ALL") + " " + os.Getenv("LANGUAGE"))
	if strings.Contains(env, "zh") {
		return "zh"
	}
	return "en"
}

func isZh(lang string) bool {
	l := strings.ToLower(strings.TrimSpace(lang))
	return strings.HasPrefix(l, "zh")
}

func NewRootCommand() *cobra.Command {
	opts := &options{}

	root := &cobra.Command{
		Use:     "port-detective",
		Version: version.Value,
		Short:   "本地端口占用侦探与进程体检工具 / Local Port Detective & Process Diagnostic Tool",
		Long: strings.Join([]string{
			"port-detective —— Local Port Detective & Process Diagnostic CLI",
			"",
			"Instantly inspect ports, detect owning processes, identify project frameworks,",
			"and safely clean orphan dev processes.",
			"Native MCP server support for AI Agents (Claude Code / Cursor / Windsurf).",
		}, "\n"),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&opts.lang, "lang", defaultLanguage(), "interface language / 界面语言 (en/zh)")
	root.PersistentFlags().BoolVar(&opts.jsonOutput, "json", false, "output in pure JSON format / 以 JSON 格式输出")

	root.AddCommand(
		newCheckCommand(opts),
		newScanCommand(opts),
		newKillCommand(opts),
		newCleanCommand(opts),
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

// check inspects a single port.
func newCheckCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check <端口号>",
		Short: "体检指定端口：占用状态、归属进程、框架指纹与资源开销 / Inspect specified port",
		Long:  "体检单个端口。除占用状态外，还会给出归属进程的名称、PID、框架指纹、常驻内存与 CPU 占用。",
		Example: strings.Join([]string{
			"  port-detective check 8080",
			"  port-detective check 5432 --json",
			"  port-detective check 3000 --lang en",
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
			renderCheck(out, newPalette(out), info, elapsed, opts.lang)
			return nil
		},
	}
	return cmd
}

func renderCheck(w io.Writer, p palette, info *core.PortDiagnostic, elapsed time.Duration, lang string) {
	const width = 64
	zh := isZh(lang)

	state := p.paint(p.warn, "已占用")
	if !zh {
		state = p.paint(p.warn, "Occupied")
	}
	if !info.IsOpen {
		state = p.paint(p.title, "空闲")
		if !zh {
			state = p.paint(p.title, "Free")
		}
	}

	titleText := fmt.Sprintf("端口体检  TCP/%s", p.paint(p.accent, strconv.Itoa(info.Port)))
	if !zh {
		titleText = fmt.Sprintf("Port Diagnostic  TCP/%s", p.paint(p.accent, strconv.Itoa(info.Port)))
	}
	sectionTitle(w, p, titleText, width)

	connHead := "连接状态"
	statusKey := "状态"
	protoKey := "协议"
	diagKey := "诊断摘要"
	if !zh {
		connHead = "Connection State"
		statusKey = "Status"
		protoKey = "Protocol"
		diagKey = "Summary"
	}
	fmt.Fprintf(w, "  %s\n", p.paint(p.head, connHead))
	field(w, p, statusKey, keyWidth, state)
	field(w, p, protoKey, keyWidth, strings.ToUpper(info.Protocol))
	field(w, p, diagKey, keyWidth, info.Status)

	if !info.IsOpen {
		conclusionKey := "结论"
		conclusionVal := "该端口当前无人监听，可以直接使用。"
		if !zh {
			conclusionKey = "Conclusion"
			conclusionVal = "Port is free and ready for use."
		}
		field(w, p, conclusionKey, keyWidth, conclusionVal)
		fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
		timeSummary := fmt.Sprintf("探测耗时 %s", humanDuration(elapsed))
		if !zh {
			timeSummary = fmt.Sprintf("Elapsed: %s", humanDuration(elapsed))
		}
		summaryLine(w, p, timeSummary)
		return
	}

	procHead := "归属进程"
	procNameKey := "进程名"
	cmdKey := "启动命令"
	rssKey := "常驻内存"
	cpuKey := "CPU 占用"
	frameworkKey := "开发框架"
	projectKey := "所属项目"
	pathKey := "项目路径"
	if !zh {
		procHead = "Owning Process"
		procNameKey = "Process"
		cmdKey = "Command"
		rssKey = "Memory"
		cpuKey = "CPU"
		frameworkKey = "Framework"
		projectKey = "Project"
		pathKey = "Path"
	}

	fmt.Fprintf(w, "\n  %s\n", p.paint(p.head, procHead))
	if info.ProcessName != "" {
		field(w, p, procNameKey, keyWidth, info.ProcessName)
	}
	if info.PID > 0 {
		field(w, p, "PID", keyWidth, p.paint(p.accent, strconv.Itoa(int(info.PID))))
	}
	if info.Project != nil {
		if info.Project.Framework != "" {
			field(w, p, frameworkKey, keyWidth, info.Project.Framework)
		}
		if info.Project.Name != "" {
			field(w, p, projectKey, keyWidth, info.Project.Name)
		}
		if info.Project.Path != "" {
			field(w, p, pathKey, keyWidth, truncate(info.Project.Path, 50))
		}
	}
	if info.Cmdline != "" {
		field(w, p, cmdKey, keyWidth, truncate(info.Cmdline, 96))
	}
	if info.MemoryMB > 0 {
		field(w, p, rssKey, keyWidth, fmt.Sprintf("%.1f MB", info.MemoryMB))
	}
	if info.CPUPercent > 0 {
		field(w, p, cpuKey, keyWidth, fmt.Sprintf("%.1f%%", info.CPUPercent))
	}

	fmt.Fprintf(w, "\n")
	if info.Status == core.StatusPermissionRequired {
		if zh {
			hint(w, p, "该端口归属系统保护进程，当前权限无法读取其完整详情。")
			fmt.Fprintf(w, "    %s\n", p.paint(p.dim, "以管理员（Windows）或 root（Linux/macOS）身份运行可查看完整信息。"))
		} else {
			hint(w, p, "Target process is system-protected. Current permissions insufficient to read details.")
			fmt.Fprintf(w, "    %s\n", p.paint(p.dim, "Run as Administrator (Windows) or root (Linux/macOS) for full details."))
		}
	}

	fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
	timeSummary := fmt.Sprintf("探测耗时 %s", humanDuration(elapsed))
	releaseCmd := fmt.Sprintf("释放命令  port-detective kill %d", info.Port)
	if !zh {
		timeSummary = fmt.Sprintf("Elapsed: %s", humanDuration(elapsed))
		releaseCmd = fmt.Sprintf("Release: port-detective kill %d", info.Port)
	}
	parts := []string{timeSummary}
	if info.PID > 0 {
		parts = append(parts, releaseCmd)
	}
	summaryLine(w, p, parts...)
}

// scan concurrently scans ports, defaulting to DefaultDevPorts.
func newScanCommand(opts *options) *cobra.Command {
	var portsStr string

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "并发扫描并列出本机被占用的端口 / Concurrently scan occupied ports",
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
			"  port-detective scan --lang en",
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
			if !isZh(opts.lang) {
				engine = fmt.Sprintf("Concurrent (%d target ports)", len(targetPorts))
			}
			renderScan(out, newPalette(out), infos, elapsed, engine, opts.lang)
			return nil
		},
	}

	cmd.Flags().StringVar(&portsStr, "ports", "", "指定要并发扫描的端口列表（逗号分隔，如 3000,8080,5432）/ Ports to scan")
	return cmd
}

func renderScan(w io.Writer, p palette, infos []core.PortDiagnostic, elapsed time.Duration, engine string, lang string) {
	const width = 80
	zh := isZh(lang)

	title := "端口扫描  TCP"
	if !zh {
		title = "Port Scanner  TCP"
	}
	sectionTitle(w, p, title, width)

	if len(infos) == 0 {
		emptyMsg := "未发现被占用的端口。"
		engineMsg := fmt.Sprintf("引擎 %s", engine)
		timeMsg := fmt.Sprintf("耗时 %s", humanDuration(elapsed))
		if !zh {
			emptyMsg = "No occupied ports found."
			engineMsg = fmt.Sprintf("Engine: %s", engine)
			timeMsg = fmt.Sprintf("Elapsed: %s", humanDuration(elapsed))
		}
		fmt.Fprintf(w, "  %s\n\n", p.paint(p.dim, emptyMsg))
		summaryLine(w, p, engineMsg, timeMsg)
		fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
		return
	}

	fmt.Fprintf(w, "\n")
	renderPortTable(w, p, infos, lang)
	fmt.Fprintf(w, "\n")

	occupiedMsg := fmt.Sprintf("共 %d 个端口被占用", len(infos))
	engineMsg := fmt.Sprintf("引擎 %s", engine)
	timeMsg := fmt.Sprintf("耗时 %s", humanDuration(elapsed))
	footer := "查看详情  port-detective check <端口>      释放端口  port-detective kill <端口>"
	if !zh {
		occupiedMsg = fmt.Sprintf("%d port(s) occupied", len(infos))
		engineMsg = fmt.Sprintf("Engine: %s", engine)
		timeMsg = fmt.Sprintf("Elapsed: %s", humanDuration(elapsed))
		footer = "Details: port-detective check <port>      Release: port-detective kill <port>"
	}

	summaryLine(w, p, occupiedMsg, engineMsg, timeMsg)
	degradedNote(w, p, infos, lang)

	fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
	fmt.Fprintf(w, "%s\n", p.paint(p.dim, footer))
}

// kill releases a port and supports --dry-run.
func newKillCommand(opts *options) *cobra.Command {
	var (
		dryRun bool
		yes    bool
	)

	cmd := &cobra.Command{
		Use:   "kill <端口号>",
		Short: "释放被指定端口占用的进程 / Terminate process occupying port",
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
			zh := isZh(opts.lang)

			// Inspect the target process before displaying the action.
			diag, err := core.InspectPort(cmd.Context(), port)
			if err != nil {
				return err
			}
			if !diag.IsOpen {
				errMsg := fmt.Sprintf("端口 %d 当前未被占用，无需释放", port)
				if !zh {
					errMsg = fmt.Sprintf("Port %d is currently free, no process to terminate", port)
				}
				return fmt.Errorf("%s", errMsg)
			}

			if !opts.jsonOutput {
				title := fmt.Sprintf("释放端口  %s", p.paint(p.accent, strconv.Itoa(port)))
				if !zh {
					title = fmt.Sprintf("Release Port  %s", p.paint(p.accent, strconv.Itoa(port)))
				}
				if dryRun {
					if zh {
						title += " (预演模式)"
					} else {
						title += " (Dry Run)"
					}
				}
				sectionTitle(out, p, title, 58)
				headTitle := "目标进程信息"
				procKey := "进程名"
				cmdKey := "启动命令"
				rssKey := "物理内存"
				frameworkKey := "开发框架"
				projectKey := "所属项目"
				if !zh {
					headTitle = "Target Process"
					procKey = "Process"
					cmdKey = "Command"
					rssKey = "Memory"
					frameworkKey = "Framework"
					projectKey = "Project"
				}
				fmt.Fprintf(out, "\n  %s\n", p.paint(p.head, headTitle))
				if diag.ProcessName != "" {
					field(out, p, procKey, 8, diag.ProcessName)
				}
				field(out, p, "PID", 8, strconv.Itoa(int(diag.PID)))
				if diag.Project != nil {
					if diag.Project.Framework != "" {
						field(out, p, frameworkKey, 8, diag.Project.Framework)
					}
					if diag.Project.Name != "" {
						field(out, p, projectKey, 8, diag.Project.Name)
					}
				}
				if diag.Cmdline != "" {
					field(out, p, cmdKey, 8, truncate(diag.Cmdline, 96))
				}
				if diag.MemoryMB > 0 {
					field(out, p, rssKey, 8, fmt.Sprintf("%.1f MB", diag.MemoryMB))
				}
				fmt.Fprintf(out, "\n")
			}

			if !dryRun && !yes && !opts.jsonOutput {
				confirmPrompt := fmt.Sprintf("确认结束该进程？%s ", p.paint(p.dim, "[y/N]:"))
				if !zh {
					confirmPrompt = fmt.Sprintf("Terminate this process? %s ", p.paint(p.dim, "[y/N]:"))
				}
				fmt.Fprintf(out, "  %s", confirmPrompt)
				if !confirm(cmd.InOrStdin()) {
					abortMsg := "已取消，未对进程做任何操作。"
					if !zh {
						abortMsg = "Cancelled. Process was not modified."
					}
					fmt.Fprintf(out, "  %s\n", p.paint(p.dim, abortMsg))
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
			if !zh {
				statusText = "Done"
				if dryRun {
					statusText = "DryRun"
				}
			}
			fmt.Fprintf(out, "  %s %s\n", p.paint(p.title, statusText), msg)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "安全预演模式，仅返回将被终止的进程而不执行实际终止 / Preview without killing")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "跳过交互确认，直接执行终止 / Skip confirmation")
	return cmd
}

// clean releases orphan processes associated with the current project.
func newCleanCommand(opts *options) *cobra.Command {
	var (
		dryRun bool
		yes    bool
	)

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "一键清理当前项目占用的开发端口孤儿进程 / Clean orphan processes for the current project",
		Long: strings.Join([]string{
			"并发扫描高频开发端口，仅释放属于当前工作目录项目的孤儿进程。",
			"Concurrently scan development ports and clean up orphan processes belonging to current working directory.",
		}, "\n"),
		Example: strings.Join([]string{
			"  port-detective clean",
			"  port-detective clean --dry-run",
			"  port-detective clean -y",
			"  port-detective clean --json",
		}, "\n"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := stdout(cmd)
			p := newPalette(out)
			zh := isZh(opts.lang)
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("无法获取当前工作目录: %w", err)
			}

			if !dryRun && !yes && !opts.jsonOutput {
				promptMsg := fmt.Sprintf("确认清理当前项目目录 %s 下占用的孤儿进程？", cwd)
				if !zh {
					promptMsg = fmt.Sprintf("Clean orphan processes occupying ports for project in %s?", cwd)
				}
				fmt.Fprintf(out, "  %s %s ", promptMsg, p.paint(p.dim, "[y/N]:"))
				if !confirm(cmd.InOrStdin()) {
					abortMsg := "已取消，未对进程做任何操作。"
					if !zh {
						abortMsg = "Cancelled. No processes were touched."
					}
					fmt.Fprintf(out, "  %s\n", p.paint(p.dim, abortMsg))
					return nil
				}
			}

			cleaned, err := core.CleanCurrentProjectPorts(cmd.Context(), cwd, dryRun)
			if err != nil {
				return err
			}

			if opts.jsonOutput {
				return writeJSON(out, cleaned)
			}

			title := "项目端口清理"
			if !zh {
				title = "Project Ports Clean"
			}
			if dryRun {
				if zh {
					title += " (预演模式)"
				} else {
					title += " (Dry Run)"
				}
			}
			sectionTitle(out, p, title, 80)

			if len(cleaned) == 0 {
				emptyMsg := "当前工作目录项目未发现占用端口的孤儿进程。"
				if !zh {
					emptyMsg = "No orphan processes found for current project."
				}
				fmt.Fprintf(out, "  %s\n\n", p.paint(p.dim, emptyMsg))
				fmt.Fprintf(out, "%s\n", p.paint(p.dim, rule(80)))
				return nil
			}

			fmt.Fprintf(out, "\n")
			renderPortTable(out, p, cleaned, opts.lang)
			fmt.Fprintf(out, "\n")

			summaryMsg := fmt.Sprintf("共处理 %d 个当前项目端口 (预演: %v)", len(cleaned), dryRun)
			if !zh {
				summaryMsg = fmt.Sprintf("Processed %d port(s) for current project (dry-run: %v)", len(cleaned), dryRun)
			}
			summaryLine(out, p, summaryMsg)
			fmt.Fprintf(out, "%s\n", p.paint(p.dim, rule(80)))
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "安全预演模式，仅返回将被清理的进程而不实际杀除 / Preview without killing")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "跳过交互确认直接清理 / Skip confirmation")
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

// mcp starts the MCP Stdio server.
func newMCPCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "以 MCP 协议 Stdio 服务端模式启动，供 AI Agent 调用 / Start MCP Stdio server",
		Long: strings.Join([]string{
			"以 MCP（Model Context Protocol）Stdio 服务端模式启动。",
			"",
			"启动后通过标准输入输出与 AI 客户端通信，注册以下工具：",
			"  inspect_port           体检单个端口，返回占用进程全景体检报告与框架指纹",
			"  scan_dev_ports         并发扫描开发端口（默认覆盖 DefaultDevPorts）",
			"  free_port              释放端口占用进程（默认 dry_run=true 安全预演）",
			"  clean_current_project  一键清理当前项目占用的开发端口孤儿进程",
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
			fmt.Fprintf(os.Stderr, "  已注册工具：inspect_port / scan_dev_ports / free_port / clean_current_project\n")

			ctx, cancel := mcpserver.WaitForSignal(cmd.Context())
			defer cancel()

			if err := mcpserver.RunStdioServer(ctx); err != nil {
				return fmt.Errorf("MCP 服务端异常退出：%w", err)
			}
			return nil
		},
	}
	return cmd
}

// ============================ Argument Parsing ============================

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
