# receipt-service Outbox Relay Smoke

## 目标

验证 `receipt-service` 在已完成送达 / 已读 read model 之后，可以把本服务事实通过 outbox relay 发布到 Kafka：

```text
im.delivery.events
-> receipt-service delivery-consumer
-> receipt_inbox_projection / message_receipt_states
-> MarkRead
-> receipt_outbox
-> receipt-service outbox-relay
-> Kafka im.receipt.events
```

这是小规模 smoke，不是容量压测。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `abfe0ec` |
| git_dirty | `false` |
| PostgreSQL | Docker `nexusim-postgres`，`localhost:5432` |
| Kafka | Docker `nexusim-kafka` |
| timeline topic | `conversation.timeline.receipt.20260610-005435` |
| delivery topic | `im.delivery.events` |
| receipt topic | `im.receipt.events` |
| conversation-service gRPC | `127.0.0.1:11696` |
| message-service gRPC | `127.0.0.1:11695` |
| delivery-service gRPC | `127.0.0.1:11697` |
| receipt-service gRPC | `127.0.0.1:11699` |
| delivery consumer group | `nexusim-delivery-receipt-smoke-20260610005435` |
| receipt delivery consumer group | `nexusim-receipt-smoke-20260610005435` |
| receipt event readback group | `nexusim-receipt-events-smoke-20260610005435` |

原始结果目录：

```text
H:\NexusIM\loadtest-results\receipt-service-outbox-smoke-20260610-005435
```

## 执行命令

```powershell
.\loadtest\receipt\run-local-smoke.ps1 -RunName receipt-service-outbox-smoke-20260610-005435
```

脚本会启动：

```text
conversation-service grpc
message-service grpc / outbox-relay
delivery-service grpc / timeline-consumer / outbox-relay
receipt-service grpc / delivery-consumer / outbox-relay
```

并创建本轮隔离 timeline topic，同时确保 `im.delivery.events` 和 `im.receipt.events` 存在。

## 关键结果

| 指标 | 结果 |
| --- | --- |
| smoke success | `true` |
| member boundary seq | `1` |
| SendMessage seq | `2` |
| message_id | `msg_1e674306-f668-461d-ab8c-dc1778504ccb` |
| PullInbox item_count / max_seq | `1 / 2` |
| AckDelivery last_received_seq | `2` |
| GetReceiptState before read | `received_user_count=1`，`read_user_count=0` |
| MarkRead last_read_seq | `2` |
| GetReceiptState after read | `received_user_count=1`，`read_user_count=1` |
| MarkRead too far | `FailedPrecondition: read out of visible range` |
| receipt_outbox | `total=2`，`PUBLISHED=2`，`PENDING=0`，`DLQ=0` |
| receipt_outbox event types | `receipt.message.received.v1=1`，`receipt.message.read.v1=1` |
| Kafka receipt events readback | `2` |
| delivery_outbox | `total=2`，`PUBLISHED=2`，`PENDING=0`，`DLQ=0` |

Kafka `im.receipt.events` 读回：

| event_type | payload | offset | aggregate_version | cursor_seq | user / device |
| --- | --- | ---: | ---: | ---: | --- |
| `receipt.message.received.v1` | `message_received` | `0` | `2` | `2` | `receipt-user-1 / receipt-device-1` |
| `receipt.message.read.v1` | `message_read` | `1` | `2` | `2` | `receipt-user-1 / receipt-device-1` |

两条 Kafka event 的 `message_id` 均为 `msg_1e674306-f668-461d-ab8c-dc1778504ccb`，`partition_key` 均为 `tenant-receipt-smoke:conv-receipt-smoke`。

## 判断

本轮证明 `receipt-service` 已具备完整的最小回执事件发布链路：

- `AckDelivery` 仍由 `delivery-service` 负责，receipt-service 只消费 `im.delivery.events` 重建回执 read model。
- `MarkRead` 受可见 / 已送达边界约束，不能越界把未投递消息标已读。
- `receipt_outbox` 通过独立 relay 发布 `receipt.message.received.v1` 和 `receipt.message.read.v1` 到 `im.receipt.events`。
- 回执 Kafka payload 包含 `message_id`、`source_event_id`、`cursor_seq`、`user_id`、`device_id`，后续可供 audit、通知、会话摘要或客户端同步消费。

## 设计取舍

`receipt_outbox.aggregate_version` 当前是 cursor seq，不是 conversation 全局严格顺序轴。因此 receipt outbox relay 不使用 message/delivery 那种“低版本 PENDING/DLQ 阻塞高版本”的 conversation-order 策略。这样可以避免某个用户某条回执 DLQ 阻塞同会话其它用户或后续回执事件。

## 限制

- 本轮只验证单 receiver、单 device、单消息的 received/read 事件。
- `im.receipt.events` 还没有下游真实消费者。
- 当前 receipt gRPC 使用 `StaticAllowAccess`，真实权限后续应接入 policy / AuthContext。
- 本轮不是容量压测，不代表 receipt relay 吞吐上限。
