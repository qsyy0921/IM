# Outbox PublishBatch Smoke 2026-06-09

## 1. 压测目标

验证 `outbox relay -> Kafka` 从逐条 `Publish` 改为批量 `PublishBatch` 后，真实链路仍保持：

```text
SendMessage accepted
-> message_outbox PENDING
-> relay batch publish to Kafka
-> batch mark PUBLISHED
-> outbox pending drains to 0
```

本轮是 smoke + 重复验证，不作为正式容量结论。

## 2. 设计变化

改动前：

```text
OutboxStore.ProcessReady
-> for each message
   -> relay publish callback
   -> kafka writer WriteMessages(single)
-> batch mark PUBLISHED
-> commit
```

改动后：

```text
OutboxStore.ProcessReadyBatch
-> relay build Kafka records
-> kafka writer WriteMessages(records...)
-> per-message success/error result
-> batch mark PUBLISHED
-> retry/DLQ failed messages
-> commit
```

兼容约束：

- 旧 `ProcessReady` 单条 callback 仍保留。
- Relay 只有在 store 支持 `ProcessReadyBatch` 时才走 batch path。
- Kafka batch 写失败时，当前批次中已构造成功的消息按同一个错误进入 retry/DLQ。
- 单条 payload 构造失败仍按对应消息自己的错误处理，不阻塞其它可构造消息发布。
- 仍然是 at-least-once：Kafka publish 成功但 mark/commit 失败时，后续允许重复发布，依赖 `event_id` 消费幂等。

## 3. 固定配置

```text
before commit: aeaf88a docs: record relay metrics smoke
after commit: 9f26d0c feat: batch publish outbox events to kafka
git_dirty: false
PG_MAX_CONNS: 64
Backpressure: enabled
BackpressureMinAvailableConns: 8
relay workers: 8
batch size: 500
VU: 1200
duration: 30s
stats_wait: 20s
conversation_count: 3000
max_retries: 2
retry_jitter: 100ms
```

## 4. 执行命令

```powershell
. .\tools\go-env.ps1
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 64 `
  -VUs 1200 `
  -Duration 30s `
  -StatsWait 20s `
  -ConversationCount 3000 `
  -RelayWorkers 8 `
  -BatchSize 500 `
  -BackpressureEnabled `
  -BackpressureMinAvailableConns 8 `
  -RetryOverloaded `
  -MaxRetries 2 `
  -RetryJitter 100ms `
  -ResultRoot loadtest\results\outbox-publishbatch-smoke-20260609
```

重复验证使用相同参数，输出到：

```text
loadtest\results\outbox-publishbatch-smoke-repeat-20260609
```

## 5. 结果文件

```text
before:
loadtest/results/outbox-batchsize-smoke-20260609/batch-500/bpon-pgmax-64-vu-1200-20260609-052656/sendmessage-summary.json

after run 1:
loadtest/results/outbox-publishbatch-smoke-20260609/bpon-pgmax-64-vu-1200-20260609-053621/sendmessage-summary.json

after run 2:
loadtest/results/outbox-publishbatch-smoke-repeat-20260609/bpon-pgmax-64-vu-1200-20260609-053747/sendmessage-summary.json
```

矩阵结束后全局 `message_outbox` 状态：

```text
PUBLISHED=88242
PENDING=0
DLQ=0
```

## 6. 核心结果

| run | commit | logical success rate | accepted RPS | attempt overload rate | p99 | success p99 | outbox pending |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| before batch 500 | `aeaf88a` | 0.7160 | 1694.93 | 0.6132 | 99.54ms | 600.94ms | 0 |
| after run 1 | `9f26d0c` | 0.6358 | 1282.57 | 0.6805 | 529.86ms | 919.21ms | 0 |
| after run 2 | `9f26d0c` | 0.7125 | 1658.80 | 0.6180 | 101.65ms | 576.48ms | 0 |

Relay 分段指标：

| run | kafka avg | process ready avg | fetch ready avg | mark published avg | commit avg |
| --- | ---: | ---: | ---: | ---: | ---: |
| before batch 500 | 4.46ms | 415.83ms | 110.05ms | 10.63ms | 2.19ms |
| after run 1 | 61.57ms | 153.52ms | 122.63ms | 13.30ms | 6.02ms |
| after run 2 | 41.19ms | 41.95ms | 9.14ms | 5.85ms | 6.68ms |

## 7. 瓶颈排查过程

1. 先跑单元、构建、真实 Kafka producer integration 和真实 PostgreSQL + Kafka relay integration，确认语义没有在测试层断裂。
2. 使用上一轮 batch size 500 的 clean summary 作为 before。
3. 用相同参数跑 after run 1，发现 `outbox_process_ready_latency_ms` 下降，但成功请求 p99 和 accepted RPS 变差。
4. 立即用相同参数重复 after run 2，p99 回到 before 接近水平，说明 run 1 不能单独作为负面容量结论。
5. 查看 relay 分段：
   - after run 2 的 `outbox_process_ready_latency_ms` 从 before 的 `415.83ms` 降到 `41.95ms`。
   - `kafka_publish_latency_ms` 变大，因为它现在是一轮 batch `WriteMessages(records...)` 的耗时，不再等价于旧的单条 publish 耗时。
   - 三组 outbox pending 均为 0，说明本轮窗口内 relay 都能追平。

## 8. 当前结论

- `PublishBatch` 真实链路可运行，outbox 可追平，未出现 DLQ 或 PENDING 残留。
- after run 2 显示 batch publish 可以显著缩短 `outbox_process_ready_latency_ms`。
- 单次 run 波动明显，不能宣称整体容量已经提升。
- 后续报告比较 Kafka 指标时必须注明：batch path 下 `kafka_publish_latency_ms` 是 batch 调用耗时，不是单条消息平均耗时。

## 9. 下一步

- 跑正式 before/after 重复矩阵，至少覆盖 1200/1600 VU，并保留每组两次以上结果。
- 在正式矩阵前继续补真实 PostgreSQL 层的 batch failure integration；当前 trigger 层已覆盖 Kafka batch error 和单条 payload build error。
- 继续评估是否要把 `kafka_publish_latency_ms` 拆成 `kafka_publish_call_latency_ms` 和 `kafka_publish_record_count`，避免 batch 与 single 口径混淆。
- adaptive limit 需要同时参考 `outbox_process_ready_latency_ms` 和 outbox pending，而不是只看 PostgreSQL pool acquire。
