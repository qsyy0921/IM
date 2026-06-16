# NexusIM Agent Guide

This file tells Codex and sub-agents how to manage NexusIM progress without
reading or duplicating long documents.

## Start Every Turn

1. Run `git status --short --branch --untracked-files=all`.
2. Read `prompt.md`.
3. Read `agent.md` to confirm progress-management rules.
4. Read additional documents only when the current task needs them. Use the
   routing table below instead of opening every runbook or SDD file.

Do not read long SDD, archive, history, or loadtest reports unless the current
slice needs exact evidence from them.

## Document Routing

Use this table to decide what to read and what to maintain.

| Task | Read first | Maintain when facts change |
| --- | --- | --- |
| Continue the active Codex goal | `docs/runbook/current-goal.md` | `docs/runbook/current-goal.md` only if the concrete goal changed |
| Understand current phase | `docs/runbook/current-brief.md` | `docs/runbook/current-brief.md` |
| Choose next unfinished backend work | `docs/runbook/remaining-goals.md` | `docs/runbook/remaining-goals.md` |
| Work on one service | `docs/runbook/service-briefs/README.md`, then that service brief | that service brief, plus `development-progress.md` if public progress changed |
| Repair / DLQ / operator work | `docs/runbook/repair-operators.md` and the relevant service brief | `repair-operators.md`, relevant service brief |
| Distributed smoke / fault evidence | relevant runbook README and only the exact report path needed | the new report or summary, not the entrance docs |
| Interview progress narrative | `docs/interview/project-progress.md` | `docs/interview/project-progress.md` |
| Architecture target or service split rules | `docs/architecture/target-architecture.md` only when architecture changes | target architecture or ADR, not routine progress docs |

Do not duplicate the same status into every document. Keep entrance documents
short, route to the owner document, and update only the owner document.

## Current Project Direction

NexusIM is currently in the "clean the existing backend services" phase.

The existing service set is:

- `api-gateway`
- `identity-service`
- `message-service`
- `conversation-service`
- `delivery-service`
- `push-gateway`
- `receipt-service`
- `contacts-service`
- `policy-service`

Do not start `search-service`, RAG, summary, agent, media, notification, admin,
or clients unless `docs/runbook/current-brief.md` and
`docs/runbook/remaining-goals.md` explicitly move the project into that phase.

## Progress Documents

Use these documents for different jobs:

- `prompt.md`: Codex goal-box prompt and document routing.
- `agent.md`: document-routing and progress-maintenance rules for Codex and sub-agents.
- `docs/runbook/current-goal.md`: concrete execution goal for Codex.
- `docs/runbook/current-brief.md`: current phase and where to look next.
- `docs/runbook/remaining-goals.md`: only unfinished work.
- `docs/runbook/development-progress.md`: human-readable current progress.
- `docs/runbook/service-briefs/<service>.md`: current state for one service.
- `docs/interview/project-progress.md`: interview-facing backend progress.

When a slice changes current facts, update the matching service brief and, if
the high-level progress changed, `development-progress.md`.

When a slice removes or discovers unfinished work, update
`remaining-goals.md`.

Do not copy the same long status paragraph across multiple files. Prefer links
and short summaries. Do not maintain every document on every turn; update only
the documents whose facts changed in the current slice.

When new work is discovered, add it to `remaining-goals.md` first. Only promote
it into `current-goal.md` when it becomes the active execution goal.

## Work Selection

Prefer work in this order:

1. Security boundaries: public listener guards, mock auth boundaries, trusted
   metadata, TLS / mTLS.
2. Service P2 hardening from `remaining-goals.md`.
3. Repair / DLQ / audit and operator safety.
4. Observability and fault-smoke evidence.
5. Capacity and complexity governance.

Each slice should be small enough to close with code, tests, docs, and a clean
commit.

## Engineering Rules

- Do not revert user changes.
- Do not read another service's private tables from production code.
- Do not introduce mesh-like synchronous RPC dependencies.
- Do not create shared packages until at least two real callers need the same
  stable contract.
- Keep abstractions local until the second real use case appears.
- If a production hand-written file approaches 2500 lines, split by topic in
  the same package before adding more behavior.
- If a test or runner approaches 3000 lines, split helpers or scenario files in
  the same package.
- Raw loadtest data belongs under `H:\NexusIM\loadtest-results`; the repository
  only stores summaries and reports.

## Sub-Agent Guidance

Use sub-agents for parallel read-only review, test-gap discovery, or targeted
implementation support when the scope is large.

Keep each sub-agent narrow:

- one service,
- one concern,
- one output format,
- no broad project-wide scans unless explicitly needed.

Close sub-agents when their result has been incorporated. Do not keep stale
sub-agents running.

## Validation Before Finishing

For meaningful changes:

1. Run focused service/package tests.
2. Run any relevant integration or smoke command.
3. Run `.\tools\check-local.ps1`.
4. Check `git status --short --branch --untracked-files=all`.
5. Commit with a focused message.

If a check is skipped, document why in the final response.
