// Package version exposes the build version shared by the CLI and MCP server.
package version

// Value is replaced by the release workflow through Go linker flags.
var Value = "1.2.0"
