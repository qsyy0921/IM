# NexusIM SendMessage Adaptive Best Candidate Repeat

## 1. 压测目标

`adaptive threshold matrix v1` 中 `gap8-outbox50-base1000` 在 30s 矩阵里表现最好。本阶段对该候选做 60s 重复验证，判断它是否稳定。

## 2. 压测拓扑

```text
Windows loadtest
-> message-service gRPC with app adaptive admission
-> PostgreSQL local transaction
-> outbox relay
-> Kafka
```

服务端、客户端、PostgreSQL、Kafka 都在 Windows 本机。PostgreSQL 和 Kafka 运行在 Docker Desktop，gRPC 和 relay 为本机进程。

## 3. 固定配置

| 项 | 值 |
| --- | --- |
| commit | `fb872a9` |
| git_dirty | `false` |
| PG_MAX_CONNS | `64` |
| VU | `1200`, `1600` |
| duration | `60s` |
| stats_wait | `30s` |
| conversation_count | `1000` |
| relay workers | `8` |
| batch size | `100` |
| PublishBatch | enabled |
| AdaptiveMinAvailableConns | `8` |
| AdaptiveReleaseAvailableConns | `16` |
| AdaptiveMaxPoolAcquireP95 | `250ms` |
| AdaptiveMaxOutboxPending | `20000` |
| AdaptiveReleaseOutboxPending | `10000` |
| AdaptiveRetryBaseDelay | `1000ms` |
| AdaptiveRetryMaxDelay | `2s` |
| max_retries | `2` |
| retry_jitter | `100ms` |

结果目录：

```text
loadtest/results/adaptive-threshold-best-repeat-20260609/
```

## 4. 核心结果

| round | VU | logical success | accepted RPS | success p99 | overload rate | retry delay p95 | outbox pending |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| r1 | 1200 | 0.3901 | 261.55 | 1897.89ms | 0.2005 | 2094.23ms | 0 |
| r2 | 1200 | 0.4169 | 273.58 | 1959.22ms | 0.2053 | 2092.86ms | 0 |
| r1 | 1600 | 0.2796 | 226.45 | 1902.79ms | 0.1989 | 2094.80ms | 0 |
| r2 | 1600 | 0.3059 | 246.82 | 1987.29ms | 0.2231 | 2093.82ms | 0 |

压测后全局 outbox：

```text
PUBLISHED=2394747
PENDING=0
DLQ=0
```

## 5. 瓶颈排查过程

1. 对比 30s 矩阵：`gap8-outbox50-base1000` 在 30s 下 1200 VU logical success `80.78%`、accepted RPS `578.30`；60s 重复后 1200 VU 只有 `39.01%-41.69%`、accepted RPS `261.55-273.58`。
2. 检查 outbox：所有 60s 重复结果 `outbox_pending_count=0`，全局 DB 也只有 `PUBLISHED`，说明 relay/Kafka 追平不是本轮瓶颈。
3. 检查 recent sample count：`repository_pool_acquire_recent_sample_count=4096`，relay active recent count `1491-1811`，不是样本不足。
4. 检查 retry delay：`retry_delay_p95_ms` 接近 `2.1s`，说明客户端被长期建议等待，admission 保护过强。
5. 检查 success p99：成功 attempt p99 仍约 `1.9s-2.0s`，即使大量拒绝也没有明显改善成功写入尾延迟。

## 6. 当前结论

30s 矩阵里的最佳候选不能直接作为稳定配置。`gap8-outbox50-base1000` 能保护 outbox 清零，但 60s 下 accepted RPS 过低，属于过度保护。

下一轮不应继续只调 retry delay。更应该放宽或重新定义 PG acquire 阈值，例如比较：

```text
AdaptiveMaxPoolAcquireP95 = 250ms / 500ms / 750ms
```

同时必须新增 logical end-to-end latency。当前 `success_p99_ms` 是最终成功 attempt 的耗时，不包含 retry sleep，不能代表用户看到的端到端等待。
