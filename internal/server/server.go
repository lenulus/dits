package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lenulus/pf/internal/blob"
	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/logging"
	"github.com/lenulus/pf/internal/store"
	dsync "github.com/lenulus/pf/internal/sync"
)

type Server struct {
	db       store.DB
	blobs    blob.Store
	engine   *dsync.Engine
	router   chi.Router
	logger   *slog.Logger
	syncLock sync.Mutex // serializes sync handlers; SQLite permits one writer
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

	// v1 protocol endpoints (sync, blobs)
	r.Post("/api/v1/sync", s.handleSync)
	r.Get("/api/v1/health", s.handleHealth)
	r.Post("/api/v1/blobs/check", s.handleBlobCheck)
	r.Put("/api/v1/blobs/{hash}", s.handleBlobUpload)
	r.Get("/api/v1/blobs/{hash}", s.handleBlobDownload)

	// v2 query API
	r.Route("/api/v2", func(r chi.Router) {
		r.Get("/work", s.handleListWorkItemsV2)
		r.Get("/work/{id}", s.handleGetWorkItemV2)
		r.Get("/work/{id}/events", s.handleWorkItemEvents)
		r.Get("/work/{id}/artifacts", s.handleWorkItemArtifacts)
		r.Get("/work/{id}/attempts", s.handleWorkItemAttempts)
		r.Get("/work/{id}/checkpoints", s.handleWorkItemCheckpoints)
		r.Get("/work/{id}/evals", s.handleWorkItemEvals)
		r.Get("/work/{id}/outcomes", s.handleWorkItemOutcomes)
		r.Get("/events", s.handleListEvents)
		r.Get("/meta", s.handleGetMeta)
	})

	s.router = r
	return s
}

// SetSignatureMode configures signature enforcement on the sync engine.
func (s *Server) SetSignatureMode(mode string) {
	s.engine.SetSignatureMode(mode)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) ListenAndServe(addr string) error {
	s.logger.Info("server starting", "addr", addr)
	return http.ListenAndServe(addr, s)
}

