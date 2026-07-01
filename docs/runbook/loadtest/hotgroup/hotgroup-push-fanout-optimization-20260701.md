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

## Second Retest Result

The second optimization was committed as `d8d78fd`, rebuilt, archived and
redeployed to Ubuntu Docker. The same 400 subscriber coordinator + 4 shard
scenario was then rerun:

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

- coordinator:
  `H:\NexusIM\loadtest-results\hotgroup-pushpreenc-clean-400sub-coordinator-20260701-024044`
- shards:
  `H:\NexusIM\loadtest-results\hotgroup-pushpreenc-clean-400sub-shard*-20260701-024044`
- analysis:
  `hotgroup-multirunner-analysis-20260701-pushpreenc-400sub.md`
- metrics window:
  `hotgroup-metrics-window-20260701-pushpreenc-clean-400sub.md`

Result:

- coordinator send, PullInbox and ACK succeeded;
- `message_outbox_pending=0`, `delivery_outbox_pending=0`;
- 4 shards read 400000 conversation signals with no subscriber error;
- aggregate signal span rate was about `2863.092 signals/s`;
- previous single-runner 400 subscriber baseline was about `2839.888 signals/s`;
- the registry lock optimization retest was about `2891.8 signals/s`.

Judgment: pre-encoding the WebSocket payload did not materially move the drain
rate. Repeated JSON marshaling is not the dominant bottleneck. The next module
should shift to WebSocket writer flush cadence, per-connection write scheduling,
nhooyr write behavior, connection-side read backpressure or network throughput
rather than message / delivery outbox or Kafka.

## Writer Duration Retest

The next diagnostic change was committed as `4f45519`, rebuilt, archived and
redeployed to Ubuntu Docker. It added low-cardinality WebSocket writer duration
histograms for `frame_write` and `delivery_notify`, without changing protocol,
fanout, durable inbox, PullInbox or ACK behavior.

Comparable scenario:

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

- coordinator:
  `H:\NexusIM\loadtest-results\hotgroup-writerdur-clean-400sub-coordinator-20260701-031058`
- shards:
  `H:\NexusIM\loadtest-results\hotgroup-writerdur-clean-400sub-shard*-20260701-031058`
- analysis:
  `hotgroup-multirunner-analysis-20260701-writerdur-400sub.md`
- metrics window:
  `hotgroup-metrics-window-20260701-writerdur-clean-400sub.md`

Result:

- coordinator send, PullInbox and ACK succeeded;
- `message_outbox_pending=0`, `delivery_outbox_pending=0`;
- 4 shards read 400000 conversation signals with no subscriber error;
- aggregate signal span rate was about `2876.698 signals/s`;
- previous single-runner 400 subscriber baseline was about `2839.888 signals/s`;
- `delivery_notify` write p95 / p99 were about `0.345ms / 0.499ms`;
- `delivery_notify` write avg was about `0.125ms`, max about `10.056ms`;
- writer error, Redis subscriber error and slow eviction stayed at 0.

Judgment: single-call WebSocket `conn.Write` latency is not the dominant
bottleneck. The next module should instrument or optimize Redis subscriber
local fanout / enqueue duration and per-session writer scheduling. The key
question is now where the time is spent between one Redis subscriber message
and 400 session queues / writers draining it, not whether one `conn.Write` call
has a large long tail.

## Redis Subscriber Fanout Duration Instrumentation

The next diagnostic change adds low-cardinality Redis subscriber local fanout
duration metrics. It records the time spent inside the local enqueue call after
the Redis subscriber has parsed a remote route message:

```text
delivery_notify      -> LocalRegistry.EnqueueNotification
conversation_signal  -> LocalRegistry.EnqueueConversationSignal
```

The metrics are exported as:

```text
nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds
nexusim_push_gateway_redis_subscriber_fanout_duration_max_milliseconds
```

Only the operation label is used; tenant, user, conversation, session and event
identifiers are intentionally excluded. The hotgroup Prometheus window script
now records conversation-signal fanout p95 / p99 / avg / max.

