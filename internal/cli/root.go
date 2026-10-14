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
	// 细横线：用于分组标题下方的分隔线，视觉上比全边框更克制。
	ruleLine = "─"

	// 进程名最长展示宽度，超出则省略，避免撑破表格对齐。
	maxProcessNameWidth = 26

	// degradedMarker 是「端口有主、但当前权限读不到进程名」时的占位符。
	// 刻意使用纯 ASCII：中日韩占位符属于双列宽字符，会破坏表格列对齐，
	// 且宽字符在部分终端字体下宽度不稳定。语义由脚注统一解释。
	degradedMarker = "<no-access>"

	// keyWidth 是 check 报告中键名（含中文）的对齐宽度（显示列数）。
	// 「可执行文件」为 5 个汉字，占 10 列，故取 10。
	keyWidth = 10
)

// ============================ 颜色（遵循 NO_COLOR 约定） ============================

// palette 保存各个语义位置使用的 ANSI 颜色码。
// 颜色为空字符串时即表示「不输出颜色」，所有打印函数都无需再做判断。
type palette struct {
	reset  string
	title  string // 分组标题：品牌色
	head   string // 表头
	dim    string // 次要信息、脚注
	accent string // 端口号等关键值
	warn   string // 降级提示
	key    string // 键值对的键名
}

// newPalette 根据目标输出流的实际能力决定是否启用颜色。
//
// 关闭颜色的情形（任一命中即关闭）：
//   - 设置了 NO_COLOR 环境变量（https://no-color.org 约定）；
//   - TERM=dumb；
//   - 输出被重定向到文件或管道（非终端）。
//
// 这样既能让 `tool | grep` 拿到干净文本，也不会在 CI 日志里留下转义序列。
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
		title:  "\x1b[38;5;39m",  // 清亮的青蓝色，作为品牌主色
		head:   "\x1b[38;5;245m", // 中性灰，弱化表头
		dim:    "\x1b[38;5;244m", // 灰，用于脚注与单位
		accent: "\x1b[38;5;81m",  // 浅青，用于端口号
		warn:   "\x1b[38;5;179m", // 温和的琥珀色，用于降级提示
		key:    "\x1b[38;5;250m", // 浅灰，用于键值对的键
	}
}

// paint 在启用颜色时包裹 ANSI 码，否则原样返回。
func (p palette) paint(code, s string) string {
	if code == "" {
		return s
	}
	return code + s + p.reset
}

// ============================ 渲染工具 ============================

// rule 生成一条指定宽度的细横线。
func rule(width int) string {
	if width <= 0 {
		width = 60
	}
	return strings.Repeat(ruleLine, width)
}

// truncate 按显示宽度截断字符串，超长时以省略号收尾。
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

// humanBytes 把字节数格式化为便于阅读的单位。
// 采用 1024 进制与单字母单位（B/K/M/G），比 "MiB" 更紧凑，且在等宽字体下对齐更整齐。
func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// humanDuration 把耗时格式化为紧凑形式。
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

// sectionTitle 打印一个分组标题及其下方的细线。
func sectionTitle(w io.Writer, p palette, title string, width int) {
	fmt.Fprintf(w, "%s\n", p.paint(p.title, title))
	fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
}

// field 打印一行「键 : 值」。
//
// 键名按**显示宽度**补齐到 keyWidth 列，而不是用 fmt 的 %-*s：
// 后者按 rune 计数，遇到「进程名」「启动时间」这类中文键名会算少宽度，
// 导致冒号无法对齐。
func field(w io.Writer, p palette, key string, keyWidth int, value string) {
	fmt.Fprintf(w, "  %s %s %s\n",
		p.paint(p.key, padRight(key, keyWidth)),
		p.paint(p.dim, ":"),
		value)
}

// hint 打印一条缩进的脚注（用于优雅降级提示等）。
func hint(w io.Writer, p palette, text string) {
	fmt.Fprintf(w, "  %s %s\n", p.paint(p.warn, "!"), p.paint(p.dim, text))
}

// ============================ 输出目标 ============================

