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

This section is the routing guard. If a future prompt or short Codex goal is
ambiguous, the active slice comes from `docs/runbook/current-goal.md`. Do not
hard-code the active slice in this file.

```text
Read current-goal.md first, then read only the service brief / SDD needed by
that active slice.
```

The stable Codex goal prompt intentionally does not name the active service or
priority. Keep concrete scope in `current-goal.md`, not in the goal box.
If the goal box is stale or conflicts with repository documents, trust
`current-goal.md`, `current-brief.md`, and `remaining-goals.md`.
Production HA drills, long load tests, sizing, provider-grade operations and
broad backlog cleanup remain hardening backlog unless explicitly named or they
block the active slice.

Existing real services: `api-gateway`, `identity-service`, `message-service`,
`conversation-service`, `delivery-service`, `push-gateway`, `receipt-service`,
`contacts-service`, and `policy-service`.

AI foundation services already landed enough first paths to act as a usable
base: search, memory, retrieval, RAG, summary, agent, skill-registry,
mcp-gateway, action-executor and ai-eval. Details live in service briefs and
progress docs.

Current active targets live in `current-goal.md`. Promote future services
service-by-service; do not create every planned service directory in one change.

AI invariants: separate facts, projections, retrieval and controlled execution.
Memory requires source refs, scope, validity, supersession, confidence and review state. RAG / summary / Agent consume EvidencePack only;
actions go through policy, proposal / approval, executor and audit; Python AI
workers only return candidates, while Go owns control, state and audit.

## Progress Documents

Use the routing table above as the source of truth. When facts change, update
only the owning document. New work goes into `remaining-goals.md`; promote it to
`current-goal.md` only when active.

## Work Selection

Prefer:

1. The active slice in `current-goal.md`.
2. Security boundaries: public listener guards, mock auth, trusted metadata,
   TLS / mTLS, sensitive data hygiene.
3. Service hardening from `remaining-goals.md` only when it blocks active work.
4. Repair / DLQ / audit and operator safety.
5. Search / memory / retrieval / EvidencePack boundaries before RAG or Agent changes.
6. Observability, fault smoke, capacity, and complexity governance as
   non-blocking hardening unless requested.

If a candidate task does not move the current active slice, treat it as backlog
unless it is a P0/P1 fix, a user-explicit request, or a small cleanup needed to
unblock the active slice.

Each slice should close with code, tests, docs, and a focused commit when
practical.

## Engineering Rules

- Do not revert user changes.
- Do not read another service's private tables from production code.
- Do not introduce mesh-like synchronous RPC dependencies.
- Do not create shared packages until at least two real callers need a stable contract.
- Keep abstractions local until the second real use case appears.
- Split production files near 2500 lines; split tests or runners near 3000 lines.
- Refresh legitimate file-size baseline drift with `tools/update-file-size-hotspot-baseline.ps1`.
- Raw loadtest data belongs under `H:\NexusIM\loadtest-results`; repo stores
  summaries and reports only.

## Sub-Agents

Use sub-agents only for disjoint review, implementation or verification. Keep one
service / concern / output per agent, avoid concurrent edits, and close stale agents.

## Validation Before Finishing

Use tiered gates to avoid wasting time:

1. Small docs or one-package code: run focused tests/scripts only.
2. One-service changes: run that service tests, build, and relevant smoke.
3. Cross-service, generated code, migration, registry, Docker/compose, security boundary, or pre-push changes: run `.\tools\check-local.ps1`.
4. End with `git status --short --branch --untracked-files=all`; full gates run only when risk, scope, or the user asks.
