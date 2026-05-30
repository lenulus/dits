# Implementation Plan: Radical Execution on DITS — v2

**Status:** Draft / Plan (supersedes v1)
**Author:** generated for aglaforge@gmail.com
**Date:** 2026-05-29
**Tracks:** [`radical-execution-on-dits.md`](./radical-execution-on-dits.md) (the design)
        + the `DITS RE Prototype` design bundle from Claude Design (the UI)
**Replaces:** [`radical-execution-implementation-plan.md`](./radical-execution-implementation-plan.md)

---

## 1. Context

v1 mounted the RE web UI inside `dits-server` and routed OAuth, sessions,
custodial signing, and the scheduler through DITS. After re-reading, this
collapses two things that should be separate:

- **DITS is the substrate.** A client speaks to it via the CLI, the
  in-process Go library, or the bi-di MCP protocol. DITS owns the event
  DAG, signatures, sync, validation, materialised state, and the meta
  vocabulary. Nothing methodology-specific belongs here.
- **Pilot is a reference frontend.** A separate binary that hosts the
  web UI for one methodology (RE). It owns the human-facing surface —
  HTML, OAuth, sessions, custodial keys, the methodology scheduler, the
  cached indicator projections. Pilot talks to DITS through MCP.
  Future frontends (observability, builder console, etc.) sit alongside
  Pilot as siblings, all consuming the same MCP surface.

This v2 reflects that split. The substrate generalisations and the
generic events from v1 stay in DITS. Everything web/identity/scheduler
moves into Pilot. The plan also adds an explicit Phase for **building
out the MCP tool surface** as the load-bearing protocol between the two.

The three design deviations forced by the Claude Design prototype carry
forward unchanged:

| Deviation | Reason |
|---|---|
| ACK is a pair (`specifierAck` × `builderAck`) with derived alignment rollup. | Chat #2 — the user corrected the original ACK as a single state. |
| Goals are a third taxonomy (`goal → node → result`), not work-item hierarchy. | Chat #2 — same shape as Org and Product. |
| "For you" Attention is the default landing, not Leadership Portfolio. | Chat #3 — the user wanted an aggregated need-attention surface. |

---

## 2. Runtime topology

The picture this plan is built around:

```
  ┌──────────────────────────────────────────────────────────────────────┐
  │  laptop / server (Pilot host)                                        │
  │                                                                      │
  │   browser ──HTTPS──▶  cmd/pilot  :7070                               │
  │       │               ├─ HTML/HTMX views                             │
  │       │               ├─ OAuth + session cookies                     │
  │       │               ├─ Custodial Ed25519 keys (KMS-wrapped)        │
  │       │               ├─ RE scheduler (goroutine)                    │
  │       │               ├─ Indicator projection cache                  │
  │       │               └─ MCP client ◀──────────┐                     │
  │       │                                        │                     │
  │       │                                        │ MCP                 │
  │       │                                        ▼                     │
  │   Google / GitHub / OIDC                  cmd/dits-mcp  (stdio/HTTP) │
  │                                                ▲                     │
  └────────────────────────────────────────────────┼─────────────────────┘
                                                   │ in-process (workops)
                                                   ▼
                              ┌────────────────────────────────────────┐
                              │  DITS substrate                        │
                              │   ├─ event DAG (SQLite, file per proj) │
                              │   ├─ meta (taxonomies, roles, etc.)    │
                              │   ├─ reducer + materialised state      │
                              │   ├─ schema validation                 │
                              │   └─ workops mutation funnel           │
                              └────────────────────────────────────────┘
                                                   ▲
                                                   │ /api/v1/sync
                                                   ▼
                                            other DITS instances
                                            (peer-to-peer event sync)

  Other frontends (Claude Code, future observability dashboard, TUI)
  sit beside Pilot, each talking to dits-mcp the same way.
```

Process inventory:

