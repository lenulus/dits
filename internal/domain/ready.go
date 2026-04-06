package domain

// IsReady determines if a work item is operationally actionable.
// A work item is ready when:
//   - Not blocked
//   - No active lease holder (unleased or expired — caller must handle expiry before calling)
//   - No running authoritative attempt
//   - Status is in the provided set of actionable statuses (open-category)
//
// This is the derived contract behind the `ready=true` query parameter.
func IsReady(wi *WorkItem, openStatuses []string) bool {
	if wi == nil {
		return false
	}

	// Blocked work items are not ready.
	if wi.Blocked {
		return false
	}

	// Leased work items are not ready (caller should clear expired leases first).
	if wi.LeaseHolder != nil {
		return false
	}

	// Running authoritative attempts block readiness.
	for _, a := range wi.Attempts {
		if a.Status == "running" && a.Authoritative {
			return false
		}
	}

	// Status must be in an actionable set.
	if len(openStatuses) > 0 {
		statusOK := false
		for _, s := range openStatuses {
			if wi.Status == s {
				statusOK = true
				break
			}
		}
		if !statusOK {
			return false
		}
	}

	return true
}
