# NexusIM SendMessage Multi-Instance PG Budget Loadtest

日期：2026-06-09

## 1. 压测目标

本轮验证一个更严格的问题：

```text
message-service 实例数增加后，如果 PostgreSQL 总连接预算不变，入口扩容是否还能降低 SendMessage 尾延迟。
```

上一轮 multi-instance 矩阵的问题是变量混在一起：实例数从 1 到 4 增加时，每个实例仍配置 `PG_MAX_CONNS=16`，所以 PostgreSQL 总连接预算同时从 16 增加到 32、64。本轮把它拆成两类：

```text
固定每实例连接预算：1x16、2x16、4x16，观察总连接数放大后的表现。
固定总连接预算：1x64、2x32、4x16，观察入口扩容是否仍有价值。
```

## 2. 压测拓扑

```text
loadtest/sendmessage
-> 1/2/4 个 message-service gRPC 进程
-> PostgreSQL local transaction
-> message_log / conversation_timeline_events / message_outbox
-> 1 个 outbox relay 进程
-> Kafka conversation.timeline.events
```

本轮是 Windows 本机压测：

```text
客户端：Windows 本机 loadtest/sendmessage
服务端：Windows 本机多个 message-service 进程
数据库：Docker Desktop 内 nexusim-postgres
Kafka：Docker Desktop 内 nexusim-kafka
Docker 资源：16 CPU，约 24GB memory
```

## 3. 执行方式

脚本已扩展：

```powershell
.\loadtest\sendmessage\run-local-multi-instance.ps1 `
  -ConnectionBudgetMode FixedPerInstance `
  -Instances 1,2,4 `
  -PGMaxConns 16 `
  -VUs 1200 `
  -Duration 60s `
  -StatsWait 30s `
  -ConversationCount 3000 `
  -RelayWorkers 8 `
  -BatchSize 500 `
  -ResultRoot loadtest\results\multi-instance-budget-formal-20260609

.\loadtest\sendmessage\run-local-multi-instance.ps1 `
  -ConnectionBudgetMode FixedTotal `
  -Instances 1,2,4 `
  -TotalPGMaxConns 64 `
  -VUs 1200 `
  -Duration 60s `
  -StatsWait 30s `
  -ConversationCount 3000 `
  -RelayWorkers 8 `
  -BatchSize 500 `
  -ResultRoot loadtest\results\multi-instance-budget-formal-20260609
```

固定总连接预算 smoke 已先验证：

```text
1x8：success_rate=1.0，p99=20.72ms，outbox_pending=0，service_pg_pool.max_conns=8
2x4：success_rate=1.0，p99=27.96ms，outbox_pending=0，service_pg_pool.max_conns=8
```

## 4. 通过标准

沿用前一阶段本地门槛：

```text
success_rate >= 0.99
p99 <= 1000ms
outbox_pending_count <= 1000
```

## 5. 正式结果

所有正式 summary 的 `git_dirty=false`，对应本地 clean commit：

```text
ede5dd7 feat: add multi-instance pg budget modes
```

### 5.1 固定每实例连接预算

| topology | requests | success | p95 ms | p99 ms | outbox pending | service PG max | PG acquire avg ms | repository begin p99 ms | repository append p99 ms | commit p99 ms | Kafka publish p99 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1x16 | 55394 | 0.9593 | 1907.57 | 2000.53 | 0 | 16 | 1271.79 | 2000.37 | 2000.44 | 23.70 | 12.72 |
| 2x16 | 72420 | 1.0000 | 1334.40 | 1437.37 | 0 | 32 | 973.57 | 1434.96 | 1469.70 | 27.84 | 19.20 |
| 4x16 | 83594 | 0.9907 | 1710.06 | 1975.75 | 979 | 64 | 817.92 | 2000.32 | 2000.54 | 42.87 | 26.77 |

### 5.2 固定总连接预算