This is observation only. It does not change Redis route semantics, WebSocket
frame payloads, durable inbox, PullInbox, ACK, slow-session handling or resume
behavior. The next comparable run should redeploy this commit and rerun the same
400 subscriber coordinator + shard scenario, then compare Redis enqueue
duration, WebSocket writer duration and runner signal drain span in one report.

## Redis Subscriber Fanout Duration Retest

The Redis subscriber fanout duration instrumentation was committed as `6099ecd`,
rebuilt, archived and redeployed to Ubuntu Docker. The same 400 subscriber
coordinator + 4 shard scenario was rerun:

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

- coordinator:
  `H:\NexusIM\loadtest-results\hotgroup-redisfanout-clean-400sub-coordinator-20260701-033606`
- shards:
  `H:\NexusIM\loadtest-results\hotgroup-redisfanout-clean-400sub-shard*-20260701-033606`
- analysis:
  `hotgroup-multirunner-analysis-20260701-redisfanout-400sub.md`
- metrics window:
  `hotgroup-metrics-window-20260701-redisfanout-clean-400sub.md`

Result:

- coordinator send, PullInbox and ACK succeeded;
- `message_outbox_pending=0`, `delivery_outbox_pending=0`;
- 4 shards read 400000 conversation signals with no subscriber error;
- aggregate signal span rate was about `2883.976 signals/s`;
- previous single-runner 400 subscriber baseline was about `2839.888 signals/s`;
- WebSocket `delivery_notify` write p95 / p99 were about `0.406ms / 0.63ms`;
- Redis subscriber conversation signal fanout/enqueue whole-window p95 / p99
  were about `56.14ms / 91.228ms`;
- Redis subscriber conversation signal fanout/enqueue five-minute last p95 /
  p99 were about `60.263ms / 92.053ms`;
- Redis subscriber fanout/enqueue average was about `16.485ms`, max about
  `84.305ms`;
- writer error, Redis subscriber error and slow eviction stayed at 0.

Judgment: the bottleneck is now narrowed to Redis subscriber local
fanout/enqueue scheduling for conversation signals. The subscriber receives one
conversation signal, then spends tens of milliseconds fanning it out to the 400
local WebSocket sessions, while the individual WebSocket write remains
sub-millisecond at p99. The next module should introduce a controlled
conversation fanout worker / shard queue so Redis subscriber goroutines can
handoff quickly and worker-side queue depth, drain latency and backpressure can
be measured directly.

## Redis Subscriber Conversation Signal Worker Queue

The next implementation step adds a dedicated worker / shard queue for Redis
subscriber conversation signals:

- `delivery_notify` continues to run synchronously through the existing local
  registry path.
- `conversation_signal` is enqueued into a bounded dispatcher queue.
- Jobs are sharded by `tenant_id + conversation_id`, so signals for the same
  conversation stay on the same worker and preserve local order.
- Redis subscriber handling now returns after enqueue instead of spending the
  fanout time in the Pub/Sub receive path.
- Queue-full is explicit backpressure: it increments queue-full / error metrics
  and does not pretend the signal was delivered. Durable recovery remains
  `PullInbox`; there is no hidden fallback.

New runtime knobs:

```text
NEXUSIM_PUSH_REDIS_SUBSCRIBER_SIGNAL_WORKERS=4
NEXUSIM_PUSH_REDIS_SUBSCRIBER_SIGNAL_QUEUE_SIZE=4096
```

New metrics:

```text
nexusim_push_gateway_redis_route_events_total{event="subscriber_signal_fanout_queued",role="subscriber"}
nexusim_push_gateway_redis_route_events_total{event="subscriber_signal_fanout_queue_full",role="subscriber"}
nexusim_push_gateway_redis_route_events_total{event="subscriber_signal_fanout_worker_error",role="subscriber"}
nexusim_push_gateway_redis_subscriber_signal_fanout_queue_depth
nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds
nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_max_milliseconds
```

The Prometheus window recorder now captures queued / queue-full / worker-error,
queue depth and queue-wait p95 / p99 / avg / max. The next comparable retest
should rebuild a clean push-gateway image and rerun the same 400 subscriber
coordinator + 4 shard scenario. The expected interpretation is:

