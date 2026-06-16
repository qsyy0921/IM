# Push Gateway Redis Cluster Capacity Smoke - 2026-06-16

## Scope

This is a local short capacity baseline for `push-gateway` with Redis Cluster route backend.

It verifies:

- local six-node Redis Cluster topology is usable by push-gateway route / PubSub;
- two online devices for the same user receive online `delivery.notify`;
- 16 consecutive `SendMessage` calls are projected to delivery and pushed online;
- `PullInbox` returns all 16 durable inbox items;
- each device can ACK the final visible seq;
- delivery outbox drains without pending or DLQ rows.

It does not prove production sizing, long-running stability, cross-machine Redis Cluster throughput, cross-AZ behavior, or zero-loss failover windows.

## Command

```powershell
. .\tools\go-env.ps1
.\tools\local-redis-cluster-capacity-smoke.ps1 `
  -RunName redis-cluster-capacity-smoke-20260616-clean `
  -MessageCount 16 `
  -ReceiverDeviceIds "push-device-1,push-device-2" `
  -SkipBuild
```

## Raw Artifacts

```text
H:\NexusIM\loadtest-results\redis-cluster-capacity-smoke-20260616-clean\redis-cluster-capacity-summary.json
H:\NexusIM\loadtest-results\redis-cluster-capacity-smoke-20260616-clean\pushgateway-summary.json
```

## Environment

```text
commit = 84f2e6e
git_dirty = false
redis_mode = cluster
redis_cluster_addrs = 127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002,127.0.0.1:7003,127.0.0.1:7004,127.0.0.1:7005
redis_cluster_ha_node_lines = 6
redis_cluster_ha_masters = 3
redis_cluster_ha_replicas = 3
scenario = full
route_backend = redis
message_count = 16
receiver_device_ids = push-device-1,push-device-2
```

## Result

```text
success = true
member_join.boundary_seq = 1
last SendMessage conversation_seq = 17
PullInbox item_count = 16
PullInbox max_seq = 17
device push-device-1 cursor_last_received_seq = 17
device push-device-2 cursor_last_received_seq = 17
delivery_outbox_total = 18
delivery_outbox_published = 18
delivery_outbox_pending = 0
delivery_outbox_dlq = 0
```

`delivery_outbox_total=18` is expected:

```text
16 delivery.inbox_item.created.v1
2 delivery.ack.recorded.v1
```

## Capacity Summary

```text
duration_ms = 5228.73
device_count = 2
message_count = 16
notify_frame_count = 32
ack_frame_count = 2
pull_inbox_item_count = 16
delivery_outbox_published = 18
messages_per_second = 3.0600
notify_frames_per_second = 6.1200
ack_frames_per_second = 0.3825
pull_items_per_second = 3.0600
```

## Redis Route Metrics

Consumer gateway metrics after the run:

```text
redis_route_remote_matched_sessions = 32
redis_route_remote_publish_call_count = 16
redis_route_remote_publish_error_count = 0
redis_route_remote_no_subscriber_count = 0
redis_route_remote_enqueued_sessions = 32
redis_resume_append_count = 32
```

This matches two active device sessions for each of the 16 message notifications.

## Interpretation

This smoke closes the first local Redis Cluster short-capacity evidence gap for `push-gateway`.

The remaining Redis work is still:

- longer capacity runs with resource curves;
- production Redis Cluster / Sentinel HA design;
- cross-machine Redis Cluster behavior;
- sizing under real connection counts and mixed traffic.
