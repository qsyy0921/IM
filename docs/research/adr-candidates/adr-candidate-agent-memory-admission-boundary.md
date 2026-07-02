# ADR Candidate: Memory Admission Boundary

Status: candidate. Not accepted. Does not freeze MemoryCandidate, MemoryClaim or
memory event schemas.

## Context

The skeleton proves MemoryCandidate fixtures, group speaker/audience refs,
supersession, revocation, stale-memory blocking, dedupe, confidence calibration,
procedural migration and policy-source governance.

The production risk is memory pollution: model-generated summaries becoming
long-term truth without source, scope, review, version or revocation.

## Candidate Decision

Python emits MemoryCandidate only. Go-owned `memory-service` owns ACTIVE
admission, rejection, review state, supersession, revocation, expiry and audit.

Personal, group, project and procedural memory share the admission machinery but
use different evidence thresholds and review rules.

## Owned Objects

| Object | Owner | Purpose |
| --- | --- | --- |
| MemoryClaim | memory-service | Durable memory interpretation |
| MemoryAdmissionDecision | memory-service | Admit, reject or send to review |
| MemoryReviewTask | memory-service + workflow-service | Human review for risky claims |
| MemoryScope | memory-service / policy-service | Where claim may be used |
| MemoryVersion | memory-service | Versioned update and validity window |
| MemoryRevocationLedger | memory-service / audit | Forget, source deletion and revocation |
| MemorySupersessionChain | memory-service | Old/new state chain |
| GroupConsensus | memory-service | Group-level decision, agreement and objections |
| KnowledgeState | memory-service | Who knows what and since when |
| RelationMemory | memory-service | Scoped relation claim between participants |

## Admission Rules

- Missing source refs reject.
- Forbidden scope rejects.
- Broad scope requires stronger evidence and review.
- Group memory must carry group, speaker, subject and audience refs.
- One user's preference in a group is not group preference.
- GroupConsensus requires explicit decision or multi-party confirmation.
- Policy-like memory must come from governed policy sources.
- Revoked or deleted sources invalidate or downgrade dependent memory.

## Boundary Rules

- Python cannot make final ACTIVE decisions.
- Runtime cannot make memory ACTIVE.
- workflow-service cannot make memory ACTIVE by itself.
- Memory cannot become fallback for unavailable source services.
- Retrieval must respect memory state, scope, version and revocation.

## Rejection Rules

Reject the ADR if:

- Python can write ACTIVE memory;
- memory lacks source refs;
- group memory lacks speaker/audience refs;
- revoked memory can be returned as active;
- memory is used to hide source-service failure.

## Next Evidence Needed

- Real memory-service retrieval proof for scope/version/revocation.
- Audit explanation path for ACTIVE decisions.
- Operator UX for review, correction and forget requests.
- Group/project/procedural fixtures promoted into eval suites.
