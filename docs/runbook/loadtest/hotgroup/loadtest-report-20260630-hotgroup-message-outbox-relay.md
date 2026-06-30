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

## 2026-06-30 后续诊断补充

### push signal writer 观测

提交 `7ec899d8 feat: add push signal pressure metrics` 后，push-gateway WebSocket writer
已暴露低敏写出指标，`loadtest/hotgroup` 也会记录每个 conversation subscriber 的
signal 数、最大 seq、首帧 / 末帧耗时、完成状态和 read error。

在 Ubuntu Docker redeploy push-gateway 后，诊断 run：

```text
run_name = hotgroup-readfanout-6000-400qps-100sub-20260630-2258
commit = 7ec899d
git_dirty = true
group_size = 6000
message_count = 1000
message_rate = 400/s
subscriber_count = 100
fanout_mode = READ_FANOUT
```

结果：

```text
success = true
conversation_signal_count = 100000
subscriber completed = 100 / 100
send_p95_ms = 17.75
send_p99_ms = 22.66
PullInbox p95_ms = 14.05
user_inbox_rows = 0
delivery_outbox_pending = 0
Kafka lag = 0
```

该 run 证明 READ_FANOUT 路径在 6000 人 / 400 msg/s / 100 个在线订阅者下可以完成
online signal、PullInbox 和 ACK 抽样。它的限制是：当时工作区包含 delivery outbox
query 未提交改动，因此只能作为诊断证据；后续需要在 clean commit 镜像下复压，才能写入
可复现实验基线。

原始目录：

```text
H:\NexusIM\loadtest-results\hotgroup-readfanout-6000-400qps-100sub-20260630-2258
```

### delivery_outbox ready query 瓶颈

HYBRID 诊断 run：

```text
run_name = hotgroup-hybrid-1000-400qps-pushmetrics-20260630-2232
group_size = 1000
message_count = 1000
message_rate = 400/s
fanout_mode = HYBRID_FANOUT
```

失败原因不是 SendMessage，也不是 message outbox。该 run 产生百万级 per-user
`delivery_outbox` row 后，旧 ready query 对每个候选 row 都用 anti-join 检查低版本
blocker。`EXPLAIN ANALYZE` 显示旧查询在约 100 万 pending row 下取 500 行约 24s，
会让 relay 发布速度被 PostgreSQL ready query 吃掉。

本轮将 ready query 改为 per-conversation frontier 形态：

```text
frontier = 每个 tenant + conversation 当前最低 PENDING / DLQ aggregate_version
current = 只锁 frontier version 中已经到 available_at / next_retry_at 的 PENDING row
```

语义：

- 同一个 conversation 的高 `aggregate_version` 仍会被低版本 PENDING / DLQ 阻塞；
- 同一个 `aggregate_version` 内的多条 user-level delivery event 可以被多 worker 用
  `FOR UPDATE SKIP LOCKED` 分批发布；
- 如果最低版本尚未到 retry 时间，后续高版本不会被越过；
- 这是 first-stage 查询优化，不是完整 outbox frontier/progress 表。

本地 Docker 配置同时把 delivery outbox relay worker 提高到 8：

```text
NEXUSIM_DELIVERY_OUTBOX_WORKERS=8
NEXUSIM_DELIVERY_OUTBOX_BATCH_SIZE=500
NEXUSIM_DELIVERY_KAFKA_BATCH_SIZE=500
```

后续判断：

- 如果目标是继续支撑千人级 HYBRID per-user materialized outbox，应继续评估显式
  outbox frontier / progress 表，减少每批扫描成本；
- 如果目标是热点群 / 大群吞吐，应优先让策略进入 READ_FANOUT / BROADCAST_SIGNAL，
  避免把百万级 per-user outbox 当作主路径；
- Kafka / Redis 只负责事件传播和在线 signal，不应替代业务 fanout 策略。

### clean commit READ_FANOUT 阶梯复压

提交 `01b2a70e fix: speed delivery outbox frontier fetch` 已推送并重建
`nexusim/delivery-service:local`，镜像归档：

