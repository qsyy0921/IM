# delivery-service Full Smoke

## 目标

验证 `delivery-service` 的第一条真实链路：

```text
CreateMemberChange(JOIN user-0 / delivery-user-1)
-> message-service outbox relay
-> Kafka conversation.timeline.events
-> delivery-service timeline-consumer
-> SendMessage(user-0)
-> message-service outbox relay
-> Kafka conversation.timeline.events
-> delivery-service timeline-consumer
-> user_inbox(delivery-user-1)
-> PullInbox
-> AckDelivery
```

这是小规模 smoke，不是容量压测。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `ef817f7` |
| git_dirty | `false` |
| PostgreSQL | Docker `nexusim-postgres`，`localhost:5432` |
| Kafka | Docker `nexusim-kafka`，topic `conversation.timeline.events` |
| conversation-service gRPC | `127.0.0.1:13496` |
| message-service gRPC | `127.0.0.1:13495` |
| message-service outbox relay | 本机进程，workers `2`，batch size `100` |
| delivery-service timeline-consumer | consumer group `nexusim-delivery-smoke-20260609132344` |
| delivery-service gRPC | `127.0.0.1:13497` |
| tenant | `tenant-delivery-smoke` |
| conversation | `conv-delivery-smoke-0` |
| sender | `user-0` |
| receiver | `delivery-user-1` |

原始结果目录：

```text
loadtest/results/delivery-fullsmoke-clean-20260609-132344/
```

## 启动拓扑

```powershell
$env:NEXUSIM_CONVERSATION_SERVICE_MODE='grpc'
$env:NEXUSIM_CONVERSATION_GRPC_ADDR='127.0.0.1:13496'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
bin\conversation-service.exe
```

```powershell
$env:NEXUSIM_MESSAGE_SERVICE_MODE='outbox-relay'
$env:NEXUSIM_KAFKA_BROKERS='localhost:9092'
$env:NEXUSIM_KAFKA_TOPIC='conversation.timeline.events'
$env:NEXUSIM_OUTBOX_BATCH_SIZE='100'
$env:NEXUSIM_OUTBOX_WORKERS='2'
$env:NEXUSIM_OUTBOX_POLL_INTERVAL='100ms'
bin\message-service.exe
```

```powershell
$env:NEXUSIM_DELIVERY_SERVICE_MODE='timeline-consumer'
$env:NEXUSIM_KAFKA_BROKERS='localhost:9092'
$env:NEXUSIM_TIMELINE_TOPIC='conversation.timeline.events'
$env:NEXUSIM_DELIVERY_CONSUMER_GROUP='nexusim-delivery-smoke-20260609132344'
bin\delivery-service.exe
```

```powershell
$env:NEXUSIM_DELIVERY_SERVICE_MODE='grpc'
$env:NEXUSIM_DELIVERY_GRPC_ADDR='127.0.0.1:13497'
bin\delivery-service.exe
```

```powershell
$env:NEXUSIM_MESSAGE_SERVICE_MODE='grpc'
$env:NEXUSIM_GRPC_ADDR='127.0.0.1:13495'
$env:NEXUSIM_CONVERSATION_SERVICE_ADDR='127.0.0.1:13496'
$env:NEXUSIM_MOCK_PERMISSION_VERSION='3'
bin\message-service.exe
```

本轮启动日志：

```text
loadtest/results/delivery-fullsmoke-clean-20260609-132344/logs/
```

## 执行命令

先把 smoke consumer group reset 到 latest，避免读取历史压测事件：

```powershell
docker exec nexusim-kafka kafka-consumer-groups `
  --bootstrap-server localhost:9092 `
  --group nexusim-delivery-smoke-20260609132344 `
  --topic conversation.timeline.events `
  --reset-offsets --to-latest --execute
```

添加发送者和接收者：

```powershell
bin\memberchange-loadtest.exe `
  --target 127.0.0.1:13496 `
  --vus 1 `
  --duration 5s `
  --request-count 1 `
  --tenant-id tenant-delivery-smoke `
  --conversation-id conv-delivery-smoke-0 `
  --operator-user-id owner-1 `
  --target-user-id user-0 `
  --idempotency-prefix join-user0 `
  --pg-dsn 'postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable' `
  --result-dir loadtest\results\delivery-fullsmoke-clean-20260609-132344\member-user0
```

```powershell
bin\memberchange-loadtest.exe `
  --target 127.0.0.1:13496 `
  --vus 1 `
  --duration 5s `
  --request-count 1 `
  --tenant-id tenant-delivery-smoke `
  --conversation-id conv-delivery-smoke-0 `
  --operator-user-id owner-1 `
  --target-user-id delivery-user-1 `
  --idempotency-prefix join-delivery-user1 `
  --pg-dsn 'postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable' `
  --result-dir loadtest\results\delivery-fullsmoke-clean-20260609-132344\member-delivery-user1
```

