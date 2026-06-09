# NexusIM PostgreSQL Loadtest Profile Report

日期：2026-06-09

## 1. 压测目标

本轮验证 PostgreSQL 压测专用配置是否能缓解 SendMessage 尾延迟：

```text
默认 PostgreSQL 参数
-> loadtest override: max_connections / shared_buffers / WAL / checkpoint / autovacuum
-> PG pool 梯度压测
-> PostgreSQL wait_event 采样
```

重点不是只看请求成功率，而是继续解释：

```text
repository_begin 的等待到底是 pgxpool acquire，还是 SQL BEGIN / DB 内部等待。
```

## 2. 配置变更

新增压测专用 compose override：

```text
deploy/local/docker-compose.postgres-loadtest.yml
```

启用命令：

```powershell
docker compose `
  -f deploy\local\docker-compose.yml `
  -f deploy\local\docker-compose.postgres-loadtest.yml `
  up -d postgres
```

生效参数：

| 参数 | 当前值 | 说明 |
| --- | ---: | --- |
| `max_connections` | 200 | 允许更大的本地 PG pool 梯度 |
| `shared_buffers` | 1GB | 默认约 128MB，压测 profile 提高缓存 |
| `effective_cache_size` | 8GB | 提示规划器本地可用缓存更大 |
| `work_mem` | 16MB | 本阶段 SQL 不依赖大排序，仅保守提高 |
| `maintenance_work_mem` | 512MB | 给 vacuum / maintenance 留空间 |
| `wal_buffers` | 16MB | 提高 WAL buffer |
| `max_wal_size` | 4GB | 降低 checkpoint 频率 |
| `checkpoint_timeout` | 15min | 降低 checkpoint 频率 |
| `checkpoint_completion_target` | 0.9 | 平滑 checkpoint 写入 |
| `autovacuum_*_scale_factor` | 0.02 | 更积极处理 outbox update dead tuples |
| `autovacuum_*_threshold` | 1000 | 降低触发门槛 |

## 3. 观测能力

本轮也新增并使用：

```text
loadtest/sendmessage/watch-postgres-diagnostics.ps1
```

它在压测期间按间隔采集：

```text
pg_stat_activity state / wait_event
pg_locks waiting
pg_stat_database
pg_stat_user_tables
pg_stat_bgwriter
pg_stat_wal
```

输出：

```text
loadtest/results/postgres-watch-pgpool-tuned-formal-20260609/postgres-wait-samples.jsonl
```

同时，代码已把 `repository_begin_latency_ms` 拆为：

```text
repository_pool_acquire_latency_ms
repository_tx_begin_latency_ms
```

旧的 `repository_begin_latency_ms` 保持总耗时，兼容旧报告。

## 4. 执行命令

先跑 smoke：

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 128 `
  -VUs 20 `
  -Duration 5s `
  -StatsWait 5s `
  -ConversationCount 100 `
  -RelayWorkers 4 `
  -BatchSize 200 `
  -ResultRoot loadtest\results\postgres-loadtest-profile-smoke-20260609