| topology | requests | success | p95 ms | p99 ms | outbox pending | service PG max | PG acquire avg ms | repository begin p99 ms | repository append p99 ms | commit p99 ms | Kafka publish p99 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1x64 | 96112 | 1.0000 | 1013.44 | 1178.97 | 31889 | 64 | 711.69 | 1110.25 | 1174.51 | 32.89 | 22.28 |
| 2x32 | 89474 | 1.0000 | 1407.50 | 1509.55 | 64553 | 64 | 765.88 | 1491.33 | 1535.13 | 38.60 | 20.70 |
| 4x16 | 80460 | 1.0000 | 1452.94 | 1657.18 | 65323 | 64 | 853.14 | 1723.02 | 1762.82 | 43.87 | 24.70 |

固定总连接预算矩阵后，额外启动 relay drain 60s，最终 outbox 全部变为 `PUBLISHED`：

```text
PUBLISHED|4687706
```

## 6. 瓶颈排查过程

### 6.1 先看请求 p99 是否贴近 repository append

固定总连接预算下：

```text
1x64：request p99=1178.97ms，repository append p99=1174.51ms
2x32：request p99=1509.55ms，repository append p99=1535.13ms
4x16：request p99=1657.18ms，repository append p99=1762.82ms
```

请求尾延迟仍主要贴在 repository append 这一整段，而不是 gRPC handler 或 loadtest 客户端。

### 6.2 再拆 repository append 内部

`repository_begin` 是主要等待段：

```text
1x64：begin p99=1110.25ms
2x32：begin p99=1491.33ms
4x16：begin p99=1723.02ms
```

其他写入段远低于 begin：

```text
commit p99：32.89ms / 38.60ms / 43.87ms
insert_outbox p99：16.50ms / 18.51ms / 27.54ms
Kafka publish p99：22.28ms / 20.70ms / 24.70ms
```

因此当前主要问题不是 Kafka publish，也不是单条 insert 的 SQL 本身，而是进入 PostgreSQL 事务前后的连接获取 / begin 阶段排队。

### 6.3 再看连接池等待

固定总连接预算下，PG acquire 平均等待仍很高：

```text
1x64：711.69ms
2x32：765.88ms
4x16：853.14ms
```

这说明把同样的 64 个数据库连接切给更多 gRPC 进程，并没有消除连接获取等待；实例越多，调度、连接池切分和共享 PostgreSQL 争用越明显。

### 6.4 最后看 outbox 追平

固定总预算组全部 100% 写入成功，但 outbox pending 很高：

```text
1x64：31889
2x32：64553
4x16：65323
```

这说明写入吞吐被拉高后，当前 relay 配置不能在 `stats-wait=30s` 内追平。即使业务请求成功率达标，也不能把链路称为健康闭环。

## 7. 当前结论

1. 固定每实例连接预算时，2 个实例比 1 个实例更稳，但 4 个实例又回到接近 2s p99；继续堆实例不是稳定解。
2. 固定总连接预算时，1 个实例反而是三组里 p99 最低的，2/4 个实例没有降低尾延迟。
3. 当前瓶颈主要表现为 PostgreSQL 连接获取 / begin 阶段排队，仍需要更细的 PostgreSQL 侧观测来区分 pgxpool 等待、PostgreSQL 连接上限、WAL/checkpoint、OS 调度等因素。
4. relay 已不是请求 p99 主因，但在高写入吞吐下会形成大量 outbox pending，是第二瓶颈。

## 8. 下一步

下一阶段不继续盲目加实例，优先做：

```text
PostgreSQL 调优实验：max_connections、shared_buffers、WAL/checkpoint、autovacuum。
pg_stat_activity / wait_event / pg_stat_statements 采集。
拆分 pgxpool acquire 与 BEGIN 执行时间。
outbox relay 批量 publish / 批量 mark published。
admission control / backpressure：连接池等待超过阈值时快速限流，而不是让请求排队到 2s。
```

下一阶段继续新增独立报告，不覆盖本文。
