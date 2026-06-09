# NexusIM SendMessage Adaptive Limit On/Off Loadtest Report

## 1. 压测目标

本阶段验证第一版 app 层 adaptive admission 是否能替代或补充 repository 层 backpressure：

```text
client/loadtest
-> message-service gRPC
-> app AdmissionPort
-> adaptive controller
-> SendMessage use case
-> PostgreSQL local transaction
-> outbox relay
-> Kafka
```

重点不是证明容量提升，而是确认：

- admission 可以在权限读取、conversation 读取和 PostgreSQL 事务之前拒绝。
- 拒绝语义仍是 retryable `SERVICE_OVERLOADED`。
- outbox 不积压。
- adaptive 输入不能误用累计指标或 idle relay 样本。

## 2. 环境和基线

运行环境：

```text
Windows 本机
Docker Desktop
PostgreSQL loadtest profile
Kafka local broker
message-service gRPC process
message-service outbox relay process
```

固定参数：

```text
PG_MAX_CONNS=64
NEXUSIM_OUTBOX_BATCH_SIZE=100
NEXUSIM_OUTBOX_WORKERS=8
PublishBatch=true
RetryOverloaded=true
MaxRetries=2
RetryJitter=100ms
Duration=30s
StatsWait=20s
ConversationCount=1000
```

当前 relay 基线来自：

```text
docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-candidate-repeat.md
```

## 3. 对照组

### 3.1 Repository Backpressure

使用旧保护方式：

```text
NEXUSIM_PG_BACKPRESSURE_ENABLED=true
NEXUSIM_PG_BACKPRESSURE_MIN_AVAILABLE_CONNS=8
NEXUSIM_ADAPTIVE_LIMIT_ENABLED=false
```

结果路径：

```text
loadtest/results/adaptive-onoff-v3-repo-backpressure-20260609/
```

### 3.2 App Adaptive Admission

使用 app 入口保护，不启用 repository backpressure：

```text
NEXUSIM_PG_BACKPRESSURE_ENABLED=false
NEXUSIM_ADAPTIVE_LIMIT_ENABLED=true
NEXUSIM_ADAPTIVE_MIN_AVAILABLE_CONNS=8
NEXUSIM_ADAPTIVE_MAX_POOL_ACQUIRE_P95=0s
NEXUSIM_ADAPTIVE_MAX_OUTBOX_PENDING=50000
NEXUSIM_ADAPTIVE_MAX_RELAY_ACTIVE_P95=0s
NEXUSIM_ADAPTIVE_MIN_OUTBOX_FETCHED_PER_CALL=0
NEXUSIM_ADAPTIVE_MIN_KAFKA_RECORDS_PER_CALL=0
NEXUSIM_ADAPTIVE_SAMPLE_INTERVAL=500ms
```

结果路径：

```text
loadtest/results/adaptive-relaxed-v1-admission-20260609/
```

说明：本轮最终有效 adaptive 配置只保留 app 层 pool floor 和较高 outbox pending 阈值。累计 p95 和 relay 低吞吐指标暂不作为独立硬拒绝条件，原因见第 6 节。

## 4. 执行命令

Repository backpressure：

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 64 `
  -VUs 1200,1600 `
  -Duration 30s `
  -StatsWait 20s `
  -ConversationCount 1000 `
  -RelayWorkers 8 `
  -BatchSize 100 `
  -PublishBatchEnabled:$true `
  -BackpressureEnabled `
  -BackpressureMinAvailableConns 8 `
  -RetryOverloaded `
  -MaxRetries 2 `
  -RetryJitter 100ms `
  -ResultRoot loadtest\results\adaptive-onoff-v3-repo-backpressure-20260609
```

App adaptive admission：

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 64 `
  -VUs 1200,1600 `
  -Duration 30s `
  -StatsWait 20s `
  -ConversationCount 1000 `
  -RelayWorkers 8 `
  -BatchSize 100 `
  -PublishBatchEnabled:$true `
  -AdaptiveLimitEnabled `
  -AdaptiveMinAvailableConns 8 `
  -AdaptiveMaxPoolAcquireP95 0s `
  -AdaptiveMaxOutboxPending 50000 `
  -AdaptiveMaxRelayActiveP95 0s `
  -AdaptiveMinOutboxFetchedPerCall 0 `
  -AdaptiveMinKafkaRecordsPerCall 0 `
  -AdaptiveSampleInterval 500ms `
  -RetryOverloaded `
  -MaxRetries 2 `
  -RetryJitter 100ms `
  -ResultRoot loadtest\results\adaptive-relaxed-v1-admission-20260609
```

## 5. 核心结果

所有结果均来自 clean commit：