- if queue wait stays low and drain rate improves, Redis subscriber handoff was
  the bottleneck;
- if queue depth / wait grows, worker drain is still the bottleneck and needs
  worker count or per-session scheduling analysis;
- if queue full appears, the system is applying explicit online-notify
  backpressure and the run must be treated as lossy online wakeup, not as a
  successful notify-drain capacity result.

Focused checks run before this note:

```powershell
. .\tools\go-env.ps1; go test ./services/push-gateway/internal/infrastructure/redisroute ./services/push-gateway/internal/infrastructure/monitoring ./services/push-gateway/cmd/push-gateway -count=1
. .\tools\go-env.ps1; go test ./services/push-gateway/... -count=1
. .\tools\go-env.ps1; go build ./services/push-gateway/cmd/push-gateway ./loadtest/hotgroup
```

## Redis Subscriber Conversation Signal Worker Queue Retest

The worker / shard queue implementation was committed as `93654117`, rebuilt,
archived and redeployed to Ubuntu Docker. The image archive is:

```text
H:\NexusIM\docker-images\archives\nexusim-push-gateway-93654117-20260701-041255.tar
```

Comparable scenario:

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

- coordinator:
  `H:\NexusIM\loadtest-results\hotgroup-signalqueue-clean-400sub-coordinator-20260701-041641`
- shards:
  `H:\NexusIM\loadtest-results\hotgroup-signalqueue-clean-400sub-shard*-20260701-041641`
- analysis:
  `hotgroup-multirunner-analysis-20260701-signalqueue-400sub.md`
- metrics window:
  `hotgroup-metrics-window-20260701-signalqueue-400sub.md`

Result:

- coordinator send, PullInbox and ACK succeeded;
- `message_outbox_pending=0`, `delivery_outbox_pending=0`;
- 4 shards read 400000 conversation signals with no subscriber error;
- aggregate signal span rate was about `2876.076 signals/s`;
- previous single-runner 400 subscriber baseline was about `2839.888 signals/s`;
- queue-full and worker-error were both 0;
- queue depth stayed at 0 in the captured Prometheus window;
- queue wait p95 / p99 were about `0.095ms / 0.099ms`;
- worker-side conversation signal fanout p95 / p99 remained about
  `38.636ms / 87.5ms`;
- WebSocket `delivery_notify` write p95 / p99 were about `0.349ms / 0.495ms`.

Judgment: Redis subscriber handoff is now cheap and not the dominant bottleneck.
The end-to-end signal drain rate did not materially move, so the next module
should stop optimizing the Pub/Sub receive path and instead inspect worker-side
local fanout, per-session outbound queue drain, WebSocket writer goroutine
scheduling, flush / batching behavior, runner-side read backpressure and
network throughput.

## WebSocket Writer Queue Latency and Batch Drain

The next diagnostic / optimization module targets the gap between local fanout
enqueue and per-connection writer drain.

Changes:

- `ServerFrame` now carries a transport-only `EnqueuedAtMS` field. It is excluded
  from JSON and does not change the WebSocket protocol.
- Memory registry and WebSocket app-response enqueue paths stamp outbound
  frames when they enter a session queue.
- WebSocket writer metrics now record queue duration histograms for all frames
  and for `delivery.notify` / `delivery.hide` frames:

```text
nexusim_push_gateway_ws_writer_queue_duration_milliseconds
nexusim_push_gateway_ws_writer_queue_duration_max_milliseconds
```

- The writer loop now drains up to `NEXUSIM_PUSH_WS_WRITER_BATCH_SIZE` already
  queued frames per wakeup, defaulting to 16. This preserves per-connection
  order and does not change ACK, PullInbox, durable inbox, Redis route or
  slow-session behavior.
- `tools/record-hotgroup-metrics-window.ps1` now records delivery notify queue
  p95 / p99 / avg / max for both five-minute and whole-window views.

Focused checks:

