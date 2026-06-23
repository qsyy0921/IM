# AI Eval Collaborative Memory Service-Stack Gate - 2026-06-24

Scope: low-sensitive local AI eval gate run. This is not a production benchmark,
capacity result, model-quality claim, or full retrieval negative-suite claim.

## Result

Passed.

Raw summaries were written under:

```text
H:\NexusIM\loadtest-results\ai-eval-service-stack-live-20260624-collab-memory-v4
```

## Command

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter memory-service,retrieval-gateway,rag-service,summary-service,agent-action-executor `
  -ResultRoot H:\NexusIM\loadtest-results `
  -RunName ai-eval-service-stack-live-20260624-collab-memory-v4
```

Before the final run, the local live stack was refreshed for the affected
services: `memory-service`, `retrieval-gateway`, `rag-service`,
`summary-service`, `agent-service`, and `action-executor`.

## Gate Summary

`ai-eval-regression-gate-summary.json`:

| Field | Value |
| --- | --- |
| `status` | `passed` |
| `adapter_count` | `8` |
| `case_count` | `51` |
| `passed_count` | `47` |
| `failed_count` | `0` |
| `skipped_count` | `4` |

Selected optional service-stack adapters:

- `memory-service`
- `retrieval-gateway`
- `rag-service`
- `summary-service`
- `agent-action-executor`

Adapter summary:

| Adapter | Cases | Passed | Failed | Skipped |
| --- | ---: | ---: | ---: | ---: |
| `profile-agent-safety` | 20 | 20 | 0 | 0 |
| `action-external-http-provider` | 2 | 2 | 0 | 0 |
| `action-external-mcp-failure` | 4 | 4 | 0 | 0 |
| `memory-service` | 2 | 2 | 0 | 0 |
| `retrieval-gateway` | 5 | 1 | 0 | 4 |
| `rag-service` | 6 | 6 | 0 | 0 |
| `summary-service` | 4 | 4 | 0 | 0 |
| `agent-action-executor` | 8 | 8 | 0 | 0 |

## Confirmed Boundaries

- `memory-service` live adapter proved projection, source refs, `GetMemoryEvent`,
  visibility, graph edge, validity window, supersession, reviewed multi-source
  profile activation, and deleted supporting-memory exclusion.
- `retrieval-gateway` positive live smoke proved EvidencePack source refs,
  speaker attribution, current-memory temporal filtering, stale / future memory
  exclusion, and projection versions.
- `rag-service`, `summary-service`, and `agent-service` consumed the live
  EvidencePack path while preserving citation/source-chain and proposal-only
  boundaries.
- `action-executor` retained hash-only / low-sensitive external tool result
  boundaries for the required action adapters.

## Skipped Scope

The 4 skipped cases are retrieval-gateway negative / miss cases. They require a
dedicated negative fixture, such as no-memory source coverage, cross-tenant deny,
or explicit source-ref exclusion. They are not counted as covered by the positive
EvidencePack live smoke.

## Fixes Made During Run

- `loadtest/memory` now uses explicit PostgreSQL parameter typing for JSON seed
  values and contiguous parameter numbering for profile aggregate seed SQL.
- `ai-eval-record-smoke` now paginates `ListEvalRuns` readback instead of
  assuming the newly recorded run appears on the first page of a long suite.
- `run-ai-eval-retrieval-adapter.ps1` now marks retrieval negative / miss cases
  as skipped for the positive live smoke instead of failing on unsupported case
  assertions or pretending those negative cases passed.

## Next Step

Add a dedicated retrieval negative / miss adapter to cover the skipped cases,
then continue the group-memory / EvidencePack / RAG-Agent demo module.
