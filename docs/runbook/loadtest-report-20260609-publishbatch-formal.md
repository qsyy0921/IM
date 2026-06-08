# PublishBatch Formal Matrix 2026-06-09

## 1. 压测目标

验证 Kafka `PublishBatch` 是否能改善 outbox relay 追平能力，并用同一 HEAD 的开关对照，避免不同 commit 或指标口径造成误判。

本轮重点不是提高 `SendMessage` accepted RPS，而是观察：

```text
message_outbox pending
outbox_process_ready_latency_ms
kafka_publish_call_latency_ms
kafka_publish_records_per_call
kafka_publish_record_latency_estimate_ms
```

## 2. 压测拓扑

```text
loadtest/sendmessage
-> message-service gRPC
-> PostgreSQL local transaction
-> message_outbox
-> outbox relay
-> Kafka conversation.timeline.events
```

服务端和压测器均在 Windows 本机运行；PostgreSQL 和 Kafka 运行在本机 Docker Desktop。

## 3. 环境和配置

```text
commit: aec449c
git_dirty: false
PostgreSQL profile: loadtest override
PostgreSQL max_connections: 200
PG_MAX_CONNS: 64
relay workers: 8
outbox batch size: 500
backpressure: enabled
backpressure min available conns: 8
client retry: enabled
max_retries: 2
retry_jitter: 100ms
duration: 30s
stats_wait: 20s
conversation_count: 1000
```

## 4. 执行方式

先重建当前 HEAD 的本地二进制：

```powershell
. .\tools\go-env.ps1
go build -o bin\message-service.exe ./services/message-service/cmd/message-service
go build -o bin\sendmessage-loadtest.exe ./loadtest/sendmessage
```

`PublishBatch=false`：

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 64 `
  -VUs 1200,1600 `
  -Duration 30s `
  -StatsWait 20s `
  -ConversationCount 1000 `
  -RelayWorkers 8 `
  -BatchSize 500 `
  -PublishBatchEnabled:$false `
  -BackpressureEnabled `
  -BackpressureMinAvailableConns 8 `
  -RetryOverloaded `
  -MaxRetries 2 `
  -RetryJitter 100ms `
  -ResultRoot loadtest\results\publishbatch-formal-valid-off-r1-20260609 `
  -SkipBuild
