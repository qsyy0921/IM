# Outbox Batch / Worker Matrix 2026-06-09

## 1. 压测目标

在 `PublishBatch=true` 已验证可降低 outbox backlog 后，继续验证 outbox relay 的两个关键参数：

```text
NEXUSIM_OUTBOX_BATCH_SIZE
NEXUSIM_OUTBOX_WORKERS
```

目标是找出下一轮 adaptive limit 前更稳的 relay 配置候选，并确认是否继续盲目增加 worker。

本轮不是最终容量结论。每个组合只跑 1 次，用于筛选候选；下一轮需要对候选组合重复运行。

## 2. 压测拓扑

```text
loadtest/sendmessage
-> message-service gRPC
-> PostgreSQL local transaction
-> message_outbox
-> outbox relay PublishBatch
-> Kafka conversation.timeline.events
```

服务端、relay、压测器均在 Windows 本机运行；PostgreSQL 和 Kafka 运行在本机 Docker Desktop。

## 3. 环境和固定参数

```text
commit: 134613c
git_dirty: false
PostgreSQL profile: loadtest override
PG_MAX_CONNS: 64
PublishBatch: true
backpressure: enabled
backpressure min available conns: 8
client retry: enabled
max_retries: 2
retry_jitter: 100ms
duration: 30s
stats_wait: 20s
conversation_count: 1000
```

变量：

```text
batch_size: 100 / 500 / 1000
relay_workers: 8 / 12 / 16
VUs: 1200 / 1600
```

## 4. 执行入口

本轮新增矩阵脚本：

```text
loadtest/sendmessage/run-local-outbox-batch-worker-matrix.ps1
```

执行命令：

```powershell
. .\tools\go-env.ps1
.\loadtest\sendmessage\run-local-outbox-batch-worker-matrix.ps1 `
  -BatchSizes 100,500,1000 `
  -RelayWorkers 8,12,16 `
  -VUs 1200 `
  -PGMaxConns 64 `
  -Duration 30s `
  -StatsWait 20s `
  -ConversationCount 1000 `
  -PublishBatchEnabled:$true `
  -BackpressureEnabled `
  -BackpressureMinAvailableConns 8 `
  -RetryOverloaded `
  -MaxRetries 2 `
  -RetryJitter 100ms `
  -ResultRoot loadtest\results\outbox-batch-worker-matrix-1200-formal-20260609
```

1600 VU 只把 `-VUs 1200` 改为 `-VUs 1600`，结果根目录改为：

```text
loadtest\results\outbox-batch-worker-matrix-1600-formal-20260609
```

## 5. 结果文件

```text
loadtest/results/outbox-batch-worker-matrix-1200-formal-20260609/outbox-batch-worker-matrix-summary.json
loadtest/results/outbox-batch-worker-matrix-1600-formal-20260609/outbox-batch-worker-matrix-summary.json
```

矩阵结束后 PostgreSQL 状态：

```text
PUBLISHED|996037
```

没有遗留 `PENDING` / `DLQ`。

## 6. 1200 VU 结果

| batch | workers | logical success | accepted RPS | overload rate | success p99 ms | pending | records/call | Kafka call ms | record estimate ms | process ready ms |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 100 | 8 | 0.7398 | 1930.83 | 0.5829 | 541.51 | 0 | 41.46 | 57.05 | 5.73 | 50.64 |
| 100 | 12 | 0.7467 | 1876.77 | 0.5858 | 545.34 | 0 | 29.04 | 43.97 | 5.90 | 40.05 |
| 100 | 16 | 0.7434 | 1878.37 | 0.5867 | 551.72 | 0 | 23.33 | 36.67 | 5.39 | 34.20 |
| 500 | 8 | 0.7619 | 1969.90 | 0.5717 | 581.22 | 0 | 42.95 | 53.78 | 4.58 | 47.24 |
| 500 | 12 | 0.7436 | 1909.70 | 0.5836 | 558.09 | 0 | 32.53 | 47.25 | 5.66 | 41.45 |
| 500 | 16 | 0.7235 | 1813.57 | 0.5992 | 536.80 | 0 | 24.08 | 34.95 | 4.90 | 34.98 |
| 1000 | 8 | 0.7371 | 1875.77 | 0.5886 | 611.56 | 0 | 40.90 | 51.47 | 4.91 | 48.01 |
| 1000 | 12 | 0.7387 | 1902.37 | 0.5856 | 583.84 | 0 | 31.48 | 44.38 | 5.27 | 40.43 |
| 1000 | 16 | 0.7410 | 1849.50 | 0.5901 | 583.57 | 0 | 23.77 | 38.28 | 5.61 | 35.59 |

