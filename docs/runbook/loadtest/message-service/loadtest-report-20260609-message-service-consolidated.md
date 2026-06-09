# NexusIM message-service Loadtest Consolidated Report

本文整合 `message-service` 第一阶段所有压测报告。原始报告仍然保留，本文件只做统一索引、阶段结论和面试叙事整理。

## 1. 覆盖范围

本阶段只覆盖已经真实落地的链路：

```text
client / loadtest
-> message-service gRPC
-> SendMessage use case
-> PostgreSQL local transaction
-> conversation_seq
-> message_log
-> conversation_timeline_events
-> message_outbox
-> outbox relay
-> Kafka conversation.timeline.events
```

本阶段不覆盖：

```text
conversation-service 真实 RPC
delivery-service inbox fanout
push-gateway WebSocket
timeline-service hotspot sequencer
客户端 UI
RAG / Agent
```

## 2. 总体结论

`message-service` 第一阶段不是 demo endpoint，而是已经跑通了真实写入链路：

- gRPC API、app 用例、domain command hash、PostgreSQL 本地事务、outbox relay、Kafka publish path 均已落地。
- 普通会话 `SendMessage` 能保证 `conversation_seq + message_log + conversation_timeline_events + message_outbox` 同事务写入。
- outbox relay 支持 retry / DLQ / 多 worker / batch mark published / Kafka PublishBatch。
- 压测中持续记录了 commit、dirty 状态、p95/p99、logical latency、outbox backlog、Kafka publish、PostgreSQL pool、relay active/idle 指标。
- 当前性能问题已经从“链路是否可用”推进到“如何在有限 PostgreSQL/Kafka/本机资源下做准入、背压和重试策略”。

当前最稳的本机候选不是生产默认值，只是 Windows 本机开发环境下的阶段结论：

```text
PG_MAX_CONNS=64
relay workers=8
outbox batch size=100
PublishBatch=true
AdaptiveMaxInFlight=64
```

该组合在 60s repeat 中：

| VU | accepted RPS | success p99 | logical p99 | outbox pending |
| ---: | ---: | ---: | ---: | ---: |
| 1200 | 1922.23 | 63.58ms | 2200.80ms | 0 |
| 1600 | 1926.97 | 63.35ms | 2203.44ms | 0 |

解释：

- `success p99` 代表一次成功写入的延迟，已经被 token gate 控制在几十毫秒级。
- `logical p99` 包含客户端收到 `SERVICE_OVERLOADED` 后按 `RetryInfo` 等待再重试的时间，所以约 2.2s。
- 因此后续优化重点不是继续压硬件，而是调整客户端 retry 策略，并继续开发其它微服务。

## 3. 阶段演进

### 3.1 基线：真实链路先跑通

第一份正式报告是 [loadtest-report-20260609.md](./loadtest-report-20260609.md)。

目标是验证真实链路：

```text
SendMessage -> PostgreSQL -> outbox -> Kafka
```

关键结论：

- Docker 资源矩阵和 Windows+Mac 分布式客户端都能跑通。
- `16 CPU / 23GB / relay workers=8 / 1200 VU` 观察到约 `2493 rps`、p99 `736.28ms`、outbox pending 0。
- `1600 VU` 或更高时开始进入资源争用区，p99 波动明显。
- 盲目增加 relay worker 不一定变好：`workers=16` 反而会放大 DB 争用。

第一轮瓶颈排查方法：

1. 对比 request p99、send_message p99、repository_append p99。
2. 单独看 commit、conversation_seq、Kafka publish、outbox pending。
3. 如果某个内部指标 p99 远低于 request p99，则排除它是主瓶颈。

当时的证据链：

```text
request p99 ~= send_message p99
send_message p99 ~= repository_append p99
repository_commit p99 远小于 request p99
conversation_seq p99 远小于 request p99
Kafka publish p99 远小于 request p99
outbox pending = 0
```

因此第一轮定位为：瓶颈收敛到 repository append 内部，但不是 Kafka、outbox relay、commit 或 conversation_seq 单步递增。

### 3.2 细分 PostgreSQL pool：证明不是 CPU 不够

报告：

