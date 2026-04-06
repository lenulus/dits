package sync

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log/slog"

	"github.com/lenulus/pf/internal/crypto"
	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/store"
)

// SignatureMode controls how the engine handles event signatures during sync.
const (
	SignatureModeWarn   = "warn"   // log warnings, accept events (default)
	SignatureModeReject = "reject" // reject events with invalid or missing signatures
	SignatureModeIgnore = "ignore" // skip signature verification entirely
)

// Engine handles sync logic for both server and client sides.
type Engine struct {
	db            store.DB
	logger        *slog.Logger
	signatureMode string
}

func NewEngine(db store.DB) *Engine {
	return &Engine{db: db, logger: slog.Default(), signatureMode: SignatureModeWarn}
}

func NewEngineWithLogger(db store.DB, logger *slog.Logger) *Engine {
	return &Engine{db: db, logger: logger, signatureMode: SignatureModeWarn}
}

// SetSignatureMode configures signature enforcement: "warn" (default), "reject", or "ignore".
func (e *Engine) SetSignatureMode(mode string) {
	e.signatureMode = mode
}

// HandleSync processes a sync request (server-side).
func (e *Engine) HandleSync(ctx context.Context, req SyncRequest) (*SyncResponse, error) {
	// 0. Register actor if public key provided.
	if req.ActorID != "" && req.PublicKey != "" {
		if err := e.db.RegisterActor(ctx, req.ActorID, req.PublicKey, req.NodeID); err != nil {
			e.logger.Warn("failed to register actor", "actor_id", req.ActorID, "error", err)
		}
	}

	// 0b. Verify signatures on pushed events.
	if len(req.Events) > 0 && e.signatureMode != SignatureModeIgnore {
		if err := e.verifyEventSignatures(ctx, req.Events); err != nil {
			return nil, err
		}
	}

	// 0c. Advisory protocol validation on pushed events (log, don't reject).
	if len(req.Events) > 0 {
		e.advisoryProtocolValidation(ctx, req.Events)
	}

	// 1. Ingest client events.
	if len(req.Events) > 0 {
		if err := e.db.AppendEvents(ctx, req.Events); err != nil {
			return nil, fmt.Errorf("ingesting events: %w", err)
		}
	}

	// 1b. Accept client meta if it has a higher version.
	meta, err := e.db.GetCurrentMeta(ctx)
	if err != nil {
		return nil, err
	}
	if req.Meta != nil {
		serverVersion := domain.MetaVersion(0)
		if meta != nil {
			serverVersion = meta.Version
		}
		if req.Meta.Version > serverVersion {
			if err := e.db.SaveMeta(ctx, req.Meta); err != nil {
				return nil, fmt.Errorf("saving client meta: %w", err)
			}
			meta = req.Meta
		}
	}

	// 2. Assign shared IDs to new work items and rematerialize.
	sharedIDs := make(map[domain.WorkItemID]domain.SharedID)
	affectedWorkItems, err := e.db.GetAffectedWorkItemIDs(ctx, req.Events)
	if err != nil {
		return nil, err
	}

	for _, wiID := range affectedWorkItems {
		if err := e.rematerialize(ctx, wiID); err != nil {
			return nil, fmt.Errorf("rematerializing %s: %w", wiID, err)
		}

		wi, err := e.db.GetWorkItem(ctx, wiID)
		if err != nil {
			return nil, err
		}
		if wi != nil && wi.SharedID == "" && meta != nil {
			sid, err := e.db.AllocateSharedID(ctx, wiID, meta.ProjectKey)
			if err != nil {
				return nil, fmt.Errorf("allocating shared ID: %w", err)
			}
			sharedIDs[wiID] = sid
		}
	}

	// 3. Compute events the client needs.
	serverHeads, err := e.db.GetAllHeads(ctx)
	if err != nil {
		return nil, err
	}

	missingEvents, err := e.findMissingEvents(ctx, serverHeads, req.Heads)
	if err != nil {
		return nil, fmt.Errorf("finding missing events: %w", err)
	}

	// 4. Include shared IDs for any work items in pulled events.
	pulledWorkItemIDs, _ := e.db.GetAffectedWorkItemIDs(ctx, missingEvents)
	for _, wiID := range pulledWorkItemIDs {
		if _, ok := sharedIDs[wiID]; ok {
			continue
		}
		wi, err := e.db.GetWorkItem(ctx, wiID)
		if err != nil {
			return nil, err
		}
		if wi != nil && wi.SharedID != "" {
			sharedIDs[wiID] = wi.SharedID
		}
	}

	// 5. Store the client's heads as remote heads.
	newClientHeads := serverHeads

	resp := &SyncResponse{
		Events:    missingEvents,
		SharedIDs: sharedIDs,
		Heads:     serverHeads,
	}

	if meta != nil && req.MetaVersion < meta.Version {
		resp.Meta = meta
	}

	if err := e.db.SetRemoteHeads(ctx, req.NodeID, newClientHeads); err != nil {
		return nil, err
	}

	return resp, nil
}

