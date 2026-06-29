# Hot Group Loadtest: delivery outbox relay bottleneck and follow-up

Date: 2026-06-28

Scope: local three-machine lab, Ubuntu Docker backend, Windows `loadtest/hotgroup` runner.

This report records the first comparable hot-group QPS steps around the
`delivery_outbox -> Kafka im.delivery.events` relay hardening. It is a local
engineering benchmark and interview evidence. It is not a production SLO or
final capacity sizing result.

## Runtime

- PostgreSQL / Kafka / Redis / backend services ran on Ubuntu Docker host
  `172.31.50.2`.
- Windows ran `loadtest/hotgroup`.
- Tested service path:

```text
CreateConversation(GROUP)
-> CreateMemberChange(JOIN)
-> SendMessage
-> message_outbox
-> Kafka conversation.timeline.events
-> delivery timeline projection
-> user_inbox fanout
-> delivery_outbox
-> delivery outbox relay
-> Kafka im.delivery.events
-> sampled PullInbox / AckDelivery
```

## Change Under Test

The delivery outbox relay was changed from a single-worker, small batch path to
a sharded worker path:

- `NEXUSIM_DELIVERY_OUTBOX_WORKERS=4`
- `NEXUSIM_DELIVERY_OUTBOX_BATCH_SIZE=500`
- `NEXUSIM_DELIVERY_KAFKA_BATCH_SIZE=500`
- `NEXUSIM_DELIVERY_KAFKA_BATCH_TIMEOUT=20ms`
- added ready / blocking indexes for `delivery_outbox`
- changed worker sharding to align with `tenant_id + conversation_id`, so one
  conversation is handled by one worker in seq order, while different
  conversations can progress in parallel

The image used for the second run was rebuilt locally and loaded into Ubuntu
Docker. The latest archived image copy is:

```text
H:\NexusIM\docker-images\nexusim-delivery-service-local-latest.tar
```

## Results

### Baseline before relay hardening

Raw result root:

```text
H:\NexusIM\loadtest-results\hotgroup-qps-step-20260628-210213
```

This run used a 20-member group and 2 senders. It showed the first bottleneck
clearly: `SendMessage` and durable `user_inbox` completed, but
`delivery_outbox` could not drain inside the wait window once the target rate
exceeded 10 QPS.

| Run | Group | Senders | Target QPS | Messages | Send OK | Send p95 ms | user_inbox | delivery_outbox pending | Result |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `qps-10-210213` | 20 | 2 | 10 | 50 | 50 | 11.881 | 1000 / 1000 | 0 | pass |
| `qps-50-210223` | 20 | 2 | 50 | 250 | 250 | 14.222 | 5000 / 5000 | 3000 | fail: relay drain timeout |
| `qps-100-210531` | 20 | 2 | 100 | 500 | 500 | 9.780 | 10000 / 10000 | 9025 | fail: relay drain timeout |
| `qps-150-210855` | 20 | 2 | 150 | 750 | 750 | 9.737 | 15000 / 15000 | 14485 | fail: relay drain timeout |

### First relay worker attempt

Raw result root family:

```text
H:\NexusIM\loadtest-results\hotgroup-relayopt-*qps-20260628-221338
```

This run rebuilt `delivery-service` with multi-worker settings, but the first
sharding implementation used `delivery_outbox.id`. That was the wrong unit for
the business ordering rule: outbox ordering is per `tenant_id + conversation_id
+ aggregate_version`. A single hot conversation was split across worker shards,
but lower sequence rows still blocked later sequence rows. The result was no
real throughput gain and higher pending counts.

This intermediate result is kept as a diagnostic artifact, not a successful
optimization result.

### Corrected conversation-sharded relay

Raw result root family:

```text
H:\NexusIM\loadtest-results\hotgroup-relayshard-*qps-20260628-222758
H:\NexusIM\loadtest-results\hotgroup-relayshard-probe-*qps-20260628-223453
```

