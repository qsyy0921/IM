# NexusIM SendMessage Client Retry Loadtest Report

日期：2026-06-09

## 1. 压测目标

本轮目标是验证客户端遵守服务端 `SERVICE_OVERLOADED` 的 gRPC `RetryInfo` 后，系统行为是否比“立即继续发新请求”更真实、更稳定。

本轮不是为了证明容量提升，而是回答：

- 客户端按 `RetryInfo=500ms` 等待，并加入 jitter 后，错误是否仍主要是 `SERVICE_OVERLOADED`。
- 用户层 logical request 成功率是否改善。
- 实际 gRPC attempt 数、retry attempt 数和 logical request 数如何变化。
- backpressure + client retry 是否会重新把压力转移到 outbox relay。

## 2. 压测拓扑

```text
loadtest/sendmessage --retry-overloaded
-> gRPC SendMessage
-> SERVICE_OVERLOADED + RetryInfo
-> client waits retry_delay + jitter
-> retry same client_msg_id
-> PostgreSQL transaction / outbox
-> outbox relay
-> Kafka
```

客户端重试复用同一个 `client_msg_id`，符合 SendMessage 幂等要求。

## 3. 环境配置

| 项 | 配置 |
| --- | --- |
| Git commit | `0c542a1` |
| Git dirty | `false` |
| Docker Desktop | 约 16 CPU / 24GB memory |
| PostgreSQL profile | `deploy/local/docker-compose.postgres-loadtest.yml` |
| PostgreSQL `max_connections` | `200` |
| PostgreSQL `shared_buffers` | `1GB` |
| PostgreSQL `max_wal_size` | `4GB` |
| message-service PG pool | `NEXUSIM_PG_MAX_CONNS=64` |
| backpressure | enabled |
| `MinAvailableConns` | `8` |
| client retry | enabled |
| max retries | `2` |
| retry jitter | `100ms` |
| relay workers | `8` |
| relay batch size | `500` |

## 4. 执行命令

```powershell
. .\tools\go-env.ps1
.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 64 `
  -VUs 1200,1600 `
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
  -ResultRoot loadtest\results\backpressure-client-retry-formal-20260609
```

## 5. 结果摘要

| VU | logical requests | logical success rate | gRPC attempts | retry attempts | retried requests | accepted RPS | overload rate | success p99 | error p99 | outbox pending |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1200 | 133,326 | 68.68% | 255,901 | 122,575 | 69,764 | 1526.23 | 64.22% | 421.58ms | 4.10ms | 25,579 |
| 1600 | 138,002 | 56.04% | 301,592 | 163,590 | 90,758 | 1288.98 | 74.36% | 1154.47ms | 7.43ms | 32,091 |

结果目录：

```text
loadtest/results/backpressure-client-retry-formal-20260609/
```

压测结束后额外启动 relay drain，最终 PostgreSQL outbox 状态：

```text
PUBLISHED=5,944,884
PENDING=0
```

## 6. 瓶颈排查过程

### 6.1 先区分 logical request 和 gRPC attempt

开启客户端重试后，`request_count` 不再等于用户层消息数：

```text
VU1200 logical_request_count=133326 request_count=255901
VU1600 logical_request_count=138002 request_count=301592
```

因此后续报告必须同时看：

```text
logical_success_rate
request_count
retry_attempt_count
retried_request_count
```

不能把 retry attempt 误读成新消息吞吐。

### 6.2 再看错误语义是否稳定

本轮错误全部是可解释的 `SERVICE_OVERLOADED`：

```text
VU1200 SERVICE_OVERLOADED=164327
VU1600 SERVICE_OVERLOADED=224253
```

没有出现上一轮 `MinAvailable=0` 中的 `DeadlineExceeded` 和 `DB_WRITE_FAILED`。这说明客户端等待 `RetryInfo` 后，错误语义更稳定，系统没有退化到连接池完全打满后的超时/数据库写失败。

### 6.3 再看用户层成功率

与不重试相比，客户端遵守 retry hint 后，用户层 logical success rate 明显高于“立即继续打新请求”的模型：

```text
VU1200 logical_success_rate=68.68%
VU1600 logical_success_rate=56.04%
```

这更接近真实客户端行为：用户发送一条消息，遇到 overload 后等待再用同一个 `client_msg_id` 重试。

### 6.4 最后看新瓶颈是否转移

本轮 accepted RPS 回升到：

```text
VU1200 accepted_rps=1526.23
VU1600 accepted_rps=1288.98
```

同时 outbox pending 重新出现：

```text
VU1200 outbox_pending=25579
VU1600 outbox_pending=32091
```

这说明客户端退避让服务接受了更多成功写入，但 relay 追平能力再次成为后续瓶颈。backpressure 保护了数据库错误语义，但并没有解决 outbox relay 吞吐。

### 6.5 指标口径注意

开启 backpressure 后，`repository_append_latency_ms` 会同时包含：

- 很快返回的 overload 拒绝。
- 成功进入 PostgreSQL 事务的写入。

因此它会被快速拒绝路径拉低，不能单独代表成功写入成本。成功路径仍需结合：

```text
success_p99_ms
repository_pool_acquire_latency_ms
repository_commit_latency_ms
outbox_pending_count
```

本轮成功路径的关键指标：

| VU | success p99 | pool acquire p99 | commit p99 | Kafka publish p99 | outbox pending |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 1200 | 421.58ms | 353.48ms | 27.92ms | 22.78ms | 25,579 |
| 1600 | 1154.47ms | 1000.12ms | 40.98ms | 26.30ms | 32,091 |

## 7. 当前结论

1. 客户端遵守 `RetryInfo` 后，错误语义稳定，没有退化为 DeadlineExceeded / DB_WRITE_FAILED。
2. 用户层 logical success rate 明显高于“立即继续打新请求”的模型。
3. accepted RPS 上升后，outbox pending 重新积压，说明 relay 追平能力成为下一阶段重点。
4. `RetryInfo=500ms + max_retries=2 + jitter=100ms` 是第一版实验参数，不是最优客户端策略。
5. 后续容量结论必须同时报告 logical request、gRPC attempt、retry attempt、success p99、error p99 和 outbox pending。

## 8. 下一步

- 做 outbox relay 批量优化：批量 publish、批量 mark published，降低每条事件的 DB/Kafka 往返。
- 在客户端 retry 矩阵中增加 `max_retries=1/2/3` 和 `jitter=100/300/500ms` 对照。
- 服务端 adaptive limit 需要把 outbox pending 纳入输入，否则只保护 PG pool 会把瓶颈转移到 relay。
- 后续报告增加 actual elapsed seconds，避免 retry/wait 模型下只用配置 duration 计算 RPS。