```powershell
. .\tools\go-env.ps1; go test ./services/push-gateway/internal/types ./services/push-gateway/internal/api/websocket ./services/push-gateway/internal/infrastructure/memory ./services/push-gateway/internal/infrastructure/monitoring ./services/push-gateway/cmd/push-gateway -count=1
. .\tools\go-env.ps1; go test ./services/push-gateway/... ./loadtest/hotgroup -count=1; go build ./services/push-gateway/cmd/push-gateway ./loadtest/hotgroup
```

Judgment before retest: this module is not yet a capacity result. It makes the
next 400 subscriber coordinator + shard run able to answer whether the
remaining online signal drain curve is caused by outbound queue wait, writer
scheduling / select overhead, worker-side local fanout, runner read backpressure
or network throughput.

## WebSocket Writer Queue Latency Retest

The writer queue latency / batch drain commit `fedb5f43` was rebuilt, archived
and redeployed to Ubuntu Docker. The image archive is:

```text
H:\NexusIM\docker-images\archives\nexusim-push-gateway-fedb5f43-20260701-044622.tar
```

Comparable scenario:

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

- coordinator:
  `H:\NexusIM\loadtest-results\hotgroup-writerqueue-clean-400sub-coordinator-20260701-045022`
- shards:
  `H:\NexusIM\loadtest-results\hotgroup-writerqueue-clean-400sub-shard*-20260701-045022`
- analysis:
  `hotgroup-multirunner-analysis-20260701-writerqueue-400sub.md`
- metrics window:
  `hotgroup-metrics-window-20260701-writerqueue-clean-400sub.md`

Result:

- coordinator send, PullInbox and ACK succeeded;
- `message_outbox_pending=0`, `delivery_outbox_pending=0`;
- 4 shards read 400000 conversation signals with no subscriber error;
- aggregate signal span rate was about `2884.066 signals/s`;
- previous signal queue run was about `2876.076 signals/s`, so the change is
  not a material throughput improvement;
- WebSocket `delivery_notify` queue p95 / p99 were about `4.665ms / 4.942ms`;
- WebSocket `delivery_notify` write p95 / p99 were about `0.383ms / 0.587ms`;
- Redis subscriber signal queue wait p95 / p99 stayed about
  `0.095ms / 0.099ms`;
- worker-side conversation signal fanout p95 / p99 remained about
  `57.759ms / 92.241ms`;
- writer / Redis subscriber error, queue full and eviction were 0.

Judgment: per-session writer queue wait and single `conn.Write` duration are
not the dominant bottleneck. The remaining bottleneck is still the local fanout
work that maps one conversation signal to about 400 session enqueues. Because
the current conversation signal worker preserves order by serializing a whole
conversation through one worker, the next optimization should evaluate
conversation-local fanout buckets: split the subscriber set into stable buckets
and let each bucket drain in order for its sessions, while preserving per-session
signal order and keeping durable PullInbox as the recovery path.

## Conversation-Local Fanout Buckets

The next implementation step keeps Redis Pub/Sub, `delivery.notify` payloads,
durable inbox, PullInbox and ACK semantics unchanged. It only changes how a
single push-gateway process maps one conversation signal to local WebSocket
session queues.

Change:

- `memory.Registry` now accepts `ConversationFanoutBuckets`.
- `EnqueueConversationSignal` still snapshots eligible subscribers and marks
  event ids under the registry lock.
- After snapshot, local outbound enqueue is split by stable `session_id` bucket.
- Buckets run in parallel for the same conversation signal.
- The outer Redis subscriber worker still processes a conversation signal before
  the next signal for that conversation, so each session sees ordered
  `conversation_seq` frames.
- Queue-full handling is unchanged: a full session queue causes explicit
  slow-session eviction, not silent drop.

Runtime knob:

```text
NEXUSIM_PUSH_CONVERSATION_FANOUT_BUCKETS=8
```

The local Docker profile enables 8 buckets on `push-gateway-ws` for the next
hotgroup retest.

Focused checks:

```powershell
. .\tools\go-env.ps1; go test ./services/push-gateway/... ./loadtest/hotgroup -count=1
. .\tools\go-env.ps1; go build ./services/push-gateway/cmd/push-gateway ./loadtest/hotgroup
```

