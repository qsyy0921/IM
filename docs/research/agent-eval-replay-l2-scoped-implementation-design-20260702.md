# Agent Eval / Replay L2 Scoped Implementation Design

Date: 2026-07-02

Status: L2 scoped implementation design draft for the Eval / Replay promotion
gate. This is not an accepted production ADR, proto, OpenAPI, Kafka schema,
migration, service directory, startup path, release automation or runtime
implementation.

## Verdict

Conditionally passed as the first L2 scoped design draft.

Rejected for implementation until main integration, ai-eval, Agent Runtime,
governance/release, audit/security, operator and SRE/incident owners review the
design and approve the required L3 real-service smoke plan.

Reason: L1 accepted Eval / Replay for reviewability only. This L2 design narrows
the next implementation discussion to one controlled slice: make Eval / Replay
release evidence reviewable through low-sensitive refs, without freezing wire
schemas or wiring production release automation.

## Scope

The scoped slice is the Eval / Replay promotion gate design.

It covers:

- low-sensitive EvalSuiteManifest, EvalRun, EvalReport and ReplayBundle refs;
- release-gate semantics for required suites, P0/P1 failures, replay gaps,
  baseline approval gaps and dataset reproducibility gaps;
- replay-reader policy and compatibility-window refs;
- failure-class lifecycle and owner workflow;
- retention, redaction and deletion behavior as owner-approved policy refs;
- operator inspect-and-act surfaces for report, replay, failure class, baseline
  approval, rollback and blocked promotion state;
- L3 real-service smoke requirements.

It does not cover:

- final EvalReport or ReplayBundle field shape;
- production storage table or API design;
- production release automation;
- baseline auto-refresh;
- real NexusIM IM data;
- production model, MCP, workflow, memory, executor or audit integration.

## Proposed Ownership

| Object / Decision | Owner | Cannot Be Owned By |
| --- | --- | --- |
| EvalSuiteManifest | ai-eval owner / governance | Python worker, Agent Runtime |
| EvalRun catalog record | ai-eval-service or approved Go eval module | Python worker, model output |
| EvalReport ref | ai-eval-service or approved Go eval module | Python worker as production source truth |
| ReplayBundle ref | Agent Runtime plus ai-eval reader policy | workflow-service, action-executor, Python worker |
| ReplayReaderPolicy | Agent Runtime plus audit/security | model output, fixture code |
| RegressionDelta | ai-eval-service | release UI, Python worker |
| BaselineApproval | governance / release owner | test runner, Python worker |
| FailureClassOwner | governance / operator owner | model output, passive dashboard |
| BlockedPromotionReason | governance / ai-eval-service | ad hoc release script |
| RetentionRedactionPolicy | audit/security owner | ai-eval harness |
| ReleaseGateDecision | governance / release owner | ai-eval harness, Agent Runtime |

Python remains fixture/eval/candidate-only. It can generate low-sensitive
candidate artifacts in isolated tests, but it cannot own production catalog
state, final release decisions, baseline approval, audit archive or replay
reader policy.

## Non-Owner State

The Eval / Replay slice must not own:

- production message, conversation, memory, workflow, execution or audit source
  truth;
- ACTIVE memory decisions;
- approval or action execution decisions;
- raw prompt, raw provider body, raw MCP payload, raw IM message or secret
  archive;
- release automation or deployment state;
- incident on-call policy.

## Candidate L2 Flow

```text
public/synthetic eval suite
-> Python isolated harness produces low-sensitive candidate report artifacts
-> ai-eval owner reviews catalog/recording design
-> EvalReportRef and ReplayBundleRef become release evidence candidates
-> governance performs dry-run ReleaseGateDecision over refs
-> operator inspects failures, baseline approval and replay completeness
-> L3 smoke proves refs survive real service boundaries
```

No production release is triggered by this flow. Until L4 controlled
implementation approval, governance decisions are dry-run evidence only.

## Release Gate Rules

A candidate AgentDefinition or SkillPackage cannot promote when any of these is
true:

- required suite is missing;
- suite version, adapter version or dataset snapshot ref is missing;
- P0/P1 failure class exists;
- high-risk case lacks complete replay refs;
- baseline refresh lacks explicit BaselineApproval;
- failure class lacks owner or regression fixture disposition;
- dataset manifest, license, split, import hash or deterministic report ref is
  missing;
- retention/redaction policy is missing or requires raw payload replay;
- operator cannot inspect and act on failure, baseline, replay or rollback
  state.

Unknown failures block promotion until classified.

## Version And Replay Policy

Every future Eval / Replay implementation design must carry low-sensitive refs
for:

- harness version;
- suite version;
- adapter version;
- dataset or fixture snapshot;
- report compatibility window;
- ReplayBundle reader policy;
- redaction and deprecation policy;
- migration or backfill policy when older reports remain readable;
- preservation-matrix ref;
- audit lineage ref.

