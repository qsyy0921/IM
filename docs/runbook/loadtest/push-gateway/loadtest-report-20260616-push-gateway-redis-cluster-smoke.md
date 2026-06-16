# push-gateway Redis Cluster topology smoke

Date: 2026-06-16

## Result

Passed.

This run verifies the first real local Redis Cluster topology for `push-gateway` route and Redis-backed resume:

```text
delivery_outbox -> im.delivery.events -> push-gateway delivery-consumer
-> Redis Cluster route / PubSub -> WebSocket gateway
-> delivery.notify -> PullInbox -> delivery.ack.ok
-> reconnect to second gateway -> Redis-backed resume replay
```

## Command

```powershell
. .\tools\go-env.ps1
.\tools\local-redis-cluster-smoke.ps1 -RunName redis-cluster-smoke-20260616-clean
```

The wrapper starts local PostgreSQL / Kafka, creates a three-node Redis Cluster container, applies message / conversation / delivery migrations, then delegates to:

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario cross-instance-resume `
  -RouteBackend redis `
  -RedisMode cluster `
  -RedisClusterAddrs 127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002
```

## Raw Artifacts

```text
H:\NexusIM\loadtest-results\redis-cluster-smoke-20260616-clean\redis-cluster-summary.json
H:\NexusIM\loadtest-results\redis-cluster-smoke-20260616-clean\pushgateway-summary.json
```

## Evidence

```text
commit = c235edb
git_dirty = false
redis_mode = cluster
redis_cluster_addrs = 127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002
scenario = cross-instance-resume
success = true
```

Functional checks:

```text
server.hello received
member JOIN boundary_seq = 1
SendMessage conversation_seq = 2
delivery.notify received with conversation_seq = 2
PullInbox item_count = 1, max_seq = 2
delivery.ack.ok last_received_seq = 2
delivery_outbox total = 2
delivery_outbox PUBLISHED = 2
delivery_outbox PENDING = 0
delivery_outbox DLQ = 0
```

Cross-instance resume checks:

```text
original gateway session_id = sess_43c1775166ea0f7b74f86746840893f2
reconnected gateway session_id = sess_6d86ba13accf62e7722dcdb2aaebc3de
resume_token reused = resume_c269366826d19d60be3c4bcee999a092
replayed delivery.notify event_id = evt_delivery_inbox_59b531f85adb09271618e4afb9f25f9a
redis_resume_replay_count = 1
```

Redis route metrics from the consumer gateway:

```text
redis_route_remote_matched_sessions = 1
redis_route_remote_publish_call_count = 1
redis_route_remote_publish_error_count = 0
redis_route_remote_no_subscriber_count = 0
redis_route_remote_enqueued_sessions = 1
redis_resume_append_count = 1
```

## Interpretation

This proves:

- `NEXUSIM_PUSH_REDIS_MODE=cluster` can connect to a real local Redis Cluster.
- Route and resume keys use cluster-compatible hash tags well enough for the current route / resume smoke.
- Cross-process online notify still reaches the WebSocket gateway.
- Redis-backed resume replay works after reconnecting to a second gateway.
- Durable recovery remains grounded in `PullInbox` and `AckDelivery`.

This does not prove:

- production-grade Redis HA;
- Redis Cluster failover or resharding behavior;
- zero-loss behavior during slot migration or node loss;
- cross-machine Redis Cluster topology;
- capacity or long-running stability.

Those remain future hardening items.
