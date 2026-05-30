// Package workops contains the shared business logic for DITS work-item
// operations. Both the `dits` CLI and the `dits-mcp` MCP server are thin
// front-ends over this package: they do I/O (flag parsing / JSON-RPC) and
// then delegate the actual event construction, validation, signing,
// appending, and materialization to the methods here.
//
// All mutating methods return the updated WorkItem (after reduction) plus
// any auxiliary IDs (attempt, lease, eval, artifact, handoff, review)
// generated along the way via a small result struct so callers can render
// both human and JSON output without re-reading state.
package workops

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"time"

	"github.com/lenulus/pf/internal/blob"
	"github.com/lenulus/pf/internal/crypto"
	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/logging"
	"github.com/lenulus/pf/internal/project"
	"github.com/lenulus/pf/internal/store"
)

// WorkOps wraps an open project and exposes high-level work-item operations.
// Callers are responsible for closing the underlying project (Close()).
type WorkOps struct {
	Proj   *project.Project
	logger *slog.Logger
}

// Logger returns the configured logger, never nil.
func (w *WorkOps) Logger() *slog.Logger {
	if w.logger != nil {
		return w.logger
	}
	return logging.Discard()
}

// SetLogger replaces the logger used for event-emission and warning logs.
func (w *WorkOps) SetLogger(l *slog.Logger) {
	if l != nil {
		w.logger = l
	}
}

// Open locates the project from cwd (same discovery rules as the CLI) and
// opens it. Callers must defer w.Close().
func Open() (*WorkOps, error) {
	root, err := project.FindRoot()
	if err != nil {
		return nil, err
	}
	p, err := project.Load(root)
	if err != nil {
		return nil, err
	}
	return &WorkOps{Proj: p}, nil
}

// OpenAt opens the project rooted at the given .dits parent directory.
func OpenAt(root string) (*WorkOps, error) {
	p, err := project.Load(root)
	if err != nil {
		return nil, err
	}
	return &WorkOps{Proj: p}, nil
}

// Shutdown releases the underlying database.
func (w *WorkOps) Shutdown() error {
	if w == nil || w.Proj == nil {
		return nil
	}
	return w.Proj.DB.Close()
}

// ActorID returns the configured actor for this project.
func (w *WorkOps) ActorID() domain.ActorID { return w.Proj.Config.ActorID }

// LoadMeta fetches the current meta config, erroring if none exists.
func (w *WorkOps) LoadMeta(ctx context.Context) (*domain.MetaConfig, error) {
	meta, err := w.Proj.DB.GetCurrentMeta(ctx)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, fmt.Errorf("no meta configuration found")
	}
	return meta, nil
}

// SignEvent signs the event in place if the project has a private key.
func (w *WorkOps) SignEvent(e *domain.Event) error {
	if k := w.Proj.PrivKey(); k != nil {
		return crypto.SignEvent(e, k)
	}
	return nil
}

// ResolveWorkItem resolves by shared ID or work item ID.
func (w *WorkOps) ResolveWorkItem(ctx context.Context, ref string) (*domain.WorkItem, error) {
	wi, err := w.Proj.DB.GetWorkItemBySharedID(ctx, domain.SharedID(ref))
	if err != nil {
		return nil, err
	}
	if wi != nil {
		return wi, nil
	}
	wi, err = w.Proj.DB.GetWorkItem(ctx, domain.WorkItemID(ref))
	if err != nil {
		return nil, err
	}
	if wi != nil {
		return wi, nil
	}
	return nil, fmt.Errorf("work item not found: %s", ref)
}

