# push-gateway Redis Sentinel master-stop smoke - 2026-06-09

## 结论

本轮在 clean commit `8ddc2fb` 上验证了本地三 Redis / 三 Sentinel 拓扑下的自动 master-stop recovery：

```text
push-gateway-ws registers route through Sentinel
-> query Sentinel current master
-> stop the current master container
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

这条 smoke 比手动 `SENTINEL failover mymaster` 更进一步：它不是主动命令 Sentinel 切主，而是停止 Sentinel 当前认定的 master 容器，让 Sentinel 自主检测故障并选出新 master。

它仍不是完整 Redis HA 验收，不覆盖 quorum 异常、网络分区、Redis Cluster、切主窗口内零 notify 丢失或容量结论。可靠投递仍以 `delivery-service user_inbox -> PullInbox -> AckDelivery` 为准。

## 环境

```text
commit=8ddc2fb
git_dirty=false
scenario=redis-sentinel-master-stop
route_backend=redis
redis_mode=sentinel
sentinel_master_name=mymaster
sentinel_addrs=127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381
result=H:\NexusIM\loadtest-results\push-gateway-redis-sentinel-master-stop-smoke-20260609\pushgateway-summary.json
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

## 命令

```powershell
. .\tools\go-env.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario redis-sentinel-master-stop `
  -RouteBackend redis `
  -RedisMode sentinel `
  -RedisSentinelAddrs '127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381' `
  -RedisSentinelMasterName mymaster `
  -RunName push-gateway-redis-sentinel-master-stop-smoke-20260609
```

`redis-sentinel-master-stop` 场景会在 WebSocket route 已注册、membership projection 已就绪之后执行临时脚本：

```text
query Sentinel current master
map current master port to local Docker container
docker stop that current master container
wait until Sentinel reports a different master
verify new master PING and ROLE master
continue SendMessage / notify / resume / PullInbox / AckDelivery
finally restart the stopped container and wait for healthy
```

## Failover 证据

summary 中记录了 stop-master 脚本输出：

```text
sentinel_master_before=172.31.50.1:6381
stopped_container=nexusim-redis-ha-replica-1
sentinel_master_after=172.31.50.1:6380
```

本轮开始时 Sentinel 当前 master 是 `172.31.50.1:6381`，所以脚本停止了对应容器 `nexusim-redis-ha-replica-1`。随后 Sentinel 报告新 master 为 `172.31.50.1:6380`，并且 runner 已验证新 master 可 `PING` 且 `ROLE` 为 `master`。

结束后恢复脚本输出：

```text
sentinel_restored_container=nexusim-redis-ha-replica-1
sentinel_restored_health=healthy
```

恢复后手工抽查拓扑：

```text
sentinel_current_master=172.31.50.1:6380
redis_6380_role=master
redis_6381_role=slave
redis_6382_role=slave
nexusim-redis-ha-replica-1 Up (healthy)
nexusim-redis-ha-replica-2 Up (healthy)
nexusim-redis-ha-master Up (healthy)
```

## 关键结果

```text
success=true
server.hello resume_token=resume_0f7cddca8a5d60cab889d25e20fd8075
SendMessage conversation_seq=2
delivery.notify seq=2
delivery.notify event_id=evt_delivery_inbox_f443693350694aa8e899e4fe66be8e9c
delivery.notify message_id=msg_7ae6ff0d-69e8-433a-b52d-7636a1e1921f
resume replay event_id matched original notify
resume replay message_id matched original notify
resume replay conversation_seq matched original notify
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
```

## 解释

这条 smoke 的成功标准不是“Redis master 停止期间没有任何 online notify 丢失”。`push-gateway` 的 Redis route、Pub/Sub 和 Redis-backed resume buffer 仍然是在线唤醒与短重连优化，不是可靠投递事实源。

可靠事实仍然来自：

```text
delivery-service user_inbox
-> client PullInbox
-> delivery-service AckDelivery
-> device_delivery_cursors
```

本轮证明的是：当前 master 容器被停止后，Sentinel 能自主选主；push-gateway 的 Sentinel client 后续能发现新 master；consumer gateway 能继续写 Redis route/resume；WebSocket gateway 能继续收到远端 notify；另一个 WebSocket gateway 能从 Redis-backed resume buffer replay 同一条 `delivery.notify`。

## 面试可讲点

可以这样讲：

```text
我没有只做 Redis Sentinel 配置，而是做了真实 master-stop smoke。
测试先通过 Sentinel 查询当前 master，再停掉这个 master 容器，等待 Sentinel 自主选出新 master，并验证新 master 的 PING 和 ROLE。
切主后继续走完整 IM 链路：SendMessage、outbox、Kafka、delivery projection、im.delivery.events、Redis route、WebSocket notify、跨 gateway resume replay、PullInbox、AckDelivery。
即使 Redis route 是在线唤醒层，可靠投递仍落在 delivery-service 的 durable inbox 和 ACK cursor。
```

这能说明系统不是单机 WebSocket demo，而是把在线连接、Kafka consumer、Redis route、Sentinel discovery、resume buffer 和 durable delivery read model 分层实现。

## 未覆盖

- 未模拟 Sentinel quorum 异常。
- 未模拟网络分区。
- 未验证 Redis Cluster。
- 未验证切主窗口内所有 Pub/Sub notify 都不丢。
- 未跑容量压测。
- 未覆盖跨实例慢连接 + Sentinel failover 的组合场景。

下一步如果继续增强 Redis HA 证据，应优先做小而清晰的故障 smoke，而不是重型矩阵：

```text
1. Sentinel quorum / 网络分区语义 smoke。
2. failover 窗口内 online notify 可退化但 PullInbox / AckDelivery 不丢的 smoke。
```