| Process | Owner | Responsibility | Holds |
|---|---|---|---|
| `dits` (CLI) | DITS | Human/script frontend; self-sovereign keys. | `~/.dits/identity.pem` |
| `dits-server` | DITS | Peer-to-peer event sync; read-only HTTP query API. **No app UI.** | Project SQLite |
| `dits-mcp` | DITS | The bi-di interface every non-CLI frontend speaks. | Project SQLite (in-proc) |
| `cmd/pilot` | Pilot | Web UI, OAuth, sessions, custodial signing, RE scheduler, indicator cache. | Its own SQLite (users, sessions, wrapped keys, cache) |
| `cmd/dits-tui` (future, Phase 5 of design — not in this plan) | DITS or Pilot | TUI for the Pilot role's daily loop. | None special |

**No process change inside DITS** beyond the new MCP tools. CLI and MCP
both still use `workops` in-process. The web UI is a new process entirely,
in its own binary, talking MCP.

---

## 3. Scope

**In scope:**
1. The four generic substrate primitives in DITS.
2. The generalised ACK lifecycle events in DITS (so any methodology
   that wants a "commitment + acceptance" flow can use them).
3. Building out the MCP tool surface to cover every operation Pilot
   needs (audit + gap fill).
4. Pilot the binary: scaffold, OAuth, sessions, custodial signing,
   MCP client.
5. Pilot the UI: the prototype's twelve views, sheet primitive, slide-over
   panel, ⌘K palette.
6. Pilot's RE scheduler and indicator projections.
7. The RE default meta bundle, applied during Pilot first-run setup.

**Out of scope:**
- Adding web routes, OAuth, sessions, or schedulers to `dits-server`.
  DITS stays sync-only.
- A TUI. Independent ship.
- Notifications (email/Slack). In-app "For you" only.
- A second methodology pack or a second frontend. The architecture
  supports them; building one is the proof, not a v1 deliverable.
- A separate Go module for Pilot. Same repo, same go.mod for now;
  promote later if useful.

---

## 4. Phase 0 — MCP surface audit

**~1 week. No code changes; deliverable is a gap report.**

Pilot's blast radius is bounded by what MCP can do. Today's MCP tool
surface (`internal/mcp/tools.go`) covers most of `workops` but probably
not every read shape Pilot needs (filtered list queries, projection
endpoints, diagnostic queries, meta admin).

Deliverable: a checklist of every operation Pilot performs (cross-
referenced against the prototype's `App.jsx` actions and `views.jsx`
queries) against the current MCP surface. Output is a table:

| Pilot operation | Today's MCP tool | Gap |
|---|---|---|
| Create milestone | `work_create` | ✓ |
| List milestones with filters (status, role, etc.) | `work_list` | needs filter args |
| Bind role to actor | (none) | new tool — depends on Phase 1 |
| Get diagnostics for work item | (none) | new tool — depends on Phase 1 |
| File ACK | (none) | new tool — depends on Phase 2 |
| … | | |

This drives Phase 3.

---

## 5. Phase 1 — DITS substrate extensions

**~3–4 weeks. Same as v1 §4, repeated tersely for completeness.**

Add to DITS, methodology-agnostic. The change set verbatim from v1:

- `internal/domain/meta.go` — add `Taxonomies`, `Roles`, `RoleConstraints`
  fields to `MetaConfig`, plus helpers (`HasTaxonomy`, `GetRole`, etc.).
- `internal/domain/event.go` — add four event types: `work.classified`,
  `work.declassified`, `work.role_bound`, `work.role_unbound`.
- `internal/domain/validate.go` — tier-1 schema validation for the new
  events (taxonomy/node exists, role exists).
- `internal/domain/reduce.go` — add `Classifications` and `RoleBindings`
  to `WorkItem`; new reducer cases.
- `internal/constraints/` — NEW package; evaluator + four predicates:
  `distinct_actors`, `not_reports_to_within`, `classified_in_same_node`,
  `requires_classification`. Diagnostics, not gates.
- `internal/workops/workops.go` — new methods: `Classify`, `Declassify`,
  `BindRole`, `UnbindRole`.

Test discipline mirrors `internal/workops/workops_test.go`. A
`TestRoleConstraint_PilotIndependence` golden seeds the prototype's
PROJ-176 collapse scenario and pins diagnostic output.

