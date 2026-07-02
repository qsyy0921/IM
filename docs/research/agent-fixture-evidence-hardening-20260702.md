# Agent Fixture Evidence Hardening

Date: 2026-07-02

Status: fixture-only evidence update for Agent ADR candidates. This is not an
accepted ADR, production contract, migration, service directory or runtime
implementation.

## Verdict

Conditionally passed for the Eval / Replay version-bump rehearsal slice.

The fixture harness now proves that an older low-sensitive ReplayBundle can be
read by a current replay reader policy when required refs, hashes, versions,
lineage and failure taxonomy are present. It also proves fail-closed behavior
for raw/unsupported legacy payload fields.

This does not complete the overall Agent architecture goal. No production
schema, service integration or runtime path is authorized.

## Added Evidence

Code:

- `ai/python/nexusim_ai_eval/replay_compatibility.py`
- `ai/python/tests/test_agent_eval_replay_compatibility.py`

Fixture:

- `ai/python/fixtures/agent_eval/replay_version_bump_rehearsal.json`

The helper verifies:

- old ReplayBundle remains low-sensitive;
- required replay refs are present;
- required hash, version, lineage, audit and failure taxonomy refs are present;
- replay is complete;
- side effects are not re-executed;
- raw payload is not returned;
- deprecated legacy fields fail closed, expire or are ignored only with an
  explicit deprecation record.

## Review Closure

This closes the fixture implementation portion of the Eval / Replay ADR review
condition:

- "Fixture implementation of the contract-version bump rehearsal."

It does not close:

- main integration owner review for failure-class lifecycle;
- admin/release UX owner for baseline approval;
- legal/security owner for final retention duration and redaction policy.

## Follow-Up Evidence

Runtime / Workflow fixture evidence is now recorded in
`docs/research/agent-runtime-workflow-fixture-evidence-20260702.md`.

Remaining fixture-only targets are Context denied-lane retention, Memory
revocation/category threshold, Tool prepare-expiry re-prepare and AgentOps
release-blocking proof.
