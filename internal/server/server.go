package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lenulus/pf/internal/store"
	dsync "github.com/lenulus/pf/internal/sync"
)

type Server struct {
	db     store.DB
	engine *dsync.Engine
	router chi.Router
	logger *slog.Logger
}

func New(db store.DB, logger *slog.Logger) *Server {
	s := &Server{
		db:     db,
		engine: dsync.NewEngine(db),
		logger: logger,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Post("/api/v1/sync", s.handleSync)
	r.Get("/api/v1/health", s.handleHealth)

	s.router = r
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) ListenAndServe(addr string) error {
	s.logger.Info("server starting", "addr", addr)
	return http.ListenAndServe(addr, s)
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	var req dsync.SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	s.logger.Info("sync request",
		"node_id", req.NodeID,
		"project", req.ProjectKey,
		"events_pushed", len(req.Events),
		"heads", len(req.Heads),
	)

	resp, err := s.engine.HandleSync(r.Context(), req)
	if err != nil {
		s.logger.Error("sync failed", "error", err)
		s.jsonError(w, fmt.Sprintf("sync failed: %v", err), http.StatusInternalServerError)
		return
	}

	s.logger.Info("sync response",
		"events_pulled", len(resp.Events),
		"shared_ids", len(resp.SharedIDs),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
