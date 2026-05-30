# Pilot Completion Plan — closing the prototype gap

**Status:** Plan / ready to execute in a fresh session
**Tracks:** [`radical-execution-implementation-plan-v2.md`](./radical-execution-implementation-plan-v2.md) (the design) · [`re-implementation-status.md`](./re-implementation-status.md) (what shipped) · the `DITS RE Prototype` bundle under `docs/proposals/design-prototype/` (the UX spec)
**Branch:** `pilot` (single branch; do not create feature branches)

This plan takes the Pilot frontend from its current state (chrome + read
views + a partial write path) to **prototype parity**. It is written to be
executed by an agent team in a **fresh session** with no dependence on prior
conversation — everything needed is here or cited.

---

## 1. Where we are vs. the prototype

**Live and correct today:**

- All 12 routes render with the real chrome (TopBar, Sidebar 4×sections,
  header, hintbar, ⌘K shell), faithful CSS, and live MCP data.
- Sheet primitive: frozen ID+Title, pinned table width, sticky gutter,
  column-group bands, JTBD presets, keyboard nav (arrows/Tab/⌘Enter/Space/Esc),
  multi-select.
- Slide-over panel: open/close/tab deep-links; **ACK** and **Status** tabs are
  editable (file/accept/reject/amend, role bind, classify, set status) — each
  POST is a real signed event.
- Leadership: live four-indicator KPI row. Taxonomies / Events / Roadmap:
  read-only projections. Scheduler + `pilot init` work.

**The gap, by class** (full per-item detail in §4):

1. **Sheet is not yet a spreadsheet** — no inline cell editing, no filter-chip
   builder, no group-by control, no bulk-action bar, quick-add row is inert.
2. **4 of 6 panel tabs are stubs** — Stages, Deps, Updates, Receipts; and the
   ACK tab is missing its history table, target-precision editor, stages
   preview, diagnostics line, and click-to-edit commitment fields.
3. **No detail panels for RFC / Decision / Outcome** — only milestones open a
   real panel.
4. **Per-view richness missing** — Attention shows 3 of 9 buckets and its
   actions are detours; Leadership lacks pattern blocks + goal health; Pilot's
   Log lacks the quick-entry row; Decisions/Outcomes use a generic sheet
   missing their real columns and idle/SLA visuals; Roles never calls
   `DiagnosticsGet`/`RoleBindingsList`; Taxonomies is read-only (no node CRUD,
   no Goals result column, no "used by" counts).
5. **Cross-cutting** — actor identity is raw IDs (no avatars/pickers;
   `ActorList` unused); ⌘K does nav only (no record jump-in / create); no
   RFC↔milestone lineage; no nested-deliverable tree; no target-precision
   editor; full-page reloads instead of HTMX swaps.
6. **Unused client methods** (the surface is ready, the UI isn't):
   `Declassify`, `UnbindRole`, `Link`, `WorkCreate`, `TaxonomyNode{Add,Move,Retire}`,
   `ActorRegister`, `RoleBindingsList`, `DiagnosticsGet`, `ActorList`.

---

## 2. The long-pole: methodology state the substrate doesn't model yet

**Read this before scoping anything else.** A large fraction of the
prototype's surface is **methodology state that does not exist on the DITS
`WorkItem`**: RYG health, the stages timeline (Dogfood/Beta/GA + dates +
precision), risks (severity), next-steps, the status narrative, the delivery
target + its precision (Q/M/D), `customer_visible`, and the RFC→milestone
lineage (`fromRfc`). Today the substrate carries kind, status, classifications,
role bindings, ACKs, relations, diagnostics — and nothing else from that list.

The architectural thesis is **"nothing methodology-specific belongs in DITS"**,
so the answer is **not** to add `ryg`/`stages` columns to `domain.WorkItem`.
The two clean options:

- **Option A — generic events + payload conventions (recommended).** Extend the
  design's existing PilotLog convention (§6.6): ride methodology state on
  generic DITS events with a typed payload discriminator, and let Pilot project
  them. Concretely:
  - status narrative → `work.observation_recorded` (or `work.commented`) with
    `entry_type=status`; RYG → same with `entry_type=integrity_call` /
    `health=g|y|r` (latest wins).
  - risks → `work.finding_recorded` (`entry_type=risk`, `severity`); next-steps
    → `work.observation_recorded` (`entry_type=next`).
  - stages, delivery target (+precision), `customer_visible` → a small set of
    **generic** events that are not RE-specific: e.g. `work.field_set`
    (`{field, value}`) for opaque projection fields, or a generic
    `work.schedule_set` for stages/target. These stay methodology-agnostic
    (any methodology can use a key/value projection field).
  - lineage → existing `work.linked` with relation `derived_from`
    (RFC) / `parent_of` (nested deliverables). **No new event needed.**
  Pilot's MCP client DTO gains projected fields (RYG, Stages, Risks, NextSteps,
  StatusNarrative, Target, TargetPrecision, CustomerVisible, FromRfc) computed
  by the **reducer or by Pilot** from those events.
