package domain

import (
	"encoding/json"
	"fmt"
)

// ValidateEvent checks that an event's references (kinds, labels, statuses, priorities,
// artifact types, relation types) are valid against the given meta configuration.
func ValidateEvent(e Event, meta *MetaConfig) error {
	if meta == nil {
		return nil
	}

	switch e.Type {
	case EventWorkCreated:
		var p WorkCreatedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if p.Kind != "" && !meta.HasWorkKind(p.Kind) {
			return fmt.Errorf("unknown work kind %q", p.Kind)
		}

	case EventWorkStatusSet:
		var p StatusSetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !meta.HasStatus(p.To) {
			return fmt.Errorf("unknown status %q", p.To)
		}

	case EventWorkLabelAdded:
		var p LabelPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !meta.HasLabel(p.LabelSlug) {
			return fmt.Errorf("unknown label %q", p.LabelSlug)
		}

	case EventWorkPrioritySet:
		var p PrioritySetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !meta.HasPriority(p.Priority) {
			return fmt.Errorf("unknown priority %q", p.Priority)
		}

	case EventWorkArtifactAdded:
		var p ArtifactAddedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if p.ArtifactType != "" && !meta.HasArtifactType(p.ArtifactType) {
			return fmt.Errorf("unknown artifact type %q", p.ArtifactType)
		}

	case EventWorkEvidenceAttached:
		var p EvidenceAttachedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if p.ArtifactType != "" && !meta.HasArtifactType(p.ArtifactType) {
			return fmt.Errorf("unknown artifact type %q", p.ArtifactType)
		}

	case EventWorkLinked, EventWorkUnlinked:
		var p RelationPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !meta.HasRelationType(p.RelationType) {
			return fmt.Errorf("unknown relation type %q", p.RelationType)
		}

	case EventWorkClassified, EventWorkDeclassified:
		var p ClassificationPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !meta.HasTaxonomy(p.TaxonomySlug) {
			return fmt.Errorf("unknown taxonomy %q", p.TaxonomySlug)
		}
		// The node must exist; for new classifications it must also be active.
		if e.Type == EventWorkClassified {
			if !meta.TaxonomyHasActiveNode(p.TaxonomySlug, p.NodeSlug) {
				return fmt.Errorf("unknown or retired node %q in taxonomy %q", p.NodeSlug, p.TaxonomySlug)
			}
		} else if !meta.TaxonomyHasNode(p.TaxonomySlug, p.NodeSlug) {
			return fmt.Errorf("unknown node %q in taxonomy %q", p.NodeSlug, p.TaxonomySlug)
		}

	case EventWorkRoleBound, EventWorkRoleUnbound:
		var p RoleBindingPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !meta.HasRole(p.RoleSlug) {
			return fmt.Errorf("unknown role %q", p.RoleSlug)
		}
	}

	return nil
}
