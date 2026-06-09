# receipt-service Full Smoke

## 目标

验证 `receipt-service` 第一条真实进程链路：

```text
CreateMemberChange(JOIN receiver)
-> SendMessage
-> message-service outbox relay
-> Kafka conversation timeline
-> delivery-service timeline consumer
-> delivery-service outbox relay
-> Kafka im.delivery.events
-> receipt-service delivery-consumer
-> GetReceiptState(received)
-> MarkRead
-> GetReceiptState(read)
```

这是小规模 smoke，不是容量压测。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `3c28105` |
| git_dirty | `false` |
| PostgreSQL | Docker `nexusim-postgres`，`localhost:5432` |
| Kafka | Docker `nexusim-kafka`，timeline topic `conversation.timeline.receipt.20260610-003630`，delivery topic `im.delivery.events` |
| conversation-service gRPC | `127.0.0.1:11696` |
| message-service gRPC | `127.0.0.1:11695` |
| delivery-service gRPC | `127.0.0.1:11697` |
| receipt-service gRPC | `127.0.0.1:11699` |
| delivery consumer group | `nexusim-delivery-receipt-smoke-20260610003630` |
| receipt consumer group | `nexusim-receipt-smoke-20260610003630` |
| tenant | `tenant-receipt-smoke` |
| conversation | `conv-receipt-smoke` |
| sender | `owner-1` |
| receiver | `receipt-user-1` / `receipt-device-1` |

原始结果目录：

```text
H:\NexusIM\loadtest-results\receipt-service-smoke-20260610-003630
```

## 执行命令

```powershell
.\loadtest\receipt\run-local-smoke.ps1
```

脚本会幂等应用 message / conversation / delivery / receipt PostgreSQL migration，创建本轮隔离的 timeline topic，并启动：

```text
conversation-service grpc
message-service outbox-relay
delivery-service timeline-consumer
delivery-service grpc
delivery-service outbox-relay
receipt-service delivery-consumer
receipt-service grpc
message-service grpc
```

## 关键结果

| 指标 | 结果 |
| --- | --- |
| smoke success | `true` |
| member boundary seq | `1` |
| SendMessage seq | `2` |
| message_id | `msg_5c23cf23-9730-47c7-85d4-1b5c69973f1c` |
| PullInbox item_count / max_seq | `1 / 2` |
| AckDelivery last_received_seq | `2` |
| GetReceiptState before read | `received_user_count=1`，`read_user_count=0` |
| MarkRead last_read_seq | `2` |
| GetReceiptState after read by seq | `received_user_count=1`，`read_user_count=1` |
| GetReceiptState after read by message_id | `received_user_count=1`，`read_user_count=1` |
| MarkRead too far | `FailedPrecondition: read out of visible range` |
| receipt_inbox_projection | `count=1`，`min_seq=2`，`max_seq=2` |
| device_received_cursor / user_received_cursor / user_read_cursor | `2 / 2 / 2` |
| receipt_outbox | `total=2`，`PENDING=2`，`PUBLISHED=0`，`DLQ=0` |
| receipt_outbox event types | `receipt.message.received.v1=1`，`receipt.message.read.v1=1` |
| delivery_outbox | `total=2`，`PUBLISHED=2`，`PENDING=0`，`DLQ=0` |

## 判断

本轮证明 `receipt-service` 已能通过真实 Kafka `im.delivery.events` 重建送达状态，并通过显式 `MarkRead` 推进已读状态：

- receiver `AckDelivery` 后，`GetReceiptState` 看到 `received_seq=2`、`read_seq=0`。
- receiver `MarkRead(seq=2)` 后，`GetReceiptState` 看到 `read_seq=2`。
- 同一状态可通过 `conversation_seq` 和 `message_id` 两种入口查询。
- `MarkRead(seq=3)` 被拒绝为 `FailedPrecondition`，证明不能越过可见 / 已送达边界。

## 限制

- 当前 `receipt_outbox` 只落库，不发布 `im.receipt.events`；下一阶段需要实现 receipt outbox relay。
- 当前 receipt gRPC 使用 `StaticAllowAccess`，无关用户权限负向 smoke 不作为本阶段门槛。
- 本轮只覆盖单会话、单 receiver、单消息，不代表容量结论。
- 本轮没有覆盖多设备 read 合并、群聊 count-only 可见性和真实 policy-service。
