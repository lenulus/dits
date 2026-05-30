// The prototype write API: a single JSON endpoint, POST /api/mutate, that the
// embedded React app (/app) fires after each optimistic local update. Every
// request names one prototype action and carries its args; the handler maps it
// to exactly one mcp.Client mutator (sometimes two, for the few actions that
// are inherently a create-then-link or an accept-both). It returns
// {"ok":true} or {"ok":false,"error":"…"}.
//
// The prototype speaks in shared ids (PROJ-204, DB-091, OA-031, …). The dits_*
// tools resolve shared ids themselves, so ids pass straight through to the
// client. This file uses only s.Client + stdlib (no DITS-core imports), per the
// pilot import boundary.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// registerAPIMutate wires the JSON write endpoint. Called from Register().
func (s *Server) registerAPIMutate(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/mutate", s.postAPIMutate)
}

// mutateReq is the wire shape: an action discriminator plus a flat bag of args.
// Each action reads the subset of fields it needs; absent fields decode to "".
type mutateReq struct {
	Action string `json:"action"`
	ID     string `json:"id"`

	// Generic field edits (updateMilestone / updateRfc patches arrive as a
	// {field: value} map so the prototype can send any single-field patch).
	Patch map[string]any `json:"patch"`

	// ACK lifecycle.
	Who    string `json:"who"`    // specifier | builder
	AckAct string `json:"ackAct"` // accepted | rejected | cleared
	Note   string `json:"note"`

	// ACK file fields.
	ScopeSummary       string `json:"scopeSummary"`
	DeliveryTiming     string `json:"deliveryTiming"`
	TargetOutcome      string `json:"targetOutcome"`
	AcceptanceCriteria string `json:"acceptanceCriteria"`

	// Journal entries (status / risk / next / generic log).
	Body     string `json:"body"`
	Severity string `json:"severity"`
	Owner    string `json:"owner"`
	By       string `json:"by"`
	Kind     string `json:"kind"` // log-entry kind, or work kind for create

	// Role bindings.
	Role  string `json:"role"`
	Actor string `json:"actor"`

	// Classification.
	Taxonomy string `json:"taxonomy"`
	Node     string `json:"node"`

	// Dependencies / lineage.
	Dep   string `json:"dep"`
	RfcID string `json:"rfcId"`

	// Schedule.
	Stages []mcp.Stage `json:"stages"`

	// Create.
	Title string `json:"title"`
}

// postAPIMutate decodes one mutation request, dispatches it to the client, and
// writes the JSON envelope. The UI does not block on the response (it updated
// optimistically), so a non-2xx here just gets logged client-side.
func (s *Server) postAPIMutate(w http.ResponseWriter, r *http.Request) {
	var req mutateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMutateJSON(w, http.StatusBadRequest, false, "bad request: "+err.Error())
		return
	}
	if err := s.dispatchMutate(r.Context(), req); err != nil {
		writeMutateJSON(w, http.StatusBadGateway, false, err.Error())
		return
	}
	s.Indicators.Invalidate()
	writeMutateJSON(w, http.StatusOK, true, "")
}

