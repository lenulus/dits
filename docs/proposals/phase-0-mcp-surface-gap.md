# Phase 0 — MCP Surface Gap Report

**Status:** Deliverable for Phase 0 of the v2 plan
**Tracks:** [`radical-execution-implementation-plan-v2.md`](./radical-execution-implementation-plan-v2.md) §4, §7
**Sources:** `internal/mcp/tools.go` (today's surface) · `design-prototype/project/app/App.jsx` (`actions`, `PORTFOLIO_FILTER_FIELDS`, `pushEvent`) · `design-prototype/project/app/views.jsx` (view query shapes)
**Date:** 2026-05-29

---

Pilot's blast radius is exactly what MCP can do. Every mutation Pilot
performs lives in the prototype's `actions` object; every read shape
lives in a view's filter/pivot/bucket logic. This report cross-references
both against the current MCP tool surface and tags each row with a gap
class. The phase dependencies in the `Gap` column drive Phase 3 scope —
the closing table mirrors the v2 plan §7 tool families so the gap report
visibly produces them.

Gap vocabulary:

- **✓** — covered by an existing `dits_*` tool as-is.
- **needs filter args** — existing tool, new optional arguments (filter
  predicates, group_by, pagination).
- **needs new field** — existing tool, but the reduced shape lacks a
  field Pilot renders.
- **new tool — depends on Phase 1** — classification, role bindings,
  diagnostics; gated on the substrate primitives.
- **new tool — depends on Phase 2** — ACK lifecycle; gated on the ACK
  events in DITS core.
- **new tool** — meta admin, actor register/list, indicators, filtered
  list queries, events list, roadmap projection; no substrate dependency
  beyond Phase 3 itself.

Today's tool names are abbreviated (the `dits_work_` prefix is implied
where unambiguous).

---

## Filtered queries

These come from the Portfolio/ACK/RFC/Decisions/Outcomes sheets. The
current `dits_work_list` filters on `status`, `kind`, `ready`,
`claimed_by`, `blocked`, `include_closed` only — none of the RE field
filters, no group-by, no pagination. The twelve `PORTFOLIO_FILTER_FIELDS`
and the sheet pivots define the gap.

| Pilot operation | Today's MCP tool | Gap |
|---|---|---|
| List milestones (Portfolio sheet base query) | `work_list` (kind=milestone) | ✓ |
| List RFCs (RFC queue) | `work_list` (kind=rfc) | ✓ |
| List DecisionBlocks | `work_list` (kind=decision_block) | ✓ |
| List OutcomeAssessments | `work_list` (kind=outcome_assessment) | ✓ |
| Filter by status | `work_list` | ✓ |
| Filter by RYG (`ryg` field) | `work_list` | needs filter args + needs new field |
| Filter by specifier / builder / pilot (role actor) | `work_list` | new tool — depends on Phase 1 (role bindings filter) |
| Filter by alignment rollup (aligned / *_pending / rejected) | (none) | new tool — depends on Phase 2 |
| Filter by target quarter | `work_list` | needs filter args + needs new field |
| Filter by org node / product node (classification) | (none) | new tool — depends on Phase 1 |
| Filter by customer-visible (public/internal) | `work_list` | needs filter args + needs new field |
| Filter by "has diagnostics" / "violations only" | (none) | new tool — depends on Phase 1 |
| Filter by "has high-severity risk" | `work_list` | needs filter args + needs new field |
| Filter by "stale (>14d since status)" | `work_list` | needs filter args + needs new field |
| Filter by origin RFC (`fromRfc`) | `work_list` | needs filter args (relation predicate) |
| Pivot "Mine" (actor is specifier/builder/pilot) | (none) | new tool — depends on Phase 1 |
| Group by Status / Team / Product / Quarter | `work_list` | needs filter args (group_by) |
| Paginate large portfolio (limit/offset) | `work_list` | needs filter args |
| Show single work item (panel open / `openRecord`) | `work_show` | ✓ |

## Diagnostics & projections

The Roles diagnostic column, the Portfolio `Diag` column, the Attention
buckets, and the Leadership KPI row. None of this exists in MCP today.

