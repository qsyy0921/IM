# NexusIM PG Pool And Multi Instance Load Test Report - 2026-06-09

本文记录 `message-service` 性能诊断第二阶段：补齐 repository 内部分段指标、支持多 gRPC target、多 service metrics URL，并验证 PG 连接池梯度脚本和多实例脚本能在真实链路上运行。

本报告不是最终容量报告。本文中的 smoke 只用于验证工具、指标和结果文件结构；正式容量结论仍需要后续长时间梯度压测。

## 1. 压测目标

上一份报告已经把高 VU 下的主要瓶颈初步定位到：

```text
message-service gRPC process
-> PostgreSQL pgxpool acquire / repository append
```

本阶段目标是继续把这个区间拆细：

```text
SendMessage request
-> repository append
-> pgxpool begin/acquire
-> idempotency advisory lock
-> find existing message
-> ensure conversation_seq
-> allocate conversation_seq
-> insert message_log
-> insert conversation_timeline_events
-> insert message_outbox
-> commit
```

同时让压测器支持多个 `message-service` gRPC target，模拟：

```text
loadtest client
-> message-service instance 1
-> message-service instance 2
-> ...
-> shared PostgreSQL
-> shared outbox relay
-> Kafka
```

## 2. 本阶段代码变化

新增 repository 细分指标：

```text
repository_begin_latency_ms
repository_idempotency_lock_latency_ms
repository_find_existing_latency_ms
repository_ensure_seq_latency_ms
repository_allocate_seq_latency_ms
repository_insert_message_latency_ms
repository_insert_timeline_latency_ms
repository_insert_outbox_latency_ms
repository_commit_latency_ms
```

压测 summary 新增：

```text
targets
service_metrics[]
relay_metrics[]
service_latency_metrics
relay_latency_metrics
```

新增脚本：

```text
loadtest/sendmessage/run-local-pgpool-gradient.ps1
loadtest/sendmessage/run-local-multi-instance.ps1
```

## 3. 执行方式

### 3.1 PG 连接池梯度

脚本入口：

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1
```

正式建议命令：

```powershell
. .\tools\go-env.ps1
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 16,32,64,96,128 `
  -VUs 1200,1600,2000,2400 `
  -Duration 60s `
  -StatsWait 30s `
  -ConversationCount 3000 `
  -RelayWorkers 8 `
  -PGMinConns 16
```

### 3.2 多 message-service 实例

脚本入口：

```powershell
.\loadtest\sendmessage\run-local-multi-instance.ps1
```

正式建议命令：

```powershell
. .\tools\go-env.ps1
.\loadtest\sendmessage\run-local-multi-instance.ps1 `
  -Instances 1,2,4 `
  -VUs 1200 `
  -Duration 60s `
  -StatsWait 30s `
  -ConversationCount 3000 `
  -PGMaxConns 64 `
  -PGMinConns 16 `
  -RelayWorkers 8
```

压测器会把多个 target 作为逗号分隔值传入：

```text
--target=127.0.0.1:10495,127.0.0.1:10496
```

并轮询发送请求，模拟客户端侧负载均衡。

## 4. Smoke 验证

### 4.1 PG pool smoke

执行命令：

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 16,64 `
  -VUs 20 `
  -Duration 5s `
  -StatsWait 5s `
  -ConversationCount 100 `
  -RelayWorkers 2 `
  -ResultRoot loadtest\results\pgpool-smoke-20260609-013424
```

结果：

```text
commit=e87bb9b
git_dirty=false

pgmax=16:
  requests=6106
  success_rate=1.0
  p95=27.03ms
  p99=42.52ms
  summary=loadtest/results/pgpool-smoke-20260609-013424/pgmax-16-vu-20-20260609-013428/sendmessage-summary.json

pgmax=64:
  requests=6512
  success_rate=1.0
  p95=23.48ms
  p99=33.46ms
  summary=loadtest/results/pgpool-smoke-20260609-013424/pgmax-64-vu-20-20260609-013442/sendmessage-summary.json
```

这组 smoke 的意义：

```text
1. PG_MAX_CONNS 参数能正确传入真实 gRPC / relay 进程。
2. summary 能记录细分 repository 指标。
3. 短低压下 PG max conns 16 -> 64 有可见改善，但该结果不能外推到高 VU。
```

