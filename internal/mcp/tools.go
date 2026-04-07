package mcp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

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

func registerTools(s *server.MCPServer, cfg Config) {
	// ---------- Read-only tools ----------

	s.AddTool(mcp.NewTool("dits_work_list",
		mcp.WithDescription("List work items with optional filters (status, kind, ready, include_closed)."),
		mcp.WithString("status", mcp.Description("Filter by status")),
		mcp.WithString("kind", mcp.Description("Filter by kind")),
		mcp.WithBoolean("ready", mcp.Description("Only items that are ready (unleased, unblocked, open)")),
		mcp.WithString("claimed_by", mcp.Description("Filter by lease holder actor ID")),
		mcp.WithBoolean("blocked", mcp.Description("Filter by blocked state")),
		mcp.WithBoolean("include_closed", mcp.Description("Include closed items")),
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
		var out []domain.WorkItem
		for _, wi := range items {
			if ready {
				if wi.LeaseHolder != nil || wi.Blocked || wi.Status == "closed" {
					continue
				}
			}
			if claimedBy != "" {
				if wi.LeaseHolder == nil || string(*wi.LeaseHolder) != claimedBy {
					continue
				}
			}
			if gotBlockedFlag && wi.Blocked != wantBlocked {
				continue
			}
			out = append(out, wi)
		}
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
		mcp.WithDescription("Record an investigation observation."),
		mcp.WithString("id", mcp.Required()),
		mcp.WithString("summary", mcp.Required()),
	), withOps(cfg, func(ctx context.Context, w *workops.WorkOps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, errR, _ := resolveID(ctx, w, req)
		if errR != nil {
			return errR, nil
		}
		wi, err := w.Observe(ctx, id, req.GetString("summary", ""))
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
