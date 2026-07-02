# Agent Governance / AgentOps SDD

Date: 2026-07-01
Status: Detailed SDD draft for Agent Exploration Mode. This is not an ADR,
proto, OpenAPI, Kafka schema, migration, production service directory or release
schema.

## 1. Goal

Define how NexusIM governs AgentDefinition, SkillPackage, release channels,
eval gates, observability, incident response, rollback and kill switches.

AgentOps treats agents as governed operational systems that read context, use
tools, propose actions and influence memory. It is broader than model
monitoring.

## 2. Non-Goals

- Do not build an admin console in this phase.
- Do not freeze AgentDefinition or SkillPackage schema.
- Do not create production release pipelines.
- Do not replace service SDDs or ADR process.
- Do not let governance metadata override policy-service authorization.

## 3. Governed Objects

| Object | Purpose | Minimum Governance Metadata |
| --- | --- | --- |
| AgentDefinition | Runnable agent configuration | owner, purpose, tenant scope, risk tier, release channel, model policy, memory scope, disable switch |
| SkillPackage | Versioned capability package | owner, version, required evidence, allowed tools, approval policy, eval suite, rollback ref |
| Prompt / Instruction Ref | Versioned instruction material | hash, owner, review status, retention policy |
| Tool Grant | Tool capability for a skill/agent | tool id, risk, schema hash, policy ref, approval requirement |
| Memory Grant | Memory scope available to agent/skill | scope, audience, read/write candidate rules |
| AgentRelease | Promotion record | eval report ref, approver, canary plan, rollback plan, kill switch |

These objects are conceptual in this SDD.

## 4. Component Responsibilities

`agent-service` / future governance module owns:

- AgentDefinition metadata refs;
- release channel refs;
- run enable/disable decisions;
- proposal facade linkage;
- owner/oncall metadata.

`skill-registry` owns:

- SkillPackage catalog;
- tool/action allowlist;
- risk tier;
- approval policy refs;
- eval suite refs.

`policy-service` owns:

- authorization decisions;
- delegated identity policy;
- tenant/admin policy;
- data and action policy enforcement.

`ai-eval-service` owns:

- eval run/report refs;
- regression result refs;
- release gate outcome refs.

`audit-service` owns:

- append-only low-sensitive lineage and export/query boundary.

AgentOps views aggregate data from runtime, model-gateway, retrieval, memory,
MCP, workflow, executor, audit and eval.

## 5. State Ownership

| State | Owner | Cannot Be Owned By |
| --- | --- | --- |
| AgentDefinition metadata | agent-service/governance | model worker, MCP provider |
| SkillPackage metadata | skill-registry | Agent Runtime |
| Runtime run state | Agent Runtime | governance plane |
| Policy decision | policy-service | prompt text or MCP server |
| Eval report | ai-eval-service | model provider |
| Release approval | governance/admin workflow | model output |
| Kill switch | governance/control plane | Python worker |
| Audit archive | audit-service | runtime debug store |

Governance cannot own business facts, workflow transitions, execution results
or memory admission state. It can reference them for release and incident
review.

## 6. Release Channels

| Channel | Purpose | Required Gate |
| --- | --- | --- |
| Draft | Design or local fixture only | No production access |
| Shadow | Runs offline or hidden from users | Public dataset eval and safety fixtures |
| Beta | Limited tenants/users with visible output | Eval pass, owner, rollback, audit refs |
| Production | General allowed use | Eval gate, canary, monitoring, kill switch |
| Disabled | Blocked | Reason and audit ref |

High-risk skills cannot skip from Draft to Production.

## 7. Eval Gate Policy

Every production candidate should declare required suites:

- grounded RAG;
- permission leakage;
- memory scope/pollution if memory is used;
- tool/MCP security if tools are used;
- state-diff if actions are proposed/executed;
- approval/HITL if high-risk actions are involved;
- replay completeness;
- regression against previous release.

Gate result can be:

- pass;
- conditional pass with P2/P3 known risks;
- blocked by P0/P1 risk;
- not applicable with explicit reason.

P0/P1 failure blocks release.

## 8. Observability Model

AgentOps metrics should be grouped by agent, skill, tenant, channel and risk:

| Dimension | Metrics |
| --- | --- |
| Quality | grounded correctness, citation coverage, abstain correctness, verifier failure |
| Memory | candidate volume, admit/reject rate, pollution, revocation, stale-use attempts |
| Tool | prepare allow/deny, tool poisoning blocks, unsafe output, state-diff mismatch |
| Workflow | approval required, approved, rejected, timeout, resume failure |
| Runtime | run success, failure class, cancel, replay availability |
| Model | provider latency, timeout, token, cost, model version |
| Security | permission leakage, prompt injection block, approval bypass attempt |
| Release | canary failure, rollback, disabled agent, regression delta |

## 9. Audit and Explainability

User-facing explanation should be concise:

- sources used;
- whether an action needs approval;
- whether the answer is uncertain or abstained;
- whether memory was used or proposed;
- final status.

Operator-facing audit should include:

- actor and `on_behalf_of` refs;
- AgentDefinition and SkillPackage refs;
- policy decisions;
- EvidencePack/ContextPackage refs;
- model/provider metadata;
- prepared tool refs;
- workflow decisions;
- execution refs;
- memory candidate/admission refs;
- eval/replay refs.

Raw prompt/provider body is not the default audit artifact.

