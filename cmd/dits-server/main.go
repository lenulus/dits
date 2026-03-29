package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/server"
	"github.com/lenulus/pf/internal/store/sqlite"
)

func main() {
	addr := flag.String("addr", ":8484", "listen address")
	dbPath := flag.String("db", "dits-server.db", "database path")
	projectKey := flag.String("project", "", "project key (required)")
	flag.Parse()

	if *projectKey == "" {
		fmt.Fprintln(os.Stderr, "error: --project is required")
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	db, err := sqlite.Open(*dbPath)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Ensure meta config exists.
	meta, err := db.GetCurrentMeta(context.Background())
	if err != nil {
		logger.Error("failed to get meta", "error", err)
		os.Exit(1)
	}
	if meta == nil {
		m := domain.DefaultMetaConfig(*projectKey)
		if err := db.SaveMeta(context.Background(), &m); err != nil {
			logger.Error("failed to save meta", "error", err)
			os.Exit(1)
		}
		logger.Info("initialized meta config", "project", *projectKey)
	}

	srv := server.New(db, logger)
	logger.Info("dits-server starting", "addr", *addr, "project", *projectKey)
	if err := srv.ListenAndServe(*addr); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
