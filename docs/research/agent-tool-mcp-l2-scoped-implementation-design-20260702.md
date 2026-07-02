# Agent Tool / MCP L2 Scoped Implementation Design

Date: 2026-07-02

Status: L2 scoped implementation design draft for the Tool / MCP boundary.
This is not an accepted production ADR, proto, OpenAPI, Kafka schema,
migration, service directory, startup path, MCP provider contract, tool schema,
gateway API, action-executor API or runtime implementation.

## Verdict

Conditionally passed as the fifth L2 scoped design draft.

Rejected for implementation until main integration, mcp-gateway,
skill-registry, policy/security, Agent Runtime, workflow-service,
action-executor, audit/security, product/operator and SRE/incident owners review
the design and approve the required L3 real-service smoke plan.

Reason: L1 accepted the Tool / MCP candidate for reviewability only. The L2
question is narrower: how can a future controlled implementation prove that MCP
providers, tool descriptions and tool outputs remain untrusted inputs, while
high-risk side effects still require prepare, policy, approval,
action-executor ownership, replayable audit lineage and operator controls.

## Scope

The scoped slice is Tool / MCP governance and side-effect handoff.

It covers:

- ToolIntent, ToolProvider, ProviderAttestation, CapabilityLease,
  PreparedToolRef, ToolSchemaHash, ToolOutputEnvelope, ToolRiskTier and
  ToolExecutionPolicy ownership;
- skill allowlist and release-state refs;
- provider provenance, sandbox onboarding and trusted-provider downgrade;
- capability lease scope, expiry, approval requirement and replay-reader refs;
- prepare, re-prepare, state-diff and stale PreparedToolRef rejection;
- tool output validation, taint and reuse policy;
- workflow approval wait for high-risk proposals;
- action-executor execution, idempotency, state-diff, provider failure and
  redrive boundaries;
- L3 real-service smoke requirements.

It does not cover:

- final MCP provider, tool or prepared-call field shape;
- production gateway or action-executor APIs;
- production provider onboarding;
- production tool schema registry;
- production database tables, migrations, queues or startup paths;
- production secret storage design;
- real MCP providers or real model providers;
- real NexusIM IM data;
- production provider capacity limits or SLOs.

## Boundary Thesis

Tools are capabilities, not authority. MCP providers, descriptions, resources,
prompts, sampling output and tool results are untrusted.

```text
Agent Runtime owns candidate intent:
  ToolIntent, candidate args, relevance reason and proposal linkage.

mcp-gateway owns prepare authority:
  provider provenance, attestation refs, schema hashes, capability leases,
  prepare/precheck refs, output envelopes, taint labels and prepare audit refs.

workflow-service owns durable approval waits:
  approval timer, decision and callback refs only.

action-executor owns side effects:
  execution attempt, idempotency, state-diff, provider failure projection,
  controlled redrive and execution audit refs.
```

No component may treat a tool description, MCP provider claim or model output as
authorization.

## Proposed Ownership

| Object / State | Owner | Cannot Be Owned By |
| --- | --- | --- |
| SkillPackage allowlist | skill-registry / governance | MCP provider, model output |
| ToolIntent | Agent Runtime | mcp-gateway as final action decision |
| ToolProvider | mcp-gateway / provider governance | Agent Runtime |
| ProviderAttestation | mcp-gateway / security governance | provider self-claim |
| ToolSchemaHash | mcp-gateway | Runtime candidate args |
| CapabilityLease | policy + mcp-gateway | MCP provider, Python worker |
| PreparedToolRef | mcp-gateway | Agent Runtime, workflow-service |
| ToolOutputEnvelope | mcp-gateway plus verifier owner | raw provider body archive |
| ToolRiskTier | governance / policy | MCP provider |
| ToolExecutionPolicy | policy / action-executor | model output |
| Approval wait / decision | workflow-service | Agent Runtime, action-executor |
| Execution attempt | action-executor | Agent Runtime, mcp-gateway |
| Idempotency ledger | action-executor | workflow-service |
| State-diff validation | action-executor plus owning business service | Agent Runtime |
| Provider failure projection | action-executor for execution, mcp-gateway for prepare | Python worker |
| Tool-output taint | mcp-gateway / Runtime verifier | unversioned free text |
| Audit archive | audit-service plus owning service | Python worker |
| Operator provider controls | product/operator plus governance/security | MCP provider |