| Pilot operation | Today's MCP tool | Gap |
|---|---|---|
| Get constraint diagnostics for a work item (panel header, `Diag` cell) | (none) | new tool — depends on Phase 1 |
| Get diagnostics for a role binding (`computeBindingDiagnostic`) | (none) | new tool — depends on Phase 1 |
| Dependency-closure-rate indicator (`depRate`) | (none) | new tool |
| Scope-change-velocity indicator (`scopeVel`) | (none) | new tool — depends on Phase 2 (ack_amended source) |
| Decision-friction indicator (`decFric`) | (none) | new tool |
| ACK-to-start-latency indicator (`ackLat`) | (none) | new tool — depends on Phase 2 |
| Attention: ACKs needed (my pending S/B-ACK) | (none) | new tool — depends on Phase 2 |
| Attention: targets passed / imminent (≤21d) | `work_list` | needs filter args + needs new field (target) |
| Attention: stale status (>14d on in-flight) | `work_list` | needs filter args + needs new field |
| Attention: high-severity risks on my work | (none) | needs new field (risks) |
| Attention: diagnostics on my work | (none) | new tool — depends on Phase 1 |
| Attention: decisions idle ≥5d on my work | `work_list` | needs filter args + needs new field (idle_days) |
| Attention: outcome assessments overdue | `work_list` (status=overdue) | ✓ |
| Attention: rejected ACKs in my view | (none) | new tool — depends on Phase 2 |
| Attention: approved RFCs with nothing spawned (≥30d) | (none) | needs filter args (RFC + no-inbound-`fromRfc` lineage) |
| Roadmap public projection (customer-visible + committed/in-flight/shipped) | (none) | new tool |

## Classification

The Portfolio Org/Product cells (`TaxonomyPicker`), `updateMilestone`
with `orgNode`/`productNode`, and the TaxonomyView. Depends on Phase 1's
`work.classified` / `work.declassified` events.

| Pilot operation | Today's MCP tool | Gap |
|---|---|---|
| Classify work item into a taxonomy node (set org/product) | (none) | new tool — depends on Phase 1 |
| Declassify work item from a node | (none) | new tool — depends on Phase 1 |
| Read a work item's classifications (panel "Roles & classification") | `work_show` | needs new field (Classifications) |
| Filter portfolio by classification node | (none) | new tool — depends on Phase 1 |
| TaxonomyView: list taxonomy nodes (org/product/goals tabs, levels) | `meta_show` | needs new field (Taxonomies in meta) |
| TaxonomyView: "used by N milestones" rollup per node | (none) | new tool — depends on Phase 1 |

## Role bindings

The RolesView sheet, the Portfolio role columns, `updateBinding`,
`newBinding`. Depends on Phase 1's `work.role_bound` /
`work.role_unbound` events. (Note: setting specifier/builder/pilot in the
prototype's Portfolio sheet is modeled as role binding, not the existing
generic `assign`.)

| Pilot operation | Today's MCP tool | Gap |
|---|---|---|
| Bind a role to an actor on a work item (`newBinding`, role cell edit) | (none) | new tool — depends on Phase 1 |
| Unbind / rebind a role | (none) | new tool — depends on Phase 1 |
| List role bindings for a work item (panel roles section) | (none) | new tool — depends on Phase 1 |
| List all role bindings (RolesView sheet) | (none) | new tool — depends on Phase 1 |
| Generic assign (legacy, not RE role) | `work_assign` | ✓ |

## ACK lifecycle

The ACK workspace, the Portfolio S-ACK/B-ACK cells, the panel ACK tab,
`setAck` / `updateAck` / `acceptBoth` / `commitAck` / `amendAck`, and the
`ackHistory`. All depend on Phase 2's ACK events in DITS core.

| Pilot operation | Today's MCP tool | Gap |
|---|---|---|
| File an ACK (specifier files commitment) | (none) | new tool — depends on Phase 2 |
| Accept one side (S or B) of an ACK (`setAck` accepted) | (none) | new tool — depends on Phase 2 |
| Reject one side of an ACK (`setAck` rejected) | (none) | new tool — depends on Phase 2 |
| Quick-accept both sides (`acceptBoth` / `commitAck`) | (none) | new tool — depends on Phase 2 |
| Amend a commitment (`amendAck`, scope/timeline/target/clarification) | (none) | new tool — depends on Phase 2 |
| Auto-clear ACKs on material amendment (reducer side effect) | (none) | new tool — depends on Phase 2 (server-side reducer) |
| Read ACK state + rollup for a work item (panel/sheet) | `work_show` | needs new field (Acks) |
| Read ACK history table (`ackHistory`) | `work_events` | ✓ (filtered to ack_* types) |
| ACK pivots (needs-my-S/B, misaligned, aligned, rejected) | (none) | new tool — depends on Phase 2 |

## Meta admin

TaxonomyView add/move/retire node, the RE meta bundle apply
(`pilot init`), reading allowed kinds/statuses/labels. `meta_show` exists
read-only; everything write-side and the taxonomy/role extensions are
new.

| Pilot operation | Today's MCP tool | Gap |
|---|---|---|
| Read meta config (kinds, labels, statuses) | `meta_show` | ✓ |
| Read meta taxonomies / roles / role-constraints | `meta_show` | needs new field (Phase 1 meta fields) |
| Apply an RE meta bundle (`meta_apply(json)`, idempotent) | (none) | new tool |
| Add a taxonomy node | (none) | new tool |
| Move a taxonomy node | (none) | new tool |
| Retire a taxonomy node | (none) | new tool |

## Identity

