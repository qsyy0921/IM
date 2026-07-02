# AgentOps / Governance L2 Scoped Implementation Design

Date: 2026-07-02

Status: L2 scoped implementation design draft for the AgentOps / Governance
boundary. This is not an accepted production ADR, proto, OpenAPI, Kafka schema,
migration, service directory, startup path, release pipeline, admin console,
control-plane API, release schema, SLO policy or runtime implementation.

## Verdict

Conditionally passed as the sixth L2 scoped design draft.

Rejected for implementation until main integration, governance/release,
Agent Runtime, ai-eval-service, skill-registry, memory-service,
retrieval-gateway, mcp-gateway, workflow-service, action-executor,
audit/security, product/operator and SRE/incident owners review the design and
approve the required L3 real-service smoke plan.

Reason: L1 accepted the AgentOps / Governance candidate for reviewability only.
The L2 question is narrower: how can a future controlled implementation prove
that AgentDefinition and SkillPackage promotion is owner-approved, eval-gated,
replayable, rollbackable, kill-switchable and operator-governed without making
AgentOps a business fact store, workflow engine, eval service, runtime engine or
production admin shortcut.

## Scope

The scoped slice is Agent release governance and AgentOps control evidence.

It covers:

- AgentDefinitionRelease, SkillPackageRelease, ReleaseChannel,
  ReleaseGateDecision, EvalGateBinding, BaselineApproval, ReplayRequirement,
  FailureClassOwner, KillSwitch, RollbackPlan, CanaryShadowPlan,
  OperatorActionLedger, PolicyException, PromotionException,
  IncidentEscalation, GovernanceEvidenceBundle and ControlPlaneView refs;
- release gate dry-run, shadow, beta, production, disabled and rollback state;
- pinned AgentDefinition, SkillPackage, model policy, memory grant, tool grant,
  EvalReport, ReplayBundle, baseline, audit and rollback refs;
- kill-switch scope, propagation, running-run behavior and audit refs;
- baseline refresh and failure-class owner workflow;
- canary / shadow comparability and rollback or hold behavior;
- operator inspect-and-act surfaces for release, baseline, failure class, kill
  switch, rollback, memory, evidence, replay, approval, tool and execution refs;
- incident, SLO, capacity and telemetry evidence required before promotion;
- L3 real-service smoke requirements.

It does not cover:

- production release pipeline implementation;
- production admin UI or control-plane API design;
- final AgentDefinition, SkillPackage, AgentRelease, gate, incident, SLO or
  operator-action field shape;
- production database tables, queues, migrations or service registry changes;
- production on-call policy, SLO numbers or telemetry wiring;
- real NexusIM IM data;
- real model, MCP, workflow, memory, action-executor, audit or backend service
  integration;
- final agent, skill, risk, incident or failure-class taxonomy.

## Boundary Thesis

AgentOps is release control and operational governance. It is not runtime
cognition, durable workflow waiting, business execution, memory admission,
source retrieval, eval computation or audit archive.

```text
governance / release owner decides:
  whether an AgentDefinition or SkillPackage version can be enabled, held,
  promoted, rolled back, killed or baseline-refreshed.

Agent Runtime enforces:
  release pin, runtime budget, kill-switch read, proposal/run linkage and
  cancellation/resume behavior exposed by owners.

ai-eval-service produces:
  EvalReport, ReplayBundle, regression delta and blocked promotion reasons.

audit-service records:
  low-sensitive release, operator, rollback, kill-switch and incident lineage.
```

No model output, Python worker, MCP provider, prompt text or passive dashboard
may approve a release, override a gate, refresh a baseline, close a P0/P1
failure class or bypass a kill switch.

## Proposed Ownership

