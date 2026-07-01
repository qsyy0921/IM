# Agent Tool / MCP Boundary SDD

Date: 2026-07-01
Status: Detailed SDD draft for Agent Exploration Mode. This is not an ADR,
proto, OpenAPI, Kafka schema, migration, production service directory, MCP
contract or tool contract.

## 1. Goal

Define the boundary for tools, MCP providers and external capabilities used by
NexusIM Agent Runtime.

The tool plane must allow agents to prepare useful actions while preventing
models, external tool descriptions and MCP server outputs from becoming a
permission system or direct side-effect path.

## 2. Non-Goals

- Do not add production MCP server integrations.
- Do not freeze tool, MCP or A2A schemas.
- Do not execute high-risk side effects from Agent Runtime.
- Do not let MCP server descriptions define authorization.
- Do not use fake providers as production fallback.

## 3. Component Responsibilities

`skill-registry` owns:

- skill id, version, owner and risk tier;
- allowed tool/action list;
- required scopes;
- approval policy refs;
- eval suite refs;
- release state.

`mcp-gateway` owns:

- tool registry and provider provenance;
- MCP server registration metadata;
- provider attestation refs;
- schema/version/capability hashes;
- capability lease and scope refs;
- tool risk label;
- tool prepare / precheck;
- input validation;
- output validation and taint labels for safe read-only tools;
- low-sensitive prepare audit refs.

Agent Runtime owns:

- tool intent candidate;
- why the tool is relevant;
- args candidate before prepare;
- tool output consumption after validation.

`workflow-service` owns:

- approval wait for high-risk tool/action proposals.

`action-executor` owns:

- approved execution;
- idempotency ledger;
- provider failure projection;
- controlled redrive;
- execution result refs.

## 4. State Ownership

| State | Owner | Notes |
| --- | --- | --- |
| Tool registry | mcp-gateway / skill-registry | Tool availability and provenance |
| Skill allowlist | skill-registry | Bound to SkillPackage version |
| ToolIntent | Agent Runtime | Candidate only |
| PreparedToolCallRef | mcp-gateway | Validated, not necessarily executed |
| Capability lease | mcp-gateway / policy owner | Low-sensitive lease and scope refs only |
| Provider attestation | mcp-gateway / provider governance | Provenance proof refs, not credentials |
| Approval decision | workflow-service | For high-risk tools/actions |
| Execution attempt | action-executor | Side-effect owner |
| Provider failure | action-executor or mcp-gateway by phase | Depends on prepare vs execute |
| Tool output taint | mcp-gateway / Runtime | Required before context reuse |
| Audit refs | audit-service plus owning service | Low-sensitive lineage |

Tool/MCP boundary cannot own:

- actor authorization truth beyond policy decision refs;
- workflow timers;
- business final state;
- raw prompt;
- ACTIVE memory;
- long-term provider secrets in Runtime.

## 5. Tool Trust Levels

| Level | Description | Default Behavior |
| --- | --- | --- |
| L0 Catalog | Name, owner, risk, short description only | Search/discovery only |
| L1 Schema | Input/output schema and examples | Tainted until verified |
| L2 Read-only prepare | Validated read or dry-run/precheck | Output tainted and source-labeled |
| L3 Proposal-only side effect | Tool can describe possible mutation | Requires approval/executor |
| L4 Approved execution | Side effect is executed by action-executor | Requires proposal, prepare and approval |
| L5 External/unknown MCP | Untrusted provider/server | Sandbox-only or blocked |

High-risk tools must not be auto-executed by Agent Runtime.

## 6. Prepare Envelope

Conceptual prepare metadata, not schema:

```text
PreparedToolCall
  prepared_ref
  agent_run_ref
  skill_ref
  tool_ref
  provider_ref
  tool_schema_hash
  provider_attestation_ref
  capability_lease_ref
  capability_scope_refs
  args_hash
  actor_ref
  tenant_ref
  policy_decision_ref
  risk_tier
  idempotency_ref
  dry_run_result_ref
  output_taint_labels
  approval_required
  expiry
```

Prepared refs expire. Execution requires fresh enough prepare lineage and
approval when risk policy demands it.

## 7. MCP Security Rules

MCP server data is untrusted:

- tool name and description;
- schema examples;
- resources;
- prompts;
- sampling output;
- tool results;
- remote server metadata.

Required controls:

- provider allowlist or sandbox classification;
- capability and schema hash tracking;
- capability lease validation before prepare is accepted;
- provider attestation verification before trusted provider selection;
- owner and provenance refs;
- tenant policy allow/deny;
- tool description prompt-injection scan;
- output safety gate;
- taint labels when output enters ContextPackage;
- no secrets in model-visible tool descriptions;
- no auto-execute for unknown or high-risk providers.

## 8. A2A Boundary

Peer agents are not tools. Future A2A-style integration must model:

- peer identity;
- trust level;
- capability advertisement;
- scope and budget;
- task envelope;
- trace lineage;
- result provenance;
- refusal and escalation behavior.

Until that contract exists, external peer-agent output is candidate material and
must go through the same source/taint verification as tool output.

## 9. Key Flows

### 9.1 Read-Only Tool Use

```text
Runtime creates ToolIntent
-> mcp-gateway validates skill/tool/schema/policy
-> provider read/precheck
-> output labeled as tool output
-> Runtime verifies before using in ContextPackage
```

