// dits-mcp is a Model Context Protocol (MCP) server exposing DITS
// operations over stdio, intended for use by Claude Code and other MCP
// clients. It is a thin wrapper around internal/mcp which in turn calls
// into internal/workops.
//
// Logging discipline: the MCP protocol speaks JSON-RPC over stdout, so
// logs MUST go to a file by default. The default path is
// $XDG_STATE_HOME/dits/mcp.log (or ~/.dits/mcp.log if XDG isn't set).
// Pass --log-file=- to opt into stderr logging explicitly.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/lenulus/pf/internal/logging"
	ditsmcp "github.com/lenulus/pf/internal/mcp"
)

func main() { os.Exit(run()) }

func run() int {
	project := flag.String("project", "", "path to a DITS project directory (containing or inside a .dits/). Defaults to cwd discovery.")
	logLevel := flag.String("log-level", "info", "log level: error, warn, info, debug, trace")
	logFormat := flag.String("log-format", "json", "log format: text or json")
	logFile := flag.String("log-file", "", "log file path; defaults to $XDG_STATE_HOME/dits/mcp.log. Use '-' for stderr.")
	flag.Parse()

	level, err := logging.Parse(*logLevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dits-mcp:", err)
		return 2
	}

	sinkPath := *logFile
	if sinkPath == "" {
		p, err := logging.DefaultMCPLogPath()
		if err != nil {
			fmt.Fprintln(os.Stderr, "dits-mcp: resolving default log path:", err)
			return 2
		}
		sinkPath = p
	}
	sink, err := logging.OpenSink(sinkPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dits-mcp:", err)
		return 2
	}
	defer sink.Close()

	logger := logging.New(sink, level, *logFormat)

	version := "dev"
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		version = bi.Main.Version
	}

	logger.Info("mcp_server_start",
		"version", version,
		"project_root", *project,
		"log_level", level.String(),
		"log_file", sinkPath,
	)

	if err := ditsmcp.Serve(ditsmcp.Config{ProjectRoot: *project, Logger: logger}); err != nil {
		logger.Error("mcp_server_exit", "err", err)
		fmt.Fprintln(os.Stderr, "dits-mcp:", err)
		return 1
	}
	return 0
}
