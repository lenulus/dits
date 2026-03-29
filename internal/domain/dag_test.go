package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCausalOrder_Linear(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events := []Event{
		{ID: "evt_C", IssueID: "iss_1", ParentEventIDs: []EventID{"evt_B"}, Timestamp: t0.Add(2 * time.Second)},
		{ID: "evt_A", IssueID: "iss_1", ParentEventIDs: nil, Timestamp: t0},
		{ID: "evt_B", IssueID: "iss_1", ParentEventIDs: []EventID{"evt_A"}, Timestamp: t0.Add(1 * time.Second)},
	}

	ordered := CausalOrder(events)
	require.Len(t, ordered, 3)
	assert.Equal(t, EventID("evt_A"), ordered[0].ID)
	assert.Equal(t, EventID("evt_B"), ordered[1].ID)
	assert.Equal(t, EventID("evt_C"), ordered[2].ID)
}

func TestCausalOrder_Diamond(t *testing.T) {
	// A -> B, A -> C, B -> D, C -> D (diamond)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events := []Event{
		{ID: "evt_D", IssueID: "iss_1", ParentEventIDs: []EventID{"evt_B", "evt_C"}, Timestamp: t0.Add(3 * time.Second)},
		{ID: "evt_B", IssueID: "iss_1", ParentEventIDs: []EventID{"evt_A"}, Timestamp: t0.Add(1 * time.Second)},
		{ID: "evt_C", IssueID: "iss_1", ParentEventIDs: []EventID{"evt_A"}, Timestamp: t0.Add(2 * time.Second)},
		{ID: "evt_A", IssueID: "iss_1", ParentEventIDs: nil, Timestamp: t0},
	}

	ordered := CausalOrder(events)
	require.Len(t, ordered, 4)
	assert.Equal(t, EventID("evt_A"), ordered[0].ID)
	// B before C because B has earlier timestamp
	assert.Equal(t, EventID("evt_B"), ordered[1].ID)
	assert.Equal(t, EventID("evt_C"), ordered[2].ID)
	assert.Equal(t, EventID("evt_D"), ordered[3].ID)
}

func TestCausalOrder_ConcurrentTiebreak(t *testing.T) {
	// Two concurrent events with same timestamp, tiebreak by ID.
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events := []Event{
		{ID: "evt_Z", IssueID: "iss_1", ParentEventIDs: []EventID{"evt_A"}, Timestamp: t0.Add(1 * time.Second)},
		{ID: "evt_A", IssueID: "iss_1", ParentEventIDs: nil, Timestamp: t0},
		{ID: "evt_M", IssueID: "iss_1", ParentEventIDs: []EventID{"evt_A"}, Timestamp: t0.Add(1 * time.Second)},
	}

	ordered := CausalOrder(events)
	require.Len(t, ordered, 3)
	assert.Equal(t, EventID("evt_A"), ordered[0].ID)
	// M < Z lexically
	assert.Equal(t, EventID("evt_M"), ordered[1].ID)
	assert.Equal(t, EventID("evt_Z"), ordered[2].ID)
}

func TestCausalOrder_Empty(t *testing.T) {
	assert.Empty(t, CausalOrder(nil))
}

func TestCausalOrder_Single(t *testing.T) {
	events := []Event{{ID: "evt_A", IssueID: "iss_1", Timestamp: time.Now()}}
	ordered := CausalOrder(events)
	require.Len(t, ordered, 1)
	assert.Equal(t, EventID("evt_A"), ordered[0].ID)
}

func TestHeads_Linear(t *testing.T) {
	events := []Event{
		{ID: "evt_A", ParentEventIDs: nil},
		{ID: "evt_B", ParentEventIDs: []EventID{"evt_A"}},
		{ID: "evt_C", ParentEventIDs: []EventID{"evt_B"}},
	}
	heads := Heads(events)
	assert.Equal(t, []EventID{"evt_C"}, heads)
}

func TestHeads_Diamond(t *testing.T) {
	events := []Event{
		{ID: "evt_A", ParentEventIDs: nil},
		{ID: "evt_B", ParentEventIDs: []EventID{"evt_A"}},
		{ID: "evt_C", ParentEventIDs: []EventID{"evt_A"}},
		{ID: "evt_D", ParentEventIDs: []EventID{"evt_B", "evt_C"}},
	}
	heads := Heads(events)
	assert.Equal(t, []EventID{"evt_D"}, heads)
}

func TestHeads_Diverged(t *testing.T) {
	// Two branches, not merged.
	events := []Event{
		{ID: "evt_A", ParentEventIDs: nil},
		{ID: "evt_B", ParentEventIDs: []EventID{"evt_A"}},
		{ID: "evt_C", ParentEventIDs: []EventID{"evt_A"}},
	}
	heads := Heads(events)
	assert.ElementsMatch(t, []EventID{"evt_B", "evt_C"}, heads)
}
