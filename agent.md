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
ambiguous, this wins unless the user gives a more specific request.

```text
AI application foundation is the default main line; broad 9-service P2 backlog
cleanup is not the default development entry:
group memory -> cross-group/time EvidencePack -> RAG -> summary -> multi-agent
-> skill registry -> MCP/tool gateway -> action executor -> approval/audit
-> AI evaluation.
```

The AI/RAG/Agent line is the main product direction, not future side work; when
the user says "continue development", move this chain forward by default.
Production HA drills, long load tests, sizing, and provider-grade operations are
hardening backlog unless explicitly named or they expose a P0/P1 blocker.
The existing IM backend and distributed base are treated as a usable foundation;
do not choose broad 9-service P2 hardening as the next task unless it blocks the
AI line or the user explicitly asks for it.
If the visible Codex goal prompt does not make this main line obvious, update
`prompt.md` and the short prompt in `docs/runbook/current-goal.md` first before
continuing implementation.
The short prompt first line must say the only default main line is the AI
application foundation. Do not hide it only in other docs.

Existing real services: `api-gateway`, `identity-service`, `message-service`,
`conversation-service`, `delivery-service`, `push-gateway`, `receipt-service`,
`contacts-service`, and `policy-service`.

Current active slice: `skill-registry` first catalog path, `mcp-gateway`
first prepare path, `action-executor` first execution audit path, Agent ->
mcp-gateway adapter smoke, proposal store, approval workflow,
approval outbox relay, action-executor approved proposal handoff, Agent
execution eval adapter, low-sensitive tool result projection, local safe tool
adapter, and proposal approval operator first paths are landed. `agent-service` now calls
`mcp-gateway.PrepareToolCall`, persists low-sensitive proposal / approval
metadata, exposes `VerifyApprovedAgentProposal`, and publishes low-sensitive
`im.agent.events` approval events through the approval outbox relay.
`action-executor` can execute only the deterministic low-sensitive
`nexusim.local.echo` local adapter and records output hash only; external
MCP/provider fallback is classified as stable low-sensitive failure and unsafe
tool output is suppressed. Python AI Worker foundation is landed with the
`ai/python` directory, reproducible `IM` conda toolchain and candidate contract
guard. RAG/Summary guarded external HTTP boundary, Python worker safety eval,
Go-side smoke, and RAG/Summary/Agent candidate guards are landed. Next move is external MCP / provider tool guarded adapter first path, or `current-goal.md`.
Search, memory, retrieval, real RAG / summary / Agent adapter smokes, the skill
catalog foundation, the MCP prepare boundary, and approved proposal preflight
are passed.
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
- Refresh legitimate file-size baseline drift with `tools/update-file-size-hotspot-baseline.ps1`.
- Raw loadtest data belongs under `H:\NexusIM\loadtest-results`; repo stores
  summaries and reports only.

## Sub-Agents

Use sub-agents only for disjoint review, implementation or verification. Keep
one service / concern / output per agent, avoid concurrent edits to the same
file or section, and close stale agents after integration.

## Validation Before Finishing

Use tiered gates to avoid wasting time:

1. Small docs or one-package code: run focused tests/scripts only.
2. One-service changes: run that service tests, build, and relevant smoke.
3. Cross-service, generated code, migration, registry, Docker/compose, security
   boundary, or pre-push changes: run `.\tools\check-local.ps1`.
4. Always end with `git status --short --branch --untracked-files=all`.
Long load tests, fault drills, production-readiness checks, and full gates run only when risk, scope, or the user asks for them.
