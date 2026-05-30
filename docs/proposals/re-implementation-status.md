# Radical Execution on DITS — Implementation Status

**Tracks:** [`radical-execution-implementation-plan-v2.md`](./radical-execution-implementation-plan-v2.md)
**Branch:** `pilot`

A running record of what has landed against the v2 plan, and the
follow-ups deliberately left open. Honest about depth: the substrate half
(Phases 0–3) is complete and fully tested; Pilot is a working live
frontend with the methodology automation in place; the identity bridge and
the inline-mutation UI are the main remaining build.

## Status by phase

| Phase | Plan ref | Status | Notes |
|---|---|---|---|
| 0 — MCP surface audit | §4 | ✅ Done | `phase-0-mcp-surface-gap.md`. |
| 1 — Substrate primitives | §5 | ✅ Done | Taxonomies/Roles/RoleConstraints, 4 generic events, `internal/constraints` (4 predicates), workops Classify/Declassify/BindRole/UnbindRole. PROJ-176 golden. |
| 2 — ACK lifecycle (core) | §6 | ✅ Done | 5 events, `ack.go` (AckState/AckRollup/ComputeAckRollup), reducer materialization, workops auto-clear orchestration. |
| 3 — MCP surface buildout | §7 | ✅ Done | 18 tools (classification, roles, ACK, diagnostics, meta admin, identity, events, filtered work_list). Also fixed a sqlite persistence gap the new read tools exposed. |
| 4 — Pilot scaffold | §8 | ◑ Partial | Binary + package tree + forbidden-imports lint ✅. Typed MCP client over stdio ✅. **OAuth/sessions/custodial signing: still stubs** (see follow-ups). |
| 5 — Pilot UI | §9 | ◑ Partial | Foundation (CSS lift, layout, Sheet primitive, slide-over panel, ⌘K) ✅. All 12 views wired to **live MCP data** ✅; Leadership shows the live four-indicator KPI row. **Mutation write path live in the slide-over panel** (status, ACK file/accept/reject/amend, role bind, classify — each a signed event). Still pending: in-sheet inline cell editing, the remaining panel tab bodies (Stages/Deps/Updates/Receipts), filter-chip UI, bulk actions. |
| 6 — Scheduler / projections / meta | §10 | ✅ Done | RE meta bundle + `pilot init` ✅. Indicator projections (4 indicators + cache) ✅, rendered on Leadership. Scheduler ✅ (`pilot serve --schedule`) — verified auto-creating an outcome_assessment for a shipped milestone over MCP. |

## Verified end-to-end

`dits init` → `pilot init --project` (applies the RE meta bundle over MCP
via `dits_meta_apply`) → seed milestones via the `dits` CLI →
`pilot serve --project --mcp-command` renders live data:
`/portfolio` (live milestones), `/taxonomies` (the RE taxonomy skeletons
from `MetaGet`), `/roadmap` (committed+ only — an `open` milestone is
correctly excluded), `/events` (live signed events), `/` → `/for-you`.
The full path is **browser → Pilot → stdio MCP → dits-mcp → DITS substrate
→ reduced state → DTO → server-rendered HTML**, with Pilot importing no
DITS-internal package (enforced by `TestForbiddenImports`).

With `pilot serve --schedule`, the RE scheduler runs in-process and, on the
seeded project, auto-created `REDEMO-5` (`Outcome: Mobile API parity`) for
the shipped milestone `REDEMO-4` — `WorkList(shipped) → WorkCreate →
Link`, all over MCP. The Leadership view renders the four leading
indicators computed Pilot-side from the event log.

## Follow-ups / known gaps

These were surfaced during implementation and are deliberately deferred,
each with a recommended resolution:

1. **Custodial signing needs a "submit pre-signed event" MCP tool.**
   Plan §8.2 has Pilot sign each mutation with the user's custodial key and
   submit it via MCP, preserving per-user attribution. But today's `dits_*`
   mutation tools construct *and sign* events server-side as the dits-mcp
   project actor — there is no tool to append an externally-signed event.
   For a single-operator demo this is fine (DITS genuinely cannot tell a
   Pilot event from a CLI event — exactly the §8.2 invariant). True
   multi-user custodial signing needs a `dits_event_submit(signed_event)`
   tool that verifies the signature and appends, plus Pilot reimplementing
   `CanonicalEventJSON` + Ed25519 signing in its own tree (it cannot import
   `internal/crypto`). Until then, OAuth/sessions/custodial keys in
   `internal/pilot/{auth,signing,store}` remain stubs.

2. **Auto-escalation has no `review_requested` MCP tool.** Plan §10.2 has
   the scheduler emit `work.review_requested` targeting Leadership for idle
   DecisionBlocks. No MCP tool exposes that event, so the scheduler uses the
   substrate-visible equivalent (`status=escalated`). A
   `dits_review_request(id, reviewer_role)` tool would let it target
   Leadership explicitly.

3. **Mutation UI (Phase 5 write path).** ✅ The slide-over panel is now
   editable — status, ACK file/accept/reject/amend, role bind, and classify
   each POST to a one-mutation endpoint and land as signed events. Still
   pending: in-sheet inline cell editing, the Stages/Deps/Updates/Receipts
   panel tab bodies, the filter-chip builder, and bulk actions. No substrate
   work required; this is Pilot-side UI over the existing client mutators.

4. **Indicator source.** Per §10.3, indicators are computed in Pilot
   (keeps DITS substrate-clean). Promote to a DITS-side `indicators_get`
   tool only if performance demands.

5. **Actor org positions for hierarchy constraints.** `diagnostics_get`
   evaluates only the position-free predicates (`distinct_actors`,
   `requires_classification`); `not_reports_to_within` /
   `classified_in_same_node` need an actor→taxonomy-node store the
   substrate does not have yet (design open-question #2). The constraint
   evaluator already accepts positions as a parameter, so wiring a store
   later is additive.
