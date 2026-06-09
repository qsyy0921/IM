# Outbox Candidate Repeat 2026-06-09

## 1. 压测目标

在 batch/worker 3x3 矩阵后，重复验证两个候选：

```text
batch_size=100, workers=8
batch_size=500, workers=8
```

目标是确认 1200/1600 VU 下的波动范围，并使用新增 active/idle relay metrics 判断配置是否稳定。

## 2. 固定参数

```text
commit: c41b26a
git_dirty: false
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

## 3. 执行入口

```powershell
. .\tools\go-env.ps1
.\loadtest\sendmessage\run-local-outbox-batch-worker-matrix.ps1 `
  -BatchSizes 100,500 `
  -RelayWorkers 8 `
  -VUs 1200,1600 `
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
  -ResultRoot loadtest\results\outbox-candidate-repeat-r1-20260609
```

第二轮只把 `ResultRoot` 改为：

```text
loadtest\results\outbox-candidate-repeat-r2-20260609
```

## 4. 结果文件

```text
loadtest/results/outbox-candidate-repeat-r1-20260609/outbox-batch-worker-matrix-summary.json
loadtest/results/outbox-candidate-repeat-r2-20260609/outbox-batch-worker-matrix-summary.json
```

矩阵结束后 PostgreSQL：

```text
PUBLISHED|1433964
```

没有遗留 `PENDING` / `DLQ`。

## 5. 单轮结果

| run | batch | VU | logical success | accepted RPS | overload rate | success p99 ms | pending | records/call | fetched/call | active ready ms | idle ready ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| r1 | 100 | 1200 | 0.7435 | 1905.40 | 0.5839 | 555.05 | 0 | 41.66 | 20.61 | 91.27 | 11.95 |
| r2 | 100 | 1200 | 0.7374 | 1846.77 | 0.5921 | 494.43 | 0 | 41.04 | 19.95 | 87.23 | 12.34 |
| r1 | 100 | 1600 | 0.6440 | 1744.83 | 0.6775 | 928.61 | 0 | 38.12 | 18.69 | 85.48 | 12.70 |
| r2 | 100 | 1600 | 0.6538 | 1797.30 | 0.6712 | 708.32 | 0 | 39.47 | 19.37 | 87.10 | 12.63 |
| r1 | 500 | 1200 | 0.7153 | 1715.07 | 0.6109 | 698.61 | 0 | 40.07 | 18.83 | 89.14 | 12.22 |
| r2 | 500 | 1200 | 0.7343 | 1820.63 | 0.5943 | 693.94 | 0 | 45.10 | 20.57 | 94.30 | 13.23 |
| r1 | 500 | 1600 | 0.6600 | 1785.77 | 0.6661 | 1455.08 | 0 | 43.80 | 20.14 | 91.21 | 15.45 |
| r2 | 500 | 1600 | 0.6503 | 1798.90 | 0.6713 | 845.17 | 0 | 43.21 | 20.04 | 90.52 | 12.92 |

## 6. 两轮平均

| batch | VU | logical success avg | accepted RPS avg | success p99 avg ms | pending avg | records/call avg | fetched/call avg | active ready avg ms | idle ready avg ms |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 100 | 1200 | 0.7404 | 1876.08 | 524.74 | 0 | 41.35 | 20.28 | 89.25 | 12.14 |
| 500 | 1200 | 0.7248 | 1767.85 | 696.28 | 0 | 42.59 | 19.70 | 91.72 | 12.73 |
| 100 | 1600 | 0.6489 | 1771.07 | 818.47 | 0 | 38.80 | 19.03 | 86.29 | 12.66 |
| 500 | 1600 | 0.6552 | 1792.33 | 1150.13 | 0 | 43.51 | 20.09 | 90.86 | 14.19 |

## 7. 排查结论

1. 两个候选均能追平 outbox。
   所有 run 的 `outbox_pending_count=0`，说明当前压力下 relay backlog 已可控。

2. batch 100 / workers 8 的 tail latency 更稳。
   1200 VU 下 success p99 平均 `524.74ms`，1600 VU 下 `818.47ms`；均低于 batch 500。

3. batch 500 的 accepted RPS 不稳定。
   1600 VU 下 accepted RPS 平均略高，但 success p99 被 r1 的 `1455.08ms` 拉高，说明它不是更稳的默认候选。

4. active/idle 拆分有效。
   active ready 平均约 `86-92ms`，idle ready 平均约 `12-14ms`。后续 adaptive limit 应参考 active 指标，而不是混合 `outbox_process_ready_latency_ms`。

## 8. 当前结论

短期候选配置：

```text
NEXUSIM_OUTBOX_BATCH_SIZE=100
NEXUSIM_OUTBOX_WORKERS=8
```

该配置不是最终生产默认值，只是下一轮 adaptive limit 和 retry 参数矩阵的本地基线。

## 9. 下一步

- 以 batch 100 / workers 8 为 relay 基线，设计 adaptive limit。
- adaptive limit 输入优先级：PG pool acquire、outbox pending、active ready latency、fetched per call、Kafka records per call。
- 后续可再验证 batch 100 / workers 8 在 2000 VU 或多客户端分布式压测下的稳定性。
