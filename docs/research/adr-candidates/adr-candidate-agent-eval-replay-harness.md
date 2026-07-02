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

## Failure-Class Lifecycle

Every failure class used by a release gate must have:

- severity: P0, P1 or P2;
- owner;
- first-seen report ref;
- disposition: block, allow with condition, or backlog;
- regression fixture requirement;
- retirement rule.

Unknown failures block promotion until classified. Repeated production
incidents must become regression fixtures before the next high-risk release.

## Baseline Approval

Baseline refresh is a governed decision, not a test update.

BaselineApproval must record:

- old baseline ref;
- new baseline ref;
- changed suites and failure classes;
- reviewer;
- reason;
- blocked promotion reasons that were waived or rejected;
- rollback ref.

Unreviewed baseline changes block promotion even when aggregate scores improve.

## Retention And Redaction

EvalReport and ReplayBundle retention must use low-sensitive refs by default.

Retention policy must define:

- retention class;
- redaction policy ref;
- allowed archive fields;
- forbidden raw payload fields;
- replay reader behavior after source expiry;
- audit owner for deletion or expiry.

Normal replay must not depend on raw prompt text, raw provider bodies, raw IM
messages, secrets or full external MCP payload archives.

## Contract-Version Bump Rehearsal

Before this ADR can authorize production contract design, the fixture harness
must demonstrate one version-bump rehearsal:

1. Produce an older EvalReport / ReplayBundle fixture.
2. Read it with the current replay reader policy.
3. Verify required refs, hashes, versions and failure class remain explainable.
4. Verify removed or unsupported fields fail closed with a deprecation or expiry
   reason.
5. Verify no raw payload is required for the replay explanation.

## Rejection Rules

Reject the ADR if it:

- treats fixture report fields as final production schemas;
- stores raw prompts or raw provider payloads as normal replay material;
- lets Python own production truth;
- allows promotion when replay or audit refs are missing.

## Next Evidence Needed

- Main integration owner review for failure-class lifecycle.
- Admin/release UX owner for baseline approval.
- Legal/security owner for final retention duration and redaction policy.
- Fixture implementation of the contract-version bump rehearsal.