```text
H:\NexusIM\docker-images\archives\nexusim-delivery-service-01b2a70e-frontier-20260630-231103.tar
```

Ubuntu Docker 已重新创建：

```text
nexusim-delivery-service-grpc
nexusim-delivery-service-timeline-consumer
nexusim-delivery-service-outbox-relay
```

relay 启动日志：

```text
delivery-service outbox relay started topic=im.delivery.events workers=8 batch_size=500 kafka_batch_size=500 kafka_batch_timeout=20ms
```

clean commit `01b2a70` 的 READ_FANOUT 阶梯复压结果：

| run | target msg/s | messages | senders | subscribers | send p95 | send p99 | signals | slowest signal drain ms | completed subscribers | Pull p95 | message pending | delivery pending |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `hotgroup-readfanout-6000-400qps-clean-01b2a70e-20260630-2313` | 400 | 1000 | 32 | 100 | 17.73 | 20.04 | 100000 | 35541 | 100 | 22.56 | 0 | 0 |
| `hotgroup-readfanout-6000-800qps-clean-01b2a70e-20260630-2316` | 800 | 2000 | 64 | 100 | 18.31 | 22.13 | 200000 | 70744 | 100 | 16.73 | 0 | 0 |
| `hotgroup-readfanout-6000-1200qps-clean-01b2a70e-20260630-2320` | 1200 | 3000 | 96 | 100 | 18.25 | 21.91 | 300000 | 104999 | 100 | 37.90 | 0 | 0 |
| `hotgroup-readfanout-6000-2000qps-clean-01b2a70e-20260630-2325` | 2000 | 5000 | 128 | 100 | 17.55 | 20.63 | 500000 | 173888 | 100 | 24.02 | 0 | 0 |
| `hotgroup-readfanout-6000-4000qps-clean-01b2a70e-20260630-2330` | 4000 | 5000 | 192 | 100 | 19.17 | 23.35 | 500000 | 176197 | 100 | 24.92 | 0 | 0 |
| `hotgroup-readfanout-6000-8000qps-clean-01b2a70e-20260630-2336` | 8000 | 5000 | 256 | 100 | 18.54 | 22.41 | 500000 | 176554 | 100 | 26.93 | 0 | 0 |

当前判断：

- 目标 8000 msg/s、5000 条消息的 READ_FANOUT burst 没有打穿 SendMessage、message outbox、
  delivery projection、delivery outbox 或 Kafka consumer；
- `delivery_outbox_pending=0`、Kafka `nexusim-delivery-service-local` lag=0、push writer
  `delivery_notify_write_error_count=0`；
- 500000 条 signal 在 100 个 WebSocket subscriber 上全部读完，但最慢 subscriber drain
  约 176s，说明当前更像 online signal drain / 压测端读取容量问题，而不是事实写入容量问题；
- 后续如果要继续找上限，应优先增加 subscriber 数或总 signal 数，并配套 Prometheus /
  Grafana 时间窗口，而不是只继续提高 message-rate。

原始目录：

```text
H:\NexusIM\loadtest-results\hotgroup-readfanout-6000-400qps-clean-01b2a70e-20260630-2313
H:\NexusIM\loadtest-results\hotgroup-readfanout-6000-800qps-clean-01b2a70e-20260630-2316
H:\NexusIM\loadtest-results\hotgroup-readfanout-6000-1200qps-clean-01b2a70e-20260630-2320
H:\NexusIM\loadtest-results\hotgroup-readfanout-6000-2000qps-clean-01b2a70e-20260630-2325
H:\NexusIM\loadtest-results\hotgroup-readfanout-6000-4000qps-clean-01b2a70e-20260630-2330
H:\NexusIM\loadtest-results\hotgroup-readfanout-6000-8000qps-clean-01b2a70e-20260630-2336
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

1. 补本轮阶梯复压的 Prometheus / Grafana 或 debug metrics 时间窗口记录。
2. 继续扩大 subscriber 数或总 signal 数，定位 online signal drain 的真实上限。
3. 必要时把 push conversation signal 改成按 conversation subscription bucket 批量广播，
   或引入更明确的 online signal backpressure / sampling 策略。
