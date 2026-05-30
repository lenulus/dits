# Implementation Plan: Radical Execution on DITS

**Status:** Draft / Plan
**Author:** generated for aglaforge@gmail.com
**Date:** 2026-05-29
**Tracks:** [`radical-execution-on-dits.md`](./radical-execution-on-dits.md) (the design)
        + the `DITS RE Prototype` design bundle from Claude Design (the UI)

---

## 1. Context

The companion proposal frames Radical Execution (RE) as a **methodology
overlay** on the DITS substrate. It defines four substrate primitives
(taxonomies, classifications, role bindings, role-constraint diagnostics),
an RE-specific work-kind/event pack, an OAuth-bridged custodial identity
layer for browser users, and a server-rendered web UI projecting both.

In parallel, the user prototyped the UI in `claude.ai/design`. The
prototype is a **spreadsheet-first** projection with twelve views, ⌘K
palette, slide-over detail panel, column groups + JTBD presets, and
inline editing as the dominant interaction. It also forced two
substantive changes to the proposal that this plan adopts:

| Change | Where it came from |
|---|---|
| **ACK is a pair** (`specifierAck` × `builderAck`, each ∈ `{accepted, pending, rejected}`), with a computed **alignment** rollup ∈ `{aligned, builder_pending, specifier_pending, both_pending, rejected}`. | Chat #2: "It's actually Specifier ACK and Builder ACK; the ACK is simple Accepted/Rejected/Pending." |
| **Goals are a third taxonomy** (`goals`: `goal → node → result`), not a hierarchy of work items. | Chat #2: "Under Taxonomies I'd also add Goals with the same structure." |
| **"For you" Attention is the default landing**, not Leadership Portfolio. | Chat #3: introduced after the user asked for an aggregated need-attention surface. |
| **Pilot's Log de-emphasised** in favour of a per-milestone Updates stream. | Chat #6–7: the user agreed there is no separate PilotLog entity — Pilot is *who* not *what*. |

This plan turns the proposal's five phases into an actionable build
sheet, with the prototype's UI as a concrete target. Nothing in here
contradicts the proposal; it specialises the underdetermined parts and
makes the UI work shippable.

---

## 2. Scope

**In scope (this plan covers):**

1. The four generic substrate primitives (Phase 1 of the design).
2. OAuth + custodial signing for browser actors (Phase 2).
3. The RE pack — work kinds, workflows, ACK-pair events, scheduler (Phase 3).
4. The server-rendered web UI matching the prototype (Phase 4).
5. Test discipline equal to current `internal/workops` standards.

**Out of scope (defer or skip):**

- The Pilot's Log top-level view as a kitchen-sink feed. The prototype
  keeps it for v1 but the user has explicitly back-burnered redesigning
  it into a per-milestone Updates stream; this plan ships it as-is and
  flags the refactor as a follow-on.
- The TUI (Phase 5 of the design). Independent ship, not blocking RE adoption.
- Notification delivery (email/Slack). In-app inbox via "For you" only.
- Real-time streaming. Polling against the v2 query API is sufficient.
- A separate SPA. HTML routes added to `dits-server` per design §8.2-A.
- A second methodology pack. The substrate is built generic; a second
  consumer is the proof, not a v1 deliverable.

---

## 3. Substrate gap report (Phase 0)

The Phase 0 audit confirms the substrate is ready. Notable findings:

- **Event substrate:** clean three-tier validation
  (`internal/domain/validate.go` → `protocol.go` → `reduce.go`). New
  event types are an additive triple — constant in `event.go`, payload
  struct, case in `reduce.go`. No retrofits needed.
- **MetaConfig** (`internal/domain/meta.go:6–26`) is extensible; the
  three new fields (`Taxonomies`, `Roles`, `RoleConstraints`) slot in
  cleanly. Helper methods follow an established pattern (`HasLabel`,
  `OpenStatuses`, etc.) that the new fields should mirror.
- **WorkOps** (`internal/workops/workops.go:30–52`) is the single
  mutation funnel via `AppendAndMaterialize()`. The new RE-specific
  operators (ACK file/commit/amend, role bind/unbind, classify) belong
  here, not in CLI or server code.
