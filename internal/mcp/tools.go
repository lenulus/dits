package mcp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/lenulus/pf/internal/constraints"
	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/logging"
	"github.com/lenulus/pf/internal/store"
	"github.com/lenulus/pf/internal/workops"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/oklog/ulid/v2"
)

// newRequestID returns a fresh ULID string used as the per-tool-call
// request identifier. Threaded through context.Context so workops and
// store-level log lines tagged with the same id reconstruct one Claude
// Code action across the stack.
func newRequestID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
}

// argKeys returns a sorted slice of argument keys, used at INFO so we can
// see "what fields were passed" without logging the values themselves.
func argKeys(args map[string]any) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for k := range args {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// redactedArgs returns the safe-to-log subset of args used at DEBUG.
// Values for high-sensitivity fields (body, payload, plan, summary,
// statement, context, error, metrics_json) are intentionally excluded;
// only id-shaped fields and titles round-trip.
func redactedArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	allow := map[string]bool{
		"id": true, "target": true, "actor": true, "to": true,
		"status": true, "kind": true, "scope": true, "verdict": true,
		"subject": true, "subject_kind": true, "rubric_ref": true,
		"eval_id": true, "eval_ref": true, "type": true, "title": true,
		"server": true, "claimed_by": true, "blocked": true,
		"include_closed": true, "ready": true, "retryable": true,
		"progress": true, "confidence": true,
	}
	out := make(map[string]any, len(allow))
	for k, v := range args {
		if allow[k] {
			out[k] = v
		}
	}
	return out
}

// openOps opens a workops handle, preferring the configured project root.
// The configured logger is attached so workops emits its own structured
// lines (event-appended, sync warnings, etc.) under the same sink.
func openOps(cfg Config) (*workops.WorkOps, error) {
	var (
		w   *workops.WorkOps
		err error
	)
	if cfg.ProjectRoot != "" {
		w, err = workops.OpenAt(cfg.ProjectRoot)
	} else {
		w, err = workops.Open()
	}
	if err != nil {
		return nil, err
	}
	w.SetLogger(cfg.logger())
	return w, nil
}

// jsonResult marshals v as an indented JSON string and wraps it as a tool result.
func jsonResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func errResult(err error) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(err.Error()), nil
}

type handler func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error)

// withOps is the middleware for every dits_* tool call. It:
//   - mints a request_id (ULID) and threads it through context.Context so
//     downstream workops/store log lines carry the same id;
//   - opens a fresh WorkOps tied to the configured project root;
//   - logs an INFO "tool start" before dispatch and INFO "tool ok" or
//     ERROR "tool failed" after, with tool, dur_ms, request_id, err;
//   - at INFO logs only the *keys* of the tool arguments — values can
//     contain user prompts and bodies, so we redact by default. At DEBUG
//     a safelisted subset is logged; at TRACE the full request payload.
func withOps(cfg Config, h handler) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logger := cfg.logger()
		reqID := newRequestID()
		ctx = logging.WithRequestID(ctx, reqID)

		toolName := req.Params.Name
		args := req.GetArguments()

		actorID := ""
		w, err := openOps(cfg)
		if err != nil {
			logger.ErrorContext(ctx, "tool failed",
				slog.String("tool", toolName),
				slog.Any("arg_keys", argKeys(args)),
				slog.Any("err", err),
			)
			return errResult(err)
		}
		defer w.Shutdown()
		actorID = string(w.Proj.Config.ActorID)

		logger.InfoContext(ctx, "tool start",
			slog.String("tool", toolName),
			slog.String("actor_id", actorID),
			slog.Any("arg_keys", argKeys(args)),
		)
		if logger.Enabled(ctx, slog.LevelDebug) {
			logger.DebugContext(ctx, "tool args (redacted)",
				slog.String("tool", toolName),
				slog.Any("args", redactedArgs(args)),
			)
		}
		if logger.Enabled(ctx, logging.LevelTrace) {
			logger.Log(ctx, logging.LevelTrace, "tool args (full)",
				slog.String("tool", toolName),
				slog.Any("args", args),
			)
		}

		start := time.Now()
		res, hErr := h(ctx, w, req)
		dur := time.Since(start)

		switch {
		case hErr != nil:
			logger.ErrorContext(ctx, "tool failed",
				slog.String("tool", toolName),
				slog.String("actor_id", actorID),
				slog.Int64("dur_ms", dur.Milliseconds()),
				slog.Any("err", hErr),
			)
		case res != nil && res.IsError:
			logger.ErrorContext(ctx, "tool failed",
				slog.String("tool", toolName),
				slog.String("actor_id", actorID),
				slog.Int64("dur_ms", dur.Milliseconds()),
				slog.String("err", "tool returned error result"),
			)
		default:
			logger.InfoContext(ctx, "tool ok",
				slog.String("tool", toolName),
				slog.String("actor_id", actorID),
				slog.Int64("dur_ms", dur.Milliseconds()),
			)
		}
		return res, hErr
	}
}

