---
name: dits-eval-judge
description: How to play the judge role on a DITS work item, grading an attempt or artifact against a rubric. TRIGGER when asked to "judge", "evaluate", "grade", or "review" a DITS attempt, artifact, or eval-request.
---

# DITS eval judge

A judge agent's job is to take an unopened eval request, run the rubric
against the subject, and record a verdict. Judges must not modify the
artifact they grade — the `dits-judge` subagent enforces this by
restricting the tool surface.

## Tools used

Read: `dits_work_show`, `dits_work_events`, plus filesystem `Read`.
Write: `dits_work_eval_complete`, `dits_work_retain`, `dits_work_discard`,
`dits_work_block`, `dits_work_comment`.

## The flow

1. **Find the request.** Either you were handed an `eval_id` directly,
   or you scan `dits_work_events id=<ID> type="work.eval_requested"`
   on the work item.
2. **Load the subject.** The eval request carries `subject_kind` and
   `subject_ref`. Use `dits_work_show` to load the work item and find
   the matching attempt/artifact/finding.
3. **Apply the rubric.** Read any referenced `rubric_ref`. For
   artifacts that are files, use filesystem `Read` to inspect the
   actual bytes.
4. **Emit the verdict.** Exactly one of:
   ```
   dits_work_eval_complete(
     id=<work-item-id>,
     eval_id=<eval-id>,
     verdict="pass" | "fail" | "partial",
     summary="human-readable rationale",
     metrics_json='{"score": 0.87, "criteria": {...}}',
     subject_kind=<same as request>,
     subject=<same as request>
   )
   ```
5. **Follow through on the decision.**
   - `pass` → `dits_work_retain subject_kind=<...> subject=<...>
     eval_ref=<eval_id>` to mark this output as the accepted result.
   - `fail` → `dits_work_discard` the subject, and leave the worker to
     retry.
   - `partial` → comment with what's missing and leave it to the worker.
6. **Escalate on ambiguity.** If you genuinely cannot decide
   (conflicting signals, missing context), `dits_work_block` with a
   reason explaining what a human reviewer needs to decide, and open
   a review via the CLI `dits work review` if available. Do not guess.

## Rules of thumb

- Judges never edit artifacts. If you find yourself wanting to "just
  fix the obvious thing," stop — that's a worker action.
- Every verdict must carry a summary. `metrics_json` is optional but
  strongly encouraged for reproducible grading.
- Don't `retain` and `discard` the same subject. The last write wins,
  which will confuse downstream workers.
