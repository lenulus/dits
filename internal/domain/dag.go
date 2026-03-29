package domain

import "sort"

// CausalOrder returns events in deterministic causal order using Kahn's algorithm.
// Tiebreak for concurrent events: timestamp ascending, then EventID lexically ascending.
func CausalOrder(events []Event) []Event {
	if len(events) <= 1 {
		return events
	}

	// Build adjacency: parent -> children, and in-degree map.
	eventMap := make(map[EventID]*Event, len(events))
	eventSet := make(map[EventID]struct{}, len(events))
	children := make(map[EventID][]EventID)
	inDegree := make(map[EventID]int, len(events))

	for i := range events {
		e := &events[i]
		eventMap[e.ID] = e
		eventSet[e.ID] = struct{}{}
		inDegree[e.ID] = 0
	}

	// Only count parent edges where the parent is in our event set.
	for i := range events {
		e := &events[i]
		for _, pid := range e.ParentEventIDs {
			if _, ok := eventSet[pid]; ok {
				inDegree[e.ID]++
				children[pid] = append(children[pid], e.ID)
			}
		}
	}

	// Collect roots (in-degree 0).
	var queue []EventID
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sortEventIDs(queue, eventMap)

	result := make([]Event, 0, len(events))
	for len(queue) > 0 {
		// Pop first (lowest priority).
		curr := queue[0]
		queue = queue[1:]
		result = append(result, *eventMap[curr])

		for _, childID := range children[curr] {
			inDegree[childID]--
			if inDegree[childID] == 0 {
				queue = append(queue, childID)
			}
		}
		// Re-sort queue after adding new candidates.
		sortEventIDs(queue, eventMap)
	}

	return result
}

// Heads returns the DAG tip events (events that are not a parent of any other event in the set).
func Heads(events []Event) []EventID {
	eventSet := make(map[EventID]struct{}, len(events))
	for _, e := range events {
		eventSet[e.ID] = struct{}{}
	}

	isParent := make(map[EventID]struct{})
	for _, e := range events {
		for _, pid := range e.ParentEventIDs {
			if _, ok := eventSet[pid]; ok {
				isParent[pid] = struct{}{}
			}
		}
	}

	var heads []EventID
	for _, e := range events {
		if _, ok := isParent[e.ID]; !ok {
			heads = append(heads, e.ID)
		}
	}
	return heads
}

// sortEventIDs sorts event IDs by: timestamp ascending, then EventID lexically ascending.
func sortEventIDs(ids []EventID, eventMap map[EventID]*Event) {
	sort.Slice(ids, func(i, j int) bool {
		ei := eventMap[ids[i]]
		ej := eventMap[ids[j]]
		if !ei.Timestamp.Equal(ej.Timestamp) {
			return ei.Timestamp.Before(ej.Timestamp)
		}
		return ei.ID < ej.ID
	})
}