- [loadtest-report-20260609-pgpool-multi-instance.md](./loadtest-report-20260609-pgpool-multi-instance.md)
- [loadtest-report-20260609-multi-instance-budget.md](./loadtest-report-20260609-multi-instance-budget.md)
- [loadtest-report-20260609-postgres-loadtest-profile.md](./loadtest-report-20260609-postgres-loadtest-profile.md)

新增指标：

```text
repository_pool_acquire_latency_ms
repository_tx_begin_latency_ms
repository_begin_latency_ms
repository_insert_message_latency_ms
repository_insert_timeline_latency_ms
repository_insert_outbox_latency_ms
repository_commit_latency_ms
```

关键发现：

- 高并发下 request p99 贴近 `repository_pool_acquire`，而不是 SQL `BEGIN` 本身。
- 提高 PG 连接池能提升部分吞吐，但继续加大到 `PG_MAX_CONNS=128` 会放大 commit / WAL / outbox pressure。
- 多个 message-service 实例没有降低 p99，因为它们共享同一个 PostgreSQL，压力最终集中到连接和写入路径。
- PostgreSQL wait_event 采样开始出现 `LWLock:WALWrite`、`LWLock:WALInsert`、`BufferContent`、`CheckpointWriteDelay`，说明高并发写入下 WAL/checkpoint 进入瓶颈视野。

这一步可以在面试里讲成：

> 我没有直接说“CPU 不够”，而是把 request p99 拆到 gRPC handler、use case、repository、pool acquire、tx begin、SQL insert、commit、Kafka publish、outbox backlog。证据显示尾延迟主要贴在 pgxpool acquire / PostgreSQL 写入压力，而不是 Kafka 或业务代码。

### 3.3 Backpressure：从排队超时改成快速失败

报告：

- [loadtest-report-20260609-backpressure.md](./loadtest-report-20260609-backpressure.md)
- [loadtest-report-20260609-backpressure-onoff.md](./loadtest-report-20260609-backpressure-onoff.md)
- [loadtest-report-20260609-backpressure-gradient.md](./loadtest-report-20260609-backpressure-gradient.md)

新增机制：

```text
MESSAGE_ERROR_CODE_SERVICE_OVERLOADED
gRPC codes.Unavailable
MessageError.retryable=true
RetryInfo
repository-level PG pool backpressure
```

关键结论：

- backpressure 能把过载请求快速返回 `SERVICE_OVERLOADED`。
- 不能只看 overall p99，因为大量快速失败会让混合 p99 变低。
- 必须拆开看：

```text
overall p99
success p99
error p99
accepted RPS
overload rate
logical success rate
```

这一步的工程价值：

- 系统不会无限把请求堆进 PostgreSQL connection pool。
- 客户端拿到 retryable 错误和 RetryInfo，具备退避重试基础。
- 压测报告开始区分 attempt-level 指标和 logical-level 用户请求指标。

### 3.4 Client retry：把过载语义变成客户端行为

报告：

- [loadtest-report-20260609-client-retry.md](./loadtest-report-20260609-client-retry.md)
- [loadtest-report-20260609-logical-latency-smoke.md](./loadtest-report-20260609-logical-latency-smoke.md)

新增压测能力：

```text
--retry-overloaded
--max-retries
--retry-jitter
logical_request_count
logical_success_rate
logical_p99_ms
retry_attempt_count
retry_delay_count
retry_delay_p95_ms
```

关键结论：

- 开启 retry 后，不能把 `request_count` 当消息吞吐，因为它包含重试 attempt。
- 要区分：

```text
logical_request_count = 用户层消息数
request_count = gRPC attempt 数
logical p99 = 用户感知端到端等待
attempt p99 = 单次 RPC 延迟
```

这一步让后续 adaptive admission 的调参有了正确口径。

### 3.5 Outbox relay：解决 Kafka publish 后的积压问题

报告：

