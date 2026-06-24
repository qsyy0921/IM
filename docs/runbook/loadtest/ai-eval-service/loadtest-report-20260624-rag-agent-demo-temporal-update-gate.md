# RAG-Agent Demo Temporal Update Gate 2026-06-24

## Scope

This report records a focused RAG-Agent demo service-stack gate for public
memory candidate temporal updates.

The run verifies:

- memory-service public `SubmitMemoryCandidate -> ReviewMemoryCandidate(APPROVE)`
  can approve an initial memory and then approve a replacement candidate with
  `supersedes_event_ids`;
- approving the replacement marks the original memory as `SUPERSEDED` and sets
  `valid_to_seq` to the sequence before the replacement;
- RAG and Agent EvidencePacks only contain the current `ACTIVE + APPROVED`
  replacement memory;
- Agent proposal, approval and action-executor audit still complete around the
  same tenant / conversation;
- the summary remains low-sensitive and stores hashes / ids / counts, not raw
  answer, proposal or memory text.

## Command

```powershell
.\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter rag-agent-demo `
  -RunName ai-eval-rag-agent-demo-live-20260624-temporal-update-v2 `
  -ResultRoot H:\NexusIM\loadtest-results
```

Before the run, `memory-service` was rebuilt and `memory-service-grpc` was
recreated so the live service used the updated approval / supersede transaction
logic.

## Result

| Field | Value |
| --- | --- |
| gate status | `passed` |
| run name | `ai-eval-rag-agent-demo-live-20260624-temporal-update-v2` |
| result dir | `H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-temporal-update-v2` |
| adapter count | `4` |
| selected optional adapter | `rag-agent-demo` |
| case count | `27` |
| passed | `27` |
| failed | `0` |
| skipped | `0` |

## Key RAG-Agent Summary

Source:

```text
H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-temporal-update-v2-rag-agent-demo\rag-agent-demo-summary.json
```

Important low-sensitive fields:

| Field | Value |
| --- | --- |
| `rag_answer_status` | `GROUNDED` |
| `rag_citation_count` | `3` |
| `rag_evidence_item_count` | `3` |
| `agent_proposal_status` | `APPROVED` |
| `agent_requires_approval` | `true` |
| `action_execution_status` | `RECORDED` |
| `shared_tenant_and_conversation` | `true` |
| `cross_group_source_refs_preserved` | `true` |
| `cross_group_speaker_attribution_preserved` | `true` |
| `memory_graph_edges_preserved` | `true` |
| `profile_aggregate_preserved` | `true` |
| `public_candidate_review_approved` | `true` |
| `public_candidate_evidence_in_rag` | `true` |
| `public_candidate_evidence_in_agent` | `true` |
| `public_candidate_temporal_update_preserved` | `true` |

The summary includes `public_candidate_memory_event_id`,
`public_candidate_superseded_memory_event_id` and `public_candidate_fact_sha256`
for traceability. It does not include raw answer text, raw proposal text or raw
memory fact text.

## Interpretation

This run upgrades the previous public candidate review evidence chain into a
temporal update chain. The important invariant is that the current EvidencePack
contains the replacement memory only; the original public candidate is still
auditable through memory-service state as `SUPERSEDED`, but it is not treated as
current evidence by RAG or Agent.

## Limits

- This is not a capacity or long-running HA test.
- It does not prove vector / BM25 / rerank quality.
- It does not exercise profile repair approval.
- It does not make Agent execute a real business mutation; action-executor still
  records the approved action boundary and audit status for the demo.
