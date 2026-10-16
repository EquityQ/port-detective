// Command port-detective is a cross-platform local port inspector and process
// diagnostic CLI with native MCP support for AI agents.
//
// This is the sole entry point and delegates execution to internal/cli.
package main

import (
	"github.com/EquityQ/port-detective/internal/cli"
	_ "github.com/EquityQ/port-detective/internal/fingerprint"
)

func main() {
	cli.Execute()
}
