# NexusIM SendMessage Load Test Report - 2026-06-09

本文记录 `message-service SendMessage -> PostgreSQL -> outbox -> Kafka` 第一阶段链路的压测方式、结果和瓶颈排查过程。

## 1. 压测目标

本轮只验证已经真实落地的后端链路：

```text
loadtest client
-> message-service gRPC
-> SendMessage use case
-> PostgreSQL local transaction
-> conversation_seq / message_log / conversation_timeline_events / message_outbox
-> outbox relay
-> Kafka topic: conversation.timeline.events
```

不验证 WebSocket、delivery、push、timeline-service 查询、客户端 UI、RAG / Agent。

## 2. 通过标准

本轮按以下门槛判断某个资源档位是否通过：

```text
success_rate >= 0.99
p99 <= 1000ms
outbox_pending_count <= 1000
```

`p99 <= 1000ms` 是体验线，不是写入能力线。某些档位 100% 成功但 p99 超过 1 秒，归类为“能写入但不稳定”。

## 3. 压测拓扑

### 3.1 win-win Docker 单机拓扑

Windows Docker Desktop 内运行完整链路：

```text
sendmessage-loadtest container
-> message-service gRPC container
-> PostgreSQL container
-> message_outbox
-> outbox relay container
-> Kafka container
```

容器通过 Docker network `nexusim-local_default` 通信。`message-service` 和 `outbox relay` 使用 Docker 硬限制：

```text
--cpus
--memory
GOMAXPROCS
NEXUSIM_OUTBOX_WORKERS
NEXUSIM_PG_MAX_CONNS
```

Windows Docker Desktop 已调整为：

```text
processors=16
memory=24GB
swap=8GB
```

`docker info` 显示 Docker VM 可用约：

```text
16 CPU
23.47GiB memory
```

### 3.2 Windows + Mac 分布式客户端拓扑

Windows 承载服务端，Windows 和 Mac 同时作为压测客户端：

```text
Windows loadtest client \
                         -> Windows message-service gRPC -> PostgreSQL -> outbox -> Kafka
Mac loadtest client     /
```

Windows 暴露端口：

```text
10495  message-service gRPC
10497  message-service /debug/metrics
10500  outbox relay /debug/metrics
5432   PostgreSQL
9092   Kafka
```

本轮历史结果执行时，Mac 通过 Wi-Fi 局域网访问 Windows `192.168.0.141`。后续 Win-Mac 压测统一使用有线直连网段：Windows `172.31.50.1`，Mac `172.31.50.2`。

## 4. 执行方式

### 4.1 Docker 资源矩阵

主要脚本：

```powershell
loadtest/sendmessage/run-docker-resource-matrix.ps1
```

示例命令：

```powershell
. .\tools\go-env.ps1
.\loadtest\sendmessage\run-docker-resource-matrix.ps1 `
  -CpuLimits 16 `
  -MemoryLimits 23g `
  -VUSteps 1600,2400 `
  -OutboxWorkers 8 `
  -PGMaxConns 64 `
  -PGMinConns 16 `
  -Duration 20s `
  -StatsWait 20s `
  -ConversationCount 3000 `
  -MaxP99MS 1000 `
  -MaxPending 1000
