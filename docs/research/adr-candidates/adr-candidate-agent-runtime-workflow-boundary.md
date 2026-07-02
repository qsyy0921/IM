# ADR Candidate: Agent Runtime / Workflow Boundary

Status: candidate. Not accepted. Does not authorize production service creation.

## Context

The skeleton proves AgentRun, AgentStep, runtime control, checkpoint, cancel,
resume and replay lineage in fixture form. The production risk is ownership
confusion between Agent Runtime and `workflow-service`.

## Candidate Decision

Agent Runtime owns cognitive run state. `workflow-service` owns durable
human-wait and external callback state.

Start as a runtime module/harness boundary. Do not create
`agent-runtime-service` until queue ownership, wakeup durability and operator
control-plane needs are proven.

## Ownership Matrix

| State | Owner | Must Not Own |
| --- | --- | --- |
| AgentRun metadata | Agent Runtime | Business facts, ACTIVE memory |
| AgentStep trace | Agent Runtime | Approval final state |
| AgentCheckpoint | Agent Runtime | Raw provider body |
| RuntimeWakeup consumption | Agent Runtime | Durable wait queue |
| Approval wait | workflow-service | Planner state |
| External callback wait | workflow-service | Raw prompt or EvidencePack body |
| Compensation workflow | workflow-service | Tool execution truth |
| Side-effect execution | action-executor | Cognitive planner state |
| Audit archive | audit-service | Mutable runtime decisions |

## Owned Objects

- AgentRunStore
- AgentCheckpoint
- RuntimeWakeup
- CancelToken
- ResumeToken
- ReplayIndex
- BudgetLedger
- ApprovalRequest
- ApprovalDecision

## Boundary Rules

- Runtime may pause after emitting an approval request ref.
- workflow-service may wake Runtime with a decision ref.
- workflow-service must not inspect raw prompt, EvidencePack body or planner
  state.
- Runtime must not durably wait for human approval as a workflow engine.
- Runtime resume must verify checkpoint version and wakeup correlation.

## Checkpoint Storage Boundary

AgentCheckpoint is a low-sensitive restart reference, not a raw conversation or
provider-body archive.

Checkpoint storage must:

- store refs, hashes, versions and decision lineage;
- reject raw prompt, raw provider body, secrets and service private rows;
- carry checkpoint version and replay reader policy ref;
- fail closed when a required checkpoint version is unsupported;
- link to ReplayIndex and audit refs.

Runtime cannot use checkpoints as business fact storage or ACTIVE memory.

## Wakeup And Dedupe Semantics

workflow-service owns durable human waits and external callback receipt. Runtime
owns wakeup consumption only.

Runtime resume must verify:

- wakeup id;
- approval or callback decision ref;
- checkpoint version;
- run id and step correlation;
- cancel token state;
- dedupe key.

Duplicate wakeups must be idempotent. Stale, mismatched or already-consumed
wakeups reject resume and emit a failure class for replay.

## Operator Controls

Before production promotion, authorized operators must be able to:

- cancel a pending or running AgentRun;
- inspect checkpoint, wakeup, workflow and audit refs without raw prompt body;
- resume from a compatible checkpoint;
- replay from ReplayIndex without re-executing side effects;
- see why cancel, resume or replay was rejected.

These controls belong to a control-plane or AgentOps surface, not Python worker.

## Budget And Abuse Boundary

BudgetLedger is required for any long-running or tool-using Runtime path.

It must track:

- model budget;
- retrieval budget;
- tool prepare/execution budget refs;
- retry and redrive counts;
- runtime duration;
- tenant / actor / AgentDefinition / SkillPackage refs.

Over-budget runs fail closed or require explicit review. Runtime cannot hide
budget exhaustion behind fallback summaries.

## Replay Requirements

ReplayIndex must link:

- run id;
- step refs;
- checkpoint refs;
- wakeup refs;
- approval decision refs;
- tool prepare refs;
- execution and audit refs;
- failure class.

## Rejection Rules

Reject the ADR if:

- Runtime becomes a second workflow engine;
- workflow-service understands planner internals;
- cancel/resume is not idempotent;
- wakeup races cannot be deduped;
- checkpoint compatibility is undefined;
- checkpoints contain raw prompts, raw provider bodies, secrets or business
  fact storage;
- over-budget runs continue without explicit policy or review.

## Next Evidence Needed

- Main integration owner review for checkpoint storage.
- Fixture proof for wakeup race, duplicate callback and stale checkpoint
  rejection now exists in
  `docs/research/agent-runtime-workflow-fixture-evidence-20260702.md`; production
  integration remains blocked.
- Control-plane / AgentOps owner for cancel, resume and replay controls.
- Policy owner for BudgetLedger limits and abuse review path.
