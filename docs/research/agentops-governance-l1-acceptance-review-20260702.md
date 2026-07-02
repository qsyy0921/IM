# AgentOps / Governance L1 Acceptance Review

Date: 2026-07-02

Status: Agent Lab L1 self-review for
`adr-candidate-agentops-governance-boundary.md`. This is not an accepted ADR,
release pipeline, production admin console, schema, migration, service
directory, control-plane API or runtime implementation.

## Verdict

Recommendation: accept the AgentOps / Governance Boundary candidate for L1 ADR
acceptance review after the accepted Eval / Replay, Runtime / Workflow,
Context / EvidencePack, Memory Admission and Tool / MCP L1 gates.

Do not approve production implementation from this review.

Reason: the candidate has no known P0/P1 gap inside Agent Lab scope after the
focused review and fixture-evidence hardening. It keeps AgentOps as a
release-control and governance boundary, not business truth. It requires
release owners, pinned release inputs, explicit baseline approval, eval/replay
and audit gates, rollback refs, kill-switch refs, failure-class owners,
operator inspect-and-act surfaces and canary/shadow comparability before
production promotion can be considered.

## Playbook Result

```text
Candidate: AgentOps / Governance Boundary
Verdict: recommend accept for L1 ADR review; defer implementation
Severity: none inside Agent Lab scope
Required signoffs checked: Agent Lab complete; Eval / Replay, Runtime / Workflow, Context / EvidencePack, Memory Admission and Tool / MCP L1 accepted; main integration pending; governance, release, operator, audit and SRE/incident owners required before implementation
Agent Lab evidence checked: AgentOps SDD, ADR candidate, focused review, AgentOps governance fixture evidence, operator governance, operational readiness, object completeness, cross-service preservation, contract compatibility and controlled implementation readiness
External blocker, if any: governance/control-plane owner acceptance; production release/admin UX owner review; production telemetry/on-call/incident policy; real rollback/kill-switch smoke; final release-gate field design
Rejected production shortcuts: passive-only governance, release without owner/disable switch, silent baseline refresh, open P0/P1 release, missing rollback or kill switch, unowned failure class, incomparable canary/shadow report, Python release override and fixture-authorized production release automation
Allowed next step: main integration may decide whether to accept this ADR candidate
Disallowed next step: production release pipeline, admin console, control-plane API changes, schema, migrations, service registry, real backend/provider integration, release automation or runtime implementation
```

## Evidence Reviewed

| Evidence | Result |
| --- | --- |
| `docs/sdd/agent-governance-agentops.md` | Pass; governance controls release channels, owner metadata, eval gates, rollback, kill switches and incident surfaces without becoming business truth |
| `docs/research/adr-candidates/adr-candidate-agentops-governance-boundary.md` | Pass; AgentOps is a release-control boundary and every production candidate requires owner, pinned refs, eval/replay/audit gates, rollback and disable switch |
| `docs/research/agentops-governance-adr-review-20260702.md` | Pass; earlier P1 findings were closed to fixture/review level or moved to explicit external owner conditions |
| `docs/research/agentops-governance-fixture-evidence-20260702.md` | Pass; release blocking, baseline approval, kill-switch propagation, failure-class owner workflow and canary/shadow comparability are fixture-proven |
| `docs/research/agent-operator-governance-fixture-evidence-20260702.md` | Pass; operator governance requires inspect-and-act paths for memory, evidence, replay, approval, release, failure class, kill switch and rollback |
| `docs/research/agent-operational-readiness-fixture-evidence-20260702.md` | Pass; runtime, spend, timeout, latency, retention, canary telemetry and incident escalation budgets require owner, measurement, operator, audit, release-gate and failure-class refs |
| `docs/research/agent-object-completeness-fixture-evidence-20260702.md` | Pass; AgentRelease, BaselineApproval, FailureClassOwner, KillSwitch, RollbackPlan and CanaryReport have owner/lifecycle/version/permission/audit/replay/operator/rejection refs in the conceptual object catalog |
| `docs/research/agent-cross-service-preservation-fixture-evidence-20260702.md` | Pass; release, rollback, audit, replay, runtime, MCP, workflow, executor and memory refs are expected to preserve role, scope, version, taint and audit-lineage refs across boundaries |
| `docs/research/agent-contract-version-compatibility-fixture-evidence-20260702.md` | Pass; governance must preserve compatibility windows, replay-reader policy, redaction, deprecation, migration, audit and operator refs before production contract review |
| `docs/research/agent-controlled-implementation-readiness-fixture-evidence-20260702.md` | Pass; implementation remains blocked without accepted ADR, owner review, preservation evidence and operator gates |

## Requirement Matrix

| L1 Requirement | Review Result | Notes |
| --- | --- | --- |
| AgentOps is release control, not business truth | Pass | Governance can gate, pin, disable and roll back agent releases; it cannot own message, memory, execution, approval or audit source truth |
| Production candidate requires owner and disable switch | Pass | AgentDefinition and SkillPackage cannot promote beyond draft-like scope without owner and disable/kill-switch refs |
| Release inputs are pinned | Pass | AgentRelease must pin AgentDefinition, SkillPackage, model/provider policy, tool grant, memory grant, EvalReport, ReplayBundle, BaselineApproval and rollback refs |
| Eval/replay/audit gates block release | Pass | P0/P1 failures, replay gaps and audit gaps are release blockers |
| Baseline refresh is explicit | Pass | Score improvement cannot silently replace a baseline when datasets, failure classes, risk tier or required suites changed |
| Kill switch has owner, scope and propagation | Pass | KillSwitch requires owner, activation reason, target scope, propagation target, running-run policy, audit and rollback refs |
| Failure classes are owned | Pass | P0/P1 failure classes require owner, severity, first-seen report, replay bundle and regression fixture or explicit no-fixture reason |
| Canary/shadow reports are comparable | Pass | Promotion requires comparable metrics against required offline baselines and rollback/hold behavior for P0/P1 regressions |
| Operators can inspect and act | Pass | High-risk governance states cannot be passive-only, body-exposing, unaudited, unauthorized or Python-overridable |
| Operational budgets gate promotion | Pass | Runtime step, model spend, provider timeout, retrieval latency, eval retention, canary telemetry and incident escalation budgets need measurable owner-approved refs |
| Production field shape remains unfrozen | Pass | Candidate freezes ownership and rejection rules only, not release, admin, SLO, incident, telemetry or control-plane schemas |