```

脚本行为：

```text
1. 编译 linux/amd64 message-service 和 sendmessage-loadtest
2. 构建 runtime Docker image
3. 启动 gRPC 容器
4. 启动 outbox relay 容器
5. 启动 loadtest 容器发真实 SendMessage gRPC 请求
6. 等待 stats_wait 后读取 PostgreSQL outbox 状态和 /debug/metrics
7. 写出 sendmessage-summary.json 和矩阵 summary
```

### 4.2 分布式双客户端

Windows 服务端：

```text
nexusim-dist-win-grpc
nexusim-dist-win-relay
nexusim-postgres
nexusim-kafka
```

Mac 侧使用：

```text
~/Desktop/IM/bin/darwin-arm64/sendmessage-loadtest
```

典型配置：

```text
Windows client: 600 VU / 20s
Mac client:     600 VU / 20s
```

以及：

```text
Windows client: 1000 VU / 20s
Mac client:     1000 VU / 20s
```

## 5. 结果摘要

### 5.1 win-win 单机 Docker 资源矩阵

已观察到的最佳通过档：

```text
16 CPU / 23g memory / relay workers=8 / 1200 VU
requests ~= 49,865
rps ~= 2,493
success_rate = 1.0
p99 = 736.28ms
outbox_pending_count = 0
```

`16 CPU / 23g / relay workers=8 / 1600 VU` 曾出现波动：

```text
run A: p99 = 1120.48ms, failed by p99
run B: p99 = 854.50ms, passed
run C with PG max conns 64: p99 = 779.63ms, passed
```

这说明当前系统已经进入资源争用区，单次压测不能直接当作稳定结论，需要重复跑和取趋势。

`16 CPU / 23g / relay workers=16 / 1200 VU`：

```text
p99 = 1477.19ms
```

说明盲目提高 relay worker 会放大数据库争用，并不会自动提高 SendMessage p99。

### 5.2 Windows + Mac 分布式客户端

`600 + 600 VU`：

```text
Windows client: requests=23358, success=1.0, p99=730.11ms, pending=0
Mac client:     requests=22025, success=1.0, p99=739.50ms, pending=0
```

`1000 + 1000 VU`：

```text
Windows client: requests=22761, success=1.0, p99=1331.04ms, pending=0
Mac client:     requests=21439, success=1.0, p99=1331.29ms, pending=0
```

结论：

```text
总 1200 VU 左右可以视为当前较稳分布式客户端档位。
总 2000 VU 可以成功写入，但 p99 已明显超过 1 秒体验线。
```

## 6. 瓶颈排查过程

这一节只记录排查方法和证据链，不直接跳到结论。

### 6.1 先定义可能瓶颈

当 `p99 > 1000ms` 时，请求路径上可能慢的地方有：

```text
1. loadtest client 本身发不动
2. 局域网 / Docker network 慢
3. gRPC handler / app 编排慢
4. PostgreSQL connection pool 获取连接慢
5. conversation_seq 行锁慢
6. message_log / timeline / outbox 插入慢
7. transaction commit 慢
8. outbox relay 发布 Kafka 慢
9. Kafka broker 慢
```

排查策略是沿着真实请求路径逐段打点，然后比较：

```text
请求总 p99
SendMessage handler p99
repository append p99
repository commit p99
conversation_seq p99
Kafka publish p99
outbox pending
pgxpool acquire 统计
```

如果某个内部指标的 p99 接近请求总 p99，它就是主要瓶颈所在的区间；如果某个指标只有几毫秒，就可以排除它是主要瓶颈。

### 6.2 第一轮：用已有指标排除 outbox / Kafka / seq

原始压测 summary 已经有：

```text
request p99
outbox_pending_count
conversation_seq_alloc_latency_ms
kafka_publish_latency_ms
```

高并发时观察到：

```text
request p99 = 700ms - 1300ms
outbox_pending_count = 0
conversation_seq_alloc p99 = 几毫秒
kafka_publish p99 = 几毫秒
```

由此先排除三类瓶颈：

```text
outbox relay 追不上：排除，因为 pending = 0
Kafka publish 慢：排除，因为 kafka_publish p99 是毫秒级
conversation_seq 单步更新慢：基本排除，因为 seq alloc p99 是毫秒级
```

但这一步还不能定位剩余延迟在哪里，因为 request p99 仍可能来自：

```text
gRPC handler
SendMessage app 编排
repository 整体事务
pgxpool 等连接
insert message/timeline/outbox
commit
```

### 6.3 第二轮：补分段指标

为继续定位，本轮新增了服务端 `/debug/metrics` 分段指标：

```text
send_message_latency_ms
repository_append_latency_ms
repository_commit_latency_ms
conversation_seq_alloc_latency_ms
kafka_publish_latency_ms
pg_pool
```

并让 `loadtest` summary 自动记录：

```text
send_message_latency_ms / p95 / p99
repository_append_latency_ms / p95 / p99
repository_commit_latency_ms / p95 / p99
conversation_seq_alloc p99
kafka_publish p99
service_pg_pool
relay_pg_pool
```

这些指标的含义：

```text
send_message_latency_ms
  gRPC handler 收到请求到返回响应的总服务端耗时。

