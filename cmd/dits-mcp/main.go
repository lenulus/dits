// dits-mcp is a Model Context Protocol (MCP) server exposing DITS
// operations over stdio, intended for use by Claude Code and other MCP
// clients. It is a thin wrapper around internal/mcp which in turn calls
// into internal/workops.
package main

import (
	"flag"
	"fmt"
	"os"

	ditsmcp "github.com/lenulus/pf/internal/mcp"
)

func main() {
	project := flag.String("project", "", "path to a DITS project directory (containing or inside a .dits/). Defaults to cwd discovery.")
	flag.Parse()

	if err := ditsmcp.Serve(ditsmcp.Config{ProjectRoot: *project}); err != nil {
		fmt.Fprintln(os.Stderr, "dits-mcp:", err)
		os.Exit(1)
	}
}