// stdout 返回命令的标准输出；测试中可通过 SetOut 注入缓冲区。
func stdout(cmd *cobra.Command) io.Writer {
	if out := cmd.OutOrStdout(); out != nil {
		return out
	}
	return os.Stdout
}

// writeJSON 以缩进 JSON 输出任意值。JSON 模式下面板装饰一律不输出。
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false) // 中文与路径保持原样，便于阅读
	return enc.Encode(v)
}

// newTabWriter 已不再使用：标准库 text/tabwriter 按 rune 计数估算列宽，
// 无法正确处理双列宽的中日韩文字，故改由 renderTable 显式测量显示宽度。
// 详见 renderTable 的注释说明。

// ============================ 对齐渲染引擎 ============================

// displayWidth 计算字符串在等宽终端中占用的列数。
//
// 关键点：中日韩文字（CJK）与全角标点在终端中占 **2 列**，
// 而 Go 的 len() 按字节、[]rune 按字符计数，两者都会导致列对齐错位。
// 因此这里按 Unicode 区段累加真实的显示宽度。
func displayWidth(s string) int {
	width := 0
	for _, r := range s {
		switch {
		case r == 0:
			// 空字符，忽略
		case r < 0x20 || (r >= 0x7f && r < 0xa0):
			// 控制字符不占列
		case isWideRune(r):
			width += 2
		default:
			width++
		}
	}
	return width
}

// isWideRune 判断一个字符是否属于「双列宽」区段。
// 覆盖范围参考 Unicode East Asian Width 属性中的 W（Wide）与 F（Fullwidth）。
func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F: // 谚文字母
		return true
	case r >= 0x2E80 && r <= 0x303E: // CJK 部首、康熙部首、CJK 符号标点
		return true
	case r >= 0x3041 && r <= 0x33FF: // 平假名、片假名、注音、CJK 兼容
		return true
	case r >= 0x3400 && r <= 0x4DBF: // CJK 扩展 A
		return true
	case r >= 0x4E00 && r <= 0x9FFF: // CJK 统一表意文字
		return true
	case r >= 0xA000 && r <= 0xA4CF: // 彝文
		return true
	case r >= 0xAC00 && r <= 0xD7A3: // 谚文音节
		return true
	case r >= 0xF900 && r <= 0xFAFF: // CJK 兼容表意文字
		return true
	case r >= 0xFE10 && r <= 0xFE19: // 竖排标点
		return true
	case r >= 0xFE30 && r <= 0xFE6F: // CJK 兼容形式
		return true
	case r >= 0xFF00 && r <= 0xFF60: // 全角 ASCII
		return true
	case r >= 0xFFE0 && r <= 0xFFE6: // 全角符号
		return true
	case r >= 0x1F300 && r <= 0x1F64F: // 表情符号
		return true
	case r >= 0x1F900 && r <= 0x1F9FF:
		return true
	case r >= 0x20000 && r <= 0x3FFFD: // CJK 扩展 B 及以后
		return true
	}
	return false
}

// padRight 按显示宽度在右侧补空格，使该单元格占满 width 列。
func padRight(s string, width int) string {
	if gap := width - displayWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// tableColumn 描述表格中的一列。
type tableColumn struct {
	header string // 表头文字
	align  int    // alignLeft 或 alignRight
}

const (
	alignLeft = iota
	alignRight
)

// renderTable 渲染一张按「显示宽度」对齐的表格。
//
// 为什么不用标准库 text/tabwriter：tabwriter 只识别制表符并**按 rune 计数**估算列宽，
// 遇到双列宽的中日韩文字时会算少一半宽度，导致后续所有列错位。
// 这里改为显式测量每个单元格的显示宽度后手工补齐，中英文混排也能严格对齐。
func renderTable(w io.Writer, p palette, columns []tableColumn, rows [][]string) {
	// 第一遍：测量每列的最大显示宽度（表头也参与测量）。
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = displayWidth(col.header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && displayWidth(cell) > widths[i] {
				widths[i] = displayWidth(cell)
			}
		}
	}

	// 表头：先按宽度补齐，再整体着色。着色只影响观感，不参与宽度计算。
	heads := make([]string, len(columns))
	for i, col := range columns {
		heads[i] = col.header
	}
	fmt.Fprintf(w, "%s\n", renderRow(heads, widths, columns, func(s string) string {
		return p.paint(p.head, s)
	}))

	// 数据行：不着色，由调用方在生成单元格时自行着色（同样遵守「先补齐、后着色」）。
	for _, row := range rows {
		fmt.Fprintf(w, "%s\n", renderRow(row, widths, columns, nil))
	}
}

