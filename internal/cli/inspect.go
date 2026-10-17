package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/EquityQ/port-detective/internal/core"
	"github.com/spf13/cobra"
)

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