// ApplySync processes a sync response (client-side).
func (e *Engine) ApplySync(ctx context.Context, resp *SyncResponse, serverNodeID domain.NodeID) error {
	// 1. Ingest server events.
	if len(resp.Events) > 0 {
		if err := e.db.AppendEvents(ctx, resp.Events); err != nil {
			return fmt.Errorf("ingesting server events: %w", err)
		}
	}

	// 2. Rematerialize affected work items.
	affectedWorkItems, err := e.db.GetAffectedWorkItemIDs(ctx, resp.Events)
	if err != nil {
		return err
	}
	for _, wiID := range affectedWorkItems {
		if err := e.rematerialize(ctx, wiID); err != nil {
			return fmt.Errorf("rematerializing %s: %w", wiID, err)
		}
	}

	// 3. Apply shared IDs from server.
	for wiID, sharedID := range resp.SharedIDs {
		wi, err := e.db.GetWorkItem(ctx, wiID)
		if err != nil {
			return err
		}
		if wi != nil && wi.SharedID == "" {
			wi.SharedID = sharedID
			if err := e.db.UpsertWorkItem(ctx, wi); err != nil {
				return err
			}
		}
	}

	// 4. Update meta if server sent a newer version.
	if resp.Meta != nil {
		localMeta, _ := e.db.GetCurrentMeta(ctx)
		localVersion := domain.MetaVersion(0)
		if localMeta != nil {
			localVersion = localMeta.Version
		}
		if resp.Meta.Version > localVersion {
			if err := e.db.SaveMeta(ctx, resp.Meta); err != nil {
				return fmt.Errorf("saving meta: %w", err)
			}
		}
	}

	// 5. Store server heads.
	if err := e.db.SetRemoteHeads(ctx, serverNodeID, resp.Heads); err != nil {
		return err
	}

	return nil
}

// BuildSyncRequest creates a sync request to send to the server.
func (e *Engine) BuildSyncRequest(ctx context.Context, nodeID domain.NodeID, projectKey string, serverNodeID domain.NodeID) (*SyncRequest, error) {
	localHeads, err := e.db.GetAllHeads(ctx)
	if err != nil {
		return nil, err
	}

	remoteHeads, err := e.db.GetRemoteHeads(ctx, serverNodeID)
	if err != nil {
		return nil, err
	}

	eventsToPush, err := e.findMissingEvents(ctx, localHeads, remoteHeads)
	if err != nil {
		return nil, err
	}

	meta, err := e.db.GetCurrentMeta(ctx)
	if err != nil {
		return nil, err
	}
	var metaVersion domain.MetaVersion
	if meta != nil {
		metaVersion = meta.Version
	}

	return &SyncRequest{
		NodeID:      nodeID,
		ProjectKey:  projectKey,
		Heads:       localHeads,
		Events:      eventsToPush,
		MetaVersion: metaVersion,
		Meta:        meta,
	}, nil
}

