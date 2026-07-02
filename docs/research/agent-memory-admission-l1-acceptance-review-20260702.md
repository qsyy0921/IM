# Agent Memory Admission L1 Acceptance Review

Date: 2026-07-02

Status: Agent Lab L1 self-review for
`adr-candidate-agent-memory-admission-boundary.md`. This is not an accepted
ADR, memory schema, MemoryCandidate schema, MemoryClaim schema, memory event
contract, production service directory, migration or runtime implementation.

## Verdict

Recommendation: accept the Memory Admission candidate for L1 ADR acceptance
review after the accepted Eval / Replay, Runtime / Workflow and Context /
EvidencePack L1 gates.

Do not approve production implementation from this review.

Reason: the candidate has no known P0/P1 gap inside Agent Lab scope after the
focused review and fixture-evidence hardening. It keeps Python candidate-only,
keeps ACTIVE memory admission Go-owned by memory-service, requires distinct
personal / group / project / procedural / policy-like thresholds, preserves
source, scope, version, revocation, explanation and operator refs, and rejects
memory as fallback for source-service failure.

## Playbook Result

```text
Candidate: Memory Admission Boundary
Verdict: recommend accept for L1 ADR review; defer implementation
Severity: none inside Agent Lab scope
Required signoffs checked: Agent Lab complete; Eval / Replay, Runtime / Workflow and Context / EvidencePack L1 accepted; main integration pending; memory, policy, workflow, audit and operator owners required before implementation
Agent Lab evidence checked: Memory SDD, ADR candidate, focused review, memory governance fixture evidence, object completeness, cross-service preservation, dataset reproducibility, operator governance and controlled implementation readiness
External blocker, if any: real memory admission smoke; production retrieval eligibility smoke; production retention/deletion policy; operator review/correction/forget UX; final memory event and claim field design
Rejected production shortcuts: Python ACTIVE memory writes, Runtime-owned admission, workflow-owned admission, source-less memory, identical category thresholds, revoked memory retrieval, memory fallback for unavailable source services and fixture-authorized schema promotion
Allowed next step: main integration may decide whether to accept this ADR candidate
Disallowed next step: production MemoryCandidate/MemoryClaim/event schema, memory-service API changes, migrations, service registry, real backend/model/MCP integration or ACTIVE memory implementation
```

## Evidence Reviewed

| Evidence | Result |
| --- | --- |
| `docs/sdd/agent-memory-admission.md` | Pass; memory is a governed subsystem, not prompt cache or model guess storage |
| `docs/research/adr-candidates/adr-candidate-agent-memory-admission-boundary.md` | Pass; Python emits candidates only and memory-service owns ACTIVE admission, rejection, review, supersession, revocation, expiry and audit |
| `docs/research/agent-memory-admission-adr-review-20260702.md` | Pass; earlier P1 findings were closed to fixture/review level or moved to explicit external conditions |
| `docs/research/agent-memory-fixture-evidence-20260702.md` | Pass; category thresholds, revocation dependency invalidation, retrieval eligibility, ACTIVE explanation and operator review/correction/forget controls are fixture-proven |
| `docs/research/agent-object-completeness-fixture-evidence-20260702.md` | Pass; MemoryCandidate, MemoryClaim and related memory objects have owner/lifecycle/version/permission/audit/replay/operator/rejection refs in the conceptual object catalog |
| `docs/research/agent-cross-service-preservation-fixture-evidence-20260702.md` | Pass; memory -> runtime boundary preserves role refs, scope refs, version refs, taint refs, audit lineage refs and replay-reader refs |
| `docs/research/agent-dataset-reproducibility-fixture-evidence-20260702.md` | Pass; memory public-dataset-style evidence remains public/synthetic and separate from product facts |
| `docs/research/agent-operator-governance-fixture-evidence-20260702.md` | Pass; memory governance requires inspect-and-act, not passive-only views |
| `docs/research/agent-controlled-implementation-readiness-fixture-evidence-20260702.md` | Pass; implementation remains blocked without accepted ADR, owner review and preservation evidence |

## Requirement Matrix

| L1 Requirement | Review Result | Notes |
| --- | --- | --- |
| Python is candidate-only | Pass | Python can produce candidate text, classification, confidence and hints, but cannot make final ACTIVE decisions |
| memory-service owns ACTIVE state | Pass | Admission, rejection, review outcome, supersession, revocation, expiry and retrieval eligibility stay memory-service owned |
| workflow-service owns wait only | Pass | Human review wait, timeout and escalation can be workflow-service owned, but workflow cannot make memory ACTIVE |
| Source-backed admission | Pass | Missing source refs reject; model statements alone are not durable memory |
| Category-specific thresholds | Pass | Personal, group, project, procedural and policy-like memory have distinct required refs and evidence thresholds |
| Group memory has speaker and audience | Pass | Group memory requires group, speaker, subject, audience and membership-window refs |
| Revocation affects retrieval | Pass | Revoked, expired, quarantined and superseded memory cannot influence normal retrieval |
| Memory cannot hide source failure | Pass | Memory is not a fallback for unavailable source services or missing retrieval lanes |
| ACTIVE decisions are explainable | Pass | Admission decisions require low-sensitive explanation refs, not raw transcript replay |
| Operator can inspect and act | Pass | Review, correction, revocation and forget controls preserve authority, redaction and audit refs |
| Production field shape remains unfrozen | Pass | Candidate freezes ownership and rejection rules only, not MemoryCandidate, MemoryClaim or event fields |