- **Server v2 query API** (`internal/server/server.go:51–62`) already
  exposes `/api/v2/work/{id}` and per-projection endpoints. The web UI
  consumes these plus a new `/api/v2/projections/*` family.
- **No UI, no templates, no auth, no static assets** in tree today. The
  web is greenfield. CLI and MCP are peer frontends with no shared
  code with the future web UI beyond `workops` and the query API.
- **Tests** follow `internal/workops/workops_test.go` pattern
  (fixture + table-driven, hits the real DB in `t.TempDir()`). All
  Phase 1–3 work follows this discipline.

No code changes in Phase 0; the audit lands as this document.

---

## 4. Phase 1 — Generic substrate extensions (~3–4 weeks)

These extensions are **methodology-agnostic**. RE consumes them; a
future pack consumes them differently.

### 4.1 MetaConfig additions

Add three top-level fields to `internal/domain/meta.go` (matching design §6.1):

- `Taxonomies []Taxonomy` — named hierarchical classification namespaces.
- `Roles []Role` — declared roles with `slug`, `name`, `cardinality`.
- `RoleConstraints []RoleConstraint` — predicate-backed diagnostics.

Add helpers mirroring existing patterns: `HasTaxonomy(slug)`,
`GetTaxonomyNode(taxSlug, nodeSlug)`, `HasRole(slug)`, `GetRoleConstraint(slug)`.

Default meta (`DefaultMetaConfig()`) stays minimal — empty taxonomies/
roles/constraints. The RE pack (§6) ships its own seeded meta.

### 4.2 New event types

Add to `internal/domain/event.go`:

| Event | Payload |
|---|---|
| `work.classified` | `{TaxonomySlug, NodeSlug}` |
| `work.declassified` | `{TaxonomySlug, NodeSlug}` |
| `work.role_bound` | `{RoleSlug, ActorID, Scope?}` |
| `work.role_unbound` | `{RoleSlug, ActorID}` |

Each gets:

1. An `EventType` constant.
2. A payload struct in `event.go`.
3. Tier-1 schema validation in `validate.go` (taxonomy/node/role exists
   in meta, node not retired).
4. A reducer case in `reduce.go` that updates `WorkItem.Classifications`
   and `WorkItem.RoleBindings` (two new fields on the materialised state).
5. WorkOps methods: `Classify`, `Declassify`, `BindRole`, `UnbindRole`.

Schema validation only rejects malformed references. **Cardinality and
role-constraint violations are diagnostics, not errors** — matching both
the proposal's "attribution-not-prevention" rule and DITS's existing
"advisory, do not block during sync" stance.

### 4.3 Constraint predicate evaluator

New package `internal/constraints/`:

- `evaluator.go` — `Evaluate(item *WorkItem, meta *MetaConfig) []Diagnostic`
  iterates declared `RoleConstraints`, dispatches to the predicate registry,
  returns `{ConstraintSlug, Severity, Message}` for each finding.
- `predicates.go` — initial four predicates as Go functions, registered
  in a `map[string]Predicate`:
  - `distinct_actors`
  - `not_reports_to_within`
  - `classified_in_same_node`
  - `requires_classification`
- `evaluator_test.go` — table-driven cases per predicate, plus
  end-to-end "given this meta + item, expect these diagnostics."

Diagnostics are exposed via the v2 query API
(`GET /api/v2/work/{id}/diagnostics`) and surfaced in the UI; they never
gate event ingestion.

### 4.4 Tests and contract

- Mirror `internal/workops/workops_test.go`: a `TestClassifyLoop`
  exercises classify → declassify → re-classify, asserting materialised
  state and event payloads.
- A `TestRoleConstraint_PilotIndependence` golden seeds the org
  taxonomy and bindings used in the prototype (`PROJ-176` collapse and
  reports-to violations) and pins the diagnostic output.
- The MCP conformance test (`internal/mcp/conformance_test.go`) gains
  coverage for the four new tools.

Phase 1 is shippable independently. Any non-RE consumer can declare its
own taxonomies/roles immediately.

---

