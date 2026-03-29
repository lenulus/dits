package sync

import (
	"context"
	"fmt"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/store"
)

// Engine handles sync logic for both server and client sides.
// The same logic applies: ingest foreign events, compute missing events to send back.
type Engine struct {
	db store.DB
}

func NewEngine(db store.DB) *Engine {
	return &Engine{db: db}
}

// HandleSync processes a sync request (server-side).
// 1. Ingests client events
// 2. Assigns shared IDs to new issues
// 3. Rematerializes affected issues
// 4. Computes events the client is missing
// 5. Returns response
func (e *Engine) HandleSync(ctx context.Context, req SyncRequest) (*SyncResponse, error) {
	// 1. Ingest client events (INSERT OR IGNORE for idempotency).
	if len(req.Events) > 0 {
		if err := e.db.AppendEvents(ctx, req.Events); err != nil {
			return nil, fmt.Errorf("ingesting events: %w", err)
		}
	}

	// 1b. Accept client meta if it has a higher version than ours.
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

	// 2. Assign shared IDs to new issues and rematerialize.
	sharedIDs := make(map[domain.CanonicalID]domain.SharedID)
	affectedIssues, err := e.db.GetAffectedIssueIDs(ctx, req.Events)
	if err != nil {
		return nil, err
	}

	for _, issueID := range affectedIssues {
		if err := e.rematerialize(ctx, issueID); err != nil {
			return nil, fmt.Errorf("rematerializing %s: %w", issueID, err)
		}

		// Check if issue needs a shared ID.
		issue, err := e.db.GetIssue(ctx, issueID)
		if err != nil {
			return nil, err
		}
		if issue != nil && issue.SharedID == "" && meta != nil {
			sid, err := e.db.AllocateSharedID(ctx, issueID, meta.ProjectKey)
			if err != nil {
				return nil, fmt.Errorf("allocating shared ID: %w", err)
			}
			sharedIDs[issueID] = sid
		}
	}

	// 3. Compute events the client needs.
	// Walk from our heads backward, stopping at the client's known heads.
	serverHeads, err := e.db.GetAllHeads(ctx)
	if err != nil {
		return nil, err
	}

	missingEvents, err := e.findMissingEvents(ctx, serverHeads, req.Heads)
	if err != nil {
		return nil, fmt.Errorf("finding missing events: %w", err)
	}

	// 4. Include shared IDs for any issues in the pulled events that the client doesn't know about.
	pulledIssueIDs, _ := e.db.GetAffectedIssueIDs(ctx, missingEvents)
	for _, issueID := range pulledIssueIDs {
		if _, ok := sharedIDs[issueID]; ok {
			continue // already included from step 2
		}
		issue, err := e.db.GetIssue(ctx, issueID)
		if err != nil {
			return nil, err
		}
		if issue != nil && issue.SharedID != "" {
			sharedIDs[issueID] = issue.SharedID
		}
	}

	// 5. Store the client's heads as remote heads for this node.
	// After sync, the client will have our heads, so store our heads as their known state.
	newClientHeads := serverHeads // after sync, client will have all our events

	resp := &SyncResponse{
		Events:    missingEvents,
		SharedIDs: sharedIDs,
		Heads:     serverHeads,
	}

	// Include meta if client is behind.
	if meta != nil && req.MetaVersion < meta.Version {
		resp.Meta = meta
	}

	// Update stored remote heads for this client.
	if err := e.db.SetRemoteHeads(ctx, req.NodeID, newClientHeads); err != nil {
		return nil, err
	}

	return resp, nil
}

// ApplySync processes a sync response (client-side).
// 1. Ingests server events
// 2. Rematerializes affected issues
// 3. Applies shared IDs
// 4. Stores server heads
func (e *Engine) ApplySync(ctx context.Context, resp *SyncResponse, serverNodeID domain.NodeID) error {
	// 1. Ingest server events.
	if len(resp.Events) > 0 {
		if err := e.db.AppendEvents(ctx, resp.Events); err != nil {
			return fmt.Errorf("ingesting server events: %w", err)
		}
	}

	// 2. Rematerialize affected issues (must happen before shared ID assignment).
	affectedIssues, err := e.db.GetAffectedIssueIDs(ctx, resp.Events)
	if err != nil {
		return err
	}
	for _, issueID := range affectedIssues {
		if err := e.rematerialize(ctx, issueID); err != nil {
			return fmt.Errorf("rematerializing %s: %w", issueID, err)
		}
	}

	// 3. Apply shared IDs from server.
	for issueID, sharedID := range resp.SharedIDs {
		issue, err := e.db.GetIssue(ctx, issueID)
		if err != nil {
			return err
		}
		if issue != nil && issue.SharedID == "" {
			issue.SharedID = sharedID
			if err := e.db.UpsertIssue(ctx, issue); err != nil {
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

	// Find events to push: events reachable from local heads but not from last-known server heads.
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

// findMissingEvents does a BFS backward from `fromHeads` through parent edges,
// stopping at events in `knownHeads` (or their ancestors). Returns all visited
// events that are not ancestors of knownHeads.
func (e *Engine) findMissingEvents(ctx context.Context, fromHeads, knownHeads []domain.EventID) ([]domain.Event, error) {
	if len(fromHeads) == 0 {
		return nil, nil
	}

	// Build known set from the provided heads.
	knownSet := make(map[domain.EventID]struct{}, len(knownHeads))
	for _, h := range knownHeads {
		knownSet[h] = struct{}{}
	}

	// BFS from fromHeads backward.
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

// rematerialize re-reduces all events for an issue and upserts the result.
func (e *Engine) rematerialize(ctx context.Context, issueID domain.CanonicalID) error {
	events, err := e.db.GetEventsForIssue(ctx, issueID)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	ordered := domain.CausalOrder(events)
	issue, err := domain.Reduce(ordered)
	if err != nil {
		return err
	}

	// Preserve existing shared ID.
	existing, err := e.db.GetIssue(ctx, issueID)
	if err != nil {
		return err
	}
	if existing != nil && existing.SharedID != "" {
		issue.SharedID = existing.SharedID
	}

	return e.db.UpsertIssue(ctx, issue)
}