Judgment before retest: this is an implementation change, not a capacity result.
The next step is a clean commit, rebuild / archive the push-gateway image,
redeploy it, then rerun the same 400 subscriber coordinator + 4 shard scenario
and compare worker fanout p95 / p99 and total signal drain rate against
`fedb5f43`.

## Conversation-Local Fanout Buckets Retest

The conversation-local fanout bucket implementation was committed as `a15e0ad`,
rebuilt, archived and redeployed to Ubuntu Docker. The image archive is:

```text
H:\NexusIM\docker-images\archives\nexusim-push-gateway-a15e0ad7-20260701-052941.tar
```

Comparable scenario:

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

- coordinator:
  `H:\NexusIM\loadtest-results\hotgroup-fanoutbuckets-clean-400sub-coordinator-20260701-053403`
- shards:
  `H:\NexusIM\loadtest-results\hotgroup-fanoutbuckets-clean-400sub-shard*-20260701-053403`
- analysis:
  `hotgroup-multirunner-analysis-20260701-fanoutbuckets-400sub.md`
- metrics window:
  `hotgroup-metrics-window-20260701-fanoutbuckets-400sub.md`

Result:

- coordinator send, PullInbox and ACK succeeded;
- `message_outbox_pending=0`, `delivery_outbox_pending=0`;
- 4 shards read 400000 conversation signals with no subscriber error;
- aggregate signal span rate was about `2874.378 signals/s`;
- previous writer-queue / batch-drain run was about `2884.066 signals/s`;
- WebSocket `delivery_notify` queue p95 / p99 stayed about
  `4.616ms / 4.931ms`;
- WebSocket `delivery_notify` write p95 / p99 stayed about
  `0.383ms / 0.574ms`;
- Redis subscriber conversation signal fanout p95 / p99 were about
  `54.133ms / 90.827ms` in the five-minute window and about
  `48.684ms / 89.796ms` across the captured window;
- Redis subscriber queue wait p95 / p99 stayed about `0.095ms / 0.099ms`;
- writer error, Redis subscriber error, queue-full and slow eviction stayed at
  0.

Judgment: conversation-local fanout buckets did not materially improve total
online signal drain. The metric did not move out of the existing
`2.85k-2.89k signals/s` band. Worker-side local fanout remains the strongest
measured bottleneck, but per-event goroutine buckets are not enough to break the
curve. The next optimization should avoid adding more ad hoc concurrency in the
same call path and instead evaluate a bigger design choice: persistent per
conversation / per bucket workers, distributing subscribers across multiple
push-gateway instances, or reducing total online signal volume through a more
aggressive pull-first policy for very large rooms.

## Multi Push-Gateway WS Topology Retest

The next experiment changed Docker topology rather than the push protocol:
four `push-gateway` WebSocket processes share the same Redis route backend, but
each process has a distinct `NEXUSIM_PUSH_GATEWAY_ID`, container IP and debug
port.

Topology change commit:

```text
4be4b2d deploy: add push gateway multi ws hotgroup topology
```

Runtime ports:

| instance | gateway_id | ws_url | debug_metrics |
| --- | --- | --- | --- |
| ws-1 | `nexusim-push-gateway-ws-1` | `ws://172.31.50.2:10498/ws` | `http://172.31.50.2:11913/metrics` |
| ws-2 | `nexusim-push-gateway-ws-2` | `ws://172.31.50.2:11001/ws` | `http://172.31.50.2:11941/metrics` |
| ws-3 | `nexusim-push-gateway-ws-3` | `ws://172.31.50.2:11002/ws` | `http://172.31.50.2:11942/metrics` |
| ws-4 | `nexusim-push-gateway-ws-4` | `ws://172.31.50.2:11003/ws` | `http://172.31.50.2:11943/metrics` |

Prometheus `nexusim-push-gateway` now scrapes all four debug endpoints. The
experiment assigns one `subscriber-only` shard to each WebSocket endpoint, so
the same 400 online subscribers are split 100 / 100 / 100 / 100 across four
processes.

Comparable scenario:

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