## Auto-Reject Audit

No auto-reject condition was found inside the current Agent Lab scope.

| Auto-Reject Rule | Result |
| --- | --- |
| Python writes ACTIVE memory | Not triggered; Python remains candidate-only |
| Runtime or workflow-service makes memory ACTIVE | Not triggered; memory-service owns final admission state |
| Memory lacks source refs | Not triggered; source-less memory rejects |
| Group memory lacks speaker/audience refs | Not triggered; speaker, audience and membership-window refs are required |
| Revoked memory can be returned as active | Not triggered in fixture evidence; real retrieval smoke remains external |
| Memory is used as fallback for source-service failure | Not triggered; fallback behavior is rejected |
| Category thresholds are identical | Not triggered; thresholds differ by category |
| Operator correction, revocation or forget path is missing | Not triggered in fixture evidence; production UX remains external |
| Fixture evidence authorizes production schema | Not triggered; every related doc rejects schema and service-contract freeze |

## Findings

| Severity | Finding | Disposition |
| --- | --- | --- |
| P0 | None inside Agent Lab scope | No production memory writes exist and Python final ownership remains rejected |
| P1 | None inside Agent Lab scope | Previous P1s for lifecycle, thresholds, revocation, explanation and operator UX are closed to fixture/review level |
| P2 | Calibration thresholds remain research baselines | Production readiness backlog, not an L1 ownership blocker |
| P2 | Production retention and deletion policy is not owner-approved | External product/legal/security/audit owner evidence before implementation |
| P2 | Real memory retrieval/admission smoke is missing | External L2/L3 evidence before implementation |

## External Deferrals

These items must be deferred to L2/L3 review before implementation:

- memory-service owner approval for MemoryCandidate, MemoryClaim, lifecycle,
  category threshold, ACTIVE decision, revocation and retrieval eligibility
  behavior;
- workflow-service owner approval for human review waits, timeout, escalation
  and redrive refs without workflow-owned ACTIVE memory;
- policy/security owner approval for scope rules, governed policy-source refs,
  sensitive content handling, cross-group leakage prevention and forget
  authority;
- audit owner approval for admission explanation refs, revocation ledger refs,
  memory-use audit and replay-reader behavior after source expiry or deletion;
- operator/product owner approval for review, correction, revocation and forget
  inspect-and-act UX;
- real-service preservation smoke proving memory refs, scope refs, version refs,
  taint labels, revocation refs, audit lineage and replay-reader refs survive
  memory-service, retrieval, Runtime, workflow and audit boundaries.

## Allowed After L1 Acceptance

If main integration accepts this ADR candidate, the only allowed next step is a
scoped implementation design for the Memory Admission boundary.

That design must name:

- memory-service ownership of ACTIVE admission, rejection, supersession,
  revocation, expiry and retrieval eligibility;
- workflow-service ownership only for durable human review waits;
- policy/security ownership for scope and governed policy-source decisions;
- memory object lifecycle and non-owner state;
- version, compatibility window and replay-reader policy;
- revocation/deletion and dependency invalidation behavior;
- operator review, correction, revocation and forget surfaces;
- fixture/public-dataset gates that must continue to pass.

## Still Disallowed

L1 acceptance must not create or freeze:

- production MemoryCandidate, MemoryClaim or memory event schema;
- memory-service, retrieval, workflow or audit service API changes;
- proto, OpenAPI, Kafka schema, migration or database tables;
- real PostgreSQL, Kafka, Redis, OpenSearch, model provider, MCP provider,
  workflow-service, memory-service or action-executor integration;
- raw prompt, raw provider body or full IM message archive as normal replay
  material;
- Python ownership of ACTIVE memory, final proposal, approval, execution,
  production source truth or audit archive;
- memory fallback for unavailable retrieval or source-of-truth services.

## Re-Review Result

After applying the ADR acceptance playbook, the Memory Admission candidate is
reviewable and has no known Agent Lab P0/P1 blocker.

Agent Lab recommends that main integration review this candidate fourth. If
main integration accepts it, Agent Lab should then prepare the Tool / MCP L1
review package. If main integration rejects or defers, Agent Lab should handle
that P0/P1 or owner-evidence request before moving on.
