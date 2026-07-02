# Agent Cross-Service Preservation Fixture Evidence

Date: 2026-07-02

Status: fixture-only evidence update for the cross-service preservation matrix.
This is not an accepted ADR, production contract, schema, migration, service
directory or backend integration.

## Verdict

Conditionally passed for the cross-service preservation rehearsal slice.

The fixture harness now proves, with low-sensitive refs only, that the shared
appendix preservation matrix has executable evidence across retrieval, memory,
MCP, workflow, executor and audit lanes before any production integration
design can be promoted.

## Added Evidence

Code:

- `ai/python/nexusim_ai_eval/cross_service_preservation.py`
- `ai/python/tests/test_agent_eval_cross_service_preservation.py`

Fixture:

- `ai/python/fixtures/agent_eval/cross_service_preservation_rehearsal.json`

The helper verifies:

- all required boundaries are represented: retrieval -> runtime, memory ->
  runtime, MCP -> runtime, workflow -> runtime, executor -> eval/replay and
  audit -> AgentOps;
- each boundary keeps the role-specific refs required by the appendix;
- scope refs, version refs, taint refs and audit-lineage refs are preserved;
- compatibility-window, replay-reader, redaction and taint-policy refs are
  present on every boundary;
- promotion blocks missing boundaries, dropped refs, widened scope, raw payload
  exposure, production data use and side-effect re-execution.

## Review Closure

This closes the fixture-only portion of the cross-service preservation P1 gap:

- the preservation ladder now has an executable rung after document review and
  version-bump replay rehearsal;
- the matrix no longer relies only on prose to prove refs survive boundaries;
- a happy-path answer cannot pass promotion when a required preservation ref is
  dropped.

It does not close:

- real-service preservation smoke;
- accepted ADR promotion;
- production schema or API contracts;
- integration with actual retrieval, memory, MCP, workflow, executor or audit
  services.

## Next Evidence Target

The remaining next step should be main integration review of the ADR candidate
package and fixture evidence, or a focused P0/P1 requested by that review. Do
not promote production contracts from this evidence alone.
