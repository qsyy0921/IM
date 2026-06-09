# Outbox Batch Size Smoke 2026-06-09

## 1. 压测目标

在已接入 relay 分段 metrics 后，初步观察不同 `NEXUSIM_OUTBOX_BATCH_SIZE` 对 outbox 追平、成功请求尾延迟和 relay 内部分段耗时的影响。

本轮是 30 秒探索矩阵，不作为正式容量结论。

## 2. 压测拓扑

```text
loadtest/sendmessage
-> message-service gRPC
-> PostgreSQL local transaction
-> message_outbox
-> outbox relay
-> Kafka conversation.timeline.events
```

所有组件运行在 Windows 本机。客户端开启 `--retry-overloaded`，遵守 gRPC `RetryInfo=500ms`，并叠加 `100ms` jitter。

## 3. 固定配置

```text
commit: aeaf88a docs: record relay metrics smoke
git_dirty: false
PG_MAX_CONNS: 64
Backpressure: enabled
BackpressureMinAvailableConns: 8
relay workers: 8
VU: 1200
duration: 30s
stats_wait: 20s
conversation_count: 3000
max_retries: 2
retry_jitter: 100ms
```

变量：

```text
NEXUSIM_OUTBOX_BATCH_SIZE = 100 / 500 / 1000
```

## 4. 执行命令

```powershell
. .\tools\go-env.ps1
$root = 'loadtest\results\outbox-batchsize-smoke-20260609'
foreach ($batch in @(100, 500, 1000)) {
  .\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
    -PGMaxConns 64 `
    -VUs 1200 `
    -Duration 30s `
    -StatsWait 20s `
    -ConversationCount 3000 `
    -RelayWorkers 8 `
    -BatchSize $batch `
    -BackpressureEnabled `
    -BackpressureMinAvailableConns 8 `
    -RetryOverloaded `
    -MaxRetries 2 `
    -RetryJitter 100ms `
    -ResultRoot (Join-Path $root "batch-$batch")
}
```

## 5. 结果文件

```text
loadtest/results/outbox-batchsize-smoke-20260609/batch-100/bpon-pgmax-64-vu-1200-20260609-052559/sendmessage-summary.json
loadtest/results/outbox-batchsize-smoke-20260609/batch-500/bpon-pgmax-64-vu-1200-20260609-052656/sendmessage-summary.json
loadtest/results/outbox-batchsize-smoke-20260609/batch-1000/bpon-pgmax-64-vu-1200-20260609-052753/sendmessage-summary.json
```

矩阵结束后全局 `message_outbox` 状态：

```text
PUBLISHED=149062
PENDING=0
DLQ=0
```

## 6. 核心结果

| batch size | logical success rate | accepted RPS | attempt overload rate | success p99 | error p99 | outbox pending |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 100 | 0.6893 | 1551.60 | 0.6365 | 584.42ms | 5.89ms | 0 |
| 500 | 0.7160 | 1694.93 | 0.6132 | 600.94ms | 4.76ms | 0 |
| 1000 | 0.6848 | 1547.33 | 0.6381 | 619.86ms | 5.59ms | 0 |

Relay 分段指标：

| batch size | kafka avg | process ready avg | fetch ready avg | mark published avg | commit avg |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 100 | 4.02ms | 354.63ms | 143.06ms | 6.76ms | 3.90ms |
| 500 | 4.46ms | 415.83ms | 110.05ms | 10.63ms | 2.19ms |
| 1000 | 4.33ms | 357.99ms | 110.08ms | 11.78ms | 2.87ms |

## 7. 瓶颈排查过程

本轮不只看 `outbox_pending_count`，而是同时看：

```text
logical_success_rate
accepted_rps
attempt overload rate
success_p99_ms
outbox_pending_count
kafka_publish_latency_ms
outbox_process_ready_latency_ms
outbox_fetch_ready_latency_ms
outbox_mark_published_latency_ms
outbox_commit_latency_ms
```

观察步骤：

1. 先确认三组 `git_dirty=false`，避免把未提交代码混入结果。
2. 看 `outbox_pending_count`，三组均为 0，说明当前 30 秒窗口内 relay 都能追平。
3. 看 `logical_success_rate` 和 `accepted_rps`，batch 500 略高于 100/1000。
4. 看 `success_p99_ms`，batch 100 最低，batch 500 次之，batch 1000 最高。
5. 看 relay 分段，batch 500/1000 的 mark published 平均耗时高于 batch 100，但 fetch ready 平均耗时低于 batch 100。
6. 看 `outbox_process_ready_latency_ms`，batch 500 最高，说明更大的批次会拉长单轮事务处理时间，不能只按 accepted RPS 选值。

## 8. 当前结论

- 三档 batch size 在本轮 1200 VU / 30s 探索中都能追平 outbox。
- batch 500 的 logical success rate 和 accepted RPS 较好，但 `outbox_process_ready_latency_ms` 也最高。
- batch 100 的 success p99 最低，但 logical success rate 和 accepted RPS 低于 batch 500。
- batch 1000 没有带来明显收益，success p99 最高，mark published 平均耗时也最高。

当前只支持一个保守判断：

```text
下一轮正式矩阵优先继续验证 batch size 500；
不要把 1000 当作默认值；
不要仅用 outbox pending=0 判断 relay 已经没有瓶颈。
```

## 9. 下一步

- 在 1200/1600 VU 下重复 batch 100/500/1000，确认本轮 30 秒探索结果是否稳定。
- 继续设计 `Publisher.PublishBatch`，目标是降低 `outbox_process_ready_latency_ms` 中由逐条 Kafka publish 带来的事务持锁窗口。
- adaptive limit 需要同时参考 `outbox_pending_count` 和 `outbox_process_ready_latency_ms`，避免 accepted RPS 回升后再次压垮 relay。
