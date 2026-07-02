# ADR Candidate: Context / EvidencePack Boundary

Status: candidate. Not accepted. Does not freeze EvidencePack or ContextPackage
schema.

## Context

Agent answers, proposals and tool choices must be grounded in authorized
sources. The existing SDD and fixtures prove source coverage, conflict, stale
evidence, permission abstain, denied lanes, taint labels and citation repair.

The production risk is freezing a body schema too early or allowing Agent/RAG to
bypass the AI read boundary.

## Candidate Decision

EvidencePack remains the AI read boundary. ContextPackage is a derived runtime
input package and never a fact source.

The first ADR freezes lineage and verifier requirements, not final field names.

## Owned Objects

| Object | Owner | Purpose |
| --- | --- | --- |
| SourceVisibilityVersion | retrieval-gateway / policy-integrated source service | Proves source visibility at context build time |
| CitationMap | retrieval-gateway / RAG / Runtime verifier | Maps claims to exact refs |
| DeniedLane | retrieval-gateway | Records denied/unavailable lanes without leaking content |
| TaintLabel | retrieval-gateway / mcp-gateway / Runtime | Tracks untrusted external/tool/peer content |
| ConflictSet | retrieval / RAG / Runtime verifier | Forces abstain, clarification or version resolution |
| EvidenceCoverageReport | retrieval-gateway / ai-eval | Shows searched, denied, missing and used lanes |
| CitationVerifierResult | Runtime verifier / RAG | Blocks unsupported grounded answers |

## Boundary Rules

- Agent cannot directly read service private tables.
- Missing retrieval lanes must be visible.
- Unauthorized evidence must not enter ContextPackage.
- Tool/MCP/peer-agent output enters context as tainted content.
- Memory is labeled as memory, not current source truth.
- Grounded answers require citation verification or abstain.

## Versioning Requirements

Future EvidencePack contracts must carry:

- source visibility version refs;
- retrieval profile ref;
- source and lane refs;
- taint vocabulary version;
- citation verifier version;
- replay reader policy.

## Source Visibility And Denied-Lane Gate

SourceVisibilityVersion is required for every source or lane that influences a
grounded answer, proposal or tool choice.

The gate must record:

- source and lane refs;
- visibility or projection version;
- permission decision ref;
- freshness or temporal window ref;
- denied, unavailable or expired lane refs;
- redaction policy ref when content is hidden.

DeniedLane must be visible to Runtime, eval and operator review without exposing
forbidden content. If a required lane is denied, unavailable or expired, the
answer must abstain, clarify or mark the coverage gap; it must not pretend the
lane had no relevant facts.

## Citation Verifier Gate

Grounded answers and action-driving proposals require a CitationVerifierResult
before finalization.

The verifier must decide:

- supported;
- unsupported;
- conflicting;
- stale;
- denied or unavailable;
- insufficient coverage.

Unsupported, conflicting, stale or insufficient high-risk claims block answer
finalization or force clarification/abstain. A model answer cannot self-certify
its citations.

## Taint Vocabulary And Context Reuse

TaintLabel must carry a vocabulary version and source lane.

Tainted content includes:

- tool and MCP output;
- peer-agent output;
- external provider text;
- retrieved text with prompt-injection risk;
- memory derived from tainted or disputed sources.

Tainted content can be quoted, summarized or used as evidence only under the
reuse policy for that taint class. It cannot become trusted instruction, ACTIVE
memory or action authority by appearing in ContextPackage.

## Operator Evidence Inspection

Before production promotion, authorized operators must be able to inspect:

- EvidencePack and ContextPackage refs;
- selected source refs and citation map;
- denied, unavailable and expired lanes;
- conflict set and verifier result;
- taint labels and vocabulary version;
- redaction and replay reader policy refs.

This inspection path must not expose content the operator is not authorized to
see.

## Rejection Rules

Reject the ADR if:

- ContextPackage can include unauthorized evidence;
- answer candidates can bypass citation verification;
- denied lanes are invisible;
- tool output can become trusted instruction;
- memory replaces current source truth without labels;
- source visibility versions or taint vocabulary versions are absent;
- operators cannot inspect denied-lane or verifier decisions for high-risk
  failures.

## Next Evidence Needed

- Main integration owner review for retrieval-gateway source visibility.
- Product / security review for denied-lane reporting semantics.
- Fixture proof for visibility expiry, denied-lane retention and citation
  verifier blocking now exists in
  `docs/research/agent-context-evidence-fixture-evidence-20260702.md`;
  production integration remains blocked.
- Operator UX owner for evidence, citation, denied-lane and taint inspection.
