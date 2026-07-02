# Cross-Service Versioning, Replay And Governance Appendix

Status: shared appendix for Agent ADR candidates. Not a production contract.

## Required Contract Version Envelope

Every future Agent contract candidate must define:

- `contract_name`
- `schema_version`
- `semantic_version`
- `producer_owner`
- `consumer_owners`
- `compatibility_window`
- `replay_reader_policy`
- `redaction_policy`
- `deprecation_policy`
- `migration_or_backfill_policy`, if persisted
- `rejection_conditions`

This envelope applies to EvidencePack, ContextPackage, MemoryCandidate,
MemoryClaim, ToolIntent, PreparedToolRef, ApprovalDecision, ExecutionReceipt,
EvalReport and ReplayBundle.

## Compatibility Window

An Agent object version is not production-ready unless old artifacts remain
readable and explainable for the declared compatibility window.

Minimum requirement:

- old `ReplayBundle` artifacts remain replayable by refs and hashes;
- old `EvidencePack` refs can be interpreted or explicitly marked expired;
- removed fields have a deprecation record;
- readers fail closed when a required version is unsupported.

## Replay Reader Policy

Replay must reconstruct decisions from low-sensitive refs, hashes, versions and
decision lineage. Normal replay must not require:

- raw prompts;
- raw provider bodies;
- raw IM messages;
- secrets;
- full external MCP payload archives.

Replay readers must be able to explain:

- what the Agent was allowed to see;
- which evidence was denied or unavailable;
- which memory version was active;
- which tool prepare and approval refs existed;
- whether state diff matched expected outcome;
- why a promotion or action was blocked.

## Integration Preservation Matrix

| Boundary | Must Preserve |
| --- | --- |
| retrieval-gateway -> Agent Runtime | EvidencePack ref, source refs, visibility versions, denied lanes, taint labels |
| memory-service -> Agent Runtime | memory refs, scope, version, status, revocation/supersession refs |
| mcp-gateway -> Agent Runtime | prepared ref, provider ref, schema hash, lease, attestation, taint labels |
| workflow-service -> Agent Runtime | approval decision ref, timeout ref, wakeup id, resume/cancel correlation |
| action-executor -> Eval/Replay | execution ref, idempotency ref, state-diff ref, audit ref, repair/redrive refs |
| audit-service -> AgentOps | low-sensitive archive refs, actor refs, policy refs, retention refs |

Promotion is rejected if a boundary drops a required ref even when the happy path
answer is correct.

## Governance Requirements

Every production Agent release must have:

- owner and on-call;
- release channel;
- eval suite manifest;
- baseline approval;
- rollback plan;
- kill switch;
- failure-class owner;
- replay retention policy;
- canary or shadow comparison plan for high-risk changes.

P0/P1 eval failures, replay gaps and audit gaps block promotion.
