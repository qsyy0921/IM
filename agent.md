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
| Public project overview | `README.md` | `README.md` when active slice, service promotion, client capability, AI boundary or next-step status changes |
| New feature architecture analysis | `docs/runbook/current-goal.md`, relevant service brief / SDD, target architecture when boundary changes | current-goal/current-brief/remaining-goals, service brief, SDD/ADR, README when public capability changes |
| Work on one service | `docs/runbook/service-briefs/README.md`, then service brief | service brief; `development-progress.md` only for public progress |
| Repair / DLQ / operator | `docs/runbook/repair-operators.md`, service brief | same |
| Distributed smoke / fault evidence | relevant runbook README and exact report path | new report or summary only |
| Interview narrative | `docs/interview/project-progress.md` | same |
| Architecture / service split | `docs/architecture/target-architecture.md`, then `docs/architecture/target-architecture-complete.md` | architecture doc or ADR |
| New service promotion | `docs/architecture/target-architecture-complete.md`, `docs/runbook/service-briefs/README.md` | README, service brief, SDD/ADR, service registry/docs/runbook progress |
| Middleware / platform capability | `docs/platform/middleware-catalog.md` | same; add ADR/SDD and runtime profile for active adoption |
| Fail-closed / local-test / compat question | `docs/architecture/fail-closed-policy.md` | same; update SDD only when a concrete service boundary changes |

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

## Mutable Strategy And Boundary Owners

推进策略和架构边界会随项目演进而变化，不写进 Codex 目标框。需要变更时更新下面的
owner document，而不是复制到所有入口文件。

| Concern | Owner document |
| --- | --- |
| Active slice and immediate order | `docs/runbook/current-goal.md` |
| Phase summary and current short direction | `docs/runbook/current-brief.md` |
| Unfinished work and postponed hardening | `docs/runbook/remaining-goals.md` |
| Complete target architecture and service split | `docs/architecture/target-architecture-complete.md` |
| Middleware adoption / replacement rules | `docs/platform/middleware-catalog.md` |
| Hidden fallback / fail-closed governance | `docs/architecture/fail-closed-policy.md` |
| Public overview | `README.md` |

Existing real services: `api-gateway`, `identity-service`, `message-service`,
`conversation-service`, `delivery-service`, `push-gateway`, `receipt-service`,
`contacts-service`, and `policy-service`.

The long-term architecture baseline is `docs/architecture/target-architecture-complete.md`.
New business-platform, data-platform, AI / Agent-platform, client-platform and
middleware-platform work must follow that document's ownership, event, data,
security and evolution rules.

AI foundation services already landed enough first paths to act as a usable
base: search, memory, retrieval, RAG, summary, agent, skill-registry,
mcp-gateway, action-executor and ai-eval. Details live in service briefs and
progress docs.

Language boundary: Go owns backend services, BFF endpoints, control-plane
decisions, durable state, authorization, audit and repair. TypeScript owns Web,
desktop and Android shared client contracts, sync core and UI. Rust, Kotlin and
Swift are thin platform adapters only. Python owns AI workers, model/algorithm
candidates, eval and offline tooling; it does not own durable business facts or
security decisions.

Current active targets live in `current-goal.md`. Promote future services
service-by-service; do not create every planned service directory in one change.

AI invariants: separate facts, projections, retrieval and controlled execution.
Memory requires source refs, scope, validity, supersession, confidence and review state. RAG / summary / Agent consume EvidencePack only;
actions go through policy, proposal / approval, executor and audit; Python AI
workers only return candidates, while Go owns control, state and audit.

## Progress Documents

Use the routing table above as the source of truth. When facts change, update
only the owning document. Root `README.md` is the public GitHub overview and must
be kept aligned with current architecture and progress, but it should stay short.
New work goes into `remaining-goals.md`; promote it to `current-goal.md` only
when active.

## Feature Development Protocol

Before coding a new feature, write or state a compact architecture analysis and
then implement. The analysis must identify:

1. owner service / package and whether a new service is justified;
2. data ownership, migration impact, and whether the state is fact or projection;
3. public API, event, outbox, worker, or client contract changes;
4. authorization, audit, trusted metadata, and fail-closed behavior;
5. whether a new technology, middleware, provider, runtime, or platform component is needed;
6. platform placement: middleware platform, data platform, AI / Agent platform,
   business / product platform, client platform, or operations platform;
7. runtime profile, Docker/compose, deployment, and observability impact;
8. documents to update.

If a new microservice is promoted, update `README.md`, target architecture,
`service-briefs/README.md`, the new service brief, relevant SDD / ADR and
progress docs. If a new middleware or provider is introduced, update
`docs/platform/middleware-catalog.md`, runtime profile docs, relevant SDD / ADR
and README when it changes the public overview. Middleware belongs in the
middleware platform; data processing belongs in the data platform; AI runtime
or model-facing pieces belong in the AI / Agent platform; product-facing
business capabilities belong in the business / product platform.

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
- Do architecture analysis before coding new features; keep it proportional to
  risk and write the durable version into the owning docs when boundaries change.
- Do not read another service's private tables from production code.
- Do not introduce mesh-like synchronous RPC dependencies.
- Do not introduce hidden business alternate paths. Unknown dependency, permission,
  projection, provider or fact-source state must fail closed, retry, repair, or
  recover from the owning fact source. Read `docs/architecture/fail-closed-policy.md`.
- When touching a code path, remove nearby old hidden fallback / fallback-like
  branches if they are not explicitly allowed by `fail-closed-policy.md`.
  If cleanup is too large for the slice, record it in `docs/runbook/remaining-goals.md`
  with the owning service and risk.
- Do not create shared packages until at least two real callers need a stable contract.
- Keep abstractions local until the second real use case appears.
- Keep language boundaries explicit: Go for business/control services,
  TypeScript for client product code, Python for AI/eval candidates, native
  languages only for small runtime bridges.
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
