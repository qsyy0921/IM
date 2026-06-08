# NexusIM SendMessage Adaptive Limit Smoke Report

## 1. 压测目标

本阶段不是容量结论，而是验证第一版 adaptive admission limit 是否真实接入 `message-service`：

```text
client/loadtest
-> message-service gRPC
-> app AdmissionPort
-> adaptive controller
-> SERVICE_OVERLOADED
```

验证重点：

- admission 判断发生在 app 用例入口，早于权限读取、conversation 读取和 PostgreSQL 写事务。
- `SERVICE_OVERLOADED` 仍走稳定 gRPC 错误码和 retryable 语义。
- 被拒绝请求不会写入 `message_log / conversation_timeline_events / message_outbox`。
- 压测脚本可以注入 adaptive 配置和 relay metrics URL，后续可用于正式矩阵。

## 2. 本轮实现

新增 `services/message-service/internal/infrastructure/admission`：

- `PoolStatsProvider`：读取本进程 PG pool `acquired/max`。
- `MetricsProvider`：读取本进程 debug metrics 中的 `repository_pool_acquire_latency_ms`。
- `OutboxBacklogProvider`：按采样周期查询 `message_outbox` 的 `PENDING` 数。
- 可选 `RelayMetricsURL`：读取 relay `/debug/metrics` 中的：
  - `outbox_process_ready_active_latency_ms`
  - `outbox_fetched_per_call`
  - `kafka_publish_records_per_call`

app 层新增 `AdmissionPort`，`SendMessageUseCase` 在 command validation 后、依赖读取前调用。

第一版仍是保护策略，不是最终自适应算法。它按阈值触发拒绝，后续正式 adaptive limit 会把这些输入变成滑动窗口、迟滞和动态 retry hint。

## 3. 压测拓扑

全部组件运行在 Windows 本机：

```text
sendmessage loadtest
-> 127.0.0.1:10495 message-service gRPC
-> adaptive admission
-> PostgreSQL / Kafka / relay process
```

本轮故意设置非常激进的阈值：

```text
PG_MAX_CONNS=4
NEXUSIM_ADAPTIVE_MIN_AVAILABLE_CONNS=4
```

由于可用连接数一开始就是 `4`，满足 `available <= min_available`，所以所有请求应被 admission 快速拒绝。这个 smoke 用来验证保护链路，不衡量吞吐容量。

## 4. 执行命令

```powershell
. .\tools\go-env.ps1

.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 4 `
  -VUs 5 `
  -Duration 3s `
  -StatsWait 3s `
  -ConversationCount 50 `
  -RelayWorkers 2 `
  -BatchSize 100 `
  -PublishBatchEnabled:$true `
  -AdaptiveLimitEnabled `
  -AdaptiveMinAvailableConns 4 `
  -AdaptiveMaxPoolAcquireP95 50ms `
  -AdaptiveMaxOutboxPending 1 `
  -AdaptiveMaxRelayActiveP95 50ms `
  -AdaptiveMinOutboxFetchedPerCall 1 `
  -AdaptiveMinKafkaRecordsPerCall 1 `
  -AdaptiveSampleInterval 500ms `
  -ResultRoot loadtest\results\adaptive-limit-smoke-final-20260609
```

结果文件：

```text
loadtest/results/adaptive-limit-smoke-final-20260609/bpoff-adapton-pbatchon-pgmax-4-vu-5-20260609-072955/sendmessage-summary.json
```

## 5. 核心结果

| 指标 | 值 |
| --- | ---: |
| commit | `d2af748` |
| git_dirty | `false` |
| VU | 5 |
| duration | 3s |
| request_count | 15136 |
| success_count | 0 |
| success_rate | 0 |
| p95 | 2.5732ms |
| p99 | 3.1742ms |
| retryable_error_count | 15136 |
| service_overloaded_count | 15136 |
| outbox_total_count | 0 |
| outbox_pending_count | 0 |
| outbox_dlq_count | 0 |

错误分布：

```text
MESSAGE_ERROR_CODE_SERVICE_OVERLOADED retryable=true count=15136
```

压测后全局 outbox 状态：

```text
PUBLISHED=1433964
PENDING=0
DLQ=0
```

## 6. 如何排查是否真的提前拒绝

本轮不是只看 `p99`，而是按以下顺序判断：

1. 看 `message_error_counts[]`。
   - 全部错误都必须是 `MESSAGE_ERROR_CODE_SERVICE_OVERLOADED`。
   - 如果出现 `DB_WRITE_FAILED` 或 `DeadlineExceeded`，说明拒绝太晚，仍然打进数据库等待路径。

2. 看 `outbox_total_count / pending / published`。
   - 本轮 tenant 的 outbox 计数全部为 0，说明被拒绝请求没有创建 outbox event。
   - 如果 outbox 有新增，说明 admission 没有发生在写事务之前。

3. 看全局 PostgreSQL outbox。
   - 压测前后全局只有历史 `PUBLISHED`，没有新增 `PENDING/DLQ`。
   - 这说明 smoke 没有给 relay/Kafka 制造额外积压。

4. 看 gRPC 启动日志。
   - gRPC 进程打印了 adaptive limit 配置，说明脚本环境变量进入了真实服务进程。

## 7. 当前结论

第一版 adaptive admission limit 已经形成真实可运行链路：

```text
app AdmissionPort
-> infrastructure/admission controller
-> SERVICE_OVERLOADED
-> gRPC RetryInfo / MessageError
```

它现在可以作为正式 adaptive 实验的入口，但还不能称为生产级自适应限流。

## 8. 下一步

下一轮不要继续用这种极端阈值。应基于当前本地 relay 基线：

```text
NEXUSIM_OUTBOX_BATCH_SIZE=100
NEXUSIM_OUTBOX_WORKERS=8
PG_MAX_CONNS=64
```

做 adaptive on/off 或阈值梯度矩阵：

- `AdaptiveMinAvailableConns=4/8`
- `AdaptiveMaxPoolAcquireP95=200ms/400ms`
- `AdaptiveMaxOutboxPending=5000/10000/20000`
- `AdaptiveMaxRelayActiveP95=150ms/300ms`

正式报告必须同时展示：

- logical success rate
- accepted RPS
- attempt overload rate
- success p99
- error p99
- outbox pending
- repository pool acquire p95/p99
- outbox active process ready p95/p99
- outbox fetched per call
- Kafka records per call
