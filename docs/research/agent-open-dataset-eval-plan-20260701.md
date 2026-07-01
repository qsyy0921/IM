# Agent Open Dataset Eval Plan

Date: 2026-07-01
Mode: Agent Exploration Mode
Status: Eval design draft. This is not production code, schema, migration or
service contract.

## 1. Purpose

NexusIM should validate Agent capabilities with public datasets and synthetic
IM-like fixtures before using real IM data. This plan defines the first
evaluation loop for RAG, memory, tools, workflow, state-diff, security,
multi-agent handoff and governance signals.

The plan supports the SDD package without freezing proto, OpenAPI, Kafka schema,
EvidencePack shape, memory event shape, tool shape or runtime shape.

## 2. Evaluation Principles

1. Public datasets stay separate from NexusIM product facts.
2. Synthetic IM-like fixtures model tenants, users, groups, projects, policies,
   approvals, tools and memory scopes with fake data only.
3. Eval measures final outcome, source grounding, policy adherence and state
   changes, not only model answer similarity.
4. Failing evals must produce a low-sensitive failure class and replay artifact.
5. Eval output cannot become production fallback.
6. Real NexusIM IM messages, private users and tenant data are out of scope for
   this first stage.

## 3. First Dataset Families

| Capability | First Candidates | Why |
| --- | --- | --- |
| Grounded RAG | Qasper, HotpotQA, BEIR subset, Natural Questions | Tests evidence retrieval, multi-hop grounding, citation and abstain |
| Tool / workflow | ToolSandbox, tau-bench, BFCL, MCP-Bench | Tests tool selection, arguments, hidden state and provider failure |
| Memory | STATE-Bench, LoCoMo, LongMemEval, EverMemBench, GroupMemBench | Tests whether memory improves later work without pollution |
| State diff | Agent-Diff style fixtures | Tests final external state, not only trace similarity |
| Policy adherence | JourneyBench, tau-bench policy domains | Tests dynamic SOP and approval requirements |
| MCP security | MCPSecBench and custom poisoned-tool fixtures | Tests malicious tool descriptions, outputs and selection attacks |
| Multi-agent | MultiAgentBench / MARBLE plus bounded handoff fixtures | Tests delegation under budget and scope limits |

First practical trio:

1. Qasper or HotpotQA for grounded RAG.
2. ToolSandbox or tau-bench for tool/workflow.
3. STATE-Bench or LoCoMo for memory.

Add MCPSecBench-style malicious fixtures as soon as tool prepare exists in the
offline harness.

## 4. Synthetic IM-Like Fixture Model

Every dataset task can be wrapped in a fake IM envelope:

| Fixture Entity | Required Fields | Purpose |
| --- | --- | --- |
| Tenant | `tenant_id`, policy profile, retention profile | Tests isolation and admin policy |
| User | `user_id`, role, group memberships, delegated scopes | Tests `on_behalf_of` and permission windows |
| Group | `group_id`, members, historical membership windows | Tests group memory and visibility |
| Conversation | message refs, speaker refs, timestamps, redactions | Tests retrieval, citation and temporal validity |
| Project | project id, decisions, artifacts, supersedes chain | Tests project memory |
| Tool Registry | tool id, owner, risk tier, schema hash, provenance | Tests skill allowlist and MCP poisoning |
| Approval | approver, timeout, outcome, decision ref | Tests HITL pause/resume |
| Business State | fake records and expected state diff | Tests action outcome |
| Memory Scope | personal, group, project, org, eval-only | Tests leakage and revocation |

All fixture ids are synthetic. Dataset ground truth remains external to product
state, and product-like records exist only to test Agent boundaries.

## 5. Conceptual Eval Artifacts

These are draft fields for documentation and harness design, not schemas.

### 5.1 EvalCase

```text
EvalCase
  case_id
  dataset_name
  dataset_version
  capability_family
  fixture_version
  tenant_ref
  actor_ref
  agent_definition_ref
  skill_package_ref
  input_refs
  visible_evidence_refs
  hidden_or_forbidden_refs
  allowed_tool_refs
  memory_scope_refs
  approval_policy_ref
  expected_answer_facts
  expected_citation_refs
  expected_state_diff
  expected_memory_outcome
  expected_failure_class
```

### 5.2 EvalRun

```text
EvalRun
  run_id
  suite_id
  case_ids
  model_provider_ref
  model_version
  adapter_version
  harness_version
  agent_runtime_version
  seed_or_determinism_ref
  start_time
  end_time
  aggregate_result_ref
```

### 5.3 EvalResult

```text
EvalResult
  case_id
  status
  answer_score
  citation_score
  abstain_score
  tool_score
  state_diff_score
  memory_score
  security_score
  policy_score
  replay_score
  failure_class
  replay_bundle_ref
  low_sensitive_examples_ref
```

