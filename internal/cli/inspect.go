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
	var watch bool
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "check <port>",
		Short: "Inspect a port and its owning process",
		Long:  "Inspect a TCP port and report its listener, process, framework, and resource usage. With --watch --json, emit compact newline-delimited JSON (NDJSON). TCP occupancy does not prove application readiness.",
		Example: strings.Join([]string{
			"  port-detective check 8080",
			"  port-detective check 5432 --json",
		}, "\n"),
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(fmt.Errorf("exactly one port is required, for example: port-detective check 8080"))
			}
			if _, err := parsePort(args[0]); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval <= 0 {
				return usageError(fmt.Errorf("--interval must be greater than zero"))
			}
			port, _ := parsePort(args[0])
			if watch {
				return watchPort(cmd, opts, port, interval)
			}
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
	cmd.Flags().BoolVar(&watch, "watch", false, "repeat inspection until canceled; --json emits NDJSON")
	cmd.Flags().DurationVar(&interval, "interval", time.Second, "delay between watched inspections")
	return cmd
}

func renderCheck(w io.Writer, p palette, info *core.PortDiagnostic, elapsed time.Duration) {
	const width = 64

	state := p.paint(p.warn, "Occupied")
	if !info.IsOpen {
		state = p.paint(p.title, "Free")
	}

	titleText := fmt.Sprintf("Port Diagnostic  TCP/%s", p.paint(p.accent, strconv.Itoa(info.Port)))
	sectionTitle(w, p, titleText, width)

	fmt.Fprintf(w, "  %s\n", p.paint(p.head, "Connection State"))
	field(w, p, "Status", keyWidth, state)
	field(w, p, "Protocol", keyWidth, strings.ToUpper(info.Protocol))
	field(w, p, "Summary", keyWidth, info.Status)

	if !info.IsOpen {
		field(w, p, "Conclusion", keyWidth, "Port is free and ready for use.")
		fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
		summaryLine(w, p, fmt.Sprintf("Elapsed: %s", humanDuration(elapsed)))
		return
	}

	fmt.Fprintf(w, "\n  %s\n", p.paint(p.head, "Owning Process"))
	if info.ProcessName != "" {
		field(w, p, "Process", keyWidth, info.ProcessName)
	}
	if info.PID > 0 {
		field(w, p, "PID", keyWidth, p.paint(p.accent, strconv.Itoa(int(info.PID))))
	}
	if info.Project != nil {
		if info.Project.Framework != "" {
			field(w, p, "Framework", keyWidth, info.Project.Framework)
		}
		if info.Project.Name != "" {
			field(w, p, "Project", keyWidth, info.Project.Name)
		}
		if info.Project.Path != "" {
			field(w, p, "Path", keyWidth, truncate(info.Project.Path, 50))
		}
	}
	if info.Cmdline != "" {
		field(w, p, "Command", keyWidth, truncate(info.Cmdline, 96))
	}
	if info.MemoryMB > 0 {
		field(w, p, "Memory", keyWidth, fmt.Sprintf("%.1f MB", info.MemoryMB))
	}
	if info.CPUPercent > 0 {
		field(w, p, "CPU", keyWidth, fmt.Sprintf("%.1f%%", info.CPUPercent))
	}

	fmt.Fprintf(w, "\n")
	if info.Status == core.StatusPermissionRequired {
		hint(w, p, "Process owner or metadata unavailable: permissions, changing listeners, or multiple owners may limit details.")
		fmt.Fprintf(w, "    %s\n", p.paint(p.dim, "Check the host/network scope and run port-detective doctor before considering elevated privileges."))
	}

	fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
	timeSummary := fmt.Sprintf("Elapsed: %s", humanDuration(elapsed))
	releaseCmd := fmt.Sprintf("Release: port-detective kill %d", info.Port)
	parts := []string{timeSummary}
	if info.PID > 0 {
		parts = append(parts, releaseCmd)
	}
	summaryLine(w, p, parts...)
}

// scan concurrently scans ports, defaulting to DefaultDevPorts.
func newScanCommand(opts *options) *cobra.Command {
	var portsStr, preset, projectPath string

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Concurrently scan occupied ports",
		Long: strings.Join([]string{
			"Concurrently scan local TCP ports.",
			"",
			"The default target is DefaultDevPorts; use --ports for a custom list or --preset for explicit presets.",
			"The project preset reads .port-detective.json from the selected project root only when requested.",
		}, "\n"),
		Example: strings.Join([]string{
			"  port-detective scan",
			"  port-detective scan --ports 3000,8080,5432",
			"  port-detective scan --preset web",
			"  port-detective scan --preset project --project ../my-app",
			"  port-detective scan --json",
		}, "\n"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := stdout(cmd)

			targetPorts, scope, err := selectScanPorts(cmd, portsStr, preset, projectPath)
			if err != nil {
				return err
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
			engine := fmt.Sprintf("Concurrent (%d target ports)", len(targetPorts))
			if scope != "" {
				fmt.Fprintf(out, "Scope: %s\n", scope)
			}
			renderScan(out, newPalette(out), infos, elapsed, engine)
			return nil
		},
	}

	cmd.Flags().StringVar(&portsStr, "ports", "", "comma-separated ports or ranges, e.g. 3000-3010,8080")
	cmd.Flags().StringVar(&preset, "preset", "", "explicit port preset: web, python, database, or project")
	cmd.Flags().StringVar(&projectPath, "project", "", "project directory for --preset project (default: nearest project above current directory)")
	return cmd
}

func renderScan(w io.Writer, p palette, infos []core.PortDiagnostic, elapsed time.Duration, engine string) {
	const width = 80

	sectionTitle(w, p, "Port Scanner  TCP", width)

	if len(infos) == 0 {
		fmt.Fprintf(w, "  %s\n\n", p.paint(p.dim, "No occupied ports found."))
		summaryLine(w, p, fmt.Sprintf("Engine: %s", engine), fmt.Sprintf("Elapsed: %s", humanDuration(elapsed)))
		fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
		return
	}

	fmt.Fprintf(w, "\n")
	renderPortTable(w, p, infos)
	fmt.Fprintf(w, "\n")

	occupiedMsg := fmt.Sprintf("%d port(s) occupied", len(infos))
	engineMsg := fmt.Sprintf("Engine: %s", engine)
	timeMsg := fmt.Sprintf("Elapsed: %s", humanDuration(elapsed))
	footer := "Details: port-detective check <port>      Release: port-detective kill <port>"

	summaryLine(w, p, occupiedMsg, engineMsg, timeMsg)
	degradedNote(w, p, infos)

	fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
	fmt.Fprintf(w, "%s\n", p.paint(p.dim, footer))
}