repository_append_latency_ms
  MessageRepository.AppendMessage 的总耗时，包括获取连接、begin、锁、查询、insert、commit。

repository_commit_latency_ms
  PostgreSQL commit 耗时。

conversation_seq_alloc_latency_ms
  确保 conversation_seq 行存在并递增 current_seq 的耗时。

kafka_publish_latency_ms
  relay 向 Kafka writer publish 的耗时。

service_pg_pool
  gRPC 服务进程的 pgxpool 状态，重点看 max_conns、acquire_duration、empty_acquire_count。

relay_pg_pool
  relay 进程的 pgxpool 状态，用于判断 relay 是否在抢 DB 连接。
```

### 6.4 第三轮：比较请求 p99 和内部 p99

诊断组：

```text
16 CPU / 23g / 1600 VU / relay workers=8 / default pgxpool max_conns=16
```

结果：

```text
request p99             = 854.50ms
send_message p99        = 852.78ms
repository_append p99   = 852.76ms
repository_commit p99   = 8.10ms
conversation_seq p99    = 1.91ms
kafka_publish p99       = 1.86ms
outbox_pending_count    = 0
```

对比方法：

```text
request p99 ~= send_message p99
send_message p99 ~= repository_append p99
repository_commit p99 远小于 request p99
conversation_seq p99 远小于 request p99
kafka_publish p99 远小于 request p99
```

因此可以进一步缩小范围：

```text
瓶颈不在 Kafka publish。
瓶颈不在 outbox relay 追平。
瓶颈不在 transaction commit。
瓶颈不在 conversation_seq 单步递增。
瓶颈在 repository_append 内部，但不在已单独打点的 seq / commit。
```

`repository_append` 内剩余未排除的主要段落是：

```text
pgxpool 获取连接 / Begin
advisory lock
find existing message
insert message_log
insert conversation_timeline_events
insert message_outbox
```

### 6.5 第四轮：检查 pgxpool 统计

同一组诊断的 `service_pg_pool`：

```text
service_pg_pool.max_conns            = 16
service_pg_pool.total_conns          = 16
service_pg_pool.acquire_count        = 49213
service_pg_pool.acquire_duration_ms  = 31811629
service_pg_pool.empty_acquire_count  = 49213
```

计算：

```text
平均 acquire 等待 = 31811629 / 49213 ~= 646.41ms
```

这一步是关键证据：

```text
request p99             = 854.50ms
repository_append p99   = 852.76ms
平均 acquire 等待       = 646.41ms
```

并且：

```text
empty_acquire_count = acquire_count
```

说明几乎每次 repository append 获取连接时，连接池都没有空闲连接，需要排队等待。

relay 侧对比：

```text
relay_pg_pool.max_conns           = 16
relay acquire avg                 ~= 0.02ms
outbox_pending_count              = 0
```

这说明 relay 自己没有明显 DB 连接等待，也没有积压 outbox。

因此当前最强证据指向：

```text
message-service gRPC 进程的 PostgreSQL connection pool 等待。
```

### 6.6 第五轮：调大连接池做反证

为了确认不是误判，本轮新增配置：

```text
NEXUSIM_PG_MAX_CONNS
NEXUSIM_PG_MIN_CONNS
```

并做对比：

```text
16 CPU / 23g / 1600 VU / relay workers=8 / PG max conns=64
```

结果：

```text
requests               = 64635
request p99            = 779.63ms
repository_append p99  = 777.71ms
repository_commit p99  = 27.37ms
conversation_seq p99   = 12.79ms
kafka_publish p99      = 8.03ms
service_pg_pool.max    = 64
service acquire avg    = 472.62ms
outbox pending         = 0
```

和默认连接池对比：

```text
requests               49213 -> 64635
request p99            854.50ms -> 779.63ms
service acquire avg    646.41ms -> 472.62ms
```

这说明调大连接池确实能缓解 p99 和提升吞吐，反过来证明 pgxpool 等连接是当前主要瓶颈之一。

但它没有完全解决问题。继续压：

```text
16 CPU / 23g / 2400 VU / relay workers=8 / PG max conns=64
```

结果：

```text
request p99            = 1452.87ms
repository_append p99  = 1450.44ms
service acquire avg    = 776.75ms
outbox pending         = 0
```

说明在更高 VU 下，连接池排队仍然回来，并且 request p99 仍然贴着 repository append p99。

### 6.7 排查结论如何得出

最终不是凭感觉判断，而是按以下证据链得出：

```text
1. outbox pending = 0
   -> 排除 outbox relay 追不上。

