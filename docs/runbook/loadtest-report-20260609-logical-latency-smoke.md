# NexusIM SendMessage Logical Latency Smoke

## 1. 压测目标

验证 loadtest summary 新增的 logical end-to-end latency 字段是否能把 retry sleep 计入用户层等待时间。

## 2. 执行方式

使用极端 adaptive admission 阈值强制 `SERVICE_OVERLOADED`，并开启一次 retry：

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 4 `
  -VUs 5 `
  -Duration 3s `
  -StatsWait 1s `
  -ConversationCount 50 `
  -RelayWorkers 2 `
  -BatchSize 100 `
  -PublishBatchEnabled:$true `
  -AdaptiveLimitEnabled `
  -AdaptiveMinAvailableConns 4 `
  -AdaptiveReleaseAvailableConns 8 `
  -AdaptiveMaxPoolAcquireP95 0s `
  -AdaptiveMaxOutboxPending 1 `
  -AdaptiveReleaseOutboxPending 1 `
  -AdaptiveMaxRelayActiveP95 0s `
  -AdaptiveMinOutboxFetchedPerCall 0 `
  -AdaptiveMinKafkaRecordsPerCall 0 `
  -AdaptiveSampleInterval 500ms `
  -AdaptiveRetryBaseDelay 250ms `
  -AdaptiveRetryMaxDelay 1s `
  -RetryOverloaded `
  -MaxRetries 1 `
  -RetryJitter 0s `
  -ResultRoot loadtest\results\logical-latency-smoke-20260609
```

结果文件：

```text
loadtest/results/logical-latency-smoke-20260609/bpoff-adapton-pbatchon-pgmax-4-vu-5-20260609-092242/sendmessage-summary.json
```

## 3. 核心结果

| 指标 | 值 |
| --- | ---: |
| commit | `91cbb8c` |
| git_dirty | `false` |
| logical_request_count | 31 |
| request_count | 57 |
| retry_attempt_count | 26 |
| retry_delay_count | 31 |
| attempt p99 | 7.3318ms |
| logical p99 | 507.6808ms |
| logical error p99 | 507.6808ms |
| retry_delay_p95 | 500ms |
| outbox_pending_count | 0 |

压测后全局 outbox：

```text
PUBLISHED=2394747
PENDING=0
DLQ=0
```

## 4. 结论

logical latency 字段可用。它和 attempt latency 的差异明显：

```text
attempt p99 = 7.3318ms
logical p99 = 507.6808ms
```

后续 adaptive 阈值矩阵必须同时展示：

- `logical_p99_ms`
- `logical_success_p99_ms`
- `logical_error_p99_ms`
- `success_p99_ms`
- `retry_delay_p95_ms`
- `retry_attempt_count`

其中 `success_p99_ms` 仍是单次成功 attempt 延迟，不能代表用户层完整等待。
