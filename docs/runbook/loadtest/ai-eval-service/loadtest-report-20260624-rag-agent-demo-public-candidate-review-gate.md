# AI Eval RAG-Agent Public Candidate Review Gate - 2026-06-24

Scope: low-sensitive local AI eval service-stack gate run. This is not a
production benchmark, capacity result, model-quality claim, or provider-grade
Agent runtime claim.

## Result

Passed.

Raw summaries were written under:

```text
H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-public-candidate-review-v3
```

The composed RAG-Agent demo summary was written under:

```text
H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-public-candidate-review-v3-rag-agent-demo
```

## Command

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter rag-agent-demo `
  -RunName ai-eval-rag-agent-demo-live-20260624-public-candidate-review-v3 `
  -ResultRoot H:\NexusIM\loadtest-results
```

During bring-up, the local Docker service stack was started from existing
compose files. `memory-service:local` was rebuilt and the dependent containers
were recreated because the previously running image did not expose the current
`SubmitMemoryCandidate` RPC. No new middleware or provider was introduced.

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

## RAG-Agent Public Candidate Evidence

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
| `public_candidate_evidence_in_rag` | `true` |
| `public_candidate_evidence_in_agent` | `true` |

The `rag-agent-demo` adapter also asserted:

```text
public_candidate_review_must_enter_rag_agent_evidence_chain = true
```

This proves the first-stage public path:

```text
memory-service SubmitMemoryCandidate
-> memory-service ReviewMemoryCandidate(APPROVE)
-> retrieval-gateway EvidencePack
-> RAG grounded answer
-> Agent proposal evidence
-> approval / action-executor audit
```

## Confirmed Boundaries

- The candidate is created through memory-service public RPC, not by direct
  database writes.
- The candidate must become `ACTIVE + APPROVED` before RAG / Agent can consume
  it as evidence.
- RAG and Agent only consume retrieval-gateway EvidencePack.
- The report records low-sensitive IDs, hashes, counts, statuses and summary
  paths. It does not persist raw answer text, raw proposal text, raw memory
  fact text, prompt text, EvidencePack payloads, tool input or provider output.

## Notes

An earlier run failed because the existing `memory-service:local` image was
older than the current source and returned `unknown method SubmitMemoryCandidate`.
After rebuilding `memory-service:local`, a second run exposed a client contract
issue: the direct RAG / Agent check lacked `device_id` in `AuthContext`. The
runner now sends explicit device / session / trace / request metadata.

## Next Step

Continue deepening the RAG-Agent demo around EvidencePack / approval / audit:
temporal update cases, profile recompute / repair approval regression, and
additional group-memory answer / proposal scenarios.