The corrected relay shards by `tenant_id + conversation_id`; it keeps one
conversation in seq order and allows different conversations to progress in
parallel.

| Run | Group | Senders | Target QPS | Messages | Send OK | Send p95 ms | user_inbox | delivery_outbox pending at summary | Result |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `hotgroup-relayshard-10qps-20260628-222758` | 100 | 5 | 10 | 50 | 50 | 13.458 | 5000 / 5000 | 0 | pass |
| `hotgroup-relayshard-probe-50qps-20260628-223453` | 100 | 5 | 50 | 250 | 250 | 11.224 | 25000 / 25000 | 0 | pass |
| `hotgroup-relayshard-100qps-20260628-222758` | 100 | 5 | 100 | 500 | 500 | 10.101 | 50000 / 50000 | 0 | pass |
| `hotgroup-relayshard-150qps-20260628-222758` | 100 | 5 | 150 | 750 | 750 | 9.879 | 75000 / 75000 | 0 | pass |
| `hotgroup-relayshard-probe-200qps-20260628-223453` | 100 | 5 | 200 | 1000 | 1000 | 10.893 | 85800 / 100000 at timeout | 1400 at summary | fail: `user_inbox` fanout wait timeout |

After the 200 QPS probe timed out, a follow-up SQL check showed that the system
eventually caught up:

```text
user_inbox rows:      100000 / 100000
delivery_outbox rows: 100000 PUBLISHED, 0 PENDING
```

That means the 200 QPS probe did not expose data loss. It exposed that the
current wait window was exceeded by the asynchronous projection / fanout path.

## Bottleneck Analysis

Before the relay hardening, the first visible bottleneck was:

```text
delivery_outbox -> Kafka im.delivery.events
```

Symptoms:

- `SendMessage` completed.
- `user_inbox` was fully materialized.
- `delivery_outbox PENDING` accumulated.
- Kafka delivery events could not be published quickly enough.

After the corrected relay hardening, that bottleneck moved:

```text
conversation.timeline.events
-> delivery timeline consumer
-> user_inbox fanout
```

Symptoms at 200 QPS:

- `SendMessage` completed 1000 / 1000.
- p95 send latency stayed around 10.893 ms.
- `user_inbox` was still catching up when the runner hit the wait timeout.
- Later DB checks showed both `user_inbox` and `delivery_outbox` caught up.

## Interview Narrative

The useful answer is not "we increased QPS by adding workers". The more precise
answer is:

```text
The first hot-group test showed that delivery_outbox relay was the bottleneck:
messages and durable inbox were already written, but im.delivery.events could
not drain. We optimized the relay with batch publish, ready indexes and worker
sharding. The first sharding attempt used row id and failed because ordering is
per conversation. After aligning sharding to tenant + conversation, 100-member
groups passed 150 QPS with full outbox drain. The next 200 QPS probe moved the
bottleneck to delivery timeline projection / user_inbox fanout, so the next
engineering target is fanout batching, timeline consumer parallelism and
hot-group strategy promotion, not more relay workers.
```

## Limits

- `loadtest/hotgroup` still sends from one runner process and is not yet a
  final high-concurrency traffic generator.
- The run does not include WebSocket notify storm or slow-client pressure.
- The run is not a production capacity number.
- Grafana / Prometheus trend screenshots are not included in this report yet;
  future formal capacity reports must attach the dashboard time range.

## Next Work

1. Optimize delivery timeline projection / `user_inbox` fanout:
   - batch fanout writes;
   - shard fanout by conversation / user bucket where semantics allow;
   - expose projection lag and inbox rows per message as metrics.
2. Upgrade `loadtest/hotgroup`:
   - multiple goroutines / sender connections;
   - Kafka lag and projection lag capture;
   - optional WebSocket receiver pressure.
3. Add formal dashboard evidence:
   - Prometheus range;
   - Grafana panel export;
   - PostgreSQL / Kafka / service metrics around each run.
