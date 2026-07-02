# Agent Production Object Model

Date: 2026-07-02

Status: conceptual object model for ADR candidates. This is not a proto,
OpenAPI, Kafka schema, database migration, production service directory or final
runtime implementation.

## Purpose

The isolated Agent skeleton proves the first executable baseline. The next
architecture problem is object ownership: which durable concepts must exist
before NexusIM can safely promote Agent features into production.

This document defines the missing production-grade objects at concept level. It
does not freeze field names, wire schemas or storage tables. Each object must be
converted into an ADR decision before any production contract is created.

## Object Promotion Rule

An object can move from this catalog to an ADR only when it has:

- owner service or module;
- producer and consumer list;
- lifecycle states;
- versioning policy;
- permission/audit boundary;
- replay behavior;
- rejection conditions;
- fixture or eval evidence.

An object can move from ADR to production contract only when the main
integration session approves the specific integration slice.

Current fixture-only evidence:

- `ai/python/nexusim_ai_eval/object_completeness.py`;
- `ai/python/fixtures/agent_eval/object_completeness_rehearsal.json`;
- `docs/research/agent-object-completeness-fixture-evidence-20260702.md`;
- `ai/python/nexusim_ai_eval/operator_governance.py`;
- `ai/python/fixtures/agent_eval/operator_governance_rehearsal.json`;
- `docs/research/agent-operator-governance-fixture-evidence-20260702.md`.

This evidence verifies catalog coverage, operator-surface coverage and
promotion blockers only. It does not authorize production object fields,
schemas, storage, service APIs or admin UI contracts.

## Ownership Principles

- Go services own auth, policy, audit, persistence, final proposal, execution,
  ACTIVE memory and business facts.
- Python AI Worker owns candidate intelligence only: extraction, planning,
  scoring and eval artifacts.
- Agent Runtime owns cognitive run state but not durable human-wait workflows.
- workflow-service owns approval wait, external callbacks and long-running
  compensation.
- action-executor owns side effects.
- retrieval-gateway owns EvidencePack as AI read boundary.
- mcp-gateway owns provider provenance, prepare and tool-output tainting.

## Object Map

| Domain | Objects | First ADR |
| --- | --- | --- |
| Decision | ArchitectureDecision, DecisionReview, RejectionCondition | All ADR candidates |
| Contract versioning | ContractVersion, CompatibilityWindow, ReplayReaderPolicy, DeprecationPolicy, MigrationPolicy | ADR-Agent-Eval-Replay-Harness |
| Runtime | AgentRunStore, AgentCheckpoint, RuntimeWakeup, CancelToken, ResumeToken, ReplayIndex, BudgetLedger | ADR-Agent-Runtime-Workflow-Boundary |
| Evidence / context | SourceVisibilityVersion, CitationMap, DeniedLane, TaintLabel, ConflictSet, EvidenceCoverageReport, CitationVerifierResult | ADR-Agent-Context-EvidencePack-Boundary |
| Memory | MemoryClaim, MemoryAdmissionDecision, MemoryReviewTask, MemoryScope, MemoryVersion, MemoryRevocationLedger, MemorySupersessionChain, GroupConsensus, KnowledgeState, RelationMemory | ADR-Agent-Memory-Admission-Boundary |
| Tool / MCP | ToolProvider, ProviderAttestation, CapabilityLease, PreparedToolRef, ToolSchemaHash, ToolOutputEnvelope, ToolRiskTier, ToolExecutionPolicy | ADR-Agent-Tool-MCP-Boundary |
| Workflow / action | ApprovalRequest, ApprovalDecision, ExecutionIntent, ExecutionReceipt, StateDiffReport, RepairRequest, RedriveRequest, CompensationPlan | ADR-Agent-Runtime-Workflow-Boundary |
| AgentOps | AgentDefinition, SkillPackage, AgentRelease, ReleaseChannel, BaselineApproval, FailureClassOwner, KillSwitch, RollbackPlan, CanaryReport | ADR-AgentOps-Governance-Boundary |
| Dataset / eval | DatasetManifest, DatasetSnapshot, DatasetSplit, EvalSuiteManifest, BaselineReport, RegressionDelta, BlockedPromotionReason | ADR-Agent-Eval-Replay-Harness |
| Operator UX | MemoryInspectView, EvidenceInspectView, ReplayView, ApprovalConsole, AgentReleaseConsole, FailureReviewQueue | ADR-AgentOps-Governance-Boundary |

