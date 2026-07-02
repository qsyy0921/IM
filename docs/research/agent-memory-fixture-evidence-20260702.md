# Agent Memory Admission Fixture Evidence

Date: 2026-07-02

Status: fixture-only evidence update for the Memory Admission ADR candidate.
This is not an accepted ADR, memory event schema, service directory or
production memory integration.

## Verdict

Conditionally passed for the Memory Admission governance rehearsal slice.

The fixture harness now proves, with low-sensitive refs only, that Python stays
candidate-only, memory-service owns ACTIVE decisions, category-specific
thresholds differ across personal / group / project / procedural / policy-like
memory, revoked memory is removed from retrieval eligibility and operator
review / correction / forget paths remain auditable.

This does not authorize production MemoryCandidate, MemoryClaim or memory event
fields.

## Added Evidence

Code:

- `ai/python/nexusim_ai_eval/memory_admission_governance.py`
- `ai/python/tests/test_agent_eval_memory_admission_governance.py`

Fixture:

- `ai/python/fixtures/agent_eval/memory_admission_governance_rehearsal.json`

The helper verifies:

- Python AI Worker cannot own ACTIVE decisions, retrieval eligibility or final
  admission state;
- personal, group, project, procedural and policy-like memory use distinct
  required refs and threshold policy refs;
- group memory requires speaker, audience, membership-window and
  decision/confirmation refs;
- project memory requires supersedes/conflict and review-policy refs;
- procedural memory requires skill version and migration/invalidation refs;
- policy-like memory requires governed policy source and policy owner refs;
- revoked memory invalidates dependent memories and cannot be retrieved;
- non-ACTIVE lifecycle states are not normal retrieval eligible;
- ACTIVE decision explanations are reconstructable from low-sensitive refs and
  do not require raw text;
- operator review, correction and forget paths preserve authority, redaction and
  audit refs without Python override.

## Review Closure

This closes the fixture evidence portion of the Memory Admission ADR review
condition:

- "Fixture hardening should later prove category threshold and revocation
  behavior in repeatable eval suites."

It also adds fixture evidence for ACTIVE-decision explanations and operator
review / correction / forget governance.

It does not close:

- main integration owner review for memory-service admission and retrieval;
- workflow-service owner review for production review waits;
- legal / product owner review for production retention and deletion policy;
- production threshold acceptance from larger public memory datasets.

## Next Evidence Target

Tool / MCP fixture evidence is now recorded in
`docs/research/agent-tool-mcp-fixture-evidence-20260702.md`.

AgentOps fixture evidence is now recorded in
`docs/research/agentops-governance-fixture-evidence-20260702.md`.

Next work should focus on main integration review or memory calibration /
dataset reproducibility hardening.
