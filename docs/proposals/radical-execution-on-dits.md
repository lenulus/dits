# Proposal: Radical Execution on DITS

**Status:** Draft / Proposal
**Author:** generated for aglaforge@gmail.com
**Date:** 2026-05-23
**Branch:** claude/eloquent-rubin-zuAB5

---

## 1. Executive summary

The Radical Execution (RE) methodology described in
[`lenulus/radical_execution` PRD v0.2](https://github.com/lenulus/radical_execution/blob/main/docs/prd_v0.2.md)
is a strong fit for the DITS substrate. RE and DITS independently arrived at
three of the same foundational decisions: event log as ground truth,
attribution-not-prevention as the enforcement mechanism, and derived
operational health computed from event projections.

This proposal frames RE as a **methodology overlay** on the DITS coordination
substrate. The DITS core gains a small number of **generic primitives**
(taxonomies, classifications, role bindings, role-constraint diagnostics) that
are not RE-specific — they are general enough that any future methodology can
adopt them. RE-specific structure (ACK lifecycle, PilotLog entry conventions,
DecisionBlock escalations, OutcomeAssessment SLAs) ships as a **methodology
pack** that uses those primitives but does not contaminate DITS core.

A new web frontend, served by `dits-server`, renders the views the RE PRD
specifies (Leadership portfolio default landing, role workspaces, milestone
detail, external customer roadmap). Identity for browser users is bridged via
OAuth with server-side custodial signing keys, preserving DITS's signed-event
invariant without forcing per-user CLI installation.

Nothing in this proposal asks DITS to abandon a v2 invariant, and nothing in
it forces an in-place migration. RE becomes a versioned meta configuration
and a thin software layer; DITS remains the substrate.

---

## 2. Background and motivation

### 2.1 What RE is

RE is a portfolio-management system that operationalizes a specific execution
methodology: bilateral ACK (acknowledgment) between Specifier and Builder,
an independent Pilot function for navigation and integrity, and four
automated leading indicators (dependency closure rate, scope-change velocity,
decision friction, ACK-to-start latency). Its philosophy is "accountability
through legibility, not prevention" — full attribution, no field-level locks.

### 2.2 Why DITS

The DITS v2 PRD frames the system as a "durable coordination substrate for
multiple actors, including agents." Ticketing is one projection over that
substrate; the underlying primitives (work items with kinds, an event DAG,
materialized coordination state) are deliberately general. RE is exactly the
kind of methodology that benefits from running on a substrate rather than
re-implementing one.

### 2.3 Why now

Three forces converge:

1. **DITS is feature-complete enough at the substrate layer.** Events, sync,
   blobs, signatures, reducers, and v2 query API are implemented.
2. **DITS has no UI yet.** The first UI built will set expectations for what
   DITS looks like. RE's PRD describes views that are useful far beyond RE
   (portfolio rollup, freshness queues, audit feeds), so RE is a productive
   forcing function for that UI.
3. **The alignment is unusually deep.** Most "portfolio tools" assume
   row-level CRUD with locks. RE rejects that model; so does DITS. Building
   RE on a different substrate would be fighting the substrate.

---

## 3. Goals and non-goals

### 3.1 Goals

1. Express the RE PRD's data model on the DITS substrate without altering
   any v2 invariant.
2. Generalize the substrate extensions so RE is one consumer, not the
   privileged consumer.
3. Specify an identity bridge that lets browser users mutate state while
   preserving signed-event attribution.
4. Specify a web UI architecture that renders the RE PRD's required views.
5. Provide a phasing plan that can be executed incrementally — RE adoption
   is not gated on every extension landing at once.

### 3.2 Non-goals

1. Migrating any existing RE installation. This is a greenfield design;
   RE does not exist yet outside its PRD.
2. Imposing RBAC on DITS. Role assignments are first-class **data**;
   constraints are **diagnostics**, not gates.
3. Replacing the DITS CLI or MCP server. They remain peer frontends.
4. Cross-project federation. RE assumes a single coordinated portfolio
   (one project), consistent with DITS v2 non-goals.
5. Real-time push (WebSocket/SSE). Polling against the existing query API
   is sufficient for v1; streaming is a later optimization.

---

## 4. Architectural thesis

> **DITS provides the substrate. RE provides the methodology. The web UI is
> a projection layer over both.**

Concretely, the system splits into four layers:

```
+--------------------------------------------------+
|  Web UI (cmd/dits-web or routes on dits-server)  |
|    - Leadership portfolio (default landing)      |
|    - Specifier / Builder / Pilot workspaces       |
|    - Milestone detail page                       |
|    - Customer roadmap (read-only, no-auth)       |
+--------------------------------------------------+
|  Methodology overlay (RE pack)                   |
|    - ACK event conventions and reducer view      |
|    - PilotLog entry type conventions             |
|    - Indicator projections                       |
|    - Auto-scheduler (escalations, assessments)   |
+--------------------------------------------------+
|  DITS substrate extensions (generic)             |
|    - Taxonomies + nodes                          |
|    - Classifications on work items               |
|    - Role bindings + constraint diagnostics      |
+--------------------------------------------------+
|  DITS v2 core (unchanged)                        |
|    - Event DAG, reducer, sync, blob store        |
|    - Identity, signatures, meta config           |
+--------------------------------------------------+
```

Each layer below is reusable independently of the layers above. A future
methodology that is not RE can sit alongside the RE pack and share all the
generic primitives. A different UI (TUI, mobile app) can render the same
substrate without going through the web layer.

---

## 5. Generalization framework

This section is the design lens for §6. RE introduces several structural
needs (two parallel hierarchies, role assignments with independence
constraints, ACK as structured commitment); the lens is to extract the
**general primitive** behind each need and add that to DITS core, then let
RE consume it.

### 5.1 Taxonomies (the dual-hierarchy generalization)

The RE PRD describes one explicit hierarchy (org → node → team) and one
adjacent hierarchy not formally called out (product_group → product → node
→ features). These are two **independent classification axes**. A milestone
exists at the intersection: a particular team works on a particular feature.

**Generalization:** DITS gains a first-class concept of a **Taxonomy** — a
named, hierarchical classification namespace. Each project may declare any
number of taxonomies. RE happens to declare two:

| Taxonomy slug | Levels (advisory) | Example node path |
|---|---|---|
| `org` | org → node → team | `acme/platform/payments-team` |
| `product` | product_group → product → node → features | `commerce/checkout/payments/recurring-billing` |

A future project might declare `geography`, `compliance_scope`, `customer_segment`,
or anything else. DITS makes no commitments about what taxonomies mean —
it only commits to *storing them, versioning them, and letting work items
be classified against them*.

Goal hierarchies are **not** taxonomies. Goals have lifecycle, status, and
outcomes — they are work items linked via `parent_of`/`child_of`.
Taxonomies are structural classifiers; their nodes do not themselves have
status or attempts.

### 5.2 Classifications

A **Classification** is the link between a work item and a taxonomy node.
Work items may carry many classifications, across many taxonomies. This is
the substrate primitive that lets an RE Milestone declare both "owned by
the payments team" and "contributes to the recurring-billing feature"
without overloading labels or relations.

### 5.3 Roles and role bindings

A **Role** is a named function (Specifier, Builder, Pilot, Reviewer,
Reporter, On-call, …) that an actor may hold **with respect to a specific
work item**. Roles are declared in meta. A **role binding** is an event
that assigns an actor to a role on a work item; another event revokes it.

Roles generalize the existing `work.assigned` event. `work.assigned` becomes
a degenerate case (the implicit "owner" role); explicit role assignments
sit alongside it.

### 5.4 Role constraints

A **Role Constraint** is a meta-declared predicate over the role bindings
of a work item and the positions of the bound actors in declared
taxonomies. RE happens to declare two:

- **No role collapse:** the same actor may not hold Specifier, Builder,
  and Pilot simultaneously on a single milestone.
- **Pilot independence:** the Pilot's actor must not report to the
  Specifier's or Builder's actor within N levels in the `org` taxonomy.

Both are expressible as constraint predicates parameterized over role
slugs, taxonomy slugs, and integer thresholds. The constraint engine
**produces diagnostics**; it does not block events. This matches RE's
"flag, do not prevent" rule and DITS's "advisory, do not block during
sync" rule simultaneously.

### 5.5 Why this generalization holds water

The four extensions above (taxonomies, classifications, role bindings,
role constraints) are independently useful for any coordination project,
not only RE. Examples of consumers other than RE:

- A purely-agent project using DITS could declare a `capability` taxonomy
  and classify executions by required capability, then route based on
  classification.
- A research project could declare a `domain` taxonomy and a `reviewer`
  role with a constraint that requires the reviewer to be classified
  within the same domain as the work item.
- An open-source maintainership project could declare `module_owner` roles
  with constraints that require ownership of all classified modules.

If DITS shipped these primitives RE-shaped (e.g., a hard-coded `org`
hierarchy and a hard-coded `pilot` role), all of those use cases would
have to fight the schema. Shipped generic, they each consume the same four
primitives differently.

---

## 6. Data model additions

### 6.1 Meta configuration extensions

Three new top-level fields on `MetaConfig` (spec §7.1):

```go
type MetaConfig struct {
    // ... existing fields ...
    Taxonomies       []Taxonomy        `json:"taxonomies"`
    Roles            []Role            `json:"roles"`
    RoleConstraints  []RoleConstraint  `json:"role_constraints"`
}

type Taxonomy struct {
    Slug        string   `json:"slug"`         // e.g., "org"
    Name        string   `json:"name"`         // "Organization"
    Description string   `json:"description,omitempty"`
    Levels      []string `json:"levels,omitempty"` // advisory naming: ["org", "node", "team"]
    Nodes       []TaxonomyNode `json:"nodes"`
}

type TaxonomyNode struct {
    Slug         string          `json:"slug"`          // path-like, stable: "acme/platform/payments-team"
    Name         string          `json:"name"`          // "Payments Team"
    ParentSlug   string          `json:"parent_slug,omitempty"`
    Metadata     json.RawMessage `json:"metadata,omitempty"`
    Retired      bool            `json:"retired,omitempty"`
}

type Role struct {
    Slug        string `json:"slug"`         // "pilot"
    Name        string `json:"name"`         // "Pilot"
    Description string `json:"description,omitempty"`
    Cardinality string `json:"cardinality"`  // "exactly_one" | "at_most_one" | "many"
}

type RoleConstraint struct {
    Slug      string         `json:"slug"`      // "pilot_independence"
    Name      string         `json:"name"`
    Predicate string         `json:"predicate"` // see §6.2
    Args      map[string]any `json:"args"`
    Severity  string         `json:"severity"`  // "warning" | "violation"
}
```

Taxonomy nodes are versioned with meta (highest-version-wins on sync, per
existing spec §7.3). Node renames, moves, and retirements are meta
mutations and produce a new meta version. This gives full audit history of
org restructures via the meta version log.

> **Open question:** if node churn is high, the full-replacement meta
> sync model gets noisy. A possible later optimization is event-sourcing
> taxonomy nodes as `kind=taxonomy_node` work items. The initial proposal
> uses meta to keep scope tight. Revisit if real-world usage shows meta
> churn becoming the bottleneck.

### 6.2 Constraint predicates

A small fixed set of predicates ships with DITS; methodology packs may
contribute more. Initial set:

| Predicate slug | Args | Meaning |
|---|---|---|
| `distinct_actors` | `{roles: [slug, slug, ...]}` | The actors bound to all listed roles must be distinct. |
| `not_reports_to_within` | `{role: slug, other_roles: [slug, ...], taxonomy: slug, max_levels: int}` | The actor bound to `role` must not report to the actors bound to `other_roles` within `max_levels` in `taxonomy`. |
| `classified_in_same_node` | `{taxonomy: slug, role: slug, level: int}` | The actor bound to `role` must be classified in the same taxonomy node (at `level`) as the work item. |
| `requires_classification` | `{taxonomy: slug}` | The work item must carry at least one classification in `taxonomy`. |

The predicate evaluator runs against the materialized work item, current
meta config (for taxonomy structure), and the set of role bindings. It
produces a list of diagnostics: `{constraint_slug, severity, message}`.
Diagnostics are surfaced in the UI and exposed via the query API; they
never reject events.

### 6.3 New event types (DITS core)

Four new event types, generic across methodologies:

| Event | Payload | Description |
|---|---|---|
| `work.classified` | `{taxonomy_slug, node_slug}` | Classifies the work item in a taxonomy node. |
| `work.declassified` | `{taxonomy_slug, node_slug}` | Removes a classification. |
| `work.role_bound` | `{role_slug, actor_id, scope?}` | Binds an actor to a role on the work item. |
| `work.role_unbound` | `{role_slug, actor_id}` | Removes a role binding. |

Schema validation (spec §3.4 tier 1) enforces:
- `taxonomy_slug` exists in meta and `node_slug` exists within it (and is not retired)
- `role_slug` exists in meta
- Role cardinality is respected at the materialized level (e.g., `exactly_one`
  emits a constraint violation if more than one binding remains active)

Cardinality enforcement is **advisory diagnostic**, consistent with the
attribution-not-prevention rule. Schema validation only rejects malformed
references, not policy violations.

### 6.4 New work kinds (RE pack)

Added via meta, not core:

| Kind | Workflow | Notes |
|---|---|---|
| `rfc` | `rfc` | Lifecycle: `draft → leadership_review → approved → backlog → resourced → rejected` |
| `milestone` | `milestone` | Lifecycle: `draft → ack_filed → ack_committed → in_flight → shipped → aborted` |
| `goal` | `goal` | Lifecycle: `proposed → committed → achieved → abandoned` |
| `decision_block` | `decision_block` | Lifecycle: `open → resolved → escalated` |
| `outcome_assessment` | `outcome_assessment` | Lifecycle: `pending → completed → overdue` |

Each ships as a `Workflow` in default meta for projects that adopt the RE
pack. None of these become DITS-core defaults.

### 6.5 ACK as event convention (RE pack)

ACK is structured enough to deserve dedicated events. They are RE-specific
and live in the RE pack, not DITS core:

| Event | Payload |
|---|---|
| `work.ack_filed` | `{ack_id, scope_summary, delivery_timing, target_outcome, primary_goal_ref, acceptance_criteria}` |
| `work.ack_committed` | `{ack_id, comment?}` |
| `work.ack_amended` | `{ack_id, amendment_id, amendment_type, fields, reason}` — `amendment_type ∈ {scope_change, timeline_change, target_change, clarification}` |
| `work.ack_cleared` | `{ack_id, reason}` — emitted automatically when a material amendment lands; mirrors `work.plan_rejected` |
| `work.ack_recommitted` | `{ack_id, comment?}` |

The materialized view exposes an `ACK` on a Milestone with `state ∈
{filed, committed, drifting, invalid, cleared}` and a list of amendments
with full attribution. Conventions enforced in the reducer (RE-pack
reducer extension, not core):

- A `scope_change`, `timeline_change`, or `target_change` amendment
  emitted after `ack_committed` automatically triggers `ack_cleared`,
  setting state back to `filed`.
- A `target_change` amendment additionally emits `work.review_requested`
  with `reviewer` resolved via the Leadership role binding.
- A `clarification` amendment does not clear commitment.

These are **layered behaviors** implemented in the RE pack's operator
(thin shim around `workops`), not in DITS core. DITS core does not know
what an ACK is.

### 6.6 PilotLog conventions (RE pack)

No new event types. PilotLog rides on existing DITS events with a
`pilot_log_entry_type` field in the payload:

| PilotLog entry type | DITS event |
|---|---|
| `observation` | `work.observation_recorded` |
| `risk` | `work.finding_recorded` with `pilot_log_entry_type: risk` |
| `stress_test` | `work.eval_completed` with `pilot_log_entry_type: stress_test` |
| `decision` | `work.decision_recorded` |
| `integrity_call` | `work.observation_recorded` with `pilot_log_entry_type: integrity_call` and `ack_integrity ∈ {intact, drifting, invalid}` |

The Risk Register, Pilot's Log tab, and Integrity Dashboard are queries
over these events with the `pilot_log_entry_type` filter.

### 6.7 DecisionBlock (RE pack)

A `kind=decision_block` work item. Opened by emitting `work.created` and
linked to its parent Milestone with `relates_to`. Resolution emits
`work.closed` with a `resolution` reason. Auto-escalation after N days
idle (default 5, per-team configurable) emits `work.review_requested`
targeting Leadership. Performed by the RE scheduler (§8.3).

### 6.8 OutcomeAssessment (RE pack)

A `kind=outcome_assessment` work item, auto-created by the scheduler when
a Milestone transitions to `shipped`. Carries SLA (default 90d). The
Specifier records the assessment as `work.eval_completed` with a
`{result, value}` verdict. The "landed but didn't matter" pattern is the
verdict `{result: achieved, value: low}` — surfaced in the Leadership
pattern blocks.

---

## 7. Identity and access

### 7.1 Trust model

DITS today: each actor is an Ed25519 keypair stored locally in `.dits/`.
Every event is signed by the local key. The server verifies signatures
in warn mode. The actor identity is durable and self-sovereign.

RE introduces three new requirements:
1. Humans clicking buttons in a browser need to mutate state.
2. Mutations must remain attributable to a real actor identity.
3. External customers need read-only access to a projection.

### 7.2 OAuth-bridged custodial signing

The proposed identity architecture for the web UI:

```
                      +----------------------+
   browser  ---->     |  dits-server (web)   |  ---->  signed event
                      |                      |
                      |   OAuth (Google,     |
                      |   GitHub, OIDC)      |
                      |        |             |
                      |        v             |
                      |   user_id   <----->  actor_id  <----->  Ed25519 keypair
                      |                          (KMS-wrapped)
                      +----------------------+
```

Flow:
1. User signs in via OAuth provider (Google / GitHub / generic OIDC). The
   server maps the OAuth subject to an `actor_id`.
2. On first sign-in, the server generates an Ed25519 keypair for that
   user, stores the private key wrapped by KMS (or sealed by a
   server-side key), and registers the public key in the existing
   `actors` table.
3. On every mutation, the server unwraps the user's key, signs the
   event with the same canonical-JSON scheme DITS already uses, and
   appends it to the local event log.
4. CLI/TUI users continue to hold their own keys. Their events are
   indistinguishable from server-signed events at the protocol level —
   same canonical JSON, same signature scheme, same actor registration.

Key properties:
- **Signed-event invariant preserved.** Every event in the DAG still
  carries a real Ed25519 signature verifiable against the public key in
  `actors`.
- **Attribution is real.** OAuth identity is bound to actor identity at
  registration; subsequent events carry the actor ID.
- **Custodial trust is honest.** The server holds the wrapped key. If
  the server is compromised, that user's events could be forged. This is
  the same trust assumption every web app makes; RE does not need
  cryptographic non-repudiation, it needs attribution.
- **CLI parity.** A user who wants self-sovereign keys can install the
  CLI, generate their own keypair, and use the same OAuth-bound actor
  identity by uploading their public key during account setup (the
  inverse of the custodial flow).

### 7.3 Public projection (Customer Roadmap)

The Customer Roadmap is a **read-only filtered query** with no
authentication. It exposes only:

- Milestones with `customer_visible=true` and `state=ack_committed` or
  later.
- For each, the latest committed ACK's `scope_summary`, `delivery_timing`,
  and the names of `primary_goal_ref` chains.

Everything else (RYG, indicators, PilotLog, risks, internal commentary,
amendments, evals) is filtered out at the query layer. This is a pure
projection over the substrate, served at e.g. `GET /roadmap`. No new
storage; no new ACL.

### 7.4 Role assignment is not access control

A user authenticated as actor X may emit events on any work item;
DITS does not gate by role. Role bindings determine **diagnostics** (does
this milestone have a Pilot? does the Pilot satisfy independence?), not
**permissions**. The UI may grey-out actions for users not bound to a
relevant role on a given milestone, but the substrate does not enforce
it. This is consistent with RE's "off-Pilot status edits are recorded,
not blocked."

---

## 8. UI architecture

### 8.1 Surface inventory

Mapping RE PRD §UI to concrete routes/views, served from `dits-server`:

| RE view | Route | Backed by |
|---|---|---|
| Leadership Portfolio (default) | `/` | Indicator projections + query API |
| Milestone page | `/m/:shared_id` | `GET /api/v2/work/:id` + classifications + role bindings |
| Specifier workspace | `/me/specifier` | Filter on role bindings + lifecycle state |
| Builder workspace | `/me/builder` | Same |
| Pilot workspace | `/me/pilot` | Indicator history + freshness queue + log entries |
| Customer Roadmap | `/roadmap` | Public filtered projection |
| RFC review queue | `/queues/rfc-review` | Filter on `kind=rfc`, `status=leadership_review` |
| Adjudication queue | `/queues/adjudication` | Open `work.review_requested` targeting Leadership |
| Backlog | `/backlog` | `kind=rfc`, `status=backlog`, sortable |

### 8.2 Implementation shape

Two viable approaches:

**A. Server-rendered HTML routes added to `dits-server`.** Simpler to
build, no separate frontend toolchain. Templating via Go's `html/template`.
Interactivity via HTMX or minimal JS. Best for v1: the views are
read-heavy, mutations are well-bounded, and shipping a single binary
matches DITS's deployment story.

**B. Separate SPA against the existing v2 JSON API.** More client-side
interactivity, more flexibility for future features, but adds a build
pipeline and a deployment target. Defer to v2 unless v1 routes prove
inadequate.

The proposal endorses **A** for v1. The HTML routes are additive to the
existing JSON API; the SPA path remains open.

### 8.3 The TUI is not abandoned

`cmd/dits-tui` from the earlier discussion remains useful for a specific
audience: the Pilot's daily driving loop (browse freshness queue, file
log entries, call integrity, scan adjudication queue). It's a peer to
the web UI, not a replacement. Both consume the same `workops` and query
API. Ship it after the web UI proves the underlying primitives are right.