// resolveID looks up a work item by shared or work item ID and returns its
// canonical WorkItemID, or an error result ready to return.
func resolveID(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (domain.WorkItemID, *mcp.CallToolResult, error) {
	ref := req.GetString("id", "")
	if ref == "" {
		return "", mcp.NewToolResultError("id is required"), nil
	}
	wi, err := w.ResolveWorkItem(ctx, ref)
	if err != nil {
		r, _ := errResult(err)
		return "", r, nil
	}
	return wi.ID, nil, nil
}

// hasRoleBinding reports whether the work item has a binding for role; if
// actor is non-empty it must also match the bound actor.
func hasRoleBinding(wi domain.WorkItem, role, actor string) bool {
	for _, rb := range wi.RoleBindings {
		if rb.RoleSlug == role {
			return actor == "" || string(rb.Actor) == actor
		}
	}
	return false
}

// hasClassification reports whether the work item is classified in taxonomy;
// if node is non-empty the classification node must have it as a slug prefix.
func hasClassification(wi domain.WorkItem, taxonomy, node string) bool {
	for _, cl := range wi.Classifications {
		if cl.TaxonomySlug != taxonomy {
			continue
		}
		if node == "" || strings.HasPrefix(cl.NodeSlug, node) {
			return true
		}
	}
	return false
}

// latestAckRollup returns the alignment rollup of the most recently filed ACK
// on the work item, or "" if there is none.
func latestAckRollup(wi domain.WorkItem) domain.AckRollup {
	if len(wi.Acks) == 0 {
		return ""
	}
	latest := wi.Acks[0]
	for _, a := range wi.Acks[1:] {
		if a.FiledAt.After(latest.FiledAt) {
			latest = a
		}
	}
	return domain.ComputeAckRollup(latest.Specifier, latest.Builder)
}

// paginate applies offset then limit to a slice. offset/limit <= 0 are no-ops
// for that dimension.
func paginate(items []domain.WorkItem, offset, limit int) []domain.WorkItem {
	if offset > 0 {
		if offset >= len(items) {
			return []domain.WorkItem{}
		}
		items = items[offset:]
	}
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items
}

// applyMetaMutation loads the current meta, applies a mutation that is
// expected to bump Version, and saves it conditionally on the prior version
// (optimistic concurrency). Returns the saved meta as the tool result.
func applyMetaMutation(ctx context.Context, w *workops.WorkOps, mutate func(*domain.MetaConfig) error) (*mcp.CallToolResult, error) {
	meta, err := w.LoadMeta(ctx)
	if err != nil {
		return errResult(err)
	}
	prevVersion := meta.Version
	if err := mutate(meta); err != nil {
		return errResult(err)
	}
	if err := w.Proj.DB.SaveMetaIfVersion(ctx, meta, prevVersion); err != nil {
		return errResult(err)
	}
	return jsonResult(meta)
}

func registerTools(s *server.MCPServer, cfg Config) {
	// ---------- Read-only tools ----------

	s.AddTool(mcp.NewTool("dits_work_list",
		mcp.WithDescription("List work items with optional filters (status, kind, ready, include_closed, "+
			"role/role_actor, taxonomy/node, alignment, limit/offset)."),
		mcp.WithString("status", mcp.Description("Filter by status")),
		mcp.WithString("kind", mcp.Description("Filter by kind")),
		mcp.WithBoolean("ready", mcp.Description("Only items that are ready (unleased, unblocked, open)")),
		mcp.WithString("claimed_by", mcp.Description("Filter by lease holder actor ID")),
		mcp.WithBoolean("blocked", mcp.Description("Filter by blocked state")),
		mcp.WithBoolean("include_closed", mcp.Description("Include closed items")),
		mcp.WithString("role", mcp.Description("Filter to items with a binding for this role slug")),
		mcp.WithString("role_actor", mcp.Description("With role: require this actor bound to that role")),
		mcp.WithString("taxonomy", mcp.Description("Filter to items classified in this taxonomy")),
		mcp.WithString("node", mcp.Description("With taxonomy: prefix-match classification node slug")),
		mcp.WithString("alignment", mcp.Description("Filter by latest ACK rollup: aligned | builder_pending | "+
			"specifier_pending | both_pending | rejected")),
		mcp.WithNumber("limit", mcp.Description("Max items to return (applied after filtering)")),
		mcp.WithNumber("offset", mcp.Description("Skip this many items before limit")),
		readOnly(),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filter := store.WorkItemFilter{
			Status: req.GetString("status", ""),
			Kind:   req.GetString("kind", ""),
		}
		items, err := w.ListWorkItems(ctx, filter, req.GetBool("include_closed", false))
		if err != nil {
			return errResult(err)
		}
		ready := req.GetBool("ready", false)
		claimedBy := req.GetString("claimed_by", "")
		wantBlocked := req.GetBool("blocked", false)
		gotBlockedFlag := false
		if _, ok := req.GetArguments()["blocked"]; ok {
			gotBlockedFlag = true
		}
		role := req.GetString("role", "")
		roleActor := req.GetString("role_actor", "")
		taxonomy := req.GetString("taxonomy", "")
		node := req.GetString("node", "")
		alignment := req.GetString("alignment", "")

		// When ready=true, defer to the canonical domain.IsReady contract
		// (blocked + leased + running-attempt + open-status checks). The
		// inline filter this used to do was a strict subset and could
		// drift from the v2 list endpoint's definition.
		var openStatuses []string
		if ready {
			meta, err := w.LoadMeta(ctx)
			if err != nil {
				return errResult(err)
			}
			openStatuses = meta.OpenStatuses()
		}

		var out []domain.WorkItem
		for i := range items {
			wi := items[i]
			if ready && !domain.IsReady(&wi, openStatuses) {
				continue
			}
			if claimedBy != "" {
				if wi.LeaseHolder == nil || string(*wi.LeaseHolder) != claimedBy {
					continue
				}
			}
			if gotBlockedFlag && wi.Blocked != wantBlocked {
				continue
			}
			if role != "" && !hasRoleBinding(wi, role, roleActor) {
				continue
			}
			if taxonomy != "" && !hasClassification(wi, taxonomy, node) {
				continue
			}
			if alignment != "" && string(latestAckRollup(wi)) != alignment {
				continue
			}
			out = append(out, wi)
		}
		out = paginate(out, int(req.GetFloat("offset", 0)), int(req.GetFloat("limit", 0)))
		if out == nil {
			out = []domain.WorkItem{}
		}
		return jsonResult(out)
	}))

	s.AddTool(mcp.NewTool("dits_work_show",
		mcp.WithDescription("Show reduced state for a single work item."),
		mcp.WithString("id", mcp.Description("Work item ID or shared ID"), mcp.Required()),
		readOnly(),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wi, err := w.ResolveWorkItem(ctx, req.GetString("id", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_events",
		mcp.WithDescription("Return the full event log for a work item."),
		mcp.WithString("id", mcp.Description("Work item ID or shared ID"), mcp.Required()),
		mcp.WithString("type", mcp.Description("Filter by event type")),
		readOnly(),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		events, err := w.GetEvents(ctx, id)
		if err != nil {
			return errResult(err)
		}
		filter := req.GetString("type", "")
		if filter != "" {
			var filtered []domain.Event
			for _, e := range events {
				if string(e.Type) == filter {
					filtered = append(filtered, e)
				}
			}
			events = filtered
		}
		return jsonResult(events)
	}))

	s.AddTool(mcp.NewTool("dits_meta_show",
		mcp.WithDescription("Show the current meta configuration (allowed kinds, labels, statuses)."),
		readOnly(),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		meta, err := w.LoadMeta(ctx)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(meta)
	}))

	s.AddTool(mcp.NewTool("dits_identity_show",
		mcp.WithDescription("Show the current actor identity for this project."),
		readOnly(),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(map[string]any{
			"actor_id":    w.Proj.Config.ActorID,
			"project_key": w.Proj.Config.ProjectKey,
		})
	}))

	s.AddTool(mcp.NewTool("dits_role_bindings_list",
		mcp.WithDescription("List the role bindings (role -> actor) materialized on a work item."),
		mcp.WithString("id", mcp.Description("Work item ID or shared ID"), mcp.Required()),
		readOnly(),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wi, err := w.ResolveWorkItem(ctx, req.GetString("id", ""))
		if err != nil {
			return errResult(err)
		}
		bindings := wi.RoleBindings
		if bindings == nil {
			bindings = []domain.RoleBinding{}
		}
		return jsonResult(bindings)
	}))

	s.AddTool(mcp.NewTool("dits_diagnostics_get",
		mcp.WithDescription("Evaluate role constraints against a work item and return diagnostics. "+
			"NOTE: actor org positions are not yet stored in the substrate, so this passes "+
			"nil positions — only position-free predicates fire (distinct_actors, "+
			"requires_classification). Hierarchy predicates (not_reports_to_within, "+
			"classified_in_same_node) need actor positions and stay silent until that store lands "+
			"(design open-question #2)."),
		mcp.WithString("id", mcp.Description("Work item ID or shared ID"), mcp.Required()),
		readOnly(),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wi, err := w.ResolveWorkItem(ctx, req.GetString("id", ""))
		if err != nil {
			return errResult(err)
		}
		meta, err := w.LoadMeta(ctx)
		if err != nil {
			return errResult(err)
		}
		// nil ActorPositions: the substrate has no actor->org-node store yet, so
		// hierarchy-aware predicates are intentionally skipped (open-question #2).
		diags := constraints.Evaluate(wi, meta, nil)
		if diags == nil {
			diags = []domain.Diagnostic{}
		}
		return jsonResult(diags)
	}))

	s.AddTool(mcp.NewTool("dits_events_list",
		mcp.WithDescription("Read the global event log across all work items (EventLogView), "+
			"optionally filtered by type and a since-timestamp (RFC3339). group_by is deferred."),
		mcp.WithString("type", mcp.Description("Filter by event type, e.g. work.role_bound")),
		mcp.WithString("since", mcp.Description("Only events strictly after this RFC3339 timestamp")),
		mcp.WithNumber("limit", mcp.Description("Max events to return (default 100)")),
		readOnly(),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filter := store.EventFilter{
			Type:  domain.EventType(req.GetString("type", "")),
			Limit: int(req.GetFloat("limit", 0)),
		}
		if s := req.GetString("since", ""); s != "" {
			ts, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid since timestamp: %v", err)), nil
			}
			filter.Since = ts
		}
		events, err := w.Proj.DB.ListEvents(ctx, filter)
		if err != nil {
			return errResult(err)
		}
		if events == nil {
			events = []domain.Event{}
		}
		return jsonResult(events)
	}))

	s.AddTool(mcp.NewTool("dits_actor_list",
		mcp.WithDescription("List registered actors (actor_id, public_key, node_id, first_seen)."),
		readOnly(),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		actors, err := w.Proj.DB.ListActors(ctx)
		if err != nil {
			return errResult(err)
		}
		if actors == nil {
			actors = []store.ActorRecord{}
		}
		return jsonResult(actors)
	}))

	// ---------- Mutating tools ----------

	s.AddTool(mcp.NewTool("dits_work_create",
		mcp.WithDescription("Create a new work item."),
		mcp.WithString("title", mcp.Required()),
		mcp.WithString("body"),
		mcp.WithString("kind", mcp.DefaultString("task")),
		mcp.WithArray("labels", mcp.WithStringItems()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := w.CreateWorkItem(ctx,
			req.GetString("kind", "task"),
			req.GetString("title", ""),
			req.GetString("body", ""),
			req.GetStringSlice("labels", nil),
		)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(map[string]any{
			"work_item": res.WorkItem, "shared_id": res.SharedID,
		})
	}))

	s.AddTool(mcp.NewTool("dits_work_lease",
		mcp.WithDescription("Lease a work item for execution."),
		mcp.WithString("id", mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		res, err := w.Lease(ctx, id)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(res)
	}))

	s.AddTool(mcp.NewTool("dits_work_lease_release",
		mcp.WithDescription("Release the current lease on a work item."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("reason"),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.LeaseRelease(ctx, id, req.GetString("reason", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_start",
		mcp.WithDescription("Start a new execution attempt on a work item."),
		mcp.WithString("id", mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		res, err := w.Start(ctx, id)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(res)
	}))

	s.AddTool(mcp.NewTool("dits_work_checkpoint",
		mcp.WithDescription("Record progress on the current attempt."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("summary", mcp.Required()),
		mcp.WithNumber("progress", mcp.DefaultNumber(0)),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Checkpoint(ctx, id, req.GetString("summary", ""), req.GetFloat("progress", 0))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_complete",
		mcp.WithDescription("Complete the current attempt."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("summary"),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Complete(ctx, id, req.GetString("summary", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_fail",
		mcp.WithDescription("Fail the current attempt."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("error", mcp.Required()),
		mcp.WithBoolean("retryable"),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Fail(ctx, id, req.GetString("error", ""), req.GetBool("retryable", false))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_observe",
		mcp.WithDescription("Record an investigation observation. An optional opaque JSON data blob rides alongside (methodology-agnostic: a consumer uses it for a typed discriminator)."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("summary", mcp.Required()),
		mcp.WithString("data", mcp.Description(`Optional opaque JSON blob, e.g. {"entry_type":"risk","severity":"high"}.`)),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		var (
			wi  *domain.WorkItem
			err error
		)
		if raw := strings.TrimSpace(req.GetString("data", "")); raw != "" {
			if !json.Valid([]byte(raw)) {
				return errResult(fmt.Errorf("data: not valid JSON"))
			}
			wi, err = w.ObserveData(ctx, id, req.GetString("summary", ""), json.RawMessage(raw))
		} else {
			wi, err = w.Observe(ctx, id, req.GetString("summary", ""))
		}
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_finding",
		mcp.WithDescription("Record an investigation finding with a confidence score."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("statement", mcp.Required()),
		mcp.WithNumber("confidence", mcp.DefaultNumber(0.5)),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Finding(ctx, id, req.GetString("statement", ""), req.GetFloat("confidence", 0.5))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_attach",
		mcp.WithDescription("Attach a local file as an artifact to a work item."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("path", mcp.Description("Absolute or relative path to the file"), mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		res, err := w.Attach(ctx, id, req.GetString("path", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(res)
	}))

	s.AddTool(mcp.NewTool("dits_work_eval_request",
		mcp.WithDescription("Request a machine evaluation on an attempt/artifact/work item."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("scope", mcp.Required()),
		mcp.WithString("subject_kind", mcp.Description("attempt | artifact | work_item | finding | plan")),
		mcp.WithString("subject", mcp.Description("Subject reference")),
		mcp.WithString("rubric_ref"),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		res, err := w.EvalRequest(ctx, id,
			req.GetString("scope", ""),
			req.GetString("subject_kind", ""),
			req.GetString("subject", ""),
			req.GetString("rubric_ref", ""),
		)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(res)
	}))

	s.AddTool(mcp.NewTool("dits_work_eval_complete",
		mcp.WithDescription("Complete an eval with a verdict and optional metrics."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("eval_id", mcp.Required()),
		mcp.WithString("verdict", mcp.Required(), mcp.Description("pass | fail | partial")),
		mcp.WithString("subject_kind"),
		mcp.WithString("subject"),
		mcp.WithString("rubric_ref"),
		mcp.WithString("summary"),
		mcp.WithString("metrics_json", mcp.Description("Metrics as inline JSON string")),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		var metrics json.RawMessage
		if m := req.GetString("metrics_json", ""); m != "" {
			if !json.Valid([]byte(m)) {
				return mcp.NewToolResultError("metrics_json is not valid JSON"), nil
			}
			metrics = json.RawMessage(m)
		}
		wi, err := w.EvalComplete(ctx, id,
			req.GetString("eval_id", ""),
			req.GetString("subject_kind", ""),
			req.GetString("subject", ""),
			req.GetString("rubric_ref", ""),
			req.GetString("verdict", ""),
			req.GetString("summary", ""),
			metrics,
		)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_retain",
		mcp.WithDescription("Mark an output (attempt/artifact) as the retained result."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("subject", mcp.Required()),
		mcp.WithString("subject_kind", mcp.Required()),
		mcp.WithString("reason"),
		mcp.WithString("eval_ref"),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Retain(ctx, id,
			req.GetString("subject_kind", ""),
			req.GetString("subject", ""),
			req.GetString("reason", ""),
			req.GetString("eval_ref", ""),
		)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_discard",
		mcp.WithDescription("Mark an output as discarded/superseded."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("subject", mcp.Required()),
		mcp.WithString("subject_kind", mcp.Required()),
		mcp.WithString("reason"),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Discard(ctx, id,
			req.GetString("subject_kind", ""),
			req.GetString("subject", ""),
			req.GetString("reason", ""),
		)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_handoff",
		mcp.WithDescription("Hand off a work item to another actor."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("to", mcp.Required()),
		mcp.WithString("context", mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		res, err := w.Handoff(ctx, id,
			domain.ActorID(req.GetString("to", "")),
			req.GetString("context", ""),
		)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(res)
	}))

	s.AddTool(mcp.NewTool("dits_work_link",
		mcp.WithDescription("Add a relation (depends_on, relates_to, etc.) between work items."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("type", mcp.Required()),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target work item ID or shared ID")),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		target, err := w.ResolveWorkItem(ctx, req.GetString("target", ""))
		if err != nil {
			return errResult(fmt.Errorf("target: %w", err))
		}
		wi, err := w.Link(ctx, id, req.GetString("type", ""), target.ID)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_unlink",
		mcp.WithDescription("Remove a relation (depends_on, relates_to, etc.) between work items."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("type", mcp.Required()),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target work item ID or shared ID")),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		target, err := w.ResolveWorkItem(ctx, req.GetString("target", ""))
		if err != nil {
			return errResult(fmt.Errorf("target: %w", err))
		}
		wi, err := w.Unlink(ctx, id, req.GetString("type", ""), target.ID)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_status",
		mcp.WithDescription("Set the status of a work item."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("status", mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wi, err := w.ResolveWorkItem(ctx, req.GetString("id", ""))
		if err != nil {
			return errResult(err)
		}
		newWI, err := w.SetStatus(ctx, wi.ID, wi.Status, req.GetString("status", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(newWI)
	}))

	s.AddTool(mcp.NewTool("dits_work_assign",
		mcp.WithDescription("Assign a work item to an actor."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("actor", mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Assign(ctx, id, domain.ActorID(req.GetString("actor", "")))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_comment",
		mcp.WithDescription("Add a comment to a work item."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("body", mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Comment(ctx, id, req.GetString("body", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_block",
		mcp.WithDescription("Mark a work item as blocked."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("reason", mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Block(ctx, id, req.GetString("reason", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_unblock",
		mcp.WithDescription("Unblock a work item."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("reason"),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Unblock(ctx, id, req.GetString("reason", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_close",
		mcp.WithDescription("Close a work item."),
		mcp.WithString("id", mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Close(ctx, id)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_reopen",
		mcp.WithDescription("Reopen a closed work item."),
		mcp.WithString("id", mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Reopen(ctx, id)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	// ---------- Classification & role bindings ----------

	s.AddTool(mcp.NewTool("dits_work_classify",
		mcp.WithDescription("Classify a work item within a taxonomy node."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("taxonomy", mcp.Required(), mcp.Description("Taxonomy slug, e.g. org / product")),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node slug within the taxonomy")),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Classify(ctx, id, req.GetString("taxonomy", ""), req.GetString("node", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_declassify",
		mcp.WithDescription("Remove a classification from a work item."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("taxonomy", mcp.Required()),
		mcp.WithString("node", mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Declassify(ctx, id, req.GetString("taxonomy", ""), req.GetString("node", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_role_bind",
		mcp.WithDescription("Bind an actor to a named role on a work item. Re-binding the same role replaces the actor."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("role", mcp.Required(), mcp.Description("Role slug, e.g. specifier / builder / pilot")),
		mcp.WithString("actor", mcp.Required(), mcp.Description("Actor ID to bind")),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.BindRole(ctx, id, req.GetString("role", ""), domain.ActorID(req.GetString("actor", "")))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_role_unbind",
		mcp.WithDescription("Remove a role binding from a work item."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("role", mcp.Required()),
		mcp.WithString("actor", mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.UnbindRole(ctx, id, req.GetString("role", ""), domain.ActorID(req.GetString("actor", "")))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	// ---------- Generic projection (methodology-agnostic) ----------

	s.AddTool(mcp.NewTool("dits_work_field_set",
		mcp.WithDescription("Set an opaque scalar projection field on a work item (latest write per field wins). Methodology-agnostic: DITS attaches no meaning to field or value — a consuming methodology decides what e.g. ryg / target / customer_visible mean."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("field", mcp.Required(), mcp.Description("Opaque projection field name, e.g. ryg / target / target_precision / customer_visible")),
		mcp.WithString("value", mcp.Description("Opaque scalar value")),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.FieldSet(ctx, id, req.GetString("field", ""), req.GetString("value", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_work_schedule_set",
		mcp.WithDescription("Replace a work item's staged delivery timeline wholesale (latest write wins). Methodology-agnostic: stage keys/states/precisions are opaque to DITS."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("stages_json", mcp.Required(), mcp.Description(`JSON array of stages: [{"key":"beta","label":"Beta","date":"2026 Q3","precision":"Q","state":"open"}]. Empty array clears the schedule.`)),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		var stages []domain.ScheduleStage
		raw := req.GetString("stages_json", "")
		if strings.TrimSpace(raw) != "" {
			if err := json.Unmarshal([]byte(raw), &stages); err != nil {
				return errResult(fmt.Errorf("stages_json: %w", err))
			}
		}
		wi, err := w.ScheduleSet(ctx, id, stages)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	// ---------- ACK lifecycle ----------

	s.AddTool(mcp.NewTool("dits_ack_file",
		mcp.WithDescription("File a two-party commitment (ACK) on a work item for bilateral acceptance."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("scope_summary", mcp.Required()),
		mcp.WithString("delivery_timing"),
		mcp.WithString("target_outcome"),
		mcp.WithString("acceptance_criteria"),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.AckFile(ctx, id,
			req.GetString("scope_summary", ""),
			req.GetString("delivery_timing", ""),
			req.GetString("target_outcome", ""),
			req.GetString("acceptance_criteria", ""),
		)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_ack_accept",
		mcp.WithDescription("Record that one side (specifier|builder) stands behind the current commitment."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("who", mcp.Required(), mcp.Description("specifier | builder")),
		mcp.WithString("note"),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.AckAccept(ctx, id, req.GetString("who", ""), req.GetString("note", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_ack_reject",
		mcp.WithDescription("Record that one side (specifier|builder) rejects the current commitment."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("who", mcp.Required(), mcp.Description("specifier | builder")),
		mcp.WithString("note"),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.AckReject(ctx, id, req.GetString("who", ""), req.GetString("note", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	s.AddTool(mcp.NewTool("dits_ack_amend",
		mcp.WithDescription("Amend the current commitment. A material amendment (scope_change|timeline_change|"+
			"target_change) after acceptance auto-clears both sides to pending; clarification never clears."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("amendment_type", mcp.Required(),
			mcp.Description("scope_change | timeline_change | target_change | clarification")),
		mcp.WithArray("fields", mcp.WithStringItems(), mcp.Description("Which commitment fields changed")),
		mcp.WithString("reason"),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.AckAmend(ctx, id,
			req.GetString("amendment_type", ""),
			req.GetStringSlice("fields", nil),
			req.GetString("reason", ""),
		)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	// ---------- Identity ----------

	s.AddTool(mcp.NewTool("dits_actor_register",
		mcp.WithDescription("Register (or update) an actor's public key and optional node ID."),
		mcp.WithString("actor_id", mcp.Required()),
		mcp.WithString("public_key", mcp.Required()),
		mcp.WithString("node_id"),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		actorID := req.GetString("actor_id", "")
		if actorID == "" {
			return mcp.NewToolResultError("actor_id is required"), nil
		}
		if err := w.Proj.DB.RegisterActor(ctx, domain.ActorID(actorID),
			req.GetString("public_key", ""), domain.NodeID(req.GetString("node_id", ""))); err != nil {
			return errResult(err)
		}
		return jsonResult(map[string]any{"registered": actorID})
	}))

	s.AddTool(mcp.NewTool("dits_review_request",
		mcp.WithDescription("Request a review on a work item targeting a reviewer role (e.g. leadership). "+
			"Emits work.review_requested with the role; the methodology routes it to whoever holds that role. "+
			"Used by Pilot's scheduler to escalate idle decisions to Leadership."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("reviewer_role", mcp.Required(), mcp.Description("Role slug to route the review to, e.g. leadership")),
		mcp.WithString("scope", mcp.Description("Free-form reason for the review, e.g. decision_idle")),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		res, err := w.ReviewRequest(ctx, id, req.GetString("reviewer_role", ""), req.GetString("scope", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(res)
	}))

	s.AddTool(mcp.NewTool("dits_event_submit",
		mcp.WithDescription("Append an externally-signed event after verifying its Ed25519 signature against the "+
			"actor's registered public key. Rejects on a missing/invalid signature or an unknown actor. This is the "+
			"per-user custodial-signing path: a client (e.g. Pilot) constructs and signs an event as a specific actor "+
			"and submits it, so the appended event is attributed to that actor — not to the dits-mcp project actor."),
		mcp.WithString("signed_event_json", mcp.Required(),
			mcp.Description("A complete, signed domain.Event as a JSON string (with a non-empty signature).")),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw := strings.TrimSpace(req.GetString("signed_event_json", ""))
		if raw == "" {
			return mcp.NewToolResultError("signed_event_json is required"), nil
		}
		var evt domain.Event
		if err := json.Unmarshal([]byte(raw), &evt); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid signed_event_json: %v", err)), nil
		}
		wi, err := w.SubmitSignedEvent(ctx, evt)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(wi)
	}))

	// ---------- Meta administration ----------

	s.AddTool(mcp.NewTool("dits_meta_apply",
		mcp.WithDescription("Replace the project meta with the supplied MetaConfig JSON, saved at the next "+
			"version. NOTE: this is a full replace; Phase 6 will refine to a diff-apply."),
		mcp.WithString("meta_json", mcp.Required(), mcp.Description("A full domain.MetaConfig as a JSON string")),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw := req.GetString("meta_json", "")
		if raw == "" {
			return mcp.NewToolResultError("meta_json is required"), nil
		}
		var next domain.MetaConfig
		if err := json.Unmarshal([]byte(raw), &next); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid meta_json: %v", err)), nil
		}
		cur, err := w.LoadMeta(ctx)
		if err != nil {
			return errResult(err)
		}
		next.Version = cur.Version + 1
		if next.ProjectKey == "" {
			next.ProjectKey = cur.ProjectKey
		}
		if err := w.Proj.DB.SaveMeta(ctx, &next); err != nil {
			return errResult(err)
		}
		return jsonResult(&next)
	}))

	s.AddTool(mcp.NewTool("dits_taxonomy_node_add",
		mcp.WithDescription("Add a node to an existing taxonomy in meta (bumps meta version)."),
		mcp.WithString("taxonomy", mcp.Required()),
		mcp.WithString("slug", mcp.Required()),
		mcp.WithString("name", mcp.Required()),
		mcp.WithString("parent_slug"),
		mcp.WithString("metadata", mcp.Description("Optional opaque JSON metadata blob stored on the node, e.g. {\"result\":\"achieved\",\"resultNote\":\"...\"} for a goals node.")),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var metadata json.RawMessage
		if raw := strings.TrimSpace(req.GetString("metadata", "")); raw != "" {
			if !json.Valid([]byte(raw)) {
				return errResult(fmt.Errorf("metadata: not valid JSON"))
			}
			metadata = json.RawMessage(raw)
		}
		return applyMetaMutation(ctx, w, func(m *domain.MetaConfig) error {
			return m.AddTaxonomyNode(req.GetString("taxonomy", ""), domain.TaxonomyNode{
				Slug:       req.GetString("slug", ""),
				Name:       req.GetString("name", ""),
				ParentSlug: req.GetString("parent_slug", ""),
				Metadata:   metadata,
			})
		})
	}))

	s.AddTool(mcp.NewTool("dits_taxonomy_node_set",
		mcp.WithDescription("Set the opaque JSON metadata blob on an existing taxonomy node (bumps meta version). Methodology-agnostic: DITS attaches no meaning to it — a methodology stores e.g. {result, resultNote} for a goals node there."),
		mcp.WithString("taxonomy", mcp.Required()),
		mcp.WithString("slug", mcp.Required()),
		mcp.WithString("metadata", mcp.Required(), mcp.Description("Opaque JSON metadata blob, e.g. {\"result\":\"achieved\",\"resultNote\":\"1 incident\"}.")),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw := strings.TrimSpace(req.GetString("metadata", ""))
		if raw == "" || !json.Valid([]byte(raw)) {
			return errResult(fmt.Errorf("metadata: not valid JSON"))
		}
		return applyMetaMutation(ctx, w, func(m *domain.MetaConfig) error {
			return m.SetTaxonomyNodeMetadata(req.GetString("taxonomy", ""), req.GetString("slug", ""), json.RawMessage(raw))
		})
	}))

	s.AddTool(mcp.NewTool("dits_taxonomy_node_move",
		mcp.WithDescription("Re-parent a taxonomy node in meta (bumps meta version)."),
		mcp.WithString("taxonomy", mcp.Required()),
		mcp.WithString("slug", mcp.Required()),
		mcp.WithString("new_parent_slug", mcp.Description("New parent slug; empty makes it a root node")),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return applyMetaMutation(ctx, w, func(m *domain.MetaConfig) error {
			return m.MoveTaxonomyNode(req.GetString("taxonomy", ""),
				req.GetString("slug", ""), req.GetString("new_parent_slug", ""))
		})
	}))

	s.AddTool(mcp.NewTool("dits_taxonomy_node_retire",
		mcp.WithDescription("Retire a taxonomy node so new classifications can no longer target it (bumps meta version)."),
		mcp.WithString("taxonomy", mcp.Required()),
		mcp.WithString("slug", mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return applyMetaMutation(ctx, w, func(m *domain.MetaConfig) error {
			return m.RetireTaxonomyNode(req.GetString("taxonomy", ""), req.GetString("slug", ""))
		})
	}))

	// ---------- Sync tool ----------
	s.AddTool(mcp.NewTool("dits_sync",
		mcp.WithDescription("Push local events to and pull remote events from the configured DITS server. Optionally override the server URL."),
		mcp.WithString("server", mcp.Description("Server URL override; defaults to the project's configured remote.")),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		serverURL := req.GetString("server", "")
		res, err := w.Sync(ctx, serverURL)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(res)
	}))
}