## 5. Phase 2 — Identity bridge (~2–3 weeks)

Per design §7.2, the web UI signs every mutation as the OAuth-mapped
actor, using a server-held Ed25519 key. CLI keys remain self-sovereign;
the two paths produce protocol-identical events.

### 5.1 OAuth (OIDC) provider

- New `internal/auth/oidc.go` — generic OIDC client, configurable per
  provider (Google, GitHub, generic). Provider config in
  `cmd/dits-server` flags or env.
- New `actors_oauth` table — maps `(provider, subject) → actor_id`.
- Session model: short-lived signed cookie carrying `actor_id`.
  No JWT; the server is the only consumer.

### 5.2 Custodial keypair storage

- New `internal/crypto/custodial.go` — generates Ed25519 keypairs,
  stores the private key **wrapped** by either:
  - a server-side master key loaded from disk (single-binary deploys), OR
  - an external KMS via a small interface (`kms.Wrap/Unwrap`).
- Public key registered through the existing `ActorStore.RegisterActor`
  path so every event keeps its existing verification flow.

### 5.3 Server-side signing path

`AppendAndMaterialize` already accepts a signing identity. The web
handlers thread `actor_id` from the session, unwrap the custodial key,
and call into the same code path. **No fork** in event handling
between CLI and web — verified by a round-trip test that creates the
same event via both routes and diff-checks the resulting DAG.

### 5.4 Self-sovereign opt-in

CLI users who want their own key continue to generate one with
`dits identity init`. Web users wanting to upgrade upload their
public key during account setup; the server stops generating a custodial
key for them and routes future mutations through the user's CLI.

### 5.5 Tests

- `TestCustodialRoundtrip` — sign via web path, verify via CLI path; bytes match.
- `TestSessionScope` — bound session signs only as its actor.
- `TestKeyRotation` — invariants under unwrap-rewrap.

---

## 6. Phase 3 — RE methodology pack (~3–4 weeks)

The pack is a Go package (`internal/repack/`) that registers:

1. Default meta (work kinds, workflows, roles, role constraints, taxonomy skeletons).
2. RE-specific event types (the ACK pair).
3. A reducer extension for ACK state.
4. The RE scheduler daemon.

The pack ships disabled by default; enable per project via
`dits init --pack=re` (or `meta.Packs += "re"` in MetaConfig).

### 6.1 Work kinds and workflows

Added via meta (no DITS-core defaults):

| Kind | Workflow states |
|---|---|
| `rfc` | `draft → leadership_review → approved → backlog → resourced → rejected` |
| `milestone` | `draft → ack_filed → ack_committed → in_flight → shipped → aborted` |
| `decision_block` | `open → resolved → escalated` |
| `outcome_assessment` | `pending → completed → overdue` |

Goals are **not** a work kind — they are a third taxonomy
(`goals: goal → node → result`), per the prototype.

### 6.2 ACK as a pair (revised from design §6.5)

Two ACK fields per milestone, each `accepted | pending | rejected`,
plus a derived `alignment` rollup. Events (RE pack, not DITS core):

| Event | Payload | Meaning |
|---|---|---|
| `work.ack_filed` | `{AckID, ScopeSummary, DeliveryTiming, TargetOutcome, AcceptanceCriteria}` | The Specifier commits a proposal. |
| `work.ack_accepted` | `{Who: specifier\|builder, Note?}` | One side accepts. |
| `work.ack_rejected` | `{Who, Note}` | One side rejects with reason. |
| `work.ack_cleared` | `{Who, Reason}` | Auto-emitted when a material amendment lands; resets that side to `pending`. |
| `work.ack_amended` | `{AmendmentType, Fields, Reason}` | `scope_change | timeline_change | target_change | clarification`. |

Reducer extension (in the pack):

- A `scope_change`, `timeline_change`, or `target_change` amendment
  emitted after either side has `accepted` automatically clears **both**
  ACKs back to `pending`.
- A `clarification` amendment does not clear.
- A `target_change` additionally emits `work.review_requested` to Leadership.

The pack's reducer wraps the core reducer; it does not modify DITS
core. The rollup function is pure and lives in `repack/ack.go`:

