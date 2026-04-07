// Package mcp wires the dits-mcp Model Context Protocol server.
//
// The server is a thin adapter over internal/workops: each MCP tool handler
// opens a WorkOps against the configured project, invokes the corresponding
// method, and returns a JSON-encoded tool result. No DITS business logic
// lives here — this package is I/O only.
package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Config carries the runtime config for the MCP server.
type Config struct {
	// ProjectRoot is the directory containing (or inside) a .dits/ repo.
	// If empty, each handler will fall back to cwd-based discovery.
	ProjectRoot string
}

// NewServer constructs an MCPServer with all dits_* tools registered.
func NewServer(cfg Config) *server.MCPServer {
	s := server.NewMCPServer(
		"dits-mcp",
		"0.1.0",
		server.WithToolCapabilities(true),
	)
	registerTools(s, cfg)
	return s
}

// Serve runs the server over stdio. Blocks until EOF or error.
func Serve(cfg Config) error {
	return server.ServeStdio(NewServer(cfg))
}

// --- helpers for tool construction ---

func readOnly() mcp.ToolOption {
	return mcp.WithReadOnlyHintAnnotation(true)
}