- [loadtest-report-20260609-outbox-batchmark.md](./loadtest-report-20260609-outbox-batchmark.md)
- [loadtest-report-20260609-relay-metrics-smoke.md](./loadtest-report-20260609-relay-metrics-smoke.md)
- [loadtest-report-20260609-outbox-batchsize-smoke.md](./loadtest-report-20260609-outbox-batchsize-smoke.md)
- [loadtest-report-20260609-outbox-publishbatch-smoke.md](./loadtest-report-20260609-outbox-publishbatch-smoke.md)
- [loadtest-report-20260609-publishbatch-metrics-smoke.md](./loadtest-report-20260609-publishbatch-metrics-smoke.md)
- [loadtest-report-20260609-publishbatch-formal.md](./loadtest-report-20260609-publishbatch-formal.md)
- [loadtest-report-20260609-outbox-batch-worker-matrix.md](./loadtest-report-20260609-outbox-batch-worker-matrix.md)
- [loadtest-report-20260609-outbox-active-idle-metrics-smoke.md](./loadtest-report-20260609-outbox-active-idle-metrics-smoke.md)
- [loadtest-report-20260609-outbox-candidate-repeat.md](./loadtest-report-20260609-outbox-candidate-repeat.md)

新增机制：

```text
batch mark PUBLISHED
Kafka PublishBatch
outbox_process_ready_active_latency_ms
outbox_process_ready_idle_latency_ms
outbox_fetched_per_call
kafka_publish_records_per_call
```

关键结论：

- outbox relay 不能只看混合 `outbox_process_ready_latency_ms`，因为 stats_wait 阶段会混入 idle 样本。
- 需要拆 active / idle：

```text
outbox_process_ready_active_latency_ms
outbox_process_ready_idle_latency_ms
outbox_fetched_per_call
```

- 当前本机 relay 基线候选是：

```text
NEXUSIM_OUTBOX_BATCH_SIZE=100
NEXUSIM_OUTBOX_WORKERS=8
PublishBatch=true
```

候选重复验证结论：

- batch 100 / workers 8 能稳定追平 outbox。
- batch 500 不是稳定更优，部分 run 的 success p99 被拉高。
- 增大 worker 数不一定有收益，过多 worker 会增加 DB 争用。

### 3.6 Adaptive admission：从固定 backpressure 进入可解释准入

报告：

- [loadtest-report-20260609-adaptive-limit-smoke.md](./loadtest-report-20260609-adaptive-limit-smoke.md)
- [loadtest-report-20260609-adaptive-limit-onoff.md](./loadtest-report-20260609-adaptive-limit-onoff.md)
- [loadtest-report-20260609-recent-metrics-smoke.md](./loadtest-report-20260609-recent-metrics-smoke.md)
- [loadtest-report-20260609-adaptive-hysteresis-smoke.md](./loadtest-report-20260609-adaptive-hysteresis-smoke.md)
- [loadtest-report-20260609-adaptive-retry-hint-smoke.md](./loadtest-report-20260609-adaptive-retry-hint-smoke.md)
- [loadtest-report-20260609-adaptive-threshold-v1.md](./loadtest-report-20260609-adaptive-threshold-v1.md)
- [loadtest-report-20260609-adaptive-best-repeat.md](./loadtest-report-20260609-adaptive-best-repeat.md)
- [loadtest-report-20260609-adaptive-poolp95-v1.md](./loadtest-report-20260609-adaptive-poolp95-v1.md)
- [loadtest-report-20260609-adaptive-inflight-v1.md](./loadtest-report-20260609-adaptive-inflight-v1.md)

新增机制：

```text
app AdmissionPort
SERVICE_OVERLOADED dynamic RetryInfo
recent 4096 sample window
hysteresis release thresholds
MaxInFlight token gate
```

阶段演进：

1. 极端阈值 smoke 证明 app 入口拒绝不会写 DB/outbox。
2. recent metrics 避免累计样本让阈值粘住。
3. hysteresis 避免过载状态频繁抖动。
4. dynamic RetryInfo 让服务端可以根据过载原因给客户端提示。
5. 纯 pool p95 阈值矩阵发现 accepted RPS 过低，说明策略过度保护。
6. `MaxInFlight` token gate 把问题从“DB pool acquire 等待”转为“retry 策略和用户层等待”。

当前最有价值的结果：

