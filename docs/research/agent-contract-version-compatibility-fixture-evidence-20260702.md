# Agent Contract Version Compatibility Fixture Evidence

Date: 2026-07-02

Status: fixture-only evidence update for Agent contract version compatibility.
This is not an accepted ADR, production contract, schema, service directory,
migration or runtime implementation.

## Verdict

Conditionally passed for the isolated contract-version compatibility rehearsal.

The fixture harness now proves that the future Agent contract targets named in
the shared appendix have low-sensitive compatibility-window, replay-reader,
redaction, deprecation, migration/backfill, preservation, audit, operator, eval
gate and rejection refs before any production contract can be promoted.

This does not authorize production implementation. It closes only the
fixture-only proof that compatibility governance is complete enough to review.

## Added Evidence

Code:

- `ai/python/nexusim_ai_eval/contract_version_compatibility.py`
- `ai/python/tests/test_agent_eval_contract_version_compatibility.py`

Fixture:

- `ai/python/fixtures/agent_eval/contract_version_compatibility_rehearsal.json`

## Covered Contract Targets

The rehearsal requires exactly one compatibility record for each target:

- EvidencePack;
- ContextPackage;
- MemoryCandidate;
- MemoryClaim;
- ToolIntent;
- PreparedToolRef;
- ApprovalDecision;
- ExecutionReceipt;
- EvalReport;
- ReplayBundle.

## Required Compatibility Shape

Each record must carry:

- producer owner and consumer owners;
- current and previous version refs;
- compatibility-window ref;
- replay-reader policy ref;
- redaction and deprecation policy refs;
- migration or backfill policy ref;
- preservation-matrix ref;
- audit, operator and eval-gate refs;
- rejection-condition refs.

The rehearsal rejects missing or duplicate target coverage, unsupported targets,
missing compatibility window, missing replay-reader policy, missing redaction /
deprecation / migration / preservation refs, unsupported reader version, replay
that needs archived bodies, inline-content retention for normal replay, removed
preservation refs, scope widening, dropped taint, lost audit lineage, Python
final ownership and fixture claims that production contracts are authorized.

## Review Closure

This closes a local Agent Lab P1 gap: existing replay version-bump evidence
proved one ReplayBundle reader path, and cross-service preservation proved refs
survive boundaries, but the package still lacked an executable matrix over all
future Agent contract targets.

Still not closed:

- accepted ADR review;
- main integration acceptance;
- owner review for real service integration;
- production schema/API compatibility tests;
- real-service preservation smoke;
- production operator UX.

## Checks

Focused verification:

```powershell
python -m pytest ai/python/tests/test_agent_eval_contract_version_compatibility.py -q
```

Expected result:

```text
8 passed
```

Full workspace checks are tracked in the handoff for this module.

## Next Evidence Target

Wait for main integration review or continue only focused fixture-only hardening
requested by review. This evidence can support ADR acceptance discussion, but
it cannot promote production contracts by itself.
