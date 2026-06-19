# NexusIM action-executor external MCP fallback eval

Date: 2026-06-20

Scope: first-stage, low-sensitive local eval adapter for action-executor
external MCP fallback. This is not a production benchmark and does not call a
real MCP server, external network, database or business tool.

## Result

Passed.

Command:

```powershell
.\tools\run-ai-eval-action-mcp-fallback-adapter.ps1 `
  -RunName action-mcp-fallback-eval-20260620 `
  -OutputPath H:\NexusIM\loadtest-results\action-mcp-fallback-eval-20260620\action-external-mcp-fallback-eval-summary.json
```

Raw summary:

```text
H:\NexusIM\loadtest-results\action-mcp-fallback-eval-20260620\action-external-mcp-fallback-eval-summary.json
```

## Covered Cases

The adapter validates four active `action-executor-mcp-fallback` cases from
`docs/runbook/ai-eval/retrieval-eval-cases.json`:

| Case | Classification | Executed | Output hash |
| --- | --- | --- | --- |
| `external-mcp-provider-unavailable` | `TOOL_PROVIDER_UNAVAILABLE` | false | none |
| `external-mcp-provider-timeout` | `TOOL_EXECUTION_TIMEOUT` | false | none |
| `external-mcp-provider-rate-limited` | `TOOL_PROVIDER_RATE_LIMITED` | false | none |
| `external-mcp-provider-permission-denied` | `TOOL_PROVIDER_PERMISSION_DENIED` | false | none |

All cases also verify:

- no raw tool input is sent;
- no raw provider body is persisted;
- tool result projection records `FAILED`;
- action-executor keeps provider failure separate from proposal approval state.

## Validation

```powershell
go run ./services/action-executor/cmd/action-external-mcp-fallback-smoke
go test ./services/action-executor/internal/infrastructure/tool -count=1
.\tools\run-ai-eval-action-mcp-fallback-adapter.ps1 -RunName action-mcp-fallback-eval-20260620
.\tools\validate-ai-eval-cases.ps1
.\tools\validate-ai-eval-gate-policy.ps1
```

Observed case catalog after this slice:

```text
case_count = 38
action_execution_safety = 9
```

The CI-safe local gate policy now includes three required adapters:

```text
profile-agent-safety
action-external-http-provider
action-external-mcp-fallback
```

## Boundary

This proves stable local fallback classifications and hash-only / raw-output
safety for first-stage eval. It does not prove real external MCP connectivity,
real provider reliability, retries, DLQ, or production incident recovery.
