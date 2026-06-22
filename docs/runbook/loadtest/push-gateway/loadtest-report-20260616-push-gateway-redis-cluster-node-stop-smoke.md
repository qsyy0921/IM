# push-gateway Redis Cluster node-stop recovery smoke

Date: 2026-06-16

## Result

Passed.

This run verifies the first local Redis Cluster node-stop fault smoke for
`push-gateway`. The test stops the Redis Cluster master that owns the route key
slot for the online receiver, then verifies that online `delivery.notify` may be
missed while durable `PullInbox + AckDelivery` still recovers the message.

```text
Redis Cluster route key slot owner stopped
-> delivery_outbox -> im.delivery.events -> push-gateway delivery-consumer
-> Redis route lookup fails
-> online delivery.notify is not observed
-> client PullInbox reads durable user_inbox
-> client sends delivery.ack
-> delivery.ack.ok returned
```

## Command

```powershell
. .\tools\go-env.ps1
.\tools\local-redis-cluster-node-stop-smoke.ps1 -RunName redis-cluster-node-stop-smoke-20260616-clean
```

The wrapper starts local PostgreSQL / Kafka, creates a three-node Redis Cluster
container, applies message / conversation / delivery migrations, locates the
Redis Cluster route key slot owner, stops that node, and delegates to:

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario redis-cluster-node-stop `
  -RouteBackend redis `
  -RedisMode cluster `
  -RedisClusterAddrs 127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002
```

## Raw Artifacts

```text
H:\NexusIM\loadtest-results\redis-cluster-node-stop-smoke-20260616-clean\redis-cluster-node-stop-summary.json
H:\NexusIM\loadtest-results\redis-cluster-node-stop-smoke-20260616-clean\pushgateway-summary.json
```

## Evidence

```text
commit = fb75bf1
git_dirty = false
redis_mode = cluster
redis_cluster_addrs = 127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002
scenario = redis-cluster-node-stop
success = true
```

Fault details:

```text
redis_cluster_fault_key = nexusim:push:redis-cluster-node-stop-smoke-20260616-clean:route:{user:tenant-redis-cluster-node-stop-smoke-20260616-clean:push-user-1}:user
redis_cluster_fault_slot = 13965
redis_cluster_stopped_port = 7002
```

Functional checks:

```text
server.hello received
member JOIN boundary_seq = 1
SendMessage conversation_seq = 2
delivery.notify received = false
notify_wait_error = notify timeout
PullInbox item_count = 1, max_seq = 2
delivery.ack.ok last_received_seq = 2
delivery_outbox total = 2
delivery_outbox PUBLISHED = 2
delivery_outbox PENDING = 0
delivery_outbox DLQ = 0
```

Redis route metrics from the consumer gateway:

```text
redis_route_lookup_error_count = 1
redis_route_remote_publish_error_count = 0
redis_route_remote_no_subscriber_count = 0
```

Capacity summary for this fault smoke:

```text
duration_ms = 7628.026
message_count = 1
notify_frame_count = 0
ack_frame_count = 1
pull_inbox_item_count = 1
messages_per_second = 0.1311
```

## Interpretation

This proves:

- `push-gateway` can run a local Redis Cluster route-key owner node-stop
  fault smoke.
- If route lookup fails during the fault window, the online wakeup can be
  missed without losing the durable delivery fact.
- `delivery-service PullInbox` remains the recovery source of truth.
- `AckDelivery` can still advance the device cursor after recovery recovery.
- The delivery outbox relay drains successfully: no pending or DLQ rows remain.

This does not prove:

- production-grade Redis Cluster HA;
- Redis Cluster automatic failover correctness under all slot owner failures;
- cross-machine Redis Cluster failure behavior;
- zero-loss online wakeup during Redis Cluster node loss;
- capacity or long-running stability.

Those remain future hardening items.
