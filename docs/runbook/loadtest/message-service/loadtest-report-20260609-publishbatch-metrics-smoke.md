# PublishBatch Metrics Smoke 2026-06-09

## 1. 压测目标

修复评审指出的指标口径问题：`kafka_publish_latency_ms` 在 single path 与 batch path 下语义不同。新增明确字段后，用真实链路 smoke 验证 summary 能写出新指标。

本轮不是容量压测。

## 2. 新增指标

保留旧字段：

```text
kafka_publish_latency_ms
```

新增字段：

```text
kafka_publish_call_latency_ms
kafka_publish_records_per_call
kafka_publish_record_latency_estimate_ms
```

口径说明：

- `kafka_publish_call_latency_ms`：一次 Kafka write 调用耗时。single path 是单条 `WriteMessages`，batch path 是一次 `WriteMessages(records...)`。
- `kafka_publish_records_per_call`：一次 Kafka write 调用包含的 record 数。
- `kafka_publish_record_latency_estimate_ms`：`call latency / records_per_call` 的估算值，只用于排查趋势，不代表 Kafka broker 对每条消息的真实独立处理时间。
- `kafka_publish_latency_ms` 继续保留兼容旧报告；正式 before/after 结论优先使用新字段。

## 3. 压测拓扑

```text
loadtest/sendmessage
-> message-service gRPC
-> PostgreSQL local transaction
-> message_outbox
-> outbox relay PublishBatch
-> Kafka conversation.timeline.events
```

## 4. 执行命令

```powershell
. .\tools\go-env.ps1
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 16 `
  -VUs 10 `
  -Duration 5s `
  -StatsWait 5s `
  -ConversationCount 100 `
  -RelayWorkers 2 `
  -BatchSize 100 `
  -ResultRoot loadtest\results\publishbatch-metrics-smoke-20260609
```

## 5. 结果文件

```text
loadtest/results/publishbatch-metrics-smoke-20260609/bpoff-pgmax-16-vu-10-20260609-055454/sendmessage-summary.json
```

## 6. 核心结果

```text
commit: 8742f84
git_dirty: false
request_count: 5645
success_rate: 1.0000
p99_ms: 12.7919
outbox_pending_count: 0
kafka_publish_latency_ms: 11.5020
kafka_publish_call_latency_ms: 11.5020
kafka_publish_records_per_call: 21.0672
kafka_publish_record_latency_estimate_ms: 0.7350
outbox_process_ready_latency_ms: 16.5239
```

## 7. 排查结论

- 新字段已从真实 relay 进程 `/debug/metrics` 进入 loadtest summary。
- `kafka_publish_call_latency_ms` 与旧 `kafka_publish_latency_ms` 当前数值相同，这是兼容保留；后续报告不要用旧字段比较 single 和 batch。
- `kafka_publish_records_per_call` 非空，说明 batch path 下可以观察每次 Kafka write 承载的 record 数。
- 本轮 outbox pending 为 0，只证明短 smoke 可追平，不证明容量提升。

## 8. 下一步

- 正式 before/after PublishBatch 矩阵必须展示 `kafka_publish_call_latency_ms`、`kafka_publish_records_per_call`、`kafka_publish_record_latency_estimate_ms`。
- 继续把 adaptive limit 输入从单一 PG pool acquire 扩展到 outbox pending 和 relay process ready latency。