---

## 9. Derived projections

All of the following are pure functions over the event log; none require
new tables beyond optional caches.

### 9.1 Four leading indicators (Pilot dashboard, Leadership view)

| Indicator | Definition |
|---|---|
| Dependency closure rate | Among `work.linked` `depends_on` edges on shipped milestones in window, fraction closed before downstream ship. |
| Scope-change velocity | Count of `work.ack_amended` with `amendment_type=scope_change` per milestone per week. |
| Decision friction | Median time from `work.created` (kind=rfc) to `work.status_set(to=approved)`; median time from `decision_block` open to `closed` or `escalated`. |
| ACK-to-start latency | Median time from `work.ack_committed` to first `work.execution_started` event on the milestone. |

Computed client-side or by a cache table refreshed on `AppendAndMaterialize`.

### 9.2 Freshness tiers

Per-entity-type thresholds (configurable per team via meta):

| Entity | Fresh | Aging | Stale |
|---|---|---|---|
| Milestone status (in_flight) | <7d | 7–14d | >14d |
| ACK (committed) | <14d | 14–30d | >30d |
| PilotLog (per-milestone) | <7d | 7–14d | >14d |
| Open Risk | <14d | 14–30d | >30d |
| CrossTeamDependency | until `required_by` | within 7d of `required_by` | past `required_by` |

