# Hot Group Clean Redeploy Smoke

日期：2026-06-30

## 结论

本轮完成最新 conversation-service 镜像重建、H 盘归档、Ubuntu Docker redeploy，并用
clean commit 跑通三档热点群复验。

本轮证明：

- `SEQUENCER_BLOCK` 下消息写入和成员边界事件都能通过 timeline-service seq block lease 工作；
- 热点群走 `BROADCAST_SIGNAL` 后不再写全员 `user_inbox`；
- delivery outbox 能追平到 `PENDING=0 / DLQ=0`；
- `conversation.timeline.events` 和 `im.delivery.events` consumer lag 在复验后均为 `0`；
- Prometheus、Grafana、OTel 观测入口可用。

本轮仍不声明生产容量上限；它是本地 / 三机 Docker 环境的 clean redeploy smoke 与受控放大验证。

## 环境

```text
tested_commit = d13bff6c
git_dirty = false
Windows runner = 172.31.50.1
Ubuntu Docker host = 172.31.50.2
MacBook = auxiliary
conversation-service image archive =
  H:\NexusIM\docker-images\archives\nexusim-conversation-service-d13bff6c-20260630-002306.tar
```

Ubuntu redeploy 后日志确认：

```text
conversation-service using timeline-service at timeline-service-seq-block-allocator:10780
for member boundary sequencer
conversation-service gRPC server started on 172.30.80.11:10496
```

观测入口：

```text
Prometheus: http://172.31.50.2:19091
Grafana:    http://172.31.50.2:13000
OTel:       http://172.31.50.2:14333
```

Prometheus active targets：`10 up / 0 down`。

## 运行结果

| run | group | sender | messages | rate | subscribers | send p95 / p99 | signals | Pull p95 | DB / Kafka 状态 |
| --- | ---: | ---: | ---: | ---: | ---: | --- | ---: | --- | --- |
| `hotgroup-clean-smoke-20260630-002412` | 61 | 4 | 20 | 20/s | 3 | 12.292 / 37.387 ms | 60 | 12.517 ms | `message_log=20`、`delivery_timeline=20`、`user_inbox=0`、`delivery_outbox=28`、pending=0、DLQ=0 |
| `hotgroup-clean-medium-20260630-002431` | 200 | 16 | 500 | 100/s | 20 | 11.345 / 13.054 ms | 10000 | 16.771 ms | `message_log=500`、`delivery_timeline=500`、`user_inbox=0`、`delivery_outbox=516`、pending=0、DLQ=0 |
| `hotgroup-clean-500u-1000m-20260630-002606` | 500 | 32 | 1000 | 200/s | 50 | 10.633 / 13.013 ms | 50000 | 9.436 ms | `message_log=1000`、`delivery_timeline=1000`、`user_inbox=0`、`delivery_outbox=1040`、pending=0、DLQ=0 |

三档均满足：

```text
conversation_mode = SEQUENCER_BLOCK
fanout_mode = BROADCAST_SIGNAL
send_error_count = 0
receiver_pull_error_count = 0
receiver_ack_error_count = 0
```

复验后 Kafka lag：

```text
nexusim-delivery-service-local / conversation.timeline.events lag = 0
nexusim-push-gateway-local / im.delivery.events lag = 0
```

## 解释

500 人 / 1000 消息这一档产生 5 万条 conversation signal：

```text
1000 messages * 50 conversation subscribers = 50000 signals
```

但因为当前是 `BROADCAST_SIGNAL`，delivery-service 不对 500 个成员做全员 `user_inbox`
写扩散，所以：

```text
user_inbox_rows = 0
delivery_timeline_rows = message_count
```

这说明热点群 first-stage 路径已经从“小群全员写扩散”切到“timeline + online signal +
PullInbox 动态补拉”的模型。

## 当前限制

- 本轮是本地 / 三机 Docker 受控压测，不是生产 sizing。
- Prometheus / Grafana 已可用，但本报告没有嵌入截图；后续正式容量报告应记录 dashboard
  时间窗口和趋势图。
- 当前 runner 的 sampled PullInbox 只拉首批可见页；`max_pulled_seq` 不能等同于所有 sampled
  receiver 已完整追到最新 seq。
- 下一步需要继续扩大并发、增加在线比例 / 慢连接比例，记录 PostgreSQL lock / WAL、
  timeline allocation、projection lag、push signal storm 和 ACK 追平曲线。

## 下一步

1. 增加更高 message rate / subscriber count 的 step run，找出下一瓶颈。
2. 补 Prometheus exporter：fanout mode distribution、Kafka lag by group / partition、
   delivery timeline insert rate、inbox rows per message、PostgreSQL lock / WAL。
3. 继续做 timeline virtual partition mapping、leader ownership audit 和更完整 repair workflow。
