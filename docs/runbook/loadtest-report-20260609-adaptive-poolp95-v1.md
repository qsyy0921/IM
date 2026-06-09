# NexusIM SendMessage Adaptive Pool Acquire P95 Matrix V1

## 1. 压测目标

上一轮 60s repeat 证明 `gap8-outbox50-base1000` 可以让 outbox 清零，但 accepted RPS 过低。本阶段只调整 `AdaptiveMaxPoolAcquireP95`，验证放宽 PG acquire p95 阈值能否提升 accepted RPS，同时保持 outbox 不积压。

## 2. 压测拓扑

```text
Windows loadtest
-> message-service gRPC
-> app adaptive admission
-> PostgreSQL local transaction
-> outbox relay
-> Kafka
```

服务端、客户端、PostgreSQL、Kafka 都在 Windows 本机。PostgreSQL 和 Kafka 运行在 Docker Desktop，gRPC 和 relay 为本机进程。

## 3. 固定配置

| 项 | 值 |
| --- | --- |
| commit | `b98e3c9` |
| git_dirty | `false` |
| PG_MAX_CONNS | `64` |
| VU | `1200`, `1600` |
| duration | `60s` |
| stats_wait | `30s` |
| relay workers | `8` |
| batch size | `100` |
| PublishBatch | enabled |
| AdaptiveMinAvailableConns | `8` |
| AdaptiveReleaseAvailableConns | `16` |
| AdaptiveMaxOutboxPending | `20000` |
| AdaptiveReleaseOutboxPending | `10000` |
| AdaptiveRetryBaseDelay | `1000ms` |
| AdaptiveRetryMaxDelay | `2s` |
| max_retries | `2` |
| retry_jitter | `100ms` |

结果目录：

```text
loadtest/results/adaptive-poolp95-v1-20260609/
```

## 4. 核心结果

| AdaptiveMaxPoolAcquireP95 | VU | logical success | accepted RPS | logical p99 | logical success p99 | attempt success p99 | overload rate | outbox pending |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 250ms | 1200 | 0.3796 | 248.95 | 6229.08ms | 6599.63ms | 1547.58ms | 0.1973 | 0 |
| 500ms | 1200 | 0.3866 | 250.55 | 5499.87ms | 5583.24ms | 1979.16ms | 0.2099 | 0 |
| 750ms | 1200 | 0.4370 | 277.55 | 5968.57ms | 6416.50ms | 1976.82ms | 0.2157 | 0 |
| 250ms | 1600 | 0.2449 | 202.68 | 5462.60ms | 5525.89ms | 1965.97ms | 0.1545 | 0 |
| 500ms | 1600 | 0.2366 | 195.82 | 5229.32ms | 5252.23ms | 1273.26ms | 0.1793 | 0 |
| 750ms | 1600 | 0.2285 | 195.63 | 5091.10ms | 4899.22ms | 1583.83ms | 0.1625 | 0 |

压测后全局 outbox：

```text
PUBLISHED=2495207
PENDING=0
DLQ=0
```

## 5. 瓶颈排查过程

1. 先看 outbox：所有组合 `outbox_pending_count=0`，全局 DB 也没有 PENDING/DLQ，说明 relay/Kafka 不是本轮瓶颈。
2. 再看 attempt success p99：部分组合看起来只有 `1.2s-2.0s`，但这是最终成功 attempt，不包含 retry sleep。
3. 再看 logical p99：所有组合都在 `5.0s-6.6s`，说明用户层等待非常长。
4. 对比 P95 阈值：1200 VU 下从 250ms 放宽到 750ms accepted RPS 从 `248.95` 增至 `277.55`，但提升有限；1600 VU 下没有改善。
5. 检查 recent sample count：`repository_pool_acquire_recent_sample_count=4096`，relay active recent count `1085-1870`，不是样本不足导致的结论。

## 6. 当前结论

单纯放宽 `AdaptiveMaxPoolAcquireP95` 不能解决当前过度保护问题。当前策略虽然能保护 outbox 清零，但 logical p99 太高，accepted RPS 也偏低。

下一步应转向 admission 策略本身：

- 不要只按 reason count 线性放大 retry delay。
- 增加服务端 admission token / concurrency limit，让 accepted RPS 更平滑。
- 把 `RetryInfo` 生成从 `reason_count * base_delay` 改为结合 accepted RPS、pool acquire recent p95、outbox drain rate 的动态策略。
- 后续矩阵必须以 logical latency 作为主口径，而不是 attempt p99。
