---
name: dits-handoff
description: When and how to hand a DITS work item off to another actor, and how to express dependencies via `dits_work_link depends_on`. TRIGGER when the user says "hand off", "pass to", "delegate", or when a work item cannot proceed without another actor.
---

# DITS handoff

Hand off a work item when the next step genuinely requires a different
actor — a different agent role, a specialist, or a human reviewer.
Don't hand off just to avoid finishing.

## Tools used

`dits_work_handoff`, `dits_work_link`, `dits_work_comment`,
`dits_work_show`.

## Checklist before handing off

1. The current attempt is in a clean state. Either
   `dits_work_checkpoint` with the latest progress, or `dits_work_fail`
   if you were blocked.
2. Everything the next actor needs to resume is linked or attached:
   artifacts via `dits_work_attach`, related work items via
   `dits_work_link type="depends_on" target=<id>`.
3. The receiving actor exists. Use `dits_identity_show` for your own
   actor ID and ask the user for the recipient if unsure.

## The call

```
dits_work_handoff(
  id="PROJ-42",
  to="reviewer-alice",
  context="Attempt 3 produced a working binary but failing e2e test
           `auth_login`. See artifact `trace.log`. Needs a judge call
           on whether this is acceptable partial output."
)
```

The `context` field is free-form but should answer three questions:
**where am I**, **why am I stopping**, and **what should you do next**.

## Cross-reference: dependencies

Use `dits_work_link type="depends_on"` when the current item is
blocked *by data or work produced elsewhere* rather than by a need for
another actor. A handoff transfers ownership; `depends_on` records a
prerequisite.
