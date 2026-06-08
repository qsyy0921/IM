# 本地双机压测 Runbook

## 1. 机器与网络

当前本地双机压测只用于开发阶段，不代表目标态生产拓扑。

| 角色 | 地址 | 用途 |
| --- | --- | --- |
| Windows 本机 | `192.168.0.141` | 服务端、开发机 |
| MacBook | `192.168.0.182` | 压测客户端、双向 callback/mock receiver |

两端本地 Git/HTTP 代理统一使用：

```text
127.0.0.1:7890
```

## 2. 端口分配

`8080` 不作为 NexusIM 本地压测端口。

| 端口 | 方向 | 用途 |
| ---: | --- | --- |
| `10495` | MacBook -> Windows；Windows -> MacBook 可对称使用 | 主 HTTP/API 压测入口 |
| `10496` | MacBook -> Windows | push-gateway WebSocket 压测入口 |
| `10497` | MacBook -> Windows | message-service gRPC 进程 metrics/debug，只在压测窗口开放 |
| `10498` | Windows -> MacBook | callback/mock receiver，用于双向新建连接场景 |
| `10499` | MacBook <-> Windows | load coordinator / report endpoint |
| `10500` | MacBook -> Windows | message-service outbox relay 进程 metrics/debug，只在压测窗口开放 |
| `10501-10510` | 按需双向 | 预留给服务级 SDD、故障注入、临时对照实验 |

两台机器可以使用相同端口号，因为监听地址不同，例如：

```text
192.168.0.141:10495
192.168.0.182:10495
```

这两个监听不冲突。

## 3. 防火墙约束

Windows 防火墙规则：

```text
规则名: NexusIM LoadTest 10495-10510 from MacBook
协议: TCP
本地端口: 10495-10510
远端地址: 192.168.0.182
动作: Allow
```

如 MacBook 开启系统防火墙，只允许 Windows 访问同一端口段：

```text
192.168.0.141 -> 10495-10510
```

非压测窗口不启动这些端口上的服务。

## 4. 服务监听要求

被压测服务必须监听：

```text
0.0.0.0:<port>
```

不能只监听：

```text
127.0.0.1:<port>
```

否则对端机器无法访问。

## 5. 验证命令

MacBook 验证 Windows 服务端口：

```bash
nc -vz 192.168.0.141 10495
curl -I http://192.168.0.141:10495/healthz
```

Windows 验证 MacBook callback/mock receiver：

```powershell
Test-NetConnection 192.168.0.182 -Port 10498
```

## 6. 压测原则

- 第一轮只压真实服务进程，不压固定字符串 toy endpoint。
- 压测脚本不能写死 IP、端口、并发和持续时间，必须通过参数或环境变量传入。
- 每次压测记录目标 commit、机器、端口、并发、请求数、p95/p99、错误率。
- 压测结果输出到 `loadtest/results/<date>/`。
- 先跑短压测确认功能，再跑长压测观察资源和稳定性。
- 本地双机结果只用于发现早期瓶颈和趋势，不作为生产容量承诺。
- 如需记录 `conversation_seq_alloc_latency`、Kafka publish、outbox relay 分段指标，gRPC 进程与 outbox relay 进程必须分别设置 `NEXUSIM_DEBUG_ADDR`，并把对应地址传给压测脚本。`kafka_publish_latency` 只保留兼容旧报告；正式报告优先看 `kafka_publish_call_latency`、`kafka_publish_records_per_call` 和 `kafka_publish_record_latency_estimate`，避免 single path 和 batch path 口径混用。

推荐参数形式：

```bash
go run ./loadtest/sendmessage \
  --target=192.168.0.141:10495 \
  --vus=100 \
  --duration=60s \
  --stats-wait=8s \
  --result-dir=loadtest/results/2026-06-08 \
  --pg-dsn=postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable \
  --service-metrics-url=http://192.168.0.141:10497/debug/metrics \
  --relay-metrics-url=http://192.168.0.141:10500/debug/metrics
```

本机 worker 梯度压测可以使用脚本启动 gRPC 进程、outbox relay 进程和压测客户端：

```powershell
.\loadtest\sendmessage\run-local-gradient.ps1 `
  -Workers 4,8,16 `
  -VUs 100 `
  -Duration 60s `
  -StatsWait 30s `
  -ConversationCount 1000 `
  -ResultRoot loadtest\results\gradient-2026-06-08
```

该脚本会为每个 worker 数分别启动独立的 gRPC 和 relay 进程，并通过 `NEXUSIM_DEBUG_ADDR` 采集 seq alloc / Kafka publish latency。

压测期间需要采集 PostgreSQL wait_event / 锁等待 / WAL / checkpoint / 表 dead tuples 时，先启动采样脚本，再运行压测脚本：

