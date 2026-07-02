# Agent Context / EvidencePack ADR Candidate Review

Date: 2026-07-02

Status: focused review of
`adr-candidate-agent-context-evidencepack-boundary.md`. This is not an accepted
ADR and does not freeze EvidencePack, ContextPackage, retrieval, RAG or citation
schemas.

## Verdict

Initial verdict: rejected for ADR acceptance.

Reason: the candidate correctly kept EvidencePack as the AI read boundary, but
left three P1 concerns as future evidence: real source/taint preservation,
denied-lane product semantics and enforceable citation verification.

After this pass: conditionally passed for main integration review as the third
ADR candidate.

The condition is that main integration must still accept the retrieval-gateway
and product ownership split. No EvidencePack body schema is authorized by this
review.

## P0/P1/P2 Findings

| Severity | Finding | Impact | Closure |
| --- | --- | --- | --- |
| P0 | None inside isolated Agent Lab scope | No production source access exists and private-table bypass remains rejected | Keep hard boundary unchanged |
| P1 | Source visibility and denied-lane behavior were not an acceptance gate | Context could look complete while hiding denied or unavailable lanes | Candidate now requires visibility refs, denied-lane refs and fail-closed behavior |
| P1 | Citation verifier policy was a future note | Grounded answers could bypass verifier results | Candidate now requires verifier result before final grounded answer/proposal |
| P1 | Taint vocabulary ownership was implicit | Tool, provider or peer-agent content could lose untrusted labels across services | Candidate now requires taint vocabulary version and preservation across lanes |
| P1 | Operator evidence inspection was implied but not required | Evidence governance could be unreviewable after incidents | Candidate now requires EvidenceInspectView-style inspect path |
| P2 | Large public RAG dataset thresholds remain undefined | Does not block candidate review, but blocks production release gates | Keep for eval hardening |

## Re-Review Checklist

| Gate | Result | Evidence |
| --- | --- | --- |
| AI read boundary | Pass | Candidate keeps EvidencePack as boundary and ContextPackage as derived input |
| Private table bypass rejection | Pass | Candidate rejects direct Agent access to service private tables |
| Source visibility | Pass after this pass | Candidate now requires SourceVisibilityVersion and expiry handling |
| Denied lanes | Pass after this pass | Candidate now requires denied/unavailable lanes to be visible without leaking content |
| Citation verifier | Pass after this pass | Candidate now requires verifier result for grounded answers and proposals |
| Taint propagation | Pass after this pass | Candidate now requires taint vocabulary version and cross-lane preservation |
| Operator evidence UX | Pass after this pass | Candidate now requires inspect path for evidence, citations, denied lanes and taint |
| Production boundary | Pass | Candidate still does not freeze body schema or create service contracts |

## Remaining Conditions

- Main integration must accept the retrieval-gateway and product UX ownership
  split.
- Later fixture hardening can add visibility expiry and denied-lane retention
  cases, but real-service integration remains blocked.
- Production EvidencePack and ContextPackage field names remain out of scope.

## Next Review Target

Review Memory Admission next. It must prove that Python remains candidate-only,
ACTIVE memory stays Go-owned, and personal/group/project/procedural memory
cannot become hidden fallback state.