```text
commit=7d6e59f
git_dirty=false
```

| 模式 | VU | logical success | accepted RPS | attempt overload | success p99 | error p99 | outbox pending |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| repository backpressure | 1200 | 0.7613 | 1950.10 | 0.5741 | 503.7984ms | 4.8239ms | 0 |
| app adaptive relaxed | 1200 | 0.7382 | 1858.43 | 0.5902 | 575.0696ms | 4.2084ms | 0 |
| repository backpressure | 1600 | 0.6482 | 1811.07 | 0.6710 | 841.7406ms | 3.6507ms | 0 |
| app adaptive relaxed | 1600 | 0.6442 | 1772.47 | 0.6758 | 709.9379ms | 4.7169ms | 0 |

Relay / Kafka 观测：

| 模式 | VU | repository pool acquire p99 | relay active p95 | outbox fetched avg | Kafka records/call avg |
| --- | ---: | ---: | ---: | ---: | ---: |
| repository backpressure | 1200 | 457.8365ms | 141.0177ms | 21.3203 | 44.0535 |
| app adaptive relaxed | 1200 | 495.5611ms | 123.0634ms | 19.9331 | 41.7001 |
| repository backpressure | 1600 | 784.8018ms | 121.9760ms | 19.7141 | 40.9435 |
| app adaptive relaxed | 1600 | 614.6709ms | 123.0286ms | 19.1480 | 39.6230 |

压测后全局 outbox：

```text
PUBLISHED=1912396
PENDING=0
DLQ=0
```

## 6. 瓶颈排查过程

### 6.1 第一次误判：idle relay 样本

早期 adaptive-only 结果出现 `success_rate=0`。排查路径：

1. summary 显示所有请求都是 `SERVICE_OVERLOADED`，且 outbox 没有新增。
2. DB 没有 pending，说明不是业务事务或 relay 卡住。
3. 检查 adaptive 输入发现 `outbox_fetched_per_call` 包含 idle 轮询的 0。
4. 在没有 backlog 的情况下，idle 轮询为 0 是正常状态，不应触发拒绝。

修复：

```text
只有 outbox pending > 0 时，低 outbox_fetched_per_call 才参与拒绝。
```

对应提交：

```text
64a6c52 fix: avoid adaptive relay idle false positives
```

### 6.2 第二次误判：累计 p95 粘住

修复 idle false positive 后，adaptive-only 仍然过度拒绝，accepted RPS 只有约 300-360。排查路径：

1. `repository_pool_acquire_p95` 一旦在高压下超过阈值，就因为 debug collector 保存全量样本而长期保持高位。
2. 即使当前 pool 已经不再满载，累计 p95 仍继续触发 admission。
3. 这会导致低吞吐、长 retry 等待，`success_p99/error_p99` 都接近 2s。

修复：

```text
累计 PG p95 不能作为独立硬门槛。
只有当前 pool 也接近饱和时，repository_pool_acquire_p95 才作为辅助信号。
relay active p95 / Kafka records per call 也必须在 outbox pending > 0 时才参与拒绝。
```

对应提交：

```text
7d6e59f fix: gate adaptive cumulative metrics with live pressure
```

### 6.3 为什么最终采用 relaxed 配置

本轮 debug collector 仍是全量样本，不是滑动窗口；因此正式矩阵里不把累计 p95、relay active p95、fetched-per-call、records-per-call 直接设为硬拒绝条件。

最终 relaxed 配置先验证 app 入口保护是否可替代 repository backpressure：

```text
pool floor + high outbox pending threshold
```

## 7. 当前结论

1. app adaptive admission 已经可运行，并且能在写事务之前拒绝。
2. relaxed 配置下，它和 repository backpressure 的行为接近，outbox 都能在 `stats_wait=20s` 后清零。
3. 它没有证明容量提升：1200 VU accepted RPS 从 `1950.10` 降到 `1858.43`，1600 VU 从 `1811.07` 降到 `1772.47`。
4. 它的价值是把过载拒绝提前到 app 入口，减少权限、conversation 和 repository 路径的无效消耗。
5. 不能把累计 p95 或 idle relay 样本作为硬拒绝条件；生产化必须引入滑动窗口、迟滞和动态 retry hint。

## 8. 下一步

下一阶段建议顺序：

1. 把 debug metrics collector 从全量样本改成短窗口或 HDR histogram，至少为 adaptive limit 提供窗口化 p95/p99。
2. 给 adaptive controller 增加 hysteresis：进入过载和退出过载使用不同阈值，避免抖动。
3. 将固定 `RetryInfo=500ms` 改为根据过载级别动态输出。
4. 再跑 adaptive threshold matrix，而不是继续用累计 p95 做硬门槛。

