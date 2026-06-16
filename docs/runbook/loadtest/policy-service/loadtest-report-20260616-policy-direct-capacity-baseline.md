# policy-service Direct Capacity Baseline Smoke

## Scope

This report records a local direct capacity-baseline smoke for `policy-service`.
It is a development/interview evidence point only, not a production SLO, sizing
claim, or HA result.

## Environment

- Run name: `capacity-baseline-direct-20260616-v3`
- Raw result root: `H:\NexusIM\loadtest-results\capacity-baseline-direct-20260616-v3`
- Suite command:

```powershell
.\tools\run-loadtest-capacity-baseline-suite.ps1 `
  -RunName capacity-baseline-direct-20260616-v3 `
  -PGDSN postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable `
  -KafkaBrokers localhost:9092 `
  -VUs 2 `
  -Duration 5s `
  -ConversationCount 5
```

## Result

- Suite status: `passed`
- Runnable services: `1/9`
- Executed direct runner: `policy-service / loadtest/policy`
- Skipped stack runners: `api-gateway`, `identity-service`, `push-gateway`, `receipt-service`, `contacts-service`
- Skipped seeded runners: `message-service`, `conversation-service`, `delivery-service`

Policy summary:

```text
duration_seconds = 0.0978111
action_count = 4
allowed_action_count = 4
decisions_per_second = 40.895154026485748
latency_p95_ms = 6.202
latency_p99_ms = 6.202
```

## Notes

The suite now fails closed after a runner exits if the generated
`capacity_summary` has `success=false`, zero successful operations, or no
positive throughput signal. This prevents all-error loadtest output from being
reported as a passing capacity baseline.

The other eight service runners still need either seeded business state
(`message-service`, `conversation-service`, `delivery-service`) or extra runtime
roles / fixtures (`api-gateway`, `identity-service`, `push-gateway`,
`receipt-service`, `contacts-service`) before they can produce meaningful
capacity baselines.
