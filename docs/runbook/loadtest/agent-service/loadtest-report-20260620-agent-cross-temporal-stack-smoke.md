# agent-service Cross-Group / Temporal Stack Smoke - 2026-06-20

Scope: low-sensitive local service-stack smoke for collaborative memory
consumption in the proposal path. This is not a production benchmark, capacity
result, or LLM quality claim.

## Result

Passed.

Raw summary:

```text
H:\NexusIM\loadtest-results\agent-cross-temporal-stack-20260620\agent-proposal-summary.json
```

## Command

```powershell
. .\tools\go-env.ps1
.\tools\run-agent-adapter-smoke.ps1 `
  -RunName "agent-cross-temporal-stack-20260620" `
  -RequestTimeout "30s"
```

## Key Evidence

`agent-proposal-summary.json` recorded:

| Field | Value |
| --- | --- |
| `proposal_status` | `PROPOSED` |
| `requires_approval` | `true` |
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
| `mcp_audit.status` | `ALLOWED` |
| `execution_executed` | `false` |

The seeded memory event carried source refs from the primary conversation and a
secondary strategy conversation. The proposal path preserved attribution through
EvidencePack, then continued through `mcp-gateway` prepare audit and approval
preflight without executing a business mutation.

## Boundary

`agent-service` still produced a proposal only. Real writes remain behind
policy, proposal / approval, action-executor, and audit.
