# NexusIM SendMessage Backpressure On/Off Loadtest Report

日期：2026-06-09

## 1. 压测目标

本轮目标不是证明 backpressure 能提高成功请求性能，而是验证：

- PostgreSQL pool backpressure 是否能在过载时快速拒绝请求。
- 快速拒绝是否能保护 outbox backlog 和数据库连接池。
- 压测报告是否能区分成功请求延迟和错误请求延迟，避免把大量快速拒绝后的 mixed p99 当成成功写入体验。

本轮对比 `backpressure off` 与 `backpressure on`，固定 `PG_MAX_CONNS=64`，分别跑 `VU=1200` 和 `VU=1600`。

## 2. 压测拓扑

```text
loadtest/sendmessage
-> message-service gRPC
-> SendMessageUseCase
-> PostgreSQL local transaction
-> message_log / conversation_timeline_events / message_outbox
-> outbox relay
-> Kafka conversation.timeline.events
```

服务端、压测器、PostgreSQL、Kafka 都在 Windows 本机运行。本轮不是 Mac/Windows 双机压测。

## 3. 环境配置

| 项 | 配置 |
| --- | --- |
| Git commit | `6f0aa55` |
| Git dirty | `false` |
| Docker Desktop | 约 16 CPU / 24GB memory |
| PostgreSQL profile | `deploy/local/docker-compose.postgres-loadtest.yml` |
| PostgreSQL `max_connections` | `200` |
| PostgreSQL `shared_buffers` | `1GB` |
| PostgreSQL `max_wal_size` | `4GB` |
| PostgreSQL `checkpoint_timeout` | `900s` |
| PostgreSQL `synchronous_commit` | `on` |
| message-service PG pool | `NEXUSIM_PG_MAX_CONNS=64` |
| relay workers | `NEXUSIM_OUTBOX_WORKERS=8` |
| relay batch size | `NEXUSIM_OUTBOX_BATCH_SIZE=500` |

## 4. 执行命令

Backpressure off：

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
  -ResultRoot loadtest\results\backpressure-off-formal-20260609
```

Backpressure on：

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
  -BackpressureMinAvailableConns 0 `
  -ResultRoot loadtest\results\backpressure-on-formal-20260609
```

两组之间执行了额外 relay drain，确保上一组留下的 `message_outbox PENDING` 不影响下一组。

## 5. 结果摘要

| 模式 | VU | 请求数 | 成功率 | accepted RPS | overload rate | overall p99 | success p99 | error p99 | outbox pending |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| off | 1200 | 86,604 | 100.00% | 1443.40 | 0.00% | 1187.23ms | 1187.23ms | 0ms | 30,689 |
| off | 1600 | 79,932 | 100.00% | 1332.20 | 0.00% | 1735.38ms | 1735.38ms | 0ms | 47,736 |
| on | 1200 | 2,606,049 | 2.72% | 1180.30 | 97.28% | 958.22ms | 1403.95ms | 12.49ms | 5,554 |
| on | 1600 | 3,148,118 | 1.99% | 1045.90 | 98.01% | 1381.90ms | 1808.10ms | 14.26ms | 0 |

关键解释：

- `overall p99` 是所有请求的混合分位数，包含成功写入和错误返回。
- `success p99` 只看成功写入请求，才代表 SendMessage 被接受后的用户体验。
- `error p99` 只看错误请求，主要是 `SERVICE_OVERLOADED` 快速拒绝。

因此，backpressure on 的 `overall p99` 看起来比 off 低，但这主要来自 97% 以上请求被快速拒绝；成功请求的 `success p99` 没有改善。

## 6. 瓶颈排查过程

### 6.1 先看成功率和错误分布

backpressure off 时没有错误，所有请求都进入数据库事务路径：

```text
VU1200 success_rate=100%
VU1600 success_rate=100%
```

backpressure on 时大量请求返回 `SERVICE_OVERLOADED`：

```text
VU1200 overload_rate=97.28%
VU1600 overload_rate=98.01%
```

这说明 on/off 不能只比较 `overall p99`。开启 backpressure 后，请求数量暴涨是因为拒绝路径非常短，压测器能更快发起下一次请求；这不是服务成功吞吐提升。

### 6.2 再拆 success/error latency

backpressure on 的 `error p99` 只有十几毫秒，说明快速拒绝链路有效：

```text
VU1200 error_p99=12.49ms
VU1600 error_p99=14.26ms
```

但成功请求的 `success p99` 仍然较高：

```text
VU1200 success_p99=1403.95ms
VU1600 success_p99=1808.10ms
```

所以本轮不能得出“backpressure 改善成功请求 p99”的结论，只能得出“backpressure 能把绝大多数过载请求快速拒绝，并降低 outbox 积压”的结论。

### 6.3 最后定位成功请求慢在哪里

服务端 metrics 显示，成功请求的 p99 仍贴近 repository append / PostgreSQL pool acquire：

| 模式 | VU | repository pool acquire p99 | tx begin p99 | repository append p99 | commit p99 | Kafka publish p99 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| off | 1200 | 1117.19ms | 17.65ms | 1185.07ms | 37.08ms | 33.96ms |
| off | 1600 | 1675.89ms | 16.83ms | 1733.75ms | 55.26ms | 22.93ms |
| on | 1200 | 1330.35ms | 23.24ms | 953.98ms | 43.82ms | 38.82ms |
| on | 1600 | 1723.17ms | 25.51ms | 1377.22ms | 47.21ms | 39.96ms |

排查链路：

```text
request success p99
≈ SendMessage / repository append p99
≈ repository_pool_acquire p99
>> tx begin / insert / commit / Kafka publish p99
```

这说明当前成功请求的主等待段仍然是 PostgreSQL pool acquire，而不是 Kafka publish，也不是单条 insert SQL。

## 7. 当前结论

1. Backpressure 快速拒绝路径有效：`SERVICE_OVERLOADED` 的 error p99 在 12-15ms 左右。
2. Backpressure 能降低 outbox backlog：同等 VU 下 pending 明显低于 off。
3. Backpressure 当前策略过于粗糙：`MinAvailableConns=0` 会在 pool 接近满载时拒绝 97%-98% 请求，成功率过低。
4. Backpressure 没有改善成功请求尾延迟：成功请求仍然在 PostgreSQL pool acquire 上排队。
5. 后续报告必须同时展示 `overall p99`、`success p99`、`error p99`、`accepted RPS` 和 `overload rate`，不能只用混合 p99 做容量结论。

## 8. 下一步

- 做 backpressure 梯度：`MinAvailableConns=0/4/8/16`，寻找成功率、accepted RPS、success p99 和 outbox pending 的平衡点。
- 将当前瞬时 `MaxConns - AcquiredConns` 策略升级为 adaptive limit：结合 pool acquire p95/p99、等待队列、timeout/error rate、outbox pending 和 PostgreSQL wait_event。
- 继续优化成功写入路径：减少事务内 round trip，评估 `conversation_seq` 热点和 outbox 写入/update 带来的 WAL/dead tuple 压力。
- 单独优化 relay：批量 publish、批量 mark published、batch size/worker 梯度和 outbox autovacuum。