```powershell
$watch = Start-Job -ScriptBlock {
  Set-Location 'E:\development\IM'
  .\loadtest\sendmessage\watch-postgres-diagnostics.ps1 `
    -Samples 90 `
    -IntervalSeconds 2 `
    -ResultDir loadtest\results\postgres-watch-20260609
}

.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 64 `
  -VUs 1200 `
  -Duration 60s `
  -StatsWait 30s `
  -ConversationCount 3000 `
  -RelayWorkers 8 `
  -ResultRoot loadtest\results\pgpool-with-pgwatch-20260609

Wait-Job $watch | Out-Null
Receive-Job $watch
Remove-Job $watch
```

采样结果写入 `postgres-wait-samples.jsonl`。每一行是一个时间点，可以和 loadtest summary 的 `started_at` / `finished_at` 对齐。注意 `Client:ClientRead` 通常代表空闲连接等待客户端输入，不等价于数据库内部瓶颈；优先关注 `Lock`、`LWLock`、`IO`、`WALWrite`、checkpoint 和 dead tuple 变化。

等价环境变量形式：

```bash
NEXUSIM_TARGET=192.168.0.141:10495
NEXUSIM_VUS=100
NEXUSIM_DURATION=60s
NEXUSIM_STATS_WAIT=8s
NEXUSIM_RESULT_DIR=loadtest/results/2026-06-08
NEXUSIM_PG_DSN=postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable
NEXUSIM_SERVICE_METRICS_URL=http://192.168.0.141:10497/debug/metrics
NEXUSIM_RELAY_METRICS_URL=http://192.168.0.141:10500/debug/metrics
```

## 7. 边搭建边压测流程

第一阶段不等待全部目标态服务部署完成，只压真实落地的 `message-service SendMessage` 主写链路。

真实范围：

```text
message-service
PostgreSQL local transaction
conversation_seq
message_log
conversation_timeline_events
message_outbox
outbox relay
Kafka publish path
```

mock 范围：

```text
policy-service
conversation-service
timeline-service
delivery-service
push-gateway
RAG
Agent
```

压测阶梯：

| 阶段 | 目标 | 建议参数 | 通过标准 |
| --- | --- | --- | --- |
| smoke | 验证链路可用 | `--vus=10 --duration=30s` | 无 5xx；事务和 outbox 可查询 |
| baseline | 建立 SendMessage 基线 | `--vus=100 --duration=60s` | p95/p99、错误率、outbox pending age 有记录 |
| idempotency | 验证重复发送 | 固定 `client_msg_id` 重放 | 不重复写 `message_log`，冲突返回明确错误 |
| kafka-outage | 验证 outbox 积压 | 临时停止 Kafka 或 producer mock fail | accepted 不受影响，outbox 可追平 |
| ramp | 逐步加压 | `--vus=500` 起按倍数增加 | 找到 PG 行锁、producer 或 CPU 瓶颈 |

每轮压测必须记录：

```text
commit
target
vus
duration
request_count
success_rate
p95
p99
conversation_seq_alloc_latency
outbox_pending_count
outbox_oldest_pending_age
kafka_publish_call_latency
kafka_publish_records_per_call
kafka_publish_record_latency_estimate
error_topn
```

普通会话链路稳定后，才能扩展到热点 sequencer mock、delivery/push mock 和 DLQ repair 演练。

## 8. PostgreSQL 压测配置

默认 `deploy/local/docker-compose.yml` 保持开发配置，不直接塞入高压压测参数。需要跑 PG 调优矩阵时，叠加压测 override：

```powershell
docker compose `
  -f deploy\local\docker-compose.yml `
  -f deploy\local\docker-compose.postgres-loadtest.yml `
  up -d postgres
```

该 override 当前配置：

```text
max_connections=200
shared_buffers=1GB
effective_cache_size=8GB
work_mem=16MB
maintenance_work_mem=512MB
wal_buffers=16MB
max_wal_size=4GB
checkpoint_timeout=15min
checkpoint_completion_target=0.9
autovacuum_vacuum_scale_factor=0.02
autovacuum_analyze_scale_factor=0.02
autovacuum_vacuum_threshold=1000
autovacuum_analyze_threshold=1000
```

启用后验证：

```powershell
docker exec nexusim-postgres psql -U nexusim -d nexusim -c "show max_connections;"
docker exec nexusim-postgres psql -U nexusim -d nexusim -c "show shared_buffers;"
docker exec nexusim-postgres psql -U nexusim -d nexusim -c "show max_wal_size;"
docker exec nexusim-postgres psql -U nexusim -d nexusim -c "show checkpoint_timeout;"
```

恢复默认开发配置：

```powershell
docker compose `
  -f deploy\local\docker-compose.yml `
  up -d --force-recreate postgres
```

该操作不删除 named volume；不要执行 `down -v`，除非明确要清空本地数据库。

## 9. Backpressure Smoke

