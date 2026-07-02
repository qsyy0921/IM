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
- kill switch cannot stop new runs.

## Next Evidence Needed

- Control-plane ownership for kill switch.
- Admin UX for release pinning and baseline refresh approval.
- Failure-class owner workflow.
- Canary/shadow comparison policy.
