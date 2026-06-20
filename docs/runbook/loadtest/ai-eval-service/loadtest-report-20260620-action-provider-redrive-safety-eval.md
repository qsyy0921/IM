# NexusIM action provider failure worker / redrive safety eval

Date: 2026-06-20

Scope: first-stage local verification for `action-executor` provider failure
bookkeeping and generic redrive safety. This is not a production DLQ / redrive
workflow, provider replay test, HA test or capacity benchmark.

## Verified

- `action_executor_provider_failures` due `RETRY_PENDING` rows can be processed
  by repository / worker logic:
  - due retryable row is rescheduled with incremented `retry_count`
  - row reaching max attempts is moved to `DLQ`
  - future `next_retry_at` row is not touched
- `NEXUSIM_ACTION_EXECUTOR_MODE=provider-failure-worker` exists as an explicit
  worker mode; default mode remains `noop`.
- Generic provider-failure redrive action is blocked by preflight guard:
  `ACTION_REPAIR_REQUIRES_OPERATOR`, `executed=false`, no output hash.
- Eval catalog now has 11 active `action-executor-preflight-safety` cases.

## Commands

```powershell
. .\tools\go-env.ps1
go test ./services/action-executor/... -count=1
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
go test ./services/action-executor/internal/infrastructure/postgres -count=1 -v
go run ./services/action-executor/cmd/action-preflight-safety-smoke
.\tools\run-ai-eval-action-preflight-safety-adapter.ps1 -RunName action-provider-redrive-safety-20260620 -OutputPath H:\NexusIM\loadtest-results\action-provider-redrive-safety-20260620\action-preflight-safety-eval-summary.json
```

## Limits

- Worker does not replay provider calls and does not execute tools.
- No redrive API, operator UI, provider-grade DLQ repair or production metrics.
- Raw output remains outside the repository:
  `H:\NexusIM\loadtest-results\action-provider-redrive-safety-20260620\action-preflight-safety-eval-summary.json`.
