# Use Cases

This document shows how DITS is used in practice across three audiences: agent builders integrating with a harness, humans using the CLI for everyday work, and platform operators running the infrastructure.

---

## 1. Agent Harness Integration

### Single-Agent Execution Loop

The core agent pattern: claim work, execute it, record progress, assess the result.

```bash
# Agent polls for ready work
curl http://localhost:8484/api/v2/work?ready=true&kind=execution

# Agent claims a work item
dits work lease PROJ-42

# Agent starts an execution attempt
dits work start PROJ-42

# Agent records progress as it works
dits work checkpoint PROJ-42 --summary "Downloaded dependencies" --progress 0.25
dits work checkpoint PROJ-42 --summary "Built project" --progress 0.5
dits work checkpoint PROJ-42 --summary "Tests passed" --progress 0.75

# Agent attaches output artifacts
dits work attach PROJ-42 build-log.txt

# Agent completes the attempt
dits work complete PROJ-42 --summary "Build and test succeeded"

# Agent requests a machine eval of its own output
dits work eval-request PROJ-42 --scope "build quality" --subject-kind artifact --subject sha256:abc123
# Judge agent (or same agent) completes the eval
dits work eval-complete PROJ-42 --eval-id evl_01... --verdict pass --summary "All checks green"

# Release the lease
dits work lease-release PROJ-42
```

### Autonomous Retry Loop

When an attempt fails with `retryable=true`, the agent can try again with a different strategy.

```bash
# First attempt fails
dits work start PROJ-42
dits work checkpoint PROJ-42 --summary "Trying approach A" --progress 0.3
dits work fail PROJ-42 --error "Timeout on database connection" --retryable

# Agent evaluates the failure
dits work eval-request PROJ-42 --scope "failure analysis" --subject-kind attempt
dits work eval-complete PROJ-42 --eval-id evl_02... --verdict fail \
  --summary "Approach A not viable, try connection pooling"

# Discard the failed attempt's output
dits work discard PROJ-42 --subject atp_first --subject-kind attempt --reason "Approach A not viable"

# Second attempt with different strategy
dits work start PROJ-42
dits work checkpoint PROJ-42 --summary "Trying approach B with connection pooling" --progress 0.5
dits work checkpoint PROJ-42 --summary "All queries succeeded" --progress 1.0
dits work complete PROJ-42 --summary "Approach B succeeded"

# Retain the successful attempt's output
dits work retain PROJ-42 --subject atp_second --subject-kind attempt \
  --reason "Approach B passed eval" --eval-ref evl_02...
```

The event history preserves both attempts. The reducer marks the first attempt as completed (failed) and the second as completed (succeeded). Both are queryable:

```bash
curl http://localhost:8484/api/v2/work/PROJ-42/attempts
curl http://localhost:8484/api/v2/work/PROJ-42/evals
```

### Multi-Agent Coordination

Three agents coordinate through the substrate: one investigates, one executes, one evaluates.

```bash
# Agent A: create and investigate
dits work create --title "Diagnose production latency spike" --kind investigation
dits work lease PROJ-43
dits work observe PROJ-43 --summary "P99 latency increased 3x at 14:00"
dits work observe PROJ-43 --summary "Correlates with deploy of service-auth v2.3"
dits work finding PROJ-43 --statement "service-auth v2.3 introduced N+1 query" --confidence 0.85
dits work lease-release PROJ-43

# Agent A hands off to Agent B for remediation
dits work create --title "Fix N+1 query in service-auth" --kind execution
dits work link PROJ-44 depends_on PROJ-43
dits work handoff PROJ-44 --to actor_agent_b --context "See finding in PROJ-43"

# Agent B: lease, execute, produce a patch
dits work lease PROJ-44
dits work start PROJ-44
dits work attach PROJ-44 fix.patch
dits work complete PROJ-44 --summary "Patch applied, query batched"
dits work lease-release PROJ-44

# Agent C: evaluate the fix
dits work eval-request PROJ-44 --scope "performance regression check" --subject-kind artifact
dits work eval-complete PROJ-44 --eval-id evl_03... --verdict pass \
  --summary "P99 latency back to baseline"
```

### Evidence Collection with Provenance

Agents attach artifacts with type, role, and producer metadata:

```bash
# Attach a trace file produced by a profiler tool
dits work attach PROJ-43 trace.json
# The artifact carries: artifact_type=trace, semantic_role=evidence, produced_by={tool: "profiler"}

# Record a finding backed by evidence
dits work finding PROJ-43 \
  --statement "Memory leak in batch processor" \
  --confidence 0.9
```

Query artifacts by type and role:

```bash
curl http://localhost:8484/api/v2/work/PROJ-43/artifacts?type=trace&role=evidence
```

### Artifact Selection via Eval + Outcome

