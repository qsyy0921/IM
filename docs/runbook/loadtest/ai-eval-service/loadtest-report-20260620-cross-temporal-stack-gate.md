# AI Eval Cross-Group / Temporal Stack Gate - 2026-06-20

Scope: low-sensitive local AI eval gate run. This is not a production
benchmark, capacity result, or LLM quality claim.

## Result

Passed.

Raw summaries were written under:

```text
H:\NexusIM\loadtest-results\ai-eval-cross-temporal-stack-gate-20260620
```

## Command

```powershell
. .\tools\go-env.ps1
.\tools\run-ai-eval-regression-gate-smoke.ps1 `
  -OptionalAdapter "rag-service,summary-service,agent-action-executor" `
  -RunName "ai-eval-cross-temporal-stack-gate-20260620" `
  -RequestTimeout "30s"
```

## Gate Summary

`ai-eval-regression-gate-summary.json`:

| Field | Value |
| --- | --- |
| `status` | `passed` |
| `adapter_count` | `6` |
| `case_count` | `40` |
| `passed_count` | `40` |
| `failed_count` | `0` |
| `skipped_count` | `0` |

Selected optional service-stack adapters:

- `rag-service`
- `summary-service`
- `agent-action-executor`

## New Assertions In Gate

The existing RAG, Summary, and Agent current-memory service-stack cases now
also assert:

- `must_not_return_future_memory_as_current`
- `must_preserve_cross_group_source_refs`
- `must_preserve_speaker_attribution`

All three adapters passed these assertions while preserving the previous stale
memory exclusion, projection-version, citation, proposal-only, and low-sensitive
audit checks.

## Boundary

This proves the local AI eval gate can verify cross-group source attribution and
temporal current-memory behavior when explicit service-stack adapters are
selected. The default CI-safe gate still avoids live service dependencies.
