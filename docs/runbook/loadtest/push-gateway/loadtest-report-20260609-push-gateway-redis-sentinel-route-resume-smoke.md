# push-gateway Redis Sentinel route / resume smoke - 2026-06-09

## 结论

本轮在 clean commit `7bc35a5` 上验证了 `push-gateway` 使用 Redis Sentinel 发现 master 后，仍能完成跨实例 route / resume 最小链路：

```text
push-gateway-ws
-> Redis Sentinel discovers master
-> Redis route / PubSub
-> push-gateway-consumer
-> Redis-backed resume append
-> reconnect to another push-gateway ws
-> Redis-backed resume replay
-> PullInbox
-> AckDelivery
```

这证明的是 Sentinel discovery 正常路径，不是 Redis HA / failover 验收。没有在本轮停 master、触发 `SENTINEL failover`、模拟网络分区或验证生产级高可用。

## 环境

```text
commit=7bc35a58a56dd70b7c043303097dbe72ba1cb9d3
git_dirty=false
scenario=cross-instance-resume
route_backend=redis
redis_mode=sentinel
sentinel_master_name=mymaster
sentinel_addrs=127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381
sentinel_reported_master=172.31.50.1:6380
result=H:\NexusIM\loadtest-results\push-gateway-redis-sentinel-route-resume-final-20260609\pushgateway-summary.json
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

Sentinel master discovery 复核。`tools/local-up-redis-sentinel.ps1` 会设置 `NEXUSIM_REDIS_SENTINEL_ANNOUNCE_IP`，启动三 Redis / 三 Sentinel，并验证 Sentinel 返回的 master 可从宿主机 TCP 连接、可从 Sentinel 容器内 `PING`：

```text
SENTINEL get-master-addr-by-name mymaster -> 172.31.50.1 6380
```

## 命令

```powershell
. .\tools\go-env.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario cross-instance-resume `
  -RouteBackend redis `
  -RedisMode sentinel `
  -RedisSentinelAddrs '127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381' `
  -RedisSentinelMasterName mymaster `
  -RunName push-gateway-redis-sentinel-route-resume-final-20260609
```

## 关键结果

```text
success=true
server.hello resume_token=resume_d360eb464ac6488a46a1591b6ce19ebc
SendMessage conversation_seq=2
delivery.notify seq=2
resume replay event_id matched original notify
resume replay message_id matched original notify
PullInbox item_count=1
PullInbox max_seq=2
delivery.ack.ok last_received_seq=2
delivery_outbox_total=2
delivery_outbox_published=2
delivery_outbox_pending=0
delivery_outbox_dlq=0
```

Consumer gateway Redis 指标：

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

## 说明

这条 smoke 和之前 single Redis cross-instance resume 的业务语义相同，差异是 push-gateway 不再直接连接固定 Redis 地址，而是通过 Sentinel 发现 master。

可靠投递仍不依赖 Redis：

```text
Redis route / PubSub / resume buffer = online wakeup and short reconnect optimization
delivery-service PullInbox + AckDelivery = reliable delivery truth
```

因此，Sentinel 正常路径通过以后，面试里可以说：

```text
我把 push-gateway 的 Redis route 从单点地址扩展成 single / Sentinel 两种拓扑。
Sentinel 模式下，WebSocket gateway 和 delivery consumer gateway 仍能通过 Redis route 找到对方，并且跨实例 reconnect 可以从 Redis-backed resume buffer replay 同一条 notify。
但我没有把它说成生产级 Redis HA；真正 failover 仍需要单独停 master / 触发 Sentinel failover 验证。
```

## 未覆盖

- 未触发 `SENTINEL failover mymaster`。
- 未停止 `redis-ha-master` 验证自动切主。
- 未验证 master 切换期间 Pub/Sub 丢失窗口。
- 未验证 Sentinel quorum 异常或网络分区。
- 未验证 Redis Cluster。
- 未做容量压测。

下一步应补 Redis Sentinel failover smoke：在已有 WebSocket route 注册后触发 master 切换，验证 failover 后新连接、route publish、resume replay 或 `PullInbox` recovery 都能恢复。