// AppendAndMaterialize validates, signs, appends, and re-reduces an event.
//
// Logs one INFO line per event with type/work_item/event_id/actor_id and any
// kind-specific allocated IDs (lease_id, attempt_id, eval_id, ...). On
// failure logs ERROR with err. The middleware-injected request_id flows
// through ctx so the line stitches into the per-tool-call request stream.
func (w *WorkOps) AppendAndMaterialize(ctx context.Context, workItemID domain.WorkItemID, event domain.Event) (*domain.WorkItem, error) {
	existing, _ := w.Proj.DB.GetWorkItem(ctx, workItemID)

	hasEvent := func(key string) bool {
		allEvents, err := w.Proj.DB.GetEventsForWorkItem(ctx, workItemID)
		if err != nil {
			return false
		}
		for _, e := range allEvents {
			evtKey := domain.EventLookupKey(e.Type, string(e.ID))
			if evtKey == key {
				return true
			}
			if payloadKey := extractPayloadRefKey(e); payloadKey == key {
				return true
			}
		}
		return false
	}

	log := w.Logger()
	if err := domain.ValidateProtocolFull(event, existing, hasEvent); err != nil {
		log.ErrorContext(ctx, "event rejected",
			slog.String("type", string(event.Type)),
			slog.String("work_item", string(workItemID)),
			slog.String("event_id", string(event.ID)),
			slog.String("actor_id", string(event.ActorID)),
			slog.Any("err", err),
		)
		return nil, err
	}
	if err := w.SignEvent(&event); err != nil {
		log.ErrorContext(ctx, "event sign failed",
			slog.String("type", string(event.Type)),
			slog.String("work_item", string(workItemID)),
			slog.String("event_id", string(event.ID)),
			slog.Any("err", err),
		)
		return nil, fmt.Errorf("signing event: %w", err)
	}
	if err := w.Proj.DB.AppendEvents(ctx, []domain.Event{event}); err != nil {
		log.ErrorContext(ctx, "event append failed",
			slog.String("type", string(event.Type)),
			slog.String("work_item", string(workItemID)),
			slog.String("event_id", string(event.ID)),
			slog.Any("err", err),
		)
		return nil, fmt.Errorf("appending event: %w", err)
	}

	events, err := w.Proj.DB.GetEventsForWorkItem(ctx, workItemID)
	if err != nil {
		log.ErrorContext(ctx, "event reload failed",
			slog.String("work_item", string(workItemID)),
			slog.Any("err", err),
		)
		return nil, err
	}
	ordered := domain.CausalOrder(events)
	wi, err := domain.Reduce(ordered)
	if err != nil {
		log.ErrorContext(ctx, "reduce failed",
			slog.String("work_item", string(workItemID)),
			slog.Any("err", err),
		)
		return nil, err
	}

	existing2, _ := w.Proj.DB.GetWorkItem(ctx, workItemID)
	if existing2 != nil && existing2.SharedID != "" {
		wi.SharedID = existing2.SharedID
	}
	if err := w.Proj.DB.UpsertWorkItem(ctx, wi); err != nil {
		log.ErrorContext(ctx, "upsert failed",
			slog.String("work_item", string(workItemID)),
			slog.Any("err", err),
		)
		return nil, err
	}

	attrs := []any{
		slog.String("type", string(event.Type)),
		slog.String("work_item", string(workItemID)),
		slog.String("event_id", string(event.ID)),
		slog.String("actor_id", string(event.ActorID)),
	}
	for _, a := range eventPayloadAttrs(event) {
		attrs = append(attrs, a)
	}
	log.InfoContext(ctx, "event appended", attrs...)
	return wi, nil
}

