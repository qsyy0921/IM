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

## 5. 正式 PG Pool 矩阵

执行命令：

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 16,32,64,96,128 `
  -VUs 1200,1600,2000,2400 `
  -Duration 60s `
  -StatsWait 30s `
  -ConversationCount 3000 `
  -RelayWorkers 8 `
  -PGMinConns 16 `
  -ResultRoot loadtest\results\pgpool-formal-20260609-014259
```

有效结果：

| PG max conns | VU | requests | success | p95 ms | p99 ms | pending@stats | begin p99 ms | append p99 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 16 | 1200 | 60979 | 1.0000 | 1581.79 | 1725.16 | 0 | 1695.76 | 1716.43 |
| 16 | 1600 | 57777 | 0.6870 | 2004.82 | 2009.65 | 0 | 2005.84 | 2015.76 |
| 16 | 2000 | 62105 | 0.0632 | 2009.91 | 2043.41 | 0 | 2024.31 | 2032.61 |
| 16 | 2400 | 74240 | 0.0386 | 2007.55 | 2012.89 | 0 | 2008.90 | 2021.96 |
| 32 | 1200 | 81878 | 1.0000 | 1135.84 | 1381.33 | 0 | 1312.18 | 1342.15 |
| 32 | 1600 | 86937 | 1.0000 | 1341.16 | 1476.37 | 1611 | 1420.18 | 1443.58 |
| 32 | 2000 | 82950 | 0.9712 | 1858.23 | 2001.39 | 0 | 2000.85 | 2001.25 |
| 32 | 2400 | 74479 | 0.0919 | 2013.00 | 2025.33 | 0 | 2019.32 | 2041.80 |
| 64 | 1200 | 83052 | 1.0000 | 1171.66 | 1363.46 | 8044 | 1231.19 | 1280.70 |
| 64 | 1600 | 81569 | 1.0000 | 1578.49 | 1707.24 | 19851 | 1588.07 | 1638.54 |
| 64 | 2000 | 95081 | 0.9993 | 1550.04 | 1820.74 | 49948 | 1651.36 | 1704.62 |
| 64 | 2400 | 74125 | 0.0522 | 2014.86 | 2028.63 | 0 | 2029.61 | 2090.88 |

无效结果：

```text
PostgreSQL max_connections=100。
PG_MAX_CONNS=96/128 时，gRPC 进程 + relay 进程 + loadtest 统计连接会超过 PostgreSQL 当前连接上限。
PostgreSQL 返回：
FATAL: sorry, too many clients already (SQLSTATE 53300)
```

`pgmax-128-vu-2000` 曾产生一个异常 summary：

```text
requests=14451745
success_rate=0.0028
p99=0.77ms
```

这是服务端连接失败后的快速错误样本，不是有效吞吐结果，不纳入容量判断。

## 6. 正式 Multi Instance 矩阵

执行命令：

```powershell
.\loadtest\sendmessage\run-local-multi-instance.ps1 `
  -Instances 1,2,4 `
  -VUs 1200 `
  -Duration 60s `
  -StatsWait 30s `
  -ConversationCount 3000 `
  -PGMaxConns 16 `
  -RelayWorkers 8 `
  -ResultRoot loadtest\results\multi-instance-formal-20260609-021254 `
  -SkipBuild
```

结果：

| instances | VU | requests | success | errors | p95 ms | p99 ms | pending@stats | service count | begin p99 min/max ms | append p99 min/max ms |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1200 | 58492 | 0.9660 | 1989 | 1781.87 | 2000.52 | 0 | 1 | 2000.25 / 2000.25 | 2000.37 / 2000.37 |
| 2 | 1200 | 53979 | 0.5572 | 23902 | 2001.98 | 2005.63 | 0 | 2 | 2003.54 / 2005.69 | 2003.82 / 2005.93 |
| 4 | 1200 | 90392 | 0.9014 | 8914 | 2000.43 | 2001.52 | 0 | 4 | 603.59 / 2002.33 | 650.55 / 2002.58 |

多实例没有改善请求 p99。原因不是 loadtest 无法分发，summary 中 `service_metrics` 已确认分别采集到 1/2/4 个实例。需要注意：旧 summary 顶层 `service_latency_metrics` 只代表第一个 service metrics URL，因此本表使用 `service_metrics[]` 重新计算每个实例的 min/max。

4 实例时不是每个实例都达到 2s，但至少部分实例的 `repository_begin` / `repository_append` p99 仍贴近 2s，请求整体 p99 也贴近 2s。这说明本地多实例没有绕开共享 PostgreSQL 压力，继续堆服务实例会把问题转移到 PostgreSQL 连接和写入能力。

## 7. Outbox 状态

smoke 使用 `stats-wait=5s`，summary 读取时仍可能存在短暂 pending。追加短 relay drain 后，smoke tenant 均已清零：

```text
tenant-loadtest-20260609013431 total=6106 published=6106 pending=0 dlq=0
tenant-loadtest-20260609013444 total=6512 published=6512 pending=0 dlq=0
tenant-loadtest-20260609013513 total=6127 published=6127 pending=0 dlq=0
tenant-loadtest-20260609013526 total=6182 published=6182 pending=0 dlq=0
```

正式矩阵追加短 relay drain 后确认：

```text
tenant_count=16
total_pending=0
```

因此本阶段没有留下未发布 outbox 积压。

## 8. 瓶颈证据链

本阶段进一步确认：

```text
request p99 ~= repository_append p99 ~= repository_begin p99
```

典型样本：

```text
PG_MAX_CONNS=16 / VU=1200:
  request p99 = 1725.16ms
  repository_append p99 = 1716.43ms
  repository_begin p99 = 1695.76ms

