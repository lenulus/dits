// Command pilot is the Pilot web frontend for DITS (Radical Execution
// methodology). It is a separate binary that talks to the DITS substrate
// ONLY over MCP — it never imports DITS-internal packages such as
// internal/domain, internal/workops, internal/mcp, etc. See
// docs/proposals/radical-execution-implementation-plan-v2.md §2 (topology)
// and §8.1 (binary layout).
//
// Phase-4 skeleton: flag parsing + an HTTP server stub wired to the web
// route table. No real OAuth, signing, MCP, SQLite, or scheduler yet.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lenulus/pf/internal/pilot/web/handlers"
)

func main() {
	// TODO(phase 5/6): add flags for OAuth provider config, MCP endpoint,
	// SQLite path, KMS/master-key config, and scheduler interval.
	addr := flag.String("addr", ":7070", "HTTP listen address")
	flag.Parse()

	log.Printf("pilot skeleton — not yet implemented (listening on %s)", *addr)

	mux := http.NewServeMux()
	// TODO(phase 5): register the typed MCP client, auth middleware,
	// session store, and template-backed handlers. For now the route
	// table maps the twelve UI paths to placeholder handlers.
	handlers.Register(mux)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("pilot: graceful shutdown failed: %v", err)
		}
	}
}
