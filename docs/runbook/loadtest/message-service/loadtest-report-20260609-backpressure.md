# NexusIM SendMessage Backpressure Smoke Report

日期：2026-06-09

## 1. 压测目标

前几轮压测已经确认：

```text
SendMessage p99 主要贴在 repository_pool_acquire
SQL BEGIN 本身不是主瓶颈
继续调大 PostgreSQL max_connections / PG pool 会放大 WAL、commit 和 outbox 压力
```

本轮目标是验证最小 backpressure 能否工作：

```text
当 PostgreSQL pool 已满时，message-service 快速返回 retryable overload
而不是让请求排队到 2s deadline 后再失败
```

## 2. 实现范围

新增公共错误码：

```text
MESSAGE_ERROR_CODE_SERVICE_OVERLOADED = 10
```

新增错误 sentinel：

```text
types.ErrServiceOverloaded
```

gRPC 映射：

```text
gRPC code: Unavailable
MessageError.code: SERVICE_OVERLOADED
retryable: true
public message: service overloaded
```

repository 默认不启用 backpressure。启用环境变量：

```text
NEXUSIM_PG_BACKPRESSURE_ENABLED=true
NEXUSIM_PG_BACKPRESSURE_MIN_AVAILABLE_CONNS=0
```

当前策略：

```text
在 acquire 前读取 pgxpool.Stat()
如果 MaxConns - AcquiredConns <= MinAvailableConns
则直接返回 ErrServiceOverloaded
```

## 3. 验证方式

真实 PostgreSQL 集成测试：

```powershell
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
go test ./services/message-service/internal/infrastructure/postgres `
  -run TestMessageRepositoryBackpressureRejectsWhenPoolSaturated `
  -count=1 -v
```

测试做法：

```text
pgxpool MaxConns=1
测试先手动 acquire 唯一连接
repository 启用 backpressure
AppendMessage 应返回 ErrServiceOverloaded
```

clean commit smoke：

```powershell
$env:NEXUSIM_PG_BACKPRESSURE_ENABLED='true'
$env:NEXUSIM_PG_BACKPRESSURE_MIN_AVAILABLE_CONNS='0'

.\loadtest\sendmessage\run-local-pgpool-gradient.ps1 `
  -PGMaxConns 1 `
  -VUs 20 `
  -Duration 5s `
  -StatsWait 5s `
  -ConversationCount 100 `
  -RelayWorkers 2 `
  -BatchSize 100 `
  -ResultRoot loadtest\results\backpressure-clean-smoke-20260609
```

## 4. Smoke 结果

结果文件：

```text
loadtest/results/backpressure-clean-smoke-20260609/pgmax-1-vu-20-20260609-033732/sendmessage-summary.json
```

核心结果：

| 项 | 值 |
| --- | ---: |
| commit | `78e8375` |
| git_dirty | `false` |
| request_count | 163055 |
| success_rate | 0.0032 |
| p99 ms | 1.6246 |
| outbox_pending_count | 0 |
| top_error | `Unavailable: service overloaded` |
| top_error_count | 162526 |

## 5. 判断

这个 smoke 不追求成功率。它故意设置 `PG_MAX_CONNS=1`，目标是验证系统保护行为。

本轮证明：

```text
1. overload 不再被伪装为 DB_WRITE_FAILED。
2. 客户端能看到 retryable Unavailable。
3. 请求不会在连接池中排队到 2s，p99 保持毫秒级。
4. 被拒绝请求不会写入 message_log / timeline / outbox，因此 outbox pending 为 0。
```

## 6. 风险和下一步

当前策略非常保守，只看瞬时 pgxpool stats。它适合证明保护链路，但还不是最终生产策略。

下一步需要：

```text
为 loadtest 增加 SERVICE_OVERLOADED 专门计数。
在正式 VU 梯度下比较 backpressure on/off：成功率、p99、outbox pending、overload rate。
考虑基于滑动窗口的 pool acquire p95/p99，而不是只看瞬时 acquired conns。
决定 overload 返回策略：gRPC retry hint、客户端退避、是否进入 adaptive limit。
```

后续正式 backpressure 容量压测需要新建独立报告，不覆盖本文。

## 7. Loadtest 错误计数补充

后续又补充了 loadtest summary 的 MessageError 统计字段：

```text
retryable_error_count
service_overloaded_count
message_error_counts[]
```

clean smoke：

```text
commit=a9fbdf8
git_dirty=false
PG_MAX_CONNS=1
VU=10
duration=3s
request_count=62884
success_rate=0.0052
error_count=62556
retryable_error_count=62556
service_overloaded_count=62556
p99=1.3024ms
outbox_pending_count=0
message_error_counts[0].code=MESSAGE_ERROR_CODE_SERVICE_OVERLOADED
message_error_counts[0].count=62556
```

这让后续正式 backpressure 矩阵可以直接计算 overload rate，不再只能从 `error_topn` 字符串里人工判断。