// eventPayloadAttrs extracts the kind-specific allocated IDs from an event
// payload (lease_id, attempt_id, eval_id, handoff_id, review_id) so the
// "event appended" log line carries them as discrete fields.
func eventPayloadAttrs(e domain.Event) []slog.Attr {
	var out []slog.Attr
	switch e.Type {
	case domain.EventWorkLeased:
		var p domain.LeasedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			out = append(out, slog.String("lease_id", string(p.LeaseID)))
		}
	case domain.EventWorkExecutionStarted:
		var p domain.ExecutionStartedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			out = append(out, slog.String("attempt_id", string(p.AttemptID)))
		}
	case domain.EventWorkCheckpointed:
		var p domain.CheckpointedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			out = append(out, slog.String("attempt_id", string(p.AttemptID)))
		}
	case domain.EventWorkExecutionCompleted:
		var p domain.ExecutionCompletedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			out = append(out, slog.String("attempt_id", string(p.AttemptID)))
		}
	case domain.EventWorkExecutionFailed:
		var p domain.ExecutionFailedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			out = append(out, slog.String("attempt_id", string(p.AttemptID)))
		}
	case domain.EventWorkEvalRequested:
		var p domain.EvalRequestedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			out = append(out, slog.String("eval_id", string(p.EvalID)))
		}
	case domain.EventWorkEvalCompleted:
		var p domain.EvalCompletedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			out = append(out, slog.String("eval_id", string(p.EvalID)))
		}
	case domain.EventWorkHandedOff:
		var p domain.HandedOffPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			out = append(out, slog.String("handoff_id", string(p.HandoffID)))
		}
	case domain.EventWorkReviewRequested:
		var p domain.ReviewRequestedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			out = append(out, slog.String("review_id", string(p.ReviewID)))
		}
	}
	return out
}

func extractPayloadRefKey(e domain.Event) string {
	switch e.Type {
	case domain.EventWorkPlanProposed:
		return domain.EventLookupKey(domain.EventWorkPlanProposed, string(e.ID))
	case domain.EventWorkReviewRequested:
		var p domain.ReviewRequestedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			return domain.EventLookupKey(domain.EventWorkReviewRequested, string(p.ReviewID))
		}
	case domain.EventWorkEvalRequested:
		var p domain.EvalRequestedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			return domain.EventLookupKey(domain.EventWorkEvalRequested, string(p.EvalID))
		}
	case domain.EventWorkHandedOff:
		var p domain.HandedOffPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			return domain.EventLookupKey(domain.EventWorkHandedOff, string(p.HandoffID))
		}
	}
	return ""
}

// ---------- Read operations ----------

// ListWorkItems returns matching items; if includeClosed is false, closed items are filtered out.
func (w *WorkOps) ListWorkItems(ctx context.Context, filter store.WorkItemFilter, includeClosed bool) ([]domain.WorkItem, error) {
	items, err := w.Proj.DB.ListWorkItems(ctx, filter)
	if err != nil {
		return nil, err
	}
	if includeClosed {
		return items, nil
	}
	var out []domain.WorkItem
	for _, wi := range items {
		if wi.Status != "closed" {
			out = append(out, wi)
		}
	}
	return out, nil
}

// GetEvents returns all events for a work item (raw, unordered).
func (w *WorkOps) GetEvents(ctx context.Context, id domain.WorkItemID) ([]domain.Event, error) {
	return w.Proj.DB.GetEventsForWorkItem(ctx, id)
}

// ---------- Mutating operations ----------

// CreateResult carries the new work item plus its allocated shared ID.
type CreateResult struct {
	WorkItem *domain.WorkItem
	SharedID domain.SharedID
}

