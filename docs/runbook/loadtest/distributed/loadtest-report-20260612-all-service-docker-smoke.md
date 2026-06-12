# NexusIM All-Service Docker Smoke - 2026-06-12

## 结论

本轮使用 Docker 容器完成小规模系统级 smoke。测试入口是 Windows Docker 中的临时 loadtest runner 容器；被测基础设施和微服务是 Mac Docker 中的 `nexusim-mac-*` 容器；两台机器通过有线 `172.31.50.2` 通信。

最终有效结果：七个服务的核心链路均通过容器化验证：

```text
conversation-service
message-service
delivery-service
push-gateway
receipt-service
contacts-service
identity-service
```

这仍是功能 smoke，不是容量压测、生产 HA、Kafka HA 或 PostgreSQL failover 结论。

## 测试拓扑

- Windows Docker：运行临时 runner 镜像 `nexusim/loadtest-pushgateway:local`、`nexusim/loadtest-contacts:local`、`nexusim/loadtest-receipt:local`、`nexusim/loadtest-memberchange:local`。
- Mac Docker：运行 PostgreSQL、Redis、Kafka、Schema Registry、Kafka UI，以及 7 个 NexusIM 服务镜像对应的 22 个 `nexusim-mac-*` 容器。
- 通信路径：Windows Docker runner -> 有线 `172.31.50.2` -> Mac Docker published ports。
- 原始结果：`H:\NexusIM\loadtest-results\docker-*20260612-*`。

## 有效通过项

| 场景 | 结果目录 | 覆盖点 |
| --- | --- | --- |
| Full IM path | `docker-full-smoke-20260612-1724` | `CreateMemberChange -> SendMessage -> delivery.notify -> PullInbox -> AckDelivery` |
| Message edit notify | `docker-message-edit-smoke-20260612-1725` | `message.edited.v1 -> delivery projection -> push notify -> PullInbox` |
| Message revoke notify | `docker-message-revoke-smoke-20260612-1725` | `message.revoked.v1 -> delivery projection -> push notify -> PullInbox` |
| Message delete notify | `docker-message-delete-smoke-20260612-1725` | `message.deleted.v1 -> delivery projection -> push notify -> PullInbox` |
| Contacts all scenarios | `docker-contacts-*-smoke-20260612-1726` | accept / decline / cancel / delete / block / unblock / remark / readd |
| Receipt and conversation list | `docker-receipt-smoke-20260612-1730` | delivery projection, `MarkRead`, `GetReceiptState`, unread list, archive / pin / mute, receipt outbox -> Kafka |
| Conversation member JOIN | `docker-memberchange-join-seeded-smoke-20260612-1733` | seeded conversation + `CreateMemberChange(JOIN)` + saga DONE + timeline/outbox published |
| Identity HMAC push auth | `docker-identity-hmac-push-smoke-20260612-1734` | identity-service issues gateway token; push-gateway HMAC validates; notify / PullInbox / ACK succeeds |
| Cross-instance resume | `docker-cross-instance-resume-smoke-20260612-1737` | ws-only gateway A + delivery-consumer gateway + ws-only gateway B, Redis-backed resume replay succeeds |

Key facts from successful summaries:

- Full path: `success=true`, `conversation_seq=2`, `PullInbox item_count=1`, `delivery_outbox_published=2`, `pending=0`, `dlq=0`.
- Message change: edit / revoke / delete all returned `success=true`, change event `conversation_seq=3`, `PullInbox event_type` matched the requested action.
- Contacts: all 8 scenarios returned `success=true`.
- Receipt: `success=true`, `receipt_outbox published=3`, `delivery_outbox published=4`, Kafka readback returned received/read receipt events.
- Identity: `push_auth_mode=hmac`, `push_auth_token_source=identity_service`, `push_auth_token_transport=authorization_header`, `success=true`.
- Cross-instance resume: reconnect gateway returned the same `resume_token`, replayed the same `delivery.notify`, and ACK advanced `last_received_seq=2`.

## 排查记录

本轮遇到 3 个测试拓扑 / 参数问题，均未判断为服务代码缺陷：

1. `docker-receipt-smoke-20260612-1728` 失败，因为 runner 缺少 `receipt-events-consumer-group`，导致 Kafka readback 前置参数不完整。补参数后 `docker-receipt-smoke-20260612-1730` 通过。
2. `docker-memberchange-owner-transfer-smoke-20260612-1731` 和 `docker-memberchange-join-smoke-20260612-1732` 失败，因为独立 memberchange runner 不创建初始 conversation。通过 Mac PostgreSQL 容器内 `psql` 写入 conversation / owner / conversation_seq 种子后，`docker-memberchange-join-seeded-smoke-20260612-1733` 通过。
3. `docker-cross-instance-resume-smoke-20260612-1735` / `1736` 失败，因为 `push-gateway-all` 和临时 HMAC all 容器也在消费 `im.delivery.events`，与独立 delivery-consumer 形成重复通知。停止 all 类 consumer，仅保留 ws-only A、delivery-consumer、ws-only B 后，`docker-cross-instance-resume-smoke-20260612-1737` 通过。

这些排查说明：多实例 push-gateway 测试时必须控制 consumer group 数量，避免把多个 consumer 拓扑混进同一个 smoke。

## 未覆盖

- 不代表生产级容量、p95/p99 上限或硬件资源上限。
- 不覆盖 PostgreSQL 主从 / failover、Kafka HA、Redis quorum 网络分区。
- 不覆盖完整 owner-transfer / LEAVE / REMOVE / ROLE_CHANGED 全矩阵；本轮仅用 full path 与 seeded JOIN 覆盖 conversation-service 关键路径。
- 不覆盖真实外部身份体系、mTLS、统一 tracing、告警和灰度编排。

## 后续建议

短期继续做功能完整度，不要长时间停留在重型故障矩阵：

1. 补第三层产品能力缺口：会话列表细节、回执体验、联系人 / 群管理边界、真实鉴权边界。
2. 将 Docker smoke runner 的构建脚本化，避免手工构建临时 runner 镜像。
3. 多实例 push-gateway 测试时区分 `ws`、`delivery-consumer` 和 `all` 模式，禁止重复 consumer 拓扑污染结果。