### 5.4 EvalReport

```text
EvalReport
  suite_id
  branch_or_release_ref
  summary_scores
  regression_delta
  failure_distribution
  blocked_promotion_reasons
  notable_examples
  residual_risks
```

## 6. Adapter Interfaces

Adapters should convert public tasks to `EvalCase` without importing production
service packages.

| Adapter | Input | Output | Must Not Do |
| --- | --- | --- | --- |
| RAG adapter | documents, questions, gold answers | visible evidence refs and expected citations | Use real IM data |
| Tool adapter | tool environment, user task, hidden state | fake tool registry and expected state diff | Execute production providers |
| Memory adapter | episodes, future tasks, memory labels | memory candidates and expected recall/update/forget | Store ACTIVE production memory |
| Security adapter | malicious tool docs/output/prompt content | expected block or quarantine result | Treat malicious text as trusted |
| Multi-agent adapter | roles, task, constraints | bounded delegation case | Allow open-ended peer chat |

## 7. Metrics

| Metric | Meaning | Promotion Direction |
| --- | --- | --- |
| Grounded correctness | Answer matches supported facts | Must pass for read-only QA |
| Citation coverage | Key claims have valid evidence refs | High threshold before production |
| Abstain precision | Refuses when evidence is missing or forbidden | Must not hallucinate |
| Permission leakage | Forbidden refs used or surfaced | Must be zero in eval |
| Tool selection accuracy | Correct tool chosen under allowlist | Required for tool prepare |
| Argument validity | Tool args pass schema and policy | Required before approval |
| State-diff correctness | Final fake state matches expectation | Required for action promotion |
| Memory precision | Durable memory candidates are correct | Required before ACTIVE memory |
| Memory scope safety | Personal/group/project boundaries hold | Must be zero leakage |
| Revocation correctness | Revoked memory is not used | Required for memory promotion |
| MCP poisoning block | Malicious tool description/output blocked | Required before external MCP |
| Replay completeness | Failure can be replayed from low-sensitive refs | Required for debugging |

Thresholds should start as research baselines, not production SLOs. Production
thresholds require separate ADR / launch review.

## 8. Required Eval Scenarios

Minimum first suite:

- Read-only question where evidence is sufficient.
- Read-only question where evidence is missing and the correct answer is abstain.
- Read-only question where the user lacks permission to the best evidence.
- Group memory candidate with correct speaker and group scope.
- Group memory candidate that must be rejected as overgeneralized personal
  preference.
- Project memory update that supersedes an older decision.
- Approval-required action that pauses, resumes after approval and then reaches
  executor.
- Approval timeout that finalizes without execution.
- Tool provider timeout before side effect.
- MCP poisoned tool description that tries to hijack selection.
- Unsafe tool output that tries prompt injection into the next step.
- Bounded multi-agent handoff where specialist output is candidate-only.
- Replay of failed run without re-executing side effects.

## 9. Failure Taxonomy

The offline harness should classify at least:

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
- `REPLAY_INCOMPLETE`

## 10. Development Flow

Recommended sequence:

1. Build fixture-only adapter code under an explicitly experimental or test-only
   path after this design is accepted.
2. Generate a tiny suite for the first trio: RAG, tool/workflow, memory.
3. Add malicious MCP/tool fixtures before any external MCP provider is promoted.
4. Add state-diff checks for every approval/action scenario.
5. Add ReplayBundle checks before treating failures as debuggable.
6. Link EvalReport results to AgentDefinition / SkillPackage release gates.

No production service should consume eval fixtures as fallback data.

## 11. Rejection Conditions

Reject the next implementation step if:

- it needs real NexusIM IM data to prove basic Agent capability;
- it cannot represent forbidden evidence and permission leakage;
- it cannot measure memory pollution or revocation;
- it only checks tool-call trace and not final state diff;
- it stores raw prompt, provider body, secret or private payload as replay source;
- it uses model output as ground truth for eval.

## 12. References

- Qasper, HotpotQA, BEIR, Natural Questions and MS MARCO for grounded RAG.
- ToolSandbox: <https://aclanthology.org/2025.findings-naacl.65/>
- tau-bench: <https://arxiv.org/abs/2406.12045>
- BFCL and MCP-Bench for tool selection and tool protocol evaluation.
- STATE-Bench:
  <https://opensource.microsoft.com/blog/2026/05/19/introducing-state-bench-a-benchmark-for-ai-agent-memory/>
- Agent-Diff: <https://arxiv.org/abs/2602.11224>
- GroupMemBench:
  <https://www.microsoft.com/en-us/research/publication/groupmembench-benchmarking-llm-agent-memory-in-multi-party-conversations/>
- MCPSecBench: <https://arxiv.org/abs/2508.13220>