// CreateWorkItem creates a new work item with optional labels.
func (w *WorkOps) CreateWorkItem(ctx context.Context, kind, title, body string, labels []string) (*CreateResult, error) {
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if kind == "" {
		kind = "task"
	}
	meta, err := w.LoadMeta(ctx)
	if err != nil {
		return nil, err
	}

	workItemID := domain.NewWorkItemID()
	now := time.Now().UTC()
	event := domain.Event{
		ID:          domain.NewEventID(),
		WorkItemID:  workItemID,
		Type:        domain.EventWorkCreated,
		MetaVersion: meta.Version,
		ActorID:     w.Proj.Config.ActorID,
		Timestamp:   now,
		Payload:     domain.MustMarshalPayload(domain.WorkCreatedPayload{Title: title, Body: body, Kind: kind}),
	}
	if err := domain.ValidateEvent(event, meta); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	if err := w.SignEvent(&event); err != nil {
		return nil, fmt.Errorf("signing: %w", err)
	}
	if err := w.Proj.DB.AppendEvents(ctx, []domain.Event{event}); err != nil {
		return nil, fmt.Errorf("appending event: %w", err)
	}

	for _, l := range labels {
		heads, _ := w.Proj.DB.GetHeads(ctx, workItemID)
		labelEvt := domain.Event{
			ID:             domain.NewEventID(),
			WorkItemID:     workItemID,
			Type:           domain.EventWorkLabelAdded,
			ParentEventIDs: heads,
			MetaVersion:    meta.Version,
			ActorID:        w.Proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(domain.LabelPayload{LabelSlug: l}),
		}
		if err := domain.ValidateEvent(labelEvt, meta); err != nil {
			return nil, fmt.Errorf("validation: %w", err)
		}
		if err := w.SignEvent(&labelEvt); err != nil {
			return nil, fmt.Errorf("signing: %w", err)
		}
		if err := w.Proj.DB.AppendEvents(ctx, []domain.Event{labelEvt}); err != nil {
			return nil, err
		}
	}

	events, err := w.Proj.DB.GetEventsForWorkItem(ctx, workItemID)
	if err != nil {
		return nil, err
	}
	ordered := domain.CausalOrder(events)
	wi, err := domain.Reduce(ordered)
	if err != nil {
		return nil, err
	}
	if err := w.Proj.DB.UpsertWorkItem(ctx, wi); err != nil {
		return nil, err
	}
	sharedID, err := w.Proj.DB.AllocateSharedID(ctx, workItemID, w.Proj.Config.ProjectKey)
	if err != nil {
		return nil, fmt.Errorf("allocating shared ID: %w", err)
	}
	wi.SharedID = sharedID
	w.Logger().InfoContext(ctx, "event appended",
		slog.String("type", string(domain.EventWorkCreated)),
		slog.String("work_item", string(workItemID)),
		slog.String("event_id", string(event.ID)),
		slog.String("actor_id", string(event.ActorID)),
		slog.String("shared_id", string(sharedID)),
	)
	return &CreateResult{WorkItem: wi, SharedID: sharedID}, nil
}

// simpleEvent is a helper that builds a basic event with heads-as-parents and appends.
func (w *WorkOps) simpleEvent(ctx context.Context, id domain.WorkItemID, etype domain.EventType, payload any, withMeta bool) (*domain.WorkItem, error) {
	heads, _ := w.Proj.DB.GetHeads(ctx, id)
	evt := domain.Event{
		ID: domain.NewEventID(), WorkItemID: id, Type: etype,
		ParentEventIDs: heads, ActorID: w.Proj.Config.ActorID, Timestamp: time.Now().UTC(),
		Payload: domain.MustMarshalPayload(payload),
	}
	if withMeta {
		meta, err := w.LoadMeta(ctx)
		if err != nil {
			return nil, err
		}
		evt.MetaVersion = meta.Version
		if err := domain.ValidateEvent(evt, meta); err != nil {
			return nil, fmt.Errorf("validation: %w", err)
		}
	}
	return w.AppendAndMaterialize(ctx, id, evt)
}

func (w *WorkOps) Comment(ctx context.Context, id domain.WorkItemID, body string) (*domain.WorkItem, error) {
	if body == "" {
		return nil, fmt.Errorf("body is required")
	}
	return w.simpleEvent(ctx, id, domain.EventWorkCommented, domain.CommentPayload{Body: body}, false)
}

func (w *WorkOps) Close(ctx context.Context, id domain.WorkItemID) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkClosed, domain.ClosedPayload{}, false)
}

func (w *WorkOps) Reopen(ctx context.Context, id domain.WorkItemID) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkReopened, struct{}{}, false)
}

