# rag-service Cross-Group / Temporal Stack Smoke - 2026-06-20

Scope: low-sensitive local service-stack smoke for collaborative memory
consumption. This is not a production benchmark, capacity result, or LLM quality
claim.

## Result

Passed.

Raw summaries were written under:

```text
H:\NexusIM\loadtest-results\rag-cross-temporal-stack-20260620
```

## Command

```powershell
. .\tools\go-env.ps1
.\tools\run-ai-eval-rag-adapter.ps1 `
  -RunName "rag-cross-temporal-stack-20260620" `
  -RequestTimeout "30s"
```

## Key Evidence

`rag-answer-summary.json` recorded:

| Field | Value |
| --- | --- |
| `answer_status` | `GROUNDED` |
| `citation_count` | `2` |
| `evidence_item_count` | `2` |
| `search_item_count` | `1` |
| `memory_item_count` | `1` |
| `current_memory_at_seq` | `7` |
| `expired_memory_excluded` | `true` |
| `superseded_memory_excluded` | `true` |
| `future_memory_excluded` | `true` |
| `cross_group_source_refs_preserved` | `true` |
| `cross_group_speaker_attribution_preserved` | `true` |

The seeded memory event included source refs from the primary conversation and a
secondary strategy conversation. The returned EvidencePack preserved both
source refs and actor attribution while excluding expired, superseded, and
future memory decoys at the requested conversation seq.

## Boundary

`rag-service` still consumed only `retrieval-gateway` EvidencePack. It did not
read message, conversation, search, or memory private tables directly and did
not execute Agent actions.