- **Option B — Pilot-local store.** Keep this state in Pilot's own SQLite
  (`internal/pilot/store`, currently a stub), keyed by work-item ID. Faster to
  build, but it breaks the "every change is a signed event on the DAG"
  invariant for this state and won't sync — **not recommended** beyond a cache.

**Decision (Track 0) — RESOLVED: Option A.** Use existing generic events where
they fit and add at most one or two genuinely-generic projection events to DITS
core, plus matching MCP tools, client methods, and reducer projection. This
keeps DITS substrate-clean and every methodology field a signed, syncable
event. Everything in §4 that needs this state is marked **[needs Track 0]**.
Concrete mapping to implement in 0.2/0.3:

| Methodology state | Representation (Option A) |
|---|---|
| Status narrative | `work.observation_recorded` payload `entry_type=status` (latest wins) |
| RYG health | generic `work.field_set` `{field:"ryg", value:"g\|y\|r"}` (latest wins) |
| Delivery target + precision | `work.field_set` `{field:"target", value}` + `{field:"target_precision", value:"Q\|M\|D"}` |
| `customer_visible` | `work.field_set` `{field:"customer_visible", value:"true\|false"}` |
| Stages (Dogfood/Beta/GA + dates + precision + state) | generic `work.schedule_set` `{stages:[{key,label,date,precision,state}]}` (replaces; latest wins) |
| Risks (severity) | `work.finding_recorded` payload `entry_type=risk, severity=high\|medium\|low` (open until retracted) |
| Next steps | `work.observation_recorded` payload `entry_type=next` |
| RFC → milestone lineage | `work.linked` relation `derived_from` (no new event) |
| Nested deliverables | `work.linked` relation `parent_of` (no new event) |

`work.field_set` (scalar key/value projection) and `work.schedule_set` (a
staged-timeline projection) are the only new core events, and both are
methodology-agnostic — any consumer can use them. The reducer projects them
onto `WorkItem` (e.g. `Fields map[string]string`, `Stages []Stage`), and Pilot
derives RYG/target/risks/etc. from `Fields` + the typed observations/findings.

Two known substrate gaps from the v2 build also live here (carried from
`re-implementation-status.md`):

- **`dits_event_submit(signed_event)`** — required for true per-user custodial
  signing (§8.2). Until then mutations sign as the dits-mcp actor.
- **`dits_review_request(id, reviewer_role)`** — lets the scheduler/RFC flow
  emit `work.review_requested` to Leadership instead of just setting status.

---

## 3. Execution model

Same working agreement as the v2 build:

- **Single branch `pilot`.** No worktrees. Coordinator commits; specialists
  implement disjoint file sets and report. One commit per logical unit,
  existing log style (`feat(substrate):`, `feat(mcp):`, `feat(pilot):`,
  `docs(re):`). Push every few commits.
