# AI Eval RAG-Agent Demo Service-Stack Gate - 2026-06-24

Scope: low-sensitive local AI eval gate run. This is not a production benchmark,
capacity result, model-quality claim, or provider-grade Agent runtime claim.

## Result

Passed.

Raw summaries were written under:

```text
H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-current-image-fixed
```

## Command

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter rag-agent-demo `
  -RunName ai-eval-rag-agent-demo-live-20260624-current-image-fixed
```

Before the final run, the affected live service images were refreshed and the
containers were recreated for:

- `memory-service`
- `retrieval-gateway`
- `rag-service`
- `agent-service`

This was required because the running containers were older than the current
EvidencePack profile aggregate and RAG-Agent demo code.

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

## RAG-Agent Demo Evidence

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

The report records only low-sensitive IDs, hashes, counts, statuses, and summary
paths. It does not persist raw answer text, raw proposal text, prompt text,
EvidencePack payloads, tool input, or provider output.

## Fixes Made During Run

- `loadtest/rag`, `loadtest/retrieval`, and `loadtest/agent` now use explicit
  PostgreSQL `::bigint` casts for seed parameters that are embedded into JSON
  projection rows.
- `loadtest/rag` and `loadtest/agent` now insert stale memory decoy events before
  graph edges that reference them, matching the `memory_graph_edges` foreign-key
  contract.
- `loadtest/agent` now reports the final approved proposal status in its summary
  after the explicit `ApproveAgentProposal` RPC, while still asserting that the
  proposal was initially created as `PROPOSED` and required approval.

## Confirmed Boundaries

- RAG consumes retrieval-gateway EvidencePack and returns a grounded answer with
  citations.
- Agent consumes the same tenant / conversation EvidencePack, creates a proposal,
  records approval, and hands execution to action-executor.
- action-executor records an approved execution audit and result projection
  without executing a business mutation in this demo.
- Cross-group source refs, speaker attribution, memory graph edges, and profile
  aggregate evidence are preserved through the demo path.

## Next Step

Continue with profile repair batch / approval and Python memory extraction
candidate evaluation. Keep this RAG-Agent demo as the current low-sensitive live
service-stack proof for:

```text
IM message projection -> EvidencePack -> RAG answer -> Agent proposal -> approval -> audit
```
