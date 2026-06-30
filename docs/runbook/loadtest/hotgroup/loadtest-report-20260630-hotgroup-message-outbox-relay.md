# Hot Group Message Outbox Relay Step

日期：2026-06-30

## 结论

本轮针对上一轮热点群压测中暴露的 `message_outbox -> Kafka conversation.timeline.events`
积压做优化并复验。

代码提交：

```text
tested_commit = 0a1395c
git_dirty = false
image_archive = H:\NexusIM\docker-images\archives\nexusim-message-service-0a1395c3-20260630-125317.tar
Ubuntu Docker host = 172.31.50.2
Windows runner = 172.31.50.1
```

本轮证明：

- message-service outbox relay 已支持 4 worker conversation-sharded batch publish；
- 同一 `tenant_id + conversation_id` 由同一 shard worker 处理，保持 Kafka key / conversation 顺序；
- 同一 conversation 内某条消息 publish / payload 失败时，后续同 conversation ready 行保持 `PENDING`
  且不递增 retry，避免越过失败 seq；
- 新增 `message_outbox` conversation/version partial indexes 后，ready query 不再只能每轮取同一
  hot conversation 的一条 row；
- 上一轮失败的 1000 人 / 2000 消息 / 400 msg/s 档位已通过；
- 1000 人 / 4000 消息 / 800 msg/s 档位也通过，`message_outbox_pending=0`、
  `delivery_outbox_pending=0`、Kafka lag=0；
- 更高档位失败点已经迁移到 push-gateway conversation signal 写出 / 压测端读取，不是
  message outbox、delivery projection、delivery outbox、Kafka consumer lag 或 PostgreSQL durable 写入。

本轮仍是本地 / 三机 Docker 实验，不声明生产容量上限。

## 实现变更

### message outbox relay

`message-service outbox-relay` 当前 runtime：

```text
NEXUSIM_OUTBOX_WORKERS=4
NEXUSIM_OUTBOX_BATCH_SIZE=500
NEXUSIM_OUTBOX_PUBLISH_BATCH_ENABLED=true
NEXUSIM_OUTBOX_POLL_INTERVAL=100ms
NEXUSIM_OUTBOX_FAILURE_BACKOFF=100ms
```

Ubuntu relay 日志确认：

```text
message-service outbox relay started workers=4 batch_size=500 publish_batch_enabled=true poll_interval=100ms failure_backoff=100ms
```

### PostgreSQL indexes

新增迁移：

```text
migrations/postgres/message/000006_message_outbox_ready_indexes.sql
```

新增索引：

```text
idx_message_outbox_pending_conversation_version
idx_message_outbox_blocking_conversation_version
```

远端 PostgreSQL 已确认两者存在。

## 压测结果

| run | group | sender | messages | rate | subscribers | result | send p95 / p99 | signals | Pull p95 | async state |
| --- | ---: | ---: | ---: | ---: | ---: | --- | --- | ---: | --- | --- |
| `hotgroup-message-relayopt-1000u-2000m-400qps-20260630-125922` | 1000 | 64 | 2000 | 400/s | 100 | pass | 14.699 / 17.571 ms | 200000 | 14.044 ms | message pending=0、delivery pending=0、DLQ=0 |
| `hotgroup-message-relayopt-1000u-4000m-800qps-20260630-130153` | 1000 | 96 | 4000 | 800/s | 100 | pass | 12.685 / 15.789 ms | 400000 | 22.179 ms | message pending=0、delivery pending=0、DLQ=0 |
| `hotgroup-message-relayopt-1000u-8000m-1200qps-20260630-131501` | 1000 | 128 | 8000 | 1200/s | 150 | fail | 13.204 / 15.987 ms | 0 / 1200000 observed by runner | n/a | message pending=0、delivery pending=0、DLQ=0 |
| `hotgroup-message-relayopt-2000u-8000m-1500qps-20260630-130521` | 2000 | 128 | 8000 | 1500/s | 200 | fail | 13.830 / 16.356 ms | 0 / 1600000 observed by runner | n/a | message pending=0、delivery pending=0、DLQ=0 |
| `hotgroup-message-relayopt-postfail-smoke-20260630-132237` | 61 | 4 | 20 | 20/s | 3 | pass | smoke | 60 | smoke | confirms chain recovered |

