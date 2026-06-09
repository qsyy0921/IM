# NexusIM Outbox Batch Mark Loadtest Report

日期：2026-06-09

## 1. 压测目标

上一轮 client retry 矩阵显示：客户端遵守 `RetryInfo` 后，accepted RPS 回升，但 `message_outbox` 再次积压。本轮做一个低风险 relay 优化：

```text
成功 publish 后，不再逐条 UPDATE message_outbox
改为同一事务内按 id 数组批量 mark PUBLISHED
```

目标是验证该优化是否能降低 outbox pending，而不改变 Kafka publish 顺序或至少一次投递语义。

## 2. 变更范围

代码提交：

```text
b6f0b82 feat: batch mark published outbox
```

核心变化：

- `OutboxStore.ProcessReady` 发布成功后先收集 outbox id。
- 本批 publish callback 全部执行完后，统一执行：

```sql
UPDATE message_outbox
SET status = 'PUBLISHED',
    published_at = $3,
    last_error = NULL,
    next_retry_at = NULL,
    dead_lettered_at = NULL
WHERE id = ANY($1::bigint[])
```

- publish 顺序仍由 `fetchReadyLocked` 和 relay worker 顺序控制。
- 如果 batch mark 或 commit 失败，已 publish 的事件仍可能重复发布，继续符合 outbox 至少一次语义。

已补真实 PostgreSQL 集成测试：

```text
TestOutboxStoreProcessReadyBatchMarksPublished
```

## 3. 压测拓扑

```text
loadtest/sendmessage --retry-overloaded
-> message-service gRPC
-> PostgreSQL transaction
-> message_outbox
-> outbox relay
-> Kafka publish
-> batch mark published
```

本轮对比：

- before：`0c542a1 feat: add client overload retry loadtest`
- after：`b6f0b82 feat: batch mark published outbox`

## 4. 执行命令

after 组：

```powershell
. .\tools\go-env.ps1
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 64 `
  -VUs 1200,1600 `
  -Duration 60s `
  -StatsWait 30s `
  -ConversationCount 3000 `
  -RelayWorkers 8 `
  -BatchSize 500 `
  -BackpressureEnabled `
  -BackpressureMinAvailableConns 8 `
  -RetryOverloaded `
  -MaxRetries 2 `
  -RetryJitter 100ms `
  -ResultRoot loadtest\results\backpressure-client-retry-batchmark-formal-20260609
```

before 组来自上一份报告：

```text
loadtest/results/backpressure-client-retry-formal-20260609/
```

after 组结果：

```text
loadtest/results/backpressure-client-retry-batchmark-formal-20260609/
```

## 5. 对比结果

| 版本 | VU | logical success rate | accepted RPS | success p99 | error p99 | Kafka p99 | outbox pending |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| before | 1200 | 68.68% | 1526.23 | 421.58ms | 4.10ms | 22.78ms | 25,579 |
| after | 1200 | 61.28% | 1165.65 | 624.15ms | 5.98ms | 14.64ms | 5,186 |
| before | 1600 | 56.04% | 1288.98 | 1154.47ms | 7.43ms | 26.30ms | 32,091 |
| after | 1600 | 60.29% | 1520.60 | 733.95ms | 4.81ms | 26.22ms | 0 |

after 组结束后额外检查全局 outbox：

```text
PENDING=0
```

## 6. 瓶颈排查过程

### 6.1 先看 outbox pending

批量 mark 后，最明显变化是 backlog 降低：

```text
VU1200: 25579 -> 5186
VU1600: 32091 -> 0
```

这说明逐条 mark published 的 DB 往返确实是 relay 追平路径上的一部分成本。

### 6.2 再看 accepted RPS 和 success p99

结果不能简单写成“容量提升”：

```text
VU1200 accepted_rps: 1526.23 -> 1165.65
VU1600 accepted_rps: 1288.98 -> 1520.60
```

两组有本机压测波动，不能只凭一次 run 判断容量上升或下降。更稳妥的结论是：批量 mark 明显降低了 pending，但还需要重复矩阵确认吞吐和尾延迟。

### 6.3 再看 Kafka publish

Kafka p99 没有明显恶化：

```text
VU1200 kafka_p99: 22.78ms -> 14.64ms
VU1600 kafka_p99: 26.30ms -> 26.22ms
```

说明本次优化没有把瓶颈转移到 Kafka publish latency；它主要减少了 PostgreSQL mark published 的 update 次数。

### 6.4 语义检查

该优化没有改变：

- outbox fetch 的 `FOR UPDATE SKIP LOCKED`。
- 低版本 `PENDING/DLQ` 阻塞后续版本的顺序保护。
- Kafka publish callback 执行顺序。
- commit 失败后的至少一次重复发布窗口。

因此它是低风险优化，但仍需要后续评审确认生产边界。

## 7. 当前结论

1. 批量 mark published 能显著降低 outbox pending。
2. 单次本机矩阵不能证明整体容量提升；吞吐和 success p99 仍需重复 run。
3. 下一步更大的收益应来自批量 Kafka publish 或缩小事务持锁时间。
4. adaptive limit 需要纳入 outbox pending，否则客户端 retry 会把压力重新推到 relay。

## 8. 下一步

- 重跑一轮 after 矩阵确认 pending 降低是否稳定。
- 设计 `Publisher.PublishBatch`，让 relay 可以按 batch 调用 Kafka writer。
- 评估 `ProcessReady` 是否应缩小事务范围，避免 Kafka 慢时长时间持有 outbox row lock。
- 在后续矩阵中固定展示已新增的 relay metrics：`outbox_process_ready_latency_ms`、`outbox_fetch_ready_latency_ms`、`outbox_mark_published_latency_ms`、`outbox_commit_latency_ms`。
