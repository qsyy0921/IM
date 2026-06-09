# NexusIM message-service Adaptive In-Flight Limit v1

## 压测目标

验证 app 层 admission token / concurrency limit 是否比上一轮纯指标阈值策略更适合保护 `SendMessage` 主链路。

本轮只看第一阶段真实链路：

```text
loadtest client
-> message-service gRPC
-> app admission token
-> SendMessage use case
-> PostgreSQL local transaction
-> message_outbox
-> outbox relay
-> Kafka conversation.timeline.events
```

## 环境与拓扑

| 项 | 配置 |
| --- | --- |
| 服务端 | Windows 本机 |
| 客户端 | Windows 本机 loadtest |
| PostgreSQL / Kafka | Docker Desktop 本机容器 |
| Docker 资源 | Windows Docker Desktop 已配置 16 CPU / 24GB memory |
| PostgreSQL profile | loadtest profile，PostgreSQL `max_connections=200`；服务侧 `NEXUSIM_PG_MAX_CONNS=64` |
| Relay | workers `8`，batch size `100`，PublishBatch enabled |
| Adaptive | 只启用 `NEXUSIM_ADAPTIVE_MAX_IN_FLIGHT`；PG pool / outbox / relay 指标阈值全部关闭 |
| Client retry | `--retry-overloaded --max-retries=2 --retry-jitter=100ms` |

## 执行方式

先提交代码切片，避免 dirty 结果误导：

```text
7bf37fe feat: add adaptive in-flight admission limit
```

clean smoke：

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 8 `
  -VUs 20 `
  -Duration 5s `
  -StatsWait 10s `
  -ConversationCount 100 `
  -RelayWorkers 8 `
  -BatchSize 100 `
  -ResultRoot loadtest\results\adaptive-inflight-smoke-clean-20260609 `
  -AdaptiveLimitEnabled `
  -AdaptiveMaxInFlight 4 `
  -AdaptiveMinAvailableConns 0 `
  -AdaptiveReleaseAvailableConns 0 `
  -AdaptiveMaxPoolAcquireP95 0s `
  -AdaptiveMaxOutboxPending 0 `
  -AdaptiveReleaseOutboxPending 0 `
  -AdaptiveMaxRelayActiveP95 0s `
  -AdaptiveMinOutboxFetchedPerCall 0 `
  -AdaptiveMinKafkaRecordsPerCall 0 `
  -AdaptiveRetryBaseDelay 500ms `
  -AdaptiveRetryMaxDelay 2s `
  -RetryOverloaded `
  -MaxRetries 1 `
  -RetryJitter 50ms
```

30s 梯度矩阵按 `AdaptiveMaxInFlight=64/128/256/512` 分别运行：

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 64 `
  -VUs 1200,1600 `
  -Duration 30s `
  -StatsWait 20s `
  -ConversationCount 1000 `
  -RelayWorkers 8 `
  -BatchSize 100 `
  -ResultRoot loadtest\results\adaptive-inflight-v1-20260609\inflight-<cap> `
  -AdaptiveLimitEnabled `
  -AdaptiveMaxInFlight <cap> `
  -AdaptiveMinAvailableConns 0 `
  -AdaptiveReleaseAvailableConns 0 `
  -AdaptiveMaxPoolAcquireP95 0s `
  -AdaptiveMaxOutboxPending 0 `
  -AdaptiveReleaseOutboxPending 0 `
  -AdaptiveMaxRelayActiveP95 0s `
  -AdaptiveMinOutboxFetchedPerCall 0 `
  -AdaptiveMinKafkaRecordsPerCall 0 `
  -AdaptiveRetryBaseDelay 500ms `
  -AdaptiveRetryMaxDelay 2s `
  -RetryOverloaded `
  -MaxRetries 2 `
  -RetryJitter 100ms
```

60s 重复验证只保留两个候选：`64` 和 `128`。

结果路径：

```text
loadtest/results/adaptive-inflight-smoke-clean-20260609/
loadtest/results/adaptive-inflight-v1-20260609/
loadtest/results/adaptive-inflight-repeat-20260609/
```

## clean smoke

| 指标 | 值 |
| --- | ---: |
| commit | `7bf37fe` |
| git_dirty | `false` |
| logical_request_count | 2932 |
| logical_success_rate | 0.9529 |
| request_count | 3076 |
| attempt success_rate | 0.9083 |
| service_overloaded_count | 282 |
| accepted_rps | 558.8 |
| attempt success p99 | 10.95ms |
| logical p99 | 542.41ms |
| retry_delay p95 | 545.71ms |
| outbox_pending_count | 0 |

smoke 结论：`MaxInFlight=4` 能在 app 入口拒绝过量请求，拒绝请求不会写 `message_log / timeline / outbox`；成功请求仍能落库并经 relay 发布到 Kafka。

## 30s 梯度结果

