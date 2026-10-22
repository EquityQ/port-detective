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
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/EquityQ/port-detective/internal/fingerprint"
	"github.com/EquityQ/port-detective/internal/version"
	"github.com/spf13/cobra"
)

type options struct {
	jsonOutput bool
}

func NewRootCommand() *cobra.Command {
	opts := &options{}

	root := &cobra.Command{
		Use:     "port-detective",
		Version: version.Value,
		Short:   "Local Port Detective and Process Diagnostic Tool",
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

	root.PersistentFlags().BoolVar(&opts.jsonOutput, "json", false, "output machine-readable JSON")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return usageError(err) })

	root.AddCommand(
		newCheckCommand(opts),
		newScanCommand(opts),
		newKillCommand(opts),
		newCleanCommand(opts),
		newWaitCommand(opts),
		newFindFreeCommand(opts),
		newDoctorCommand(opts),
		newMCPCommand(opts),
	)
	root.AddCommand(newCompletionCommand(root))
	return root
}

func Execute() {
	root := NewRootCommand()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	root.SetContext(ctx)
	if code := executeCommand(root); code != 0 {
		os.Exit(code)
	}
}

// newCompletionCommand generates shell completion scripts without requiring a
// separate installation-time tool.
func newCompletionCommand(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion <bash|zsh|fish|powershell>",
		Short: "Generate a shell completion script",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("a shell is required: bash, zsh, fish, or powershell")
			}
			switch strings.ToLower(args[0]) {
			case "bash", "zsh", "fish", "powershell":
				return nil
			default:
				return fmt.Errorf("unsupported shell %q: choose bash, zsh, fish, or powershell", args[0])
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch strings.ToLower(args[0]) {
			case "bash":
				return root.GenBashCompletionV2(stdout(cmd), true)
			case "zsh":
				return root.GenZshCompletion(stdout(cmd))
			case "fish":
				return root.GenFishCompletion(stdout(cmd), true)
			case "powershell":
				return root.GenPowerShellCompletion(stdout(cmd))
			default:
				return fmt.Errorf("unreachable shell %q", args[0])
			}
		},
	}
	return cmd
}
