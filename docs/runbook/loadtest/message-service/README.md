# Message Service 压测报告入口

本目录保存 `message-service` 第一阶段所有压测报告。这里不是单纯索引，而是面试和复盘时优先阅读的结论入口。

详细总报告：

```text
loadtest-report-20260609-message-service-consolidated.md
```

最新功能 smoke：

```text
loadtest-report-20260610-edit-message-smoke.md
loadtest-report-20260610-revoke-message-smoke.md
```

## 1. 一句话结论

`message-service` 已经不是 demo endpoint，而是跑通过真实 IM 写消息链路的微服务：

```text
客户端 / 压测器
-> message-service gRPC
-> SendMessage 用例
-> PostgreSQL 本地事务
-> message_log / conversation_timeline_events / message_outbox
-> outbox relay
-> Kafka conversation.timeline.events
```

压测结论不是“单机能扛多少并发”这么简单，而是已经证明：

- 写消息主链路可以真实运行，并能通过 PostgreSQL 事务保证消息、时间线事件和 outbox 同时落库。
- Kafka 不在业务事务里直接发布，而是通过 outbox relay 异步发布，避免 DB 成功但 Kafka 失败导致数据不一致。
- 性能瓶颈主要不在 Kafka、业务代码或 conversation_seq，而在 PostgreSQL 连接池等待 / 写入压力，以及高写入量下的 outbox 追平能力。
- 盲目增加 CPU、内存、relay worker 或 message-service 实例不能根治问题，因为共享 PostgreSQL 仍然是核心资源。
- 当前已经有 backpressure、RetryInfo、client retry、adaptive admission、MaxInFlight token gate 等容量治理证据，可以在面试中讲清楚“怎么发现瓶颈、怎么验证假设、怎么保护系统”。
- 用户机器资源有限，后续不再继续做重型硬件矩阵；这些结果已经足够支撑面试叙事，下一阶段应开发 `conversation-service` 等其它真实微服务。
- 第三层消息变更已开始补齐：`RevokeMessage` 和 `EditMessage` 最小真实进程 smoke 已证明消息变更事件能从 `message-service` 本地事务进入 `conversation.timeline.events`，再由 `delivery-service` 投影成 `PullInbox` 可见的 tombstone / edited item。

## 2. 覆盖范围

本阶段真实覆盖：

```text
gRPC API
app SendMessage use case
domain command hash / append record
PostgreSQL local transaction
conversation_seq
message_log
conversation_timeline_events
message_outbox
outbox relay
Kafka publish path
loadtest runner
debug metrics
```

本阶段没有覆盖：

```text
conversation-service 真实 RPC
delivery-service fanout / inbox
push-gateway WebSocket
timeline-service hotspot sequencer
Web / desktop 客户端 UI
RAG / Agent
```

因此面试时要准确表述：第一阶段完成的是 `message-service` 普通会话写入链路，不是完整 IM 系统。

## 3. 核心压测结果

### 3.1 第一轮真实链路基线

第一轮 baseline 证明链路完整可运行：

```text
SendMessage -> PostgreSQL local transaction -> outbox -> Kafka
```

典型结果：

| 场景 | 结果 |
| --- | --- |
| `--vus=100 --duration=60s` | 45212 / 45212 成功 |
| p95 | 249.62ms |
| p99 | 518.03ms |
| stats-wait 30s 后 outbox pending | 27181 |

解释：

- 写入成功率是 100%，说明主事务链路可用。
- 但 outbox pending 很高，说明写入速度已经超过 relay 追平速度。
- 这个结果把问题从“能不能跑通”推进到“如何追平异步事件和控制尾延迟”。

### 3.2 Windows 本机资源矩阵

用户机器上 Docker Desktop 最多给到：

```text
16 CPU
约 24GB 内存
```

压测覆盖过：

```text
1 / 2 / 4 CPU + 256m / 512m / 1g
8 / 12 / 16 CPU + 2g / 4g / 8g
16 CPU + 23g
```

按当时门槛：

```text
success_rate >= 0.99
p99 <= 1000ms
outbox_pending_count <= 1000
```

观察到的较好结果：