// renderRow 拼装一行表格文本。
//
// 着色约定：paint 必须作用在**已补齐宽度**的字符串上，
// 否则 ANSI 转义序列会被计入显示宽度，导致列错位。
func renderRow(cells []string, widths []int, columns []tableColumn, paint func(string) string) string {
	const gap = 3 // 列间距，保证视觉上不拥挤

	var sb strings.Builder
	for i := range columns {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}

		width := widths[i]
		last := i == len(columns)-1

		// 注意：最后一列也要参与对齐补齐，只是行尾空白随后会被统一裁掉。
		// 若因为「是最后一列」就跳过补齐，数值列的表头（如 RSS 仅 3 列）
		// 就无法与其数值（如 206.2 MB 共 8 列）右对齐，表头会左偏。
		var padded string
		if columns[i].align == alignRight {
			padded = strings.Repeat(" ", max(0, width-displayWidth(cell))) + cell
		} else {
			padded = padRight(cell, width)
		}

		if paint != nil {
			padded = paint(padded)
		}
		sb.WriteString(padded)

		if !last {
			sb.WriteString(strings.Repeat(" ", gap))
		}
	}
	// 裁掉行尾空白，避免「看起来有列但实际是空格」的假对齐。
	return strings.TrimRight(sb.String(), " ")
}

// max 返回两个整数中的较大者。
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ============================ 结果渲染 ============================

// portRow 是一行表格数据，抽出来是为了让列表与单端口详情共用同样的取值逻辑。
type portRow struct {
	port    string
	pid     string
	process string
	rss     string
	cpu     string
}

// buildRow 把 core.PortInfo 转换为表格行。
// 权限受限时进程名列给出明确占位符，而不是留空。
func buildRow(info core.PortInfo) portRow {
	process := info.Name
	if process == "" {
		process = degradedMarker
	}
	rss := "-"
	if info.RSS > 0 {
		rss = humanBytes(info.RSS)
	}
	cpu := "-"
	if info.CPU > 0 {
		cpu = fmt.Sprintf("%.1f%%", info.CPU)
	}
	return portRow{
		port:    strconv.Itoa(info.Port),
		pid:     strconv.Itoa(int(info.PID)),
		process: truncate(process, maxProcessNameWidth),
		rss:     rss,
		cpu:     cpu,
	}
}

// renderPortTable 渲染端口列表。
//
// 列宽由 renderTable 统一测量，中英文混排（如中文进程名）也能严格对齐。
func renderPortTable(w io.Writer, p palette, infos []core.PortInfo) {
	columns := []tableColumn{
		{header: "PORT", align: alignRight},
		{header: "PID", align: alignRight},
		{header: "PROCESS", align: alignLeft},
		{header: "RSS", align: alignRight},
		{header: "CPU", align: alignRight},
	}

	rows := make([][]string, 0, len(infos))
	for _, info := range infos {
		row := buildRow(info)

		// 权限受限的占位符用琥珀色标出，一眼可辨，但不影响列宽。
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
		})
	}
	renderTable(w, p, columns, rows)
}

// summaryLine 打印结果统计行，例如「共 12 个端口被占用 · 耗时 8.2s」。
func summaryLine(w io.Writer, p palette, parts ...string) {
	fmt.Fprintf(w, "  %s\n", p.paint(p.dim, strings.Join(parts, "  ·  ")))
}

