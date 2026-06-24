# AI Eval RAG-Agent Profile Repair Approval Gate - 2026-06-24

Scope: low-sensitive local AI eval service-stack gate run. This is not a
production benchmark, capacity result, model-quality claim, or provider-grade
Agent runtime claim.

## Result

Passed.

Raw summaries were written under:

```text
H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-profile-repair-approval-v3
```

The composed RAG-Agent demo summary was written under:

```text
H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-profile-repair-approval-v3-rag-agent-demo
```

## Command

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter rag-agent-demo `
  -RunName ai-eval-rag-agent-demo-live-20260624-profile-repair-approval-v3 `
  -ResultRoot H:\NexusIM\loadtest-results
```

The run used clean commit `78a11571`; the child memory-profile request and
execute summaries both recorded `git_dirty=false`.

## Gate Summary

`ai-eval-regression-gate-summary.json`:

| Field | Value |
| --- | --- |
| `status` | `passed` |
| `adapter_count` | `4` |
| `case_count` | `27` |
| `passed_count` | `27` |
| `failed_count` | `0` |
| `skipped_count` | `0` |

Selected optional service-stack adapter:

- `rag-agent-demo`

Adapter summary:

| Adapter | Cases | Passed | Failed | Skipped |
| --- | ---: | ---: | ---: | ---: |
| `profile-agent-safety` | 20 | 20 | 0 | 0 |
| `action-external-http-provider` | 2 | 2 | 0 | 0 |
| `action-external-mcp-failure` | 4 | 4 | 0 | 0 |
| `rag-agent-demo` | 1 | 1 | 0 | 0 |

## Profile Repair Evidence

`rag-agent-demo-summary.json` confirmed:

| Field | Value |
| --- | --- |
| `rag_answer_status` | `GROUNDED` |
| `rag_citation_count` | `3` |
| `rag_evidence_item_count` | `3` |
| `agent_proposal_status` | `APPROVED` |
| `agent_requires_approval` | `true` |
| `agent_approval_recorded` | `true` |
| `action_execution_status` | `RECORDED` |
| `action_executed` | `false` |
| `action_result_recorded` | `true` |
| `shared_tenant_and_conversation` | `true` |
| `cross_group_source_refs_preserved` | `true` |
| `cross_group_speaker_attribution_preserved` | `true` |
| `memory_graph_edges_preserved` | `true` |
| `profile_aggregate_preserved` | `true` |
| `public_candidate_review_approved` | `true` |
| `public_candidate_temporal_update_preserved` | `true` |
| `profile_repair_approval_requested` | `true` |
| `profile_repair_workflow_approved` | `true` |
| `profile_repair_approval_verified` | `true` |
| `profile_repair_executed` | `true` |
| `profile_repair_profile_active` | `true` |
| `profile_repair_support_count` | `2` |
| `profile_repair_supporting_memory_count` | `2` |
| `profile_repair_evidence_in_rag` | `true` |
| `profile_repair_evidence_in_agent` | `true` |

The `rag-agent-demo` adapter asserted:

```text
profile_repair_must_require_workflow_approval_and_enter_evidence_chain = true
```

The memory-profile request / execute child summaries confirmed:

| Field | Request | Execute |
| --- | --- | --- |
| `approval_workflow_type` | `REPAIR_APPROVAL` | `REPAIR_APPROVAL` |
| `approval_workflow_status` | `WAITING_DECISION` | `APPROVED` |
| `approval_requested` | `true` | n/a |
| `approval_verified` | n/a | `true` |
| `executed` | `false` | `true` |
| `success` | `true` | `true` |
| `support_count` | n/a | `2` |
| `supporting_memory_count` | n/a | `2` |

## Confirmed Boundary

This proves the first-stage profile repair path:

```text
memory-service SubmitMemoryCandidate(PROFILE_SIGNAL)
-> memory-service ReviewMemoryCandidate(APPROVE)
-> memoryprofile --request-approval
-> workflow-service RecordWorkflowDecision(APPROVE)
-> memoryprofile --execute
-> memory-service RecomputeProfileAggregate
-> retrieval-gateway EvidencePack
-> RAG grounded answer
-> Agent proposal evidence
-> approval / action-executor audit
```

Confirmed constraints:

- Profile repair requires an explicit `REPAIR_APPROVAL` workflow before execution.
- The execute step verifies workflow type, approved status, payload hash and
  target hash before calling memory-service.
- The profile aggregate is recomputed through memory-service public RPC, not by
  direct database writes.
- The repaired profile aggregate enters both RAG and Agent EvidencePack.
- The report stores low-sensitive IDs, hashes, counts, statuses and summary
  paths. It does not persist raw memory fact text, raw answer text, raw proposal
  text, prompt text, EvidencePack payloads, tool input or provider output.

## Bring-up Note

An earlier run exposed a memory-service repository edge case: a seeded profile
aggregate could already exist for the same `(tenant, subject_user_id,
aggregate_type, aggregate_key)` with a non-deterministic `profile_id`.
`RecomputeProfileAggregate` now updates that existing subject-key row before
trying to insert a deterministic profile id, and the behavior is covered by a
real PostgreSQL integration test.

## Next Step

Continue broadening the demo around group-memory answer / proposal scenarios,
more profile repair negative cases, and additional EvidencePack source-chain
coverage.