func (w *WorkOps) SetStatus(ctx context.Context, id domain.WorkItemID, from, to string) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkStatusSet, domain.StatusSetPayload{From: from, To: to}, true)
}

func (w *WorkOps) AddLabel(ctx context.Context, id domain.WorkItemID, slug string) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkLabelAdded, domain.LabelPayload{LabelSlug: slug}, true)
}

func (w *WorkOps) RemoveLabel(ctx context.Context, id domain.WorkItemID, slug string) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkLabelRemoved, domain.LabelPayload{LabelSlug: slug}, true)
}

func (w *WorkOps) Assign(ctx context.Context, id domain.WorkItemID, assignee domain.ActorID) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkAssigned, domain.AssignPayload{Assignee: assignee}, false)
}

func (w *WorkOps) Unassign(ctx context.Context, id domain.WorkItemID, assignee domain.ActorID) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkUnassigned, domain.AssignPayload{Assignee: assignee}, false)
}

func (w *WorkOps) Link(ctx context.Context, id domain.WorkItemID, relationType string, target domain.WorkItemID) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkLinked, domain.RelationPayload{RelationType: relationType, TargetWorkItem: target}, false)
}

func (w *WorkOps) Unlink(ctx context.Context, id domain.WorkItemID, relationType string, target domain.WorkItemID) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkUnlinked, domain.RelationPayload{RelationType: relationType, TargetWorkItem: target}, false)
}

// Classify places a work item within a taxonomy node. withMeta=true so the
// taxonomy/node references are validated against the current meta.
func (w *WorkOps) Classify(ctx context.Context, id domain.WorkItemID, taxonomySlug, nodeSlug string) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkClassified, domain.ClassificationPayload{TaxonomySlug: taxonomySlug, NodeSlug: nodeSlug}, true)
}

func (w *WorkOps) Declassify(ctx context.Context, id domain.WorkItemID, taxonomySlug, nodeSlug string) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkDeclassified, domain.ClassificationPayload{TaxonomySlug: taxonomySlug, NodeSlug: nodeSlug}, true)
}

// BindRole binds an actor to a named role on a work item.
func (w *WorkOps) BindRole(ctx context.Context, id domain.WorkItemID, roleSlug string, actor domain.ActorID) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkRoleBound, domain.RoleBindingPayload{RoleSlug: roleSlug, Actor: actor}, true)
}

func (w *WorkOps) UnbindRole(ctx context.Context, id domain.WorkItemID, roleSlug string, actor domain.ActorID) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkRoleUnbound, domain.RoleBindingPayload{RoleSlug: roleSlug, Actor: actor}, true)
}

// AttachResult carries the resulting work item plus allocated artifact metadata.
type AttachResult struct {
	WorkItem   *domain.WorkItem
	ArtifactID domain.ArtifactID
	Filename   string
	SizeBytes  int64
	Hash       string
}

func (w *WorkOps) Attach(ctx context.Context, id domain.WorkItemID, filePath string) (*AttachResult, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	if info.Size() > blob.MaxBlobSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), blob.MaxBlobSize)
	}
	hash, size, err := blob.ComputeFileHash(filePath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := w.Proj.Blobs.Put(ctx, hash, f); err != nil {
		return nil, fmt.Errorf("storing blob: %w", err)
	}
	filename := filepath.Base(filePath)
	mimeType := mime.TypeByExtension(filepath.Ext(filename))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	artID := domain.NewArtifactID()
	wi, err := w.simpleEvent(ctx, id, domain.EventWorkArtifactAdded, domain.ArtifactAddedPayload{
		ArtifactID: artID, ContentHash: hash, Filename: filename, MimeType: mimeType, SizeBytes: size,
	}, false)
	if err != nil {
		return nil, err
	}
	return &AttachResult{WorkItem: wi, ArtifactID: artID, Filename: filename, SizeBytes: size, Hash: hash}, nil
}