// degradedNote 在存在权限受限条目时输出一条统一的说明脚注。
func degradedNote(w io.Writer, p palette, infos []core.PortInfo) {
	degraded := 0
	for _, info := range infos {
		if info.Degraded {
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

// options 汇总所有子命令共享的输出相关开关。
type options struct {
	jsonOutput bool
}

// NewRootCommand 构建 port-detective 的根命令及其全部子命令。
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
		SilenceUsage:  true, // 出错时只打印错误本身，不刷整屏帮助
		SilenceErrors: true, // 错误由 Execute 统一渲染，避免重复
	}

	root.AddCommand(
		newCheckCommand(opts),
		newScanCommand(opts),
		newKillCommand(opts),
		newMCPCommand(opts),
	)
	return root
}

// Execute 是 main.go 的唯一入口。
func Execute() {
	root := NewRootCommand()
	if err := root.Execute(); err != nil {
		p := newPalette(os.Stderr)
		fmt.Fprintf(os.Stderr, "%s %v\n", p.paint(p.warn, "错误:"), err)
		os.Exit(1)
	}
}

// newCheckCommand 实现 check 子命令：体检单个端口。
func newCheckCommand(opts *options) *cobra.Command {
	var protocol string

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
			info, err := core.InspectPort(port, protocol)
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

	cmd.Flags().StringVarP(&protocol, "protocol", "p", core.ProtocolTCP, "探测协议：tcp 或 udp")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "以 JSON 格式输出，便于脚本消费")
	return cmd
}

// renderCheck 渲染单端口体检报告：标题 + 连接信息 + 进程画像。
func renderCheck(w io.Writer, p palette, info *core.PortInfo, elapsed time.Duration) {
	const width = 58

	state := p.paint(p.warn, "已占用")
	if !info.IsOpen {
		state = p.paint(p.title, "空闲")
	}
	sectionTitle(w, p, fmt.Sprintf("端口体检  %s/%s",
		strings.ToUpper(info.Protocol), p.paint(p.accent, strconv.Itoa(info.Port))), width)

	// 连接信息
	fmt.Fprintf(w, "  %s\n", p.paint(p.head, "连接状态"))
	field(w, p, "状态", keyWidth, state)
	field(w, p, "协议", keyWidth, strings.ToUpper(info.Protocol))

	if !info.IsOpen {
		field(w, p, "结论", keyWidth, "该端口当前无人监听，可以直接使用。")
		fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
		summaryLine(w, p, fmt.Sprintf("探测耗时 %s", humanDuration(elapsed)))
		return
	}

	// 进程画像
	fmt.Fprintf(w, "\n  %s\n", p.paint(p.head, "归属进程"))
	if info.Name != "" {
		field(w, p, "进程名", keyWidth, info.Name)
	}
	if info.PID > 0 {
		field(w, p, "PID", keyWidth, p.paint(p.accent, strconv.Itoa(int(info.PID))))
	}
	if info.Exe != "" {
		field(w, p, "可执行文件", keyWidth, info.Exe)
	}
	if info.Cmdline != "" {
		field(w, p, "启动命令", keyWidth, truncate(info.Cmdline, 96))
	}
	if info.RSS > 0 {
		field(w, p, "常驻内存", keyWidth, humanBytes(info.RSS))
	}
	if info.CPU > 0 {
		field(w, p, "CPU 占用", keyWidth, fmt.Sprintf("%.1f%%", info.CPU))
	}
	if info.Created > 0 {
		field(w, p, "启动时间", keyWidth, time.Unix(info.Created, 0).Format("2006-01-02 15:04:05"))
	}

	fmt.Fprintf(w, "\n")
	if info.Degraded {
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

// newScanCommand 实现 scan 子命令：列出被占用的端口。
func newScanCommand(opts *options) *cobra.Command {
	var (
		protocol string
		workers  int
		rangeStr string
		allConns bool
	)

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "扫描并列出本机被占用的端口",
		Long: strings.Join([]string{
			"扫描本机被占用的端口。",
			"",
			"默认直接读取系统连接表，毫秒级返回全部结果；",
			"指定 --range 时改用 goroutine 并发探测引擎，可发现连接表中未登记的监听。",
		}, "\n"),
		Example: strings.Join([]string{
			"  port-detective scan",
			"  port-detective scan --range 8000-8100",
			"  port-detective scan --json",
		}, "\n"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := stdout(cmd)

			var (
				infos   []core.PortInfo
				elapsed time.Duration
				err     error
				engine  string
			)

			if rangeStr != "" {
				// 并发探测模式：走 goroutine + WaitGroup + channel 引擎。
				start, end, parseErr := parsePortRange(rangeStr)
				if parseErr != nil {
					return parseErr
				}
				engine = fmt.Sprintf("并发探测 %d-%d", start, end)
				begin := time.Now()
				infos, err = core.ScanPortsConcurrent(cmd.Context(), start, end, core.ScanOptions{
					Protocol: protocol,
					Workers:  workers,
				})
				elapsed = time.Since(begin)
				if err != nil {
					return err
				}
			} else {
				// 默认模式：直接读取系统连接表，结果与探测引擎等价但快得多。
				engine = "系统连接表 · 监听端口"
				if allConns {
					engine = "系统连接表 · 全部连接"
				}
				begin := time.Now()
				infos, err = core.ListOccupiedPorts(cmd.Context(), core.ListOptions{
					Protocol: protocol,
					All:      allConns,
				})
				elapsed = time.Since(begin)
				if err != nil {
					return err
				}
			}

			if opts.jsonOutput {
				return writeJSON(out, infos)
			}
			renderScan(out, newPalette(out), infos, elapsed, engine, protocol)
			return nil
		},
	}

	cmd.Flags().StringVarP(&protocol, "protocol", "p", core.ProtocolTCP, "探测协议：tcp 或 udp")
	cmd.Flags().IntVarP(&workers, "workers", "w", 0, "并发协程数，0 表示按 CPU 核数自动推导")
	cmd.Flags().StringVar(&rangeStr, "range", "", "仅扫描指定区间（如 1-1024），改用并发探测引擎")
	cmd.Flags().BoolVarP(&allConns, "all", "a", false, "包含出站连接（默认仅列出监听端口）")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "以 JSON 格式输出，便于脚本消费")
	return cmd
}