- **Adopt HTMX now** (it's in the §9.1 stack but unused). Inline cell edits and
  panel tab/row swaps should be HTMX partial swaps, not full-page reloads.
  Add a tiny `hx-*` convention + a `/partials/...` handler family. This is a
  Track-B/C foundational decision — make it once, early.
- **Keep `go build ./...` + `go test ./...` green at every commit.** The
  `TestForbiddenImports` boundary (Pilot imports no `internal/{domain,workops,
  mcp,...}`) must stay green — all Pilot work goes through `internal/pilot/mcp`.
- **Verify against the prototype** — for each view, diff the rendered HTML/
  behavior against `docs/proposals/design-prototype/project/app/*.jsx`.

### Waves & tracks

```
Wave 1 (sequential-ish, gates the rest):
  Track 0  Methodology-state model  (substrate + MCP + client + DTO)  ── design spike then build

Wave 2 (parallel once Track 0's client DTO lands):
  Track B  Sheet interactivity (inline edit, filters, group-by, bulk, quick-add) + HTMX foundation
  Track C  Panel completion (Stages/Deps/Updates/Receipts + ACK-tab finish + RFC/Decision/Outcome panels)
  Track D  Per-view richness (Attention 9 buckets, Leadership pattern/goal, Log quick-entry, Decisions/Outcomes/Roles/Taxonomies)
  Track E  Identity & cross-cutting (actor directory/avatars, ⌘K record+create, lineage, nested tree, target editor)

Any time (independent):
  Track F  Identity bridge (OAuth/sessions/custodial signing + dits_event_submit + dits_review_request)
```

File-ownership partition that keeps Wave 2 collision-free (each track owns
distinct files; the coordinator integrates the shared `Server`/router):

- **Track B** → `internal/pilot/web/static/js/sheet.js`, `templates/sheet.html`,
  a new `internal/pilot/web/handlers/sheet_edit.go` (+ filter/groupby/bulk
  endpoints).
- **Track C** → `internal/pilot/web/handlers/mutations.go` (panel bodies) +
  `panel.go`, `templates/panel.html`, `static/js/panel.js`, a new
  `handlers/panels_detail.go` (RFC/Decision/Outcome).
- **Track D** → `internal/pilot/web/handlers/builders.go` + per-view files
  (`handlers/attention.go`, `leadership.go`, `log.go`, `taxonomies_admin.go`).
- **Track E** → `internal/pilot/web/static/js/palette.js`, `handlers/cells.go`
  (avatars), a new `internal/pilot/web/actors.go` (directory cache), lineage
  bits in `builders.go` (coordinate with D).
- **Track F** → `internal/pilot/{auth,signing,store}` + (substrate)
  `internal/mcp/tools.go` + `internal/domain` for the two new tools.

The coordinator owns `handlers/handlers.go` (the `Server` + route table) and
merges each track's route registrations.

---

## 4. Backlog (every gap, as executable items)

Each item: **[Track]** title — what to build — acceptance. `[needs Track 0]`
marks dependence on the methodology-state model.

### Track 0 — methodology-state model
- **0.1 Design spike** — choose Option A/B (recommend A); write the event/field
  mapping for RYG, stages, target(+precision), risks, next-steps, status
  narrative, customer_visible, lineage. Output: a short ADR appended here.
- **0.2 Substrate** — add the chosen generic event(s) (`work.field_set` and/or
  `work.schedule_set`) + payloads + validate + reducer projection onto
  `WorkItem` (new projected fields). Tests in `internal/domain`.
- **0.3 MCP + client** — tools to set those fields; extend the Pilot DTO with
  the projected fields; client methods. Conformance + client tests.
- **Acceptance:** a milestone round-trips RYG/stages/target/risks/next/narrative/
  visibility through MCP and back into the DTO.

### Track B — sheet interactivity
- **B.1 HTMX foundation** — add htmx; a `/partials/sheet/cell` swap convention.
- **B.2 Inline cell editing** — click/Enter on an editable cell opens the right
  editor in-cell (select, actor picker, taxonomy picker, target editor, ACK
  picker, text); commit → MCP mutation → swap the cell. Wire `Enter` in
  `sheet.js` (currently a TODO). Acceptance: edit status/RYG/ACK/role/
  classification/target from the grid without opening the panel.
- **B.3 Filter-chip builder** — the 12 `PORTFOLIO_FILTER_FIELDS`, the two-step
  +Add-filter popover, editable/removable chips, AND-combination. Drive the
  existing `mcp.Filters`. Acceptance: every prototype filter field works and
  composes.
- **B.4 Group-by control** — None/Status/Team/Product/Quarter with group header
  rows + counts (and the tree-sort for nested deliverables — coordinate E).
- **B.5 Bulk-action bar** — appears on selection; Reassign / Set quarter /
  Classify / Accept ACK over the selected set (loop client mutators).
- **B.6 Quick-add row** — wire the `+ New …` row to `WorkCreate` (per view kind)
  then open the new record's panel.

### Track C — slide-over panel completion
- **C.1 ACK tab finish** — diagnostics line (`DiagnosticsGet`), the ACK history
  table (from `work_events` filtered to `ack_*`), stages preview, the
  delivery-target editor (Q/M/D precision) `[needs Track 0]`, and
  click-to-edit (InlineEditableField) scope/outcome/criteria.
- **C.2 Status tab finish** — RYG segmented control `[needs Track 0]`,
  current-status card + post-update composer (⌘+Enter), **Next steps above
  Risks**, required-for-Red/Yellow chip when risks empty `[needs Track 0]`.
- **C.3 Stages tab** — timeline with per-stage state/date/precision + add/edit
  `[needs Track 0]`.
- **C.4 Deps tab** — editable upstream/downstream via `Link`/`Unlink` +
  link-search; navigate to a dep. (Substrate-ready: relations exist.)
- **C.5 Updates tab** — merged feed (observations + status transitions) with
  kind chips + quick-entry `[needs Track 0 for status/risk kinds]`.
- **C.6 Receipts tab** — signed event log scoped to the record (have the data;
  render it).
- **C.7 Panel footer actions** — Accept-both / Amend(clears) for milestones;
  Resolve/Escalate for decisions.
- **C.8 RFC / Decision / Outcome detail panels** — the prototype's RfcDetail
  (summary, acceptance, Produced milestones, Spawn), DecisionDetail (auto-
  escalation notice), OutcomeDetail (verdict×value select, landed-low-value
  pattern note). Currently only milestones get a panel.

### Track D — per-view richness
- **D.1 Attention — all 9 buckets** — targets passed/imminent (≤21d), stale
  status (>14d), high-severity risks, decisions idle ≥5d, outcomes overdue,
  approved-RFC-dead-on-arrival (≥30d) — in addition to the 3 present. Inline
  actions that mutate **in place** (Accept/Reject/Resolve/Escalate/Spawn) via
  HTMX, not links. Several `[needs Track 0]` (risks, stale, targets).
- **D.2 Leadership** — pattern blocks (scope-velocity / landed-low-value /
  pilot-collapse) + goal-health rollup (worst-of-children over the goals
  taxonomy) + adjudication queue. `[needs Track 0]` for some signals.
- **D.3 Pilot's Log** — top quick-entry row (kind select + body + ⌘Enter →
  signed event) + the journal feed. `[needs Track 0]` for entry kinds.
