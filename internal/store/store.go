package store

import (
	"context"

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
}

type DB interface {
	EventStore
	IssueStore
	MetaStore
	Close() error
}