// renderScan 渲染扫描结果：标题 + 表格 + 统计脚注。
func renderScan(w io.Writer, p palette, infos []core.PortInfo, elapsed time.Duration, engine, protocol string) {
	const width = 62

	title := fmt.Sprintf("端口扫描  %s", strings.ToUpper(protocol))
	sectionTitle(w, p, title, width)

	if len(infos) == 0 {
		fmt.Fprintf(w, "  %s\n\n", p.paint(p.dim, "未发现被占用的端口。"))
		summaryLine(w, p, fmt.Sprintf("数据来源 %s", engine), fmt.Sprintf("耗时 %s", humanDuration(elapsed)))
		fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
		return
	}

	fmt.Fprintf(w, "\n")
	renderPortTable(w, p, infos)
	fmt.Fprintf(w, "\n")

	summaryLine(w, p,
		fmt.Sprintf("共 %d 个端口被占用", len(infos)),
		fmt.Sprintf("数据来源 %s", engine),
		fmt.Sprintf("耗时 %s", humanDuration(elapsed)))
	degradedNote(w, p, infos)

	fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
	fmt.Fprintf(w, "%s\n", p.paint(p.dim, "查看详情  port-detective check <端口>      释放端口  port-detective kill <端口>"))
}

// newKillCommand 实现 kill 子命令：释放被占用的端口。
func newKillCommand(opts *options) *cobra.Command {
	var (
		force bool
		yes   bool
	)

	cmd := &cobra.Command{
		Use:   "kill <端口号>",
		Short: "释放被指定端口占用的进程",
		Long: strings.Join([]string{
			"结束占用指定端口的进程，从而释放该端口。",
			"",
			"默认发送优雅退出请求；加 --force 则强制终止。",
			"为避免误操作，非 --yes 模式下会先展示将被结束的进程并要求确认。",
		}, "\n"),
		Example: strings.Join([]string{
			"  port-detective kill 8080",
			"  port-detective kill 8080 --force --yes",
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

			// 先体检，确认端口确实被占用并展示目标进程，避免误杀。
			info, err := core.InspectPort(port, core.ProtocolTCP)
			if err != nil {
				return err
			}
			if !info.IsOpen {
				return fmt.Errorf("端口 %d 当前未被占用，无需释放", port)
			}

			if !opts.jsonOutput {
				sectionTitle(out, p, fmt.Sprintf("释放端口  %s", p.paint(p.accent, strconv.Itoa(port))), 58)
				fmt.Fprintf(out, "\n  %s\n", p.paint(p.head, "将被结束的进程"))
				if info.Name != "" {
					field(out, p, "进程名", 8, info.Name)
				}
				field(out, p, "PID", 8, strconv.Itoa(int(info.PID)))
				if info.Cmdline != "" {
					field(out, p, "启动命令", 8, truncate(info.Cmdline, 96))
				}
				fmt.Fprintf(out, "\n")
			}

			if !yes && !opts.jsonOutput {
				fmt.Fprintf(out, "  确认结束该进程？%s ", p.paint(p.dim, "[y/N]:"))
				if !confirm(cmd.InOrStdin()) {
					fmt.Fprintf(out, "  %s\n", p.paint(p.dim, "已取消，未对进程做任何操作。"))
					return nil
				}
			}

			pid, err := core.KillPortProcess(port, force)
			if err != nil {
				return err
			}

			if opts.jsonOutput {
				return writeJSON(out, map[string]any{
					"port":      port,
					"pid":       pid,
					"force":     force,
					"released":  true,
					"protocol":  core.ProtocolTCP,
					"timestamp": time.Now().Format(time.RFC3339),
				})
			}

			fmt.Fprintf(out, "  %s 端口 %s 已释放（进程 %d 已结束）。\n",
				p.paint(p.title, "完成"), p.paint(p.accent, strconv.Itoa(port)), pid)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "强制终止进程（Kill），默认请求优雅退出（Terminate）")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "跳过交互确认，直接执行")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "以 JSON 格式输出，便于脚本消费")
	return cmd
}