```go
func AckRollup(s, b Ack) Rollup {
    switch {
    case s == Rejected || b == Rejected: return Rejected
    case s == Accepted && b == Accepted: return Aligned
    case s == Pending  && b == Pending:  return BothPending
    case s == Pending:                   return SpecifierPending
    case b == Pending:                   return BuilderPending
    }
    return BothPending
}
```

### 6.3 PilotLog as event convention (no new event types)

Per design §6.6. The pack documents the `pilot_log_entry_type` payload
convention on existing events (`work.observation_recorded`,
`work.finding_recorded`, `work.eval_completed`, `work.decision_recorded`).
The Pilot's Log view queries with this filter.

The prototype's chat made clear this is likely to collapse into a
per-milestone Updates stream in a future iteration; for now we ship the
straightforward query and don't invest in a separate `pilot_log_entries`
materialisation.

### 6.4 DecisionBlock and OutcomeAssessment

Both are `work_item` rows of their respective `kind`. Linked via
`relates_to` (DecisionBlock) and `paired_with` (OutcomeAssessment).
No new event types — they use the existing lifecycle events.

### 6.5 Scheduler daemon

New `internal/repack/scheduler.go`, run as a goroutine in `dits-server`
(or invoked via `dits re scheduler --once` for testing). Triggers:

- DecisionBlock idle ≥ 5d → emit `work.review_requested` (auto-escalation).
- Milestone → `shipped` → create paired `outcome_assessment` work item (+ 90d SLA).
- OutcomeAssessment past SLA → set `status=overdue`.

All actions go through `workops.AppendAndMaterialize`; nothing escapes
the audit trail. Scheduler interval is configurable; default 5 min.

### 6.6 Default meta bundle

`repack/meta.go` exposes `DefaultMeta() *MetaConfig` containing:

- Four work kinds + workflows (above).
- Five roles: `specifier`, `builder`, `pilot`, `reviewer`, `leadership`.
- Three role constraints: `no_role_collapse`, `pilot_independence`,
  `requires_product_classification`.
- Empty `org`, `product`, `goals` taxonomy skeletons (consumer fills nodes).

### 6.7 Tests

- `TestAckLifecycle_Pair` — file → builder-accept → specifier-accept → aligned;
  amend (scope_change) → both cleared → re-accept → aligned again.
- `TestScopeAmendmentClearsBothAcks`.
- `TestSchedulerEscalation` — drive time forward, assert event emitted.
- `TestOutcomeAssessmentCreated` — milestone shipped → paired OA exists.
- MCP conformance grows to cover the new tools.

---

## 7. Phase 4 — Web UI (~4–6 weeks)

Server-rendered HTML routes added to `dits-server`. Single binary, no
separate build pipeline. Match the prototype pixel-by-pixel where
practical; lift the data model exactly.

### 7.1 Stack

- **Routing**: chi sub-router under `/web`. Default landing redirects to `/web/for-you`.
- **Templating**: Go `html/template` + a small partial system in `internal/web/templates/`.
- **Interactivity**: HTMX for cell-level edits and slide-over panel
  swaps. Vanilla JS for keyboard handlers (cell nav, ⌘K, ⌘↵).
- **CSS**: lift the prototype's three CSS files almost verbatim into
  `internal/web/static/ds/` and `internal/web/static/app/`. The
  prototype's design tokens (colors, type, spacing) are already a
  coherent system; no need to redesign.
- **No frontend toolchain.** Assets served directly via `http.FileServer`.

### 7.2 View inventory (matches prototype)

Twelve routes, four sidebar sections:

**Workspace**
- `GET /web/for-you` — **default landing**; the "Attention" view.
- `GET /web/leadership` — portfolio rollup, indicators, pattern blocks.
- `GET /web/portfolio` — the spreadsheet.
- `GET /web/ack` — bulk-ack workspace (S-ACK / B-ACK pivots).
- `GET /web/rfcs` — RFC review queue.
- `GET /web/log` — Pilot's Log (kept; refactor deferred).

**Methodology**
- `GET /web/decisions` — DecisionBlocks.
- `GET /web/outcomes` — OutcomeAssessments.