- **D.4 Decisions/Outcomes** — real columns (Decisions: idle-days color scale,
  owner; Outcomes: verdict/value/SLA) + their detail panels (C.8).
- **D.5 Roles** — call `RoleBindingsList`/`DiagnosticsGet`; show the full
  diagnostic text + actor role; the "constraints flag, don't gate" banner is
  present — keep it. Add bind/unbind from the sheet.
- **D.6 Taxonomies** — node CRUD (`TaxonomyNode{Add,Move,Retire}`), the Goals
  **result** column (achieved/partial/missed/in_progress), and the per-node
  "used by N milestones" count.

### Track E — identity & cross-cutting
- **E.1 Actor directory** — call `ActorList`; render initials avatars + names
  everywhere (replace raw-ID `actorCell`); build the searchable ActorPicker for
  role/owner cells and the filter fields.
- **E.2 ⌘K record jump-in + create** — extend `palette.js` beyond nav: index
  milestones/RFCs/decisions (open record) + Create actions (`WorkCreate`).
- **E.3 RFC ↔ milestone lineage** — `Spawn milestone from RFC` (WorkCreate +
  `Link derived_from`), "Produced milestones" on the RFC panel, "Origin RFC"
  line on the milestone, the Origin-RFC filter chip, and the Attention
  dead-on-arrival bucket (D.1).
- **E.4 Nested deliverables** — tree-sort + indent/caret in Portfolio using
  `parent_of` relations (coordinate B.4).

### Track F — identity bridge (deferred §8.2) + scheduler escalation
- **F.1 `dits_event_submit`** (substrate+MCP) — append a pre-signed event after
  verifying its signature; lets Pilot sign per-user.
- **F.2 OAuth/OIDC + sessions + custodial Ed25519** in `internal/pilot/{auth,
  signing,store}` (real, not stubs); Pilot reimplements canonical-JSON+sign in
  its tree; `actor_register` on first sign-in.
- **F.3 `dits_review_request`** (substrate+MCP) — scheduler escalation + RFC
  target-change route to Leadership emit a real `work.review_requested`.

---

## 5. Day-1 setup for the fresh session

1. Read this plan, `re-implementation-status.md`, and the prototype
   (`design-prototype/project/app/{App,views,sheet,panels,data}.jsx` +
   `chats/chat1.md` — the chat is the spec-by-iteration).
2. Build + smoke the current app to anchor: `make build`, then per
   `cmd/pilot/README.md` (`dits init` → `pilot init` → seed → `pilot serve`).
3. **Resolve Track 0.1** (the methodology-state ADR) first — it gates Wave 2.
   This is the one decision worth surfacing to the user before building.
4. Spawn the team: one specialist per Wave-2 track (B/C/D/E) + Track 0 first,
   F in parallel. Each prompt must name its file ownership (§3) explicitly.
5. After each parallel burst: coordinator runs `git status`/`diff`, one commit
   per unit, push, then launch the next burst.

## 6. Definition of done (prototype parity)

A methodology-familiar reviewer can, in the browser, drive the full v2 §13
loop **without leaving the UI**: edit cells inline in the grid; filter/group/
bulk-act; open any record's panel and edit every tab; file→accept→amend ACKs;
set RYG/stages/risks/next/status; spawn a milestone from an RFC and see the
lineage; work the Attention queue with inline actions; and watch Leadership's
indicators, pattern blocks, and goal health update — all as signed events on
the DAG. The 12 views match the prototype screen-for-screen.
