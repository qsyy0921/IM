# NexusIM Agent Guide

This file routes Codex and sub-agents without duplicating long project history.

## Start Every Turn

1. Run `git status --short --branch --untracked-files=all`.
2. Read `prompt.md`.
3. Read `agent.md` for progress-management rules.
4. Read only the extra document required by the current task.

Do not read long SDD, archive, history, or loadtest reports unless exact evidence
from them is needed.

## Document Routing

| Task | Read first | Maintain when facts change |
| --- | --- | --- |
| Continue active goal | `docs/runbook/current-goal.md` | `docs/runbook/current-goal.md` |
| Understand phase | `docs/runbook/current-brief.md` | `docs/runbook/current-brief.md` |
| Choose unfinished work | `docs/runbook/remaining-goals.md` | `docs/runbook/remaining-goals.md` |
| Work on one service | `docs/runbook/service-briefs/README.md`, then service brief | service brief; `development-progress.md` only for public progress |
| Repair / DLQ / operator | `docs/runbook/repair-operators.md`, service brief | same |
| Distributed smoke / fault evidence | relevant runbook README and exact report path | new report or summary only |
| Interview narrative | `docs/interview/project-progress.md` | same |
| Architecture / service split | `docs/architecture/target-architecture.md` | architecture doc or ADR |

Keep entrance docs short. Do not copy the same status into every file.

## Current Direction

NexusIM is in the "necessary closeout + AI large-model application foundation"
phase.

Existing real services:

- `api-gateway`
- `identity-service`
- `message-service`
- `conversation-service`
- `delivery-service`
- `push-gateway`
- `receipt-service`
- `contacts-service`
- `policy-service`

Current main line:

1. Close only the 9-service gaps that block search / memory / retrieval / Agent:
   mutation semantics, visibility windows, contacts privacy, policy, audit, and
   security boundaries.
2. Start `search-service v0.1` as projection / visibility / tombstone /
   `SearchMessages`, not an LLM demo.
3. Then build `memory-service`, `retrieval-gateway`, RAG, Agent,
   `skill-registry`, `mcp-gateway`, and `action-executor`.
4. Keep production-grade HA, long load tests, sizing, full SLOs, and provider
   operations in hardening backlog unless the user explicitly asks for them.

## Progress Documents

- `prompt.md`: goal-box prompt and route.
- `agent.md`: this routing and collaboration guide.
- `docs/runbook/current-goal.md`: concrete active goal.
- `docs/runbook/current-brief.md`: current phase.
- `docs/runbook/remaining-goals.md`: unfinished work only.
- `docs/runbook/development-progress.md`: human progress overview.
- `docs/runbook/service-briefs/<service>.md`: single-service state.
- `docs/interview/project-progress.md`: interview-facing progress.

When work changes facts, update only the owning document. New work goes into
`remaining-goals.md`; promote it to `current-goal.md` only when active.

## Work Selection

Prefer:

1. IM semantics required by search / memory / retrieval / Agent.
2. Security boundaries: public listener guards, mock auth, trusted metadata,
   TLS / mTLS, sensitive data hygiene.
3. Service hardening from `remaining-goals.md` only when it blocks the current
   foundation work.
4. Repair / DLQ / audit and operator safety.
5. Search / group memory / retrieval before RAG or Agent.
6. Observability, fault smoke, capacity, and complexity governance as
   non-blocking hardening unless requested.

Each slice should close with code, tests, docs, and a focused commit when
practical.

## Engineering Rules

- Do not revert user changes.
- Do not read another service's private tables from production code.
- Do not introduce mesh-like synchronous RPC dependencies.
- Do not create shared packages until at least two real callers need a stable
  contract.
- Keep abstractions local until the second real use case appears.
- Split production files near 2500 lines; split tests or runners near 3000 lines.
- Refresh legitimate file-size baseline drift with
  `tools/update-file-size-hotspot-baseline.ps1`.
- Raw loadtest data belongs under `H:\NexusIM\loadtest-results`; repo stores
  summaries and reports only.

## Sub-Agents

Use sub-agents only for explicitly requested parallel work, review, test-gap
discovery, or disjoint implementation. Keep each task narrow: one service, one
concern, one output format.

Bound concurrency. Normally use 2-3 agents; use more only when the user asks and
write scopes are disjoint. Do not let two agents edit the same file or section.
The main agent owns integration, consistency, validation, and closing stale
agents.

## Validation Before Finishing

For meaningful changes:

1. Run focused service/package tests.
2. Run relevant integration or smoke checks.
3. Run `.\tools\check-local.ps1` unless the change is docs-only and a focused
   doc check is sufficient.
4. Check `git status --short --branch --untracked-files=all`.
5. Commit with a focused message when requested or appropriate.

Short-term production-grade load tests, long fault drills, and full
production-readiness checks are not default finishing gates for small slices.
Run them only when the task, risk, or user request requires them.
