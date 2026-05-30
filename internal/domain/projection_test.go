package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReduce_FieldSet covers the generic scalar projection: latest write per
// field wins, distinct fields coexist.
func TestReduce_FieldSet(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated, ActorID: "a", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "M", Kind: "milestone"})},
		{ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkFieldSet, ParentEventIDs: []EventID{"evt_001"}, ActorID: "a", Timestamp: t0.Add(time.Minute),
			Payload: MustMarshalPayload(FieldSetPayload{Field: "ryg", Value: "y"})},
		{ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkFieldSet, ParentEventIDs: []EventID{"evt_002"}, ActorID: "a", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(FieldSetPayload{Field: "target", Value: "2026 Q3"})},
		{ID: "evt_004", WorkItemID: "wrk_001", Type: EventWorkFieldSet, ParentEventIDs: []EventID{"evt_003"}, ActorID: "a", Timestamp: t0.Add(3 * time.Minute),
			Payload: MustMarshalPayload(FieldSetPayload{Field: "ryg", Value: "g"})}, // latest wins
		{ID: "evt_005", WorkItemID: "wrk_001", Type: EventWorkFieldSet, ParentEventIDs: []EventID{"evt_004"}, ActorID: "a", Timestamp: t0.Add(4 * time.Minute),
			Payload: MustMarshalPayload(FieldSetPayload{Field: "customer_visible", Value: "true"})},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, "g", wi.Fields["ryg"])
	assert.Equal(t, "2026 Q3", wi.Fields["target"])
	assert.Equal(t, "true", wi.Fields["customer_visible"])
}

// TestReduce_ScheduleSet covers wholesale replacement of the staged timeline.
func TestReduce_ScheduleSet(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated, ActorID: "a", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "M", Kind: "milestone"})},
		{ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkScheduleSet, ParentEventIDs: []EventID{"evt_001"}, ActorID: "a", Timestamp: t0.Add(time.Minute),
			Payload: MustMarshalPayload(ScheduleSetPayload{Stages: []ScheduleStage{
				{Key: "dogfood", Label: "Dogfood", Date: "2026-06-15", Precision: "D", State: "done"},
				{Key: "beta", Label: "Beta", Date: "2026 Q3", Precision: "Q", State: "open"},
			}})},
		{ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkScheduleSet, ParentEventIDs: []EventID{"evt_002"}, ActorID: "a", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(ScheduleSetPayload{Stages: []ScheduleStage{
				{Key: "ga", Label: "GA", Date: "2026-09-30", Precision: "D", State: "open"},
			}})}, // replaces wholesale
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	require.Len(t, wi.Stages, 1)
	assert.Equal(t, "ga", wi.Stages[0].Key)
	assert.Equal(t, "GA", wi.Stages[0].Label)
	assert.Equal(t, "D", wi.Stages[0].Precision)
}

// TestReduce_ScheduleSetEmptyClears verifies an empty stage list clears the timeline.
func TestReduce_ScheduleSetEmptyClears(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated, ActorID: "a", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "M", Kind: "milestone"})},
		{ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkScheduleSet, ParentEventIDs: []EventID{"evt_001"}, ActorID: "a", Timestamp: t0.Add(time.Minute),
			Payload: MustMarshalPayload(ScheduleSetPayload{Stages: []ScheduleStage{{Key: "beta", Label: "Beta"}}})},
		{ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkScheduleSet, ParentEventIDs: []EventID{"evt_002"}, ActorID: "a", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(ScheduleSetPayload{Stages: nil})},
	}
	wi, err := Reduce(events)
	require.NoError(t, err)
	assert.Empty(t, wi.Stages)
}

// TestValidate_FieldSet checks the only validation rule: field is required.
func TestValidate_FieldSet(t *testing.T) {
	meta := DefaultMetaConfig("TST")
	ok := Event{Type: EventWorkFieldSet, Payload: MustMarshalPayload(FieldSetPayload{Field: "ryg", Value: "g"})}
	assert.NoError(t, ValidateEvent(ok, &meta))

	bad := Event{Type: EventWorkFieldSet, Payload: MustMarshalPayload(FieldSetPayload{Field: "", Value: "g"})}
	assert.Error(t, ValidateEvent(bad, &meta))
}

// TestSetTaxonomyNodeMetadata round-trips a node's opaque metadata blob.
func TestSetTaxonomyNodeMetadata(t *testing.T) {
	m := DefaultMetaConfig("TST")
	require.NoError(t, m.AddTaxonomy(Taxonomy{Slug: "goals", Name: "Goals", Nodes: []TaxonomyNode{
		{Slug: "customer-trust", Name: "Customer trust"},
	}}))

	err := m.SetTaxonomyNodeMetadata("goals", "customer-trust", []byte(`{"result":"achieved","resultNote":"1 incident"}`))
	require.NoError(t, err)

	n := m.GetTaxonomyNode("goals", "customer-trust")
	require.NotNil(t, n)
	assert.JSONEq(t, `{"result":"achieved","resultNote":"1 incident"}`, string(n.Metadata))

	// Unknown node errors.
	assert.Error(t, m.SetTaxonomyNodeMetadata("goals", "nope", []byte(`{}`)))
}