## Decision Objects

### ArchitectureDecision

Purpose: durable record of an accepted or rejected Agent architecture boundary.

Owner: main integration with Agent Lab authorship.

Conceptual fields:

- decision_id;
- title;
- status: proposed, accepted, rejected, superseded;
- scope;
- owner;
- affected objects;
- evidence refs;
- production blockers;
- supersedes refs.

Cannot own:

- production schema;
- runtime state;
- service configuration.

Promotion use: every Agent contract must reference an accepted ArchitectureDecision.

### DecisionReview

Purpose: records who reviewed a candidate decision and which evidence was
accepted or rejected.

Owner: main integration.

Minimum refs: reviewer, decision_id, evidence refs, requested changes, result.

### RejectionCondition

Purpose: machine-checkable or review-checkable reason a proposed design cannot
advance.

Examples:

- Python owns ACTIVE memory;
- workflow-service consumes planner state;
- tool output becomes trusted instruction;
- EvidencePack body shape is frozen from fixture refs alone.

## Contract Version Objects

### ContractVersion

Purpose: conceptual version record for any future cross-service Agent object.

Owner: producing service plus governance.

Applies to:

- EvidencePack;
- ContextPackage;
- MemoryCandidate / MemoryClaim;
- ToolIntent / PreparedToolRef;
- ReplayBundle;
- EvalReport;
- approval / execution refs.

Conceptual fields:

- contract_name;
- schema_version;
- semantic_version;
- producer_owner;
- consumer_owners;
- compatibility_window_ref;
- replay_reader_policy_ref;
- deprecation_policy_ref.

Promotion blocker: no object can become a production contract without this.

### CompatibilityWindow

Purpose: defines how many previous versions must remain readable and replayable.

Minimum decision: old AgentRun / ReplayBundle artifacts must remain explainable
for the window or be explicitly marked expired with an audit reason.

### ReplayReaderPolicy

Purpose: defines how replay readers reconstruct old artifacts without raw prompts
or raw provider payloads.

Must define:

- low-sensitive refs;
- version lookup;
- hash verification;
- unavailable-source behavior;
- redaction handling.

### DeprecationPolicy

Purpose: controlled retirement of object versions.

Must define owner, sunset criteria, migration plan, alert path and rollback
fallback.

### MigrationPolicy

Purpose: required only for persisted objects. It defines backfill, dual-read,
dual-write, audit and rollback requirements.

## Runtime Objects

### AgentRunStore

Purpose: durable index of AgentRun metadata and replayable state refs.

Owner: Agent Runtime module or future agent-runtime-service, not Python worker.

May own:

- run id;
- actor and tenant refs;
- status;
- step refs;
- checkpoint refs;
- budget ledger ref;
- replay index ref.

Cannot own:

- business facts;
- ACTIVE memory;
- approval final state;
- execution result truth.

### AgentCheckpoint

Purpose: low-sensitive restart point for cognitive run state.

Must contain refs and hashes, not raw provider bodies.

Required behavior:

- checkpoint version validation;
- resume compatibility check;
- stale checkpoint rejection;
- replay linkage.

### RuntimeWakeup

Purpose: correlation object for workflow callback or scheduled resume.

Owner split:

- workflow-service owns durable wait and callback receipt;
- Agent Runtime owns wakeup consumption and cognitive resume.

Reject if RuntimeWakeup becomes a second approval queue.

### CancelToken

Purpose: idempotent cancellation request for an AgentRun.

Must record actor, policy decision ref, reason, issued_at and consumed_at refs.

### ResumeToken

Purpose: controlled resume after checkpoint, approval or external callback.

Must bind to checkpoint version and wakeup ref.

### ReplayIndex

Purpose: maps a run to replay refs: EvidencePack, ContextPackage, tool prepare,
workflow decision, execution, memory candidate and audit refs.

Cannot store raw IM messages or raw provider payloads.

### BudgetLedger

Purpose: tracks model/tool/runtime budget use.

Owner: Agent Runtime with governance visibility.

Must support release gating and abuse review.

## Evidence And Context Objects

### SourceVisibilityVersion

Purpose: snapshot ref proving a source was visible to the actor at context build
time.

Owner: retrieval-gateway / policy-integrated source service.

Required for:

- replay;
- permission leakage audit;
- denied-lane explanation.

### CitationMap

Purpose: maps answer claims to exact source refs or snippet refs.

