package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reTestMeta builds a small meta config carrying one taxonomy (with a retired
// node) and two roles, for exercising the RE substrate events.
func reTestMeta() *MetaConfig {
	return &MetaConfig{
		Version: 1,
		Taxonomies: []Taxonomy{
			{
				Slug: "org", Name: "Organization",
				Nodes: []TaxonomyNode{
					{Slug: "acme", Name: "Acme"},
					{Slug: "acme/platform", Name: "Platform", ParentSlug: "acme"},
					{Slug: "acme/legacy", Name: "Legacy", ParentSlug: "acme", Retired: true},
				},
			},
		},
		Roles: []Role{
			{Slug: "pilot", Name: "Pilot", Cardinality: "exactly_one"},
			{Slug: "builder", Name: "Builder", Cardinality: "exactly_one"},
		},
	}
}

func reCreate(t0 time.Time) Event {
	return Event{
		ID: "evt_create", WorkItemID: "wrk_001", Type: EventWorkCreated,
		ActorID: "actor_alice", Timestamp: t0,
		Payload: MustMarshalPayload(WorkCreatedPayload{Title: "RE item", Kind: "task"}),
	}
}

func TestReduce_ClassifyDedupeAndDeclassify(t *testing.T) {
	t0 := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		reCreate(t0),
		{
			ID: "evt_c1", WorkItemID: "wrk_001", Type: EventWorkClassified,
			ParentEventIDs: []EventID{"evt_create"}, ActorID: "actor_alice", Timestamp: t0.Add(time.Minute),
			Payload: MustMarshalPayload(ClassificationPayload{TaxonomySlug: "org", NodeSlug: "acme/platform"}),
		},
		{
			ID: "evt_c2", WorkItemID: "wrk_001", Type: EventWorkClassified,
			ParentEventIDs: []EventID{"evt_c1"}, ActorID: "actor_alice", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(ClassificationPayload{TaxonomySlug: "org", NodeSlug: "acme/platform"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	// Dedupe: two identical classify events collapse to one.
	require.Len(t, wi.Classifications, 1)
	assert.Equal(t, "org", wi.Classifications[0].TaxonomySlug)
	assert.Equal(t, "acme/platform", wi.Classifications[0].NodeSlug)
	assert.Equal(t, t0.Add(2*time.Minute), wi.UpdatedAt)

	// Declassify removes it.
	events = append(events, Event{
		ID: "evt_d1", WorkItemID: "wrk_001", Type: EventWorkDeclassified,
		ParentEventIDs: []EventID{"evt_c2"}, ActorID: "actor_alice", Timestamp: t0.Add(3 * time.Minute),
		Payload: MustMarshalPayload(ClassificationPayload{TaxonomySlug: "org", NodeSlug: "acme/platform"}),
	})
	wi, err = Reduce(events)
	require.NoError(t, err)
	assert.Empty(t, wi.Classifications)
}

func TestReduce_RoleBindReplaceAndUnbind(t *testing.T) {
	t0 := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		reCreate(t0),
		{
			ID: "evt_b1", WorkItemID: "wrk_001", Type: EventWorkRoleBound,
			ParentEventIDs: []EventID{"evt_create"}, ActorID: "actor_alice", Timestamp: t0.Add(time.Minute),
			Payload: MustMarshalPayload(RoleBindingPayload{RoleSlug: "pilot", Actor: "kokafor"}),
		},
		{
			ID: "evt_b2", WorkItemID: "wrk_001", Type: EventWorkRoleBound,
			ParentEventIDs: []EventID{"evt_b1"}, ActorID: "actor_alice", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(RoleBindingPayload{RoleSlug: "pilot", Actor: "krivas"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	// Re-binding the same role replaces the actor (cardinality at materialization).
	require.Len(t, wi.RoleBindings, 1)
	assert.Equal(t, "pilot", wi.RoleBindings[0].RoleSlug)
	assert.Equal(t, ActorID("krivas"), wi.RoleBindings[0].Actor)

	// Unbind removes the binding for that role+actor.
	events = append(events, Event{
		ID: "evt_u1", WorkItemID: "wrk_001", Type: EventWorkRoleUnbound,
		ParentEventIDs: []EventID{"evt_b2"}, ActorID: "actor_alice", Timestamp: t0.Add(3 * time.Minute),
		Payload: MustMarshalPayload(RoleBindingPayload{RoleSlug: "pilot", Actor: "krivas"}),
	})
	wi, err = Reduce(events)
	require.NoError(t, err)
	assert.Empty(t, wi.RoleBindings)
}

func TestValidateEvent_RESubstrate(t *testing.T) {
	meta := reTestMeta()
	mk := func(etype EventType, payload any) Event {
		return Event{
			ID: "evt_x", WorkItemID: "wrk_001", Type: etype,
			ActorID: "actor_alice", Timestamp: time.Now().UTC(),
			Payload: MustMarshalPayload(payload),
		}
	}

	// Classify: valid active node passes.
	require.NoError(t, ValidateEvent(mk(EventWorkClassified, ClassificationPayload{TaxonomySlug: "org", NodeSlug: "acme/platform"}), meta))
	// Unknown taxonomy rejected.
	require.Error(t, ValidateEvent(mk(EventWorkClassified, ClassificationPayload{TaxonomySlug: "nope", NodeSlug: "acme"}), meta))
	// Unknown node rejected.
	require.Error(t, ValidateEvent(mk(EventWorkClassified, ClassificationPayload{TaxonomySlug: "org", NodeSlug: "ghost"}), meta))
	// Retired node rejected for classify.
	require.Error(t, ValidateEvent(mk(EventWorkClassified, ClassificationPayload{TaxonomySlug: "org", NodeSlug: "acme/legacy"}), meta))
	// Declassify tolerates a retired node (it still exists).
	require.NoError(t, ValidateEvent(mk(EventWorkDeclassified, ClassificationPayload{TaxonomySlug: "org", NodeSlug: "acme/legacy"}), meta))

	// Role bind: known role passes, unknown role rejected.
	require.NoError(t, ValidateEvent(mk(EventWorkRoleBound, RoleBindingPayload{RoleSlug: "pilot", Actor: "kokafor"}), meta))
	require.Error(t, ValidateEvent(mk(EventWorkRoleBound, RoleBindingPayload{RoleSlug: "ghost", Actor: "kokafor"}), meta))
	require.Error(t, ValidateEvent(mk(EventWorkRoleUnbound, RoleBindingPayload{RoleSlug: "ghost", Actor: "kokafor"}), meta))
}
