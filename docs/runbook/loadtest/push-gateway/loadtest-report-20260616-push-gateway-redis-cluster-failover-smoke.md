# push-gateway Redis Cluster automatic failover smoke

Date: 2026-06-16

## Result

Passed.

This run verifies a local six-node Redis Cluster topology for `push-gateway`
route and online notification during a master failure. The test creates a
three-master / three-replica Redis Cluster, stops the master that owns the
receiver route key slot, waits for a replica to be promoted, then sends a
message and expects online `delivery.notify` to still arrive.

```text
6-node Redis Cluster, replicas=1
-> WebSocket route registered
-> route key slot owner master stopped
-> replica promoted to new master
-> SendMessage
-> delivery_outbox -> im.delivery.events -> push-gateway delivery-consumer
-> Redis Cluster route / PubSub -> WebSocket gateway
-> delivery.notify -> PullInbox -> delivery.ack.ok
```

## Command

```powershell
. .\tools\go-env.ps1
.\tools\local-redis-cluster-failover-smoke.ps1 -RunName redis-cluster-failover-smoke-20260616-clean
```

The wrapper starts local PostgreSQL / Kafka, creates a six-node Redis Cluster
container, applies message / conversation / delivery migrations, locates the
Redis Cluster route key slot owner, stops that master, waits for a promoted
master, then delegates to:

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario redis-cluster-failover `
  -RouteBackend redis `
  -RedisMode cluster `
  -RedisClusterAddrs 127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002,127.0.0.1:7003,127.0.0.1:7004,127.0.0.1:7005
```

## Raw Artifacts

```text
H:\NexusIM\loadtest-results\redis-cluster-failover-smoke-20260616-clean\redis-cluster-failover-summary.json
H:\NexusIM\loadtest-results\redis-cluster-failover-smoke-20260616-clean\pushgateway-summary.json
```

## Evidence

```text
commit = 4bed34d
git_dirty = false
redis_mode = cluster
redis_cluster_addrs = 127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002,127.0.0.1:7003,127.0.0.1:7004,127.0.0.1:7005
scenario = redis-cluster-failover
success = true
```

Redis Cluster HA topology:

```text
redis_cluster_ha_node_lines = 6
redis_cluster_ha_masters = 3
redis_cluster_ha_replicas = 3
redis_cluster_ha_config = OK
```

Failover details:

```text
redis_cluster_fault_key = nexusim:push:redis-cluster-failover-smoke-20260616-clean:route:{user:tenant-redis-cluster-failover-smoke-20260616-clean:push-user-1}:user
redis_cluster_fault_slot = 13058
redis_cluster_stopped_master_port = 7002
redis_cluster_promoted_master_port = 7003
```

Functional checks:

```text
server.hello received
member JOIN boundary_seq = 1
SendMessage conversation_seq = 2
delivery.notify received with conversation_seq = 2
PullInbox item_count = 1, max_seq = 2
delivery.ack.ok last_received_seq = 2
cursor_last_received_seq = 2
delivery_outbox total = 2
delivery_outbox PUBLISHED = 2
delivery_outbox PENDING = 0
delivery_outbox DLQ = 0
```

Redis route observations from the consumer gateway:

```text
redis_route_remote_matched_sessions = 1
redis_route_remote_publish_call_count = 1
redis_route_remote_publish_error_count = 0
redis_resume_append_count = 1
```

Capacity summary for this fault smoke:

```text
duration_ms = 10291.24
message_count = 1
notify_frame_count = 2
ack_frame_count = 1
pull_inbox_item_count = 1
messages_per_second = 0.0972
```

## Interpretation

This proves:

- `push-gateway` can use a local six-node Redis Cluster topology with replicas.
- The local Redis Cluster can promote a replica after the route key slot owner
  master is stopped.
- After the promotion, `push-gateway` can still route an online
  `delivery.notify`.
- The durable `PullInbox + AckDelivery` path still confirms the delivery fact.

This does not prove:

- production-grade Redis Cluster HA;
- zero-loss online wakeup under every Redis Cluster failover window;
- cross-machine or cross-AZ Redis Cluster behavior;
- Redis Cluster resharding or slot migration safety;
- capacity or long-running stability.

Those remain future hardening items.