**Substrate**
- `GET /web/roles` — role bindings with diagnostics.
- `GET /web/taxonomies` — Org / Product / Goals tabs.
- `GET /web/events` — event log (read-only).

**External**
- `GET /web/roadmap` — public, unauthenticated, filtered projection.

### 7.3 Sheet primitive

The prototype's `Sheet` component is the load-bearing UX. Port it to a
single Go template + HTMX/JS island:

- Frozen ID + Title columns (left-sticky, opaque background on scroll).
- Click any cell → inline editor (select / actor picker / taxonomy picker / target editor).
- Keyboard nav: arrows, Tab, Enter (edit), ⌘+Enter (open panel), Space
  (select row), Esc (cancel/close).
- Multi-select via gutter; shift-click for range; bulk action bar.
- Quick-add row at sheet bottom.
- Filter chips (`+ Add filter`) with 12 field types (per `App.jsx:465–505`).
- Group by: None / Status / Team / Product / Quarter.
- Column groups with collapsible bands + JTBD presets:
  Identity → Classification → Health → ACKs → Roles → Delivery → Updates → Signals.
  Default preset ("Overview") collapses everything except Identity + Updates.

The fixed-layout / sticky-left math from the prototype's late-stage
debugging (`table-layout: fixed` + pinned table width = sum of
`<col width>`) must be preserved; the bug would re-emerge otherwise.

### 7.4 Slide-over detail panel

640px right-side panel, ends 36px above viewport (the hintbar lives below).
Tabs (the order matters — established after iteration in the prototype):

**ACK · Status · Stages · Deps · Updates · Receipts**

ACK tab layout (top → bottom):

1. Meta line (status, alignment, RYG, freshness).
2. Diagnostics (one-line compact stack).
3. **Roles & classification** (Specifier · Builder · Pilot · Org · Product).
4. **The commitment** (Delivery target, Scope, Target outcome, Acceptance).
5. Stages preview (link to Stages tab for full editing).
6. **The ACKs** — Specifier and Builder stand-behind cards, each with
   A/P/R buttons inline.
7. ACK history table.

InlineEditableField affordance: dashed border + edit-pencil on hover,
applied to all long-form fields (Scope / Outcome / Acceptance / Status narrative).

Status tab: **Next steps before Risks** (operational outcome of current
state comes first). RYG segmented control at top. "Post update" composer
with ⌘+↵ to file. When RYG is Red or Yellow with empty risks, the
section header carries a `required for Red/Yellow` chip.

### 7.5 "For you" Attention view (default landing)

The aggregator. Sections:

- ACKs needed (your S-ACK or B-ACK pending)
- Targets passed or imminent (≤21d)
- Stale status updates (>14d silent on in-flight work)
- High-severity risks on your work
- Diagnostics on your work
- Decisions idle ≥5d on your work
- Outcome assessments overdue
- Rejected ACKs in your view
- Approved RFCs with nothing spawned (≥30d) — RFC lineage signal

Each row: severity dot, reason chip, ID, title, italic context,
inline actions (Accept / Reject / Update / Resolve / Escalate / Open).
Clicking a row deep-links to the relevant sheet + opens the right tab.

### 7.6 Public roadmap

`GET /web/roadmap` is **unauthenticated**, filtered projection of:
- Milestones with `customer_visible=true` AND `status ∈ {ack_committed, in_flight, shipped}`.
- Each carries: latest committed ACK's `scope_summary`, `delivery_timing`, goal-chain names.

No RYG, no indicators, no commentary, no diagnostics, no internal fields.
Pure projection — implemented as a single query handler with whitelist
field selection. No new table.

### 7.7 Identity model

CLI: existing self-sovereign key path.
Web: OAuth → custodial signing (Phase 2). Sessions carry `actor_id`.
**Role bindings are not access control.** A user may emit events on
any work item; the UI greys out buttons for roles they don't hold, but
the substrate does not enforce. Off-Pilot RYG edits are recorded with
attribution, surfaced in the activity feed.

### 7.8 Indicator projections

Four pure functions over the event log
(`internal/repack/projections.go`):

