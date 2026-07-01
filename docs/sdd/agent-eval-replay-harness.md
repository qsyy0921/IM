# Agent Eval / Replay Harness SDD

Date: 2026-07-01
Status: Detailed SDD draft for Agent Exploration Mode. This is not an ADR,
proto, OpenAPI, Kafka schema, migration, production service directory or eval
schema.

## 1. Goal

Define the offline Agent eval and replay harness that validates NexusIM Agent
capabilities before real IM data or production runtime integration.

The harness measures grounded RAG, memory, tool use, workflow/HITL, state-diff,
security, multi-agent handoff and replay completeness with public datasets and
synthetic IM-like fixtures.

## 2. Non-Goals

- Do not use real NexusIM IM data in first-stage eval.
- Do not connect eval fixtures to production startup paths.
- Do not use eval outputs as fallback answers or business facts.
- Do not store raw prompt, provider body, secret or private payload as required
  replay source.
- Do not freeze final EvalCase/EvalRun/EvalResult schema.

## 3. Component Responsibilities

`ai-eval-harness` owns:

- dataset adapters;
- synthetic IM-like fixture loading;
- offline AgentRun trace execution or simulation;
- deterministic fake tool/workflow providers for isolation;
- metric calculation;
- failure classification;
- replay completeness checks;
- report generation.

`ai-eval-service` owns:

- low-sensitive EvalRun catalog;
- EvalReport refs;
- regression comparison metadata;
- release gate result refs.

Agent Runtime owns:

- run/step trace semantics used by harness;
- ReplayBundle refs for cognitive replay.

Other services own their own replay fragments:

- retrieval/RAG: EvidencePack and citation refs;
- memory-service: candidate/review/use refs;
- mcp-gateway: prepare refs;
- workflow-service: workflow decision refs;
- action-executor: execution refs and state-diff evidence;
- audit-service: immutable low-sensitive lineage.

## 4. State Ownership

| State | Owner | Notes |
| --- | --- | --- |
| Dataset source | Public dataset or local fixture | Not production service data |
| EvalCase | Harness | Conceptual test case |
| EvalRun | ai-eval-service catalog | Low-sensitive run metadata |
| EvalResult | Harness + ai-eval-service ref | Case score and failure class |
| EvalReport | ai-eval-service | Aggregation and regression summary |
| ReplayBundle | Agent Runtime / harness | Low-sensitive refs/hashes |
| Fake provider state | Harness only | Never production fallback |
| Expected state diff | Harness fixture | Synthetic state only |

Eval harness cannot own:

- production fallback decision;
- business service final state;
- ACTIVE memory;
- approval decision in production;
- raw private transcript archive.

## 5. Conceptual Artifacts

These are draft fields for design only.

### 5.1 EvalCase

```text
case_id
dataset_name
dataset_version
capability_family
fixture_version
tenant_ref
actor_ref
conversation_or_project_ref
agent_definition_ref
skill_package_ref
input_refs
visible_evidence_refs
forbidden_evidence_refs
allowed_tool_refs
approval_policy_ref
memory_scope_refs
expected_answer_facts
expected_citations
expected_memory_outcome
expected_tool_prepare
expected_state_diff
expected_failure_class
```

### 5.2 ReplayBundle

```text
replay_bundle_ref
agent_run_ref
agent_step_refs
input_hashes
evidence_pack_refs
context_package_refs
model_provider_metadata
candidate_hashes
prepared_tool_refs
workflow_decision_refs
execution_refs
memory_candidate_refs
audit_refs
failure_class
version_metadata
```

ReplayBundle must support analysis without re-executing external side effects.

## 6. Dataset and Fixture Plan

| Capability | Dataset / Fixture | Required Metric |
| --- | --- | --- |
| Grounded RAG | Qasper, HotpotQA, BEIR, NQ | answer correctness, citation, abstain |
| Tool use | ToolSandbox, tau-bench, BFCL, MCP-Bench | tool choice, args, policy, timeout |
| Memory | STATE-Bench, LoCoMo, LongMemEval, EverMemBench, GroupMemBench | recall, update, forget, scope, task improvement |
| State diff | Agent-Diff style synthetic state | expected vs actual state change |
| Policy/HITL | JourneyBench and synthetic approvals | approval wait, reject, timeout |
| MCP security | MCPSecBench and poisoned-tool fixtures | block/quarantine rate |
| Multi-agent | MultiAgentBench / MARBLE + bounded handoff fixtures | delegation correctness and budget containment |