## Auto-Reject Audit

No auto-reject condition was found inside the current Agent Lab scope.

| Auto-Reject Rule | Result |
| --- | --- |
| AgentOps stores business truth or raw prompt/provider archives | Not triggered; governance stores release and control refs only |
| Production release can proceed without owner or disable switch | Not triggered; owner and disable/kill-switch refs are required |
| Release can proceed with open P0/P1, replay gap or audit gap | Not triggered; gates fail closed in fixture evidence |
| Baseline refresh can silently replace prior baseline | Not triggered; explicit BaselineApproval is required |
| Kill switch cannot stop new eligible runs | Not triggered in fixture evidence; real propagation smoke remains external |
| P0/P1 failure class lacks owner or regression disposition | Not triggered; unowned classes block release and baseline refresh |
| Canary/shadow metrics are not comparable to offline baseline | Not triggered; incomparable reports cannot promote |
| Operator governance is passive-only | Not triggered; inspect-and-act surfaces are required |
| Python worker can override release, rollback or kill switch | Not triggered; Python and model output cannot own governance decisions |
| Fixture evidence authorizes production release automation | Not triggered; every related doc rejects production pipeline/admin promotion |

## Findings

| Severity | Finding | Disposition |
| --- | --- | --- |
| P0 | None inside Agent Lab scope | No production release pipeline, control-plane API or admin console exists |
| P1 | None inside Agent Lab scope | Previous P1s for kill switch, release pinning, baseline approval, failure owner workflow and canary/shadow comparison are closed to fixture/review level |
| P2 | Production admin UX is not owner-approved | External operator/release owner evidence before implementation |
| P2 | Production SLO, telemetry, on-call and incident escalation are not approved | External SRE/governance/audit evidence before implementation |
| P2 | Real kill-switch propagation, rollback and canary telemetry smoke is missing | External L2/L3 evidence before implementation |

## External Deferrals

These items must be deferred to L2/L3 review before implementation:

- governance/control-plane owner approval for AgentRelease, ReleaseChannel,
  BaselineApproval, FailureClassOwner, KillSwitch, RollbackPlan and CanaryReport
  ownership;
- release/admin owner approval for release pinning, baseline approval, channel
  transition, hold, rollback and disable workflows;
- operator owner approval for inspect-and-act UX across release, failure class,
  kill switch, rollback, memory, evidence, replay and approval surfaces;
- audit owner approval for release decision refs, failure-class refs,
  rollback/kill-switch refs, redaction policy and replay-reader behavior;
- SRE/incident owner approval for telemetry, capacity, cost, latency, retention,
  on-call escalation and incident response budgets;
- real-service smoke proving release refs, rollback refs, kill-switch refs,
  failure-class refs, audit-lineage refs, replay-reader refs, version refs and
  operator action refs survive governance, runtime, workflow, eval, audit,
  MCP/action-executor and memory/retrieval boundaries.

## Allowed After L1 Acceptance

If main integration accepts this ADR candidate, the only allowed next step is a
scoped implementation design for the AgentOps / Governance boundary.

That design must name:

- the owning governance/control-plane module or service boundary;
- release and admin owner responsibilities;
- object owner and non-owner state for AgentRelease, ReleaseChannel,
  BaselineApproval, FailureClassOwner, KillSwitch, RollbackPlan and CanaryReport;
- version, compatibility window and replay-reader policy;
- release gate, baseline refresh, canary/shadow and rollback rules;
- permission, audit, redaction and operator inspect-and-act boundaries;
- kill-switch propagation and running-run behavior;
- incident, SLO, capacity and telemetry evidence that still belongs to
  production owners;
- fixture/public-dataset gates that must continue to pass.

## Still Disallowed

L1 acceptance must not create or freeze:

- production release pipeline, admin console or control-plane API;
- AgentRelease, ReleaseChannel, BaselineApproval, FailureClassOwner,
  KillSwitch, RollbackPlan, CanaryReport, SLO or incident schemas;
- proto, OpenAPI, Kafka schema, migration or database tables;
- real PostgreSQL, Kafka, Redis, OpenSearch, model provider, MCP provider,
  workflow-service, memory-service, action-executor or audit-service integration;
- production canary telemetry, on-call, SLO, capacity or rollback automation;
- passive-only governance for high-risk states;
- raw prompt, raw provider body or full IM message archive as normal release,
  incident or replay evidence;
- Python ownership of release, baseline approval, kill switch, rollback,
  failure-class closure, final proposal, ACTIVE memory, approval, execution,
  production source truth or audit archive.

## Re-Review Result

After applying the ADR acceptance playbook, the AgentOps / Governance Boundary
candidate is reviewable and has no known Agent Lab P0/P1 blocker.

Agent Lab recommends that main integration review this candidate sixth. If main
integration accepts it, Agent Lab should close the six-candidate L1 package
audit and prepare only scoped implementation-design work requested by accepted
owners. If main integration rejects or defers, Agent Lab should handle that
P0/P1 or owner-evidence request before moving on.
