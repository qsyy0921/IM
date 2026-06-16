# NexusIM Agent Guide

This file tells Codex and sub-agents how to manage NexusIM progress without
reading or duplicating long documents.

## Start Every Turn

1. Run `git status --short --branch --untracked-files=all`.
2. Read `prompt.md`.
3. Read `agent.md` to confirm progress-management rules.
4. Read `docs/runbook/current-goal.md`.
5. Read `docs/runbook/current-brief.md`.
6. Read `docs/runbook/remaining-goals.md` only when choosing or updating work.
7. For service work, read `docs/runbook/service-briefs/README.md`, then only the
   specific `docs/runbook/service-briefs/<service>.md` file involved.

Do not read long SDD, archive, history, or loadtest reports unless the current
slice needs exact evidence from them.

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
and short summaries.

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