2. kafka_publish p99 < 10ms
   -> 排除 Kafka publish 是主瓶颈。

3. conversation_seq p99 多数为几毫秒到十几毫秒
   -> 排除 conversation_seq 单步递增是主瓶颈。

4. repository_commit p99 约 8ms - 32ms
   -> 排除 commit 是主瓶颈。

5. request p99 ~= send_message p99 ~= repository_append p99
   -> 瓶颈收敛到 repository append 内部。

6. service pgxpool acquire 平均等待 472ms - 776ms，且 empty_acquire_count 接近 acquire_count
   -> repository append 内部主要等待发生在获取 PostgreSQL 连接。

7. 调大 PG max conns 后吞吐提高、p99 降低
   -> 通过反证确认连接池等待是当前主要瓶颈之一。
```

因此当前瓶颈不是“CPU 不够”或“Kafka 慢”的单点问题，而是：

```text
高并发 SendMessage 下，单个 message-service 实例的 PostgreSQL connection pool 和数据库写入并发能力先成为瓶颈。
```

后续还需要进一步细分：

```text
pool begin/acquire
advisory lock
find existing
insert message_log
insert timeline
insert outbox
```

但本轮已经足够确定：下一轮优先做连接池梯度、多服务实例、PostgreSQL 写入路径优化，而不是继续盲目加 Docker CPU/内存。

## 7. 当前结论

当前链路的数据一致性和 outbox/Kafka 追平能力是成立的：

```text
success_rate = 1.0
outbox_pending_count = 0
DLQ = 0
Kafka publish p99 < 10ms
```

当前主要性能瓶颈不是 CPU 总量，也不是内存容量，而是：

```text
高 VU 下 message-service 到 PostgreSQL 的连接池等待
```

表现为：

```text
repository_append p99 几乎等于 request p99
pgxpool acquire wait 占据绝大部分延迟
```

提高 `NEXUSIM_PG_MAX_CONNS` 可以提升吞吐并降低 p99，但 2400 VU 仍会超线。下一步不能只继续加 CPU/内存，需要做连接池、PostgreSQL 写入路径、服务实例数和数据库写入模型的组合优化。

## 8. 下一步优化建议

优先级从高到低：

1. 将 `NEXUSIM_PG_MAX_CONNS` 纳入正式本地/压测配置，按 `32 / 64 / 96` 梯度测试。
2. 做多 `message-service` 实例压测，而不是单实例无限加连接池。
3. 在 repository 内继续细分打点：`pool begin/acquire`、`advisory lock`、`find existing`、`insert message`、`insert timeline`、`insert outbox`。
4. 评估 PostgreSQL `max_connections`、WAL、checkpoint、fsync、shared_buffers、commit 延迟。
5. 压测不同 `conversation_count`，区分单会话顺序锁瓶颈和跨会话写入吞吐瓶颈。
6. 不建议继续盲目提高 relay workers；当前 outbox 能追平，过高 worker 会增加 DB 争用。

## 9. 产物位置

压测结果：

```text
loadtest/results/
```

趋势图：

```text
loadtest/results/charts/winwin-rps-trend.png
loadtest/results/charts/winwin-p99-trend.png
loadtest/results/charts/distributed-clients-trend.png
```

摘要：

```text
loadtest/results/charts/winwin-distributed-summary.md
```

本报告：

```text
docs/runbook/loadtest/message-service/loadtest-report-20260609.md
```

## 10. 后续归档策略

后续每个阶段都新增一份独立压测报告，不覆盖旧报告。推荐命名：

```text
docs/runbook/loadtest/<service>/loadtest-report-YYYYMMDD-<stage>.md
```

报告进入 Git，用于保存阶段结论；`loadtest/results/` 继续保存所有中间 summary、趋势图和临时结果，默认不提交。阶段报告必须引用关键 `loadtest/results/` 路径，确保本地仍可追溯原始数据。
