# ADR Candidate: AgentOps / Governance Boundary

Status: candidate. Not accepted. Does not create production release pipelines.

## Context

AgentOps is the weakest production-readiness layer. SDDs define owner, release,
rollback and kill-switch concepts, but the isolated skeleton mainly proves eval
metadata and blocked promotion reasons.

The production risk is unmanaged prompts, skills, tools and memory grants moving
to production without an owner or rollback path.

## Candidate Decision

AgentOps is a release-control boundary first, not a new production
implementation service yet.

Every AgentDefinition and SkillPackage must have owner, purpose, risk tier,
release channel, eval suite, tool grants, memory grants, rollback ref and
disable switch before production.

P0/P1 eval failures, replay gaps and audit gaps block release.

## Owned Objects

| Object | Owner | Purpose |
| --- | --- | --- |
| AgentDefinition | agent-service / governance | Runnable agent configuration |
| SkillPackage | skill-registry | Versioned capability package |
| AgentRelease | governance / admin workflow | Promotion record |
| ReleaseChannel | governance | Draft, shadow, beta, production, disabled |
| BaselineApproval | governance / ai-eval | Accepted eval baseline refresh |
| FailureClassOwner | governance | Owner for failure taxonomy lifecycle |
| KillSwitch | governance / control plane | Disable agent or skill |
| RollbackPlan | governance | Return path to previous safe release |
| CanaryReport | governance / observability | Production comparison to baseline |

## Governance Rules

- No production agent without owner and disable switch.
- No high-risk skill without eval suite and approval metadata.
- No release with P0/P1 eval failure.
- No high-risk release with replay or audit gaps.
- Repeated production failure class becomes a regression fixture.
- Canary and shadow results must be comparable to offline baseline.

## Kill Switch Semantics

KillSwitch must define:

- owner;
- scope: AgentDefinition, SkillPackage, tool grant, memory grant or release
  channel;
- activation reason;
- propagation target;
- behavior for new runs;
- behavior for running or waiting runs;
- audit and rollback refs.

Kill switch activation must stop new eligible runs. Existing runs are cancelled,
drained or left to workflow-owned waits according to risk policy; Python worker
cannot own this decision.

## Release Pinning And Baseline Approval

AgentRelease must pin:

- AgentDefinition ref;
- SkillPackage refs;
- model/provider policy refs;
- tool and memory grants;
- EvalReport and ReplayBundle refs;
- BaselineApproval ref;
- rollback ref.

Baseline refresh requires explicit approval. Score improvements cannot silently
replace a baseline if failure classes, datasets, risk tier or required suites
changed.

## Failure-Class Owner Workflow

Every P0/P1 failure class must have:

- owner;
- severity;
- first-seen report ref;
- linked replay bundle;
- required regression fixture or explicit reason none is possible;
- closure and retirement rule.

Unowned P0/P1 classes block release and baseline refresh.

## Canary And Shadow Comparison

CanaryReport must compare production-like behavior to offline baselines using
compatible metrics:

- grounded quality;
- permission leakage;
- memory pollution or stale use;
- tool prepare/execute failures;
- approval and workflow failures;
- replay availability;
- cost and latency budget where available.

Canary P0/P1 regression rolls back or holds release. Shadow results cannot
promote production unless they are comparable to required offline suites.

## Operator UX Requirements

Authorized operators must inspect:

- memory candidates and ACTIVE memory status;
- evidence and citation refs;
- replay bundle for failed runs;
- approval, execution and state-diff refs;
- release channel and baseline;
- kill switch and rollback plan;
- unresolved failure classes.

## Rejection Rules

Reject the ADR if:

- AgentDefinition has no owner or disable switch;
- SkillPackage can use tools without eval/approval metadata;
- governance stores business truth or raw prompt archives;
- release can proceed with replay unavailable;
- kill switch cannot stop new runs;
- baseline refresh can occur without explicit approval;
- P0/P1 failure class has no owner or regression disposition;
- canary/shadow metrics are not comparable to offline baseline.

## Next Evidence Needed

- Main integration review for governance/control-plane kill-switch ownership.
- Admin UX owner for release pinning and baseline refresh approval.
- Fixture proof that unowned P0/P1 failure classes block release, baseline
  refresh requires approval, kill switch blocks new runs and canary / shadow
  metrics remain comparable is recorded in
  `docs/research/agentops-governance-fixture-evidence-20260702.md`.
- Production canary telemetry, on-call and incident escalation review.
