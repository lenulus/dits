package domain

import "time"

type Issue struct {
	ID          CanonicalID
	SharedID    SharedID
	Title       string
	Body        string
	Status      string
	TypeSlug    string
	Priority    string
	Labels      []string
	Assignees   []ActorID
	CreatedBy   ActorID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ClosedAt    *time.Time
	Comments    []Comment
	Attachments []Attachment
	Relations   []Relation
	EventCount  int
	HeadEvents  []EventID
}

type Relation struct {
	Type        string      // "blocks", "relates_to", "duplicates", etc.
	TargetIssue CanonicalID
}

type Comment struct {
	EventID   EventID
	ActorID   ActorID
	Body      string
	Timestamp time.Time
}

type Attachment struct {
	ID          AttachmentID
	ContentHash string
	Filename    string
	MimeType    string
	SizeBytes   int64
	AddedBy     ActorID
	AddedAt     time.Time
}