| Object / State | Owner | Cannot Be Owned By |
| --- | --- | --- |
| AgentDefinitionRelease | governance / agent-service | Python worker, model output |
| SkillPackageRelease | governance / skill-registry | Agent Runtime |
| ReleaseChannel | governance / release control | dashboard-only view |
| ReleaseGateDecision | governance / release owner | ai-eval-service as final owner |
| EvalGateBinding | governance plus ai-eval-service refs | model provider |
| EvalReport | ai-eval-service | governance plane as source report |
| ReplayRequirement | governance plus ai-eval-service | Runtime local trace only |
| BaselineApproval | governance / release owner plus ai-eval refs | score improvement alone |
| FailureClassOwner | governance / incident owner | unowned failure taxonomy |
| KillSwitch | governance / control plane | Python worker, MCP provider |
| RollbackPlan | governance / release owner | Agent Runtime |
| CanaryShadowPlan | governance plus SRE/observability | offline fixture only |
| OperatorActionLedger | audit-service plus governance/action owner | passive UI cache |
| PolicyException | policy/security plus governance | prompt or SkillPackage text |
| PromotionException | governance with expiry and owner | silent release bypass |
| IncidentEscalation | SRE/incident owner plus governance | eval fixture |
| GovernanceEvidenceBundle | governance index of refs | raw prompt/provider archive |
| ControlPlaneView | product/operator plus owners | hidden local script |
| Runtime run state | Agent Runtime | governance plane |
| Approval wait / decision | workflow-service | governance plane |
| Side-effect execution | action-executor | governance plane |
| ACTIVE memory | memory-service | governance plane |
| Source visibility | retrieval/source owner | governance plane |
| Audit archive | audit-service | governance plane local storage |

## Non-Owner State

Governance / AgentOps must not own:

- business facts, message state, source-service truth or private service rows;
- Agent Runtime planner state, checkpoints or step execution;
- workflow timers, approval decisions or durable wait callbacks;
- action-executor side effects, idempotency ledger or execution receipt truth;
- ACTIVE memory, retrieval eligibility, revocation or source visibility;
- MCP provider trust, PreparedToolRef truth or provider output validation;
- EvalReport computation, benchmark ground truth or ReplayBundle generation;
- raw prompt, raw provider body, full IM transcript or secret archive.

Agent Runtime must not own:

- release approval;
- baseline approval;
- kill-switch creation or override;
- failure-class closure;
- rollback authorization;
- production operator action ledger.

ai-eval-service must not own:

- final release decision;
- production channel transition;
- baseline refresh approval;
- incident response owner;
- kill-switch override.

Python AI Worker must not own release, baseline approval, kill switch, rollback,
failure-class closure, final proposal, ACTIVE memory, approval, execution,
production source truth or audit archive.

## Candidate L2 Flow

```text
AgentDefinition / SkillPackage candidate refs
-> governance resolves owner, risk tier, channel and required gate profile
-> ai-eval-service produces EvalReport, ReplayBundle and regression delta refs
-> governance checks EvalGateBinding, ReplayRequirement and BaselineApproval
-> policy/security and audit owners check exceptions, redaction and lineage refs
-> product/operator confirms inspect-and-act surfaces for required states
-> SRE/incident owner confirms canary, telemetry, budget and escalation refs
-> ReleaseGateDecision is allow, hold, block, shadow-only, rollback or disabled
-> Runtime and related services enforce pinned release and kill-switch refs
-> audit-service records release and operator action lineage
-> ReplayReader reconstructs the gate from low-sensitive refs
```

The L2 flow is a design path only. It does not create production release fields,
admin APIs, service code, queues, migrations or automation.

## Release Gate Rules

Every production or beta promotion must fail closed unless it has:

- owner and on-call refs;
- pinned AgentDefinition and SkillPackage refs;
- pinned model/provider policy refs;
- pinned memory and tool grant refs when those capabilities are used;
- required EvalReport refs;
- ReplayBundle refs and replay-reader policy;
- BaselineApproval for new or refreshed baseline;
- P0/P1 failure-class disposition;
- policy/security decision refs;
- audit and redaction refs;
- operator inspect-and-act refs;
- rollback plan and kill-switch refs;
- canary / shadow plan for non-trivial risk;
- operational budget and incident escalation refs.

Draft and fixture-only candidates may omit production refs, but then they must
remain unable to serve production traffic or authorize production actions.

## Channel And Pinning Rules

Conceptual channels:

- Draft: design, local fixture or candidate-only work.
- Shadow: offline or hidden evaluation with no user-visible side effects.
- Beta: limited visible scope with owner-approved monitoring and rollback.
- Production: general allowed use within declared scope.
- Disabled: new eligible runs are blocked.
- RolledBack: channel points to a prior approved release ref.

