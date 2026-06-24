# RAG-Agent business mutation execute gate

Date: 2026-06-25

Scope: low-sensitive local service-stack smoke for `rag-agent-demo` opt-in
business mutation execution. This is not a capacity test, production HA proof,
model-quality benchmark, or provider-grade operations report.

## Command

```powershell
$run='ai-eval-rag-agent-demo-live-20260625-business-mutation-execute-v7'
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter rag-agent-demo `
  -ExpectBusinessActionExecuted `
  -NoApplyMigration `
  -RunName $run `
  -RequestTimeout 120s
```

## Result

| Field | Value |
| --- | --- |
| status | `passed` |
| run name | `ai-eval-rag-agent-demo-live-20260625-business-mutation-execute-v7` |
| result dir | `H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260625-business-mutation-execute-v7` |
| adapters | `4` |
| cases | `27` |
| passed | `27` |
| failed | `0` |
| skipped | `0` |

Selected optional adapter:

- `rag-agent-demo`

## Adapter Assertions

`rag-agent-demo-eval-summary.json` confirmed:

- `rag_must_answer_grounded`
- `agent_must_record_approved_proposal`
- `action_executor_must_record_audit`
- `rag_and_agent_must_share_scope`
- `evidencepack_must_preserve_collaborative_memory`
- `public_candidate_review_must_enter_rag_agent_evidence_chain`
- `group_memory_answer_and_proposal_must_preserve_multievent_evidence`
- `business_proposal_must_execute_approved_conversation_note`
- `profile_repair_must_require_workflow_approval_and_enter_evidence_chain`
- `profile_repair_negative_gate_must_fail_closed`
- `summary_must_be_low_sensitive`
- `versions_must_be_present`

`rag-agent-demo-summary.json` confirmed:

- `rag_answer_status=GROUNDED`
- `agent_proposal_status=APPROVED`
- `action_execution_recorded=true`
- `action_executed=true`
- `group_memory_answer_verified=true`
- `group_memory_proposal_verified=true`
- `profile_repair_negative_cases_verified=true`
- `business_proposal_verified=true`
- `business_proposal_approval_recorded=true`
- `business_action_audit_recorded=true`
- `business_action_executed=true`
- `business_action_mode=execute`
- `business_note_persisted=true`
- `business_execution_status=RECORDED`
- `business_tool_name=conversation.note.create`
- `business_skill_id=conversation.note.create`
- `business_requires_approval=true`
- `business_policy_allowed=true`
- `business_policy_requires_approval=true`

## What This Proves

This run proves the first local service-stack path where:

```text
reviewed group memory
-> EvidencePack
-> RAG grounded answer
-> Agent conversation.note.create proposal
-> approval
-> action-executor
-> conversation-service CreateConversationNote
-> persisted conversation note fact
```

The smoke keeps the existing safety boundary:

- Agent does not write conversation facts directly.
- The business mutation is opt-in and requires explicit
  `-ExpectBusinessActionExecuted`.
- The default `rag-agent-demo` path remains audit-only.
- The note body is not echoed in tool output; reports store hashes and refs only.
- The business action still requires skill metadata, policy allow, approval and
  action-executor execution.

## Implementation Notes

- `action-executor-grpc` now receives
  `NEXUSIM_ACTION_EXECUTOR_CONVERSATION_GRPC_ADDR` in local compose so it can
  call conversation-service for the explicit business adapter.
- `agent-service-grpc` local compose uses `NEXUSIM_AGENT_DEPENDENCY_TIMEOUT=5s`
  for this service-stack path.
- `loadtest/ragagent` creates the target conversation through
  conversation-service public `CreateConversation` before verifying the approved
  note mutation.
- `loadtest/ragagent` seeds only skill-registry / policy configuration rows for
  the run after the child agent runner resets tenant tooling state; it does not
  write business facts directly.
- `run-ai-eval-regression-gate-smoke.ps1` now passes `rag-agent-demo` adapter
  arguments by name, so `ResultRoot` and endpoint values cannot drift into
  positional parameters.
- `loadtest/rag` and `loadtest/agent` now accept either exact primary or exact
  cross-group seeded source refs where source-chain rerank can select either
  valid evidence item.

## Boundaries

- Not a production SLO, long soak, throughput test, Redis/Kafka/PostgreSQL HA
  proof, or model-quality evaluation.
- Does not authorize arbitrary tools or mutation types. Future real mutation
  scenarios must first add public business API, explicit tool adapter, policy,
  approval, low-sensitive output and repair / redrive boundaries.
- Does not copy raw loadtest summaries into the repository; raw JSON remains
  under `H:\NexusIM\loadtest-results`.