| 资源 | Relay workers | VU | 结果 |
| --- | ---: | ---: | --- |
| 16 CPU / 23GB | 8 | 1200 | 约 2493 rps，p99 736.28ms，outbox pending 0 |
| 16 CPU / 23GB | 8 | 1600 | p99 1120.48ms，超过 1s 门槛 |
| 16 CPU / 23GB | 16 | 1200 | p99 1477.19ms，worker 过多反而放大争用 |

结论：

- CPU / 内存增加能改善一部分吞吐，但不是线性扩展。
- relay worker 不是越多越好，过多 worker 会增加数据库争用。
- 单机压测结果只能作为本地开发证据，不能当生产容量承诺。

### 3.3 Windows + Mac 分布式客户端

第一次做了四组，后续按用户要求只保留 `Windows -> Mac` / `Mac -> Windows` 方向的必要验证。已经跑通的双客户端场景：

当前 Win-Mac 压测网络已改为有线直连：Windows `172.31.50.1/24`，Mac `172.31.50.2/24`，两端 Wi-Fi 继续负责上网。历史结果中的 `192.168.0.*` 地址只代表当时 Wi-Fi 局域网路径，后续压测目标地址以 `docs/runbook/local-loadtest.md` 为准。

| 场景 | 结果 |
| --- | --- |
| Windows + Mac 各 600 VU | 两端全部成功，Windows p99 730.11ms，Mac p99 739.50ms，outbox pending 0 |
| Windows + Mac 各 1000 VU | 两端全部成功，但 p99 约 1331ms，超过 1s 门槛 |

结论：

- 可以用两台机器模拟分布式客户端，不需要真实多机集群。
- 双客户端能验证跨网络压测路径、服务端暴露端口、Mac/Windows 压测器一致性。
- 高 VU 下瓶颈仍回到服务端 DB / outbox / admission，而不是客户端机器本身。

### 3.4 PostgreSQL / 多实例诊断

关键发现：

| 实验 | 结论 |
| --- | --- |
| PG pool 梯度 | request p99 主要贴近 repository begin / pool acquire |
| multi-instance | 增加 message-service 实例没有稳定降低 p99 |
| fixed total PG budget | 固定总 PG 连接预算时，多实例不一定更好 |
| PostgreSQL loadtest profile | 单纯提高 `max_connections/shared_buffers/max_wal_size` 不能解决 p99 |

核心证据：

```text
request p99 ~= repository append p99
repository append p99 主要来自 pool acquire / PostgreSQL 写入压力
Kafka publish / conversation_seq / commit 单项延迟远低于 request p99
```

PostgreSQL 观测还看到：

```text
LWLock:WALWrite
LWLock:WALInsert
BufferContent
CheckpointWriteDelay
message_outbox dead tuples
```

这说明高并发写入下，WAL、checkpoint、outbox 高频 update 进入瓶颈视野。

### 3.5 Outbox relay 优化

做过的优化：

- 多 worker relay。
- 失败退避 `FailureBackoff`。
- 成功发布后批量 mark `PUBLISHED`。
- Kafka `PublishBatch`。
- 拆分 active / idle relay metrics。
- batch size / worker 数矩阵。

当前本机 relay 基线候选：

```text
NEXUSIM_OUTBOX_BATCH_SIZE=100
NEXUSIM_OUTBOX_WORKERS=8
PublishBatch=true
```

结论：

- batch mark 可以明显降低 outbox backlog。
- Kafka PublishBatch 对 backlog 有帮助，但不直接改善 SendMessage 成功写入 p99。
- `batch_size=100 / workers=8` 比盲目加大 batch 或 worker 更稳。
- relay 仍保持 at-least-once 语义：publish 成功但 mark/commit 失败时允许重复发布，依赖 `event_id` 消费幂等。

### 3.6 Backpressure / client retry / adaptive admission

实现过的保护机制：

```text
SERVICE_OVERLOADED
gRPC codes.Unavailable
MessageError.retryable=true
RetryInfo
client retry + jitter
app AdmissionPort
recent 4096 sample window
hysteresis
dynamic RetryInfo
MaxInFlight token gate
```

重要结论：

- backpressure 不是容量提升方案，它是系统保护机制。
- 不能只看 overall p99，因为快速失败会让整体 p99 看起来很好。
- 必须拆开看：

```text
success p99
error p99
accepted RPS
overload rate
logical success rate
logical p99
```