// --- Sync ---

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	var req dsync.SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Sync ingests events inside multiple transactions on independent
	// connections. SQLite only allows one writer at a time, so two
	// concurrent sync requests race for the write lock and one fails with
	// SQLITE_BUSY even with _busy_timeout set. Serialize sync at the
	// handler boundary — read endpoints (v2 query API, blob GETs) stay
	// concurrent because WAL keeps reads non-blocking.
	s.syncLock.Lock()
	defer s.syncLock.Unlock()

	// Promote chi's RequestID into the logging-package context key so the
	// shared ContextHandler emits it as request_id on every record. Lets
	// a single ULID grep stitch worker MCP log + judge MCP log + this
	// server's log together for one round-trip.
	ctx := logging.WithRequestID(r.Context(), middleware.GetReqID(r.Context()))

	s.logger.InfoContext(ctx, "sync request",
		slog.String("node_id", string(req.NodeID)),
		slog.String("actor_id", string(req.ActorID)),
		slog.String("project", req.ProjectKey),
		slog.Int("events_pushed", len(req.Events)),
		slog.Int("heads", len(req.Heads)),
	)

	resp, err := s.engine.HandleSync(ctx, req)
	if err != nil {
		s.logger.ErrorContext(ctx, "sync failed",
			slog.String("actor_id", string(req.ActorID)),
			slog.Any("err", err),
		)
		s.jsonError(w, fmt.Sprintf("sync failed: %v", err), http.StatusInternalServerError)
		return
	}

	s.logger.InfoContext(ctx, "sync response",
		slog.String("actor_id", string(req.ActorID)),
		slog.Int("events_pulled", len(resp.Events)),
		slog.Int("shared_ids", len(resp.SharedIDs)),
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

	resp := blobCheckResponse{Present: []string{}, Missing: []string{}}
	for _, h := range req.Hashes {
		exists, err := s.blobs.Has(r.Context(), h)
		if err != nil {
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

	r.Body = http.MaxBytesReader(w, r.Body, blob.MaxBlobSize+1)
	if err := s.blobs.Put(r.Context(), hash, r.Body); err != nil {
		s.jsonError(w, fmt.Sprintf("upload failed: %v", err), http.StatusBadRequest)
		return
	}

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

// --- v2 Query API ---

func (s *Server) handleListWorkItemsV2(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.WorkItemFilter{}

	if v := q.Get("status"); v != "" {
		filter.Status = v
	}
	if v := q.Get("kind"); v != "" {
		filter.Kind = v
	}
	if v := q.Get("label"); v != "" {
		filter.Label = v
	}
	if v := q.Get("claimed_by"); v != "" {
		filter.ClaimedBy = domain.ActorID(v)
	}
	if v := q.Get("blocked"); v != "" {
		b := v == "true"
		filter.Blocked = &b
	}
	if v := q.Get("q"); v != "" {
		filter.Query = v
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Offset = n
		}
	}

	// ready=true: open-category statuses + no lease + not blocked
	if q.Get("ready") == "true" {
		meta, err := s.db.GetCurrentMeta(r.Context())
		if err != nil {
			s.jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if meta != nil {
			var openStatuses []string
			for _, wf := range meta.Workflows {
				for _, st := range wf.Statuses {
					if st.Category == "open" {
						openStatuses = append(openStatuses, st.Slug)
					}
				}
			}
			filter.Statuses = openStatuses
		}
		blocked := false
		filter.Blocked = &blocked
		// lease_holder IS NULL is handled by checking ClaimedBy is empty + adding explicit NULL condition
		// We add a special Statuses filter and rely on the store to also filter lease_holder IS NULL
		// For simplicity, use the Ready flag
		ready := true
		filter.Ready = &ready
	}

	items, err := s.db.ListWorkItems(r.Context(), filter)
	if err != nil {
		s.jsonError(w, fmt.Sprintf("listing work items: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"work_items": items, "count": len(items)})
}

func (s *Server) handleGetWorkItemV2(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	wi, err := s.db.GetWorkItemBySharedID(r.Context(), domain.SharedID(id))
	if err != nil {
		s.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if wi == nil {
		wi, err = s.db.GetWorkItem(r.Context(), domain.WorkItemID(id))
		if err != nil {
			s.jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	if wi == nil {
		s.jsonError(w, "work item not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(wi)
}

func (s *Server) handleWorkItemEvents(w http.ResponseWriter, r *http.Request) {
	wi := s.resolveWorkItem(w, r)
	if wi == nil {
		return
	}

	q := r.URL.Query()
	filter := store.EventFilter{
		WorkItemID: wi.ID,
		Limit:      100,
	}
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Since = t
		}
	}
	if v := q.Get("type"); v != "" {
		filter.Type = domain.EventType(v)
	}

	events, err := s.db.ListEvents(r.Context(), filter)
	if err != nil {
		s.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"events": events, "count": len(events)})
}

func (s *Server) handleWorkItemArtifacts(w http.ResponseWriter, r *http.Request) {
	wi := s.resolveWorkItem(w, r)
	if wi == nil {
		return
	}

	q := r.URL.Query()
	artType := q.Get("type")
	role := q.Get("role")

	artifacts := wi.Artifacts
	if artType != "" || role != "" {
		var filtered []domain.Artifact
		for _, a := range artifacts {
			if artType != "" && a.ArtifactType != artType {
				continue
			}
			if role != "" && a.SemanticRole != role {
				continue
			}
			filtered = append(filtered, a)
		}
		artifacts = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"artifacts": artifacts, "count": len(artifacts)})
}

func (s *Server) handleWorkItemAttempts(w http.ResponseWriter, r *http.Request) {
	wi := s.resolveWorkItem(w, r)
	if wi == nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"attempts": wi.Attempts, "count": len(wi.Attempts)})
}

func (s *Server) handleWorkItemCheckpoints(w http.ResponseWriter, r *http.Request) {
	wi := s.resolveWorkItem(w, r)
	if wi == nil {
		return
	}

	checkpoints := wi.Checkpoints
	if attemptID := r.URL.Query().Get("attempt_id"); attemptID != "" {
		var filtered []domain.Checkpoint
		for _, cp := range checkpoints {
			if string(cp.AttemptID) == attemptID {
				filtered = append(filtered, cp)
			}
		}
		checkpoints = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"checkpoints": checkpoints, "count": len(checkpoints)})
}

func (s *Server) handleWorkItemEvals(w http.ResponseWriter, r *http.Request) {
	wi := s.resolveWorkItem(w, r)
	if wi == nil {
		return
	}

	evals := wi.Evals
	q := r.URL.Query()
	if v := q.Get("subject_kind"); v != "" {
		var filtered []domain.Eval
		for _, ev := range evals {
			if ev.SubjectKind == v {
				filtered = append(filtered, ev)
			}
		}
		evals = filtered
	}
	if v := q.Get("verdict"); v != "" {
		var filtered []domain.Eval
		for _, ev := range evals {
			if ev.Verdict == v {
				filtered = append(filtered, ev)
			}
		}
		evals = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"evals": evals, "count": len(evals)})
}

func (s *Server) handleWorkItemOutcomes(w http.ResponseWriter, r *http.Request) {
	wi := s.resolveWorkItem(w, r)
	if wi == nil {
		return
	}

	outcomes := wi.Outcomes
	if v := r.URL.Query().Get("decision"); v != "" {
		var filtered []domain.Outcome
		for _, oc := range outcomes {
			if oc.Decision == v {
				filtered = append(filtered, oc)
			}
		}
		outcomes = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"outcomes": outcomes, "count": len(outcomes)})
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.EventFilter{Limit: 100}

	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Since = t
		}
	}
	if v := q.Get("type"); v != "" {
		filter.Type = domain.EventType(v)
	}
	if v := q.Get("actor_id"); v != "" {
		filter.ActorID = domain.ActorID(v)
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.Limit = n
		}
	}

	events, err := s.db.ListEvents(r.Context(), filter)
	if err != nil {
		s.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"events": events, "count": len(events)})
}

func (s *Server) handleGetMeta(w http.ResponseWriter, r *http.Request) {
	meta, err := s.db.GetCurrentMeta(r.Context())
	if err != nil {
		s.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if meta == nil {
		s.jsonError(w, "no meta configuration", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meta)
}

// --- Helpers ---

func (s *Server) resolveWorkItem(w http.ResponseWriter, r *http.Request) *domain.WorkItem {
	id := chi.URLParam(r, "id")

	wi, err := s.db.GetWorkItemBySharedID(r.Context(), domain.SharedID(id))
	if err != nil {
		s.jsonError(w, "internal error", http.StatusInternalServerError)
		return nil
	}
	if wi == nil {
		wi, err = s.db.GetWorkItem(r.Context(), domain.WorkItemID(id))
		if err != nil {
			s.jsonError(w, "internal error", http.StatusInternalServerError)
			return nil
		}
	}
	if wi == nil {
		s.jsonError(w, "work item not found", http.StatusNotFound)
		return nil
	}
	return wi
}

func (s *Server) jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
