// Shared cell renderers — small trusted-markup helpers the live view builders
// use to turn MCP DTO values into the prototype's pill/chip vocabulary. None
// of these take user input directly; values are escaped where they originate
// from substrate data.
package handlers

import (
	"fmt"
	"html/template"
	"strings"

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

// actorCell renders an actor reference as an initials avatar + name. The
// substrate ActorRecord carries no display name or colour, so both are derived
// deterministically from the actor id (Track E enriches this with the live
// ActorList directory + a searchable picker).
func actorCell(actor string) template.HTML {
	if actor == "" {
		return template.HTML(`<span class="placeholder">unassigned</span>`)
	}
	return template.HTML(`<span class="dx-owner">` + string(avatarHTML(actor)) +
		`<span class="dx-owner__name">` + template.HTMLEscapeString(actor) + `</span></span>`)
}

// editableActorCell wraps actorCell in a .sht-cell whose click opens the Track E
// ActorPicker for the given work item + role. Tracks B/C use this to make an
// actor/role cell editable: the click hx-gets /partials/actor-picker (which
// returns the popover), and a selection POSTs back the avatar cell. id is the
// work-item id, role the role slug to bind (specifier/builder/pilot/owner), and
// bound the currently bound actor (passed through so the picker can offer
// Unassign and unbind-before-rebind). Keeps actorCell/avatarHTML untouched.
func editableActorCell(id, role, bound string) template.HTML {
	esc := template.HTMLEscapeString
	get := "/partials/actor-picker?id=" + esc(id) + "&role=" + esc(role)
	if bound != "" {
		get += "&bound=" + esc(bound)
	}
	return template.HTML(`<div class="sht-cell" style="cursor:pointer"` +
		` hx-get="` + get + `" hx-target="this" hx-swap="innerHTML">` +
		string(actorCell(bound)) + `</div>`)
}

// avatarHTML renders just the round initials avatar for an actor id, with a
// deterministic colour. Use inline (role stacks, event log, compact cells).
func avatarHTML(actor string) template.HTML {
	if actor == "" {
		return template.HTML(`<span class="dx-av dx-av--empty" title="unassigned">?</span>`)
	}
	return template.HTML(`<span class="dx-av" style="background:` + actorColor(actor) +
		`" title="` + template.HTMLEscapeString(actor) + `">` +
		template.HTMLEscapeString(actorInitials(actor)) + `</span>`)
}

// actorInitials derives up to two uppercase initials from an actor id. Splits
// on common separators ("e.jackson", "e_jackson", "e-jackson"); otherwise
// takes the first two letters.
func actorInitials(actor string) string {
	var parts []string
	cur := []rune{}
	for _, r := range actor {
		if r == '.' || r == '_' || r == '-' || r == ' ' {
			if len(cur) > 0 {
				parts = append(parts, string(cur))
				cur = nil
			}
			continue
		}
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		parts = append(parts, string(cur))
	}
	up := func(s string) string {
		if s == "" {
			return ""
		}
		r := []rune(s)[0]
		if r >= 'a' && r <= 'z' {
			r -= 32
		}
		return string(r)
	}
	switch {
	case len(parts) >= 2:
		return up(parts[0]) + up(parts[len(parts)-1])
	case len(parts) == 1 && len([]rune(parts[0])) >= 2:
		rs := []rune(parts[0])
		s := string(rs[0]) + string(rs[1])
		return strings.ToUpper(s)
	case len(parts) == 1:
		return strings.ToUpper(parts[0])
	default:
		return "?"
	}
}

// avatarPalette is a small fixed set echoing the prototype's actor colours.
var avatarPalette = []string{"#1E3F60", "#5C1E50", "#C56A1A", "#2F7A3D", "#3F424D", "#1F4FB8", "#7A2F2F", "#2F6F7A"}

// actorColor picks a deterministic palette colour from the actor id.
func actorColor(actor string) string {
	var h uint32 = 2166136261
	for _, b := range []byte(actor) {
		h ^= uint32(b)
		h *= 16777619
	}
	return avatarPalette[int(h)%len(avatarPalette)]
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