An agent generates multiple candidate artifacts, evaluates each, and retains the best one:

```bash
# Agent generates two candidate reports
dits work attach PROJ-50 report-v1.md
dits work attach PROJ-50 report-v2.md

# Eval each artifact
dits work eval-request PROJ-50 --scope "report quality" --subject-kind artifact --subject sha256:aaa
dits work eval-complete PROJ-50 --eval-id evl_10... --verdict partial \
  --subject-kind artifact --subject sha256:aaa \
  --summary "Missing conclusions section" --metrics '{"completeness": 0.6}'

dits work eval-request PROJ-50 --scope "report quality" --subject-kind artifact --subject sha256:bbb
dits work eval-complete PROJ-50 --eval-id evl_11... --verdict pass \
  --subject-kind artifact --subject sha256:bbb \
  --summary "Complete and well-structured" --metrics '{"completeness": 0.95}'

# Retain the better artifact, discard the other
dits work discard PROJ-50 --subject sha256:aaa --subject-kind artifact --reason "Incomplete"
dits work retain PROJ-50 --subject sha256:bbb --subject-kind artifact \
  --reason "Passed eval" --eval-ref evl_11...

# Check outcomes
curl http://localhost:8484/api/v2/work/PROJ-50/outcomes?decision=retained
```

---

## 2. Human Workflows

### Issue Tracking

DITS supports familiar issue tracking as a projection over the coordination substrate.

```bash
# Create issues (shorthand for work items with kind=issue)
dits issue create --title "Login fails on Safari" --body "500 error on POST /auth"
dits issue create --title "Add dark mode support" --label feature

# Triage
dits issue list
dits issue show PROJ-1
dits issue status PROJ-1 in_progress
dits issue assign PROJ-1 actor_alice

# Comment and close
dits issue comment PROJ-1 --body "Fixed in commit abc123"
dits issue close PROJ-1

# Reopen if needed
dits issue reopen PROJ-1
```

### Investigation Workflow

For structured research — observations build evidence, findings capture conclusions.

```bash
dits work create --title "Why is the staging DB slow?" --kind investigation

# Record what you observe
dits work observe PROJ-5 --summary "Query time 10x normal since Tuesday deploy"
dits work observe PROJ-5 --summary "Only affects queries touching users table"
dits work observe PROJ-5 --summary "EXPLAIN shows sequential scan instead of index"

# Record structured conclusions
dits work finding PROJ-5 \
  --statement "Missing index on users.email after migration 042" \
  --confidence 0.95

# Attach supporting evidence
dits work attach PROJ-5 explain-output.txt

dits work close PROJ-5
```

### Plan-Decide-Execute

Propose a plan, get human review, then execute.

```bash
# Create a plan work item
dits work create --title "Q3 infrastructure migration" --kind plan
dits work plan PROJ-6 --summary "Three-phase migration" \
  --plan "Phase 1: canary (1 week). Phase 2: 50% rollout (1 week). Phase 3: full rollout."

# Get human review
dits work review PROJ-6 --scope "Migration plan review"
# Reviewer approves (via event or comment)
dits work comment PROJ-6 --body "Approved. Proceed with Phase 1."

# Create execution work items for each phase
dits work create --title "Phase 1: canary deploy" --kind execution
dits work link PROJ-7 depends_on PROJ-6
```

### Code Review Flow

```bash
dits work create --title "Review auth refactor" --kind task
dits work attach PROJ-8 auth-refactor.patch
dits work review PROJ-8 --scope "Code quality and security review"

# Reviewer completes
dits work comment PROJ-8 --body "LGTM, one minor nit on error handling"
dits work close PROJ-8
```

---

## 3. Mixed Human-Agent Workflows

### Human Creates, Agent Executes, Human Reviews

```bash
# Human creates the task
dits work create --title "Generate API documentation" --kind execution

# Agent picks it up
# (agent polls: GET /api/v2/work?ready=true&kind=execution)
dits work lease PROJ-10
dits work start PROJ-10
dits work checkpoint PROJ-10 --summary "Parsed 47 endpoints" --progress 0.5
dits work attach PROJ-10 api-docs.md
dits work complete PROJ-10 --summary "Documentation generated for all endpoints"
dits work lease-release PROJ-10

# Human reviews the output
dits work review PROJ-10 --scope "Documentation accuracy and completeness"
dits work comment PROJ-10 --body "Looks good, merging."
dits work close PROJ-10
```

### Agent Proposes, Human Decides

```bash
# Agent creates and proposes a plan
dits work create --title "Optimize database queries" --kind plan
dits work plan PROJ-11 --summary "Batch N+1 queries in user service" \
  --plan "Add DataLoader pattern to user resolver. Expected 60% reduction in DB calls."

# Human reviews and decides
dits work comment PROJ-11 --body "Approved. Also add caching for the hot path."
dits work close PROJ-11

# Agent creates execution work item from the approved plan
dits work create --title "Implement DataLoader + caching" --kind execution
dits work link PROJ-12 depends_on PROJ-11
```

