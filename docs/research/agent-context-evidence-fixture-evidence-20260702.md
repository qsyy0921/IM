# Agent Context / EvidencePack Fixture Evidence

Date: 2026-07-02

Status: fixture-only evidence update for the Context / EvidencePack ADR
candidate. This is not an accepted ADR, EvidencePack schema, ContextPackage
schema, service directory or production integration.

## Verdict

Conditionally passed for the Context / EvidencePack preservation rehearsal
slice.

The fixture harness now proves, with low-sensitive refs only, that source
visibility, denied-lane retention, citation verifier decisions, taint labels,
scope/version refs and audit lineage survive retrieval, runtime, RAG, MCP and
operator-inspection boundaries without exposing hidden source bodies.

This does not authorize production EvidencePack or ContextPackage fields.

## Added Evidence

Code:

- `ai/python/nexusim_ai_eval/context_evidence_preservation.py`
- `ai/python/tests/test_agent_eval_context_evidence_preservation.py`

Fixture:

- `ai/python/fixtures/agent_eval/context_evidence_preservation_rehearsal.json`

The helper verifies:

- visible, denied, unavailable and expired source states carry visibility,
  permission, temporal, redaction and audit refs;
- denied lanes retain low-sensitive source refs for audit and operator review
  without exposing bodies or entering ContextPackage;
- retrieval-gateway, Agent Runtime, RAG verifier and MCP boundary hops preserve
  required refs, scope, version, taint and audit lineage;
- unsupported, stale, denied or insufficient citations cannot finalize grounded
  answers or proposals;
- tainted tool and peer-agent content cannot become trusted instruction or
  ACTIVE memory;
- operator evidence inspection exposes only low-sensitive refs plus redaction
  and replay-reader policy refs.

## Review Closure

This closes the fixture evidence portion of the Context / EvidencePack ADR
review condition:

- "Later fixture hardening can add visibility expiry and denied-lane retention
  cases."

It also adds fixture evidence for cross-service source-ref, scope/version,
taint and audit-lineage preservation.

It does not close:

- main integration owner review for retrieval-gateway source visibility;
- product/security review for denied-lane reporting semantics;
- operator UX owner review for production evidence inspection;
- larger public RAG dataset thresholds before production release gates.

## Next Evidence Target

Next fixture-only evidence should focus on one of:

- Memory revocation and category-threshold proof;
- Tool prepare-expiry re-prepare proof;
- AgentOps release-blocking and kill-switch proof.
