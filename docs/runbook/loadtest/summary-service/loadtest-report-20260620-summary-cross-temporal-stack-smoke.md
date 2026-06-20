# summary-service Cross-Group / Temporal Stack Smoke - 2026-06-20

Scope: low-sensitive local service-stack smoke for collaborative memory
consumption. This is not a production benchmark, capacity result, or LLM quality
claim.

## Result

Passed.

Raw summary:

```text
H:\NexusIM\loadtest-results\summary-cross-temporal-stack-20260620\summary-service-summary.json
```

## Command

```powershell
. .\tools\go-env.ps1
.\tools\run-summary-adapter-smoke.ps1 `
  -RunName "summary-cross-temporal-stack-20260620" `
  -RequestTimeout "30s"
```

## Key Evidence

`summary-service-summary.json` recorded:

| Field | Value |
| --- | --- |
| `summary_status` | `GROUNDED` |
| `generated_by_llm` | `false` |
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

The seeded memory event carried source refs from the primary conversation and a
secondary strategy conversation. The summary path preserved cross-group source
refs and speaker attribution, while excluding stale and future memory decoys.

## Boundary

`summary-service` still consumed only `retrieval-gateway` EvidencePack. It did
not read private source tables directly and did not perform business mutations.
