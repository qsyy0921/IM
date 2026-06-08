# Outbox Active / Idle Metrics Smoke 2026-06-09

## 1. 压测目标

修复评审指出的 relay metrics 口径问题：`outbox_process_ready_latency_ms` 会混入 `stats_wait` 阶段的空轮询样本，直接用于 adaptive limit 会稀释 active relay 周期。

本轮新增并验证：

```text
outbox_process_ready_active_latency_ms
outbox_process_ready_idle_latency_ms
outbox_fetched_per_call
```

本轮不是容量压测。

## 2. 压测拓扑

```text
loadtest/sendmessage
-> message-service gRPC
-> PostgreSQL local transaction
-> message_outbox
-> outbox relay PublishBatch
-> Kafka conversation.timeline.events
```

## 3. 执行命令

```powershell
. .\tools\go-env.ps1
.\loadtest\sendmessage\run-local-outbox-batch-worker-matrix.ps1 `
  -BatchSizes 100 `
  -RelayWorkers 2 `
  -VUs 10 `
  -PGMaxConns 16 `
  -Duration 5s `
  -StatsWait 5s `
  -ConversationCount 100 `
  -PublishBatchEnabled:$true `
  -BackpressureEnabled `
  -BackpressureMinAvailableConns 4 `
  -RetryOverloaded `
  -MaxRetries 1 `
  -RetryJitter 50ms `
  -ResultRoot loadtest\results\outbox-active-idle-metrics-smoke-20260609
```

## 4. 结果文件

```text
loadtest/results/outbox-active-idle-metrics-smoke-20260609/batch-100-workers-2/bpon-pbatchon-pgmax-16-vu-10-20260609-070214/sendmessage-summary.json
loadtest/results/outbox-active-idle-metrics-smoke-20260609/outbox-batch-worker-matrix-summary.json
```

## 5. 核心结果

```text
commit: 40baec9
git_dirty: false
request_count: 5487
success_rate: 1.0000
outbox_pending_count: 0
outbox_process_ready_latency_ms: 14.6943
outbox_process_ready_active_latency_ms: 18.4141
outbox_process_ready_idle_latency_ms: 2.2710
outbox_fetched_per_call: 12.2752
kafka_publish_records_per_call: 15.9506
```

矩阵后 PostgreSQL：

```text
PUBLISHED|1001524
```

没有遗留 `PENDING` / `DLQ`。

## 6. 排查结论

- 新字段已从 relay 进程 `/debug/metrics` 进入 loadtest summary。
- active 平均耗时高于 idle，说明拆分有意义。
- `outbox_fetched_per_call` 可用于判断一轮 relay 是否真的处理了 ready outbox，避免只看 `outbox_process_ready_latency_ms`。
- adaptive limit 后续应优先使用 active 指标和 fetched-per-call，不应只看混合口径。

## 7. 下一步

- 用新指标重复验证 `batch_size=100/workers=8` 和 `batch_size=500/workers=8`。
- adaptive limit 输入暂定为：PG pool acquire、outbox pending、active process ready、fetched per call、Kafka records per call。