两个失败档位的共同点：

```text
SendMessage success = message_count
message_log_count = message_count
delivery_timeline_rows = message_count
user_inbox_rows = 0
message_outbox_pending = 0
delivery_outbox_pending = 0
message_outbox_dlq = 0
delivery_outbox_dlq = 0
push-gateway Kafka group lag = 0
```

失败原因均为：

```text
wait conversation signals: timed out waiting for conversation signals
```

## 新瓶颈判断

本轮瓶颈已经从 `message_outbox -> conversation.timeline.events` 移走。

证据：

- 800 msg/s 档位能完成 4000 条 message facts、4000 条 delivery timeline、400000 条 conversation signal；
- 1200 / 1500 msg/s 失败档位仍然完成所有 message durable write、message outbox publish、
  delivery projection、delivery outbox publish；
- `nexusim-push-gateway-local / im.delivery.events` 在失败后 lag=0；
- push-gateway `/debug/metrics` 显示累计 `conversation_signal_matched_count` /
  `conversation_signal_enqueued_count` 超过 300 万，Redis subscriber 已收到并入队大量 signal；
- runner 侧在高压档位观测到 0 条 conversation signal，说明当前待查点在 WebSocket 写出、
  客户端读取、session queue / resume buffer 语义或 runner signal accounting。

因此下一步不应继续优先调 message relay，而应收敛：

```text
push-gateway conversation signal writer throughput
loadtest/hotgroup signal reader accounting
per-session queue / resume buffer pressure metrics
Prometheus dashboard 中 push signal egress 与 client observed signal 的差值
```

## 当前限制

- 本报告没有嵌入 Grafana 截图；低敏原始 summary 和 push debug JSON 已写入 H 盘结果目录。
- 高压失败档位只说明本地三机环境当前 push signal 观测链路过载，不代表生产上限。
- runner 当前只报告最终 observed signal count，尚不能区分 WebSocket writer 未写出、客户端没读到、
  或 reader goroutine accounting 丢失。

## 原始证据

```text
H:\NexusIM\loadtest-results\hotgroup-message-relayopt-1000u-2000m-400qps-20260630-125922
H:\NexusIM\loadtest-results\hotgroup-message-relayopt-1000u-4000m-800qps-20260630-130153
H:\NexusIM\loadtest-results\hotgroup-message-relayopt-1000u-8000m-1200qps-20260630-131501
H:\NexusIM\loadtest-results\hotgroup-message-relayopt-2000u-8000m-1500qps-20260630-130521
H:\NexusIM\loadtest-results\hotgroup-message-relayopt-postfail-smoke-20260630-132237
```

附加诊断：

```text
H:\NexusIM\loadtest-results\hotgroup-message-relayopt-1000u-8000m-1200qps-20260630-131501\push-debug-after.json
H:\NexusIM\loadtest-results\hotgroup-message-relayopt-1000u-8000m-1200qps-20260630-131501\kafka-push-group-after.txt
```

## 下一步

1. 已实现第一阶段 push-gateway writer flush 指标和 `loadtest/hotgroup` per-connection
   signal summary；下一步重建最新镜像并 redeploy。
2. 再跑 800 -> 1000 -> 1200 msg/s 的 push-focused step，定位 WebSocket writer、client
   read loop、session queue 还是 Redis route / Kafka consumer。
3. 将 `/metrics` 中 `nexusim_push_gateway_ws_writer_events_total` 和 runner
   `push.subscriber_signals[]` 一起写入新报告。
4. 必要时把 push conversation signal 改成按 conversation subscription bucket 批量广播，
   或引入更明确的 online signal backpressure / sampling 策略。
