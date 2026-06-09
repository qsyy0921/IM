# NexusIM SendMessage Adaptive Hysteresis Smoke Report

## 1. 压测目标

本阶段验证 adaptive admission 的 hysteresis 开关路径：

```text
enter overload:
available_conns <= AdaptiveMinAvailableConns

recover:
available_conns > AdaptiveReleaseAvailableConns
outbox_pending < AdaptiveReleaseOutboxPending
```

目标不是容量结论，而是确认 release 阈值接入真实服务进程后，不破坏正常写入链路。

## 2. 本轮实现

新增配置：

```text
NEXUSIM_ADAPTIVE_RELEASE_AVAILABLE_CONNS
NEXUSIM_ADAPTIVE_RELEASE_OUTBOX_PENDING
```

脚本参数：

```text
-AdaptiveReleaseAvailableConns
-AdaptiveReleaseOutboxPending
```

默认规则：

- 如果未配置 `ReleaseAvailableConns`，默认使用 `MinAvailableConns + 4`。
- 如果未配置 `ReleaseOutboxPending` 且配置了 `MaxOutboxPending`，默认使用 `MaxOutboxPending / 2`。

## 3. 执行命令

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
  -AdaptiveReleaseAvailableConns 8 `
  -AdaptiveMaxPoolAcquireP95 0s `
  -AdaptiveMaxOutboxPending 50000 `
  -AdaptiveReleaseOutboxPending 25000 `
  -AdaptiveMaxRelayActiveP95 0s `
  -AdaptiveMinOutboxFetchedPerCall 0 `
  -AdaptiveMinKafkaRecordsPerCall 0 `
  -AdaptiveSampleInterval 500ms `
  -RetryOverloaded `
  -MaxRetries 1 `
  -RetryJitter 50ms `
  -ResultRoot loadtest\results\adaptive-hysteresis-smoke-20260609
```

结果文件：

```text
loadtest/results/adaptive-hysteresis-smoke-20260609/bpoff-adapton-pbatchon-pgmax-16-vu-10-20260609-080738/sendmessage-summary.json
```

## 4. 核心结果

| 指标 | 值 |
| --- | ---: |
| commit | `6f9a438` |
| git_dirty | `false` |
| request_count | 5554 |
| success_rate | 1.0 |
| p95 | 10.7695ms |
| p99 | 12.5034ms |
| service_overloaded_count | 0 |
| outbox_pending_count | 0 |
| repository_pool_acquire_recent_latency_ms | 0.0017ms |
| outbox_process_ready_active_recent_latency_ms | 18.1226ms |
| outbox_fetched_per_call_recent | 13.4155 |
| kafka_publish_records_per_call_recent | 17.4654 |

压测后全局 outbox：

```text
PUBLISHED=1923342
PENDING=0
DLQ=0
```

## 5. 排查方法

本轮判断顺序：

1. 先确认 `git_dirty=false`，避免旧二进制或未提交代码污染。
2. 看 `success_rate` 和 `service_overloaded_count`。
   - 低压 smoke 下不应触发 overload。
3. 看 recent 指标是否非空。
   - 如果 recent 字段为空，说明 debug metrics 或 summary 解析路径没接通。
4. 看 outbox。
   - `pending=0` 说明 hysteresis 没有影响 relay 追平。

## 6. 当前结论

hysteresis 配置已接入真实服务进程，并且低压写入链路正常。

这仍不是正式容量结果。下一步需要在高压矩阵中比较：

- 无 hysteresis
- pool release gap 4 / 8 / 16
- outbox release ratio 50% / 25%

同时观察：

- logical success rate
- accepted RPS
- attempt overload rate
- success p99
- error p99
- recent PG acquire p95/p99
- outbox pending

