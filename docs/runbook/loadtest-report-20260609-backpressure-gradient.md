# NexusIM SendMessage Backpressure Gradient Loadtest Report

日期：2026-06-09

## 1. 压测目标

上一份 on/off 报告确认：backpressure 能快速拒绝过载请求，但 `MinAvailableConns=0` 仍然过于靠近连接池打满点。本轮继续跑梯度：

```text
NEXUSIM_PG_BACKPRESSURE_MIN_AVAILABLE_CONNS = 0 / 4 / 8 / 16
```

目标是找出更合理的保护阈值，并观察：

- accepted RPS 是否稳定。
- 成功请求 `success_p99_ms` 是否下降。
- 错误请求是否主要变成可识别的 `SERVICE_OVERLOADED`。
- 是否还能避免 outbox backlog。

## 2. 压测拓扑

```text
loadtest/sendmessage
-> message-service gRPC
-> repository backpressure check
-> PostgreSQL pool acquire
-> PostgreSQL local transaction
-> message_outbox
-> outbox relay
-> Kafka
```

本轮仍是 Windows 本机压测，不包含 MacBook 客户端。

## 3. 环境配置

| 项 | 配置 |
| --- | --- |
| Git commit | `dfb6776` |
| Git dirty | `false` |
| Docker Desktop | 约 16 CPU / 24GB memory |
| PostgreSQL profile | `deploy/local/docker-compose.postgres-loadtest.yml` |
| PostgreSQL `max_connections` | `200` |
| PostgreSQL `shared_buffers` | `1GB` |
| PostgreSQL `max_wal_size` | `4GB` |
| message-service PG pool | `NEXUSIM_PG_MAX_CONNS=64` |
| relay workers | `8` |
| relay batch size | `500` |
| 压测时长 | `60s` |
| stats wait | `30s` |
| conversation count | `3000` |

## 4. 执行命令

每个阈值分别执行一次：

```powershell
. .\tools\go-env.ps1
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 64 `
  -VUs 1200,1600 `
  -Duration 60s `
  -StatsWait 30s `
  -ConversationCount 3000 `
  -RelayWorkers 8 `
  -BatchSize 500 `
  -BackpressureEnabled `
  -BackpressureMinAvailableConns <0|4|8|16> `
  -ResultRoot loadtest\results\backpressure-minavail-<N>-formal-20260609
```

结果目录：

```text
loadtest/results/backpressure-minavail-0-formal-20260609/
loadtest/results/backpressure-minavail-4-formal-20260609/
loadtest/results/backpressure-minavail-8-formal-20260609/
loadtest/results/backpressure-minavail-16-formal-20260609/
```

## 5. 核心结果

| MinAvailable | VU | 请求数 | 成功率 | accepted RPS | overload rate | overall p99 | success p99 | error p99 | outbox pending |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 0 | 1200 | 1,042,898 | 2.22% | 385.68 | 95.63% | 2000.72ms | 1867.66ms | 2000.74ms | 0 |
| 4 | 1200 | 3,941,686 | 1.29% | 847.98 | 98.71% | 67.70ms | 1276.41ms | 40.77ms | 0 |
| 8 | 1200 | 3,267,222 | 1.50% | 818.73 | 98.50% | 79.47ms | 1191.10ms | 49.15ms | 0 |
| 16 | 1200 | 3,955,678 | 1.14% | 753.87 | 98.86% | 57.41ms | 1072.60ms | 41.66ms | 0 |
| 0 | 1600 | 100,690 | 2.46% | 41.22 | 51.55% | 2025.26ms | 1997.43ms | 2025.51ms | 0 |
| 4 | 1600 | 3,603,928 | 1.42% | 852.08 | 98.58% | 83.70ms | 1322.41ms | 53.44ms | 0 |
| 8 | 1600 | 3,499,890 | 1.60% | 933.93 | 98.40% | 81.61ms | 1249.85ms | 54.82ms | 0 |
| 16 | 1600 | 2,806,779 | 1.40% | 654.78 | 98.36% | 100.58ms | 1988.95ms | 58.42ms | 0 |

## 6. 瓶颈排查过程

### 6.1 为什么不直接看 overall p99

`overall p99` 包含成功请求和错误请求。阈值越激进，错误请求越多，压测器越快收到 `SERVICE_OVERLOADED`，所以 overall p99 可能变得很好看。

例如：

```text
MinAvailable=16 / VU1200
overall_p99=57.41ms
success_p99=1072.60ms
```

这不是成功请求已经 57ms，而是绝大多数请求在错误路径快速返回。

### 6.2 先看错误是否稳定可解释

`MinAvailable=4/8` 的错误几乎全部是 `SERVICE_OVERLOADED`：

```text
MinAvailable=4 / VU1200: SERVICE_OVERLOADED=3,890,807
MinAvailable=4 / VU1600: SERVICE_OVERLOADED=3,552,803
MinAvailable=8 / VU1200: SERVICE_OVERLOADED=3,218,098
MinAvailable=8 / VU1600: SERVICE_OVERLOADED=3,443,854
```

这说明请求主要在进入事务前被拦住，错误语义稳定，客户端可以按 retryable overload 处理。

`MinAvailable=0` 的复跑出现了明显不稳定：

```text
VU1200:
SERVICE_OVERLOADED=997,306
DeadlineExceeded=15,666
DB_WRITE_FAILED=2,721