## Non-Owner State

Agent Runtime must not own:

- provider trust;
- provider attestation;
- capability lease authority;
- PreparedToolRef truth;
- approval wait truth;
- side-effect execution;
- idempotency ledger;
- provider secret;
- tool output as trusted instruction;
- ACTIVE memory from tool output.

mcp-gateway must not own:

- final business mutation;
- approval decision;
- workflow timer;
- action-executor execution receipt;
- source truth for business services;
- release decision for a SkillPackage.

workflow-service must not own:

- tool prepare truth;
- tool provider trust;
- side-effect execution;
- idempotency ledger;
- tool output safety decision;
- business mutation truth.

action-executor must not own:

- tool selection;
- model planner state;
- provider onboarding policy;
- approval decision;
- raw prompt or EvidencePack body;
- memory admission.

Python AI Worker must not own final proposal, approval, prepare truth,
execution, ACTIVE memory, provider trust or audit archive.

## Candidate L2 Flow

```text
AgentRun
-> Runtime creates ToolIntent and candidate args
-> skill-registry confirms SkillPackage allowlist and release refs
-> mcp-gateway verifies provider attestation, schema hash, lease and policy refs
-> mcp-gateway returns PreparedToolRef or fail-closed denial
-> Runtime attaches evidence/proposal refs
-> workflow-service owns approval wait when policy requires approval
-> action-executor verifies fresh prepare, approval, idempotency and state-diff refs
-> action-executor calls public business/provider API
-> audit-service records low-sensitive lineage
-> ReplayReader reconstructs prepare/approval/execution refs without provider re-exec
```

The L2 flow is a design path only. It does not create production provider
contracts, schemas, APIs, migrations, queues or workers.

## Tool Trust And Risk Rules

### Catalog And Schema

- Tool catalog metadata is discovery material only.
- Schema examples and descriptions are untrusted and prompt-injection scanned.
- Schema hashes must be recorded at prepare time.
- Schema drift after prepare forces re-prepare.

### Read-Only Prepare

- Read-only tools still require provenance, policy and output taint refs.
- Output can enter ContextPackage only through ToolOutputEnvelope and reuse
  policy.
- Unsafe output is quarantined and cannot become trusted instruction or ACTIVE
  memory.

### Proposal-Only Side Effect

- Runtime can propose a possible mutation only as ToolIntent plus evidence refs.
- Proposal-driving facts must be grounded through EvidencePack / ContextPackage
  and verifier refs.
- Approval requirements come from policy/skill refs, not model preference.

### Approved Execution

- High-risk execution requires SkillPackage allowlist, current
  PreparedToolRef, policy refs, approval refs when required, idempotency refs and
  action-executor acceptance.
- Runtime and mcp-gateway cannot execute side effects directly.
- Replay never re-executes the side effect.

### Unknown Provider

- Unknown providers are sandbox-only or blocked.
- Production eligibility requires owner-reviewed ProviderAttestation.
- Stale or missing attestation downgrades trusted-provider path.

## Capability Lease Rules

CapabilityLease must carry low-sensitive refs for:

- actor and tenant scope;
- SkillPackage and AgentDefinition refs;
- tool and provider refs;
- operation class;
- risk tier;
- policy decision;
- approval requirement;
- expiry;
- schema hash;
- attestation ref;
- replay-reader policy;
- denial/re-prepare reason refs.

Missing, expired, over-broad or mismatched leases reject prepare or force
re-prepare. A provider cannot self-issue a lease by advertising a capability.

## Prepare And Re-Prepare Rules

PreparedToolRef is invalid when:

- lease expired;
- schema hash changed;
- provider attestation changed;
- provider trust tier changed;
- relevant state-diff invalidates dry-run/precheck;
- approval window expired;
- actor, tenant, skill, tool, provider or operation scope changed;
- replay-reader policy cannot explain the path.

Execution must verify fresh prepare lineage. Stale prepare refs fail closed and
must not be repaired by using old raw provider input/output.

