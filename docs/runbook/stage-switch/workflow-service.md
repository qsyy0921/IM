# workflow-service Stage-Switch Review

Date: 2026-06-21

## Result

`workflow-service` SDD v0.1 is ready to enter the implementation stage. No
P0/P1 blocker was found in the stage-switch review.

This review does not create `services/workflow-service` yet. The implementation
slice must switch `workflow-service` out of `future` in `service-registry.json`
and create the service directory in the same coherent change.

## Why Promotion Is Justified

- Independent data model: workflow requests, workflow steps, approval decisions,
  timers, compensation state and workflow outbox are not owned by agent-service,
  action-executor, audit-service, admin-service or individual business services.
- Independent scale profile: approval wait, operator decisions, long timers,
  compensation and repair orchestration scale differently from online IM writes,
  delivery, push route, model invocation and search / memory projection.
- Independent failure boundary: a stuck approval, external callback wait or
  compensation retry must not block message send, PullInbox, push notify,
  retrieval, proposal creation or audit append.
- Security boundary: high-risk actions need a durable request / decision owner
  that preserves proposal / approval / executor / audit separation and avoids
  leaking raw operator reason, EvidencePack text or business payloads.
- Complexity reduction: embedding long-running state machines inside
  agent-service, admin-service or each repair operator would duplicate approval,
  timeout, compensation and redaction logic.

## Boundary Checks

- agent-service still owns proposal creation and proposal verification.
- action-executor still owns approved tool execution and result projection.
- audit-service still owns immutable audit append and export.
- admin-service can expose operator UI/API later, but workflow-service owns
  durable workflow state and decisions.
- Business services must use public APIs, events or explicit ports. They must
  not let workflow-service directly mutate private service tables.
- Temporal or another workflow engine is only an infrastructure candidate. The
  first implementation should keep the domain model portable.
- Events, metrics and debug snapshots must not contain raw operator reason,
  EvidencePack text, proposal body, tool input/output, provider body, secret,
  token, private URL, DSN, raw business payload or raw identifiers.

## Gate Impact For Next Slice

The implementation slice is broader than a docs-only change. It must update:

- `docs/runbook/service-registry.json`: switch `workflow-service` from `future`
  to `product-active` with local process / debug / observability metadata.
- `api/proto/nexusim/workflow/v1/workflow_service.proto`.
- `migrations/postgres/workflow/000001_workflow_core.sql`.
- `services/workflow-service` six-layer skeleton and `cmd/workflow-service`.
- Docker runtime, local compose, Prometheus rules and Grafana dashboard.

Because this touches registry, proto, migration, Docker/compose and public API,
the implementation slice should run the expanded gate set or full `check-local`
before push.

## First Implementation Scope

Keep the first code slice narrow:

```text
CreateWorkflow
RecordWorkflowDecision
GetWorkflow
```

Support `ACTION_APPROVAL` and `REPAIR_APPROVAL` in the initial slice, then add
`ADMIN_OPERATION` when admin-service needs a generic CRITICAL operation approval
path. `AdvanceWorkflow`, timer workers, compensation workers, external callback
waits and outbox relay can follow after the first gRPC smoke unless they are
needed to prove the first path.

## Focused Acceptance For First Smoke

- `CreateWorkflow` is idempotent by tenant + source service + source request +
  idempotency key.
- workflow payload storage is low-sensitive: only refs, hashes, status, risk
  class, actor refs and trace refs are persisted.
- `RecordWorkflowDecision` requires operator identity and stable decision enum;
  raw reason text must not enter events, metrics or public responses.
- approval and denial transitions are monotonic and reject replay with different
  command hash.
- `GetWorkflow` returns public-safe state only and does not expose raw proposal,
  EvidencePack, tool input/output or provider body.
- no workflow code reads or writes private tables owned by agent-service,
  action-executor, audit-service, admin-service or IM business services.
