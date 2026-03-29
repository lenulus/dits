package domain

import (
	"encoding/json"
	"time"
)

type EventType string

const (
	EventIssueCreated      EventType = "issue.created"
	EventIssueTitleSet     EventType = "issue.title_set"
	EventIssueBodySet      EventType = "issue.body_set"
	EventIssueStatusSet    EventType = "issue.status_set"
	EventIssueLabelAdded   EventType = "issue.label_added"
	EventIssueLabelRemoved EventType = "issue.label_removed"
	EventIssueAssigned     EventType = "issue.assigned"
	EventIssueUnassigned   EventType = "issue.unassigned"
	EventIssueCommented    EventType = "issue.commented"
	EventIssuePrioritySet  EventType = "issue.priority_set"
	EventIssueClosed       EventType = "issue.closed"
	EventIssueReopened     EventType = "issue.reopened"
	EventSharedIDAssigned    EventType = "issue.shared_id_assigned"
	EventAttachmentAdded   EventType = "issue.attachment_added"
	EventAttachmentRemoved EventType = "issue.attachment_removed"
)

type Event struct {
	ID             EventID         `json:"id"`
	IssueID        CanonicalID     `json:"issue_id"`
	Type           EventType       `json:"type"`
	ParentEventIDs []EventID       `json:"parent_event_ids"`
	MetaVersion    MetaVersion     `json:"meta_version"`
	ActorID        ActorID         `json:"actor_id"`
	Timestamp      time.Time       `json:"timestamp"`
	Payload        json.RawMessage `json:"payload"`
	Signature      []byte          `json:"signature,omitempty"`
}

// Typed payloads for each event type.

type IssueCreatedPayload struct {
	Title    string `json:"title"`
	Body     string `json:"body,omitempty"`
	TypeSlug string `json:"type_slug,omitempty"`
}

type TitleSetPayload struct {
	Title string `json:"title"`
}

type BodySetPayload struct {
	Body string `json:"body"`
}

type StatusSetPayload struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type LabelPayload struct {
	LabelSlug string `json:"label_slug"`
}

type AssignPayload struct {
	Assignee ActorID `json:"assignee"`
}

type CommentPayload struct {
	Body string `json:"body"`
}

type PrioritySetPayload struct {
	Priority string `json:"priority"`
}

type SharedIDAssignedPayload struct {
	SharedID SharedID `json:"shared_id"`
}

type AttachmentAddedPayload struct {
	AttachmentID AttachmentID `json:"attachment_id"`
	ContentHash  string       `json:"content_hash"`
	Filename     string       `json:"filename"`
	MimeType     string       `json:"mime_type"`
	SizeBytes    int64        `json:"size_bytes"`
}

type AttachmentRemovedPayload struct {
	AttachmentID AttachmentID `json:"attachment_id"`
}

// MustMarshalPayload marshals a payload to JSON, panicking on error.
func MustMarshalPayload(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