## 7. Required Synthetic IM-Like Fixtures

Minimum fixture families:

- tenant isolation;
- user delegated identity;
- historical group membership;
- group message speaker attribution;
- project decision supersedes;
- memory revocation;
- approval required/approved/rejected/timeout;
- tool registry with safe, high-risk and malicious tools;
- fake execution state for state-diff;
- peer-agent/specialist candidate handoff;
- provider timeout and malformed output.

Fixtures must be clearly synthetic and must not import production data.

## 8. Eval Metrics

| Metric | Meaning |
| --- | --- |
| `grounded_correctness` | Answer facts match visible evidence |
| `citation_coverage` | Key claims map to source refs |
| `abstain_correctness` | Agent refuses when evidence is missing/forbidden |
| `permission_leakage` | Forbidden refs used or surfaced |
| `tool_selection_score` | Correct tool chosen under allowlist |
| `tool_argument_score` | Args pass schema and policy |
| `state_diff_score` | Final synthetic state matches expected diff |
| `memory_precision` | Admitted candidates are valid |
| `memory_scope_score` | Memory stays in allowed scope |
| `memory_revocation_score` | Revoked memory is not used |
| `security_block_score` | Malicious tool/context blocked |
| `handoff_score` | Delegation stays scoped and useful |
| `replay_completeness` | Failure can be reconstructed from refs |

Promotion thresholds start as research baselines. Production SLOs require later
launch review.

## 9. Failure Semantics

The harness should normalize:

- `POLICY_DENIED`
- `INSUFFICIENT_EVIDENCE`
- `CONFLICTING_EVIDENCE`
- `PERMISSION_LEAKAGE`
- `CITATION_MISSING`
- `TOOL_NOT_ALLOWED`
- `TOOL_ARGS_INVALID`
- `TOOL_POISONING_DETECTED`
- `UNSAFE_TOOL_OUTPUT`
- `PROVIDER_TIMEOUT`
- `APPROVAL_REQUIRED`
- `APPROVAL_REJECTED`
- `APPROVAL_TIMEOUT`
- `STATE_DIFF_MISMATCH`
- `MEMORY_SCOPE_VIOLATION`
- `MEMORY_CONFLICT`
- `MEMORY_POLLUTION`
- `HANDOFF_SCOPE_VIOLATION`
- `REPLAY_INCOMPLETE`

Unknown failures should block promotion until classified.

## 10. Key Flows

### 10.1 Dataset Adapter

```text
public task
-> adapter
-> synthetic IM envelope
-> EvalCase
-> offline run / simulation
-> EvalResult
-> EvalReport
```

### 10.2 State-Diff Eval

```text
initial synthetic state
-> agent proposal and approved execution simulation
-> final synthetic state
-> diff against expected state
```

Trace matching is not enough for action tasks.

### 10.3 Replay Eval

```text
failed EvalResult
-> ReplayBundle
-> reconstruct refs, versions and failure class
-> verify no external side effect is re-run
```

If replay needs raw prompt or private payload, replay design fails.

### 10.4 Regression Gate

```text
AgentDefinition / SkillPackage candidate
-> required eval suites
-> compare baseline
-> block if P0/P1 regression
-> record EvalReport ref
```

## 11. Security Boundary

Eval must include adversarial cases:

- prompt injection inside retrieved evidence;
- malicious MCP tool descriptions;
- unsafe tool outputs;
- peer-agent manipulation;
- memory pollution;
- permission boundary confusion;
- approval bypass attempts;
- fake provider timeout and malformed output.

The harness itself must not normalize unsafe behavior as success.

## 12. Observability / Audit

Eval reports should include:

- suite version;
- dataset versions;
- adapter versions;
- model/provider metadata;
- AgentDefinition and SkillPackage refs;
- aggregate scores;
- regression delta;
- failure class distribution;
- examples by failure class;
- replay completeness;
- blocked promotion reasons.

ai-eval-service stores low-sensitive metadata and report refs, not raw private
payload archives.

## 13. Risks / Rejection Conditions

Reject eval harness promotion if:

- it needs real IM data for initial capability proof;
- it cannot represent forbidden evidence;
- it cannot measure memory pollution;
- it only scores answer text and not state/action outcomes;
- fake providers can be wired into production fallback;
- replay re-executes side effects;
- raw provider body is required for normal replay;
- unknown failures are counted as pass.