## 10. Incident and Rollback

Incident triggers:

- permission leakage;
- approval bypass attempt;
- memory pollution confirmed;
- high-risk tool executed incorrectly;
- MCP/tool poisoning not blocked;
- repeated hallucinated grounded answers;
- cost runaway;
- replay unavailable for failed production run.

Response:

```text
detect
-> freeze or kill switch affected AgentDefinition / SkillPackage
-> preserve audit/replay refs
-> classify failure
-> run regression suite
-> rollback or disable release
-> update eval fixture if gap was missing
```

Rollback must not erase audit or memory review records.

## 11. Key Flows

### 11.1 SkillPackage Promotion

```text
Draft skill
-> required eval suites
-> review risk and approval policy
-> shadow run
-> beta canary
-> production or blocked
```

### 11.2 AgentDefinition Disable

```text
incident or admin decision
-> set disabled/kill switch
-> block new runs
-> cancel or drain eligible existing runs
-> keep audit/replay refs
```

### 11.3 Regression Review

```text
new model/prompt/skill version
-> run required suites
-> compare baseline
-> classify regression
-> pass/conditional/block
```

## 12. Failure Semantics

| Failure | Governance Behavior |
| --- | --- |
| Eval P0/P1 failure | Block release |
| Missing eval suite | Block if capability requires it |
| Owner missing | Block promotion beyond Draft |
| Kill switch activated | Stop new runs |
| Canary regression | Roll back or hold Beta |
| Replay unavailable | Block high-risk release |
| Policy mismatch | Defer to policy-service and block if unresolved |
| Audit gap | Block high-risk action release |

## 13. Security Boundary

Governance must not rely on prompt promises. Required controls:

- policy-service decisions for runtime permission;
- skill allowlist for tools;
- eval gate for release;
- workflow approval for high-risk actions;
- action-executor for side effects;
- audit-service for immutable lineage;
- kill switch for rapid disable.

MCP providers, peer agents and model outputs cannot self-certify trust.

## 14. Eval / Replay

Governance consumes eval results and replay availability:

- release cannot pass if required eval suite is missing;
- repeated production failure class should become a regression fixture;
- replay incomplete is itself a release risk;
- canary and shadow results should be comparable to offline baseline;
- memory/tool/security incidents must feed back into the eval harness.

## 15. Risks / Rejection Conditions

Reject governance design if:

- AgentDefinition has no owner or disable switch;
- SkillPackage can use tools without eval/approval metadata;
- release can proceed with P0/P1 eval failure;
- audit cannot link policy, retrieval, tool, workflow and execution refs;
- governance plane stores business truth or raw prompt archive;
- kill switch cannot stop new runs.

## 16. Promotion Conditions

Move toward ADR / implementation only after:

- AgentDefinition and SkillPackage metadata needs are validated by offline
  harness;
- eval reports can block/pin releases;
- audit lineage is sufficient for incident review;
- kill switch and rollback semantics are agreed with control-plane ownership;
- production slice risk is narrow enough to operate.

## 17. Current Isolated Fixture Coverage

Current Agent Lab code only provides offline governance fixtures. It does not
create production AgentOps APIs, release pipelines, admin UI or control-plane
contracts.

Implemented fixture coverage:

- AgentOps ownership assertions for AgentDefinition, SkillPackage,
  AgentRelease, BaselineApproval, FailureClassOwner, KillSwitch, RollbackPlan
  and CanaryReport refs;
- Python worker and model output cannot own governance decisions;
- production-enabled release gates require pinned AgentDefinition /
  SkillPackage, EvalReport, ReplayBundle, BaselineApproval, rollback, disable
  switch and audit refs;
- P0/P1 eval failure, replay gap, audit gap or missing baseline blocks release;
- active kill switch blocks new runs, records propagation acknowledgements and
  running-run behavior;
- baseline refresh requires explicit approval when datasets, failure classes,
  risk tier or required suites change;
- P0/P1 failure classes require owner and regression disposition and block
  release / baseline refresh while open;
- canary / shadow metrics must be comparable to offline baseline and P0/P1
  regression must hold or roll back release;
- operator controls expose only low-sensitive refs and cannot be overridden by
  Python worker output.
- operator governance surface completeness covers memory, evidence, replay,
  approval, release, failure-class, kill-switch and rollback surfaces with
  inspect refs, action refs, owner refs, auth-policy refs, audit refs,
  redaction refs, replay-reader refs, failure-class refs, evidence refs and
  rejection refs;
- operator surface gaps block promotion when a view is passive-only,
  unaudited, body-exposing, accessible to an unauthorized actor or overridable
  by Python worker output.

Remaining hardening:

- main integration acceptance for governance / control-plane ownership;
- admin UX owner review for production release pinning and baseline approval;
- production on-call, SLO and incident escalation design;
- production canary telemetry and rollback automation.

## 18. References

- `docs/sdd/agent-platform.md`
- `docs/sdd/agent-runtime.md`
- `docs/sdd/agent-eval-replay-harness.md`
- `docs/sdd/skill-registry.md`
- `docs/sdd/policy-service.md`
- `docs/sdd/audit-service.md`
- Databricks State of AI Agents:
  <https://www.databricks.com/resources/ebook/state-of-ai-agents>
- Microsoft Agent Framework:
  <https://learn.microsoft.com/en-us/agent-framework/overview/>
