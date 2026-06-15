# Push Gateway Redis Sentinel Network Partition Smoke

## Summary

- Date: 2026-06-15
- Commit under test: `e6071d9 test: add redis sentinel network partition smoke`
- Result: passed
- Raw result root: `H:\NexusIM\loadtest-results\redis-sentinel-network-partition-smoke-20260615-200245`
- Summary JSON: `H:\NexusIM\loadtest-results\redis-sentinel-network-partition-smoke-20260615-200245\pushgateway-summary.json`
- Wrapper summary: `H:\NexusIM\loadtest-results\redis-sentinel-network-partition-smoke-20260615-200245\redis-sentinel-network-partition-summary.json`

This is a local Docker network-partition simulation for Redis Sentinel mode. It is not a production Redis HA, Redis Cluster, cross-AZ partition, or capacity result.

## Scenario

The smoke used the existing push-gateway Redis route split:

```text
push-gateway-ws
push-gateway-consumer
Redis Sentinel route backend
delivery-service durable inbox
```

The generated fault script queried Sentinel for the current master, mapped port `6380` to `nexusim-redis-ha-master`, and disconnected that container from Docker network `nexusim-local_default`.

```text
sentinel_master_before=172.31.50.1:6380
partitioned_container=nexusim-redis-ha-master
partitioned_network=nexusim-local_default
sentinel_master_after_partition=172.31.50.1:6380
```

The restore script reconnected the container to the same Docker network and verified Sentinel master reachability:

```text
sentinel_network_restored_container=nexusim-redis-ha-master
sentinel_network_restored_network=nexusim-local_default
sentinel_master_after_network_restore=172.31.50.1:6380
```

## Evidence

The clean run reported:

```text
commit=e6071d9
git_dirty=false
scenario=redis-sentinel-network-partition
route_backend=redis
redis_mode=sentinel
success=true
```

Online notify degraded during the partition observation window:

```text
notify_received=false
notify_wait_error=notify timeout
```

Durable recovery succeeded through delivery-service:

```text
PullInbox item_count=1
PullInbox max_seq=2
delivery.ack.ok last_received_seq=2
cursor_last_received_seq=2
```

Delivery outbox drained:

```text
delivery_outbox_total=2
delivery_outbox_pending=0
delivery_outbox_published=2
delivery_outbox_dlq=0
```

## Interpretation

This proves the local best-effort online wakeup layer can lose or miss `delivery.notify` during a Redis Sentinel master network partition, while the reliable path still recovers through:

```text
delivery-service user_inbox
-> PullInbox
-> delivery.ack
-> AckDelivery cursor
```

It does not prove zero notification loss, Redis Cluster HA, Sentinel quorum safety under all partitions, or production-grade network-failure behavior.