- coordinator:
  `H:\NexusIM\loadtest-results\hotgroup-multiws-clean-400sub-coordinator-20260701-055706`
- shards:
  `H:\NexusIM\loadtest-results\hotgroup-multiws-clean-400sub-shard*-20260701-055706`
- analysis:
  `hotgroup-multirunner-analysis-20260701-multiws-400sub.md`
- metrics window:
  `hotgroup-metrics-window-20260701-multiws-400sub.md`

Result:

- coordinator send, PullInbox and ACK succeeded;
- `message_outbox_pending=0`, `delivery_outbox_pending=0`;
- 4 shards read 400000 conversation signals with no subscriber error;
- aggregate signal span rate was about `2822.479 signals/s`;
- previous single-WS fanout-buckets baseline was about `2874.378 signals/s`;
- ratio vs baseline was about `0.982`, so throughput did not improve;
- Prometheus reported all four push debug targets as up;
- `delivery_notify` queue p95 / p99 were about `3.703ms / 4.742ms`;
- `delivery_notify` write p95 / p99 were about `0.425ms / 0.769ms`;
- Redis subscriber conversation-signal fanout p95 / p99 remained about
  `69.014ms / 93.803ms`;
- writer error, Redis subscriber error, queue-full and slow eviction stayed at
  0.

Judgment: simply distributing subscribers across multiple WebSocket processes
does not break the `2.8k signals/s` online-drain curve in the current lab
topology. This rules out a simple "single push-gateway process CPU is full"
explanation. The next module should focus on changing the fanout model or
reducing signal volume: persistent per-conversation / per-bucket workers,
pull-first or sampled online signals for very large rooms, and a smaller
diagnostic to distinguish server-side enqueue cost from client/network receive
cadence.

## Pull-First Sampled Online Signal Module

The next implementation step changes the very-large-room online wakeup model
instead of adding more goroutines to the same fanout path.

Motivation:

- The previous code-level optimizations did not break the
  `2.85k-2.89k signals/s` range.
- Multi WebSocket instances did not improve the same conversation drain curve.
- For READ_FANOUT / hot rooms, `delivery.notify` is a wakeup signal. The durable
  truth remains `delivery_timeline_items` + `PullInbox`, not the count of online
  WebSocket frames.

Change:

- `push-gateway` now supports explicit conversation signal sampling through:

```text
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY=1
```

- The default value `1` preserves the old full-signal behavior.
- A value such as `10` emits only conversation signals whose
  `conversation_seq % 10 == 0`.
- The skipped signals are counted as intentional sampled suppression, not as
  delivery success and not as hidden fallback.
- Redis route mode applies the same policy before local enqueue, Redis publish
  and remote resume append, so remote gateways do not receive suppressed signal
  events.
- Memory mode applies the same policy when it is the active route backend.

New low-cardinality metrics:

```text
nexusim_push_gateway_conversation_signal_events_total{event="suppressed_events"}
nexusim_push_gateway_conversation_signal_events_total{event="suppressed_sessions"}
nexusim_push_gateway_redis_route_events_total{event="conversation_signal_suppressed",role="registry"}
```

`loadtest/hotgroup` now supports matching validation:

```text
--conversation-signal-sample-every 10
```

The runner records:

```text
push.conversation_signal_sample_every
push.expected_signals_per_subscriber
push.expected_conversation_signals
```

Judgment before retest: this is not a capacity result yet. The next clean
Docker redeploy should run a comparable 400 subscriber coordinator + 4 shard
scenario with:

```text
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY=10
--conversation-signal-sample-every 10
```

Expected interpretation:

- If SendMessage / outbox / Kafka remain clean and signal drain time drops
  roughly with emitted signal count, the current limit is confirmed as online
  frame volume.
- PullInbox / ACK must still reach the latest message seq; sampled online signal
  cannot be counted as durable delivery.
- If drain rate stays flat even with 10x fewer emitted signals, the next
  bottleneck is likely client / network scheduling rather than server fanout
  volume.

## Pull-First Sampled Online Signal Retest

The sampled signal module was committed and deployed as:

```text
562290b3 feat: add sampled hotgroup conversation signals
873ea057 deploy: wire hotgroup signal sampling
bac71c65 deploy: sample push delivery consumer signals
```

The clean `nexusim/push-gateway:local` image was rebuilt and archived at:

```text
H:\NexusIM\docker-images\archives\nexusim-push-gateway-bac71c65-20260701-063645.tar
```

The archive was loaded on Ubuntu and these containers were recreated with
`NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY=10`:

```text
nexusim-push-gateway-delivery-consumer
nexusim-push-gateway-ws
nexusim-push-gateway-ws-2
nexusim-push-gateway-ws-3
nexusim-push-gateway-ws-4
```

The comparable 400 subscriber coordinator + 4 shard run used the same
6000-member / 1000-message / 8000 msg/s / 256-sender READ_FANOUT shape as the
full-signal multi-ws run.

Artifacts:

```text
H:\NexusIM\loadtest-results\hotgroup-sample10-clean-400sub-coordinator-20260701-070655
H:\NexusIM\loadtest-results\hotgroup-sample10-clean-400sub-shard*-20260701-070655
docs/runbook/loadtest/hotgroup/hotgroup-multirunner-analysis-20260701-sample10-400sub.md
docs/runbook/loadtest/hotgroup/hotgroup-metrics-window-20260701-sample10-400sub.md
```

Results:

| metric | value |
| --- | ---: |
| success | true |
| group_size | 6000 |
| fanout_mode | READ_FANOUT |
| message_count | 1000 |
| target_message_rate | 8000 msg/s |
| sender_count | 256 |
| send_success / errors | 1000 / 0 |
| send_p95 / p99 | 17.835 ms / 22.003 ms |
| PullInbox p95 | 122.69 ms |
| message_outbox_pending / delivery_outbox_pending | 0 / 0 |
| subscriber_count | 400 |
| emitted conversation signals read | 40000 |
| signal_span_seconds | 25.243 s |
| baseline full-signal span | 141.719 s |

Prometheus window:

| metric | value |
| --- | ---: |
| core_targets_up | 7 |
| delivery_outbox_pending max | 267, then 0 |
| push writer delivery notify success window | about 40720.899 |
| Redis subscriber messages window | about 407.209 |
| Redis subscriber enqueued window | about 40720.899 |
| delivery notify write p95 / p99 window | 0.481 ms / 0.948 ms |
| delivery notify queue p95 / p99 window | 4.045 ms / 4.82 ms |
| Redis subscriber fanout p95 / p99 window | 4.767 ms / 10 ms |

Interpretation:

- The emitted online signal count dropped from 400000 to 40000, and the drain
  span dropped from 141.719s to 25.243s. This confirms that online frame volume
  is a material factor for large READ_FANOUT rooms.
- SendMessage, message outbox, delivery projection, delivery outbox, PullInbox
  and ACK remained valid; sampled online signal did not replace durable
  PullInbox semantics.
- The drain span did not drop by a full 10x. At this smaller emitted-frame
  volume, fixed setup time, WebSocket scheduling and client/network receive
  cadence become more visible. Do not report this as a production capacity
  number.
- For very large rooms, the next strategy is to keep `sample_every` explicit per
  room policy and then test larger message counts / higher subscriber counts to
  find the new sustainable QPS curve.

## Sampled Signal Larger Message Count Retest

The next comparable run kept `sample_every=10`, 400 conversation subscribers,
256 senders and a target 8000 msg/s, but increased `message_count` from 1000 to
5000.

Artifacts:

```text
H:\NexusIM\loadtest-results\hotgroup-sample10-400sub-5000msg-coordinator-20260701-072206
H:\NexusIM\loadtest-results\hotgroup-sample10-400sub-5000msg-shard*-20260701-072206
docs/runbook/loadtest/hotgroup/hotgroup-multirunner-analysis-20260701-sample10-400sub-5000msg.md
docs/runbook/loadtest/hotgroup/hotgroup-metrics-window-20260701-sample10-400sub-5000msg.md
```

Results:

| metric | value |
| --- | ---: |
| success | true |
| group_size | 6000 |
| fanout_mode | READ_FANOUT |
| message_count | 5000 |
| target_message_rate | 8000 msg/s |
| sender_count | 256 |
| send_success / errors | 5000 / 0 |
| send_p95 / p99 | 18.103 ms / 20.914 ms |
| PullInbox p95 | 23.874 ms |
| message_outbox_pending / delivery_outbox_pending | 0 / 0 |
| subscriber_count | 400 |
| emitted conversation signals read | 200000 |
| signal_span_seconds | 138.555 s |
| signal_span_rate | 1443.474 signals/s |

Prometheus window:

| metric | value |
| --- | ---: |
| core_targets_up | 7 |
| delivery_outbox_pending max | 1763, then 0 |
| push writer delivery notify success window | about 203660 |
| Redis subscriber messages window | about 2036 |
| Redis subscriber enqueued window | about 203660 |
| delivery notify write p95 / p99 window | about 0.458 ms / 0.87 ms |
| delivery notify queue p95 / p99 window | about 3.991 ms / 4.799 ms |
| Redis subscriber fanout p95 / p99 window | about 54.541 ms / 90.908 ms |
| writer / Redis errors / queue-full / eviction | 0 |

Interpretation:

- Increasing sampled messages from 1000 to 5000 scaled emitted frames from 40000
  to 200000 and kept the durable path healthy: SendMessage, PullInbox, ACK,
  message outbox and delivery outbox all completed.
- The signal span grew to 138.555s and the span rate dropped from about
  1584.587 to 1443.474 signals/s. This keeps the bottleneck in
  `online-signal-drain`.
- Single WebSocket write latency remains low. The stronger evidence is again
  Redis subscriber conversation fanout / enqueue duration around 54ms p95 and
  91ms p99 when the sampled signal count grows.
- The next module should not be another blind scale-up. Pick one concrete
  design: room-level online signal policy / adaptive cadence, or a persistent
  per-conversation / per-bucket fanout worker model inside push-gateway.

## Fanout-Mode Conversation Signal Policy Module

The next implemented module turns the previous global `sample_every` knob into
an explicit room-policy input at push-gateway.

Architecture decision:

- no new middleware is introduced;
- no service boundary changes are required;
- `delivery.conversation.signal.v1` already carries `fanout_mode`, so
  push-gateway can make the online wakeup decision from the delivery event
  itself;
- durable truth remains `delivery_timeline_items` plus `PullInbox` / ACK;
- sampled WebSocket frames are still only online wakeups, not reliable delivery.

Code changes:

- push-gateway internal `DeliveryNotification` now carries `fanout_mode`;
- delivery consumer passes `DeliveryConversationSignalV1.fanout_mode` through
  to the registry;
- memory registry and Redis route registry use the same
  `ConversationSignalPolicy`;
- invalid / missing fanout mode on a conversation signal is fail-closed;
- mode-specific env values are parsed strictly, so a typo does not silently
  fall back to a different cadence.

Configuration:

```text
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY=1
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY_WRITE_FANOUT=
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY_HYBRID_FANOUT=
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY_READ_FANOUT=10
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY_BROADCAST_SIGNAL=10
```

Semantics:

- `NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY` is the explicit default policy;
- mode-specific env overrides only that fanout mode;
- unset mode-specific env does not change behavior;
- `1` means full signal;
- `10` means emit only `conversation_seq % 10 == 0`.

Recommended next retest:

```text
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY=1
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY_READ_FANOUT=10
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY_BROADCAST_SIGNAL=10
loadtest/hotgroup --conversation-signal-sample-every 10 ...
```

This retest should compare against the previous global sample=10 runs. Expected
evidence:

- READ_FANOUT keeps the sampled signal count and drain profile;
- smaller fanout modes can remain full-signal if no mode-specific sample is set;
- PullInbox / ACK still reach latest seq;
- suppressed signal counters match expected mode-specific policy;
- Redis subscriber fanout p95 / p99 should be interpreted against the reduced
  emitted-frame count, not as durable delivery throughput.