| MaxInFlight | VU | logical success | accepted RPS | logical p99 | logical success p99 | attempt success p99 | attempt overload rate | outbox pending |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 64 | 1200 | 0.8537 | 2019.90 | 2199.70ms | 2197.77ms | 62.85ms | 0.4250 | 0 |
| 64 | 1600 | 0.7944 | 1966.73 | 2201.93ms | 2205.47ms | 60.51ms | 0.5098 | 0 |
| 128 | 1200 | 0.8595 | 2033.70 | 2198.53ms | 2195.92ms | 61.12ms | 0.4180 | 0 |
| 128 | 1600 | 0.7855 | 1854.37 | 2206.90ms | 2212.82ms | 113.87ms | 0.5236 | 0 |
| 256 | 1200 | 0.8580 | 1968.57 | 2201.77ms | 2204.18ms | 67.19ms | 0.4247 | 0 |
| 256 | 1600 | 0.7911 | 1893.70 | 2204.21ms | 2211.16ms | 70.36ms | 0.5143 | 0 |
| 512 | 1200 | 0.8630 | 1985.57 | 2201.91ms | 2203.11ms | 160.54ms | 0.4197 | 0 |
| 512 | 1600 | 0.7951 | 1881.93 | 2204.00ms | 2208.46ms | 177.92ms | 0.5136 | 0 |

## 60s 候选重复验证

| MaxInFlight | VU | logical success | accepted RPS | logical p99 | attempt success p99 | overload rate | outbox pending | relay active p95 |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 64 | 1200 | 0.8604 | 1922.23 | 2200.80ms | 63.58ms | 0.4326 | 0 | 148.85ms |
| 64 | 1600 | 0.8021 | 1926.97 | 2203.44ms | 63.35ms | 0.5115 | 0 | 136.43ms |
| 128 | 1200 | 0.8406 | 1763.95 | 2205.62ms | 81.69ms | 0.4530 | 0 | 129.94ms |
| 128 | 1600 | 0.8025 | 1893.75 | 2206.48ms | 76.89ms | 0.5114 | 0 | 146.89ms |

## 瓶颈排查过程

上一轮 `AdaptiveMaxPoolAcquireP95=250/500/750ms` 矩阵中，outbox 能清零，但 accepted RPS 只有约 `195-278`，logical p99 约 `5s`。这说明纯指标阈值会过早进入 recovering 状态，拒绝太多请求。

本轮为了隔离变量，关闭 PG pool p95、outbox pending、relay active p95、fetched per call 和 Kafka records per call 等指标阈值，只保留 app 入口 `MaxInFlight`。如果 accepted RPS 回升且 PostgreSQL pool acquire p95 不再贴近请求 p99，说明主要问题不是 PostgreSQL 单条写入，而是 admission 策略过度保护。

观察结果：

- `repository_pool_acquire_recent_p95_ms` 在所有 60s 候选中为 `0ms`，说明 token gate 把进入 DB 事务的并发压到了 pgxpool 可承受范围内。
- `outbox_pending_count=0`，说明 relay 在当前 workers/batch/publishBatch 配置下能追平。
- `attempt success p99` 在 `MaxInFlight=64` 下约 `63ms`，明显低于无 admission 时的秒级尾延迟。
- `logical p99` 仍约 `2.2s`，和 `RetryInfo` / `max_retries=2` 的等待策略高度一致；它不是本轮 DB 写入 p99。
- `MaxInFlight=512` 的 attempt success p99 明显升高，说明上限过大时会把压力重新推回成功路径。

因此当前瓶颈从“PG pool acquire 等待”转移为“admission + retry 策略如何在保护服务的同时减少用户层等待和最终失败”。token gate 解决了单次写入尾延迟和 outbox 追平，但还没有解决 logical p99。

## 当前结论

- `MaxInFlight=64` 是下一轮更稳的本地候选：60s 下 1200/1600 VU 的 accepted RPS 都约 `1.92k`，success p99 约 `63ms`，outbox pending 为 0。
- `MaxInFlight=128` 不稳定，1200 VU 下 accepted RPS 反而低于 64，success p99 更高。
- token gate 相比纯阈值策略显著提升 accepted RPS，但客户端 logical p99 仍被 retry delay 拉到约 `2.2s`。
- 这不是最终容量结论；它只说明下一轮应围绕 `MaxInFlight=64` 调整 `RetryBaseDelay / MaxRetries / jitter`，而不是继续放宽 PG pool p95 阈值。

## 下一步

1. 以 `MaxInFlight=64` 为基线，跑 `RetryBaseDelay=250ms/500ms/1000ms` 与 `MaxRetries=1/2` 的小矩阵。
2. 报告继续同时展示 `logical p99`、`logical success rate`、`accepted RPS`、`attempt success p99`、`overload rate` 和 `outbox_pending_count`。
3. 如果 retry 策略仍导致 logical p99 过高，再考虑 adaptive token 上限，而不是固定 `MaxInFlight`。
