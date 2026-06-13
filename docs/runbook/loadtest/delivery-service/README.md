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
| `loadtest-report-20260609-delivery-visibility-negative-smoke.md` | `LEAVE / REMOVE` 后边界之后的新 message event 不再写入目标用户 `user_inbox` |
| `loadtest-report-20260613-delivery-mtls-smoke.md` | delivery-service gRPC server 开启 mTLS + client identity allowlist，并用 gateway verified metadata 完成 `PullInbox -> AckDelivery` |

## 面试可讲重点

- `delivery-service` 不拥有消息事实源，也不改 `message_log`；它消费 `conversation.timeline.events`，构建面向用户和设备的投递读模型。
- 成员变更事件先进入 `delivery_membership_projection`，消息事件再按成员可见窗口写入 `user_inbox`，避免用“当前成员表”错误解释历史消息可见性。
- `AckDelivery` 只能 ACK 到该用户已可见的最大 seq，不能让客户端随便把 cursor 推到未来。
- Kafka checkpoint 使用 `consumer_group + topic + partition`，记录 next offset；业务投影落库成功后才提交 Kafka offset。
- 当前已补 `delivery_outbox -> im.delivery.events` 最小 relay，并通过真实 Kafka smoke 验证 `PENDING -> PUBLISHED` 和 protobuf `DeliveryEvent` 解码。
- 已补 LEAVE/REMOVE 负向可见性 smoke：边界后的 message event 已被 active sender 消费，但离开/移除用户没有任何 `conversation_seq > boundary_seq` 的 `user_inbox`。
- 已补 delivery-service gRPC mTLS smoke：server 端启用 TLS、require client cert、client DNS SAN allowlist=`push-gateway.nexusim.local`，client 端使用 CA/server name/client cert/key，并通过 gateway verified metadata 完成 `PullInbox -> AckDelivery`。该 smoke 不启动 delivery outbox relay，所以 `delivery.ack.recorded.v1` 留在 `PENDING` 是预期。
- `loadtest/delivery` smoke runner 默认仍使用 plaintext 和 body auth；如 delivery-service gRPC server 开启第一阶段静态 TLS / mTLS，可通过 `--delivery-tls-ca-file`、`--delivery-tls-server-name`、`--delivery-tls-client-cert-file`、`--delivery-tls-client-key-file`，或对应 `NEXUSIM_DELIVERY_TLS_*` 环境变量配置 client 侧 TLS。如需验证 gateway verified metadata auth，可用 `--verified-auth-metadata` 或 `NEXUSIM_DELIVERY_LOADTEST_VERIFIED_AUTH_METADATA=true`，runner 会对 `PullInbox / AckDelivery` 发送 `x-nexusim-*` verified identity metadata。
- `loadtest/deliveryvisibility` 默认也使用 plaintext 和 body auth；如需在 TLS / mTLS smoke 中连接 conversation-service 和 delivery-service，可分别使用 `--conversation-tls-*` / `NEXUSIM_CONVERSATION_TLS_*` 与 `--delivery-tls-*` / `NEXUSIM_DELIVERY_TLS_*`。如需验证 gateway verified metadata auth，可用 `--verified-auth-metadata`，runner 会对 conversation-service 成员变更 RPC 和 delivery-service `PullInbox` 同时发送 `x-nexusim-*` verified identity metadata。这些配置只覆盖 runner 到 gRPC server 的 transport / auth smoke，不改变 timeline / inbox / ACK 语义，也不包含证书签发、轮换、分发或完整 API gateway。
- push-gateway 后续只能依赖 delivery read model / delivery event，不能直接读 message-service 内部表，也不能绕过 ACK。
