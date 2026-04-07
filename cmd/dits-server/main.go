package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lenulus/pf/internal/blob"
	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/logging"
	"github.com/lenulus/pf/internal/server"
	"github.com/lenulus/pf/internal/store/sqlite"
)

func main() {
	addr := flag.String("addr", ":8484", "listen address")
	dbPath := flag.String("db", "dits-server.db", "database path")
	blobDir := flag.String("blobs", "./blobs", "blob storage directory")
	projectKey := flag.String("project", "", "project key (required)")
	signatureMode := flag.String("signature-mode", "warn", "signature enforcement: warn, reject, or ignore")
	logLevel := flag.String("log-level", "info", "log level: error, warn, info, debug, trace")
	logFormat := flag.String("log-format", "text", "log format: text or json")
	flag.Parse()

	if *projectKey == "" {
		fmt.Fprintln(os.Stderr, "error: --project is required")
		os.Exit(1)
	}

	level, err := logging.Parse(*logLevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	logger := logging.New(os.Stdout, level, *logFormat)

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

	blobs, err := blob.NewFSStore(*blobDir)
	if err != nil {
		logger.Error("failed to create blob store", "error", err)
		os.Exit(1)
	}

	srv := server.New(db, blobs, logger)
	srv.SetSignatureMode(*signatureMode)
	httpSrv := &http.Server{
		Addr:    *addr,
		Handler: srv,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	done := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logger.Info("shutting down", "signal", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(ctx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
		close(done)
	}()

	logger.Info("dits-server starting", "addr", *addr, "project", *projectKey)
	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
	<-done
	logger.Info("server stopped")
}
