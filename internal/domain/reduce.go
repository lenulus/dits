package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Reduce takes causally-ordered events and produces a materialized Issue.
// Events MUST be in causal order (use CausalOrder first).
func Reduce(events []Event) (*Issue, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("no events to reduce")
	}

	issue := &Issue{
		Labels:      []string{},
		Assignees:   []ActorID{},
		Comments:    []Comment{},
		Attachments: []Attachment{},
	}

	for _, e := range events {
		if err := ApplyEvent(issue, e); err != nil {
			return nil, fmt.Errorf("applying event %s: %w", e.ID, err)
		}
	}

	issue.HeadEvents = Heads(events)
	issue.EventCount = len(events)
	return issue, nil
}

// ApplyEvent applies a single event to an existing issue state.
func ApplyEvent(issue *Issue, e Event) error {
	switch e.Type {
	case EventIssueCreated:
		var p IssueCreatedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		issue.ID = e.IssueID
		issue.Title = p.Title
		issue.Body = p.Body
		issue.Status = "open"
		issue.TypeSlug = p.TypeSlug
		if issue.TypeSlug == "" {
			issue.TypeSlug = "task"
		}
		issue.Priority = "medium"
		issue.CreatedBy = e.ActorID
		issue.CreatedAt = e.Timestamp
		issue.UpdatedAt = e.Timestamp

	case EventIssueTitleSet:
		var p TitleSetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		issue.Title = p.Title
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)

	case EventIssueBodySet:
		var p BodySetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		issue.Body = p.Body
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)

	case EventIssueStatusSet:
		var p StatusSetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		issue.Status = p.To
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)

	case EventIssueLabelAdded:
		var p LabelPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !containsString(issue.Labels, p.LabelSlug) {
			issue.Labels = append(issue.Labels, p.LabelSlug)
		}
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)

	case EventIssueLabelRemoved:
		var p LabelPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		issue.Labels = removeString(issue.Labels, p.LabelSlug)
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)

	case EventIssueAssigned:
		var p AssignPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !containsActor(issue.Assignees, p.Assignee) {
			issue.Assignees = append(issue.Assignees, p.Assignee)
		}
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)

	case EventIssueUnassigned:
		var p AssignPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		issue.Assignees = removeActor(issue.Assignees, p.Assignee)
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)

	case EventIssueCommented:
		var p CommentPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		issue.Comments = append(issue.Comments, Comment{
			EventID:   e.ID,
			ActorID:   e.ActorID,
			Body:      p.Body,
			Timestamp: e.Timestamp,
		})
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)

	case EventIssuePrioritySet:
		var p PrioritySetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		issue.Priority = p.Priority
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)

	case EventIssueClosed:
		issue.Status = "closed"
		t := e.Timestamp
		issue.ClosedAt = &t
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)

	case EventIssueReopened:
		issue.Status = "open"
		issue.ClosedAt = nil
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)

	case EventSharedIDAssigned:
		var p SharedIDAssignedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		issue.SharedID = p.SharedID
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)

	case EventAttachmentAdded:
		var p AttachmentAddedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !containsAttachment(issue.Attachments, p.AttachmentID) {
			issue.Attachments = append(issue.Attachments, Attachment{
				ID:          p.AttachmentID,
				ContentHash: p.ContentHash,
				Filename:    p.Filename,
				MimeType:    p.MimeType,
				SizeBytes:   p.SizeBytes,
				AddedBy:     e.ActorID,
				AddedAt:     e.Timestamp,
			})
		}
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)

	case EventAttachmentRemoved:
		var p AttachmentRemovedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		issue.Attachments = removeAttachment(issue.Attachments, p.AttachmentID)
		issue.UpdatedAt = maxTime(issue.UpdatedAt, e.Timestamp)
	}

	return nil
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func removeString(s []string, v string) []string {
	result := make([]string, 0, len(s))
	for _, x := range s {
		if x != v {
			result = append(result, x)
		}
	}
	return result
}

func containsActor(s []ActorID, v ActorID) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func removeActor(s []ActorID, v ActorID) []ActorID {
	result := make([]ActorID, 0, len(s))
	for _, x := range s {
		if x != v {
			result = append(result, x)
		}
	}
	return result
}

func containsAttachment(s []Attachment, id AttachmentID) bool {
	for _, x := range s {
		if x.ID == id {
			return true
		}
	}
	return false
}

func removeAttachment(s []Attachment, id AttachmentID) []Attachment {
	result := make([]Attachment, 0, len(s))
	for _, x := range s {
		if x.ID != id {
			result = append(result, x)
		}
	}
	return result
}