Computed from the latest relevant event timestamp.

### 9.3 Goal health rollup

For each `kind=goal`, traverse `parent_of`/`child_of` to leaf milestones,
collect their RYG label, and apply worst-of-children. Goals at the same
level aggregate similarly.

### 9.4 Separability diagnostic

Run the role-constraint predicates on every milestone with at least one
of {Specifier, Builder, Pilot} bound. Surface diagnostics with severity
on the milestone page header and in a Leadership pattern block.

---

## 10. External services

Two thin processes that DITS does not currently have:

### 10.1 RE scheduler

A small daemon that polls the materialized state for time-based triggers:

- DecisionBlocks open for >N days → emit `work.review_requested` targeting
  Leadership (auto-escalation).
- Milestones transitioned to `shipped` → emit `work.created` for the
  paired `kind=outcome_assessment` if not already present.
- OutcomeAssessments past SLA → set status `overdue` and surface in
  Leadership pattern blocks.

Implemented as a goroutine in `dits-server` (or a separate `dits-re-scheduler`
binary). All actions emit normal events through `workops`; nothing escapes
the audit trail.

### 10.2 Notifications (out of scope for v1)

Email / Slack / webhook delivery is left to a follow-on. The substrate's
"needs me" queries (adjudication, awaiting commit, integrity call due)
are surfaced in-app via the inbox view. Push channels can be added later
by subscribing to the event stream.

