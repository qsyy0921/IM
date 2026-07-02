# Agent Memory Admission ADR Candidate Review

Date: 2026-07-02

Status: focused review of `adr-candidate-agent-memory-admission-boundary.md`.
This is not an accepted ADR and does not freeze MemoryCandidate, MemoryClaim or
memory event schemas.

## Verdict

Initial verdict: rejected for ADR acceptance.

Reason: the candidate correctly kept Python candidate-only and ACTIVE memory
Go-owned, but left four P1 concerns as future evidence: category-specific
thresholds, revocation retrieval proof, ACTIVE-decision audit explanation and
operator correction/forget UX.

After this pass: conditionally passed for main integration review as the fourth
ADR candidate.

The condition is that main integration must still accept memory-service and
workflow-service ownership for review state. No production memory schema is
authorized.

## P0/P1/P2 Findings

| Severity | Finding | Impact | Closure |
| --- | --- | --- | --- |
| P0 | None inside isolated Agent Lab scope | Python remains candidate-only and real IM data remains blocked | Keep hard boundary unchanged |
| P1 | Memory lifecycle states were implicit | Candidate could skip review or hide revocation/supersession | Candidate now names lifecycle states and retrieval eligibility |
| P1 | Personal/group/project/procedural thresholds were underspecified | Broad or procedural memory could be admitted with weak evidence | Candidate now requires category-specific admission thresholds |
| P1 | Revocation and source deletion were future evidence | Revoked memory could still influence retrieval or replay | Candidate now requires revocation ledger and dependency invalidation |
| P1 | ACTIVE decision audit and operator UX were future notes | Users/operators could not explain, correct or forget memory | Candidate now requires explanation and review/correction/forget paths |
| P2 | Calibration thresholds remain research baselines | Does not block candidate review, but blocks production admission | Keep for fixture hardening |

## Re-Review Checklist

| Gate | Result | Evidence |
| --- | --- | --- |
| Python candidate-only | Pass | Candidate rejects Python ACTIVE decisions |
| Go-owned ACTIVE memory | Pass | Candidate keeps admission, rejection, revocation and audit in memory-service |
| Lifecycle state | Pass after this pass | Candidate now names candidate/review/active/superseded/revoked/expired states |
| Category thresholds | Pass after this pass | Candidate now distinguishes personal, group, project and procedural thresholds |
| Revocation retrieval | Pass after this pass | Candidate now requires revoked/deleted source invalidation |
| Audit explanation | Pass after this pass | Candidate now requires low-sensitive admission explanation refs |
| Operator memory UX | Pass after this pass | Candidate now requires review, correction and forget inspect path |
| Production boundary | Pass | Candidate still does not freeze schemas or create service contracts |

## Remaining Conditions

- Main integration must accept memory-service ownership and workflow-service wait
  ownership.
- Fixture hardening should later prove category threshold and revocation
  behavior in repeatable eval suites.
- Production retention and deletion policy remain integration-scope decisions.

## Next Review Target

Review Tool / MCP next. It must prove providers remain untrusted, capability
leases are bounded and high-risk actions still require prepare, approval and
action-executor execution.