## 7. 1600 VU 结果

| batch | workers | logical success | accepted RPS | overload rate | success p99 ms | pending | records/call | Kafka call ms | record estimate ms | process ready ms |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 100 | 8 | 0.6640 | 1873.23 | 0.6618 | 667.54 | 0 | 39.86 | 52.79 | 4.79 | 49.09 |
| 100 | 12 | 0.6436 | 1767.10 | 0.6762 | 799.74 | 0 | 29.88 | 44.74 | 5.23 | 41.68 |
| 100 | 16 | 0.6282 | 1678.00 | 0.6881 | 825.92 | 0 | 20.17 | 30.61 | 4.72 | 34.03 |
| 500 | 8 | 0.6515 | 1795.60 | 0.6715 | 793.76 | 0 | 39.23 | 50.38 | 5.15 | 47.76 |
| 500 | 12 | 0.6406 | 1722.03 | 0.6807 | 885.65 | 0 | 28.84 | 43.68 | 5.28 | 40.98 |
| 500 | 16 | 0.6283 | 1696.13 | 0.6860 | 926.74 | 0 | 22.53 | 32.56 | 4.79 | 35.91 |
| 1000 | 8 | 0.6635 | 1822.03 | 0.6659 | 845.21 | 0 | 43.66 | 52.42 | 4.78 | 47.73 |
| 1000 | 12 | 0.6472 | 1762.03 | 0.6750 | 928.01 | 0 | 31.50 | 44.11 | 5.45 | 43.12 |
| 1000 | 16 | 0.6310 | 1706.67 | 0.6838 | 953.60 | 0 | 25.42 | 37.82 | 5.46 | 36.30 |

## 8. 瓶颈排查过程

1. 先看 outbox pending。
   两个 VU 档位、9 个组合全部在 `stats_wait=20s` 后 pending 为 0，说明当前 `PublishBatch=true` 下 relay 追平能力已经足够覆盖这组压力。

2. 再看 accepted RPS 和 logical success。
   1200 VU 下 batch 500 / workers 8 的 accepted RPS 最高，为 `1969.90`；1600 VU 下 batch 100 / workers 8 的 accepted RPS 最高，为 `1873.23`。

3. 再看 success p99。
   1600 VU 下 workers 8 明显比 12/16 更稳。batch 100 / workers 8 的 success p99 为 `667.54ms`，是 1600 VU 全矩阵最低。

4. 再看 records/call 和 Kafka call latency。
   worker 数从 8 增加到 16 后，records/call 从约 40 降到约 20-25，单次 Kafka call latency 也下降。这不是纯收益，因为更多 worker 把 batch 拆小，同时增加并发竞争。

5. 最后看 relay process ready。
   worker 越多，单次 process ready 平均值越低，但 success p99 和 accepted RPS 没有同步改善，说明主瓶颈不再只是 relay 单轮处理耗时。

## 9. 结论

- 当前条件下所有组合都能追平 outbox，`PublishBatch=true` 已经把 relay backlog 从主要瓶颈降为可控项。
- `relay_workers=8` 是当前更稳的候选。12/16 worker 没有带来 accepted RPS 或 success p99 改善，反而在 1600 VU 下整体更差。
- batch size 不宜直接定为 500。1200 VU 下 500/8 的 accepted RPS 最高，但 1600 VU 下 100/8 的 success p99 和 accepted RPS 更好。
- 当前推荐下一轮重复验证两个候选：

```text
batch_size=100, workers=8
batch_size=500, workers=8
```

## 10. 口径限制

- 本轮每个组合只跑 1 次，不能作为最终容量结论。
- `kafka_publish_record_latency_estimate_ms` 是 call-level estimate，不是全局按 record 加权的真实 per-record latency。
- `outbox_process_ready_latency_ms` 会混入 `stats_wait` 阶段的 idle 样本；做 adaptive limit 前需要 active/idle 拆分或至少记录 fetched-per-call。

## 11. 下一步

- 对 `100/8` 和 `500/8` 各重复 2 轮，确认波动范围。
- 补 active/idle relay metrics 或 `outbox_fetched_per_call`，避免 adaptive limit 使用被 idle 样本稀释的延迟。
- 设计 adaptive limit 输入：PG pool acquire、outbox pending、relay active process ready、Kafka records/call。