---

## 11. Phasing

Each phase is independently shippable. Adopting RE does not require
finishing every phase.

### Phase 0: substrate readiness check (~1 week)

- Inventory `workops` mutation surface against RE needs.
- Confirm `internal/server` v2 query API can express portfolio queries
  with reasonable indexes.
- No code changes; deliverable is a gap report.

### Phase 1: generic substrate extensions (~3–4 weeks)

- Add `Taxonomy`, `Role`, `RoleConstraint` to `MetaConfig`.
- Add `work.classified`, `work.declassified`, `work.role_bound`,
  `work.role_unbound` events with schema validation.
- Implement constraint predicate evaluator with the four initial
  predicates.
- Tests in `internal/workops` per the established discipline; pin the
  contract independently of any UI.

### Phase 2: identity bridge (~2–3 weeks)

- OAuth (OIDC) flow in `dits-server`.
- Custodial keypair generation and KMS-wrapped storage.
- Server-side signing path that produces protocol-identical events to
  CLI-signed ones.
- Per-user session model.

### Phase 3: RE pack (~3–4 weeks)

- RE-specific work kinds, workflows, ACK events, PilotLog conventions.
- Reducer extension for ACK state (`filed → committed → drifting →
  invalid → cleared`).
- RE scheduler for escalations and outcome assessments.
- Default meta config bundle for RE adoption.