Phase 1 is shippable on its own — any future frontend or methodology
pack uses these primitives.

---

## 6. Phase 2 — DITS ACK lifecycle events

**~1–2 weeks.**

v1 put these in an `internal/repack/` package inside DITS. v2 puts them
in **core DITS** instead. Reasons:

1. DITS must know about the events to validate and reduce them. Hiding
   them behind a pack would force a plugin-loading mechanism that
   doesn't exist.
2. The ACK shape (`filed → accepted/rejected/cleared → amended`) is not
   particularly RE-specific. It is a generic "two-party commitment
   lifecycle" pattern. Other future methodologies can use it.
3. Keeping ACK in core simplifies the protocol: MCP exposes one set of
   ACK tools, all frontends use the same shape.

The events (final list, ACK-pair shape per the prototype):

| Event | Payload | Notes |
|---|---|---|
| `work.ack_filed` | `{AckID, ScopeSummary, DeliveryTiming, TargetOutcome, AcceptanceCriteria}` | Specifier files the proposal. |
| `work.ack_accepted` | `{Who: specifier\|builder, Note?}` | One side accepts. |
| `work.ack_rejected` | `{Who, Note}` | One side rejects with reason. |
| `work.ack_cleared` | `{Who, Reason}` | Auto-emitted on material amendment; resets one or both sides to pending. |
| `work.ack_amended` | `{AmendmentType, Fields, Reason}` | `scope_change \| timeline_change \| target_change \| clarification` |

Reducer rules (in DITS core):

- After either side has `accepted`, a `scope_change` / `timeline_change`
  / `target_change` amendment auto-emits `ack_cleared` for both sides.
- A `clarification` amendment does not clear.
- A `target_change` additionally emits `work.review_requested` (the
  reviewer target is left to meta config — Pilot's RE meta routes it
  to the Leadership role).

The `AckRollup` function (pure, deterministic) lives next to the
reducer:

```go
type Ack int
const (AckPending Ack = iota; AckAccepted; AckRejected)
type Rollup int
const (Aligned Rollup = iota; BuilderPending; SpecifierPending; BothPending; Rejected)

func AckRollup(s, b Ack) Rollup { /* per prototype views.jsx:61–68 */ }
```