发送消息：

```powershell
bin\sendmessage-loadtest.exe `
  --target 127.0.0.1:13495 `
  --vus 1 `
  --duration 500ms `
  --request-timeout 2s `
  --stats-wait 3s `
  --tenant-id tenant-delivery-smoke `
  --conversation-prefix conv-delivery-smoke `
  --conversation-count 1 `
  --pg-dsn 'postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable' `
  --result-dir loadtest\results\delivery-fullsmoke-clean-20260609-132344\sendmessage
```

拉取 inbox 并 ACK：

```powershell
bin\delivery-loadtest.exe `
  --target 127.0.0.1:13497 `
  --wait-timeout 15s `
  --poll-interval 200ms `
  --tenant-id tenant-delivery-smoke `
  --conversation-id conv-delivery-smoke-0 `
  --user-id delivery-user-1 `
  --device-id delivery-device-1 `
  --after-seq 0 `
  --limit 100 `
  --expected-count 64 `
  --ack=true `
  --consumer-group nexusim-delivery-smoke-20260609132344 `
  --pg-dsn 'postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable' `
  --result-dir loadtest\results\delivery-fullsmoke-clean-20260609-132344\delivery
```

## 核心结果

| 指标 | 值 |
| --- | ---: |
| member join request | `2/2` success |
| SendMessage request | `64/64` success |
| SendMessage p95 / p99 | `9.24ms / 59.95ms` |
| PullInbox item_count | `64` |
| PullInbox poll_count | `1` |
| PullInbox p99 | `40.12ms` |
| AckDelivery latency | `6.39ms` |
| Ack cursor | `66` |
| message_outbox | `PUBLISHED=66` |
| user_inbox delivery-user-1 | `64` rows, seq `3..66` |
| user_inbox user-0 | `64` rows, seq `3..66` |
| delivery_outbox | `PENDING=129`, `DLQ=0` |

最终 SQL 复核：

```text
conversation_seq|66
message_outbox|PUBLISHED|66
inbox|delivery-user-1|64|3|66
inbox|user-0|64|3|66
cursor|delivery-user-1|delivery-device-1|66
delivery_outbox|PENDING|delivery.ack.recorded.v1|1
delivery_outbox|PENDING|delivery.inbox_item.created.v1|128
checkpoint|nexusim-delivery-smoke-20260609132344|1|3600811
```

## 如何排查瓶颈和断点

本轮重点是链路断点排查，不是硬件瓶颈。

1. 先检查成员事件是否发布：`message_outbox PUBLISHED=2`，说明 `CreateMemberChange -> outbox relay -> Kafka` 正常。
2. 再检查 delivery 成员投影：`delivery_membership_projection` 中 `user-0` 和 `delivery-user-1` 均为 `ACTIVE`，说明 timeline consumer 已消费 member events。
3. 再检查消息写入：`SendMessage 64/64 success`，说明 message-service 能通过 conversation-service 读取真实发送上下文。
4. 再检查消息事件发布：`message_outbox PUBLISHED=66`，说明 2 条成员事件和 64 条消息事件都已发布。
5. 再检查 fanout 投影：两个 ACTIVE 用户各有 64 条 `user_inbox`，seq 范围为 `3..66`，说明成员边界 seq 之后的消息被正确投递。
6. 最后检查 ACK：`device_delivery_cursors.last_received_seq=66`，说明客户端 ACK 没有越界，cursor 前进到实际可见最大 seq。

本轮修过一个 smoke 编排问题：`sendmessage` 的 `conversation-prefix` 会拼出 `conv-delivery-smoke-0`，所以 seed、member change、PullInbox 必须使用同一个完整 conversation id。

## 当前结论

- `delivery-service` 已经不是单纯骨架：它能消费统一 timeline，维护成员可见性投影，按消息事件写 `user_inbox`，并支持客户端 `PullInbox / AckDelivery`。
- `AckDelivery` 的 max visible seq 约束在真实链路中生效，ACK 到 `66` 后 cursor 正确落库。
- 本轮报告生成时 `delivery_outbox` 只写本地 PENDING 事件，尚未实现 delivery outbox relay / `im.delivery.events` 发布；后续已补齐 `delivery_outbox -> im.delivery.events`，并完成 push-gateway 单实例在线通知 smoke。
- 本轮只覆盖 `JOIN + TEXT SendMessage + Pull/Ack`，尚未覆盖 LEAVE/REMOVE/ROLE_CHANGED 对历史可见窗口的影响，也未覆盖 projection DLQ/repair。

## 下一步

1. 将 `delivery-service` smoke 结果纳入 `current-goal.md`。
2. 后续补 `delivery_outbox` relay 或先写 `push-gateway` SDD，明确 online push 依赖 durable inbox。
3. 在 push 前补小规模 `LEAVE/REMOVE` 可见性 smoke，验证离开后不再写入新的 inbox。