### 4.2 Multi instance smoke

执行命令：

```powershell
.\loadtest\sendmessage\run-local-multi-instance.ps1 `
  -Instances 1,2 `
  -VUs 20 `
  -Duration 5s `
  -StatsWait 5s `
  -ConversationCount 100 `
  -PGMaxConns 16 `
  -RelayWorkers 2 `
  -ResultRoot loadtest\results\multi-instance-smoke-20260609-013511 `
  -SkipBuild
```

结果：

```text
commit=e87bb9b
git_dirty=false

instances=1:
  target=127.0.0.1:10495
  service_metrics_count=1
  requests=6127
  success_rate=1.0
  p95=26.28ms
  p99=40.43ms
  summary=loadtest/results/multi-instance-smoke-20260609-013511/instances-1-vu-20-20260609-013511/sendmessage-summary.json

instances=2:
  target=127.0.0.1:10495,127.0.0.1:10496
  service_metrics_count=2
  requests=6182
  success_rate=1.0
  p95=24.22ms
  p99=39.35ms
  summary=loadtest/results/multi-instance-smoke-20260609-013511/instances-2-vu-20-20260609-013524/sendmessage-summary.json
```

这组 smoke 的意义：

```text
1. loadtest 支持多个 gRPC target。
2. loadtest 支持多个 service metrics URL。
3. summary 中 service_metrics[] 能分别保存每个 message-service 实例的指标。
```

## 5. Outbox 状态

smoke 使用 `stats-wait=5s`，summary 读取时仍可能存在短暂 pending。后续追加短 relay drain 后确认：

```text
tenant-loadtest-20260609013431 total=6106 published=6106 pending=0 dlq=0
tenant-loadtest-20260609013444 total=6512 published=6512 pending=0 dlq=0
tenant-loadtest-20260609013513 total=6127 published=6127 pending=0 dlq=0
tenant-loadtest-20260609013526 total=6182 published=6182 pending=0 dlq=0
```

因此本阶段 smoke 没有留下未发布 outbox 积压。

## 6. 如何继续排查瓶颈

正式压测时重点比较：

```text
request p99
send_message p99
repository_append p99
repository_begin p99
repository_idempotency_lock p99
repository_find_existing p99
repository_ensure_seq p99
repository_allocate_seq p99
repository_insert_message p99
repository_insert_timeline p99
repository_insert_outbox p99
repository_commit p99
service pgxpool acquire avg
service pgxpool empty_acquire_count
outbox pending
kafka_publish p99
```

判断方法：

```text
1. 如果 request p99 仍约等于 repository_append p99：
   继续看 repository 内部分段。

2. 如果 repository_begin p99 接近 request p99：
   说明主要是 pgxpool acquire / begin 排队。

3. 如果 repository_allocate_seq p99 上升：
   说明 conversation_seq 行锁或 PostgreSQL update 成为瓶颈。

4. 如果 insert_message / insert_timeline / insert_outbox p99 上升：
   说明表索引、WAL、checkpoint、磁盘写入或 batch 写入模型需要优化。

5. 如果 Kafka publish p99 上升或 outbox pending 持续增加：
   说明瓶颈转移到 relay / Kafka。

6. 如果多实例后 p99 下降：
   说明单实例并发能力不足是主要问题之一。

7. 如果多实例后 p99 不降反升：
   说明瓶颈已经转移到共享 PostgreSQL，不能继续堆服务实例。
```

## 7. 当前结论

本阶段已经完成诊断工具链，不形成最终容量结论。

可以确认：

```text
repository 内部分段指标已经落地
PG pool 梯度脚本可运行
多实例脚本可运行
loadtest 多 target 轮询可运行
多 service metrics URL 可记录
smoke 结果来自 clean commit e87bb9b
```

下一步应该跑正式矩阵：

```text
PG_MAX_CONNS = 16 / 32 / 64 / 96 / 128
VU = 1200 / 1600 / 2000 / 2400
Instances = 1 / 2 / 4
Duration = 60s
StatsWait = 30s
ConversationCount >= 3000
```

正式矩阵完成后，再写下一份容量报告，不覆盖本文。
