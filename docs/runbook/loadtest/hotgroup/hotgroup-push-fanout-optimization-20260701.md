# Hotgroup Push Fanout Optimization - 2026-07-01

## Scope

This note records the first code-level optimization after the 400 subscriber and
multi-runner READ_FANOUT diagnostics. It is not a capacity result and must be
followed by a clean Docker redeploy and comparable retest.

## Evidence Before Change

- Baseline single-runner 400 subscriber run:
  `hotgroup-readfanout-6000-8000qps-400sub-233d6956-20260701-004948`.
- Multi-runner comparison:
  `hotgroup-multirunner-400sub-coordinator-20260701-013557` plus 4
  `subscriber-only` shards.
- Both runs showed message outbox, delivery outbox, delivery projection and
  Kafka consumer lag catching up.
- Push writer, Redis subscriber enqueue and WebSocket write errors were not the
  blocker; no slow session eviction was observed.
- Total signal drain stayed around 2.84k-2.85k signals/s, so the next target is
  push-gateway online signal drain.

## Bottleneck Hypothesis

The in-memory push registry used one global mutex while iterating matched user
sessions or conversation subscribers, deduplicating the event, appending resume
buffer state and sending to each session outbound channel.

For hotspot conversation signal fanout this means every signal holds the registry
lock across the whole subscriber loop. With hundreds of subscribers, this can
serialize fanout bookkeeping and block unrelated session operations longer than
necessary.

## Change

`services/push-gateway/internal/infrastructure/memory/registry.go` now splits
local fanout into two phases:

1. Lock phase:
   - prune expired resume state;
   - snapshot eligible sessions;
   - mark event id as seen;
   - append resume buffer state.
2. Lock-free write phase:
   - non-blockingly write the prepared frame to each target outbound queue;
   - if a target queue is full, re-lock and evict only if the same session is
     still registered.

This keeps the existing fail-closed slow-session behavior and durable PullInbox
fallback, but reduces global registry lock hold time on the hot path.

## Verification

Focused checks:

```powershell
. .\tools\go-env.ps1; go test ./services/push-gateway/... -count=1
. .\tools\go-env.ps1; go build ./services/push-gateway/cmd/push-gateway ./loadtest/hotgroup
git diff --check
```

All checks passed before this note was written.

## Required Retest

After committing, rebuild and redeploy the latest push-gateway image, then rerun
a comparable online-drain test:

```text
group_size=6000
message_count=1000 or 5000
message_rate=8000
sender_count=256
subscriber_count=400
fanout_mode=READ_FANOUT
runner layout=coordinator + 4 subscriber-only shards
```

The report must compare:

- total conversation signals;
- slowest subscriber drain time;
- aggregate drain rate;
- push writer success/error;
- Redis subscriber enqueue/error;
- slow eviction;
- SendMessage p95/p99;
- PullInbox/ACK p95/p99;
- message_outbox and delivery_outbox pending;
- Kafka lag.

## Expected Outcome

If registry lock hold time was a material bottleneck, the aggregate signal drain
rate should move above the previous 2.84k-2.85k signals/s band. If the drain rate
does not improve, the next module should shift to WebSocket writer flush cadence,
per-connection write scheduling or network throughput rather than message /
delivery outbox or Kafka.