This is the only ACK-related logic in DITS core. The methodology
decisions ("which work kinds support ACK at all", "what counts as a
material amendment for *this* methodology") live in meta and in
Pilot's understanding.

Tests in `internal/domain/`:
- `TestAckLifecycle_BothAccept` — file → builder-accept → specifier-accept → aligned.
- `TestAckLifecycle_ScopeAmendmentClearsBoth` — amend after both accept → both pending.
- `TestAckLifecycle_ClarificationDoesNotClear`.

---

## 7. Phase 3 — DITS MCP surface buildout

**~2–3 weeks. Driven by Phase 0's gap report.**

Add MCP tools (`internal/mcp/tools.go`) covering everything Pilot does.
Expected new tool families:

| Family | Examples |
|---|---|
| **Filtered queries** | `work_list(filters, group_by, limit, offset)`, `work_get(id)`, `events_list(target?, type?, since?)` |
| **Diagnostics & projections** | `diagnostics_get(work_id)`, `indicators_get(window?)` |
| **Classification** | `work_classify`, `work_declassify` |
| **Role bindings** | `work_role_bind`, `work_role_unbind`, `role_bindings_list(work_id?)` |
| **ACK lifecycle** | `ack_file`, `ack_accept`, `ack_reject`, `ack_amend` |
| **Meta admin** | `meta_get`, `meta_apply(json)`, `taxonomy_node_add/move/retire` |
| **Identity** | `actor_register(public_key)`, `actor_list` |

Each tool follows the existing MCP convention (input schema, output
shape, error semantics). All ultimately funnel through `workops` or
the new query API.

The MCP conformance test (`internal/mcp/conformance_test.go`) grows to
cover the new families. A Pilot integration test (in Pilot's tree)
exercises the full surface end-to-end.

**Pilot's "client interface" is exactly this MCP surface, nothing more.**
If Pilot needs an operation, it's a tool. If it isn't a tool yet, Pilot
either does without or DITS gets a new tool. This is the architectural
constraint that keeps DITS substrate-clean.

---

## 8. Phase 4 — Pilot scaffolding (binary, identity, MCP client)

**~3–4 weeks.**

### 8.1 Binary layout

```
cmd/pilot/
  main.go              # flag parsing, MCP client setup, HTTP server start

internal/pilot/        # not imported by anything else — Pilot's internal code
  mcp/                 # MCP client wrapper (typed surface over the raw tools)
  auth/                # OAuth (OIDC) provider + session cookie model
  signing/             # custodial Ed25519 keys, KMS or local-master-key wrap
  scheduler/           # RE scheduler (Phase 6)
  projections/         # indicator cache (Phase 6)
  store/               # Pilot's own SQLite (users, sessions, wrapped keys, cache)
  web/
    handlers/          # one handler per route
    templates/         # html/template files
    static/            # CSS, JS, fonts (lifted from prototype)
```

`internal/pilot/` is under DITS's `internal/` so it can't be imported
from outside this repo, even though it's a separate binary. If Pilot
graduates to its own repo or module later, this directory moves
wholesale. Use of DITS's internal packages is forbidden — Pilot talks
to DITS *only* through MCP. (Enforced by a lint check on `cmd/pilot/`
and `internal/pilot/` imports.)

### 8.2 Identity bridge

Per design §7.2, but moved out of DITS:

- **OAuth (OIDC)** — generic client, configurable per provider (Google,
  GitHub, generic OIDC). Provider config via Pilot env/flags.
- **Custodial Ed25519 keypairs** — generated on first sign-in, stored in
  Pilot's SQLite, **wrapped** by either a local master key (single-
  binary deploys) or an external KMS (`Wrap/Unwrap` interface). Pilot
  uploads the public key to DITS via `actor_register` MCP tool.
- **Server-side signing path** — on every mutation, Pilot unwraps the
  user's key, signs the event using the **same** `crypto.CanonicalEventJSON`
  + `SignEvent` Pilot got from the DITS Go module (or reimplements
  in the Pilot tree — same algorithm, same canonical JSON). Submits the
  signed event via MCP. DITS verifies the signature exactly as it does
  for CLI-emitted events.

**Critical**: DITS does not learn that the event came from Pilot vs
the CLI. The signature is the only thing on the wire that matters for
attribution. This preserves DITS's invariants.

### 8.3 Self-sovereign opt-out

A user who installs the CLI can upload their CLI public key during
Pilot's account setup; Pilot then forgets its custodial key for them
and routes web mutations as "please sign this event payload" requests
to a local agent on the user's machine. v1 doesn't ship this; the door
just isn't closed.

### 8.4 Tests

- `TestCustodialRoundtrip` — sign via Pilot path, submit via MCP, verify
  via DITS's existing path; bytes match.
- `TestSessionScope` — session signs only as its actor.
- `TestKeyRotation`.
- `TestForbiddenImports` — lint that `internal/pilot/` never imports
  `internal/domain`, `internal/workops`, etc.

---

## 9. Phase 5 — Pilot UI (the prototype)

**~4–6 weeks.**

Server-rendered HTML routes. Match the Claude Design prototype
pixel-by-pixel where practical.

### 9.1 Stack

- chi sub-router under `/` (Pilot is the only app on its port; no `/web` prefix needed).
- Go `html/template` + a small partial system.
- HTMX for cell-level edits and slide-over swaps.
- Vanilla JS for keyboard handlers (cell nav, ⌘K, ⌘+Enter).
- CSS: lift the prototype's `ds/colors_and_type.css`, `ds/shared.css`,
  `app/app.css` near-verbatim into `internal/pilot/web/static/`.
- No frontend build pipeline. Assets served via `http.FileServer`.

### 9.2 Views (twelve routes, four sidebar sections)

**Workspace**
- `GET /for-you` — **default landing**; the Attention view.
- `GET /leadership` — portfolio rollup, indicators, pattern blocks.
- `GET /portfolio` — the spreadsheet.
- `GET /ack` — bulk-ack workspace (S-ACK / B-ACK pivots).
- `GET /rfcs` — RFC review queue.
- `GET /log` — Pilot's Log (kept; collapse-into-Updates refactor deferred per chat #6–7).

**Methodology**
- `GET /decisions` — DecisionBlocks.
- `GET /outcomes` — OutcomeAssessments.

**Substrate**
- `GET /roles` — role bindings with diagnostics.
- `GET /taxonomies` — Org / Product / Goals tabs.
- `GET /events` — event log (read-only).

**External**
- `GET /roadmap` — public, unauthenticated, filtered projection.

### 9.3 Sheet primitive

Port the prototype's `Sheet` to a Go template + HTMX/JS island.
Non-negotiable details from the prototype's iteration:

- Frozen ID + Title columns (left-sticky, opaque background on scroll).
- `table-layout: fixed` + pinned table width = sum of `<col width>`
  (the bug fix the prototype debugged at length — preserve it).
- Inline editing: select / actor picker / taxonomy picker / target editor.
- Keyboard: arrows, Tab, Enter (edit), ⌘+Enter (open panel), Space
  (select row), Esc (cancel/close).
- Multi-select via gutter, shift-click for range, bulk action bar.
- Filter chips (`+ Add filter`) — twelve field types per
  `App.jsx:465–505`.
- Group by: None / Status / Team / Product / Quarter.
- Column groups with collapsible bands + JTBD presets. Order:
  Identity → Classification → Health → ACKs → Roles → Delivery →
  Updates → Signals. Default ("Overview") preset collapses everything
  except Identity + Updates.

### 9.4 Slide-over detail panel

640px right-side panel, ends 36px above viewport (the hintbar lives below).
Tabs (order is post-iteration final):

**ACK · Status · Stages · Deps · Updates · Receipts**

ACK tab layout, top to bottom:
1. Meta line (status, alignment, RYG, freshness).
2. Diagnostics (one-line compact stack).
3. Roles & classification (Specifier · Builder · Pilot · Org · Product).
4. **The commitment** (Delivery target, Scope, Target outcome, Acceptance).
5. Stages preview (link to Stages tab for full editing).
6. **The ACKs** — Specifier and Builder stand-behind cards, A/P/R buttons inline.
7. ACK history table.

Status tab: **Next steps before Risks**. RYG segmented control at top.
"Post update" composer with ⌘+↵. Red/Yellow with empty risks shows a
`required for Red/Yellow` chip on the section header.

InlineEditableField affordance: dashed border + edit-pencil on hover.

### 9.5 "For you" Attention view (default landing)

Sections (per prototype):
- ACKs needed (your S-ACK or B-ACK pending)
- Targets passed or imminent (≤21d)
- Stale status updates (>14d silent on in-flight work)
- High-severity risks on your work
- Diagnostics on your work
- Decisions idle ≥5d on your work
- Outcome assessments overdue
- Rejected ACKs in your view
- Approved RFCs with nothing spawned (≥30d) — RFC lineage signal

Each row: severity dot, reason chip, ID, title, italic context, inline
actions, deep-links to the right tab on the right sheet.

### 9.6 Public roadmap

`GET /roadmap` is **unauthenticated**. Filtered projection: milestones
with `customer_visible=true` AND `status ∈ {ack_committed, in_flight,
shipped}`. Whitelisted fields only — scope summary, delivery timing,
goal-chain names. No RYG, no indicators, no commentary.

Pilot computes this from its indicator cache + a single MCP query.
No new DITS table.

### 9.7 Role bindings are not access control

A signed-in actor may emit any event on any work item; the substrate
does not gate. The UI greys out buttons for roles the actor doesn't
hold on a given milestone, but events still flow. Off-Pilot RYG edits
are recorded with attribution and surfaced in the activity feed.

---

## 10. Phase 6 — Pilot scheduler, projections, and the RE meta bundle

**~2–3 weeks.**

### 10.1 RE meta bundle

`internal/pilot/meta/re_default.json` — the RE methodology as data:

- Work kinds: `rfc`, `milestone`, `decision_block`, `outcome_assessment`.
- Workflows for each (states per the proposal §6.4).
- Roles: `specifier`, `builder`, `pilot`, `reviewer`, `leadership`.
- Role constraints: `no_role_collapse`, `pilot_independence`,
  `requires_product_classification`.
- Empty `org`, `product`, `goals` taxonomy skeletons (consumer fills).

Applied via Pilot's first-run flow: `pilot init --project=<path>`
calls MCP `meta_apply(json)` to load the bundle into the target DITS
project's meta. Idempotent — re-running diffs and applies only changes.

### 10.2 Scheduler (goroutine inside Pilot)

Runs every N seconds (default 60). Per cycle:

- Poll DecisionBlocks via MCP; for each open ≥5d, emit
  `work.review_requested` (auto-escalation).
- Poll milestones; for each newly `shipped`, ensure a paired
  `outcome_assessment` work item exists; create if missing with a 90d SLA.
- Poll OutcomeAssessments; for each past SLA, set `status=overdue` via
  the MCP `work_set_status` tool.

All actions emit normal signed events via MCP. The scheduler signs as
a `scheduler.pilot` actor (registered at Pilot first-run, custodial key
held by Pilot itself). Attribution is honest — these events came from
Pilot's scheduler, not a human or LLM agent.

If Pilot is down, the scheduler doesn't fire. v1 accepts this. If you
need scheduling without the UI process running, splitting `cmd/pilot-scheduler`
out is a small change later — same MCP client, no UI.

### 10.3 Indicator projections

Four pure functions over the event log
(`internal/pilot/projections/`):

| Indicator | Source |
|---|---|
| Dependency closure rate | `depends_on` edges on shipped milestones in window. |
| Scope-change velocity | `work.ack_amended` with `amendment_type=scope_change` per milestone per week. |
| Decision friction | Median RFC `created → approved`; median DecisionBlock `open → resolved/escalated`. |
| ACK-to-start latency | Median `aligned → first work.execution_started`. |

Pilot queries DITS via MCP, computes the indicators, caches in memory
(rebuilt on Pilot restart). On every event Pilot writes, the affected
indicators are invalidated. Reads serve from cache.

Alternatively, Pilot can ask DITS for projection data via a new MCP
tool `indicators_get` (added in Phase 3). Decision: compute in Pilot
for v1 — keeps DITS substrate-clean. Promote to a DITS-side projection
table only if performance demands.

---

## 11. Phase 7 — Future frontends (not in this plan)

Mentioned only to validate the architecture. Future frontends sit beside
Pilot, each as `cmd/<name>` (or eventually `frontends/<name>`):

- **Observability dashboard** — read-only, indicator-focused, charts
  rather than sheets. Same MCP client, different views.
- **Builder console** — Builder-role-focused, prioritises the "needs my
  B-ACK + active deliveries" surface. Subset of Pilot's views, repackaged.
- **TUI** — design §Phase 5. Pilot-role focused daily loop. Same MCP
  client, terminal UI.

If MCP can serve all three without backend changes, the architecture is
proved out. If any of them forces new MCP tools, those tools should be
generic enough that the others benefit. This is the same generalisation
discipline applied to substrate primitives in Phase 1.

---

## 12. Files to add / change (summary)

| Layer | Path | Action |
|---|---|---|
| DITS substrate | `internal/domain/meta.go` | Add Taxonomy, Role, RoleConstraint types + helpers. |
| DITS substrate | `internal/domain/event.go` | Add 4 generic events + 5 ACK lifecycle events. |
| DITS substrate | `internal/domain/validate.go` | Tier-1 validation for new events. |
| DITS substrate | `internal/domain/reduce.go` | Reducer cases for classification, role bindings, ACK lifecycle. |
| DITS substrate | `internal/domain/workitem.go` | Add `Classifications`, `RoleBindings`, `Acks`, `Diagnostics`. |
| DITS substrate | `internal/domain/ack.go` | NEW: `AckRollup`, ACK enums. |
| DITS substrate | `internal/constraints/` | NEW pkg: evaluator + four predicates. |
| DITS substrate | `internal/workops/workops.go` | New methods: `Classify`, `Declassify`, `BindRole`, `UnbindRole`, `AckFile`, `AckAccept`, `AckReject`, `AckAmend`. |
| DITS MCP | `internal/mcp/tools.go` | New tool families per Phase 3. |
| DITS MCP | `internal/mcp/conformance_test.go` | Cover new tools. |
| Pilot | `cmd/pilot/main.go` | NEW: binary entry, MCP client setup, HTTP server. |
| Pilot | `internal/pilot/mcp/` | NEW: typed wrapper over MCP tools. |
| Pilot | `internal/pilot/auth/` | NEW: OAuth/OIDC + session cookies. |
| Pilot | `internal/pilot/signing/` | NEW: custodial keys, KMS interface. |
| Pilot | `internal/pilot/store/` | NEW: Pilot's SQLite (users, sessions, wrapped keys, cache). |
| Pilot | `internal/pilot/scheduler/` | NEW: RE scheduler goroutine. |
| Pilot | `internal/pilot/projections/` | NEW: indicator computation + cache. |
| Pilot | `internal/pilot/web/handlers/` | NEW: one handler per route. |
| Pilot | `internal/pilot/web/templates/` | NEW: Go html/template files for 12 views. |
| Pilot | `internal/pilot/web/static/` | NEW: lifted CSS/JS/fonts from prototype. |
| Pilot | `internal/pilot/meta/re_default.json` | NEW: RE methodology meta bundle. |
| DITS docs | `docs/proposals/` | This file (v2 plan). |

No file is removed. No change to `dits-server`. CLI and MCP are extended,
not restructured.

---

## 13. Verification

Each phase has its own tests; the final acceptance is the design's §13
success criteria, repeated here in executable form:

1. **DITS substrate conformance** — `go test ./internal/...` green.
   `TestSubstrateConformance_v2` proves every event submitted via MCP
   is indistinguishable from a CLI-emitted event at the protocol layer.
2. **MCP conformance** — every Pilot operation has a corresponding tool;
   conformance test covers all families.
3. **End-to-end demo** — `dits init`, `dits-mcp serve`, `pilot init &&
   pilot serve`; sign in via OAuth stub, run RFC → leadership approve →
   spawn milestone → both ACKs → in-flight → ship → outcome assessment.
   Target: under 15 minutes.
4. **Amendment auto-clears ACKs** — emit `scope_change` amendment on a
   milestone with both ACKs `accepted`; both go to `pending`; Pilot's
   indicator panel updates within one poll cycle.
5. **Separability diagnostic** — seed PROJ-176's collapse scenario; the
   constraint engine emits expected diagnostics; Pilot UI surfaces them
   on the milestone page header.
6. **Roadmap is filtered** — `curl pilot:7070/roadmap` (no auth)
   returns whitelisted fields only; no RYG / diagnostics / commentary.
7. **Off-Pilot edit recorded** — sign in to Pilot as a non-Pilot,
   edit a milestone's RYG; assert event written via MCP and surfaced
   in the activity feed visible to the bound Pilot.
8. **Performance** — seed 1500-person, ~800-milestone corpus; Pilot
   landing renders in <500ms server-side after the first cache prime.
9. **Pilot can be replaced** — stop Pilot, replay the same demo via
   CLI calls + manual MCP invocations. The substrate state is identical.
   Confirms DITS has no Pilot-shaped coupling.
10. **DITS without Pilot** — fresh `dits init` without applying RE meta;
    substrate works as a generic event-sourced coordination tool. CLI
    and MCP are fully functional. Phase 1 primitives are available
    to any other consumer.

---

## 14. Open questions

Carried from v1, the design, and the prototype chat:

1. **Custodial key revocation in Pilot.** OAuth-revoked users: retire
   the Pilot user row, keep the DITS actor row with prior signatures
   verifiable. UX implication TBD.
2. **Meta sync churn from taxonomy nodes.** Defer; model nodes as work
   items only if real-world churn warrants. [Design §12.2]
3. **Predicate language extensibility.** Hard-code four in Go for v1.
   Revisit when a second pack appears. [Design §12.3]
4. **Pilot's Log refactor** into per-milestone Updates stream. Back-
   burnered per chat #6–7. Ship as-is; revisit after living with it.
5. **Pilot ↔ DITS deployment colocation.** v1 assumes Pilot runs next
   to `dits-mcp` (stdio or local HTTP). If they're separated across
   machines, MCP transport choice matters. Decide later.
6. **Indicator cache invalidation across multiple Pilot processes.**
   v1 assumes one Pilot per project. Multi-process Pilot needs either
   cache-in-DITS or a pub/sub model. Out of scope until needed.
7. **RFC ↔ milestone lineage** — Option A (cross-link via `fromRfc`)
   for v1, per chat #11. Option B (unify into tree-indented Portfolio)
   is the cleaner end state — revisit after living with A.

---

## 15. Deltas from v1

For reviewers comparing to v1:

| Topic | v1 | v2 |
|---|---|---|
| Web UI host | Mounted on `dits-server` under `/web/*`. | Separate binary `cmd/pilot`. |
| OAuth + sessions | Inside DITS (`internal/auth/`). | Inside Pilot. |
| Custodial signing | DITS holds wrapped keys. | Pilot holds wrapped keys. DITS trust model unchanged. |
| RE scheduler | Goroutine inside `dits-server`. | Goroutine inside Pilot. |
| RE pack location | `internal/repack/` inside DITS, mixing events + scheduler + meta. | Split: ACK events in DITS core; meta + scheduler + projections in Pilot. |
| Indicator projections | Cache in `dits-server`. | Cache in Pilot. (Or DITS-side later via `indicators_get` MCP tool if needed.) |
| Bi-di protocol to substrate | Implicit — web mounted on DITS. | Explicit — Pilot speaks MCP. |
| MCP surface scope | Existing tools assumed sufficient. | Phase 3 audit + buildout makes MCP the canonical app interface. |
| Future frontends | Implicit. | Explicit: same MCP surface, same pattern as Pilot. |
| Custodial-signing trust shift | Absorbed into DITS. | Pushed out to Pilot. DITS unchanged. |
| `dits-server` responsibilities | Sync + query + web + auth + scheduler. | Sync + query. |

---

## 16. Phasing summary

| Phase | What | Weeks | Where | Independently shippable? |
|---|---|---|---|---|
| 0 | MCP surface audit | 1 | DITS | n/a (gap report) |
| 1 | Substrate primitives (taxonomies, roles, constraints, 4 generic events) | 3–4 | DITS | Yes |
| 2 | ACK lifecycle events in DITS core | 1–2 | DITS | Yes |
| 3 | MCP surface buildout per Phase 0 report | 2–3 | DITS | Yes — improves CLI/Claude integration immediately |
| 4 | Pilot scaffold (binary, OAuth, sessions, custodial signing, MCP client) | 3–4 | Pilot | Skeleton ships before UI |
| 5 | Pilot UI (12 views, sheet, panel, palette) | 4–6 | Pilot | Demo path lights up |
| 6 | Pilot scheduler + projections + RE meta bundle | 2–3 | Pilot | Completes RE |

**Total: ~16–23 engineer-weeks.** Front-loaded so Phases 0–3 are pure
DITS substrate work, usable by any frontend (including Claude Code on
day one). Phases 4–6 build Pilot on top. The demo path needs Phases
0–5 landed; Phase 6 makes it self-driving.

---

## 17. First concrete artifacts

After this v2 is accepted in spirit:

1. **The Phase 0 MCP surface gap report.** One table, ~50 rows.
   Drives Phase 3 scope.
2. **A precise spec of the four constraint predicates** (input/output
   shape, evaluation semantics). Feeds Phase 1.
3. **A Pilot first-run UX sketch** — OAuth → project pairing →
   actor registration → RE meta apply. Feeds Phase 4 + 6.

Everything else follows.
