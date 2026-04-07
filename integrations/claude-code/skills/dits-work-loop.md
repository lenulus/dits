---
name: dits-work-loop
description: Canonical DITS work loop for an agent claiming ready work. TRIGGER when the user says "pick up DITS work", "work the queue", "claim a DITS task", or points you at a specific work item ID.
---

# DITS work loop

Use this loop whenever you are asked to pick up, work, or drive a DITS
work item to completion. Every step maps to a single MCP tool call.

## The loop

1. **Discover work.** `dits_work_list ready=true` to see unleased,
   unblocked, open items. Optionally filter by `kind` (e.g. `execution`).
   Use `dits_work_show` to read the full state of a candidate before
   committing.
2. **Claim it.** `dits_work_lease id=<ID>`. The returned `lease_id`
   proves you hold the item. If lease acquisition fails (someone else
   got it first), go back to step 1.
3. **Start an attempt.** `dits_work_start id=<ID>`. Store the returned
   `attempt_id`; you'll need it for checkpoint/complete/fail.
4. **Work in chunks.** After each meaningful chunk of progress call
   `dits_work_checkpoint id=<ID> summary="..." progress=0.25` (or
   whatever fraction). Checkpoints are what a resuming agent reads to
   pick up where you left off.
5. **Attach artifacts.** Any file you produce (logs, diffs, reports)
   should go through `dits_work_attach id=<ID> path=<local-path>`.
6. **Close out the attempt.**
   - On success: `dits_work_complete id=<ID> summary="..."`.
   - On failure: `dits_work_fail id=<ID> error="..." retryable=true`.
7. **Eval (if applicable).** For work that needs grading, emit
   `dits_work_eval_request id=<ID> scope="..." subject_kind=attempt
   subject=<attempt_id>` and stop. A judge agent (see
   `dits-eval-judge`) will complete it.
8. **Release the lease.** `dits_work_lease_release id=<ID>` so the
   item is reusable.

## Failure / retry branch

If `dits_work_fail` was called with `retryable=true`, the loop restarts
at step 3 (`dits_work_start`) under a new attempt ID — *without*
releasing the lease. This mirrors the "Autonomous Retry Loop" pattern
from `docs/use-cases.md`. After a fixed number of failed attempts,
escalate with `dits_work_block` and a human-readable reason.

## Worked example

```
# Find ready execution work
dits_work_list(ready=true, kind="execution")
# -> [{"id": "01J...", "shared_id": "PROJ-42", "title": "rebuild index"}]

dits_work_lease(id="PROJ-42")
dits_work_start(id="PROJ-42")
# ... do work ...
dits_work_checkpoint(id="PROJ-42", summary="built shard 1/4", progress=0.25)
# ... more work ...
dits_work_attach(id="PROJ-42", path="/tmp/index-report.json")
dits_work_complete(id="PROJ-42", summary="index rebuilt, 4 shards")
dits_work_lease_release(id="PROJ-42")
```