### Phase 4: web UI (~4–6 weeks)

- HTML routes added to `dits-server`.
- Leadership portfolio default landing.
- Milestone page.
- Role workspaces.
- Customer roadmap.
- Indicator projections rendered.

### Phase 5: Pilot TUI (~2–3 weeks, optional)

- `cmd/dits-tui` covering the Pilot's daily loop.
- Reuses `workops` and v2 query API; no special access.

Total: roughly 15–22 engineer-weeks for a complete end-to-end shipment,
front-loaded such that Phase 1 is reusable by non-RE projects on its own.

---

## 12. Open questions

1. **Custodial signing UX corner cases.** If a user signs in from two
   browsers concurrently, both sessions sign with the same key — fine.
   But if the user revokes OAuth access, the actor_id remains in the
   event log. Do we retire the actor (mark as `inactive` in `actors`
   table) without invalidating prior signatures? Probably yes.

2. **Meta sync churn from taxonomy nodes.** If a 1500-person org has
   hundreds of taxonomy node mutations a month, meta versioning may get
   noisy. Defer: model nodes as work items in v2 if real-world usage
   demands it. The migration path is straightforward (one-shot event
   emission per node).

3. **Constraint predicate language extensibility.** Initial predicates
   ship hard-coded in Go. A future need is methodology packs declaring
   their own predicates. Options: (a) Lua/Starlark scripting, (b)
   compile-time pack registration via build tags, (c) WASM modules.
   Pick when a second pack appears; v1 hard-codes the four predicates.

