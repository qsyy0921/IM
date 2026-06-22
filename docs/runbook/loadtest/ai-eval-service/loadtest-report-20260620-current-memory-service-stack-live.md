# Current-Memory Service-Stack Live Smoke 2026-06-20

Scope: low-sensitive local service-stack smoke for current memory consumption.
This is not a production benchmark, not a capacity result, and not an LLM quality
claim.

## Result

Passed.

The run verified that real local `rag-service`, `summary-service`, and
`agent-service` consume current memory through `retrieval-gateway` / EvidencePack
and exclude expired or superseded memory decoys at `at_conversation_seq=7`.

Raw summaries were written under:

```text
H:\NexusIM\loadtest-results\ai-eval-current-memory-service-stack-20260620
```

Repository files only keep this low-sensitive report.

## Runtime

The local stack already had these endpoints listening:

- `search-service`: `127.0.0.1:10570`
- `memory-service`: `127.0.0.1:10580`
- `retrieval-gateway`: `127.0.0.1:10590`
- `rag-service`: `127.0.0.1:10610`
- `agent-service`: `127.0.0.1:10630`
- `skill-registry`: `127.0.0.1:10640`
- `mcp-gateway`: `127.0.0.1:10650`
- `action-executor`: `127.0.0.1:10660`
- `policy-service`: `127.0.0.1:10800`
- PostgreSQL: `127.0.0.1:5432`

After reboot, `summary-service` was the only missing dependency. It was started
locally in `grpc` mode with:

```text
NEXUSIM_SUMMARY_SERVICE_MODE=grpc
NEXUSIM_SUMMARY_GRPC_ADDR=127.0.0.1:10620
NEXUSIM_RETRIEVAL_GRPC_ADDR=127.0.0.1:10590
NEXUSIM_SUMMARY_PROVIDER_MODE=extractive
```

## Commands

Preflight:

```powershell
. .\tools\go-env.ps1
.\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -PreflightOnly `
  -OptionalAdapter "rag-service,summary-service,agent-action-executor" `
  -RunName "ai-eval-current-memory-service-stack-20260620"
```

Live gate:

```powershell
. .\tools\go-env.ps1
.\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter "rag-service,summary-service,agent-action-executor" `
  -RunName "ai-eval-current-memory-service-stack-20260620" `
  -RequestTimeout "30s"
```

## Gate Summary

`ai-eval-regression-gate-summary.json`:

```text
status: passed
adapter_count: 6
case_count: 38
passed_count: 38
failed_count: 0
skipped_count: 0
selected_optional_adapters:
  - rag-service
  - summary-service
  - agent-action-executor
```

Adapter counts:

| Adapter | Cases | Passed |
| --- | ---: | ---: |
| profile-agent-safety | 14 | 14 |
| action-external-http-provider | 2 | 2 |
| action-external-mcp-recovery | 4 | 4 |
| rag-service | 6 | 6 |
| summary-service | 4 | 4 |
| agent-action-executor | 8 | 8 |

## Current-Memory Assertions

All three live service adapters passed the stale-memory exclusion assertion:

| Case | Status | Key assertion |
| --- | --- | --- |
| `rag-service-current-memory-live-excludes-stale` | passed | `must_exclude_expired_superseded_memory_items` |
| `summary-service-current-memory-live-excludes-stale` | passed | `must_exclude_expired_superseded_memory_items` |
| `agent-service-current-memory-live-excludes-stale` | passed | `must_exclude_expired_superseded_memory_items` |

The underlying service summaries recorded:

| Adapter | `current_memory_at_seq` | `expired_memory_excluded` | `superseded_memory_excluded` | Citations |
| --- | ---: | --- | --- | ---: |
| RAG | 7 | true | true | 2 |
| Summary | 7 | true | true | 2 |
| Agent | 7 | true | true | 2 |

The verified low-sensitive statements were:

- RAG answer carries source citation back to message source refs.
- Summary carries source citation back to message source refs.
- Agent proposal carries citation back to message source refs.
- Search and memory evidence are preserved in returned EvidencePack.
- Expired and superseded memory decoys are excluded from current EvidencePack.
- Projection versions and source coverage are preserved.
- Agent remains proposal-only before approved execution.

## Boundaries

This smoke proves the current local service-stack path for:

```text
search / memory -> retrieval-gateway EvidencePack
-> RAG / Summary / Agent current-memory consumption
-> ai-eval low-sensitive gate recording
```

It does not prove:

- production HA or long-running stability;
- LLM quality;
- capacity limits;
- full group-memory extraction quality;
- cross-group / temporal multi-hop reasoning beyond the seeded current-memory
  exclusion scenario.

Those remain future AI eval / memory hardening work.