```

正式矩阵：

```powershell
$watch = Start-Job -ScriptBlock {
  Set-Location 'E:\development\IM'
  .\loadtest\sendmessage\watch-postgres-diagnostics.ps1 `
    -Samples 220 `
    -IntervalSeconds 2 `
    -ResultDir loadtest\results\postgres-watch-pgpool-tuned-formal-20260609
}

.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 64,128 `
  -VUs 1200,1600 `
  -Duration 60s `
  -StatsWait 30s `
  -ConversationCount 3000 `
  -RelayWorkers 8 `
  -BatchSize 500 `
  -ResultRoot loadtest\results\pgpool-tuned-formal-20260609 `
  -SkipBuild

Wait-Job $watch | Out-Null
Receive-Job $watch
Remove-Job $watch
```

## 5. Smoke 结果

```text
commit=fb2fe97
git_dirty=false
PG_MAX_CONNS=128
VU=20
duration=5s
requests=5483
success_rate=1.0
p99=29.70ms
outbox_pending=0
repository_pool_acquire_p99=0ms
repository_tx_begin_p99=2.16ms
repository_begin_p99=2.34ms
```

## 6. 正式矩阵结果

| PG max conns | VU | requests | success | p95 ms | p99 ms | pending@stats | pool acquire p99 ms | tx begin p99 ms | repository begin p99 ms | append p99 ms | commit p99 ms | Kafka p99 ms |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 64 | 1200 | 91253 | 1.0000 | 1018.64 | 1161.70 | 16477 | 1101.82 | 14.09 | 1107.49 | 1160.00 | 43.34 | 22.56 |
| 64 | 1600 | 86652 | 1.0000 | 1543.62 | 1759.11 | 26493 | 1693.96 | 18.33 | 1698.22 | 1755.08 | 61.79 | 26.75 |
| 128 | 1200 | 88803 | 0.9760 | 1359.01 | 2001.08 | 53581 | 1999.80 | 25.24 | 1999.84 | 2000.55 | 309.52 | 44.16 |
| 128 | 1600 | 83329 | 0.9975 | 1613.79 | 1796.07 | 68649 | 1692.26 | 32.95 | 1696.66 | 1791.62 | 348.69 | 25.80 |

矩阵结束后额外 drain 60s，当前 outbox 全部为 `PUBLISHED`：

```text
PUBLISHED|5048898
```

## 7. PostgreSQL Wait 采样

采样文件：

```text
loadtest/results/postgres-watch-pgpool-tuned-formal-20260609/postgres-wait-samples.jsonl
```

摘要：

```text
samples=220
max_active=135
max_total_backends=143
max_waiting=141
top_waits:
  Client:ClientRead=10323
  LWLock:WALWrite=2269
  none:none=1471
  LWLock:WALInsert=238
  LWLock:BufferContent=234
  Timeout:CheckpointWriteDelay=122
```

`Client:ClientRead` 主要代表连接在等客户端输入，不直接等价于数据库内部瓶颈。更有价值的是：

```text
LWLock:WALWrite
LWLock:WALInsert
LWLock:BufferContent
Timeout:CheckpointWriteDelay
```

这些等待说明在高并发写入下，WAL 写入、buffer content 和 checkpoint 已经进入瓶颈视野。

## 8. 瓶颈排查

### 8.1 acquire 与 tx begin 已拆开

正式矩阵里：

```text
PG64/VU1200:
  pool acquire p99 = 1101.82ms
  tx begin p99     = 14.09ms

PG64/VU1600:
  pool acquire p99 = 1693.96ms
  tx begin p99     = 18.33ms

PG128/VU1200:
  pool acquire p99 = 1999.80ms
  tx begin p99     = 25.24ms
```

因此原来的 `repository_begin` 主体就是 pgxpool acquire 等待，不是 SQL `BEGIN` 本身。

### 8.2 提高 max_connections 不等于提升容量

`max_connections=200` 让 `PG_MAX_CONNS=128` 变得可运行，但结果更差：

```text
PG128/VU1200 success=0.9760, p99=2001.08ms
PG128/VU1600 success=0.9975, p99=1796.07ms
```

commit p99 也明显升高：

```text
PG64 commit p99: 43.34ms / 61.79ms
PG128 commit p99: 309.52ms / 348.69ms
```

这说明把更多连接放进 PostgreSQL 后，连接等待没有消失，反而放大了 WAL/checkpoint/commit 争用。

### 8.3 relay 成为第二瓶颈

`PG64/VU1200` 写入成功率 100%，但 `stats-wait=30s` 后 pending 仍有 16477。

`PG128` 两组 pending 达到 53581 / 68649。高写入并发把 outbox 追平问题放大了，不能只优化请求写路径。

## 9. 当前结论

1. PostgreSQL loadtest profile 生效，但没有把 1200/1600 VU 压到 p99 1s 内。
2. `repository_begin` 已被确认主要是 pgxpool acquire 等待。
3. SQL `BEGIN` 本身不是主瓶颈。
4. 提高 `max_connections` 到 200 后，`PG_MAX_CONNS=128` 会放大 WAL/commit/outbox 压力，不是有效解法。
5. 当前优先级应转向：

```text
限制无效排队：admission control / backpressure
降低单请求占用连接时间：减少事务 round trips / prepared statements / 批量策略
优化 outbox relay：批量 publish / 批量 mark published
继续采集 PG wait_event：重点看 WALWrite、WALInsert、BufferContent、CheckpointWriteDelay
```

## 10. 下一步

下一阶段建议先做 backpressure，而不是继续盲目调大连接数：

```text
当 pgxpool acquire 平均/分位数持续升高时，gRPC adapter 或 app 层快速返回 retryable overload。
为 loadtest 增加 overload 错误统计。
压测目标从“全部排队到 2s”改成“部分请求快速失败，但系统 p99 和 outbox 追平保持稳定”。
```

后续仍需新建独立报告，不覆盖本文。