Owner: retrieval-gateway / RAG / Runtime verifier.

Reject if an answer can bypass citation verification for grounded tasks.

### DeniedLane

Purpose: records a retrieval lane that was denied or unavailable without
exposing forbidden content.

Examples: private chat lane, revoked memory lane, unavailable project source.

### TaintLabel

Purpose: tracks untrusted content from tool output, MCP provider, peer agent or
external documents.

Rules:

- taint propagates into ContextPackage;
- tainted content cannot become trusted instruction;
- tainted content cannot become ACTIVE memory without admission.

### ConflictSet

Purpose: groups conflicting source refs and forces abstain, clarification or
version resolution.

### EvidenceCoverageReport

Purpose: records which expected lanes were searched, denied, missing or used.

Used by eval and operator debug.

### CitationVerifierResult

Purpose: verifier output before final response.

States: pass, partial, fail, abstain_required.

## Memory Objects

### MemoryClaim

Purpose: durable interpretation candidate or ACTIVE memory claim.

Owner: memory-service.

Conceptual fields:

- memory_id;
- claim_type: preference, fact, task, decision, relation, consensus,
  knowledge_state, procedural, policy_like;
- subject;
- scope;
- source_refs;
- confidence;
- status;
- valid_from / valid_to;
- version refs;
- supersession refs;
- revocation refs.

Python may propose MemoryCandidate; it cannot create ACTIVE MemoryClaim.

### MemoryAdmissionDecision

Purpose: Go-owned decision that admits, rejects or sends a MemoryCandidate to
review.

States: auto_admit, reject, needs_review, admin_review.

Must include policy refs and reason codes.

### MemoryReviewTask

Purpose: human/operator review of risky memory.

Used for group/project/procedural/broad/sensitive/conflicting claims.

### MemoryScope

Purpose: explicit boundary for where a claim can be used.

Examples: user, group, project, org, skill, tenant.

Rule: broader scope requires stronger evidence and review.

### MemoryVersion

Purpose: records versioned update to a memory claim.

Must represent old state, new state, valid_from and supersedes refs.

### MemoryRevocationLedger

Purpose: durable record of source deletion, forget request, policy revocation or
manual memory invalidation.

Retrieval must not return revoked memory as active.

### MemorySupersessionChain

Purpose: links old and new memory claims for temporal reasoning.

Required for update questions and current-state answers.

### GroupConsensus

Purpose: explicit representation of group-level agreement or decision.

Must distinguish:

- proposed by;
- agreed by;
- objected by;
- decided_at;
- source refs;
- current status.

One user's statement is not GroupConsensus.

### KnowledgeState

Purpose: records who knows a claim and from which source.

Needed for questions like "does A know B's plan?".

### RelationMemory

Purpose: scoped relationship claim between participants.

Must carry source refs, time validity and confidence.

## Tool And MCP Objects

### ToolProvider

Purpose: registered tool or MCP provider identity.

Owner: mcp-gateway / governance.

Cannot act as permission authority.

### ProviderAttestation

Purpose: proof ref that a provider is known, reviewed or sandbox-only.

No secrets or credentials are model-visible.

### CapabilityLease

Purpose: bounded grant to prepare or use a capability under policy.

Fields at concept level: scope, actor, tenant, tool, expiry, risk tier, policy
decision ref.

### PreparedToolRef

Purpose: prepare/precheck result that can later be approved and executed.

Must expire and must be tied to schema hash, provider ref and policy decision.

### ToolSchemaHash

Purpose: stable hash of tool schema used during prepare.

Reject execution if schema changed after prepare.

### ToolOutputEnvelope

Purpose: wraps tool/MCP output with provenance, taint labels, validation status
and source refs.

Tool output is never trusted instruction.

### ToolRiskTier

Purpose: risk classification for read, dry-run, proposal-only, approval-required
or blocked/sandbox-only tools.

### ToolExecutionPolicy

Purpose: maps risk tier and actor context to required prepare, approval,
executor and audit behavior.

## Workflow And Action Objects

### ApprovalRequest

Purpose: user/operator approval request for high-risk actions or memory review.

Owner: workflow-service.

### ApprovalDecision

Purpose: approved, rejected, timed out or delegated decision.

Must be referenced by Runtime, action-executor and audit.

### ExecutionIntent

Purpose: final side-effect proposal after prepare and approval.

Owner: Agent Runtime / action-executor handoff boundary.

