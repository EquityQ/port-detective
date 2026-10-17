package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/EquityQ/port-detective/internal/core"
	"github.com/spf13/cobra"
)

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

			if !dryRun && opts.jsonOutput && !yes {
				return errors.New("refusing destructive JSON operation without explicit --yes")
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

// clean processes listeners associated with the current project.
func newCleanCommand(opts *options) *cobra.Command {
	var (
		dryRun bool
		yes    bool
	)

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "一键清理当前项目占用的开发端口进程 / Clean project-scoped development port processes",
		Long: strings.Join([]string{
			"并发扫描高频开发端口，仅处理属于当前工作目录项目的进程。",
			"Concurrently scan development ports and process only listeners belonging to the current working directory.",
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

			if !dryRun && opts.jsonOutput && !yes {
				return errors.New("refusing destructive JSON operation without explicit --yes")
			}
			if !dryRun && !yes && !opts.jsonOutput {
				promptMsg := fmt.Sprintf("确认清理当前项目目录 %s 下占用的进程？", cwd)
				if !zh {
					promptMsg = fmt.Sprintf("Process listeners occupying ports for project in %s?", cwd)
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

			cleaned, cleanErr := core.CleanCurrentProjectPorts(cmd.Context(), cwd, dryRun)

			if opts.jsonOutput {
				if err := writeJSON(out, cleaned); err != nil {
					return err
				}
				return cleanErr
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
				emptyMsg := "当前工作目录项目未发现占用端口的进程。"
				if !zh {
					emptyMsg = "No project-scoped listeners found for the current project."
				}
				fmt.Fprintf(out, "  %s\n\n", p.paint(p.dim, emptyMsg))
				fmt.Fprintf(out, "%s\n", p.paint(p.dim, rule(80)))
				return cleanErr
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
			return cleanErr
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
