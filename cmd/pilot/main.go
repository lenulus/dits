// Command pilot is the Pilot web frontend for DITS (Radical Execution
// methodology). It is a separate binary that talks to the DITS substrate
// ONLY over MCP — it never imports DITS-internal packages such as
// internal/domain, internal/workops, internal/mcp, etc. See
// docs/proposals/radical-execution-implementation-plan-v2.md §2 (topology)
// and §8.1 (binary layout).
//
// Subcommands:
//
//	pilot init  --project <dir>   apply the RE meta bundle to a DITS project
//	pilot serve --project <dir>   serve the web UI (default)
//
// With no --project, the server runs against the stub MCP client (empty
// state) so the chrome is browsable without a live substrate.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
	"github.com/lenulus/pf/internal/pilot/meta"
	"github.com/lenulus/pf/internal/pilot/scheduler"
	"github.com/lenulus/pf/internal/pilot/web/handlers"
)

func main() {
	log.SetFlags(0)
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 && !isFlag(args[0]) {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		runServe(args)
	case "init":
		runInit(args)
	default:
		log.Fatalf("pilot: unknown command %q (want: serve | init)", cmd)
	}
}

func isFlag(s string) bool { return len(s) > 0 && s[0] == '-' }

// dialClient builds the MCP client from the project flag, falling back to the
// stub (empty state) when no project is configured.
func dialClient(ctx context.Context, mcpCmd, project string) (mcp.Client, error) {
	if project == "" {
		log.Print("pilot: no --project configured; using stub MCP client (empty state)")
		return mcp.NewStub(), nil
	}
	return mcp.NewClient(ctx, mcp.Config{Command: mcpCmd, ProjectRoot: project})
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":7070", "HTTP listen address")
	mcpCmd := fs.String("mcp-command", "dits-mcp", "command that launches the DITS MCP server")
	project := fs.String("project", "", "DITS project root (parent of .dits); empty → stub client")
	schedule := fs.Bool("schedule", false, "run the RE scheduler (outcome assessments, SLA overdue, decision escalation)")
	scheduleInterval := fs.Duration("schedule-interval", scheduler.DefaultInterval, "RE scheduler cycle interval")
	_ = fs.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := dialClient(ctx, *mcpCmd, *project)
	if err != nil {
		log.Fatalf("pilot: dial MCP: %v", err)
	}
	defer client.Close()

	if *schedule {
		sched := scheduler.New(client, *scheduleInterval)
		go func() {
			log.Printf("pilot: RE scheduler running every %s", *scheduleInterval)
			if err := sched.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("pilot: scheduler stopped: %v", err)
			}
		}()
	}

	mux := http.NewServeMux()
	handlers.New(client).Register(mux)

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("pilot: serving on %s (project=%q)", *addr, *project)

	errc := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()
	select {
	case err := <-errc:
		log.Fatalf("pilot: server error: %v", err)
	case <-ctx.Done():
		log.Print("pilot: shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}
}

// runInit applies the embedded RE meta bundle to the target project via the
// dits_meta_apply MCP tool (plan §10.1). Idempotent at the substrate level.
func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	mcpCmd := fs.String("mcp-command", "dits-mcp", "command that launches the DITS MCP server")
	project := fs.String("project", "", "DITS project root (parent of .dits) — required")
	_ = fs.Parse(args)
	if *project == "" {
		log.Fatal("pilot init: --project is required")
	}
	if err := meta.Validate(); err != nil {
		log.Fatalf("pilot init: embedded RE bundle invalid: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := mcp.NewClient(ctx, mcp.Config{Command: *mcpCmd, ProjectRoot: *project})
	if err != nil {
		log.Fatalf("pilot init: dial MCP: %v", err)
	}
	defer client.Close()

	if err := client.MetaApply(ctx, meta.Bundle()); err != nil {
		log.Fatalf("pilot init: apply RE meta bundle: %v", err)
	}
	fmt.Println("pilot init: applied RE meta bundle (work kinds, workflows, roles, role constraints, taxonomy skeletons)")
}