VU1600:
SERVICE_OVERLOADED=51,903
DeadlineExceeded=34,236
DB_WRITE_FAILED=4,457
```

这说明等连接池接近完全打满才拒绝已经太晚，请求会进入更深的超时/DB 写失败路径，错误语义变差。

### 6.3 再看 accepted RPS 和 success p99

`MinAvailable=4/8` 在本轮里更接近平衡点：

```text
VU1200:
min=4 accepted_rps=847.98 success_p99=1276.41ms
min=8 accepted_rps=818.73 success_p99=1191.10ms

VU1600:
min=4 accepted_rps=852.08 success_p99=1322.41ms
min=8 accepted_rps=933.93 success_p99=1249.85ms
```

`MinAvailable=16` 更保守，但收益不稳定：

```text
VU1200 success_p99=1072.60ms accepted_rps=753.87
VU1600 success_p99=1988.95ms accepted_rps=654.78
```

所以它不能直接作为更优阈值，只能说明更早拒绝可以降低部分场景下的成功 p99，但会明显牺牲 accepted throughput。

### 6.4 outbox backlog

四个阈值在本轮 summary 的 `outbox_pending_count` 均为 0。原因不是 relay 绝对能力变强，而是大量请求被拒绝后，实际 accepted writes 降到约 650-930 RPS，低于当前 relay 追平能力。

## 7. 当前结论

1. `MinAvailable=0` 不适合作为正式策略，拒绝太晚，容易出现 DeadlineExceeded 和 DB_WRITE_FAILED。
2. `MinAvailable=4/8` 是当前更合理的保守候选：错误语义稳定，outbox 不积压，accepted RPS 约 820-934。
3. `MinAvailable=16` 过于保守，成功吞吐下降明显，且 1600 VU 的 success p99 没有稳定改善。
4. 当前 backpressure 本质是保护数据库和 outbox，不是提升成功请求性能；用户体验上必须让客户端遵守 gRPC `RetryInfo`，并叠加指数退避和 jitter。
5. 下一阶段不能继续只调静态阈值，应设计 adaptive limit。

## 8. 下一步

- 短期推荐实验阈值：`MinAvailableConns=8`。
- 让客户端遵守 gRPC `RetryInfo=500ms`，并叠加指数退避和 jitter；后续再让 retry delay 随 adaptive limit 动态调整。
- 设计 adaptive limit：
  - 输入：pool acquire p95/p99、acquired conns、timeout/error rate、SERVICE_OVERLOADED rate、outbox pending、PostgreSQL wait_event。
  - 输出：动态 accepted concurrency / min available conns。
  - 目标：维持 accepted RPS，降低 success p99，并避免错误退化成 DeadlineExceeded / DB_WRITE_FAILED。
- 继续优化 repository 成功路径和 outbox relay，否则 backpressure 只能把过载显式化，不能提高真实容量。
