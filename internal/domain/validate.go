package domain

import (
	"encoding/json"
	"fmt"
)

// ValidateEvent checks that an event's references (labels, statuses, types, priorities)
// are valid against the given meta configuration.
func ValidateEvent(e Event, meta *MetaConfig) error {
	if meta == nil {
		return nil // no meta to validate against
	}

	switch e.Type {
	case EventIssueCreated:
		var p IssueCreatedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if p.TypeSlug != "" && !meta.HasIssueType(p.TypeSlug) {
			return fmt.Errorf("unknown issue type %q", p.TypeSlug)
		}

	case EventIssueStatusSet:
		var p StatusSetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !meta.HasStatus(p.To) {
			return fmt.Errorf("unknown status %q", p.To)
		}

	case EventIssueLabelAdded:
		var p LabelPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !meta.HasLabel(p.LabelSlug) {
			return fmt.Errorf("unknown label %q", p.LabelSlug)
		}

	case EventIssuePrioritySet:
		var p PrioritySetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !meta.HasPriority(p.Priority) {
			return fmt.Errorf("unknown priority %q", p.Priority)
		}
	}

	return nil
}