```text
MaxInFlight=64
1200 VU: accepted RPS=1922.23, success p99=63.58ms, logical p99=2200.80ms, pending=0
1600 VU: accepted RPS=1926.97, success p99=63.35ms, logical p99=2203.44ms, pending=0
```

解释：

- token gate 能保护成功写入路径。
- logical p99 仍高，是因为客户端等待 RetryInfo 后重试。
- 这说明后续应该转向 retry 策略和系统完整度，而不是继续用有限本机硬件做大规模矩阵。

## 4. 瓶颈排查方法总结

本阶段不是只跑一个 p99 数字，而是逐步排查：

1. **先确认链路正确性**
   `SendMessage -> DB tx -> outbox -> Kafka` 必须真实跑通，不能压 toy endpoint。

2. **拆请求路径**
   从 request p99 拆到 handler、use case、repository append、pool acquire、tx begin、insert、commit、seq、Kafka、outbox。

3. **排除非主因**
   如果 Kafka p99、seq p99、commit p99 远低于 request p99，就不把它们当主瓶颈。

4. **验证假设**
   调整 PG pool、多实例、PostgreSQL profile、relay worker、batch size、backpressure、adaptive token，看指标是否按预期变化。

5. **区分成功和失败**
   backpressure 后必须拆 `success p99` 和 `error p99`，否则快速失败会让 overall p99 看起来很好。

6. **区分 attempt 和 logical**
   开启客户端 retry 后，attempt p99 不等于用户感知延迟；必须看 logical p99。

## 5. 面试可讲要点

可以按下面这个顺序讲：

1. 我先实现了一个真实 IM 写消息链路，不是 mock endpoint：

```text
gRPC -> DDD use case -> PostgreSQL 本地事务 -> outbox -> Kafka
```

2. 数据一致性上，普通会话下 `seq + message + timeline + outbox` 同库同事务提交，Kafka 只由 outbox relay 异步发布，避免业务事务内直接 publish Kafka。

3. 性能排查不是看单个 p99，而是把链路拆成多个阶段指标，定位到最贴近 request p99 的阶段。

4. 第一轮发现 Kafka、outbox、conversation_seq 都不是主要瓶颈，p99 主要贴在 repository append。

5. 继续拆 repository 后，发现主要等待在 PostgreSQL pool acquire / 写入并发，而不是 SQL BEGIN 本身。

6. 盲目加 CPU、内存、relay worker 或 message-service 实例都不能根治，因为共享 PostgreSQL 仍是瓶颈。

7. 后续做了 outbox relay 优化：batch mark published、Kafka PublishBatch、active/idle relay 指标、batch/worker 矩阵。

8. 做了背压和 adaptive admission：过载时返回 `SERVICE_OVERLOADED + RetryInfo`，客户端可重试，服务端不会把请求无限堆进 DB pool。

9. 最后引入 `MaxInFlight` token gate，把成功写入 p99 控制到几十毫秒，outbox 也能清零；剩下的 logical p99 主要来自客户端重试等待。

10. 这说明系统已经从“能不能跑”进入“如何在有限资源下做容量治理”的阶段，下一步应该补其它微服务，而不是继续压榨单机。

## 6. 原始报告索引

