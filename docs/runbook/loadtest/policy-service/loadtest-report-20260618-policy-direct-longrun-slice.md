# policy-service Direct Long-Run Capacity Slice

## Scope

This report records one local direct `policy-service` capacity slice from the planned NexusIM long-run campaign. It is not a production SLO, production sizing result, or a completed nine-service campaign.

## Environment

- Commit: `0f650e71`
- Git dirty: `false`
- Service: `policy-service`
- Runner: `loadtest/policy`
- Target: `127.0.0.1:10800`
- Raw result directory: `H:\NexusIM\loadtest-results\capacity-longrun-nine-services-20260618-plan\policy-service`
- Summary path: `H:\NexusIM\loadtest-results\capacity-longrun-nine-services-20260618-plan\policy-service\policy-summary.json`

## Command Shape

```text
go run ./loadtest/policy --target 127.0.0.1:10800 --result-dir H:\NexusIM\loadtest-results\capacity-longrun-nine-services-20260618-plan\policy-service --vus 4 --duration 30m
```

The command was launched through:

```text
tools/invoke-capacity-longrun-campaign.ps1 -Services policy-service -SkipSummary
```

## Result

| Metric | Value |
| --- | ---: |
| Success | true |
| Requested duration | 1800s |
| Actual duration | 1800.148s |
| Virtual users | 4 |
| Action count | 4,317,626 |
| Allowed action count | 4,317,626 |
| Denied action count | 0 |
| Decisions / second | 2398.484 |
| Latency p95 | 4.444 ms |
| Latency p99 | 13.905 ms |

## Boundary

This proves the direct `policy-service` capacity runner can execute a clean 30-minute slice and produce bounded local capacity evidence. It does not prove production capacity, HA behavior, cross-service policy integration capacity, or the full nine-service campaign.
