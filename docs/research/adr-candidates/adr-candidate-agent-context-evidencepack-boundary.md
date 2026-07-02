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

## Rejection Rules

Reject the ADR if:

- ContextPackage can include unauthorized evidence;
- answer candidates can bypass citation verification;
- denied lanes are invisible;
- tool output can become trusted instruction;
- memory replaces current source truth without labels.

## Next Evidence Needed

- Real retrieval-gateway preservation proof for source refs and taint labels.
- Product acceptance of denied-lane reporting.
- Citation verifier gate policy for high-risk answers.
