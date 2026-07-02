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
- checkpoint compatibility is undefined.

## Next Evidence Needed

- Checkpoint storage owner review.
- Wakeup race and duplicate callback proof.
- Operator controls for cancel, resume and replay.
- Budget ledger policy and abuse review path.
