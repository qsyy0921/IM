# Relay Metrics Smoke 2026-06-09

## 1. 压测目标

验证 outbox relay 新增的细分指标能从真实进程 `/debug/metrics` 进入 `sendmessage-summary.json`。

本轮不是容量压测，不用于判断最大 VU 或生产吞吐。

## 2. 压测拓扑

```text
loadtest/sendmessage
-> message-service gRPC
-> PostgreSQL local transaction
-> message_outbox
-> outbox relay
-> Kafka conversation.timeline.events
```

服务端、relay、PostgreSQL、Kafka 和压测器均运行在 Windows 本机。

## 3. 环境与配置

```text
commit: 148938e feat: add outbox relay latency metrics
git_dirty: false
PG_MAX_CONNS: 16
relay workers: 2
batch size: 100
VU: 10
duration: 5s
stats_wait: 5s
conversation_count: 100
```

## 4. 执行命令

```powershell
. .\tools\go-env.ps1
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 16 `
  -VUs 10 `
  -Duration 5s `
  -StatsWait 5s `
  -ConversationCount 100 `
  -RelayWorkers 2 `
  -BatchSize 100 `
  -ResultRoot loadtest\results\relay-metrics-smoke-20260609
```

结果文件：

```text
loadtest/results/relay-metrics-smoke-20260609/bpoff-pgmax-16-vu-10-20260609-052152/sendmessage-summary.json
```

## 5. 核心结果

| 指标 | 数值 |
| --- | ---: |
| request_count | 5244 |
| success_rate | 1.0000 |
| p95_ms | 12.3340 |
| p99_ms | 15.8853 |
| outbox_pending_count | 0 |
| kafka_publish_latency_ms | 0.7816 |
| outbox_process_ready_latency_ms | 43.2098 |
| outbox_fetch_ready_latency_ms | 18.1205 |
| outbox_mark_published_latency_ms | 2.1892 |
| outbox_commit_latency_ms | 2.6068 |

## 6. 瓶颈排查方法

本轮重点不是判断瓶颈，而是把后续瓶颈排查所需的 relay 分段指标接入 summary。

新增指标口径：

```text
outbox_process_ready_latency_ms: 一轮 Store.ProcessReady 总耗时，包含 fetch、Kafka publish callback、mark、commit
outbox_fetch_ready_latency_ms: 查询并锁定 ready outbox 行的耗时
outbox_mark_published_latency_ms: 成功 publish 后批量 UPDATE PUBLISHED 的耗时
outbox_commit_latency_ms: 提交 outbox 事务的耗时
kafka_publish_latency_ms: 单条 Kafka publish callback 耗时
```

这组指标能把后续 relay backlog 拆成几类问题：

```text
fetch 高: ready SQL、索引、低版本阻塞或 SKIP LOCKED 扫描压力
Kafka 高: broker / producer / 网络或 topic partition 压力
mark 高: message_outbox 批量 update、dead tuple、WAL 或锁竞争
commit 高: WAL/checkpoint/fsync 压力
process 高但分段都不高: batch 内 publish 数量、worker 调度或未拆分阶段
```

本轮 smoke 中 `outbox_process_ready_latency_ms` 明显大于单个分段指标，符合预期：它包含批次内多条 Kafka publish callback 以及其它事务内处理，不应和单条 Kafka publish latency 直接等价比较。

## 7. 当前结论

- relay metrics 已能从真实 relay 进程进入 loadtest summary。
- 本轮 outbox 可在 `stats_wait=5s` 后追平，`outbox_pending_count=0`。
- 该结果只证明观测链路可用，不证明容量提升。

## 8. 下一步

- 在下一轮 client retry / outbox 优化矩阵中固定展示 relay 分段指标。
- 继续评估 `Publisher.PublishBatch`，重点观察是否降低 `outbox_process_ready_latency_ms` 和 Kafka publish 总持锁窗口。
- adaptive limit 设计需要同时参考 `outbox_pending_count`、`outbox_process_ready_latency_ms` 和 PostgreSQL pool acquire 指标。
