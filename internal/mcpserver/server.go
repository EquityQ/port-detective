// Package mcpserver exposes internal/core port inspection capabilities as MCP
// (Model Context Protocol) tools over Stdio for AI agents.
//
// With Stdio transport, standard output is reserved for protocol messages. Never
// write logs or status messages to os.Stdout; send diagnostics to os.Stderr.
package mcpserver

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	"github.com/EquityQ/port-detective/internal/core"
	"github.com/EquityQ/port-detective/internal/fingerprint"
	"github.com/EquityQ/port-detective/internal/version"
)

const serverName = "port-detective"

// NewServer creates an MCP server with all tools registered using the global detector hook.
func NewServer() *server.MCPServer {
	return NewServerWithInspector(nil)
}

// NewServerWithInspector creates an MCP server whose tools use the supplied Inspector.
func NewServerWithInspector(inspector *core.Inspector) *server.MCPServer {
	s := server.NewMCPServer(
		serverName,
		version.Value,
		server.WithRecovery(),
	)

	h := &mcpHandlers{inspector: inspector}
	handlers := map[string]server.ToolHandlerFunc{
		"inspect_port":          h.inspectPort,
		"scan_dev_ports":        h.scanDevPorts,
		"free_port":             h.freePort,
		"clean_current_project": h.cleanCurrentProject,
		"find_free_port":        h.findFreePort,
		"wait_for_port":         h.waitForPort,
		"doctor":                h.doctor,
	}
	for _, tool := range serverTools() {
		handler, ok := handlers[tool.Name]
		if !ok {
			panic(fmt.Sprintf("MCP tool %q has no registered handler", tool.Name))
		}
		s.AddTool(tool, handler)
	}

	return s
}

// RunStdioServer starts the MCP server over Stdio and blocks until canceled or input closes.
func RunStdioServer(ctx context.Context) error {
	return RunStdioServerWithInspector(ctx, core.NewInspector(fingerprint.DetectProject))
}

// RunStdioServerWithInspector starts the MCP server over Stdio with an explicit Inspector.
func RunStdioServerWithInspector(ctx context.Context, inspector *core.Inspector) error {
	stdio := server.NewStdioServer(NewServerWithInspector(inspector))
	return stdio.Listen(ctx, os.Stdin, os.Stdout)
}

// ToolSummary describes a registered tool.
type ToolSummary struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ReadOnly    bool   `json:"read_only"`
	Destructive bool   `json:"destructive"`
}

// ServerInfo describes the MCP server capabilities for self-inspection.
type ServerInfo struct {
	Name    string        `json:"name"`
	Version string        `json:"version"`
	Tools   []ToolSummary `json:"tools"`
}

// Describe returns the server capabilities without starting the transport.
func Describe() ServerInfo {
	info := ServerInfo{Name: serverName, Version: version.Value}

	for _, tool := range serverTools() {
		summary := ToolSummary{
			Name:        tool.Name,
			Title:       tool.Annotations.Title,
			Description: tool.Description,
		}
		if tool.Annotations.ReadOnlyHint != nil {
			summary.ReadOnly = *tool.Annotations.ReadOnlyHint
		}
		if tool.Annotations.DestructiveHint != nil {
			summary.Destructive = *tool.Annotations.DestructiveHint
		}
		info.Tools = append(info.Tools, summary)
	}
	return info
}

// WaitForSignal returns a context canceled when SIGINT or SIGTERM arrives.
func WaitForSignal(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		defer signal.Stop(sigCh)
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}