If output is unsafe or prompt-injection-like, it is quarantined.

### 9.2 High-Risk Business Action

```text
ToolIntent
-> mcp-gateway prepare/precheck
-> proposal with evidence refs
-> workflow approval
-> action-executor verifies proposal + approval + prepare
-> public business API
-> execution ref and audit
```

No model step executes the side effect.

### 9.3 Provider Timeout

Prepare timeout belongs to mcp-gateway / Runtime retry budget. Execution timeout
belongs to action-executor and may require controlled redrive. Old raw provider
input/output is not replayed directly.

### 9.4 Tool Selection Attack

If malicious tool description attempts to override instructions or attract
selection incorrectly:

- mark description tainted;
- block or lower trust;
- require skill allowlist;
- record security failure class;
- do not include description as system instruction.

## 10. Failure Semantics

| Failure | Behavior |
| --- | --- |
| Tool not allowlisted | Deny |
| Schema mismatch | Reject prepare |
| Policy deny | Deny, no retry bypass |
| Unknown provider | Sandbox-only or deny |
| Missing provider attestation | Deny trusted-provider path |
| Missing capability lease | Deny or re-prepare under policy |
| Malicious description | Block / quarantine |
| Unsafe output | Do not insert into trusted context |
| Prepare timeout | Bounded retry or failure |
| Execution timeout | action-executor owns projection/redrive |
| Approval missing | No execution |
| Prepared ref expired | Re-prepare before execution |

## 11. Security Boundary

The safe side-effect path is:

```text
SkillPackage allowlist
-> mcp-gateway prepare
-> policy decision
-> proposal
-> workflow approval
-> action-executor
-> public business API
-> audit
```

Any path that skips a step is rejected for high-risk actions.

## 12. Eval / Replay

Required eval:

- tool selection accuracy;
- tool args validity;
- policy/approval adherence;
- prepare expiration behavior;
- provider timeout behavior;
- malicious tool description block;
- unsafe output quarantine;
- MCP server provenance handling;
- provider attestation verification;
- capability lease and scope validation;
- state-diff correctness after approved execution;
- replay without re-execution.

Candidate benchmark inputs:

- ToolSandbox;
- tau-bench;
- BFCL;
- MCP-Bench;
- MCPSecBench;
- custom NexusIM synthetic tool registry fixtures.

## 13. Observability / Audit

Metrics:

- tool prepare allow/deny;
- policy deny reason;
- malicious description detections;
- unsafe output detections;
- provider timeout rate;
- provider attestation missing rate;
- capability lease validation failure rate;
- approval-required rate;
- execution request/ref linkage;
- state-diff failures;
- replay availability.

Audit lineage:

- agent run ref;
- skill ref;
- tool ref;
- provider ref;
- provider attestation ref;
- schema/capability hash;
- capability lease/scope refs;
- args hash;
- policy decision ref;
- prepared ref;
- approval ref;
- execution ref.

## 14. Risks / Rejection Conditions

Reject tool/MCP promotion if:

- MCP provider is treated as permission authority;
- tool description is inserted as trusted instruction;
- high-risk action can execute without proposal/approval/executor;
- Runtime stores provider secrets;
- tool output can become ACTIVE memory without admission;
- state-diff is not checked for side-effect eval;
- fake provider is connected as production fallback.

## 15. Promotion Conditions

Promote tool/MCP integration only after:

- malicious tool description and unsafe output fixtures pass;
- prepare/approval/execution lineage is replayable;
- state-diff eval verifies outcomes;
- provider provenance, provider attestation, capability lease and schema hashing
  are available;
- action-executor remains sole side-effect owner.

## 16. Current Isolated Fixture Coverage

Current Agent Lab code only provides offline eval fixtures. It does not create a
production MCP provider, gateway contract or tool schema.

Implemented fixture coverage:

- poisoned MCP tool description is blocked or quarantined;
- unsafe MCP output instruction is quarantined before reuse;
- provider provenance mismatch is detected;
- sandbox-only external provider path is represented as fixture metadata;
- tool argument schema mismatch, tool-selection attack, expired prepare and
  multi-candidate provider selection are represented as fixture metadata;
- ToolSandbox/MCP-Bench-like adapter samples preserve low-sensitive capability
  lease refs, capability scope refs and provider attestation refs;
- ReplayBundle keeps low-sensitive prepared refs, provider refs and audit refs,
  not raw provider payloads.

Remaining hardening:

- broader MCPSecBench/MCP-Bench fixture families beyond the current local
  sample;
- capability lease matrix review before any production contract promotion;
- provider attestation governance review before trusted provider onboarding;
- prepare expiry re-prepare policy and state-diff linkage after approved
  execution simulation.

## 17. References

- `docs/sdd/mcp-gateway.md`
- `docs/sdd/skill-registry.md`
- `docs/sdd/action-executor.md`
- `docs/sdd/agent-runtime.md`
- `docs/research/agent-open-dataset-eval-plan-20260701.md`
- MCP specification: <https://modelcontextprotocol.io/specification/2025-11-25>
- MCPSecBench: <https://arxiv.org/abs/2508.13220>
- ToolSandbox: <https://aclanthology.org/2025.findings-naacl.65/>