## 14. Promotion Conditions

The harness becomes a required release gate only after:

- first trio adapters run repeatably;
- synthetic IM-like fixture loader is deterministic;
- failure taxonomy is stable enough for regression reports;
- ReplayBundle completeness is measured;
- security fixtures are included;
- reports link to AgentDefinition / SkillPackage governance.

## 15. Implementation Slice 0

Current isolated coding slice:

```text
ai/python/nexusim_ai_eval/
  adapters.py
  adapter_runner.py
  comparison.py
  contracts.py
  evaluator.py
  fixtures.py
  trace.py
ai/python/fixtures/agent_eval/adapter_samples/
ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json
ai/python/fixtures/agent_eval/synthetic_first_trio.json
ai/python/fixtures/agent_eval/synthetic_core_scenarios.json
ai/python/fixtures/agent_eval/synthetic_runtime_control_scenarios.json
ai/python/fixtures/agent_eval/synthetic_mcp_security_scenarios.json
ai/python/scripts/run_agent_eval_fixture.py
ai/python/scripts/run_agent_dataset_adapter.py
ai/python/scripts/run_agent_eval_regression.py
ai/python/tests/test_agent_eval_contracts.py
ai/python/tests/test_agent_eval_evaluator.py
ai/python/tests/test_agent_eval_integration.py
ai/python/tests/test_agent_eval_adapters.py
ai/python/tests/test_agent_eval_adapter_runner.py
ai/python/tests/test_agent_eval_comparison.py
ai/python/tests/test_agent_eval_trace.py
```

This slice implements the first offline harness mechanics only. It does not
call backend services, model providers, databases, Kafka, Redis, OpenSearch,
MCP providers, workflow-service, action-executor or memory-service.

Implemented checks:

- low-sensitive EvalCase validation;
- rejection of raw prompt, backend URL and production payload fields;
- grounded RAG citation and permission leakage scoring;
- memory admission outcome/scope/revocation scoring;
- tool poisoning and unsafe output scoring;
- state-diff mismatch scoring;
- ReplayBundle completeness and side-effect reexecution rejection;
- CLI integration on `synthetic_first_trio.json`.
- public dataset adapter skeletons for Qasper/HotpotQA-like RAG,
  ToolSandbox/tau-bench-like tool and STATE-Bench/LoCoMo-like memory;
- AgentRun / AgentStep trace skeleton with EvidencePack, ContextPackage,
  MemoryCandidate and ToolIntent fixture refs;
- core scenario fixture coverage for abstain, permission leakage, memory
  pollution, unsafe output, approval timeout, provider timeout, state-diff
  mismatch and bounded handoff.
- local public-dataset-style sample payloads for Qasper-like RAG,
  ToolSandbox-like tool security and STATE-Bench-like memory;
- batch adapter conversion / run CLI;
- EvalReport baseline comparison with aggregate deltas, case deltas and blocked
  promotion reasons.
- runtime-control fixture coverage for cancel propagation, checkpointed approval
  resume and replay without side-effect reexecution.
- MCP security fixture coverage for poisoned tool descriptions, unsafe MCP
  output instructions, provider provenance mismatch and sandbox-only providers.

Focused verification:

```powershell
python -m pytest ai/python/tests/test_agent_eval_contracts.py ai/python/tests/test_agent_eval_evaluator.py ai/python/tests/test_agent_eval_integration.py -q
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_first_trio.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_core_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_runtime_control_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_mcp_security_scenarios.json
python ai/python/scripts/run_agent_dataset_adapter.py --run ai/python/fixtures/agent_eval/adapter_samples/qasper_like_rag_samples.json
python ai/python/scripts/run_agent_eval_regression.py ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json
```

## 16. References

- `docs/sdd/ai-eval-service.md`
- `docs/sdd/ai-eval-harness.md`
- `docs/research/agent-open-dataset-eval-plan-20260701.md`
- `docs/research/agent-coding-experiment-path-20260701.md`
- ToolSandbox: <https://aclanthology.org/2025.findings-naacl.65/>
- Agent-Diff: <https://arxiv.org/abs/2602.11224>
- STATE-Bench:
  <https://opensource.microsoft.com/blog/2026/05/19/introducing-state-bench-a-benchmark-for-ai-agent-memory/>
- MCPSecBench: <https://arxiv.org/abs/2508.13220>