Release promotion cannot rely on mutable "latest" references. A release must
pin the AgentDefinition, SkillPackage, model policy, tool grant, memory grant,
EvalReport, ReplayBundle, baseline and rollback refs it used.

## Baseline Approval Rules

BaselineApproval is required when:

- datasets or fixture manifests change;
- required suites change;
- failure-class taxonomy changes;
- risk tier changes;
- model/provider policy changes;
- scoring weights or thresholds change;
- release owner wants to accept a known regression.

Score improvement alone cannot silently refresh a baseline. A baseline refresh
with missing owner, missing replay, open P0/P1 failure, changed risk without
approval or missing audit refs must block promotion.

## Failure-Class Owner Rules

Every P0/P1 failure class must have:

- owner ref;
- severity and scope refs;
- first-seen EvalReport or incident ref;
- linked ReplayBundle ref;
- regression fixture requirement or explicit no-fixture reason;
- closure rule and retirement rule;
- audit and operator action refs.

Unowned P0/P1 failure classes block release, rollback-to-unsafe-version and
baseline refresh. A repeated production incident must either become a regression
fixture or carry an owner-approved reason why fixture capture is impossible.

## Kill Switch And Rollback Rules

KillSwitch must define:

- owner;
- scope: AgentDefinition, SkillPackage, tool grant, MCP provider, memory grant,
  model policy, release channel or tenant cohort;
- activation reason and severity;
- propagation targets;
- behavior for new runs;
- behavior for running, waiting, approved, executing and replay-only runs;
- audit, rollback and incident refs;
- expiry or review policy if temporary.

Activation must stop new eligible runs within the owner-approved propagation
budget. Running-run behavior belongs to Runtime, workflow-service,
action-executor and memory/tool owners according to state; governance records
the control decision but cannot mutate their private state directly.

RollbackPlan must pin the prior safe release, reason, compatibility window,
data/memory/tool implications, audit refs and operator confirmation. Rollback
must not erase audit, memory review or execution records.

## Canary / Shadow Rules

CanaryShadowPlan must define:

- candidate release and baseline refs;
- tenant/user cohort or synthetic/public dataset scope;
- comparable metric set;
- minimum sample or confidence refs;
- P0/P1 regression rules;
- cost, latency and provider timeout budget refs;
- hold, rollback, disable and escalation behavior.

Shadow results cannot promote production unless they are comparable to the
required offline suites. Canary P0/P1 regression holds or rolls back release.
Fixture-only canary evidence cannot substitute for production owner telemetry.

## Policy Exceptions And Promotion Exceptions

Exceptions are allowed only as explicit governed objects:

- owner and approver refs;
- exact scope and expiry;
- reason and risk tier;
- compensating controls;
- audit and operator refs;
- blocked capability list;
- renewal or retirement rule.

An exception cannot bypass P0/P1 safety failures, missing audit, missing replay,
missing owner, missing kill switch, missing rollback or Python final ownership
violations.

## Operator Surfaces

Before any implementation, owners must approve low-sensitive inspect-and-act
surfaces for:

- release channel, candidate, hold, rollback and disabled states;
- AgentDefinitionRelease and SkillPackageRelease refs;
- EvalReport, ReplayBundle, baseline and regression delta refs;
- open failure classes and owner disposition;
- kill switch scope, propagation and running-run policy;
- memory admission and retrieval eligibility refs when memory is used;
- EvidencePack / ContextPackage coverage and denied/unavailable/expired lanes;
- ToolIntent, PreparedToolRef, provider trust and execution refs when tools are
  used;
- workflow approval, timeout and resume refs;
- action-executor state-diff, idempotency, execution and redrive refs;
- policy exception and promotion exception refs;
- audit, redaction, retention, replay-reader and incident refs.

High-risk operator surfaces cannot be passive-only. They need authorized action
paths such as hold, block, rollback, disable, revoke, quarantine, redrive,
escalate, require-review or close-with-disposition. Those actions must be
audited and cannot expose raw bodies to unauthorized users.

## Incident And Operational Readiness Rules

Production promotion remains blocked until SRE/incident owners approve:

- runtime step budget;
- model spend budget;
- MCP/tool timeout budget;
- retrieval / EvidencePack latency budget;
- eval report retention budget;
- canary / shadow telemetry budget;
- incident escalation window;
- on-call ownership and handoff rule;
- release hold, rollback and disable runbook;
- post-incident regression fixture capture rule.

L2 only defines the evidence shape. It does not set production SLO numbers or
create on-call policy.

## Version And Replay Rules

Every future controlled implementation design must carry low-sensitive refs for:

- AgentDefinitionRelease version;
- SkillPackageRelease version;
- ReleaseChannel version;
- ReleaseGateDecision version;
- EvalGateBinding version;
- BaselineApproval version;
- FailureClassOwner version;
- KillSwitch version;
- RollbackPlan version;
- CanaryShadowPlan version;
- OperatorActionLedger version;
- PolicyException / PromotionException version;
- IncidentEscalation version;
- compatibility window;
- replay-reader policy;
- redaction and retention policy;
- preservation matrix;
- audit and operator action refs.

Replay must reconstruct release allow, hold, block, rollback, kill switch,
baseline refresh, canary/shadow regression, failure-class closure and operator
actions from refs. Normal replay must not require raw prompts, raw provider
bodies, full IM transcript archives, secrets, private service rows or
side-effect re-execution.

## L3 Real-Service Smoke Plan

Before implementation can start, owners must approve a smoke plan that proves
the following with low-sensitive records only:

| Smoke | Boundary | Required Proof |
| --- | --- | --- |
| Release gate dry run | governance -> ai-eval/audit/operator | missing eval, replay, audit, owner, rollback or operator refs block |
| Release pinning | governance -> Runtime/skill-registry | Runtime observes exact pinned AgentDefinition and SkillPackage refs |
| Baseline approval | governance -> ai-eval | changed suite or risk tier cannot refresh baseline silently |
| P0/P1 failure owner | governance -> incident/eval | open unowned failure blocks release and baseline refresh |
| Kill switch propagation | governance -> Runtime/workflow/executor/memory/MCP | new runs block and running-run behavior is owner-specific |
| Rollback | governance -> Runtime/audit | release points back to pinned safe version without deleting lineage |
| Canary / shadow comparability | governance -> observability/ai-eval | metrics are comparable to baseline and P0/P1 regression holds or rolls back |
| Policy exception expiry | governance -> policy/security | expired or over-broad exception fails closed |
| Operator action ledger | operator -> governance/audit | hold, rollback, disable and close actions are authorized and audited |
| Cross-service preservation | Runtime/workflow/memory/MCP/executor/audit | release, version, scope, taint, replay and audit refs survive boundaries |
| Replay reader dry run | eval/audit -> governance refs | gate explanation is reconstructed without raw body archive or side effects |
| Incident escalation | governance -> SRE/incident | P0/P1 incident creates owner, escalation and regression-capture refs |

These smokes must not use real NexusIM IM data until owner-approved test data
policy exists. Fixture evidence can prepare the plan, but cannot substitute for
L3 real-service smoke.

## Owner Review Checklist

| Owner | Must Approve |
| --- | --- |
| Main integration | Service boundaries, allowed paths and no production shortcut |
| Governance/release owner | ReleaseGateDecision, channel transitions, pinning, hold, rollback, disable and exception semantics |
| agent-service owner | AgentDefinitionRelease refs, enable/disable enforcement and no giant agent-service shortcut |
| skill-registry owner | SkillPackageRelease refs, tool/memory grants, risk tier and eval suite binding |
| Agent Runtime owner | release pin enforcement, kill-switch read, runtime cancellation/drain behavior and no release ownership |
| ai-eval-service owner | EvalReport, ReplayBundle, regression delta and baseline comparison refs |
| memory/retrieval owners | memory grant, retrieval eligibility, source visibility and no governance-owned memory truth |
| mcp-gateway owner | tool/provider grant and provider disable refs |
| workflow-service owner | approval wait state and running-wait behavior under kill switch |
| action-executor owner | execution, idempotency, redrive and running-execution behavior under kill switch |
| audit/security owner | low-sensitive lineage, redaction, retention, exception and operator action ledger |
| product/operator owner | inspect-and-act UX for release, failure, kill switch, rollback and dependent surfaces |
| SRE/incident owner | telemetry, budgets, canary, escalation, on-call and incident-response refs |

