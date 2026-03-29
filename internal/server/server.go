package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lenulus/pf/internal/blob"
	"github.com/lenulus/pf/internal/store"
	dsync "github.com/lenulus/pf/internal/sync"
)

type Server struct {
	db     store.DB
	blobs  blob.Store
	engine *dsync.Engine
	router chi.Router
	logger *slog.Logger
}

func New(db store.DB, blobs blob.Store, logger *slog.Logger) *Server {
	s := &Server{
		db:     db,
		blobs:  blobs,
		engine: dsync.NewEngine(db),
		logger: logger,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Post("/api/v1/sync", s.handleSync)
	r.Get("/api/v1/health", s.handleHealth)

	// Blob API
	r.Post("/api/v1/blobs/check", s.handleBlobCheck)
	r.Put("/api/v1/blobs/{hash}", s.handleBlobUpload)
	r.Get("/api/v1/blobs/{hash}", s.handleBlobDownload)

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

// --- Blob API ---

type blobCheckRequest struct {
	Hashes []string `json:"hashes"`
}

type blobCheckResponse struct {
	Present []string `json:"present"`
	Missing []string `json:"missing"`
}

func (s *Server) handleBlobCheck(w http.ResponseWriter, r *http.Request) {
	var req blobCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}

	resp := blobCheckResponse{
		Present: []string{},
		Missing: []string{},
	}

	for _, h := range req.Hashes {
		exists, err := s.blobs.Has(r.Context(), h)
		if err != nil {
			s.logger.Error("blob check failed", "hash", h, "error", err)
			s.jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if exists {
			resp.Present = append(resp.Present, h)
		} else {
			resp.Missing = append(resp.Missing, h)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleBlobUpload(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		s.jsonError(w, "missing hash", http.StatusBadRequest)
		return
	}

	// Enforce size limit.
	r.Body = http.MaxBytesReader(w, r.Body, blob.MaxBlobSize+1)

	if err := s.blobs.Put(r.Context(), hash, r.Body); err != nil {
		s.logger.Error("blob upload failed", "hash", hash, "error", err)
		s.jsonError(w, fmt.Sprintf("upload failed: %v", err), http.StatusBadRequest)
		return
	}

	s.logger.Info("blob uploaded", "hash", hash)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "hash": hash})
}

func (s *Server) handleBlobDownload(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		s.jsonError(w, "missing hash", http.StatusBadRequest)
		return
	}

	rc, err := s.blobs.Get(r.Context(), hash)
	if err != nil {
		s.jsonError(w, "blob not found", http.StatusNotFound)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, rc)
}

func (s *Server) jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