4. **Goal hierarchy semantics under RE.** RE's goal rollup uses
   worst-of-children. DITS has no first-class "rollup" — it's a UI-level
   computation. If RE wants other rollups later (best-of, average,
   weighted), they're additive; nothing in the data model needs to
   change.

5. **Customer roadmap caching.** Public reads at scale may want a CDN
   layer. Trivial once the projection endpoint exists; not a v1 concern.

6. **Two-frontend ergonomics.** Both the web UI and the TUI need
   consistent labels, color choices, and shortcut conventions. Worth a
   small style guide before Phase 4 and 5 ship.

---

## 13. Success criteria

Borrowed and adapted from RE PRD v0.2 §Success Criteria; each must pass
for the substrate-plus-pack-plus-UI stack to be considered complete.

1. A methodology-familiar reviewer can complete the full lifecycle (RFC →
   Leadership approval → backlog → resourced → milestone → ACK committed
   → in-flight → shipped) in under 15 minutes using only the web UI.
2. Scope renegotiation produces a `work.ack_amended` event of type
   `scope_change`, auto-clears commitment, increments scope-change
   velocity, and updates the indicator panel within one poll cycle.
3. A Pilot abort decision marks the milestone aborted as a successful
   outcome, with no `outcome_assessment` auto-created.