Custodial signing in Pilot uploads each user's public key to DITS; the
actor pickers (`ActorPicker`) and `renderActor` need an actor directory.

| Pilot operation | Today's MCP tool | Gap |
|---|---|---|
| Show current actor identity | `identity_show` | ✓ |
| Register an actor public key (`actor_register`, custodial onboarding) | (none) | new tool |
| List actors (ActorPicker directory, `D.ACTORS`) | (none) | new tool |
| Resolve actor metadata (name, initials, role, agent flag) | (none) | new tool (part of actor_list shape) |

## Lifecycle / CRUD already covered

The generic event vocabulary already exposed. These back the prototype's
status/dep/comment/close mutations and the event log, and need no new
tool — though several need extra fields on the reduced shape to render the
RE-specific cells.

| Pilot operation | Today's MCP tool | Gap |
|---|---|---|
| Create milestone / RFC / decision / outcome | `work_create` | ✓ |
| Spawn milestone from RFC (`spawnMilestone` — create + link `fromRfc`) | `work_create` + `work_link` | ✓ |
| Set status (`updateDecision`, `updateMilestone` status, scheduler overdue) | `work_status` | ✓ |
| Add dependency (`addDep`) | `work_link` (depends_on) | ✓ |
| Remove dependency (`removeDep`) | (none) | new tool (work_unlink) |
| Post status update / narrative (`addStatusUpdate`) | `work_checkpoint` or `work_comment` | needs new field (status_narrative/RYG) |
| Set RYG health (`updateMilestone` ryg) | (none) | new tool or `work_status` field — needs new field |
| Add risk (`addRisk`, severity) | (none) | new tool — needs new field (risks) |
| Add next step (`addNextStep`) | (none) | new tool — needs new field (next_steps) |
| Add Pilot's Log entry (`addLogEntry`, observation/risk/decision/...) | `work_observe` | ✓ |
| Comment on a work item | `work_comment` | ✓ |
| Record outcome verdict (`updateOutcome` result/value) | `work_eval_complete` | ✓ |
| Close / resolve a DecisionBlock | `work_close` | ✓ |
| Request review / escalate (`updateDecision` escalated) | (none) | new tool (review_request) |
| Reopen a work item | `work_reopen` | ✓ |
| Read full event log for a work item (panel Receipts tab) | `work_events` | ✓ |
| Read global event log (EventLogView, all events) | (none) | new tool (events_list) |
| Filter events by type | `work_events` (type) | ✓ (per-item only; global needs events_list) |
| Attach artifact / receipt | `work_attach` | ✓ |
| Lease / start / checkpoint / complete / fail (agent execution loop) | `work_lease*` / `_start` / `_checkpoint` / `_complete` / `_fail` | ✓ |
| Handoff to another actor | `work_handoff` | ✓ |
| Block / unblock | `work_block` / `work_unblock` | ✓ |
| Sync with peers | `sync` | ✓ |

---

## Summary of new tool families for Phase 3

This mirrors the v2 plan §7 table — the gap rows above resolve into these
families. The dependency column states what each family is gated on.

| Family | New tools | Gated on |
|---|---|---|
| **Filtered queries** | `work_list(filters, group_by, limit, offset)`, `work_get(id)`, `events_list(target?, type?, since?)`, `work_unlink` | Phase 3 (field-bearing variants need Phase 1/2 fields) |
| **Diagnostics & projections** | `diagnostics_get(work_id)`, `indicators_get(window?)`, `roadmap_get()` | Phase 1 (diagnostics), Phase 2 (scope/ACK indicators) |
| **Classification** | `work_classify`, `work_declassify` | Phase 1 |
| **Role bindings** | `work_role_bind`, `work_role_unbind`, `role_bindings_list(work_id?)` | Phase 1 |
| **ACK lifecycle** | `ack_file`, `ack_accept`, `ack_reject`, `ack_amend` | Phase 2 |
| **Meta admin** | `meta_get` (extend), `meta_apply(json)`, `taxonomy_node_add/move/retire` | Phase 1 (taxonomy/role meta) |
| **Identity** | `actor_register(public_key)`, `actor_list` | Phase 3 |

Existing tools needing only **field additions** (no new tool) once Phase
1/2 land their reduced-state fields: `work_show` (Classifications,
RoleBindings, Acks, Diagnostics, ryg, risks, next_steps, status
narrative, target), `meta_show` (Taxonomies, Roles, RoleConstraints).
`work_list` gains filter/group_by/pagination args.

**Headline:** of ~70 Pilot operations, roughly half are covered today
(the generic lifecycle/CRUD vocabulary). The uncovered half clusters into
the seven families above and is fully gated on Phases 1–2 for the
substrate-bearing tools (classification, role bindings, ACK lifecycle,
diagnostics) and otherwise on Phase 3 itself (filtered queries, meta
admin, identity, indicators, roadmap, global event list).