## Service Promotion Choice

Preferred order for future implementation review:

1. Keep runtime module plus workflow-service and add a governance/release module
   inside or adjacent to agent-service only for release refs and gate decisions.
2. Add a separate agent-runtime-service only if Runtime needs independent scale,
   fault isolation, queue ownership and operational budgets beyond a module.
3. Expand agent-service into a broader control plane only if it remains a thin
   governance facade and does not absorb Runtime, workflow, memory, MCP,
   executor, eval or audit state.

Reject service promotion if it:

- creates a second workflow engine;
- turns agent-service into a giant monolith;
- lets governance own runtime checkpoints, approval waits, execution,
  ACTIVE memory, source truth or audit archive;
- introduces production APIs, schemas or migrations before accepted ADR and
  owner review;
- makes Python or model output the release authority.

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
python -m pytest ai/python/tests/test_agent_eval_agentops_governance.py -q
python -m pytest ai/python/tests/test_agent_eval_operator_governance.py -q
python -m pytest ai/python/tests/test_agent_eval_operational_readiness.py -q
python -m pytest ai/python/tests/test_agent_eval_controlled_implementation_readiness.py -q
python -m pytest ai/python/tests/test_agent_eval_replay_compatibility.py -q
python -m pytest ai/python/tests/test_agent_eval_contract_version_compatibility.py -q
```

The rehearsal JSON files for AgentOps, operator governance, operational
readiness and contract-version compatibility are specialized governance
fixtures loaded by these pytest suites. They are not `synthetic_im_like`
fixtures and must not be run through the generic `run_agent_eval_fixture.py`
CLI.

## P0 / P1 / P2 Review

| Severity | Finding | Disposition |
| --- | --- | --- |
| P0 | None inside this L2 design draft | No production release pipeline, control-plane API, schema, service connection, real data or runtime implementation is introduced |
| P1 | Owner review is missing | External blocker before implementation |
| P1 | L3 real-service smoke is missing | External blocker before implementation |
| P1 | Production admin UX and release-control ownership are not approved | External blocker before implementation |
| P1 | Production telemetry, on-call, canary and incident escalation are not approved | External blocker before implementation |
| P2 | Final release, incident and exception field shapes remain unfrozen | Keep for ADR / implementation design review |
| P2 | SLO thresholds and capacity budgets remain fixture-only | Keep for SRE/provider owner review |

## Auto-Reject Rules

Reject any AgentOps / Governance implementation proposal that:

- creates proto, OpenAPI, Kafka schema, migration, service directory, database
  table, startup path, release pipeline, admin UI, control-plane API or
  production field shape from this L2 design alone;
- lets a release proceed without owner, pinned inputs, eval, replay, baseline,
  audit, rollback, kill switch and required operator refs;
- treats ai-eval, Python, model output, MCP provider or dashboard state as final
  release authority;
- silently refreshes a baseline when datasets, suites, failure classes, risk
  tier, scoring policy or model/provider policy changed;
- allows open or unowned P0/P1 failures to release or refresh baseline;
- makes kill switch optional or unable to block new eligible runs;
- lets rollback erase audit, memory review, execution or incident lineage;
- promotes shadow/canary results that are not comparable to offline baseline;
- uses fixture evidence, public dataset results or synthetic data as production
  telemetry or production release authorization;
- exposes raw prompts, raw provider bodies, full IM transcripts, secrets or
  private service rows as normal operator/replay artifacts;
- lets governance own runtime checkpoints, workflow waits, side-effect
  execution, ACTIVE memory, source truth or audit archive;
- lets Python own release, baseline approval, kill switch, rollback,
  failure-class closure, final proposal, approval, execution, ACTIVE memory,
  production source truth or audit archive.

## Decision

This design closes the Agent Lab-side L2 design gap for the sixth candidate:
AgentOps / Governance. It does not authorize implementation.

Next safe action after main integration review is one of:

- full six-design L2 package owner review;
- L3 real-service smoke planning across release, runtime, workflow, memory,
  MCP, executor, eval, audit and operator owners;
- fixture-only hardening requested by review.

Production implementation remains blocked until accepted ADRs, owner signoff,
real-service smoke, production operator UX, telemetry and incident governance
are approved.
