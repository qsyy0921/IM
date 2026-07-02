# Agent Context / EvidencePack L1 Acceptance Review

Date: 2026-07-02

Status: Agent Lab L1 self-review for
`adr-candidate-agent-context-evidencepack-boundary.md`. This is not an accepted
ADR, EvidencePack schema, ContextPackage schema, production contract, service
directory, migration or runtime implementation.

## Verdict

Recommendation: accept the Context / EvidencePack candidate for L1 ADR
acceptance review after the accepted Eval / Replay and Runtime / Workflow L1
gates.

Do not approve production implementation from this review.

Reason: the candidate has no known P0/P1 gap inside Agent Lab scope after the
focused review and fixture-evidence hardening. It keeps EvidencePack as the AI
read boundary, keeps ContextPackage as derived runtime input, requires source
visibility, denied-lane, citation verifier, taint vocabulary, redaction,
replay-reader and operator inspection refs, and rejects direct private table
access or raw body replay.

## Playbook Result

```text
Candidate: Context / EvidencePack Boundary
Verdict: recommend accept for L1 ADR review; defer implementation
Severity: none inside Agent Lab scope
Required signoffs checked: Agent Lab complete; Eval / Replay and Runtime / Workflow L1 accepted; main integration pending; retrieval, audit and security owners required before implementation
Agent Lab evidence checked: Context/Evidence SDD, ADR candidate, focused review, context preservation fixture evidence, cross-service preservation evidence, dataset reproducibility, operator governance and controlled implementation readiness
External blocker, if any: real retrieval source-ref preservation smoke; denied-lane product/security owner approval; production evidence inspection UX; final EvidencePack/ContextPackage field design
Rejected production shortcuts: direct private table access, frozen body schema, citation self-certification, unauthorized context inclusion, trusted tool output, raw payload replay and Python-owned source truth
Allowed next step: main integration may decide whether to accept this ADR candidate
Disallowed next step: production EvidencePack/ContextPackage schema, retrieval/RAG service changes, migrations, service registry, real backend/model/MCP integration or field-shape freeze
```

## Evidence Reviewed

| Evidence | Result |
| --- | --- |
| `docs/sdd/agent-context-evidencepack.md` | Pass; EvidencePack remains the AI read boundary and ContextPackage is derived input, not a fact source |
| `docs/research/adr-candidates/adr-candidate-agent-context-evidencepack-boundary.md` | Pass; source visibility, denied lanes, citation verifier, taint vocabulary, operator inspection and rejection rules are named |
| `docs/research/agent-context-evidencepack-adr-review-20260702.md` | Pass; earlier P1 findings were closed or moved to explicit external conditions |
| `docs/research/agent-context-evidence-fixture-evidence-20260702.md` | Pass; visibility expiry, denied-lane retention, citation verifier blocking, taint preservation, redaction and audit lineage are fixture-proven |
| `docs/research/agent-cross-service-preservation-fixture-evidence-20260702.md` | Pass; retrieval -> runtime preservation keeps refs, scope, version, taint, audit lineage, redaction and replay-reader refs |
| `docs/research/agent-dataset-reproducibility-fixture-evidence-20260702.md` | Pass; public/synthetic dataset evidence remains separate from production facts |
| `docs/research/agent-operator-governance-fixture-evidence-20260702.md` | Pass; evidence and replay surfaces require inspect-and-act paths without body exposure |
| `docs/research/agent-controlled-implementation-readiness-fixture-evidence-20260702.md` | Pass; implementation remains blocked without accepted ADR, owner review and preservation evidence |

## Requirement Matrix

| L1 Requirement | Review Result | Notes |
| --- | --- | --- |
| EvidencePack is the AI read boundary | Pass | Agent Runtime cannot directly read private source tables or bypass retrieval/RAG boundaries |
| ContextPackage is not a fact source | Pass | It is derived runtime input and cannot replace service-owned source truth |
| Unauthorized evidence is excluded | Pass | Permission uncertainty fails closed and denied refs cannot enter ContextPackage body |
| Denied and unavailable lanes are visible | Pass | DeniedLane records are low-sensitive and visible to Runtime, eval and operator review without leaking content |
| Citation verification blocks unsupported claims | Pass | Grounded answers and action-driving proposals require CitationVerifierResult before finalization |
| Taint labels survive context reuse | Pass | Tool, MCP, peer-agent, provider and risky retrieved content keep vocabulary version and reuse policy refs |
| Memory stays labeled as memory | Pass | Memory cannot replace current source truth without source lineage and labels |
| Operator can inspect decisions | Pass | Evidence refs, source refs, citation map, denied lanes, conflicts, taint labels, redaction and replay-reader refs are inspectable |
| Production field shape remains unfrozen | Pass | Candidate freezes lineage/verifier requirements only, not EvidencePack or ContextPackage schema |

