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
	var protocol string
	cmd := &cobra.Command{
		Use:   "check <port>",
		Short: "Inspect a port and its owning process",
		Long:  "Inspect a TCP or UDP port and report its listener, bind addresses, process, framework, and resource usage. With --watch --json, emit compact newline-delimited JSON (NDJSON). Port occupancy does not prove application readiness.",
		Example: strings.Join([]string{
			"  port-detective check 8080",
			"  port-detective check 5353 --protocol udp",
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
			proto, err := core.NormalizeProtocol(protocol)
			if err != nil {
				return usageError(err)
			}
			port, _ := parsePort(args[0])
			if watch {
				return watchPort(cmd, opts, port, proto, interval)
			}
			out := stdout(cmd)

			start := time.Now()
			info, err := core.InspectPortProtocol(cmd.Context(), port, proto)
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
	cmd.Flags().StringVar(&protocol, "protocol", core.ProtocolTCP, "transport protocol to inspect: tcp or udp")
	return cmd
}

func renderCheck(w io.Writer, p palette, info *core.PortDiagnostic, elapsed time.Duration) {
	const width = 64

	state := p.paint(p.warn, "Occupied")
	if !info.IsOpen {
		state = p.paint(p.title, "Free")
	}

	titleText := fmt.Sprintf("Port Diagnostic  %s/%s", strings.ToUpper(info.Protocol), p.paint(p.accent, strconv.Itoa(info.Port)))
	sectionTitle(w, p, titleText, width)

	fmt.Fprintf(w, "  %s\n", p.paint(p.head, "Connection State"))
	field(w, p, "Status", keyWidth, state)
	field(w, p, "Protocol", keyWidth, strings.ToUpper(info.Protocol))
	field(w, p, "Summary", keyWidth, info.Status)
	if len(info.BindAddresses) > 0 {
		field(w, p, "Bind", keyWidth, strings.Join(info.BindAddresses, ", "))
	}

	if !info.IsOpen {
		// Never promise a free port when the listener table was unreadable: a
		// process bound only to another interface would be invisible to the probe.
		if info.ListenerTableMissing || info.Status == core.StatusUndetermined {
			field(w, p, "Conclusion", keyWidth, "Inconclusive: the local listener table could not be read.")
			hint(w, p, "A closed loopback probe cannot rule out a listener bound to another interface.")
			fmt.Fprintf(w, "    %s\n", p.paint(p.dim, "Check the host/network scope and run port-detective doctor before treating this port as available."))
		} else {
			field(w, p, "Conclusion", keyWidth, "No loopback listener detected; the port is not reserved for you.")
		}
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
	var portsStr, preset, projectPath, protocol string

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Concurrently scan occupied ports",
		Long: strings.Join([]string{
			"Concurrently scan local TCP or UDP ports.",
			"",
			"The default target is DefaultDevPorts; use --ports for a custom list or --preset for explicit presets.",
			"The project preset reads .port-detective.json from the selected project root only when requested.",
		}, "\n"),
		Example: strings.Join([]string{
			"  port-detective scan",
			"  port-detective scan --ports 3000,8080,5432",
			"  port-detective scan --ports 53,5353 --protocol udp",
			"  port-detective scan --preset web",
			"  port-detective scan --preset project --project ../my-app",
			"  port-detective scan --json",
		}, "\n"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := stdout(cmd)
			proto, err := core.NormalizeProtocol(protocol)
			if err != nil {
				return usageError(err)
			}

			targetPorts, scope, err := selectScanPorts(cmd, portsStr, preset, projectPath)
			if err != nil {
				return err
			}

			begin := time.Now()
			summary, err := core.ScanPortsProtocolSummary(cmd.Context(), targetPorts, proto)
			elapsed := time.Since(begin)
			if err != nil {
				return err
			}
			infos := summary.Diagnostics
			// A degraded scan cannot prove the unscanned-but-listening world is empty,
			// so the warning goes to stderr and stays out of the JSON array shape.
			if summary.Degraded && summary.Warning != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: incomplete result: %s\n", summary.Warning)
			}

			if opts.jsonOutput {
				return writeJSON(out, infos)
			}
			engine := fmt.Sprintf("Concurrent (%d target ports)", len(targetPorts))
			if scope != "" {
				fmt.Fprintf(out, "Scope: %s\n", scope)
			}
			p := newPalette(out)
			renderScan(out, p, infos, elapsed, engine, proto)
			if summary.Degraded && summary.Warning != "" {
				fmt.Fprintf(out, "%s\n", p.paint(p.warn, "Incomplete result: "+summary.Warning))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&portsStr, "ports", "", "comma-separated ports or ranges, e.g. 3000-3010,8080")
	cmd.Flags().StringVar(&preset, "preset", "", "explicit port preset: web, python, database, or project")
	cmd.Flags().StringVar(&projectPath, "project", "", "project directory for --preset project (default: nearest project above current directory)")
	cmd.Flags().StringVar(&protocol, "protocol", core.ProtocolTCP, "transport protocol to scan: tcp or udp")
	return cmd
}

func renderScan(w io.Writer, p palette, infos []core.PortDiagnostic, elapsed time.Duration, engine, protocol string) {
	const width = 80

	sectionTitle(w, p, fmt.Sprintf("Port Scanner  %s", strings.ToUpper(protocol)), width)

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