## Tool Output Reuse Rules

ToolOutputEnvelope must preserve:

- provider and tool refs;
- schema and validation refs;
- output hash refs;
- taint labels and vocabulary version;
- redaction policy refs;
- replay-reader policy refs;
- audit lineage refs;
- quarantine reason refs when unsafe.

Tool output cannot become:

- system instruction;
- permission authority;
- execution approval;
- ACTIVE memory;
- untainted factual source;
- source-service fallback.

Memory candidate extraction from tool output must go through Memory Admission.
Context reuse must go through Context / EvidencePack taint and verifier rules.

## Approval And Execution Rules

For high-risk tools, the required path is:

```text
SkillPackage allowlist
-> mcp-gateway prepare
-> policy decision
-> proposal with evidence refs
-> workflow approval
-> action-executor fresh-prepare verification
-> idempotent execution
-> state-diff / result projection
-> audit lineage
```

If any required ref is missing, stale, denied or incompatible, execution is
blocked. Unknown state cannot be treated as success.

## Provider Failure And Redrive Rules

Prepare-phase failures belong to mcp-gateway / Runtime retry budget:

- provider unavailable;
- schema mismatch;
- attestation missing;
- lease expired;
- policy denial;
- output unsafe.

Execution-phase failures belong to action-executor:

- provider timeout after execution starts;
- idempotency conflict;
- state-diff mismatch;
- partial result projection;
- redrive approval requirement.

Controlled redrive requires fresh proposal, prepare, approval and execution
lineage. Old raw provider bodies are not replayed.

## Version And Replay Rules

Every future controlled implementation design must carry low-sensitive refs for:

- ToolIntent version;
- ToolProvider version;
- ProviderAttestation version;
- CapabilityLease version;
- PreparedToolRef version;
- ToolOutputEnvelope version;
- ToolRiskTier version;
- ToolExecutionPolicy version;
- compatibility window;
- replay-reader policy;
- redaction and retention policy;
- preservation matrix;
- audit and operator action refs.

Replay must reconstruct provider selection, prepare denial, approval,
execution, stale-ref rejection, redrive and failure class from refs. Normal
replay must not require raw prompts, raw EvidencePack bodies, raw provider
payloads, secrets, private service rows or side-effect re-execution.

## Operator Surfaces

Before any implementation, owners must approve low-sensitive inspect-and-act
surfaces for:

- ToolIntent, PreparedToolRef and ToolOutputEnvelope refs;
- provider identity, attestation and trust-tier refs;
- capability lease scope, expiry and denial refs;
- schema hash and drift refs;
- sandbox onboarding state;
- policy decision and approval refs;
- execution idempotency, state-diff, result and provider failure refs;
- tool-output taint, quarantine and reuse-policy refs;
- replay-reader, redaction and audit refs;
- kill switch, provider disable, rollback and redrive controls.

Operators must be able to explain why a provider/tool was allowed, sandboxed,
blocked, re-prepared, approved, executed, redriven or killed without exposing
raw provider payloads or secrets.

## L3 Real-Service Smoke Plan

Before implementation can start, owners must approve a smoke plan that proves
the following with low-sensitive records only:

| Smoke | Boundary | Required Proof |
| --- | --- | --- |
| ToolIntent dry run | Runtime -> mcp-gateway | Runtime submits candidate intent only; no execution |
| Skill allowlist | skill-registry -> mcp-gateway | unallowlisted tools reject |
| Capability lease denial | policy -> mcp-gateway | missing/expired/over-broad lease blocks prepare |
| Provider attestation downgrade | provider governance -> mcp-gateway | stale/missing attestation becomes sandbox or blocked |
| Unknown provider onboarding | mcp-gateway/operator | unknown provider cannot enter production trusted path |
| Re-prepare on drift | mcp-gateway -> action-executor | schema/state/approval drift rejects stale PreparedToolRef |
| Tool output taint | mcp-gateway -> Runtime/Context | output stays tainted and unsafe output quarantines |
| Approval wait | Runtime -> workflow-service -> action-executor | workflow owns wait only; executor verifies approval |
| Side-effect execution | action-executor -> public API/audit | fresh prepare, approval and idempotency are required |
| Provider failure redrive | action-executor -> workflow/operator | old raw provider bodies are not replayed |
| Replay reader dry run | eval/audit -> per-owner refs | replay does not call provider or execute side effects |
| Operator kill/rollback | operator -> governance/executor | provider/tool can be disabled, rolled back or blocked with audit refs |

