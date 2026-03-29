package domain

import "time"

type Issue struct {
	ID         CanonicalID
	SharedID   SharedID
	Title      string
	Body       string
	Status     string
	TypeSlug   string
	Priority   string
	Labels     []string
	Assignees  []ActorID
	CreatedBy  ActorID
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ClosedAt   *time.Time
	Comments   []Comment
	EventCount int
	HeadEvents []EventID
}

type Comment struct {
	EventID   EventID
	ActorID   ActorID
	Body      string
	Timestamp time.Time
}
