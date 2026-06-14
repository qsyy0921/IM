# push-gateway Redis Sentinel quorum-loss fallback smoke - 2026-06-14

本轮在 clean commit `a511de5` 上验证了本地三 Redis / 三 Sentinel 拓扑下的 quorum-loss fallback：

```text
route registered
-> stop two Sentinel peers
-> stop current Redis master
-> no online delivery.notify within 1s
-> PullInbox durable recovery
-> delivery.ack.ok
```

这条 smoke 证明的是：当 Sentinel 失去 quorum 且当前 master 不可用时，`push-gateway` 的在线唤醒可以退化，但客户端仍能通过 `delivery-service PullInbox + AckDelivery` 完成恢复。它不是完整 Redis HA 验收，不覆盖真实网络分区、Redis Cluster、split-brain、跨机链路抖动或容量结论。

## 运行方式

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\local-redis-sentinel-quorum-loss-smoke.ps1 `
  -RunName redis-sentinel-quorum-loss-smoke-20260614f `
  -SkipBuild
```

本地 Redis / Sentinel 拓扑：

```text
redis-ha-master        127.0.0.1:6380
redis-ha-replica-1     127.0.0.1:6381
redis-ha-replica-2     127.0.0.1:6382
redis-sentinel-1       127.0.0.1:26379
redis-sentinel-2       127.0.0.1:26380
redis-sentinel-3       127.0.0.1:26381
```

故障脚本在 route 注册后执行：

```text
1. 读取 Sentinel 当前 master（本轮为 172.31.50.1:6380）
2. 停止 nexusim-redis-sentinel-2
3. 停止 nexusim-redis-sentinel-3
4. 停止当前 master 容器 nexusim-redis-ha-master
5. 继续 SendMessage -> delivery projection -> push-gateway notify path
6. 验证 1s 观察窗内没有 delivery.notify
7. 通过 PullInbox 补拉并 ACK
8. 恢复 master 和两个 Sentinel peer
```

## 原始结果

summary:

```text
H:\NexusIM\loadtest-results\redis-sentinel-quorum-loss-smoke-20260614f\pushgateway-summary.json
```

wrapper summary:

```text
H:\NexusIM\loadtest-results\redis-sentinel-quorum-loss-smoke-20260614f\redis-sentinel-quorum-loss-summary.json
```

关键输出：

```text
commit=a511de5
git_dirty=false
scenario=redis-sentinel-quorum-loss
sentinel_master_before=172.31.50.1:6380
stopped_master_container=nexusim-redis-ha-master
stopped_sentinels=nexusim-redis-sentinel-2,nexusim-redis-sentinel-3
sentinel_master_after_fault=172.31.50.1:6380
notify_received=false
notify_wait_error=notify timeout
```

## 关键结果

WebSocket / durable recovery：

```text
server.hello session_id=sess_b7e1d67d5c26c06156dbc53026dbbc6f
member_join boundary_seq=1
send_message conversation_seq=2
delivery.notify: timeout within 1s observation window
PullInbox item_count=1
PullInbox max_seq=2
delivery.ack.ok last_received_seq=2
cursor_last_received_seq=2
```

Redis route 调试指标：

```text
redis_route_remote_matched_sessions=1
redis_route_remote_publish_call_count=1
redis_route_remote_publish_error_count=0
redis_route_remote_no_subscriber_count=1
redis_route_remote_enqueued_sessions=0
redis_resume_append_count=1
```

Delivery 持久结果：

```text
delivery_outbox_total=2
delivery_outbox_published=2
delivery_outbox_pending=0
delivery_outbox_dlq=0
```

恢复输出：

```text
sentinel_restored_containers=nexusim-redis-ha-master,nexusim-redis-sentinel-2,nexusim-redis-sentinel-3
sentinel_master_after_restore=172.31.50.1:6380
```

## 结论

本轮可以确认三点：

1. 本地 Redis Sentinel 失去 quorum 且当前 master 不可用时，`push-gateway` 在线 notify 可以退化为超时。
2. `message-service -> delivery-service -> user_inbox` durable 路径不依赖 Redis route，客户端仍能通过 `PullInbox` 恢复消息。
3. Redis 恢复后，同一链路可以继续完成 `delivery.ack.ok`，cursor 推进到可见最大 seq。

## 不代表什么

- 不代表真实网络分区已经验证完成。
- 不代表 Redis Cluster 或生产级 Redis HA 已完成。
- 不代表 split-brain、跨机链路抖动、Sentinel 配置漂移或多用户并发都已覆盖。
- 不代表在线 notify 在所有 Redis 故障窗口里都严格丢失或严格恢复；本轮只证明当前脚本下的 quorum-loss fallback。

## 下一步

1. 继续补真实 Redis 网络分区 smoke。
2. 把 Redis 故障 smoke 和 Win/Mac 双机链路组合起来。
3. 再处理服务发现、统一观测和部署编排这几类生产化缺口。