func (w *WorkOps) Detach(ctx context.Context, id domain.WorkItemID, artID domain.ArtifactID) (*domain.WorkItem, error) {
	wi, err := w.Proj.DB.GetWorkItem(ctx, id)
	if err != nil {
		return nil, err
	}
	if wi == nil {
		return nil, fmt.Errorf("work item not found")
	}
	found := false
	for _, a := range wi.Artifacts {
		if a.ID == artID {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("artifact %s not found", artID)
	}
	return w.simpleEvent(ctx, id, domain.EventWorkArtifactRemoved, domain.ArtifactRemovedPayload{ArtifactID: artID}, false)
}

// LeaseResult carries lease metadata.
type LeaseResult struct {
	WorkItem       *domain.WorkItem
	LeaseID        domain.LeaseID
	LeaseExpiresAt time.Time
	DurationSec    int
}

func (w *WorkOps) Lease(ctx context.Context, id domain.WorkItemID) (*LeaseResult, error) {
	wi, err := w.Proj.DB.GetWorkItem(ctx, id)
	if err != nil {
		return nil, err
	}
	if wi == nil {
		return nil, fmt.Errorf("work item not found")
	}
	if wi.LeaseHolder != nil {
		return nil, fmt.Errorf("work item is already leased by %s", *wi.LeaseHolder)
	}
	durationSec := 300
	meta, _ := w.LoadMeta(ctx)
	if meta != nil {
		if p := meta.GetLeasePolicy(wi.Kind); p != nil {
			durationSec = p.DefaultDurationSec
		}
	}
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(durationSec) * time.Second)
	leaseID := domain.NewLeaseID()
	heads, _ := w.Proj.DB.GetHeads(ctx, id)
	evt := domain.Event{
		ID: domain.NewEventID(), WorkItemID: id, Type: domain.EventWorkLeased,
		ParentEventIDs: heads, ActorID: w.Proj.Config.ActorID, Timestamp: now,
		Payload: domain.MustMarshalPayload(domain.LeasedPayload{
			LeaseID: leaseID, LeaseDurationSec: durationSec,
			LeaseExpiresAt: expiresAt, Generation: 1,
		}),
	}
	newWI, err := w.AppendAndMaterialize(ctx, id, evt)
	if err != nil {
		return nil, err
	}
	return &LeaseResult{WorkItem: newWI, LeaseID: leaseID, LeaseExpiresAt: expiresAt, DurationSec: durationSec}, nil
}

// LeaseReleaseResult carries lease-hold metrics so callers can log how long
// every lease was held — useful for spotting stuck/abandoned leases.
type LeaseReleaseResult struct {
	WorkItem        *domain.WorkItem
	LeaseDurHeldMs  int64
}

func (w *WorkOps) LeaseRelease(ctx context.Context, id domain.WorkItemID, reason string) (*domain.WorkItem, error) {
	res, err := w.LeaseReleaseDetailed(ctx, id, reason)
	if err != nil || res == nil {
		return nil, err
	}
	return res.WorkItem, nil
}

