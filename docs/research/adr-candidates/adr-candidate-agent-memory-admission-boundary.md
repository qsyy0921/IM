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

## Lifecycle States

Memory admission must distinguish:

- CANDIDATE;
- REJECTED;
- NEEDS_REVIEW;
- ACTIVE;
- SUPERSEDED;
- REVOKED;
- EXPIRED;
- QUARANTINED.

Only ACTIVE memory is normally retrieval-eligible. SUPERSEDED, REVOKED,
EXPIRED and QUARANTINED memory may remain as low-sensitive audit refs, but must
not influence normal context selection.

## Category-Specific Thresholds

Personal, group, project and procedural memory use the same admission machinery
but not the same threshold.

- Personal memory requires user scope, source refs and either explicit
  confirmation or repeated evidence.
- Group memory requires group, speaker, subject, audience, membership window and
  explicit decision or multi-party confirmation.
- Project memory requires project source refs, supersedes/revocation chain and
  review for material changes.
- Procedural memory requires SkillPackage or AgentDefinition version binding and
  invalidation on skill changes.
- Policy-like memory requires governed policy source refs and cannot be created
  from model summaries.

Broad scope, conflict, sensitive content or cross-group impact forces review.

## Revocation And Dependency Invalidation

MemoryRevocationLedger must link:

- revocation request or source deletion ref;
- authority decision ref;
- affected MemoryClaim refs;
- supersession or invalidation refs;
- retrieval eligibility change;
- audit ref.

If a source is deleted, revoked or no longer visible, dependent ACTIVE memory
must be revoked, downgraded or sent to review before it can be returned again.

## Admission Explanation And Operator UX

MemoryAdmissionDecision must be explainable from low-sensitive refs:

- source refs;
- candidate hash;
- scope;
- confidence and calibration refs;
- conflict/dedupe/supersession refs;
- reviewer or auto-policy decision refs;
- rejection or ACTIVE reason.

Authorized operators or users must be able to review, correct, revoke or forget
memory according to scope and policy. Python worker output cannot override that
decision path.

## Rejection Rules

Reject the ADR if:

- Python can write ACTIVE memory;
- memory lacks source refs;
- group memory lacks speaker/audience refs;
- revoked memory can be returned as active;
- memory is used to hide source-service failure;
- category thresholds are identical for personal, group, project and procedural
  memory;
- operator correction, revocation or forget paths are missing.

## Next Evidence Needed

- Main integration owner review for memory-service admission and retrieval.
- Fixture proof for category thresholds, revocation, dependency invalidation and
  stale-memory blocking.
- Audit explanation path review for ACTIVE decisions.
- Operator UX owner for review, correction, revocation and forget requests.
