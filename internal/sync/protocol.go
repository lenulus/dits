package sync

import "github.com/lenulus/pf/internal/domain"

// SyncRequest is sent by the client to the server.
type SyncRequest struct {
	NodeID      domain.NodeID      `json:"node_id"`
	ProjectKey  string             `json:"project_key"`
	Heads       []domain.EventID   `json:"heads"`
	Events      []domain.Event     `json:"events"`
	MetaVersion domain.MetaVersion `json:"meta_version"`
	Meta        *domain.MetaConfig `json:"meta,omitempty"`
	ActorID     domain.ActorID     `json:"actor_id,omitempty"`
	PublicKey   string             `json:"public_key,omitempty"`
}

// SyncResponse is returned by the server to the client.
type SyncResponse struct {
	Events    []domain.Event                    `json:"events"`
	SharedIDs map[domain.CanonicalID]domain.SharedID `json:"shared_ids,omitempty"`
	Heads     []domain.EventID                  `json:"heads"`
	Meta      *domain.MetaConfig                `json:"meta,omitempty"`
}
