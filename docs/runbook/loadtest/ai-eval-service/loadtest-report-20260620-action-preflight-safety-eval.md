# NexusIM AI Eval Action Preflight Safety Regression

Date: 2026-06-20

Scope: first-stage local regression for `action-executor` preflight safety. This is not a production benchmark, HA test or live provider test.

## What This Proves

- Policy-denied actions are blocked before any tool adapter executes.
- Disabled skills and tool mismatch are blocked before provider execution.
- Elevated-risk local-safe tool requests stay audit-only and do not record output hashes.
- Unapproved proposals fail before execution audit / result projection state is created.
- Rate-limited actions are blocked or fail closed before tool execution.
- DLQ / repair actions require the operator workflow and cannot use generic tool execution.
- The eval adapter keeps summaries low-sensitive and writes raw run artifacts under `H:\NexusIM\loadtest-results`.

## Commands

```powershell
. .\tools\go-env.ps1
go run ./services/action-executor/cmd/action-preflight-safety-smoke
go test ./services/action-executor/cmd/action-preflight-safety-smoke ./services/action-executor/internal/app ./services/action-executor/internal/infrastructure/tool -count=1
.\tools\run-ai-eval-action-preflight-safety-adapter.ps1 -RunName action-preflight-repair-20260620 -OutputPath H:\NexusIM\loadtest-results\action-preflight-repair-20260620\action-preflight-safety-eval-summary.json
.\tools\validate-ai-eval-cases.ps1
.\tools\validate-ai-eval-gate-policy.ps1
```

## Evidence

- Case catalog now contains 52 active/draft low-sensitive cases.
- `action_execution_safety` family now has 19 cases.
- Raw summary: `H:\NexusIM\loadtest-results\action-preflight-repair-20260620\action-preflight-safety-eval-summary.json`.
- Smoke summary inside the raw result contains 10 passing preflight cases.

## Limits

- Uses in-memory ports and local tool fixture only.
- Does not execute live business tools, external providers, PostgreSQL migrations, Docker, or full service-stack smoke.
- Does not prove production rate limits, provider retry / DLQ, or provider-grade tool governance.
