# NexusIM Agent Guide

This file routes Codex and sub-agents. Keep it short; concrete scope lives in
the owning runbook / SDD / ADR.

## Start Every Turn

1. Run `git status --short --branch --untracked-files=all`.
2. Read `prompt.md`.
3. Read `agent.md` for progress-management rules.
4. Read `docs/runbook/current-goal.md`.
5. Read only the extra doc required by the current task.

Do not read long SDD, archive, history, or loadtest reports unless exact
evidence is needed.

## Document Routing

| Task | Read first | Maintain when facts change |
| --- | --- | --- |
| Active slice | `docs/runbook/current-goal.md` | same |
| Phase summary | `docs/runbook/current-brief.md` | same |
| Unfinished work | `docs/runbook/remaining-goals.md` | same |
| Public overview | `README.md` | README when public capability, service, middleware, AI boundary, client status or next-step status changes |
| One service | `docs/runbook/service-briefs/README.md`, then service brief | service brief |
| Architecture / service split | `docs/architecture/target-architecture-complete.md` | architecture doc or ADR |
| Middleware | `docs/platform/middleware-catalog.md` | middleware catalog, runtime profile, SDD / ADR |
| Fail-closed / fallback | `docs/architecture/fail-closed-policy.md` | policy doc and affected SDD |
| Interview narrative | `docs/interview/project-progress.md` | same |

If the goal box conflicts with repository documents, trust `current-goal.md`,
`current-brief.md`, and `remaining-goals.md`.

## Owner Documents

| Concern | Owner |
| --- | --- |
| Active slice and order | `docs/runbook/current-goal.md` |
| Short phase state | `docs/runbook/current-brief.md` |
| Backlog and hardening | `docs/runbook/remaining-goals.md` |
| Target architecture | `docs/architecture/target-architecture-complete.md` |
| Middleware adoption | `docs/platform/middleware-catalog.md` |
| Hidden fallback governance | `docs/architecture/fail-closed-policy.md` |
| GitHub overview | `README.md` |

Existing real IM services: `api-gateway`, `identity-service`, `message-service`,
`conversation-service`, `delivery-service`, `push-gateway`, `receipt-service`,
`contacts-service`, and `policy-service`.

AI foundation service briefs cover search, memory, retrieval, RAG, summary,
agent, skill-registry, mcp-gateway, action-executor and ai-eval. Promote future
services service-by-service; do not create all planned directories in one change.

## Feature Development Protocol

Default to feature-module slices, not one-field or one-sentence fragments. A
slice should produce one user-visible or operator-visible capability unless the
repository state makes that unsafe.

Codex goals should name a full feature module, not a tiny implementation slice.
Do not turn a field addition, sentence edit, helper output, or one-test tweak
into the goal. If there are existing uncommitted changes, close and verify that
work first before starting the next module.

Before coding a new feature module, do a compact architecture analysis, then
implement. Identify owner, data ownership, API / event contracts, auth / audit,
fail-closed behavior, platform placement, runtime / middleware impact, and docs
to update.

Platform placement:

- middleware platform: Redis, Kafka, PostgreSQL, OpenSearch, vector store, object
  storage, OTel, Vault, Keycloak, OpenFGA, Temporal and similar runtime pieces;
- data platform: ingestion, CDC, lakehouse, analytics, feature store;
- AI / Agent platform: model gateway, vector, retrieval, RAG, memory, Python AI
  worker, skills, MCP, agent execution;
- business / product platform: IM, media, notification, audit, admin, workflow,
  control plane, presence;
- client platform: Web, Windows PC, Android, shared client-core.

## Engineering Rules

- Do not revert user changes.
- Do not cross-read another service's private tables.
- Do not introduce mesh-like synchronous RPC dependencies.
- Do not add hidden business fallback paths. Do not use fake data, default
  success, stale local cache, silent downgrade, or legacy endpoints to make a
  broken path appear successful. Unknown dependency, permission, projection,
  provider or fact-source state must fail closed, retry explicitly, repair, or
  recover from the owning fact source.
- When touching a path, remove nearby old hidden fallback-like branches if they
  are not allowed by `docs/architecture/fail-closed-policy.md`; otherwise record
  the cleanup in `remaining-goals.md`.
- Keep shared abstractions local until at least two real callers need them.
- Go owns backend facts / control / auth / audit. TypeScript owns client
  protocol, sync core and UI. Python owns AI / eval candidates only. Native
  languages are thin platform adapters.
- Split production files near 2500 lines; split tests or runners near 3000.
- Raw loadtest data belongs under `H:\NexusIM\loadtest-results`; repo stores
  summaries and reports only.

## Work Selection

Prefer the active slice in `current-goal.md`. Treat unrelated hardening as
backlog unless it is a P0/P1 fix, a user-explicit request, or required to unblock
the active slice.

Update documents only when phase, public capability, architecture boundary,
service, middleware, provider, or operator workflow changes. Do not fan out
documentation edits for every internal field-level implementation detail.

Status reports should stay concrete: what changed, what was verified, and what
remains.

## Sub-Agents

Use sub-agents only for disjoint review, implementation or verification. Keep one
service / concern / output per agent, avoid concurrent edits, and close stale
agents.

## Validation

Use tiered gates:

1. Client module work: `npm --prefix clients run check:no-toolchain` and
   `git diff --check; git diff --cached --check` unless the module crosses
   generated code, migrations, Docker, service registry, or security boundaries.
2. Small docs or one-package code: focused tests/scripts.
3. One-service changes: that service tests, build, and relevant smoke.
4. Cross-service, generated code, migration, registry, Docker/compose, security
   boundary, or pre-push changes: `.\tools\check-local.ps1`.
   `check-local` records the currently running / failed sub-gate in
   `.git\nexusim-check-local-state.json` and the next run resumes from that
   sub-gate by default. Use `-NoResume` for a fresh full run, `-ResetResume` to
   clear stale state, or `-StartAt "<step name>"` for an explicit continuation.
5. End with `git status --short --branch --untracked-files=all`.