PG_MAX_CONNS=32 / VU=1200:
  request p99 = 1381.33ms
  repository_append p99 = 1342.15ms
  repository_begin p99 = 1312.18ms

PG_MAX_CONNS=64 / VU=2000:
  request p99 = 1820.74ms
  repository_append p99 = 1704.62ms
  repository_begin p99 = 1651.36ms
```

这说明当前主要瓶颈不是 Kafka、不是 outbox relay、不是 commit，也不是单个 `conversation_seq` update，而是：

```text
message-service 进入 PostgreSQL 事务前后的连接获取 / begin 阶段。
```

同时，`PG_MAX_CONNS=64` 下 outbox pending 明显升高：

```text
1200 VU pending=8044
1600 VU pending=19851
2000 VU pending=49948
```

这说明单纯放大写入并发会把压力转移到 outbox relay 追平能力。

多实例矩阵进一步确认：

```text
1 instance / 1200 VU: success=0.9660, p99=2000.52ms
2 instance / 1200 VU: success=0.5572, p99=2005.63ms
4 instance / 1200 VU: success=0.9014, p99=2001.52ms
```

多实例没有降低请求 p99。更准确地说：在当前单 PostgreSQL、当前连接上限和当前写入模型下，多 gRPC 实例不能解决尾延迟，瓶颈已经不只是单个 gRPC 进程本身。

## 9. PostgreSQL 诊断

本阶段新增只读诊断脚本：

```powershell
.\loadtest\sendmessage\collect-postgres-diagnostics.ps1
```

正式矩阵后采集结果：

```text
result=loadtest/results/postgres-diagnostics-20260609-022602/postgres-diagnostics.json
max_connections=100
shared_buffers=16384
max_wal_size=1024
checkpoint_timeout=300
synchronous_commit=on
deadlocks=0
xact_commit=2012277
xact_rollback=79999
```

表统计摘要：

| table | n_live_tup | n_dead_tup | n_tup_ins | n_tup_upd | idx_scan |
| --- | ---: | ---: | ---: | ---: | ---: |
| message_log | 4211748 | 6222 | 1806744 | 0 | 1832893 |
| conversation_timeline_events | 4209611 | 3951 | 1803312 | 0 | 134539034 |
| message_outbox | 4215639 | 113312 | 1800686 | 1806169 | 236430976 |
| conversation_seq | 186184 | 0 | 101519 | 1811665 | 3629996 |

这进一步解释了 `PG_MAX_CONNS=96/128` 为什么不可用：

```text
当前 PostgreSQL max_connections=100。
如果单个 gRPC 进程 PG_MAX_CONNS=96，再叠加 relay 进程、loadtest 统计连接、后台连接，就会超过 100。
所以 too many clients 是配置上限，不是业务语义错误。
```

`message_outbox` 的 `n_dead_tup=113312` 也说明 outbox 高频 `PENDING -> PUBLISHED` update 会产生明显 dead tuples，后续需要关注 autovacuum 和批量 mark published。

## 10. 当前结论

本阶段可以得出比上一轮更明确的结论：

```text
1. 当前 repository 内部分段指标显示 p99 主要贴在 repository_begin。
2. PG_MAX_CONNS=16 太小，1200 VU 已 p99 1725ms。
3. PG_MAX_CONNS=32 能提高成功率，但 p99 仍超过 1s。
4. PG_MAX_CONNS=64 能提高部分高 VU 写入成功率，但 outbox pending 明显增加。
5. PG_MAX_CONNS=96/128 在当前 PostgreSQL max_connections 下不可用。
6. 多 message-service 实例没有降低当前请求 p99；在当前单 PostgreSQL、当前连接上限和当前写入模型下，继续堆服务实例不能解决尾延迟。
```

下一步不应该继续盲目增加 `message-service` 实例或连接池，而应优先做：

```text
PostgreSQL max_connections / shared_buffers / checkpoint / WAL 参数调优
pg_stat_activity / pg_stat_statements / lock wait 观测
repository 写入 SQL 的单语句耗时和 EXPLAIN
outbox relay 批量发布和批量 mark published 优化
降低业务请求直接等待连接池的时间，例如 admission control / backpressure
```

下一轮压测需要把变量拆开：

```text
固定每实例 PG 连接预算：观察服务实例增加后的总连接数放大效应。
固定总 PG 连接预算：例如 1x64、2x32、4x16，观察入口扩容是否仍有价值。
```

`run-local-multi-instance.ps1` 已支持这两种模式。下一轮可以直接使用：

```powershell
.\loadtest\sendmessage\run-local-multi-instance.ps1 `
  -ConnectionBudgetMode FixedPerInstance `
  -Instances 1,2,4 `
  -PGMaxConns 16 `
  -VUs 1200 `
  -Duration 60s `
  -StatsWait 30s

.\loadtest\sendmessage\run-local-multi-instance.ps1 `
  -ConnectionBudgetMode FixedTotal `
  -Instances 1,2,4 `
  -TotalPGMaxConns 64 `
  -VUs 1200 `
  -Duration 60s `
  -StatsWait 30s
```

当前 debug metrics collector 会保存全量样本并在 snapshot 时排序，适合本地短压测，不适合作为长期运行的生产 metrics。后续应替换为固定窗口、reservoir、HDR histogram 或 Prometheus histogram。

下一阶段仍需新建独立报告，不覆盖本文。