These smokes must not use real NexusIM IM data. Fixture evidence can prepare the
plan, but cannot substitute for L3 real-service smoke.

## Owner Review Checklist

| Owner | Must Approve |
| --- | --- |
| Main integration | Service boundaries, allowed paths and no production shortcut |
| mcp-gateway owner | provider registry, attestation, schema hash, prepare refs, output envelopes and taint |
| skill-registry owner | SkillPackage allowlist, risk tier, release state and eval refs |
| policy/security owner | actor/tenant scope, capability lease, sandbox, denial and provider trust rules |
| Agent Runtime owner | ToolIntent-only behavior, proposal linkage and tool-output consumption |
| workflow-service owner | durable approval wait only, no side-effect execution |
| action-executor owner | execution, idempotency, stale-prepare rejection, state-diff, provider failure and redrive |
| audit/security owner | low-sensitive lineage, redaction, retention and replay-reader policy |
| product/operator owner | provider review, sandbox release, kill switch, rollback, redrive and failure-class UX |
| SRE/incident owner | provider timeout budget, capacity, telemetry and incident escalation refs |

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
python -m pytest ai/python/tests/test_agent_eval_tool_mcp_governance.py -q
python -m pytest ai/python/tests/test_agent_eval_cross_service_preservation.py -q
python -m pytest ai/python/tests/test_agent_eval_contract_version_compatibility.py -q
python -m pytest ai/python/tests/test_agent_eval_operator_governance.py -q
python -m pytest ai/python/tests/test_agent_eval_operational_readiness.py -q
python -m pytest ai/python/tests/test_agent_eval_controlled_implementation_readiness.py -q
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_mcp_security_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_mcp_security_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_state_diff_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_state_diff_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_state_diff_deeper_hardening_scenarios.json
```

## P0 / P1 / P2 Review

| Severity | Finding | Disposition |
| --- | --- | --- |
| P0 | None inside this L2 design draft | No production provider, schema, service connection, real data or side-effect path is introduced |
| P1 | Owner review is missing | External blocker before implementation |
| P1 | L3 real-service smoke is missing | External blocker before implementation |
| P1 | Production provider onboarding and sandbox policy are not approved | External blocker before implementation |
| P1 | Production action-executor stale-prepare rejection is not approved | External blocker before implementation |
| P2 | Provider timeout and capacity budgets remain fixture-only | Keep for SRE/provider owner review |
| P2 | Final tool taxonomy and risk-tier thresholds remain unfrozen | Keep for governance review |

## Auto-Reject Rules

Reject any Tool / MCP implementation proposal that:

- creates proto, OpenAPI, Kafka schema, migration, service directory, database
  table, startup path, MCP provider contract, gateway API, action-executor API
  or production field shape from this L2 design alone;
- treats MCP provider, tool description, schema example, prompt, resource,
  metadata or output as permission authority;
- lets Runtime execute high-risk side effects;
- executes without SkillPackage allowlist, current prepare, policy, approval
  when required, action-executor validation, idempotency and audit refs;
- makes ProviderAttestation or CapabilityLease optional for trusted providers;
- lets unknown providers bypass sandbox onboarding;
- executes stale PreparedToolRef without re-prepare;
- lets tool output bypass taint labels or become system instruction, ACTIVE
  memory, permission authority or untainted fact source;
- replays raw provider payloads or side effects as normal verification;
- uses fake providers or fixture providers as production fallback;
- lets Python own final proposal, approval, execution, ACTIVE memory, provider
  trust, source truth or audit archive.

## Decision

This design closes the Agent Lab-side L2 design gap for the fifth candidate:
Tool / MCP. It does not authorize implementation.

Next safe action after main integration review is either:

- owner review of the first five L2 designs; or
- a sixth L2 scoped design for AgentOps / Governance so the full L2 package is
  ready before any real-service smoke.