当前最有价值的本机候选：

```text
PG_MAX_CONNS=64
relay workers=8
outbox batch size=100
PublishBatch=true
AdaptiveMaxInFlight=64
```

60s repeat 结果：

| VU | accepted RPS | success p99 | logical p99 | outbox pending |
| ---: | ---: | ---: | ---: | ---: |
| 1200 | 1922.23 | 63.58ms | 2200.80ms | 0 |
| 1600 | 1926.97 | 63.35ms | 2203.44ms | 0 |

解释：

- `success p99` 是成功写入 attempt 的延迟，被 token gate 控制到几十毫秒。
- `logical p99` 包含客户端收到 `SERVICE_OVERLOADED` 后等待 RetryInfo 再重试的时间，所以约 2.2s。
- 后续优化重点不是继续加硬件，而是优化 retry 策略和继续补其它微服务。

## 4. 瓶颈是怎么排查出来的

本阶段排查不是只跑一个压测命令，而是逐步缩小范围：

1. **先证明真实链路可用**

   只压真实 `message-service` 进程，不压固定字符串 endpoint。确认 gRPC、事务、outbox、Kafka 都真实参与。

2. **从 request p99 往内部拆**

   依次记录：

   ```text
   SendMessage 总耗时
   repository append
   pool acquire
   tx begin
   advisory lock
   idempotency lookup
   conversation_seq
   insert message
   insert timeline
   insert outbox
   commit
   Kafka publish
   outbox relay
   ```

3. **排除不是瓶颈的模块**

   Kafka publish、conversation_seq、commit 在多数结果中都远低于 request p99，所以不是第一主因。

4. **定位 repository / PostgreSQL**

   request p99 贴近 repository append p99，继续拆后发现高并发下主要来自 pool acquire / PostgreSQL 写入压力。

5. **验证多实例假设**

   增加 message-service 实例后，如果共享 PostgreSQL 没变，p99 没有稳定下降；这说明入口进程不是唯一瓶颈。

6. **验证 outbox 追平能力**

   在写入成功率提高后，outbox pending 变高，说明 relay 成为第二瓶颈；通过 batch mark、PublishBatch、active/idle metrics 定位和优化。

7. **验证保护策略**

   backpressure / adaptive admission 能让服务快速拒绝超载请求，避免大量请求堵在 DB pool 里；但必须同时看 success p99 和 logical p99，避免被快速失败误导。

## 5. 面试中可以怎么讲

### 5.1 项目介绍版

可以这样讲：

> 我做的是一个 NexusIM 的消息服务切片，不是简单 demo。我先把 `SendMessage` 做成真实链路：gRPC 入口进入 DDD app use case，领域层生成 command hash 和 append record，PostgreSQL 同事务写入 message_log、conversation_timeline_events 和 message_outbox，最后由 outbox relay 异步发布 Kafka 事件。这样可以保证数据库写入和消息发布之间不会出现业务事务内直接发 Kafka 导致的不一致。

### 5.2 一致性设计版

可以这样讲：

> 普通会话下，我把 `conversation_seq`、消息日志、时间线事件和 outbox 放在同一个 PostgreSQL 本地事务里。业务事务只负责落库，不直接 publish Kafka。Kafka 发布由 outbox relay 做，失败时保留 PENDING、重试，超过次数进入 DLQ。这样是 at-least-once 语义，消费者侧需要按 event_id 幂等。

### 5.3 幂等和并发版

可以这样讲：

> SendMessage 支持 client_msg_id 幂等。之前评审发现并发重复请求可能先分配 seq 再 replay，导致 conversation_seq 出现 gap。后来我在同一幂等键上加 advisory transaction lock，并补了真实 PostgreSQL 并发测试，保证重复请求不会推进 seq，也不会重复写 message / timeline / outbox。

### 5.4 瓶颈排查版

可以这样讲：

> 压测时我没有只看 QPS 和 p99，而是把链路拆成多个指标。第一轮发现 request p99 贴近 repository append p99，而 Kafka、commit、conversation_seq 都很低，所以排除了 Kafka 和 seq。继续拆 repository 后，发现主要等待在 pgxpool acquire / PostgreSQL 写入压力。后来做多实例和固定总 PG 连接预算实验，发现增加 message-service 实例不能稳定降低 p99，因为共享 PostgreSQL 仍是核心瓶颈。