| Indicator | Definition |
|---|---|
| Dependency closure rate | `depends_on` edges on shipped milestones in window, fraction closed before downstream ship. |
| Scope-change velocity | `work.ack_amended` with `amendment_type=scope_change` per milestone per week. |
| Decision friction | Median `created → approved` for RFCs; median `open → closed/escalated` for DecisionBlocks. |
| ACK-to-start latency | Median `aligned → first work.execution_started`. |

Surfaced at `/api/v2/projections/indicators` with optional `window=` param.
Cache layer: an in-memory map refreshed on every `AppendAndMaterialize`.

### 7.9 Tests

- A small integration suite in `internal/web/` spins a `dits-server`
  in-process, drives canonical flows via HTMX requests, asserts
  rendered HTML contains expected fragments.
- The substrate-side conformance test (`TestSubstrateConformance_v2`)
  is unchanged — proves the web UI is built on the same contract as
  CLI and MCP, no privileged code paths.
- **No screenshot/visual regression tests in v1.** Defer until churn
  pace warrants it.

### 7.10 What ships first

Within Phase 4, prioritise to enable the shortest demo path:

1. Static asset serving + design system (CSS/JS).
2. Auth + session (depends on Phase 2 landing).
3. Portfolio sheet read-only with column groups + presets.
4. Detail panel (ACK tab only).
5. Inline editing (ACK cells first — drives the most acks/week).
6. Attention view.
7. Remaining tabs and views.
8. Roadmap public projection.

---

## 8. Phase 5 — TUI (deferred)

Tracked but not planned in detail here. The TUI consumes the same
`workops` and v2 query API; nothing in Phases 1–4 should foreclose it.

---

## 9. Files to add / change (summary)

Concrete additions, not exhaustive line counts:

| Layer | Path | Action |
|---|---|---|
| Substrate | `internal/domain/meta.go` | Add Taxonomy, Role, RoleConstraint types + helpers. |
| Substrate | `internal/domain/event.go` | Add 4 generic event types + payloads. |
| Substrate | `internal/domain/validate.go` | Tier-1 validation for new events. |
| Substrate | `internal/domain/reduce.go` | Reducer cases + new WorkItem fields. |
| Substrate | `internal/domain/workitem.go` | Add `Classifications`, `RoleBindings`, `Diagnostics`. |
| Substrate | `internal/constraints/` | NEW pkg: predicate evaluator + four initial predicates. |
| Substrate | `internal/workops/workops.go` | New methods: `Classify`, `Declassify`, `BindRole`, `UnbindRole`. |
| Identity | `internal/auth/oidc.go` | NEW: generic OIDC client + session model. |
| Identity | `internal/crypto/custodial.go` | NEW: wrapped-key generation/storage. |
| Identity | `internal/server/server.go` | Auth middleware + signing hooks. |
| Identity | (migrations) | `actors_oauth` table; wrapped-key blob column on `actors`. |
| RE pack | `internal/repack/` | NEW pkg: meta bundle, ACK events, reducer extension, scheduler, projections. |
| Server | `internal/server/server.go` | New `/web/*` and `/api/v2/projections/*` routes. |
| Web | `internal/web/` | NEW pkg: handlers, templates, static assets, HTMX endpoints. |
| Web | `internal/web/templates/` | Sheet template + 12 view templates + panel partials. |
| Web | `internal/web/static/{ds,app}/` | Lift prototype's three CSS files. |
| Web | `internal/web/static/js/` | Cell nav, ⌘K, slide-over, HTMX hooks. |
| Web | `cmd/dits-server/main.go` | Mount web routes; OAuth flags. |
| Tests | per package | Mirror existing `workops_test.go` discipline; one conformance test per new contract. |

No file is *removed* by this plan. CLI and MCP are untouched.

---

## 10. Verification

End-to-end acceptance is the design's §13 success criteria, repeated
here in executable form:

1. **Substrate conformance** — `go test ./internal/...` green. Specifically
   `TestSubstrateConformance_v2` proves every web mutation is a normal
   signed event indistinguishable from a CLI-emitted event.
