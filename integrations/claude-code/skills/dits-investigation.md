---
name: dits-investigation
description: How to drive a DITS investigation work item (kind=investigation). TRIGGER when the user says "investigate", "diagnose", "root-cause", or when the current DITS work item has kind=investigation.
---

# DITS investigation workflow

Investigations are read-heavy, not execution-heavy. The output is a set
of **observations** (raw facts you gathered), **findings** (claims with
confidence scores), and **artifacts** (logs, screenshots, commands).

## Tools used

Read: `dits_work_show`, `dits_work_events`, `dits_work_list`.
Write: `dits_work_observe`, `dits_work_finding`, `dits_work_attach`,
`dits_work_comment`, `dits_work_handoff`.

## The pattern

1. **Understand scope.** `dits_work_show id=<ID>` to load the item,
   then `dits_work_events id=<ID>` to see anything prior investigators
   recorded.
2. **Gather evidence.** For each fact you dig up, call
   `dits_work_observe id=<ID> summary="<single concrete fact>"`. Keep
   observations atomic — one fact per call. This is the investigation's
   audit log.
3. **Attach raw data.** Logs, command output, screenshots — via
   `dits_work_attach`. Every finding should cite an artifact.
4. **State findings.** Once you've gathered enough evidence, call
   `dits_work_finding id=<ID> statement="..." confidence=0.85` for each
   conclusion. Use low confidence (0.3-0.5) for hypotheses and high
   confidence (0.9+) only when the evidence is direct.
5. **Hand off or close.** If execution work is needed, create a new
   execution work item and `dits_work_link` it as `depends_on` this
   investigation; then `dits_work_handoff` to the appropriate actor.

## Rules of thumb

- Observations are append-only and must be ground truth. Don't record
  interpretations as observations — use findings for that.
- A finding without a supporting observation or artifact is almost
  always premature. Go gather more.
- If you're blocked by missing access or data, `dits_work_block` with
  a reason explaining what you need.
