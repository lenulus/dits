// Shared cell renderers — small trusted-markup helpers the live view builders
// use to turn MCP DTO values into the prototype's pill/chip vocabulary. None
// of these take user input directly; values are escaped where they originate
// from substrate data.
package handlers

import (
	"fmt"
	"html/template"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// pill emits a dx-pill fragment.
func pill(variant, label string) template.HTML {
	return template.HTML(fmt.Sprintf(`<span class="dx-pill dx-pill--%s"><span class="dot"></span>%s</span>`,
		template.HTMLEscapeString(variant), template.HTMLEscapeString(label)))
}

// idCell renders the monospace shared/work-item ID.
func idCell(id string) template.HTML {
	return template.HTML(`<span class="dx-id">` + template.HTMLEscapeString(id) + `</span>`)
}

// statusPill maps a workflow status slug to a labelled pill with the RE colour
// vocabulary (mirrors views.jsx STATUS_KIND/STATUS_LABEL).
func statusPill(status string) template.HTML {
	label, variant := statusLabel(status), statusVariant(status)
	if status == "" {
		return pill("neutral", "—")
	}
	return pill(variant, label)
}

func statusLabel(s string) string {
	switch s {
	case "leadership_review":
		return "Leadership review"
	case "ack_filed":
		return "ACK filed"
	case "ack_committed":
		return "ACK committed"
	case "in_flight":
		return "In flight"
	default:
		if s == "" {
			return "—"
		}
		// Title-case-ish: replace underscores, capitalise first rune.
		out := []rune{}
		cap := true
		for _, r := range s {
			if r == '_' {
				out = append(out, ' ')
				continue
			}
			if cap && r >= 'a' && r <= 'z' {
				r = r - 32
			}
			cap = false
			out = append(out, r)
		}
		return string(out)
	}
}

func statusVariant(s string) string {
	switch s {
	case "approved", "ack_committed", "in_flight", "resolved", "completed", "achieved":
		return "g"
	case "leadership_review", "ack_filed", "resourced", "committed", "escalated":
		return "y"
	case "rejected", "aborted", "abandoned", "overdue":
		return "r"
	default:
		return "neutral"
	}
}

// ackStatePill maps a per-side ACK state token to a pill.
func ackStatePill(state string) template.HTML {
	switch state {
	case "accepted":
		return pill("g", "accepted")
	case "rejected":
		return pill("r", "rejected")
	default:
		return pill("neutral", "pending")
	}
}

// rollupPill maps an alignment rollup token to a pill (matches views.jsx
// ROLLUP_KIND/ROLLUP_LABEL).
func rollupPill(rollup string) template.HTML {
	switch rollup {
	case "aligned":
		return pill("g", "Aligned")
	case "builder_pending":
		return pill("y", "Builder pending")
	case "specifier_pending":
		return pill("y", "Specifier pending")
	case "rejected":
		return pill("r", "Rejected")
	default:
		return pill("neutral", "Both pending")
	}
}

// actorCell renders an actor reference (initials avatar would need the actor
// directory; for now show the actor id compactly).
func actorCell(actor string) template.HTML {
	if actor == "" {
		return template.HTML(`<span class="placeholder">unassigned</span>`)
	}
	return template.HTML(`<span class="dx-owner"><span class="dx-owner__name">` +
		template.HTMLEscapeString(actor) + `</span></span>`)
}

// classCell renders the leaf of a classification node path for a taxonomy, or
// a placeholder.
func classCell(item mcp.WorkItem, taxonomy string) template.HTML {
	for _, c := range item.Classifications {
		if c.TaxonomySlug == taxonomy {
			leaf := c.NodeSlug
			for i := len(c.NodeSlug) - 1; i >= 0; i-- {
				if c.NodeSlug[i] == '/' {
					leaf = c.NodeSlug[i+1:]
					break
				}
			}
			return template.HTML(`<span title="` + template.HTMLEscapeString(c.NodeSlug) + `">` +
				template.HTMLEscapeString(leaf) + `</span>`)
		}
	}
	return template.HTML(`<span class="placeholder">unclassified</span>`)
}

// diagBadge renders a count badge for a work item's diagnostics.
func diagBadge(item mcp.WorkItem) template.HTML {
	n := len(item.Diagnostics)
	if n == 0 {
		return template.HTML(`<span class="placeholder">—</span>`)
	}
	violation := false
	for _, d := range item.Diagnostics {
		if d.Severity == "violation" {
			violation = true
		}
	}
	cls := "dx-diag-count"
	if violation {
		cls += " dx-diag-count--violation"
	}
	title := ""
	for i, d := range item.Diagnostics {
		if i > 0 {
			title += " · "
		}
		title += d.Message
	}
	return template.HTML(fmt.Sprintf(`<span class="%s" title="%s">%d</span>`,
		cls, template.HTMLEscapeString(title), n))
}

// roleActor returns the actor bound to the named role on the work item, or "".
func roleActor(item mcp.WorkItem, role string) string {
	for _, rb := range item.RoleBindings {
		if rb.RoleSlug == role {
			return rb.Actor
		}
	}
	return ""
}

// latestAck returns the most recently filed ACK on the work item, or false.
func latestAck(item mcp.WorkItem) (mcp.Ack, bool) {
	if len(item.Acks) == 0 {
		return mcp.Ack{}, false
	}
	return item.Acks[len(item.Acks)-1], true
}
