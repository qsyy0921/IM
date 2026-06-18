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

NexusIM's current main line is no longer "keep hardening everything forever".
It is:

```text
9-service necessary closeout -> search -> group memory -> retrieval/EvidencePack
-> RAG -> summary -> multi-agent -> skill registry -> MCP/tool gateway
-> action executor -> AI evaluation
```

Production HA drills, long load tests, sizing, and provider-grade operations are
hardening backlog unless explicitly named; "continue development" means move
the AI foundation forward while keeping the IM backend clean enough.

The AI line is not a side quest. It is the main product direction after the
existing IM backend is clean enough: group memory, cross-group/cross-time
evidence, permissioned RAG, multi-agent collaboration, MCP/skill/tool execution,
and proposal / approval / executor / audit for real business actions.

Existing real services: `api-gateway`, `identity-service`, `message-service`,
`conversation-service`, `delivery-service`, `push-gateway`, `receipt-service`,
`contacts-service`, and `policy-service`.

Current execution chain:

1. Close only 9-service gaps that block search / memory / retrieval / Agent:
   mutation semantics, visibility, contacts privacy, policy, audit, and security.
2. Keep `search-service v0.1` as projection / visibility / tombstone /
   `SearchMessages`, not an LLM demo; its first projection smoke is now passed.
3. Build `memory-service`, `retrieval-gateway`, RAG, `summary-service`, Agent,
   `skill-registry`, `mcp-gateway`, and `action-executor`.
4. Keep production-grade HA, long load tests, sizing, full SLOs, and provider
   operations in hardening backlog unless they directly unblock the current
   execution chain.

Current active slice: run the real retrieval-gateway -> rag-service RAG adapter smoke.
Search, memory, and retrieval smokes are passed; EvidencePack now includes
source coverage, rerank score, and dedupe reason. Optional policy-service
retrieval precheck is wired, and first-stage AI eval cases are under
`docs/runbook/ai-eval/`. `rag-service` first read-only answer path,
`loadtest/rag`, and the first RAG eval execution adapter are landed; next is
runtime smoke. RAG / summary / Agent must stay EvidencePack-only and must not
bypass message / conversation private table boundaries.

Current AI baseline: keep facts / projections / retrieval / controlled
execution separate; build search -> memory -> retrieval before RAG / Agent;
memory requires source refs, scope, validity, supersession, confidence and
review state; RAG / Agent consume EvidencePack and actions go through policy,
proposal / approval, executor and audit.

## Progress Documents

Use the routing table above as the source of truth. When facts change, update
only the owning document. New work goes into `remaining-goals.md`; promote it to
`current-goal.md` only when active.

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

If a candidate task does not move the current main line, treat it as backlog
unless it is a P0/P1 fix, a user-explicit request, or a small cleanup needed to
unblock the active slice.

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

Use sub-agents only when requested or clearly useful for disjoint review,
implementation or verification. Keep one service / concern / output per agent,
avoid concurrent edits to the same file or section, and close stale agents after
the main agent integrates and validates their output.

## Validation Before Finishing

For meaningful changes:

1. Run focused service/package tests.
2. Run relevant integration or smoke checks.
3. Run `.\tools\check-local.ps1` unless the change is docs-only and a focused
   doc check is sufficient.
4. Check `git status --short --branch --untracked-files=all`.
5. Commit with a focused message when requested or appropriate.

Short-term production-grade load tests, long fault drills, and full production-readiness checks run only when the task, risk, or user asks for them.
