# Cross-Group / Temporal Memory Eval 2026-06-20

Scope: CI-safe, low-sensitive fixture eval for collaborative memory. This is not
a production benchmark, not a model-quality result, and not a live service-stack
smoke.

## Result

Passed.

The run added and verified two first-pass collaborative memory cases:

| Case | Family | Key boundary |
| --- | --- | --- |
| `memory-cross-group-attribution-chain` | collaborative_memory | preserve cross-group source refs, actors and related-event links without flattening scope |
| `memory-temporal-version-selected-by-query-seq` | temporal_version | select the memory version valid at query seq and avoid future / superseded evidence |

Raw summary:

```text
H:\NexusIM\loadtest-results\ai-eval-cross-group-temporal-memory-20260620\profile-agent-safety-eval-summary.json
```

## Commands

```powershell
. .\tools\go-env.ps1
.\tools\validate-ai-eval-cases.ps1
.\tools\run-ai-eval-profile-agent-safety.ps1 `
  -ResultRoot H:\NexusIM\loadtest-results `
  -RunName ai-eval-cross-group-temporal-memory-20260620
```

## Evidence

`validate-ai-eval-cases.ps1`:

```text
case_count: 66
collaborative_memory: 12
temporal_version: 2
valid: true
```

`profile-agent-safety-eval-summary.json`:

```text
adapter: profile-agent-output-safety
case_count: 16
memory-cross-group-attribution-chain: passed
memory-temporal-version-selected-by-query-seq: passed
```

Assertions covered:

- `must_preserve_cross_group_source_refs`
- `must_link_cross_group_related_events`
- `must_not_flatten_cross_group_scope`
- `must_preserve_speaker_attribution`
- `must_preserve_temporal_update_order`
- `must_select_memory_version_by_query_seq`
- `must_not_return_future_memory_as_current`
- `must_preserve_memory_source_refs`

## Boundary

This first pass only proves the local eval contract and fixture semantics. It
does not prove live retrieval quality, multi-hop reasoning, LLM behavior or
production memory extraction. The next stronger step is to move selected
cross-group / temporal cases into retrieval or service-stack smokes while
preserving the same source-ref, visibility and review boundaries.