当 PostgreSQL acquire 已经成为主等待段时，可以启用 message-service 的本地 backpressure：

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 1 `
  -VUs 20 `
  -Duration 5s `
  -StatsWait 5s `
  -ConversationCount 100 `
  -RelayWorkers 2 `
  -BatchSize 100 `
  -BackpressureEnabled `
  -BackpressureMinAvailableConns 0 `
  -ResultRoot loadtest\results\backpressure-smoke-20260609
```

预期结果：

```text
error_topn[0].error = Unavailable: service overloaded
p99 保持毫秒级
outbox_pending_count = 0
```

该 smoke 的目的不是追求高成功率，而是验证系统能快速保护自己，不把请求都堆到连接池里等 2s 超时。正式容量压测仍需单独报告。

## 10. Client Retry Smoke

`SERVICE_OVERLOADED` 会返回 gRPC `RetryInfo`。如果要模拟客户端遵守服务端 retry hint，可以显式打开压测器重试：

```powershell
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 64 `
  -VUs 1200 `
  -Duration 60s `
  -StatsWait 30s `
  -ConversationCount 3000 `
  -RelayWorkers 8 `
  -BatchSize 500 `
  -BackpressureEnabled `
  -BackpressureMinAvailableConns 8 `
  -RetryOverloaded `
  -MaxRetries 2 `
  -RetryJitter 100ms `
  -ResultRoot loadtest\results\backpressure-client-retry-YYYYMMDD
```

summary 中需要同时看：

```text
logical_request_count
logical_success_rate
request_count
retry_attempt_count
retried_request_count
accepted_rps
overload_rate
success_p99_ms
error_p99_ms
```

其中 `request_count` 是实际 gRPC attempt 数，`logical_request_count` 是用户层消息数。开启客户端重试后，两者不能混用；`overload_rate` 也是 attempt-level 指标，不代表用户层最终失败率。

## 11. PublishBatch On/Off

对比 Kafka `PublishBatch` 时必须在同一 HEAD 下使用开关，不要用不同 commit 直接比较：

```powershell
. .\tools\go-env.ps1
go build -o bin\message-service.exe ./services/message-service/cmd/message-service
go build -o bin\sendmessage-loadtest.exe ./loadtest/sendmessage
```

关闭 batch path：

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
  -ResultRoot loadtest\results\publishbatch-formal-off-YYYYMMDD `
  -SkipBuild
```

开启 batch path 时只改为 `-PublishBatchEnabled:$true`。

有效性检查：

```text
PublishBatch=false -> kafka_publish_records_per_call 应接近 1
PublishBatch=true  -> kafka_publish_records_per_call 应大于 1
```

如果使用 `-SkipBuild`，必须先确认 `bin\message-service.exe` 已经由当前 HEAD 重建；否则 summary 的 `commit` 可能是当前仓库 commit，但实际运行的服务二进制仍是旧版本。

## 12. Outbox Batch / Worker Matrix

联合验证 `NEXUSIM_OUTBOX_BATCH_SIZE` 和 `NEXUSIM_OUTBOX_WORKERS` 时，使用包装脚本：

```powershell
.\loadtest\sendmessage\run-local-outbox-batch-worker-matrix.ps1 `
  -BatchSizes 100,500,1000 `
  -RelayWorkers 8,12,16 `
  -VUs 1200,1600 `
  -PGMaxConns 64 `
  -Duration 30s `
  -StatsWait 20s `
  -ConversationCount 1000 `
  -PublishBatchEnabled:$true `
  -BackpressureEnabled `
  -BackpressureMinAvailableConns 8 `
  -RetryOverloaded `
  -MaxRetries 2 `
  -RetryJitter 100ms `
  -ResultRoot loadtest\results\outbox-batch-worker-matrix-YYYYMMDD
```

脚本会为每个 batch/worker 组合调用 `run-local-pgpool-gradient.ps1`，并增量写出：

```text
outbox-batch-worker-matrix-summary.json
```

正式解读时至少同时看：

```text
outbox_pending_count
accepted_rps
success_p99_ms
kafka_publish_records_per_call
kafka_publish_call_latency_ms
outbox_process_ready_latency_ms
outbox_process_ready_active_latency_ms
outbox_process_ready_idle_latency_ms
outbox_fetched_per_call
```

注意：`outbox_process_ready_latency_ms` 会混入 `stats_wait` 阶段的 idle 样本；做 adaptive limit 时优先使用 `outbox_process_ready_active_latency_ms` 和 `outbox_fetched_per_call`，不要只看混合口径。

当前本地 relay 基线候选：

```text
NEXUSIM_OUTBOX_BATCH_SIZE=100
NEXUSIM_OUTBOX_WORKERS=8
```

该基线来自 `docs/runbook/loadtest-report-20260609-outbox-candidate-repeat.md`，只代表当前 Windows 本机 + Docker PostgreSQL/Kafka + PG_MAX_CONNS=64 的本地压测环境。
