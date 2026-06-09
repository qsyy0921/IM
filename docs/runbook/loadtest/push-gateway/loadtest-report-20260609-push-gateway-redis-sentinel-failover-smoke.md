# push-gateway Redis Sentinel failover smoke - 2026-06-09

## 结论

本轮在 clean commit `819c14a` 上验证了本地三 Redis / 三 Sentinel 拓扑下的手动 master failover recovery：

```text
push-gateway-ws registers route through Sentinel
-> trigger SENTINEL failover mymaster
-> wait until Sentinel reports a different master
-> verify new master PING and ROLE master
-> SendMessage
-> push-gateway-consumer publishes remote route notify
-> WebSocket client receives delivery.notify
-> reconnect to another WebSocket gateway
-> Redis-backed resume replay returns the same delivery.notify
-> PullInbox
-> AckDelivery
```

这条 smoke 证明的是“本地 Sentinel 切主完成后，Redis route / Redis-backed resume / durable PullInbox + AckDelivery 仍能恢复”。它不是完整 Redis HA 验收，不覆盖 quorum 异常、网络分区、自动停 master 触发、切主窗口内零丢失或容量结论。

## 环境

```text
commit=819c14a2df443a591175904d80451023cc1c37ad
git_dirty=false
scenario=redis-sentinel-failover
route_backend=redis
redis_mode=sentinel
sentinel_master_name=mymaster
sentinel_addrs=127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381
result=H:\NexusIM\loadtest-results\push-gateway-redis-sentinel-failover-smoke-20260609\pushgateway-summary.json
```

本地 Redis/Sentinel 拓扑：

```text
redis-ha-master      172.31.50.1:6380
redis-ha-replica-1   172.31.50.1:6381
redis-ha-replica-2   172.31.50.1:6382
redis-sentinel-1     127.0.0.1:26379
redis-sentinel-2     127.0.0.1:26380
redis-sentinel-3     127.0.0.1:26381
```

`tools/local-up-redis-sentinel.ps1 -AnnounceIP 172.31.50.1` 先完成基础验证：

```text
redis_sentinel_master=172.31.50.1:6380
redis_sentinel_peer_output_lines=56
redis_sentinel_config=OK
```

## 命令

```powershell
. .\tools\go-env.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario redis-sentinel-failover `
  -RouteBackend redis `
  -RedisMode sentinel `
  -RedisSentinelAddrs '127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381' `
  -RedisSentinelMasterName mymaster `
  -RunName push-gateway-redis-sentinel-failover-smoke-20260609
```

`redis-sentinel-failover` 场景会在 WebSocket route 已注册、membership projection 已就绪之后触发 failover。默认 failover 脚本会等待 Sentinel master 地址发生变化，并验证新 master 可 `PING` 且 `ROLE` 为 `master`，然后才继续 `SendMessage`。

## Failover 证据

summary 中记录了 failover 命令输出：

```text
sentinel_master_before=172.31.50.1:6380
sentinel_master_after=172.31.50.1:6381
```

这说明本轮不是单纯 Sentinel discovery，而是确实触发了 `SENTINEL failover mymaster` 并观察到 master 从 `6380` 切到 `6381`。如果新 master 的 `PING` 或 `ROLE master` 验证失败，runner 会直接失败，不会继续发消息。

## 关键结果

```text
success=true
server.hello resume_token=resume_58eafef3c8437dc6fde92198d8d3f646
SendMessage conversation_seq=2
delivery.notify seq=2
delivery.notify message_id=msg_daa45e8d-5935-46e7-8824-557ec83d8e71
resume replay event_id matched original notify
resume replay message_id matched original notify
PullInbox item_count=1
PullInbox max_seq=2
delivery.ack.ok last_received_seq=2
cursor_last_received_seq=2
delivery_outbox_total=2
delivery_outbox_published=2
delivery_outbox_pending=0
delivery_outbox_dlq=0
```

Consumer gateway Redis route / resume 指标：

```text
redis_route_remote_matched_sessions=1
redis_route_remote_publish_call_count=1
redis_route_remote_publish_error_count=0
redis_route_remote_enqueued_sessions=1
redis_resume_append_count=1
```

Reconnect gateway Redis resume 指标：

```text
redis_resume_replay_count=1
redis_resume_miss_count=0
redis_resume_permission_denied_count=0
```

## 解释

这条 smoke 的成功标准不是“切主期间没有任何在线通知丢失”。`push-gateway` 的 Redis route / PubSub / resume buffer 仍然是在线唤醒与短重连优化，不是可靠投递事实源。

可靠事实仍然来自：

```text
delivery-service user_inbox
-> client PullInbox
-> delivery-service AckDelivery
-> device_delivery_cursors
```

本轮更强的地方在于：切主完成后，push-gateway 的 Sentinel client 能找到新 master，consumer gateway 能继续写 Redis route/resume 状态，WebSocket gateway 能继续收到远端 notify，另一个 WebSocket gateway 能从 Redis-backed resume buffer replay 同一条 `delivery.notify`。

## 面试可讲点

可以这样讲：

```text
我把 push-gateway 的 Redis route 从单点 Redis 扩展到了 Sentinel 拓扑，并做了真实 failover smoke。
测试会在 WebSocket route 注册后触发 SENTINEL failover，等待 master 从 6380 切到 6381，确认新 master 可 PING 且 ROLE 是 master，然后继续发消息。
切主后，delivery consumer gateway 仍能通过 Sentinel 写 Redis route/resume，WebSocket gateway 能收到 notify，断线重连到另一个 gateway 还能 replay 同一条 notify。
但可靠投递不依赖 Redis；即使 online wakeup 退化，客户端仍靠 PullInbox + AckDelivery 恢复。
```

这比“我用了 Redis”更有说服力，因为它展示了：

- Redis route 和 WebSocket gateway / Kafka consumer gateway 是分离进程。
- Redis master 切换不是只停留在配置层，而是跑过真实 smoke。
- 系统把 online wakeup 和 durable delivery 分层，即 Redis 出问题也不会丢 inbox 事实。

## 未覆盖

- 未模拟 Sentinel quorum 异常。
- 未模拟网络分区。
- 未验证 Redis Cluster。
- 未验证切主窗口内所有 Pub/Sub notify 都不丢。
- 未跑容量压测。
- 未覆盖跨实例慢连接 + Sentinel failover 的组合场景。

下一步若继续增强 Redis HA 证据，建议做两类 smoke：

```text
1. 自动停当前 master，让 Sentinel 自主判定并切主。
2. 在 failover 进行中立即发送消息，验证 online wakeup 可退化但 PullInbox / AckDelivery 不丢。
```
