# Seeded Capacity Baseline Smoke

## Scope

This report records a local seeded capacity-baseline smoke for three runners
that need pre-existing business state:

- `message-service / loadtest/sendmessage`
- `conversation-service / loadtest/memberchange`
- `delivery-service / loadtest/delivery`

It is a development/interview evidence point only. It is not a production SLO,
production sizing claim, HA proof, or complete end-to-end relay proof.

## Environment

- Run name: `capacity-baseline-seeded-20260616`
- Raw result root:
  `H:\NexusIM\loadtest-results\capacity-baseline-seeded-20260616`
- Fixture command:

```powershell
. .\tools\go-env.ps1
go run ./loadtest/capacityseed `
  --pg-dsn "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  --cleanup
```

- Suite command:

```powershell
.\tools\run-loadtest-capacity-baseline-suite.ps1 `
  -RunName capacity-baseline-seeded-20260616 `
  -PGDSN "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -KafkaBrokers localhost:9092 `
  -Services message-service,conversation-service,delivery-service `
  -IncludeSeededRunners `
  -VUs 2 `
  -Duration 5s `
  -ConversationCount 10
```

## Result

- Suite status: `passed`
- Runnable services: `3/3`
- Skipped services: none
- Summary files with `capacity_summary`: `3`

Capacity summary:

```text
message-service:
  runner = sendmessage
  duration_seconds = 5.016
  success_count = 408
  error_count = 0
  accepted_rps = 81.34171402861067
  p95_ms = 38.3728
  p99_ms = 79.9451

conversation-service:
  runner = memberchange
  duration_seconds = 5.0198086
  success_count = 214
  error_count = 0
  requests_per_second = 42.8
  p95_ms = 96.325
  p99_ms = 198.738

delivery-service:
  runner = delivery
  duration_ms = 95.333
  item_count = 1
  expected_count = 1
  items_per_second = 10.489536163200398
  ack_latency_ms = 29.164
```

## Notes

This run intentionally validates seeded direct runner prerequisites. It does
not start the local worker overlay, so message / conversation outbox rows remain
`PENDING` after the direct writes. Relay / consumer stack behavior is tracked by
separate stack runner smoke work.

The fixture tool only writes local capacity tenants:

```text
tenant-capacity-message
tenant-capacity-conversation
tenant-capacity-delivery
```

Do not use it against production databases.
