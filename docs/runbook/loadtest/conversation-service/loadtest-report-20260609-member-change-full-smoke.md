# conversation-service MemberChange Full Smoke

## 目标

验证成员变更链路已经从“写入 outbox 并发布 Kafka”推进到“saga 本地完成态”：

```text
loadtest/memberchange
-> conversation-service gRPC CreateMemberChange
-> PostgreSQL member_change_saga / conversation_members / conversations
-> conversation_seq
-> conversation_timeline_events
-> message_outbox
-> message-service outbox relay
-> Kafka conversation.timeline.events
-> message_outbox PUBLISHED
-> conversation-service member-change-worker
-> member_change_saga DONE
-> gRPC GetMemberChange returns DONE
```

本报告是小规模 smoke，不是容量压测。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `ca0a0b6` |
| git_dirty | `false` |
| PostgreSQL | `nexusim-postgres` Docker container，`localhost:5432` |
| Kafka | `nexusim-kafka` Docker container，`localhost:9092` |
| conversation-service gRPC | 本机进程，`127.0.0.1:12496` |
| outbox relay | `message-service` outbox-relay 模式，本机进程 |
| saga progress worker | `conversation-service` member-change-worker 模式，本机进程 |
| relay workers | `2` |
| relay batch size | `100` |
| progress worker batch size | `100` |
| VU | `2` |
| duration | `3s` |
| tenant | `tenant-member-fullsmoke` |
| conversation | `conv-member-fullsmoke` |

## 启动拓扑

本轮启动了三个真实进程：

```powershell
$env:NEXUSIM_CONVERSATION_SERVICE_MODE='grpc'
$env:NEXUSIM_CONVERSATION_GRPC_ADDR='127.0.0.1:12496'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
bin\conversation-service.exe
```

```powershell
$env:NEXUSIM_MESSAGE_SERVICE_MODE='outbox-relay'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
$env:NEXUSIM_KAFKA_BROKERS='localhost:9092'
$env:NEXUSIM_KAFKA_TOPIC='conversation.timeline.events'
$env:NEXUSIM_OUTBOX_BATCH_SIZE='100'
$env:NEXUSIM_OUTBOX_WORKERS='2'
bin\message-service.exe
```

```powershell
$env:NEXUSIM_CONVERSATION_SERVICE_MODE='member-change-worker'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
$env:NEXUSIM_MEMBER_CHANGE_PROGRESS_BATCH_SIZE='100'
$env:NEXUSIM_MEMBER_CHANGE_PROGRESS_POLL_INTERVAL='200ms'
bin\conversation-service.exe
```

启动日志保存在：

```text
loadtest/results/memberchange-fullsmoke-20260609-02/logs/
```

## 压测命令

```powershell
bin\memberchange-loadtest.exe `
  --target 127.0.0.1:12496 `
  --vus 2 `
  --duration 3s `
  --request-timeout 2s `
  --stats-wait 5s `
  --tenant-id tenant-member-fullsmoke `
  --conversation-id conv-member-fullsmoke `
  --operator-user-id owner-1 `
  --target-prefix target-fullsmoke `
  --pg-dsn 'postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable' `
  --result-dir loadtest\results\memberchange-fullsmoke-20260609-02
```

## 结果

原始 summary：

```text
loadtest/results/memberchange-fullsmoke-20260609-02/memberchange-summary.json
```

| 指标 | 值 |
| --- | ---: |
| request_count | 350 |
| success_count | 350 |
| error_count | 0 |
| success_rate | 1.0 |
| rps | 116.67 |
| avg | 17.20ms |
| p95 | 21.60ms |
| p99 | 40.90ms |
| saga_count | 350 |
| saga_done_count | 350 |
| timeline_count | 350 |
| outbox_total_count | 350 |
| outbox_pending_count | 0 |
| outbox_published_count | 350 |
| outbox_dlq_count | 0 |
| conversation_seq_current | 350 |
| sample_get_status | `MEMBER_CHANGE_STATUS_DONE` |

PostgreSQL 结束复核：

```text
350|350|350|0|0
```

字段含义：

```text
saga_total | saga_done | outbox_published | outbox_pending | outbox_dlq
```

## 如何排查链路

这次 smoke 的排查目标不是找硬件瓶颈，而是逐段证明链路没有断：

1. `CreateMemberChange` 请求 `350/350` 成功，说明 gRPC adapter、app usecase、domain 权限规则和 PostgreSQL 本地事务主路径可用。
2. `saga_count == timeline_count == outbox_total_count == 350`，说明每个成功请求都写入了 saga、timeline event 和 outbox event。
3. `conversation_seq_current=350`，说明成员边界事件复用了 conversation timeline 顺序轴，没有产生丢号或重复。
4. `outbox_published_count=350` 且 `pending/dlq=0`，说明统一 outbox relay 已经把成员边界事件发布并标记为 `PUBLISHED`。
5. `saga_done_count=350`，说明 `member-change-worker` 观察到了 outbox 发布状态，并把本地 saga 推进到 `DONE`。
6. `sample_get_status=MEMBER_CHANGE_STATUS_DONE`，说明 `GetMemberChange` 的真实 gRPC 查询也能读到完成态，而不是只靠 SQL 手工查询。

如果后续失败，排查顺序固定为：

```text
gRPC error_topn
-> member_change_saga.status
-> conversation_timeline_events count
-> message_outbox.status
-> outbox relay logs
-> member-change-worker logs
-> Kafka broker status
```

## 结论

- `conversation-service` 成员变更链路已经具备最小闭环：写成员事实、写统一 timeline/outbox、由统一 relay 发布 Kafka、再由 conversation-service worker 标记 saga `DONE`。
- `GetMemberChange` 已参与真实进程验证，能够返回 `MEMBER_CHANGE_STATUS_DONE`。
- 当前仍只验证保守权限矩阵下的 `JOIN`；`LEAVE / REMOVE / ROLE_CHANGED`、DLQ repair、ACL projection、delivery/retrieval consumer 还未实现。
- 这一步的价值是把第二个微服务从 read path 推进到“有状态写入 + 异步事件 + 后台 worker”的真实服务形态。