Cannot execute itself.

### ExecutionReceipt

Purpose: action-executor result ref.

Must include idempotency ref, state-diff ref and audit ref.

### StateDiffReport

Purpose: expected-vs-actual state comparison after approved action.

Already exists in fixture concept; production owner is action-executor/eval
integration.

### RepairRequest

Purpose: request for controlled repair when state diff or provider result is
wrong/incomplete.

### RedriveRequest

Purpose: replay or retry with preserved idempotency and audit lineage.

### CompensationPlan

Purpose: controlled compensating action plan when partial execution occurred.

Owner: workflow-service + action-executor, not Agent Runtime alone.

## AgentOps Objects

### AgentDefinition

Purpose: runnable agent configuration.

Minimum refs: owner, purpose, tenant scope, risk tier, release channel, model
policy, memory grants, tool grants, disable switch.

### SkillPackage

Purpose: versioned capability package.

Minimum refs: owner, version, allowed tools, required evidence, approval policy,
eval suite, rollback ref.

### AgentRelease

Purpose: release record for an AgentDefinition / SkillPackage version.

Must include eval report ref, approver, canary plan, rollback plan and kill
switch.

### ReleaseChannel

Purpose: draft, shadow, beta, production or disabled channel.

High-risk skills cannot skip review stages.

### BaselineApproval

Purpose: approval record for eval baseline refresh.

Baseline refresh is not automatic even if current run passes.

### FailureClassOwner

Purpose: owner for each failure class lifecycle.

Prevents failure taxonomy from becoming unowned strings.

### KillSwitch

Purpose: immediate disable path for agent or skill.

Owner: governance/control plane. Python cannot own kill switch.

### RollbackPlan

Purpose: defined return path to a previous safe release.

Must include compatibility and memory/tool grant implications.

### CanaryReport

Purpose: compares production canary or shadow results with offline baseline.

## Dataset And Eval Objects

### DatasetManifest

Purpose: source, license and import metadata for public or synthetic eval data.

### DatasetSnapshot

Purpose: immutable dataset snapshot ref with hash and import script version.

### DatasetSplit

Purpose: stable split metadata for train/dev/eval or fixture suites.

### EvalSuiteManifest

Purpose: declares cases, capability families, required fixtures, baseline and
blocked promotion rules.

### BaselineReport

Purpose: accepted baseline report for comparison.

### RegressionDelta

Purpose: structured delta between current report and baseline.

### BlockedPromotionReason

Purpose: reason a passing-looking run still cannot promote.

Examples: missing replay, unowned failure class, incomplete dataset license,
provider pollution, low coverage.

## Operator UX Objects

### MemoryInspectView

Purpose: authorized view of ACTIVE, superseded, revoked and review-pending
memory.

### EvidenceInspectView

Purpose: authorized view of source refs, citation map, denied lanes, conflict
set and taint labels.

### ReplayView

Purpose: low-sensitive run reconstruction for support and incident review.

### ApprovalConsole

Purpose: review queue for approval requests, memory review and repair/redrive.

### AgentReleaseConsole

Purpose: owner-facing view for release channel, baseline, canary, rollback and
kill switch.

### FailureReviewQueue

Purpose: triage queue for repeated failure classes and blocked promotion
reasons.

## Production Promotion Checklist

Before any object from this catalog becomes a production contract:

1. Accepted ADR exists.
2. Owner and consumer list are explicit.
3. Versioning and replay reader policy exist.
4. Permission and audit boundaries are explicit.
5. Fixture or dataset eval proves core behavior.
6. Integration preservation matrix is satisfied.
7. Operator UX path exists for inspection or remediation.
8. Rejection rules are encoded in tests or review gates.

## Immediate Next Step

Draft two ADR candidates first:

1. `ADR-Agent-Eval-Replay-Harness`
   - Converts EvalReport, ReplayBundle, BaselineReport, RegressionDelta,
     BlockedPromotionReason and ContractVersion requirements into the first
     release-gate ADR.

2. `ADR-Agent-Runtime-Workflow-Boundary`
   - Converts AgentRunStore, AgentCheckpoint, RuntimeWakeup, CancelToken,
     ResumeToken, ReplayIndex, BudgetLedger, ApprovalRequest, ApprovalDecision
     and workflow ownership rules into the first runtime-boundary ADR.

Do not start proto/schema/runtime implementation before these ADR candidates are
reviewed.