## Auto-Reject Audit

No auto-reject condition was found inside the current Agent Lab scope.

| Auto-Reject Rule | Result |
| --- | --- |
| Agent reads service private tables directly | Not triggered; retrieval-gateway/RAG/memory-service boundaries remain required |
| ContextPackage includes unauthorized evidence | Not triggered; denied or uncertain lanes are blocked from context body |
| Model answer self-certifies citations | Not triggered; citation verifier result is required |
| Tool/MCP/peer output becomes trusted instruction | Not triggered; taint labels and reuse policy refs are required |
| Boundary drops scope, version, taint, source ref or audit lineage | Not triggered in fixture evidence; real-service smoke remains external |
| Replay requires raw prompt or full message archive | Not triggered; low-sensitive refs and hashes are required |
| Fixture evidence authorizes production schema | Not triggered; every related doc rejects schema/field freeze |

## Findings

| Severity | Finding | Disposition |
| --- | --- | --- |
| P0 | None inside Agent Lab scope | No production source access exists and direct private-table bypass remains rejected |
| P1 | None inside Agent Lab scope | Previous P1s for visibility/denied lanes, citation verifier, taint vocabulary and operator inspection are closed to fixture/review level |
| P2 | Large public RAG thresholds remain research baselines | Production readiness backlog, not an L1 ownership blocker |
| P2 | Final denied-lane user/operator wording is not product-approved | External product/security owner evidence before implementation |
| P2 | Real retrieval/memory/MCP boundary preservation smoke is missing | External L2/L3 evidence before implementation |

## External Deferrals

These items must be deferred to L2/L3 review before implementation:

- retrieval-gateway owner approval for SourceVisibilityVersion, lane coverage,
  visibility expiry and projection freshness refs;
- product/security owner approval for denied-lane reporting semantics and
  operator-visible redaction behavior;
- audit owner approval for audit lineage and replay-reader behavior after
  source expiry or deletion;
- operator owner approval for production evidence, citation, denied-lane,
  conflict and taint inspection UX;
- real-service preservation smoke proving source refs, scope refs, version refs,
  taint labels, redaction refs and audit lineage survive retrieval, Runtime,
  RAG, memory, MCP and audit boundaries.

## Allowed After L1 Acceptance

If main integration accepts this ADR candidate, the only allowed next step is a
scoped implementation design for the Context / Evidence boundary.

That design must name:

- retrieval-gateway, RAG, Runtime, memory-service and audit-service ownership;
- source visibility and denied-lane refs;
- citation verifier decision policy;
- taint vocabulary and reuse policy;
- redaction and replay-reader policy;
- operator inspect-and-act surfaces;
- fixture/public-dataset gates that must continue to pass.

## Still Disallowed

L1 acceptance must not create or freeze:

- production EvidencePack or ContextPackage wire schema;
- retrieval, RAG, memory, MCP or audit service API changes;
- proto, OpenAPI, Kafka schema, migration or database tables;
- real PostgreSQL, Kafka, Redis, OpenSearch, model provider, MCP provider,
  workflow-service, memory-service or action-executor integration;
- raw prompt, raw provider body, denied source body or full IM message archive
  as normal replay material;
- Python ownership of production source truth, final proposal, ACTIVE memory,
  approval, execution or audit archive.

## Re-Review Result

After applying the ADR acceptance playbook, the Context / EvidencePack
candidate is reviewable and has no known Agent Lab P0/P1 blocker.

Agent Lab recommends that main integration review this candidate third. If main
integration accepts it, Agent Lab should then prepare the Memory Admission L1
review package. If main integration rejects or defers, Agent Lab should handle
that P0/P1 or owner-evidence request before moving on.