// dispatchMutate routes one action to its mcp.Client mutator(s). Unknown
// actions are a no-op success so a new prototype action never 502s the UI
// before its server mapping lands.
func (s *Server) dispatchMutate(ctx context.Context, req mutateReq) error {
	c := s.Client
	switch req.Action {
	case "updateMilestone", "updateRfc":
		return s.applyPatch(ctx, req.ID, req.Patch)

	case "setAck":
		return ackAction(ctx, c, req.ID, req.Who, req.AckAct, req.Note)
	case "acceptBoth", "commitAck":
		if err := c.AckAccept(ctx, req.ID, "specifier", req.Note); err != nil {
			return err
		}
		return c.AckAccept(ctx, req.ID, "builder", req.Note)
	case "amendAck":
		// Material amendment that auto-clears both ACKs (scope_change default).
		amendType := req.Kind
		if amendType == "" {
			amendType = "scope_change"
		}
		return c.AckAmend(ctx, req.ID, amendType, nil, req.Note)
	case "ackFile":
		return c.AckFile(ctx, req.ID, req.ScopeSummary, req.DeliveryTiming, req.TargetOutcome, req.AcceptanceCriteria)

	case "addStatusUpdate":
		return c.Observe(ctx, req.ID, req.Body, obsData(mcp.ObsData{EntryType: "status"}))
	case "addRisk":
		sev := req.Severity
		if sev == "" {
			sev = "medium"
		}
		return c.Observe(ctx, req.ID, req.Body, obsData(mcp.ObsData{EntryType: "risk", Severity: sev, By: req.By}))
	case "addNextStep":
		return c.Observe(ctx, req.ID, req.Body, obsData(mcp.ObsData{EntryType: "next", Owner: req.Owner}))
	case "addLogEntry":
		return c.Observe(ctx, req.ID, req.Body, obsData(mcp.ObsData{EntryType: req.Kind}))

	case "updateBinding":
		// A binding edit sets the actor on a role; clearing it unbinds.
		if req.Actor == "" {
			return c.UnbindRole(ctx, req.ID, req.Role, req.Actor)
		}
		return c.BindRole(ctx, req.ID, req.Role, req.Actor)

	case "addDep":
		return c.Link(ctx, req.ID, "depends_on", req.Dep)
	case "removeDep":
		return c.Unlink(ctx, req.ID, "depends_on", req.Dep)

	case "setStages":
		return c.ScheduleSet(ctx, req.ID, req.Stages)

	case "spawnMilestone":
		newID, err := c.WorkCreate(ctx, "milestone", req.Title, req.Body)
		if err != nil {
			return err
		}
		if req.RfcID != "" {
			return c.Link(ctx, newID, "derived_from", req.RfcID)
		}
		return nil
	case "newMilestone":
		_, err := c.WorkCreate(ctx, "milestone", req.Title, req.Body)
		return err
	case "newRfc":
		_, err := c.WorkCreate(ctx, "rfc", req.Title, req.Body)
		return err
	case "newDecision":
		_, err := c.WorkCreate(ctx, "decision_block", req.Title, req.Body)
		return err

	case "updateDecision":
		return s.applyDecisionPatch(ctx, req.ID, req.Patch)
	case "updateOutcome":
		return s.applyOutcomePatch(ctx, req.ID, req.Patch)

	default:
		// Unknown action: succeed silently so an unmapped prototype action does
		// not surface as a write failure. Server-side mappings catch up later.
		return nil
	}
}

// applyPatch maps a milestone/rfc field patch ({field: value}) to the client
// mutator that owns that field. The prototype sends one field per call.
func (s *Server) applyPatch(ctx context.Context, id string, patch map[string]any) error {
	c := s.Client
	for k, v := range patch {
		switch k {
		case "title":
			if err := c.SetTitle(ctx, id, str(v)); err != nil {
				return err
			}
		case "status":
			if err := c.SetStatus(ctx, id, str(v)); err != nil {
				return err
			}
		case "ryg":
			if err := c.FieldSet(ctx, id, "ryg", str(v)); err != nil {
				return err
			}
		case "target":
			if err := c.FieldSet(ctx, id, "target", str(v)); err != nil {
				return err
			}
		case "targetPrecision":
			if err := c.FieldSet(ctx, id, "target_precision", str(v)); err != nil {
				return err
			}
		case "customerVisible":
			if err := c.FieldSet(ctx, id, "customer_visible", boolStr(v)); err != nil {
				return err
			}
		case "specifier", "builder", "pilot":
			if err := bindOrUnbind(ctx, c, id, k, str(v)); err != nil {
				return err
			}
		case "orgNode":
			if err := classifyOrDeclassify(ctx, c, id, "org", str(v)); err != nil {
				return err
			}
		case "productNode":
			if err := classifyOrDeclassify(ctx, c, id, "product", str(v)); err != nil {
				return err
			}
		case "summary", "body":
			if err := c.SetBody(ctx, id, str(v)); err != nil {
				return err
			}
			// Keys prefixed "_" or unknown are prototype-local view state — ignore.
		}
	}
	return nil
}