// LeaseReleaseDetailed releases the current lease and returns the held
// duration in ms. Logs an INFO line so the metric shows up in mcp.log
// alongside the corresponding event-appended record.
func (w *WorkOps) LeaseReleaseDetailed(ctx context.Context, id domain.WorkItemID, reason string) (*LeaseReleaseResult, error) {
	wi, err := w.Proj.DB.GetWorkItem(ctx, id)
	if err != nil {
		return nil, err
	}
	if wi == nil {
		return nil, fmt.Errorf("work item not found")
	}
	if wi.LeaseHolder == nil {
		return &LeaseReleaseResult{WorkItem: wi}, nil
	}

	// Recover the original acquisition timestamp by walking the event log
	// for the most recent lease event. This is the only place we time
	// individual lease holds; if it ever shows up on hot paths it's worth
	// caching the value on WorkItem during reduction.
	var heldMs int64
	if events, evErr := w.Proj.DB.GetEventsForWorkItem(ctx, id); evErr == nil {
		var latest time.Time
		for _, e := range events {
			if e.Type == domain.EventWorkLeased && e.Timestamp.After(latest) {
				latest = e.Timestamp
			}
		}
		if !latest.IsZero() {
			heldMs = time.Since(latest).Milliseconds()
		}
	}

	newWI, err := w.simpleEvent(ctx, id, domain.EventWorkLeaseReleased, domain.LeaseReleasedPayload{Reason: reason}, false)
	if err != nil {
		return nil, err
	}
	w.Logger().InfoContext(ctx, "lease released",
		slog.String("work_item", string(id)),
		slog.Int64("lease_dur_held_ms", heldMs),
	)
	return &LeaseReleaseResult{WorkItem: newWI, LeaseDurHeldMs: heldMs}, nil
}

// StartResult carries the new attempt ID.
type StartResult struct {
	WorkItem  *domain.WorkItem
	AttemptID domain.AttemptID
}

func (w *WorkOps) Start(ctx context.Context, id domain.WorkItemID) (*StartResult, error) {
	attemptID := domain.NewAttemptID()
	wi, err := w.simpleEvent(ctx, id, domain.EventWorkExecutionStarted,
		domain.ExecutionStartedPayload{AttemptID: attemptID}, false)
	if err != nil {
		return nil, err
	}
	return &StartResult{WorkItem: wi, AttemptID: attemptID}, nil
}

func (w *WorkOps) Complete(ctx context.Context, id domain.WorkItemID, summary string) (*domain.WorkItem, error) {
	wi, err := w.Proj.DB.GetWorkItem(ctx, id)
	if err != nil {
		return nil, err
	}
	if wi == nil || wi.CurrentAttempt == nil {
		return nil, fmt.Errorf("no active attempt")
	}
	return w.simpleEvent(ctx, id, domain.EventWorkExecutionCompleted,
		domain.ExecutionCompletedPayload{AttemptID: *wi.CurrentAttempt, Summary: summary}, false)
}

func (w *WorkOps) Fail(ctx context.Context, id domain.WorkItemID, errMsg string, retryable bool) (*domain.WorkItem, error) {
	wi, err := w.Proj.DB.GetWorkItem(ctx, id)
	if err != nil {
		return nil, err
	}
	if wi == nil || wi.CurrentAttempt == nil {
		return nil, fmt.Errorf("no active attempt")
	}
	return w.simpleEvent(ctx, id, domain.EventWorkExecutionFailed,
		domain.ExecutionFailedPayload{AttemptID: *wi.CurrentAttempt, Error: errMsg, Retryable: retryable}, false)
}

func (w *WorkOps) Checkpoint(ctx context.Context, id domain.WorkItemID, summary string, progress float64) (*domain.WorkItem, error) {
	wi, err := w.Proj.DB.GetWorkItem(ctx, id)
	if err != nil {
		return nil, err
	}
	if wi == nil || wi.CurrentAttempt == nil {
		return nil, fmt.Errorf("no active attempt")
	}
	return w.simpleEvent(ctx, id, domain.EventWorkCheckpointed,
		domain.CheckpointedPayload{AttemptID: *wi.CurrentAttempt, Summary: summary, Progress: progress}, false)
}

func (w *WorkOps) Block(ctx context.Context, id domain.WorkItemID, reason string) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkBlocked, domain.BlockedPayload{Reason: reason}, false)
}

func (w *WorkOps) Unblock(ctx context.Context, id domain.WorkItemID, reason string) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkUnblocked, domain.UnblockedPayload{Reason: reason}, false)
}

func (w *WorkOps) Observe(ctx context.Context, id domain.WorkItemID, summary string) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkObservationRecorded, domain.ObservationRecordedPayload{Summary: summary}, false)
}

