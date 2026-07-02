# ADR Candidate: Agent Eval / Replay Harness

Status: candidate. Not accepted. Does not authorize production schema or
service implementation.

## Context

The isolated Agent skeleton can run synthetic and public-dataset-style evals end
to end and produce low-sensitive `EvalReport` and `ReplayBundle` outputs.

The missing production decision is whether eval/replay is merely a lab tool or
the first promotion gate for all Agent changes.

## Candidate Decision

Make Agent Eval / Replay the first required promotion gate for future Agent
architecture and implementation slices.

`EvalReport`, `ReplayBundle`, `BaselineReport`, `RegressionDelta`,
`BlockedPromotionReason`, failure taxonomy and baseline refresh review become
release evidence.

The first accepted ADR should freeze release-gate semantics and replay minimums,
not production wire schemas.

## Owned Objects

| Object | Owner | Notes |
| --- | --- | --- |
| EvalSuiteManifest | ai-eval / Agent Lab until integration | Declares suites, capability families and blocked promotion rules |
| EvalReport | ai-eval | Low-sensitive result surface |
| ReplayBundle | Agent Runtime + ai-eval | Low-sensitive refs, hashes, versions and decision lineage |
| BaselineReport | governance / ai-eval | Accepted comparison target |
| RegressionDelta | ai-eval | Current-vs-baseline comparison |
| BlockedPromotionReason | governance / ai-eval | Why a passing-looking run cannot promote |
| ContractVersion | producing service + governance | Required for future cross-service contracts |

## Required Versioning

Eval and replay artifacts must carry:

- harness version;
- suite version;
- adapter version;
- fixture or dataset snapshot refs;
- report schema version;
- replay reader policy ref.

Old reports must remain explainable through the declared compatibility window.

## Replay Minimums

ReplayBundle must include low-sensitive refs for:

- input hash;
- EvidencePack / ContextPackage;
- memory candidates and active memory refs;
- prepared tools;
- workflow decisions;
- execution receipts;
- state diff;
- audit;
- failure taxonomy.

Replay must not require raw prompt, raw provider body or raw IM message text.

## Release Gate Semantics

A candidate release is blocked if:

- required eval suite is missing;
- P0/P1 failure class appears;
- replay is incomplete for high-risk scenarios;
- baseline refresh is unapproved;
- dataset license or snapshot is not reproducible;
- failure class has no owner.

## Rejection Rules

Reject the ADR if it:

- treats fixture report fields as final production schemas;
- stores raw prompts or raw provider payloads as normal replay material;
- lets Python own production truth;
- allows promotion when replay or audit refs are missing.

## Next Evidence Needed

- Owner review for failure-class lifecycle.
- Baseline approval UX.
- Report retention and redaction policy.
- One contract-version bump rehearsal using fixture replay artifacts.
