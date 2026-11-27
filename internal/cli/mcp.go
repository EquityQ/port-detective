package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/EquityQ/port-detective/internal/mcpserver"
	"github.com/spf13/cobra"
)

// mcp starts the MCP Stdio server.
func newMCPCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start the MCP Stdio server",
		Long: strings.Join([]string{
			"Start the Model Context Protocol server over Stdio.",
			"",
			"The server communicates over standard input/output and registers:",
			"  inspect_port           inspect one port and its owning process",
			"  scan_dev_ports         scan DefaultDevPorts, explicit ports, or presets",
			"  free_port              release a port (dry_run=true by default)",
			"  clean_current_project  clean listeners for the current project",
			"  find_free_port         find a currently bindable loopback TCP port",
			"  wait_for_port          wait until a port becomes open or free",
			"  doctor                 diagnose binary, project path, and visibility",
			"",
			"Stdout is reserved for protocol messages; diagnostics go to stderr.",
		}, "\n"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.jsonOutput {
				return writeJSON(stdout(cmd), mcpserver.Describe())
			}

			perr := newPalette(os.Stderr)
			fmt.Fprintf(os.Stderr, "%s port-detective MCP Stdio server started; waiting for a client...\n",
				perr.paint(perr.dim, "·"))
			fmt.Fprintf(os.Stderr, "  Registered tools: inspect_port / scan_dev_ports / free_port / clean_current_project / find_free_port / wait_for_port / doctor\n")

			ctx, cancel := mcpserver.WaitForSignal(cmd.Context())
			defer cancel()

			if err := mcpserver.RunStdioServerWithInspector(ctx, opts.inspector); err != nil {
				return fmt.Errorf("MCP server exited unexpectedly: %w", err)
			}
			return nil
		},
	}
	return cmd
}