func (w *WorkOps) Finding(ctx context.Context, id domain.WorkItemID, statement string, confidence float64) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkFindingRecorded,
		domain.FindingRecordedPayload{Statement: statement, Confidence: confidence}, false)
}

func (w *WorkOps) Plan(ctx context.Context, id domain.WorkItemID, summary, plan string) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkPlanProposed, domain.PlanProposedPayload{Plan: plan, Summary: summary}, false)
}

// HandoffResult carries the allocated handoff ID.
type HandoffResult struct {
	WorkItem  *domain.WorkItem
	HandoffID domain.HandoffID
}

func (w *WorkOps) Handoff(ctx context.Context, id domain.WorkItemID, to domain.ActorID, contextText string) (*HandoffResult, error) {
	handoffID := domain.NewHandoffID()
	wi, err := w.simpleEvent(ctx, id, domain.EventWorkHandedOff,
		domain.HandedOffPayload{HandoffID: handoffID, From: w.Proj.Config.ActorID, To: to, Context: contextText}, false)
	if err != nil {
		return nil, err
	}
	return &HandoffResult{WorkItem: wi, HandoffID: handoffID}, nil
}

// ReviewResult carries the allocated review ID.
type ReviewResult struct {
	WorkItem *domain.WorkItem
	ReviewID domain.ReviewID
}

func (w *WorkOps) Review(ctx context.Context, id domain.WorkItemID, scope string) (*ReviewResult, error) {
	reviewID := domain.NewReviewID()
	wi, err := w.simpleEvent(ctx, id, domain.EventWorkReviewRequested,
		domain.ReviewRequestedPayload{ReviewID: reviewID, Scope: scope}, false)
	if err != nil {
		return nil, err
	}
	return &ReviewResult{WorkItem: wi, ReviewID: reviewID}, nil
}

// EvalRequestResult carries the allocated eval ID.
type EvalRequestResult struct {
	WorkItem *domain.WorkItem
	EvalID   domain.EvalID
}

func (w *WorkOps) EvalRequest(ctx context.Context, id domain.WorkItemID, scope, subjectKind, subjectRef, rubricRef string) (*EvalRequestResult, error) {
	evalID := domain.NewEvalID()
	wi, err := w.simpleEvent(ctx, id, domain.EventWorkEvalRequested,
		domain.EvalRequestedPayload{
			EvalID: evalID, SubjectKind: subjectKind, SubjectRef: subjectRef,
			RubricRef: rubricRef, Scope: scope,
		}, false)
	if err != nil {
		return nil, err
	}
	return &EvalRequestResult{WorkItem: wi, EvalID: evalID}, nil
}

func (w *WorkOps) EvalComplete(ctx context.Context, id domain.WorkItemID, evalID, subjectKind, subjectRef, rubricRef, verdict, summary string, metrics json.RawMessage) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkEvalCompleted,
		domain.EvalCompletedPayload{
			EvalID:      domain.EvalID(evalID),
			SubjectKind: subjectKind,
			SubjectRef:  subjectRef,
			RubricRef:   rubricRef,
			Verdict:     verdict,
			Summary:     summary,
			Metrics:     metrics,
		}, false)
}

func (w *WorkOps) Retain(ctx context.Context, id domain.WorkItemID, subjectKind, subjectRef, reason, evalRef string) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkOutcomeRetained,
		domain.OutcomeRetainedPayload{
			SubjectKind: subjectKind, SubjectRef: subjectRef,
			Reason: reason, EvalRef: evalRef,
		}, false)
}

func (w *WorkOps) Discard(ctx context.Context, id domain.WorkItemID, subjectKind, subjectRef, reason string) (*domain.WorkItem, error) {
	return w.simpleEvent(ctx, id, domain.EventWorkOutcomeDiscarded,
		domain.OutcomeDiscardedPayload{
			SubjectKind: subjectKind, SubjectRef: subjectRef, Reason: reason,
		}, false)
}