// confirm 读取一行用户输入，仅当输入以 y/Y 开头时返回 true。
func confirm(in io.Reader) bool {
	var answer string
	if _, err := fmt.Fscanln(in, &answer); err != nil {
		return false
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return strings.HasPrefix(answer, "y")
}

// newMCPCommand 实现 mcp 子命令：以 MCP Stdio 服务端模式启动。
//
// 重要：Stdio 传输下 stdin/stdout 专供 JSON-RPC 报文通信。
// 因此本命令在中继期间**绝不向 stdout 写入任何内容**，
// 启动提示与错误信息一律走 stderr。
func newMCPCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "以 MCP 协议 Stdio 服务端模式启动，供 AI Agent 调用",
		Long: strings.Join([]string{
			"以 MCP（Model Context Protocol）Stdio 服务端模式启动。",
			"",
			"启动后通过标准输入输出与 AI 客户端通信，注册以下工具：",
			"  inspect_port     体检单个端口，返回占用进程的名称、PID、内存与 CPU",
			"  scan_dev_ports   扫描被占用的端口（默认覆盖开发常用端口）",
			"  free_port        结束占用端口的进程以释放端口",
			"",
			"注意：Stdio 模式下标准输出专供协议通信，诊断信息一律写入标准错误。",
		}, "\n"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// --json 仅用于自检：打印服务端能力清单后退出，不进入协议循环。
			if opts.jsonOutput {
				return writeJSON(stdout(cmd), mcpserver.Describe())
			}

			// 启动提示写入 stderr：stdout 必须保持纯净。
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

// parsePort 解析端口号并校验范围。
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

// parsePortRange 解析形如 "8000-8100" 或 "8080" 的区间参数。
func parsePortRange(s string) (int, int, error) {
	parts := strings.SplitN(strings.TrimSpace(s), "-", 2)
	start, err := parsePort(parts[0])
	if err != nil {
		return 0, 0, err
	}
	if len(parts) == 1 {
		return start, start, nil
	}
	end, err := parsePort(parts[1])
	if err != nil {
		return 0, 0, err
	}
	if start > end {
		return 0, 0, fmt.Errorf("区间 %s 非法：起始端口不能大于结束端口", s)
	}
	return start, end, nil
}
