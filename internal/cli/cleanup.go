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
		Use:   "kill <port>",
		Short: "Terminate the process occupying a port",
		Long: strings.Join([]string{
			"Terminate the process occupying a port.",
			"",
			"Use --dry-run to preview the target without terminating it.",
			"Interactive confirmation is required by default; use -y to skip it.",
		}, "\n"),
		Example: strings.Join([]string{
			"  port-detective kill 8080",
			"  port-detective kill 8080 --dry-run",
			"  port-detective kill 8080 -y",
		}, "\n"),
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("exactly one port is required, for example: port-detective kill 8080")
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

			// Inspect the target process before displaying the action.
			diag, err := core.InspectPort(cmd.Context(), port)
			if err != nil {
				return err
			}
			if !diag.IsOpen {
				return fmt.Errorf("port %d is currently free, no process to terminate", port)
			}

			if !opts.jsonOutput {
				title := fmt.Sprintf("Release Port  %s", p.paint(p.accent, strconv.Itoa(port)))
				if dryRun {
					title += " (Dry Run)"
				}
				sectionTitle(out, p, title, 58)
				fmt.Fprintf(out, "\n  %s\n", p.paint(p.head, "Target Process"))
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

			if !dryRun && opts.jsonOutput && !yes {
				return errors.New("refusing destructive JSON operation without explicit --yes")
			}
			if !dryRun && !yes && !opts.jsonOutput {
				confirmPrompt := fmt.Sprintf("Terminate this process? %s ", p.paint(p.dim, "[y/N]:"))
				fmt.Fprintf(out, "  %s", confirmPrompt)
				if !confirm(cmd.InOrStdin()) {
					fmt.Fprintf(out, "  %s\n", p.paint(p.dim, "Cancelled. Process was not modified."))
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

			statusText := "Done"
			if dryRun {
				statusText = "Dry Run"
			}
			fmt.Fprintf(out, "  %s %s\n", p.paint(p.title, statusText), msg)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview without terminating the process")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip interactive confirmation")
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
		Short: "Clean project-scoped development port processes",
		Long: strings.Join([]string{
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
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("unable to get the current working directory: %w", err)
			}

			if !dryRun && opts.jsonOutput && !yes {
				return errors.New("refusing destructive JSON operation without explicit --yes")
			}
			if !dryRun && !yes && !opts.jsonOutput {
				promptMsg := fmt.Sprintf("Terminate project listeners in %s?", cwd)
				fmt.Fprintf(out, "  %s %s ", promptMsg, p.paint(p.dim, "[y/N]:"))
				if !confirm(cmd.InOrStdin()) {
					fmt.Fprintf(out, "  %s\n", p.paint(p.dim, "Cancelled. No processes were touched."))
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

			title := "Project Ports Clean"
			if dryRun {
				title += " (Dry Run)"
			}
			sectionTitle(out, p, title, 80)

			if len(cleaned) == 0 {
				emptyMsg := "No project-scoped listeners found for the current project."
				fmt.Fprintf(out, "  %s\n\n", p.paint(p.dim, emptyMsg))
				fmt.Fprintf(out, "%s\n", p.paint(p.dim, rule(80)))
				return cleanErr
			}

			fmt.Fprintf(out, "\n")
			renderPortTable(out, p, cleaned)
			fmt.Fprintf(out, "\n")

			summaryMsg := fmt.Sprintf("Processed %d port(s) for current project (dry-run: %v)", len(cleaned), dryRun)
			summaryLine(out, p, summaryMsg)
			fmt.Fprintf(out, "%s\n", p.paint(p.dim, rule(80)))
			return cleanErr
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview without terminating processes")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip interactive confirmation")
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
