# NexusIM SendMessage Recent Metrics Smoke Report

## 1. 压测目标

上一轮 adaptive on/off 排查发现：debug metrics collector 的累计 p95 会粘住，不能直接作为 adaptive limit 的硬拒绝输入。

本轮目标是新增短窗口 recent 指标，并验证真实进程 summary 能读取：

```text
repository_pool_acquire_recent_latency_ms
outbox_process_ready_active_recent_latency_ms
outbox_fetched_per_call_recent
kafka_publish_records_per_call_recent
```

## 2. 实现范围

保留原有累计字段，新增 recent 字段：

- `repository_pool_acquire_recent_latency_ms`
- `outbox_process_ready_active_recent_latency_ms`
- `outbox_fetched_per_call_recent`
- `kafka_publish_records_per_call_recent`

recent 窗口当前保留最近 `4096` 个样本。旧字段不变，避免破坏已有报告。

adaptive controller 优先读取 recent 字段；如果旧版本服务没有 recent 字段，再回退到累计字段。

## 3. 压测拓扑

```text
sendmessage loadtest
-> message-service gRPC
-> PostgreSQL local transaction
-> message_outbox
-> outbox relay
-> Kafka
```

全部组件运行在 Windows 本机。

## 4. 执行命令

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 16 `
  -VUs 10 `
  -Duration 5s `
  -StatsWait 5s `
  -ConversationCount 100 `
  -RelayWorkers 2 `
  -BatchSize 100 `
  -PublishBatchEnabled:$true `
  -AdaptiveLimitEnabled `
  -AdaptiveMinAvailableConns 4 `
  -AdaptiveMaxPoolAcquireP95 0s `
  -AdaptiveMaxOutboxPending 50000 `
  -AdaptiveMaxRelayActiveP95 0s `
  -AdaptiveMinOutboxFetchedPerCall 0 `
  -AdaptiveMinKafkaRecordsPerCall 0 `
  -AdaptiveSampleInterval 500ms `
  -RetryOverloaded `
  -MaxRetries 1 `
  -RetryJitter 50ms `
  -ResultRoot loadtest\results\recent-metrics-smoke-20260609
```

结果文件：

```text
loadtest/results/recent-metrics-smoke-20260609/bpoff-adapton-pbatchon-pgmax-16-vu-10-20260609-075920/sendmessage-summary.json
```

## 5. 核心结果

| 指标 | 值 |
| --- | ---: |
| commit | `6d4910b` |
| git_dirty | `false` |
| request_count | 5392 |
| success_rate | 1.0 |
| p95 | 11.7022ms |
| p99 | 14.5047ms |
| outbox_pending_count | 0 |
| repository_pool_acquire_recent_latency_ms | 0.0027ms |
| repository_pool_acquire_recent_p95_ms | 0 |
| repository_pool_acquire_recent_p99_ms | 0 |
| outbox_process_ready_active_recent_latency_ms | 18.5180ms |
| outbox_process_ready_active_recent_p95_ms | 21.0682ms |
| outbox_fetched_per_call_recent | 12.8995 |
| kafka_publish_records_per_call_recent | 16.6935 |

压测后全局 outbox：

```text
PUBLISHED=1917788
PENDING=0
DLQ=0
```

## 6. 排查价值

本轮解决的是观测口径问题：

1. 累计 p95 适合历史报告，但不适合直接控制 admission。
2. recent 字段能观察最近窗口，避免一次高压样本让 adaptive limit 长时间误拒绝。
3. old/cumulative 字段继续保留，用于趋势报告和历史兼容。

## 7. 下一步

recent 窗口只是基础能力。下一步需要：

- 给 adaptive controller 增加 hysteresis，区分进入过载和退出过载阈值。
- 将 `RetryInfo=500ms` 改成随过载级别变化的动态 retry hint。
- 用 recent 字段重跑 adaptive 阈值矩阵。

