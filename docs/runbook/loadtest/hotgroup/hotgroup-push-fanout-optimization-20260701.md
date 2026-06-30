# Hotgroup Push Fanout Optimization - 2026-07-01

## Scope

This note records the first and second code-level optimizations after the 400
subscriber and multi-runner READ_FANOUT diagnostics. It is not a capacity result.

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

## Retest Result

The first optimization was committed as `4bc4a30`, rebuilt, archived and
redeployed to Ubuntu Docker. A comparable online-drain test was then run:

```text
group_size=6000
message_count=1000
message_rate=8000
sender_count=256
subscriber_count=400
fanout_mode=READ_FANOUT
runner layout=coordinator + 4 subscriber-only shards
```

Artifacts:

- coordinator: `H:\NexusIM\loadtest-results\hotgroup-pushfanout-clean-400sub-coordinator-20260701-022043`
- shards: `H:\NexusIM\loadtest-results\hotgroup-pushfanout-clean-400sub-shard*-20260701-022043`
- analysis: `hotgroup-multirunner-analysis-20260701-pushfanout-400sub.md`
- metrics window: `hotgroup-metrics-window-20260701-pushfanout-clean-400sub.md`

Result:

- coordinator send, PullInbox and ACK succeeded;
- `message_outbox_pending=0`, `delivery_outbox_pending=0`;
- 4 shards read 400000 conversation signals with no subscriber error;
- aggregate signal span rate was about `2891.8 signals/s`;
- previous single-runner 400 subscriber baseline was about `2839.888 signals/s`;
- improvement was only about `1.8%`.

Judgment: registry lock hold time is not the dominant bottleneck. The bottleneck
remains online signal drain.

## Second Change

The next low-risk hot-path optimization is to avoid repeated JSON marshaling for
the same delivery / conversation notify frame.

`types.ServerFrame` now has a transport-only `EncodedPayload` field excluded from
JSON. The memory registry marshals a delivery notify once after it has matched
at least one local session, then fanouts the same pre-encoded payload to all
session queues. The WebSocket writer uses the cached payload when present and
falls back to normal JSON encoding only for frames that were not pre-encoded
such as server hello, pong, ACK OK, errors and resume hints.

This does not change protocol fields, durable inbox semantics, PullInbox / ACK
recovery, slow-session eviction or Redis route behavior.

Focused checks:

```powershell
. .\tools\go-env.ps1; go test ./services/push-gateway/... -count=1
. .\tools\go-env.ps1; go build ./services/push-gateway/cmd/push-gateway ./loadtest/hotgroup
git diff --check; git diff --cached --check
```

All checks passed before this section was written.

## Required Next Retest

After committing, rebuild and redeploy the latest push-gateway image, then rerun
the same 400 subscriber coordinator + 4 shard scenario. If pre-encoding does not
move drain rate materially above the current `2.85k-2.89k signals/s` band, the
next module should shift to WebSocket writer flush cadence, per-connection write
scheduling, nhooyr write behavior or network throughput rather than message /
delivery outbox or Kafka.
