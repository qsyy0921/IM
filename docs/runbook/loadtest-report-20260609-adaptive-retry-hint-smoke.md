# NexusIM SendMessage Adaptive Retry Hint Smoke Report

## 1. 压测目标

本阶段验证 `SERVICE_OVERLOADED` 的 gRPC `RetryInfo` 不再只能固定为 `500ms`。

目标：

- repository backpressure 未携带 delay 时，仍使用默认 `500ms`。
- adaptive admission 可以携带动态 retry delay。
- gRPC response 使用错误中携带的 retry delay。
- 压测器继续遵守 `RetryInfo`。

## 2. 本轮实现

新增：

```text
types.NewServiceOverloadedWithRetryDelay(reason, retryDelay)
types.ServiceOverloadedRetryDelay(err)
```

adaptive controller 新增：

```text
NEXUSIM_ADAPTIVE_RETRY_BASE_DELAY
NEXUSIM_ADAPTIVE_RETRY_MAX_DELAY
```

计算方式：

```text
retry_delay = min(reason_count * base_delay, max_delay)
recovering 状态额外增加一个 reason 权重
```

gRPC 映射：

```text
if err carries retry delay:
    RetryInfo.retry_delay = carried delay
else:
    RetryInfo.retry_delay = 500ms
```

## 3. 执行命令

本轮用极端阈值强制 adaptive admission 拒绝，并设置：

```text
AdaptiveRetryBaseDelay=250ms
AdaptiveRetryMaxDelay=1s
MaxRetries=1
RetryJitter=0s
```

原计划通过本地梯度脚本同时启动 gRPC、relay 和 metrics 采集：

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 4 `
  -VUs 5 `
  -Duration 3s `
  -StatsWait 3s `
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
  -ResultRoot loadtest\results\adaptive-retry-hint-smoke-20260609
```

该次脚本运行在 Windows 高并发 socket 资源不足时没有写出 summary。由于本轮目标只验证入口拒绝和 `RetryInfo`，拒绝发生在写事务之前，不需要 relay/Kafka 参与；随后用同一组 adaptive 阈值直接启动 gRPC 进程和压测器重跑。

最终执行方式：

```powershell
$env:NEXUSIM_MESSAGE_SERVICE_MODE='grpc'
$env:NEXUSIM_GRPC_ADDR='127.0.0.1:10495'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
$env:NEXUSIM_PG_MAX_CONNS='4'
$env:NEXUSIM_ADAPTIVE_LIMIT_ENABLED='true'
$env:NEXUSIM_ADAPTIVE_MIN_AVAILABLE_CONNS='4'
$env:NEXUSIM_ADAPTIVE_RELEASE_AVAILABLE_CONNS='8'
$env:NEXUSIM_ADAPTIVE_MAX_OUTBOX_PENDING='1'
$env:NEXUSIM_ADAPTIVE_RELEASE_OUTBOX_PENDING='1'
$env:NEXUSIM_ADAPTIVE_RETRY_BASE_DELAY='250ms'
$env:NEXUSIM_ADAPTIVE_RETRY_MAX_DELAY='1s'

.\bin\message-service.exe

.\bin\sendmessage-loadtest.exe `
  -target 127.0.0.1:10495 `
  -vus 5 `
  -duration 3s `
  -stats-wait 1ms `
  -conversation-count 50 `
  -retry-overloaded `
  -max-retries 1 `
  -retry-jitter 0s `
  -pg-dsn 'postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable' `
  -result-dir loadtest\results\adaptive-retry-hint-metrics-smoke-20260609\adaptive-retry-hint-metrics-grpc-only-20260609-082000
```

结果文件：

```text
loadtest/results/adaptive-retry-hint-metrics-smoke-20260609/adaptive-retry-hint-metrics-grpc-only-20260609-082000/sendmessage-summary.json
```

## 4. 核心结果

| 指标 | 值 |
| --- | ---: |
| commit | `c9e6cf1` |
| git_dirty | `false` |
| logical_request_count | 31 |
| request_count | 57 |
| retry_attempt_count | 26 |
| retried_request_count | 26 |
| retry_delay_count | 31 |
| retry_delay_avg_ms | 491.9355ms |
| retry_delay_p95_ms | 500ms |
| retry_delay_p99_ms | 500ms |
| success_rate | 0 |
| p95 | 5.5133ms |
| p99 | 5.5133ms |
| service_overloaded_count | 57 |
| outbox_pending_count | 0 |

压测后全局 outbox：

```text
PUBLISHED=1923342
PENDING=0
DLQ=0
```

## 5. 如何判断 RetryInfo 生效

注意：`p95/p99` 是单次 gRPC attempt 延迟，不包含 retry sleep。现在 summary 已新增 `retry_delay_*` 字段，应直接看 retry delay histogram。

本轮判断方式：

1. 单元测试确认 gRPC 会读取 `ServiceOverloadedError.RetryDelay`，并把 `1500ms` 写入 `RetryInfo`。
2. 真实进程 smoke 中开启 `MaxRetries=1`、`RetryJitter=0s`。
3. summary 记录 `retry_delay_count=31`，`retry_delay_p95_ms=500`，证明压测器确实读取并等待了 RetryInfo。
4. 本轮配置 `base=250ms`，但 hysteresis recovering 状态会额外增加一个 reason 权重，因此大部分 retry delay 被动态放大到 `500ms`。
5. outbox 没有新增，说明拒绝仍发生在写事务之前。

当前缺口：

- 本轮只验证链路，不是最佳 retry delay。
- 正式矩阵需要比较 `base=250/500/1000ms` 和不同 hysteresis release gap。

## 6. 当前结论

动态 retry hint 的代码链路已经成立：

```text
adaptive controller
-> ServiceOverloadedError{RetryDelay}
-> gRPC RetryInfo
-> loadtest retry loop
```

本轮不是容量结果。下一轮应跑 adaptive recent+hysteresis 阈值矩阵。
