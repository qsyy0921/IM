# NexusIM SendMessage Adaptive Threshold Matrix V1

## 1. 压测目标

本阶段验证 `message-service` app 层 adaptive admission 在 recent metrics、hysteresis 和 dynamic `RetryInfo` 都接入后的调参方向。

本轮不证明最终容量，只回答三个问题：

- `RetryInfo` base delay 对 logical success 和 retry storm 的影响。
- pool release gap `4/8/16` 是否明显影响吞吐和尾延迟。
- outbox release ratio `50%/25%` 是否影响 relay backlog 保护。

## 2. 压测拓扑

```text
Windows loadtest
-> message-service gRPC
-> app AdmissionPort
-> SendMessage use case
-> PostgreSQL local transaction
-> message_outbox
-> outbox relay
-> Kafka conversation.timeline.events
```

服务端、客户端、PostgreSQL、Kafka 都在 Windows 本机运行。PostgreSQL 和 Kafka 运行在 Docker Desktop，message-service gRPC、outbox relay 和 loadtest 为本机进程。

## 3. 固定配置

| 项 | 值 |
| --- | --- |
| commit | `9830f34` |
| git_dirty | `false` |
| PG_MAX_CONNS | `64` |
| relay workers | `8` |
| outbox batch size | `100` |
| PublishBatch | enabled |
| duration | `30s` |
| stats_wait | `20s` |
| VU | `1200`, `1600` |
| conversation_count | `1000` |
| retry_overloaded | enabled |
| max_retries | `2` |
| retry_jitter | `100ms` |

正式结果目录：

```text
loadtest/results/adaptive-threshold-v1-clean-20260609/
```

说明：第一次运行写到了 `loadtest/results/adaptive-threshold-v1-20260609/`，但当时使用 `-SkipBuild` 跑到了旧二进制，summary 没有 recent sample count 字段。该目录只作为无效诊断结果，不作为本报告证据。

## 4. 矩阵设计

| 配置 | MinAvailable | ReleaseAvailable | MaxOutboxPending | ReleaseOutboxPending | RetryBase |
| --- | ---: | ---: | ---: | ---: | ---: |
| `gap4-outbox50-base500` | 8 | 12 | 20000 | 10000 | 500ms |
| `gap8-outbox50-base500` | 8 | 16 | 20000 | 10000 | 500ms |
| `gap16-outbox50-base500` | 8 | 24 | 20000 | 10000 | 500ms |
| `gap8-outbox50-base250` | 8 | 16 | 20000 | 10000 | 250ms |
| `gap8-outbox50-base1000` | 8 | 16 | 20000 | 10000 | 1000ms |
| `gap8-outbox25-base500` | 8 | 16 | 20000 | 5000 | 500ms |

## 5. 核心结果

| config | VU | logical success | accepted RPS | success p99 | overload rate | retry delay p95 | outbox pending |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| gap8-outbox50-base1000 | 1200 | 0.8078 | 578.30 | 1955.41ms | 0.3634 | 2094.76ms | 0 |
| gap4-outbox50-base500 | 1200 | 0.6706 | 511.37 | 1991.14ms | 0.4014 | 1094.11ms | 0 |
| gap8-outbox50-base500 | 1200 | 0.6566 | 483.87 | 1984.98ms | 0.4051 | 1093.29ms | 0 |
| gap8-outbox50-base250 | 1200 | 0.6262 | 477.03 | 1973.77ms | 0.4091 | 594.60ms | 0 |
| gap8-outbox25-base500 | 1200 | 0.5835 | 442.23 | 1988.34ms | 0.3707 | 1093.49ms | 0 |
| gap16-outbox50-base500 | 1200 | 0.5562 | 423.70 | 1979.44ms | 0.3640 | 1091.95ms | 0 |
| gap8-outbox50-base1000 | 1600 | 0.5628 | 456.07 | 1819.91ms | 0.3375 | 2093.00ms | 0 |
| gap8-outbox25-base500 | 1600 | 0.4892 | 415.93 | 1974.38ms | 0.4153 | 1095.14ms | 0 |
| gap4-outbox50-base500 | 1600 | 0.4487 | 397.70 | 1766.49ms | 0.3553 | 1094.58ms | 0 |
| gap16-outbox50-base500 | 1600 | 0.4272 | 368.30 | 1699.42ms | 0.3657 | 1094.88ms | 0 |
| gap8-outbox50-base500 | 1600 | 0.3864 | 334.57 | 1642.63ms | 0.3597 | 1092.82ms | 0 |
| gap8-outbox50-base250 | 1600 | 0.3376 | 320.50 | 1826.74ms | 0.4373 | 594.74ms | 0 |

矩阵结束后全局 outbox：

```text
PUBLISHED=2322160
PENDING=0
DLQ=0
```

## 6. 瓶颈排查过程

1. 先看 overall p99：所有组合接近 `2s`，但这是 gRPC attempt 口径，混入了 overload 和 retry，不代表成功写入体验。
2. 再看 `success_p99_ms` 与 `error_p99_ms`：错误 attempt p99 仍约 `2s`，成功 attempt p99 也在 `1.6s-2.0s`，说明当前 adaptive 策略没有把成功请求尾延迟压低。
3. 检查 outbox：所有组合 `outbox_pending_count=0`，DB 结束状态也只有 `PUBLISHED`，因此本轮瓶颈不是 relay backlog。
4. 检查 sample count：`repository_pool_acquire_recent_sample_count=4096`，relay active recent count 约 `988-1833`，说明 recent 指标不是 warm-up 空样本。
5. 对比变量：`RetryBase=1000ms` 在 1200/1600 VU 下 logical success 和 accepted RPS 都最高，说明更长 retry hint 能缓解同步重试压力；`250ms` 过短，overload rate 更高。

## 7. 当前结论

- `gap8-outbox50-base1000` 是本轮最好的候选：1200 VU logical success `80.78%`，1600 VU logical success `56.28%`，outbox pending 均为 0。
- pool release gap `4/8/16` 没有表现出稳定收益；比起 release gap，retry base delay 对结果影响更大。
- outbox release ratio 从 `50%` 改为 `25%` 没有明显改善本轮结果；当前 outbox 可追平，不是主瓶颈。
- 当前 adaptive admission 是保护机制，不是容量提升机制；它让 outbox 不积压，但 accepted RPS 明显低于之前宽松配置。

## 8. 下一步

- 以 `gap8-outbox50-base1000` 作为下一轮候选，做 60s 重复验证。
- 对比更宽松的 pool acquire p95 阈值，例如 `250ms / 500ms / 750ms`，观察是否能提升 accepted RPS 且保持 outbox pending 为 0。
- 增加 logical end-to-end latency 统计，把 retry sleep 计入用户层等待时间；当前 `success_p99_ms` 只是最终成功 attempt 延迟。
- 后续报告继续展示 `retry_delay_count` 与 `retry_attempt_count`，避免把计划等待次数误读成真实重试次数。
