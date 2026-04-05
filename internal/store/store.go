package store

import (
	"context"
	"fmt"

	"github.com/lenulus/pf/internal/domain"
)

type WorkItemFilter struct {
	Status   string
	Kind     string
	Label    string
	Assignee domain.ActorID
	Blocked  *bool
	Query    string // free-text search on title/body
	Limit    int
	Offset   int
}

type EventStore interface {
	AppendEvents(ctx context.Context, events []domain.Event) error
	GetEvent(ctx context.Context, id domain.EventID) (*domain.Event, error)
	GetEventsForWorkItem(ctx context.Context, workItemID domain.WorkItemID) ([]domain.Event, error)
	GetHeads(ctx context.Context, workItemID domain.WorkItemID) ([]domain.EventID, error)
	GetAllHeads(ctx context.Context) ([]domain.EventID, error)
	HasEvent(ctx context.Context, id domain.EventID) (bool, error)
	GetAffectedWorkItemIDs(ctx context.Context, events []domain.Event) ([]domain.WorkItemID, error)
}

type WorkItemStore interface {
	UpsertWorkItem(ctx context.Context, workItem *domain.WorkItem) error
	GetWorkItem(ctx context.Context, id domain.WorkItemID) (*domain.WorkItem, error)
	GetWorkItemBySharedID(ctx context.Context, id domain.SharedID) (*domain.WorkItem, error)
	ListWorkItems(ctx context.Context, filter WorkItemFilter) ([]domain.WorkItem, error)
	AllocateSharedID(ctx context.Context, workItemID domain.WorkItemID, projectKey string) (domain.SharedID, error)
}

type MetaStore interface {
	GetCurrentMeta(ctx context.Context) (*domain.MetaConfig, error)
	SaveMeta(ctx context.Context, meta *domain.MetaConfig) error
	SaveMetaIfVersion(ctx context.Context, meta *domain.MetaConfig, expectedVersion domain.MetaVersion) error
}

var ErrMetaConflict = fmt.Errorf("meta version conflict: not at HEAD")

type ActorStore interface {
	RegisterActor(ctx context.Context, actorID domain.ActorID, publicKey string, nodeID domain.NodeID) error
	GetActorPublicKey(ctx context.Context, actorID domain.ActorID) (string, error)
}

type SyncStore interface {
	GetRemoteHeads(ctx context.Context, nodeID domain.NodeID) ([]domain.EventID, error)
	SetRemoteHeads(ctx context.Context, nodeID domain.NodeID, heads []domain.EventID) error
	GetSyncRemoteURL(ctx context.Context, nodeID domain.NodeID) (string, error)
	SetSyncRemote(ctx context.Context, nodeID domain.NodeID, url string) error
}

type OverlayStore interface {
	SetAnnotation(ctx context.Context, workItemID domain.WorkItemID, key, value string) error
	GetAnnotations(ctx context.Context, workItemID domain.WorkItemID) (map[string]string, error)
	DeleteAnnotation(ctx context.Context, workItemID domain.WorkItemID, key string) error
	AddPrivateLabel(ctx context.Context, workItemID domain.WorkItemID, label string) error
	RemovePrivateLabel(ctx context.Context, workItemID domain.WorkItemID, label string) error
	GetPrivateLabels(ctx context.Context, workItemID domain.WorkItemID) ([]string, error)
}

type DB interface {
	EventStore
	WorkItemStore
	MetaStore
	SyncStore
	ActorStore
	OverlayStore
	Close() error
}
