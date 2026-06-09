# conversation-service CreateMemberChange Smoke

## 目标

验证 `conversation-service` 的最小成员变更写路径已经能通过真实进程跑通，并把 member boundary event 交给统一 outbox relay 发布。

本报告不是容量压测报告，只证明这条链路可运行：

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
```

## 环境

| 项 | 值 |
| --- | --- |
| commit | `71c04c9` |
| git_dirty | `false` |
| PostgreSQL | `nexusim-postgres` Docker container，`localhost:5432` |
| Kafka | `nexusim-kafka` Docker container，`localhost:9092` |
| conversation-service | 本机进程，`127.0.0.1:12496` |
| outbox relay | `message-service` outbox-relay 模式，本机进程 |
| relay workers | `2` |
| relay batch size | `50` |
| VU | `2` |
| duration | `2s` |
| tenant | `tenant-member-smoke-clean` |
| conversation | `conv-member-smoke-clean` |

## 准备动作

1. 构建本地二进制：

```powershell
. .\tools\go-env.ps1
go build -o bin\conversation-service.exe ./services/conversation-service/cmd/conversation-service
go build -o bin\message-service.exe ./services/message-service/cmd/message-service
go build -o bin\memberchange-loadtest.exe ./loadtest/memberchange
```

2. 应用 conversation migration v3：

```powershell
docker exec nexusim-postgres psql -U nexusim -d nexusim -v ON_ERROR_STOP=1 `
  -c "CREATE UNIQUE INDEX IF NOT EXISTS uq_member_change_saga_outbox_event ON member_change_saga (tenant_id, outbox_event_id) WHERE outbox_event_id IS NOT NULL;" `
  -c "CREATE UNIQUE INDEX IF NOT EXISTS uq_member_change_saga_timeline_event ON member_change_saga (tenant_id, timeline_event_id) WHERE timeline_event_id IS NOT NULL;"
```

3. 清理并 seed 一个 active group conversation 和一个 owner：

```text
tenant_id = tenant-member-smoke-clean
conversation_id = conv-member-smoke-clean
owner_user_id = owner-1
member_version = 5
permission_version = 7
```

4. 启动 `conversation-service`：

```powershell
$env:NEXUSIM_CONVERSATION_SERVICE_MODE='grpc'
$env:NEXUSIM_CONVERSATION_GRPC_ADDR='127.0.0.1:12496'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
bin\conversation-service.exe
```

5. 启动 outbox relay：

```powershell
$env:NEXUSIM_MESSAGE_SERVICE_MODE='outbox-relay'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
$env:NEXUSIM_KAFKA_BROKERS='localhost:9092'
$env:NEXUSIM_KAFKA_TOPIC='conversation.timeline.events'
$env:NEXUSIM_OUTBOX_WORKERS='2'
$env:NEXUSIM_OUTBOX_BATCH_SIZE='50'
$env:NEXUSIM_OUTBOX_POLL_INTERVAL='100ms'
bin\message-service.exe
```

## 压测命令

```powershell
bin\memberchange-loadtest.exe `
  --target 127.0.0.1:12496 `
  --vus 2 `
  --duration 2s `
  --request-timeout 2s `
  --tenant-id tenant-member-smoke-clean `
  --conversation-id conv-member-smoke-clean `
  --operator-user-id owner-1 `
  --target-prefix target-member `
  --expected-member-version 0 `
  --pg-dsn 'postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable' `
  --stats-wait 5s `
  --result-dir loadtest\results\memberchange-smoke-clean-20260609-115544
```

`expected-member-version=0` 表示本轮 smoke 不做固定版本乐观锁检查。原因是本轮用 2 个 VU 连续加入多个不同成员，conversation 的 `member_version` 会持续递增；如果固定传 `5`，第二条开始会按设计触发版本冲突。

## 结果

原始 summary：

```text
loadtest/results/memberchange-smoke-clean-20260609-115544/memberchange-summary.json
```

| 指标 | 值 |
| --- | ---: |
| request_count | 279 |
| success_count | 279 |
| error_count | 0 |
| success_rate | 1.0 |
| rps | 139.5 |
| avg | 14.42ms |
| p95 | 19.84ms |
| p99 | 24.95ms |
| saga_count | 279 |
| timeline_count | 279 |
| outbox_total_count | 279 |
| outbox_pending_count | 0 |
| outbox_published_count | 279 |
| outbox_dlq_count | 0 |
| conversation_seq_current | 279 |

outbox 状态复核：

```text
PUBLISHED|279
```

## 如何排查瓶颈和正确性

本轮不是容量测试，所以没有做 CPU / 内存矩阵。排查重点是“链路哪里可能断”：

1. 先看 gRPC 请求结果：`279/279` 成功，说明 API adapter、app usecase、domain validation 和 repository 主路径没有返回错误。
2. 再看 DB 事实表数量：`saga_count == timeline_count == outbox_total_count == 279`，说明每条成功命令都生成了成员变更 saga、timeline event 和 outbox event。
3. 再看顺序轴：`conversation_seq_current=279`，和成功请求数一致，说明普通会话 boundary seq 没有丢号或重复。
4. 再看 outbox 状态：`PENDING=0 / PUBLISHED=279 / DLQ=0`，说明 outbox relay 已经成功把 member boundary event 写入 Kafka，并完成发布标记。
5. 如果后续出现失败，排查顺序固定为：gRPC error topN -> `member_change_saga.status` -> `conversation_timeline_events` 数量 -> `message_outbox.status` -> outbox relay 日志 -> Kafka broker 状态。

## 结论

- `conversation-service` 已不只是 read path；它已经有最小成员变更写路径。
- 成员变更事实由 `conversation-service` 写入，消息服务不直接修改成员事实。
- 成员边界事件与消息事件共享 `conversation_seq`、`conversation_timeline_events` 和 `message_outbox`，后续 delivery / retrieval / audit 可以消费同一条 `conversation.timeline.events` 流。
- 当前只证明保守权限矩阵下的最小 `JOIN` smoke；`LEAVE / REMOVE / ROLE_CHANGED`、`GetMemberChange`、saga `EVENT_PUBLISHED / DONE` 推进和 DLQ repair 仍需后续实现。
- 第一版只接受 `conflict_policy=REJECT`，`MERGE / COMPENSATE` 是协议预留值，真实语义未实现前不能对外宣称支持。