### 5.5 Outbox 优化版

可以这样讲：

> 写入成功率上来以后，outbox pending 变成第二瓶颈。我做了 batch mark PUBLISHED、Kafka PublishBatch、relay active/idle 指标和 batch/worker 矩阵。最后发现 `batch_size=100 / workers=8` 比盲目增大 worker 更稳，因为 worker 太多会增加数据库争用。

### 5.6 背压和限流版

可以这样讲：

> 当 PostgreSQL pool 已经排队时，与其让请求都等到超时，不如快速返回 `SERVICE_OVERLOADED`。我把这个错误映射成 gRPC `Unavailable`，并带 `MessageError.retryable=true` 和 `RetryInfo`。压测器也区分 logical request 和 gRPC attempt，避免把重试次数误认为真实吞吐。最后用 `MaxInFlight=64` token gate 控制进入依赖读取和 DB 事务的并发，成功请求 p99 能保持在几十毫秒，同时 outbox 不积压。

### 5.7 如何诚实说明局限

不要说：

```text
这个系统已经能生产抗住 2000 QPS。
```

应该说：

```text
在 Windows 本机 Docker PostgreSQL/Kafka 环境下，message-service 普通会话 SendMessage 链路已经完成真实压测。
当前结果说明链路可用、瓶颈可解释、保护策略有效；但它不是生产容量承诺。
后续需要接入 conversation-service、delivery-service、push-gateway，并在更接近生产的环境里重新压测。
```

## 6. 面试关键词速记

| 关键词 | 面试解释 |
| --- | --- |
| `p99` | 99% 请求都不超过的延迟，比平均值更能反映尾延迟和用户体验 |
| `VU` | virtual user，压测中的并发虚拟用户数，不等于真实在线人数 |
| `accepted RPS` | 服务真正接受并成功写入的请求速率 |
| `overall p99` | 成功和失败混在一起的 p99，backpressure 场景下容易误导 |
| `success p99` | 成功写入请求的 p99，更能代表写路径体验 |
| `logical p99` | 用户层请求端到端 p99，包含 RetryInfo 等待和重试 |
| `outbox pending` | 已落库但尚未发布 Kafka 的事件数量 |
| `backpressure` | 系统过载时主动快速拒绝，避免资源被排队请求耗尽 |
| `RetryInfo` | 服务端告诉客户端建议多久后重试 |
| `at-least-once` | 至少投递一次，允许重复，不允许丢；消费者必须幂等 |
| `DLQ` | dead letter queue，多次失败后进入死信，等待人工或补偿流程 |
| `pool acquire` | 从数据库连接池获取连接的等待时间，高并发下常成为瓶颈 |

## 7. 报告结构

| 类型 | 文件 |
| --- | --- |
| 总报告 | `loadtest-report-20260609-message-service-consolidated.md` |
| 第一轮真实链路基线 | `loadtest-report-20260609.md` |
| PostgreSQL / 多实例诊断 | `loadtest-report-20260609-pgpool-multi-instance.md`、`loadtest-report-20260609-multi-instance-budget.md`、`loadtest-report-20260609-postgres-loadtest-profile.md` |
| Backpressure / client retry | `loadtest-report-20260609-backpressure*.md`、`loadtest-report-20260609-client-retry.md` |
| Outbox relay / PublishBatch | `loadtest-report-20260609-outbox-*.md`、`loadtest-report-20260609-publishbatch-*.md`、`loadtest-report-20260609-relay-metrics-smoke.md` |
| Adaptive admission | `loadtest-report-20260609-adaptive-*.md`、`loadtest-report-20260609-recent-metrics-smoke.md`、`loadtest-report-20260609-logical-latency-smoke.md` |

## 8. 后续策略

后续原则：

- `message-service` 不再继续做大规模硬件矩阵。
- 关键机制变化后只做 smoke / 小规模验证。
- 每个新微服务也按 `docs/runbook/loadtest/<service>/` 保存小报告和总报告。
- 下一阶段优先开发 `conversation-service`，替换当前 message-service 的 strict conversation mock，形成真实跨服务 RPC 依赖。