### Eval Escalates to Human Review

```bash
# Agent executes and self-evaluates
dits work start PROJ-13
dits work complete PROJ-13 --summary "Refactored auth module"
dits work eval-request PROJ-13 --scope "refactor quality" --subject-kind attempt
dits work eval-complete PROJ-13 --eval-id evl_04... --verdict fail \
  --summary "Test coverage dropped from 85% to 72%"

# Eval failed — escalate to human
dits work block PROJ-13 --reason "Eval failed: test coverage regression. Needs human review."
dits work review PROJ-13 --scope "Review coverage regression in auth refactor"

# Human reviews and decides whether to accept or retry
dits work comment PROJ-13 --body "Coverage drop is acceptable — removed dead code. Unblocking."
dits work unblock PROJ-13
```

---

## 4. Platform Operations

### Server Deployment

```bash
# Start the server
dits-server --project MYPROJ --addr :8484 --db /data/dits-server.db --blobs /data/blobs

# Health check
curl http://localhost:8484/api/v1/health
```

The server handles sync, blob storage, shared ID allocation, and query API. It is not the source of truth — clients hold full event history and can work offline.

### Multi-Client Sync

```bash
# Client A: initialize and work offline
mkdir client-a && cd client-a
dits init --project MYPROJ
dits work create --title "Task from A"
dits remote set http://server:8484
dits sync

# Client B: initialize and pull A's work
mkdir client-b && cd client-b
dits init --project MYPROJ
dits remote set http://server:8484
dits sync
dits work list  # sees A's task

# Both clients can work offline and sync when ready
# Concurrent edits converge deterministically via causal ordering
```

### Meta Configuration Evolution

```bash
# Add custom labels
dits meta label add --slug security --name Security --color "#ff0000"
dits meta label add --slug performance --name Performance --color "#00ff00"

# Add a custom work kind
dits meta type add --slug spike --name "Spike" --workflow default

# View current configuration
dits meta show

# Changes are versioned and sync bidirectionally
# Highest meta version wins during sync
dits sync
```

### Identity Management

```bash
# View your actor identity
dits identity show
# Actor ID:   actor_7b865c83b559e82d
# Public Key:  e8748f...
# Node ID:    node_01KNFY...

# Every event is signed with your Ed25519 key
# Server verifies signatures in warn mode (logs but doesn't reject)
```

---

## 5. API Integration Patterns

### Polling for Ready Work

Agents poll for unclaimed, unblocked, open work items:

```bash
# All ready work
curl http://localhost:8484/api/v2/work?ready=true

# Ready execution work only
curl http://localhost:8484/api/v2/work?ready=true&kind=execution

# Work claimed by a specific agent
curl http://localhost:8484/api/v2/work?claimed_by=actor_agent1

# Blocked work items
curl http://localhost:8484/api/v2/work?blocked=true
```

`ready=true` means: status in an open category, no active lease, and not blocked.

### Event Streaming per Work Item

Track progress on a specific work item by polling its event stream:

```bash
# All events
curl http://localhost:8484/api/v2/work/PROJ-42/events

# Only checkpoints since last poll
curl "http://localhost:8484/api/v2/work/PROJ-42/events?type=work.checkpointed&since=2026-04-01T12:00:00Z"

# Only execution events
curl "http://localhost:8484/api/v2/work/PROJ-42/events?type=work.execution_completed"
```

### Eval Dashboards

Query eval results across work items for monitoring and reporting:

```bash
# All evals for a work item
curl http://localhost:8484/api/v2/work/PROJ-42/evals

# Only failed evals
curl http://localhost:8484/api/v2/work/PROJ-42/evals?verdict=fail

# Evals on artifacts specifically
curl http://localhost:8484/api/v2/work/PROJ-42/evals?subject_kind=artifact
```

### Cross-Item Event Monitoring

Monitor activity across all work items:

```bash
# Recent failures
curl "http://localhost:8484/api/v2/events?type=work.execution_failed&limit=20"

# All activity by a specific agent
curl "http://localhost:8484/api/v2/events?actor_id=actor_agent1&limit=50"

# All work.created events in the last hour
curl "http://localhost:8484/api/v2/events?type=work.created&since=2026-04-01T11:00:00Z"
```

### Artifact Queries

```bash
# Artifacts for a work item, filtered by type and role
curl http://localhost:8484/api/v2/work/PROJ-42/artifacts?type=log
curl http://localhost:8484/api/v2/work/PROJ-42/artifacts?role=evidence

# Download a specific artifact blob
curl http://localhost:8484/api/v1/blobs/sha256:abc123 -o output.txt
```