// applyDecisionPatch maps a DecisionBlock patch: status → SetStatus (resolved /
// escalated drive the lifecycle), summary → SetBody.
func (s *Server) applyDecisionPatch(ctx context.Context, id string, patch map[string]any) error {
	c := s.Client
	for k, v := range patch {
		switch k {
		case "status":
			if err := c.SetStatus(ctx, id, str(v)); err != nil {
				return err
			}
		case "summary", "body":
			if err := c.SetBody(ctx, id, str(v)); err != nil {
				return err
			}
		}
	}
	return nil
}

// applyOutcomePatch maps an OutcomeAssessment patch: result/value → FieldSet,
// note → SetBody, status → SetStatus.
func (s *Server) applyOutcomePatch(ctx context.Context, id string, patch map[string]any) error {
	c := s.Client
	for k, v := range patch {
		switch k {
		case "result":
			if err := c.FieldSet(ctx, id, "verdict", str(v)); err != nil {
				return err
			}
		case "value":
			if err := c.FieldSet(ctx, id, "value", str(v)); err != nil {
				return err
			}
		case "note":
			if err := c.SetBody(ctx, id, str(v)); err != nil {
				return err
			}
		case "status":
			if err := c.SetStatus(ctx, id, str(v)); err != nil {
				return err
			}
		}
	}
	return nil
}

// ackAction maps a prototype ACK verb (accepted/rejected/cleared) to the
// client. "cleared" has no direct tool; it rides an amendment elsewhere, so
// here it is a no-op (the optimistic UI already reflects it).
func ackAction(ctx context.Context, c mcp.Client, id, who, action, note string) error {
	switch action {
	case "accepted":
		return c.AckAccept(ctx, id, who, note)
	case "rejected":
		return c.AckReject(ctx, id, who, note)
	default:
		return nil
	}
}

// bindOrUnbind binds role→actor, or unbinds the role when actor is empty.
func bindOrUnbind(ctx context.Context, c mcp.Client, id, role, actor string) error {
	if actor == "" {
		return c.UnbindRole(ctx, id, role, "")
	}
	return c.BindRole(ctx, id, role, actor)
}

// classifyOrDeclassify sets the work item's node within taxonomy, or
// declassifies it from that taxonomy when node is empty.
func classifyOrDeclassify(ctx context.Context, c mcp.Client, id, taxonomy, node string) error {
	if node == "" {
		return c.Declassify(ctx, id, taxonomy, "")
	}
	return c.Classify(ctx, id, taxonomy, node)
}

// obsData marshals an ObsData blob for an Observe call (best-effort; an
// un-marshalable struct yields a nil blob, which Observe tolerates).
func obsData(d mcp.ObsData) json.RawMessage {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil
	}
	return raw
}

// str renders a JSON-decoded scalar as a string (numbers/bools included).
func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

// boolStr renders a JSON-decoded value as "true"/"false" (customer_visible is
// stored as a string field on the substrate).
func boolStr(v any) string {
	if b, ok := v.(bool); ok && b {
		return "true"
	}
	if s, ok := v.(string); ok && s == "true" {
		return "true"
	}
	return "false"
}

// writeMutateJSON writes the {ok,error} envelope with the given status.
func writeMutateJSON(w http.ResponseWriter, status int, ok bool, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]any{"ok": ok}
	if errMsg != "" {
		resp["error"] = errMsg
	}
	_ = json.NewEncoder(w).Encode(resp)
}