| 报告 | 作用 |
| --- | --- |
| [loadtest-report-20260609.md](./loadtest-report-20260609.md) | 第一轮真实链路、Docker 资源矩阵、Windows+Mac 双客户端、初始瓶颈排查 |
| [loadtest-report-20260609-pgpool-multi-instance.md](./loadtest-report-20260609-pgpool-multi-instance.md) | PG pool 梯度、多实例诊断、repository begin/append p99 贴合 |
| [loadtest-report-20260609-multi-instance-budget.md](./loadtest-report-20260609-multi-instance-budget.md) | 固定总 PG 连接预算与固定每实例预算对照 |
| [loadtest-report-20260609-postgres-loadtest-profile.md](./loadtest-report-20260609-postgres-loadtest-profile.md) | PostgreSQL loadtest profile、wait_event、WAL/checkpoint 观察 |
| [loadtest-report-20260609-backpressure.md](./loadtest-report-20260609-backpressure.md) | PG pool backpressure smoke |
| [loadtest-report-20260609-backpressure-onoff.md](./loadtest-report-20260609-backpressure-onoff.md) | backpressure on/off，拆 success/error p99 |
| [loadtest-report-20260609-backpressure-gradient.md](./loadtest-report-20260609-backpressure-gradient.md) | MinAvailableConns 梯度 |
| [loadtest-report-20260609-client-retry.md](./loadtest-report-20260609-client-retry.md) | 客户端遵守 RetryInfo 的重试矩阵 |
| [loadtest-report-20260609-logical-latency-smoke.md](./loadtest-report-20260609-logical-latency-smoke.md) | logical latency 指标验证 |
| [loadtest-report-20260609-outbox-batchmark.md](./loadtest-report-20260609-outbox-batchmark.md) | 批量 mark PUBLISHED |
| [loadtest-report-20260609-relay-metrics-smoke.md](./loadtest-report-20260609-relay-metrics-smoke.md) | outbox relay 分段指标 smoke |
| [loadtest-report-20260609-outbox-batchsize-smoke.md](./loadtest-report-20260609-outbox-batchsize-smoke.md) | batch size 探索 |
| [loadtest-report-20260609-outbox-publishbatch-smoke.md](./loadtest-report-20260609-outbox-publishbatch-smoke.md) | Kafka PublishBatch smoke |
| [loadtest-report-20260609-publishbatch-metrics-smoke.md](./loadtest-report-20260609-publishbatch-metrics-smoke.md) | PublishBatch 指标拆分 |
| [loadtest-report-20260609-publishbatch-formal.md](./loadtest-report-20260609-publishbatch-formal.md) | PublishBatch on/off 正式矩阵 |
| [loadtest-report-20260609-outbox-batch-worker-matrix.md](./loadtest-report-20260609-outbox-batch-worker-matrix.md) | batch size / worker 联合矩阵 |
| [loadtest-report-20260609-outbox-active-idle-metrics-smoke.md](./loadtest-report-20260609-outbox-active-idle-metrics-smoke.md) | relay active/idle 指标验证 |
| [loadtest-report-20260609-outbox-candidate-repeat.md](./loadtest-report-20260609-outbox-candidate-repeat.md) | outbox 候选重复验证 |
| [loadtest-report-20260609-adaptive-limit-smoke.md](./loadtest-report-20260609-adaptive-limit-smoke.md) | app adaptive admission smoke |
| [loadtest-report-20260609-adaptive-limit-onoff.md](./loadtest-report-20260609-adaptive-limit-onoff.md) | adaptive on/off 对照 |
| [loadtest-report-20260609-recent-metrics-smoke.md](./loadtest-report-20260609-recent-metrics-smoke.md) | recent 4096 样本窗口验证 |
| [loadtest-report-20260609-adaptive-hysteresis-smoke.md](./loadtest-report-20260609-adaptive-hysteresis-smoke.md) | hysteresis smoke |
| [loadtest-report-20260609-adaptive-retry-hint-smoke.md](./loadtest-report-20260609-adaptive-retry-hint-smoke.md) | dynamic RetryInfo / retry delay histogram smoke |
| [loadtest-report-20260609-adaptive-threshold-v1.md](./loadtest-report-20260609-adaptive-threshold-v1.md) | adaptive threshold v1 |
| [loadtest-report-20260609-adaptive-best-repeat.md](./loadtest-report-20260609-adaptive-best-repeat.md) | adaptive best candidate 60s repeat |
| [loadtest-report-20260609-adaptive-poolp95-v1.md](./loadtest-report-20260609-adaptive-poolp95-v1.md) | pool acquire p95 阈值矩阵 |
| [loadtest-report-20260609-adaptive-inflight-v1.md](./loadtest-report-20260609-adaptive-inflight-v1.md) | MaxInFlight token gate 矩阵 |

## 7. 后续策略

用户机器资源有限，后续不再围绕 message-service 做大规模压测矩阵。后续原则：

1. 每个新功能切片只做 smoke / 小规模验证。
2. 大规模压测只在关键机制变更后做一次，不做反复硬件拉满。
3. 优先推进更多微服务：conversation-service、delivery-service、push-gateway。
4. 保留当前 consolidated report 作为 message-service 第一阶段面试材料。