4. The Leadership portfolio view at `/` shows the four indicators, goal
   health rollup, adjudication queue, and pattern blocks — with no
   list-of-projects ever rendered.
5. Separability diagnostic flags at least one collapse case in a seeded
   test corpus.
6. The customer roadmap at `/roadmap` returns scoped data only — no
   RYG, no indicators, no internal commentary — and is accessible without
   authentication.
7. An off-Pilot RYG edit is recorded with attribution and surfaced in the
   activity feed visible to the bound Pilot.
8. The same product works (with seed data of appropriate scale) at both
   3-person and 1500-person project sizes; performance budget: portfolio
   landing page <500ms on the 1500-person corpus.
9. **DITS-specific:** every event emitted by RE flows (ACK file, ACK
   commit, amendment, classification, role binding) is a normal signed
   event on the DAG, indistinguishable from a CLI-emitted event at the
   protocol layer.
10. **DITS-specific:** disabling the RE pack and starting fresh leaves
    the substrate fully functional for non-RE projects, with the
    Phase 1 extensions still available to any other consumer.

---

## 14. Summary

RE's methodology and DITS's substrate share enough deep assumptions that
the integration is best framed as **a layered stack**, not a port. DITS
core gains four small generic primitives (taxonomies, classifications,
role bindings, role-constraint diagnostics) that no methodology owns.
The RE pack contributes the methodology-specific event conventions, the
reducer logic for ACK lifecycle, and the scheduler for time-based
events. A new web UI rendered by `dits-server` projects the substrate
into the views the RE PRD requires, with an OAuth-bridged custodial
identity layer that keeps every mutation signed and attributable. The
existing CLI, MCP server, and a future TUI remain peer frontends with
no special privileges.

If this proposal is accepted in spirit, the next concrete artifacts are:
(a) Phase 0's gap report, and (b) a precise specification of the four
constraint predicates and their evaluator semantics. Everything else
follows.