Replay must be explainable from refs, hashes, versions and lineage. Normal
replay must not require raw prompts, provider bodies, IM messages, secrets or
full external tool payloads.

## L3 Real-Service Smoke Plan

Before implementation can start, owners must approve a real-service smoke plan
that proves the following with low-sensitive data only:

| Smoke | Boundary | Required Proof |
| --- | --- | --- |
| Record low-sensitive eval run | Python harness -> ai-eval-service / Go eval module | Raw prompts, provider bodies, message text and secrets are rejected |
| Read report for release evidence | ai-eval-service -> governance | Required suite, report, baseline and blocked-reason refs survive |
| Replay reader dry run | ai-eval-service -> Agent Runtime -> audit | ReplayBundle refs remain explainable without side effects |
| Baseline approval dry run | governance / release | Baseline refresh requires explicit approval and rollback refs |
| Failure-class owner workflow | governance / operator | P0/P1 owner, regression fixture and retirement refs are visible |
| Redaction and expiry behavior | audit/security -> replay reader | Expired or deleted source refs fail closed without raw archive fallback |
| Operator inspect-and-act | operator surface -> governance | Authorized operator can inspect and act; passive-only view fails |

These smokes must run on explicitly approved low-sensitive fixtures or local
synthetic records. They must not use real NexusIM IM data.

## Owner Review Checklist

| Owner | Must Approve |
| --- | --- |
| ai-eval owner | Catalog responsibilities, report refs, regression delta refs and no raw payload storage |
| Agent Runtime owner | ReplayBundle ownership, replay-reader policy and no side-effect replay |
| Governance / release owner | ReleaseGateDecision, BaselineApproval, BlockedPromotionReason and rollback refs |
| Audit / security owner | Retention class, redaction policy, deletion/expiry behavior and audit lineage |
| Operator owner | Inspect-and-act UX for report, replay, failure class, baseline, rollback and blocked promotion |
| SRE / incident owner | Report generation budget, eval retention budget, incident escalation refs and telemetry handoff |

## Test And Gate Plan

Existing Agent Lab gates that must continue to pass:

```powershell
python -m pytest ai/python/tests -q
python -m ruff check ai/python
python -m mypy ai/python/nexusim_ai_common ai/python/nexusim_ai_memory ai/python/nexusim_ai_eval ai/python/scripts
.\tools\check-python-ai-worker-boundary.ps1
.\tools\check-runbook-consistency.ps1
git diff --check
git diff --cached --check
```

Focused fixture gates to rerun for this slice:

```powershell
python -m pytest ai/python/tests/test_agent_eval_replay_compatibility.py -q
python -m pytest ai/python/tests/test_agent_eval_contract_version_compatibility.py -q
python -m pytest ai/python/tests/test_agent_eval_dataset_reproducibility.py -q
python -m pytest ai/python/tests/test_agent_eval_operator_governance.py -q
python -m pytest ai/python/tests/test_agent_eval_operational_readiness.py -q
python -m pytest ai/python/tests/test_agent_eval_controlled_implementation_readiness.py -q
python ai/python/scripts/run_agent_eval_report_matrix.py ai/python/fixtures/agent_eval/report_matrix_sample.json --matrix-out .tmp-agent-eval-matrix/matrix.json --approval-manifest-out .tmp-agent-eval-matrix/approval-manifest.json --force
```

## P0 / P1 / P2 Review

| Severity | Finding | Disposition |
| --- | --- | --- |
| P0 | None inside this L2 design draft | No production code, schema, service connection or real data is introduced |
| P1 | Owner review is missing | External blocker before implementation |
| P1 | L3 real-service smoke is missing | External blocker before implementation |
| P1 | Production operator UX is not approved | External blocker before implementation |
| P2 | Final retention duration, SLO and report latency budgets are not approved | Keep for owner review and operational readiness |
| P2 | Large benchmark thresholds remain research baselines | Keep out of production SLO until launch review |

## Auto-Reject Rules

Reject any Eval / Replay implementation proposal that:

- creates proto, OpenAPI, Kafka schema, migration, service directory or startup
  path from this L2 design alone;
- stores raw prompt, raw provider body, raw MCP payload, raw IM message or
  secret as normal replay material;
- lets Python own production EvalReport, ReplayBundle, baseline approval,
  release gate, audit archive or final business state;
- allows release when required suite, P0/P1 failure, replay, audit, baseline,
  failure owner or dataset reproducibility evidence is missing;
- uses fixture evidence as L3 real-service smoke;
- triggers release automation before governance and operator owners approve the
  control path.

## Decision

This design closes the Agent Lab-side L2 design gap for the first candidate:
Eval / Replay. It does not authorize implementation.

Next safe action after main integration review is either:

- owner review of this L2 design and requested revisions; or
- a second L2 scoped design for Runtime / Workflow, so the two highest-priority
  boundaries are both ready for owner review.