2. **Web demo path** — `dits init --pack=re` in a scratch directory,
   `dits-server --oauth=stub`, sign in to web, run the lifecycle: file
   RFC → leadership approve → spawn milestone → both ACKs → in_flight
   → ship. Target: under 15 minutes.
3. **Amendment auto-clears ACKs** — emit a `scope_change` amendment on
   a milestone with both ACKs accepted; both go to `pending`; indicator
   panel updates within one poll cycle.
4. **Separability diagnostic** — seed PROJ-176's collapse scenario in
   the test corpus; assert constraint engine emits the expected two
   diagnostics; UI surfaces them on the milestone page header.
5. **Roadmap is filtered** — `curl /web/roadmap` (unauthenticated)
   returns scoped data only — diff against a snapshot of the public
   fields whitelist; no RYG/diagnostics/internal commentary present.
6. **Off-Pilot edit recorded** — sign in as a non-Pilot, edit a
   milestone's RYG; assert event written and surfaced in the activity
   feed visible to the bound Pilot.
7. **Performance** — seed a 1500-person, ~800-milestone corpus; Portfolio
   landing renders in <500ms server-side. Indicator cache hit on
   subsequent loads.
8. **Pack-disable** — remove `re` from meta packs; `dits-server` still
   serves the substrate and CLI/MCP work; web `/for-you` reports
   "no RE pack enabled" gracefully. Confirms RE has no privileged
   substrate coupling.

---

## 11. Open questions carried forward

From the design + the prototype iterations:

1. **Custodial key revocation.** OAuth-revoked users: retire the actor
   row, keep prior signatures verifiable. Confirm UX (does the user
   ever see "your old events are still attributed to you"?). [Design §12.1]
2. **Meta sync churn from taxonomy nodes.** Deferred — model nodes as
   work items only if real-world churn proves the meta sync model noisy.
   No initial work. [Design §12.2]
3. **Predicate language extensibility.** Hard-code the four predicates
   in Go for v1. Revisit (Lua/Starlark/WASM) when a second pack appears.
   [Design §12.3]
4. **Pilot's Log refactor.** Per chat #6–7, collapse it into a
   per-milestone Updates stream (one event payload field `update_kind`).
   Back-burner; not in v1.
5. **Web style guide.** A small consistent style guide (colours,
   shortcuts, label vocabulary) before Phase 4 and any later TUI. One
   page; do it during Phase 4.
6. **RFC ↔ milestone lineage.** Option A from chat #11 (add `fromRfc`,
   surface in detail panels, "Origin RFC" filter chip, Attention bucket
   for orphan approvals) is the v1 target. Option B (unify into one
   tree-indented Portfolio) is the cleaner end state — revisit after
   living with A.

---

## 12. Phasing summary

| Phase | What | Weeks | Independently shippable? |
|---|---|---|---|
| 0 | Substrate gap audit (this doc) | 0 | n/a |
| 1 | Generic substrate extensions | 3–4 | Yes — any consumer can use. |
| 2 | OAuth + custodial signing | 2–3 | Yes — web exists in skeleton. |
| 3 | RE pack (meta, ACK events, scheduler) | 3–4 | Yes — CLI/MCP can drive RE flows before web. |
| 4 | Web UI matching prototype | 4–6 | Yes — but the demo path needs 1–3 landed. |
| 5 | TUI | 2–3 | Yes — separate ship. |

**Total: ~15–22 engineer-weeks** for the full stack. Front-loaded so
that Phase 1 is reusable by non-RE projects on its own — proving the
generalisation claim before any RE-specific code lands.

---

## 13. First concrete artifacts

The next steps after this plan is accepted in spirit:

1. **A small spec for the four constraint predicates** (input shape,
   output shape, evaluation semantics, edge cases) — feeds Phase 1.4.
2. **A meta migration sketch** — how a project upgrades from current
   `MetaConfig` to one carrying taxonomies/roles/constraints. Should
   be a no-op for projects that don't declare any.
3. **An OIDC provider matrix** — Google + GitHub + generic, what
   each requires for first-sign-in. Feeds Phase 2.1.

Everything else follows from these three.