// findMissingEvents does a BFS backward from fromHeads, stopping at knownHeads.
func (e *Engine) findMissingEvents(ctx context.Context, fromHeads, knownHeads []domain.EventID) ([]domain.Event, error) {
	if len(fromHeads) == 0 {
		return nil, nil
	}

	knownSet := make(map[domain.EventID]struct{}, len(knownHeads))
	for _, h := range knownHeads {
		knownSet[h] = struct{}{}
	}

	visited := make(map[domain.EventID]struct{})
	queue := make([]domain.EventID, 0, len(fromHeads))
	var result []domain.Event

	for _, h := range fromHeads {
		if _, ok := knownSet[h]; !ok {
			if _, ok := visited[h]; !ok {
				queue = append(queue, h)
				visited[h] = struct{}{}
			}
		}
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		event, err := e.db.GetEvent(ctx, curr)
		if err != nil {
			return nil, fmt.Errorf("getting event %s: %w", curr, err)
		}
		if event == nil {
			continue
		}

		result = append(result, *event)

		for _, pid := range event.ParentEventIDs {
			if _, ok := knownSet[pid]; ok {
				continue
			}
			if _, ok := visited[pid]; ok {
				continue
			}
			visited[pid] = struct{}{}
			queue = append(queue, pid)
		}
	}

	return result, nil
}

// rematerialize re-reduces all events for a work item and upserts the result.
func (e *Engine) rematerialize(ctx context.Context, workItemID domain.WorkItemID) error {
	events, err := e.db.GetEventsForWorkItem(ctx, workItemID)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	ordered := domain.CausalOrder(events)
	wi, err := domain.Reduce(ordered)
	if err != nil {
		return err
	}

	// Preserve existing shared ID.
	existing, err := e.db.GetWorkItem(ctx, workItemID)
	if err != nil {
		return err
	}
	if existing != nil && existing.SharedID != "" {
		wi.SharedID = existing.SharedID
	}

	return e.db.UpsertWorkItem(ctx, wi)
}

// verifyEventSignatures checks signatures based on engine's signature mode.
// In reject mode, returns an error on invalid/missing signatures.
// In warn mode, logs warnings but returns nil.
func (e *Engine) verifyEventSignatures(ctx context.Context, events []domain.Event) error {
	reject := e.signatureMode == SignatureModeReject

	for _, evt := range events {
		if len(evt.Signature) == 0 {
			if reject {
				return fmt.Errorf("event %s has no signature (reject mode)", evt.ID)
			}
			e.logger.Debug("event has no signature", "event_id", evt.ID, "actor_id", evt.ActorID)
			continue
		}

		pubKeyHex, err := e.db.GetActorPublicKey(ctx, evt.ActorID)
		if err != nil || pubKeyHex == "" {
			e.logger.Debug("no public key for actor, skipping verification", "actor_id", evt.ActorID)
			continue
		}

		pubKeyBytes, err := hex.DecodeString(pubKeyHex)
		if err != nil {
			e.logger.Warn("invalid public key for actor", "actor_id", evt.ActorID, "error", err)
			continue
		}

		valid, err := crypto.VerifyEvent(&evt, ed25519.PublicKey(pubKeyBytes))
		if err != nil {
			if reject {
				return fmt.Errorf("signature verification failed for event %s: %w", evt.ID, err)
			}
			e.logger.Warn("signature verification error", "event_id", evt.ID, "error", err)
			continue
		}
		if !valid {
			if reject {
				return fmt.Errorf("invalid signature on event %s from actor %s (reject mode)", evt.ID, evt.ActorID)
			}
			e.logger.Warn("INVALID SIGNATURE", "event_id", evt.ID, "actor_id", evt.ActorID)
		} else {
			e.logger.Debug("signature verified", "event_id", evt.ID)
		}
	}
	return nil
}

// advisoryProtocolValidation checks pushed events against current work item state.
// Violations are logged at WARN level but events are not rejected — the reducer
// handles concurrent offline divergence via operational lineage.
func (e *Engine) advisoryProtocolValidation(ctx context.Context, events []domain.Event) {
	// Cache loaded work items to avoid repeated DB lookups.
	wiCache := make(map[domain.WorkItemID]*domain.WorkItem)

	for _, evt := range events {
		wi, ok := wiCache[evt.WorkItemID]
		if !ok {
			wi, _ = e.db.GetWorkItem(ctx, evt.WorkItemID)
			wiCache[evt.WorkItemID] = wi // may be nil for new work items
		}

		if err := domain.ValidateProtocol(evt, wi); err != nil {
			e.logger.Warn("protocol violation (advisory)",
				"event_id", evt.ID,
				"event_type", evt.Type,
				"work_item_id", evt.WorkItemID,
				"actor_id", evt.ActorID,
				"violation", err.Error(),
			)
		}
	}
}