```

`PublishBatch=true` 只把 `-PublishBatchEnabled:$false` 改为 `-PublishBatchEnabled:$true`。本轮每个模式重复 2 轮。

## 5. 有效结果路径

```text
loadtest/results/publishbatch-formal-valid-off-r1-20260609/
loadtest/results/publishbatch-formal-valid-off-r2-20260609/
loadtest/results/publishbatch-formal-valid-on-r1-20260609/
loadtest/results/publishbatch-formal-valid-on-r2-20260609/
```

以下中间结果不作为正式证据：

```text
loadtest/results/publishbatch-formal-off-r1-20260609/
loadtest/results/publishbatch-formal-off-r2-20260609/
loadtest/results/publishbatch-formal-on-r1-20260609/
loadtest/results/publishbatch-formal-on-r2-20260609/
```

原因：第一次矩阵误用了 `-SkipBuild` 和旧 `bin/message-service.exe`，`pbatchoff` 目录中的 `kafka_publish_records_per_call` 仍约 40，说明关闭 batch 的环境变量没有进入实际二进制。重建 `bin` 后，短 smoke 验证 `pbatchoff` 的 `records_per_call=1`，再重跑正式矩阵。

## 6. 单轮结果

| mode | VU | logical success | accepted RPS | overload rate | success p99 ms | pending at stats wait | Kafka call avg ms | records/call avg | record estimate ms | process ready avg ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| off r1 | 1200 | 0.6696 | 1563.53 | 0.6406 | 604.71 | 31548 | 12.8048 | 1.0000 | 12.8048 | 978.7138 |
| off r2 | 1200 | 0.7066 | 1652.50 | 0.6198 | 684.23 | 36004 | 12.8851 | 1.0000 | 12.8851 | 1019.1320 |
| on r1 | 1200 | 0.6143 | 1261.20 | 0.6892 | 911.85 | 9826 | 111.6202 | 364.5263 | 0.6704 | 1675.2260 |
| on r2 | 1200 | 0.6313 | 1298.97 | 0.6803 | 844.51 | 19983 | 96.8274 | 397.6306 | 0.4606 | 2389.8308 |
| off r1 | 1600 | 0.5740 | 1434.37 | 0.7232 | 1225.12 | 34931 | 12.6602 | 1.0000 | 12.6602 | 2690.3150 |
| off r2 | 1600 | 0.6043 | 1508.33 | 0.7093 | 1041.43 | 39028 | 12.6718 | 1.0000 | 12.6718 | 2895.2223 |
| on r1 | 1600 | 0.6311 | 1714.93 | 0.6831 | 1015.42 | 0 | 51.5102 | 51.3042 | 4.8835 | 58.0770 |
| on r2 | 1600 | 0.5393 | 1235.37 | 0.7513 | 1321.72 | 0 | 105.6714 | 325.9048 | 1.6273 | 511.8532 |

## 7. 两轮平均

| mode | VU | logical success avg | accepted RPS avg | overload rate avg | success p99 avg ms | pending avg | Kafka call avg ms | records/call avg | record estimate avg ms | process ready avg ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| off | 1200 | 0.6881 | 1608.02 | 0.6302 | 644.47 | 33776 | 12.8450 | 1.0000 | 12.8450 | 998.9229 |
| on | 1200 | 0.6228 | 1280.08 | 0.6848 | 878.18 | 14904 | 104.2238 | 381.0784 | 0.5655 | 2032.5284 |
| off | 1600 | 0.5891 | 1471.35 | 0.7163 | 1133.27 | 36980 | 12.6660 | 1.0000 | 12.6660 | 2792.7687 |
| on | 1600 | 0.5852 | 1475.15 | 0.7172 | 1168.57 | 0 | 78.5908 | 188.6045 | 3.2554 | 284.9651 |

矩阵结束后额外查询 PostgreSQL：

```text
PUBLISHED|798372
```

最终没有遗留 `PENDING` / `DLQ`。

## 8. 瓶颈排查过程

1. 先用 `kafka_publish_records_per_call` 校验实验变量。
   第一次矩阵中 `pbatchoff` 的 records/call 仍约 40，说明结果无效。排查发现脚本用了 `-SkipBuild`，二进制没有包含新开关。

2. 重建二进制并跑短 smoke。
   `PublishBatch=false` 的短 smoke 中 `records_per_call=1`，说明关闭 batch 后确实走单条 publish path。

3. 再比较正式矩阵的 pending 和 relay process ready。
   关闭 batch 时 records/call 恒为 1，Kafka call latency 低，但 relay 每轮处理效率差，`stats_wait=20s` 后仍有 31k-39k pending。

4. 开启 batch 后关注 record 维度估算。
   单次 Kafka call latency 变高，但一轮调用承载几十到数百条 record，`record_latency_estimate` 明显降低。1600 VU 两轮都能在 stats wait 内清空 pending。

5. 同时观察成功请求体验。
   PublishBatch 没有稳定改善 success p99；1200 VU 下 success p99 反而变高，1600 VU 下两轮波动较大。说明它主要改善 relay 追平，不是直接解决 gRPC 写入尾延迟。

## 9. 当前结论

- `PublishBatch` 对 outbox backlog 有明确帮助，尤其 1600 VU 下 pending 从平均 `36980` 降到 `0`。
- `PublishBatch` 不是当前 accepted RPS 或 success p99 的稳定提升手段；SendMessage 成功路径仍受 PostgreSQL/backpressure/admission control 影响。
- `kafka_publish_call_latency_ms` 不能单独用于判断性能。batch 下 call latency 高是正常现象，必须同时看 `records_per_call` 和 `record_latency_estimate`。
- 1200 VU 下 `PublishBatch=true` 仍有 pending，说明 batch size 500 并不是最终配置；需要继续做 batch size / worker / adaptive limit 联合矩阵。

## 10. 下一步

- 保留 `NEXUSIM_OUTBOX_PUBLISH_BATCH_ENABLED`，后续正式矩阵都用同一 HEAD 开关做对照。
- 继续做 batch size 100/500/1000 与 workers 8/12/16 的联合矩阵。
- adaptive limit 不能只看 PG pool，还要纳入 outbox pending、`outbox_process_ready_latency_ms` 和 `kafka_publish_records_per_call`。
- 报告中所有 Kafka batch 相关结论必须使用新指标，不再用旧 `kafka_publish_latency_ms` 做 single/batch 对比。
