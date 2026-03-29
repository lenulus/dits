package store

import (
	"context"
	"fmt"

	"github.com/lenulus/pf/internal/domain"
)

type IssueFilter struct {
	Status   string
	Label    string
	Assignee domain.ActorID
	Query    string // free-text search on title/body
	Limit    int
	Offset   int
}

type EventStore interface {
	AppendEvents(ctx context.Context, events []domain.Event) error
	GetEvent(ctx context.Context, id domain.EventID) (*domain.Event, error)
	GetEventsForIssue(ctx context.Context, issueID domain.CanonicalID) ([]domain.Event, error)
	GetHeads(ctx context.Context, issueID domain.CanonicalID) ([]domain.EventID, error)
	GetAllHeads(ctx context.Context) ([]domain.EventID, error)
	HasEvent(ctx context.Context, id domain.EventID) (bool, error)
	GetAffectedIssueIDs(ctx context.Context, events []domain.Event) ([]domain.CanonicalID, error)
}

type IssueStore interface {
	UpsertIssue(ctx context.Context, issue *domain.Issue) error
	GetIssue(ctx context.Context, id domain.CanonicalID) (*domain.Issue, error)
	GetIssueBySharedID(ctx context.Context, id domain.SharedID) (*domain.Issue, error)
	ListIssues(ctx context.Context, filter IssueFilter) ([]domain.Issue, error)
	AllocateSharedID(ctx context.Context, issueID domain.CanonicalID, projectKey string) (domain.SharedID, error)
}

type MetaStore interface {
	GetCurrentMeta(ctx context.Context) (*domain.MetaConfig, error)
	SaveMeta(ctx context.Context, meta *domain.MetaConfig) error
	// SaveMetaIfVersion saves only if expectedVersion matches current HEAD.
	// Returns ErrMetaConflict if the version doesn't match.
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
	SetAnnotation(ctx context.Context, issueID domain.CanonicalID, key, value string) error
	GetAnnotations(ctx context.Context, issueID domain.CanonicalID) (map[string]string, error)
	DeleteAnnotation(ctx context.Context, issueID domain.CanonicalID, key string) error
	AddPrivateLabel(ctx context.Context, issueID domain.CanonicalID, label string) error
	RemovePrivateLabel(ctx context.Context, issueID domain.CanonicalID, label string) error
	GetPrivateLabels(ctx context.Context, issueID domain.CanonicalID) ([]string, error)
}

type DB interface {
	EventStore
	IssueStore
	MetaStore
	SyncStore
	ActorStore
	OverlayStore
	Close() error
}
