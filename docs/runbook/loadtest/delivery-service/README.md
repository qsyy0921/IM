# delivery-service Loadtest Reports

本目录保存 `delivery-service` 的小规模验证报告和阶段总入口。

## 当前阶段结论

`delivery-service` 已完成第一条真实小闭环：

```text
conversation-service CreateMemberChange
-> message_outbox
-> message-service outbox relay
-> Kafka conversation.timeline.events
-> delivery-service timeline-consumer
-> delivery_membership_projection / user_inbox
-> delivery-service PullInbox
-> delivery-service AckDelivery
-> device_delivery_cursors / delivery_outbox
```

本阶段重点不是硬件容量，而是证明第三个微服务已经拥有 durable delivery read model，并且可以基于统一 conversation timeline 构建用户收件箱。

## 报告列表

| 报告 | 内容 |
| --- | --- |
| `loadtest-report-20260609-delivery-full-smoke.md` | `member joined + SendMessage -> delivery projection -> PullInbox -> AckDelivery` 真实进程 smoke |
| `loadtest-report-20260609-delivery-outbox-smoke.md` | `delivery_outbox -> im.delivery.events` 真实 Kafka 发布 smoke |

## 面试可讲重点

- `delivery-service` 不拥有消息事实源，也不改 `message_log`；它消费 `conversation.timeline.events`，构建面向用户和设备的投递读模型。
- 成员变更事件先进入 `delivery_membership_projection`，消息事件再按成员可见窗口写入 `user_inbox`，避免用“当前成员表”错误解释历史消息可见性。
- `AckDelivery` 只能 ACK 到该用户已可见的最大 seq，不能让客户端随便把 cursor 推到未来。
- Kafka checkpoint 使用 `consumer_group + topic + partition`，记录 next offset；业务投影落库成功后才提交 Kafka offset。
- 当前已补 `delivery_outbox -> im.delivery.events` 最小 relay，并通过真实 Kafka smoke 验证 `PENDING -> PUBLISHED` 和 protobuf `DeliveryEvent` 解码；push-gateway 接入前还需要补 LEAVE/REMOVE 负向可见性验证。
