package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/EquityQ/port-detective/internal/core"
	"github.com/spf13/cobra"
)

// newKillCommand releases a port and supports --dry-run, --protocol, and --preview-id.
func newKillCommand(opts *options) *cobra.Command {
	var (
		dryRun    bool
		yes       bool
		previewID string
		protocol  string
	)

	cmd := &cobra.Command{
		Use:   "kill <port>",
		Short: "Terminate the process occupying a port",
		Long: strings.Join([]string{
			"Terminate the process occupying a TCP or UDP port.",
			"",
			"Use --dry-run to preview the target without terminating it. In --json mode the",
			"dry run also returns a preview_id; pass it back with --preview-id so the action",
			"is bound to the process you actually previewed.",
			"Interactive confirmation is required by default; use -y to skip it.",
		}, "\n"),
		Example: strings.Join([]string{
			"  port-detective kill 8080",
			"  port-detective kill 5353 --protocol udp --dry-run",
			"  port-detective kill 8080 --dry-run",
			"  port-detective kill 8080 -y",
			"  port-detective kill 8080 --json --dry-run --yes",
			"  port-detective kill 8080 --json --yes --preview-id <token>",
		}, "\n"),
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(fmt.Errorf("exactly one port is required, for example: port-detective kill 8080"))
			}
			if _, err := parsePort(args[0]); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			proto, err := core.NormalizeProtocol(protocol)
			if err != nil {
				return usageError(err)
			}
			port, _ := parsePort(args[0])
			out := stdout(cmd)
			p := newPalette(out)

			if !dryRun && opts.jsonOutput && !yes {
				return commandFailure("confirmation_required", 2, "refusing destructive JSON operation without explicit --yes; use --dry-run to preview")
			}

			var (
				target  core.PortDiagnostic
				display *core.PortDiagnostic
			)
			if strings.TrimSpace(previewID) != "" {
				// Validate a supplied preview token before probing so a forged,
				// expired, or mismatched token always fails with preview_rejected.
				targets, _, err := core.RestoreCLIPreviewTokenProtocol(core.PreviewScopeKill, previewID, []int{port}, proto)
				if err != nil {
					return commandFailure("preview_rejected", 6, "preview id rejected for port %d: %v", port, err)
				}
				target = targets[0]
				display = &target
				if !opts.jsonOutput {
					if live, liveErr := opts.inspector.InspectPortProtocol(cmd.Context(), port, proto); liveErr == nil && live != nil && live.PID == target.PID {
						display = live
					}
				}
			} else {
				diag, err := opts.inspector.InspectPortProtocol(cmd.Context(), port, proto)
				if err != nil {
					return err
				}
				if !diag.IsOpen {
					return fmt.Errorf("port %d (%s) is currently free, no process to terminate", port, proto)
				}
				target = *diag
				display = diag
			}

			if !opts.jsonOutput {
				renderKillTarget(out, p, port, display, dryRun)
			}

			if !dryRun && !yes && !opts.jsonOutput {
				confirmPrompt := fmt.Sprintf("Terminate this process? %s ", p.paint(p.dim, "[y/N]:"))
				fmt.Fprintf(out, "  %s", confirmPrompt)
				confirmed, err := confirm(cmd.Context(), cmd.InOrStdin())
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintf(out, "  %s\n", p.paint(p.dim, "Cancelled. Process was not modified."))
					return nil
				}
			}

			// Keep the previewed or displayed process identity pinned across confirmation.
			resultDiag, msg, err := opts.inspector.KillPortTarget(cmd.Context(), target, dryRun)
			if err != nil {
				return err
			}

			if opts.jsonOutput {
				payload := map[string]any{
					"port":                  port,
					"protocol":              proto,
					"dry_run":               dryRun,
					"pre_action_diagnostic": resultDiag,
					// Deprecated alias: kept so existing consumers keep working, but the
					// truthful name is pre_action_diagnostic because the snapshot below
					// describes the port BEFORE the action ran.
					"diagnostic": resultDiag,
					"message":    msg,
				}
				if resultDiag != nil && resultDiag.PostActionStatus != "" {
					payload["post_action_status"] = resultDiag.PostActionStatus
				}
				if dryRun {
					if token, expiresAt, tokenErr := core.IssueCLIPreviewToken(core.PreviewScopeKill, []core.PortDiagnostic{*resultDiag}, core.DefaultPreviewTTL); tokenErr == nil {
						payload["preview_id"] = token
						payload["preview_expires_at_unix"] = expiresAt
					}
				}
				return writeJSON(out, payload)
			}

			statusText := "Done"
			if dryRun {
				statusText = "Dry Run"
			}
			fmt.Fprintf(out, "  %s %s\n", p.paint(p.title, statusText), msg)
			if dryRun {
				if token, _, tokenErr := core.IssueCLIPreviewToken(core.PreviewScopeKill, []core.PortDiagnostic{*resultDiag}, core.DefaultPreviewTTL); tokenErr == nil {
					field(out, p, "Preview ID", 10, token)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview without terminating the process")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip interactive confirmation")
	cmd.Flags().StringVar(&previewID, "preview-id", "", "token from a --json --dry-run preview; binds the action to the previewed process")
	cmd.Flags().StringVar(&protocol, "protocol", core.ProtocolTCP, "transport protocol to release: tcp or udp")
	return cmd
}

func renderKillTarget(out io.Writer, p palette, port int, diag *core.PortDiagnostic, dryRun bool) {
	title := fmt.Sprintf("Release Port  %s", p.paint(p.accent, strconv.Itoa(port)))
	if dryRun {
		title += " (Dry Run)"
	}
	sectionTitle(out, p, title, 58)
	fmt.Fprintf(out, "\n  %s\n", p.paint(p.head, "Target Process"))
	if diag.Protocol != "" {
		field(out, p, "Protocol", 8, strings.ToUpper(diag.Protocol))
	}
	if diag.ProcessName != "" {
		field(out, p, "Process", 8, diag.ProcessName)
	}
	field(out, p, "PID", 8, strconv.Itoa(int(diag.PID)))
	if diag.Project != nil {
		if diag.Project.Framework != "" {
			field(out, p, "Framework", 8, diag.Project.Framework)
		}
		if diag.Project.Name != "" {
			field(out, p, "Project", 8, diag.Project.Name)
		}
	}
	if diag.Cmdline != "" {
		field(out, p, "Command", 8, truncate(diag.Cmdline, 96))
	}
	if diag.MemoryMB > 0 {
		field(out, p, "Memory", 8, fmt.Sprintf("%.1f MB", diag.MemoryMB))
	}
	fmt.Fprintf(out, "\n")
}
