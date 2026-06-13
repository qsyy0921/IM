# NexusIM SDD Index

本文记录服务级 SDD 的完成状态和编码门禁。

## 文档关系

| 文档 | 路径 | 作用 |
| --- | --- | --- |
| ADD | `docs/architecture/add.md` | 系统级业务架构、服务边界、核心流程和阶段路线 |
| TADD | `docs/architecture/tadd.md` | 技术栈、六层 DDD、工程目录、中间件、部署、观测和编码门禁 |
| SDD Template | `docs/sdd/TEMPLATE.md` | 后续服务级 SDD 的统一模板 |
| message-service SDD | `docs/sdd/message-service.md` | 第一条可编码切片的服务设计 |
| conversation-service SDD | `docs/sdd/conversation-service.md` | 会话发送上下文读取、成员事实和成员变更 Saga 边界 |
| conversation-service member_change_saga SDD | `docs/sdd/conversation-service-member-change-saga.md` | 成员变更 Saga、成员边界 timeline event、ACL 投影失败窗口 |
| delivery-service SDD | `docs/sdd/delivery-service.md` | timeline 投影、user_inbox、离线补拉和设备 ACK |
| push-gateway SDD | `docs/sdd/push-gateway.md` | WebSocket 在线连接、delivery event 唤醒、PullInbox / AckDelivery 协调 |
| identity-service SDD | `docs/sdd/identity-service.md` | gateway token 签发、device / session 生命周期和撤销边界 |
| policy-service SDD | `docs/sdd/policy-service.md` | 消息动作权限决策、permission version 和策略服务边界 |
| receipt-service SDD | `docs/sdd/receipt-service.md` | 送达 / 已读回执 read model、MarkRead 和 receipt event 边界 |
| receipt-service conversation list SDD | `docs/sdd/receipt-service-conversation-list.md` | 会话列表 / 未读数 read model，复用 receipt-service projection |
| contacts-service SDD | `docs/sdd/contacts-service.md` | 联系人 / 好友关系事实源、好友申请、联系人列表、删除 / 拉黑 / 备注名和 contact outbox 边界 |

## 六层 DDD 约定

所有服务统一使用六层 DDD 架构：

| 层 | 职责 | 示例 |
| --- | --- | --- |
| `api` | 对外接口适配层 | gRPC handler、HTTP handler、request/response 转换 |
| `app` | 应用用例层 | UseCase、事务编排、调用 domain 和 infrastructure |
| `domain` | 领域规则层 | 聚合、实体、领域事件、幂等规则、状态流转 |
| `infrastructure` | 基础设施实现层 | PostgreSQL、Kafka、Redis、外部 RPC client、sqlc repo |
| `types` | 类型定义层 | Command、DTO、枚举、错误码、常量、跨层轻量类型 |
| `trigger` | 触发器 / 后台任务层 | Outbox Relay、Kafka consumer、定时巡检、补偿任务 |

## 当前状态

| 服务 / 设计 | 状态 | 对编码的影响 |
| --- | --- | --- |
| `message-service` | SDD 已冻结 v1.0；Send/Edit/Revoke/Delete、adaptive admission、policy/conversation RPC adapter、outbox relay 和第一阶段 gRPC TLS / mTLS server config 已落地 | 继续保持消息事实源边界；server TLS 默认 plaintext，证书签发 / 轮换 / 分发、动态服务身份和全服务 mTLS rollout 仍是后续项 |
| `timeline-service / sequencer` | SDD 未完成 | 不阻塞 `LOCAL_ROW_LOCK`；阻塞热点会话生产实现 |
| `conversation-service / send context` | SDD v0.1 已存在 | 可以实现 `GetSendContext` 读取路径，替换 message-service strict conversation mock |
| `conversation-service / member_change_saga` | SDD 已冻结 v1.0；proto / schema / migration v2 / relay builder / 最小 `CreateMemberChange` 写路径、saga publish 状态推进、full smoke 和当前 ACTIVE 成员 `ListConversationMembers` 读接口已落地；`LEAVE / REMOVE` 后 roster 过滤和 `ROLE_CHANGED` 后 role 更新 smoke 已覆盖 | 后续补 DLQ repair、admin-only 成员历史查询、更完整权限负例和生产韧性 |
| `push-gateway` | SDD v0.1 Draft、WebSocket frame、`im.delivery.events` consumer、ACK 转发、HMAC auth、Redis route / resume、slow session close 和多实例 smoke 已落地 | 只做在线唤醒和回源协调；后续补真实 identity、session revoke、Redis route hardening 和生产指标 |
| `identity-service` | SDD v0.1 Draft、proto / migration / 六层骨架、gateway token 签发、refresh rotation、邮箱/手机验证、密码重置 challenge、TOTP MFA 因子生命周期、Login 强制校验、factor 级 MFA 失败锁定、MFA recovery codes 生成 / 再生成 / 吊销、`MFA_UNAVAILABLE` 稳定错误和 `/debug/metrics` MFA 风险计数已落地 | 支撑 push-gateway 本地 token 验签；Refresh step-up、WebAuthn、OIDC federation、backup factor policy 和 KMS/HSM 仍是后续项 |
| `policy-service` | SDD v0.1 Draft、proto / 六层骨架、静态 message action policy、message-service 可选 RPC adapter 已落地 | 已抽出 message-service policy 边界；contacts / conversation / tenant / risk projection 和真实策略引擎仍是后续项 |
| `delivery-service` | SDD v0.1 已存在，最小 projection / PullInbox / AckDelivery / delivery outbox relay 和第一阶段 gRPC TLS / mTLS server config 已落地 | 可以支撑 push-gateway 第一阶段，只要 push-gateway 不绕过 durable inbox / ACK；server TLS 默认 plaintext，push-gateway client TLS 迁移、证书治理和全服务 mTLS rollout 仍是后续项 |
| `receipt-service` | SDD v0.1 Draft、proto、Kafka schema、migration、六层骨架、PostgreSQL repository、delivery consumer、MarkRead、receipt outbox relay、`ListReceiptStates`、最小 `ListConversations`、会话未读 read model、`unread_only` 过滤、Archive / Pin / Mute 用户列表偏好和第一阶段 gRPC TLS / mTLS server config 已落地 | 后续补真实权限、更多列表筛选和真正通知静音策略；不得直接读取 delivery-service 内部表；server TLS 默认 plaintext，客户端 TLS 迁移、证书治理和全服务 mTLS rollout 仍是后续项 |
| `contacts-service` | SDD v0.1 Draft、proto / Kafka schema / migration / 六层骨架、PostgreSQL repository、contacts outbox relay 和 ACCEPT / DECLINE / Delete / Block / Unblock / Remark / Re-add 真实进程 smoke 已落地 | 继续保持联系人关系独立事实源；不得把好友关系写入 `conversation_members`，也不得自动创建会话或让 message-service 同步依赖 contacts-service |
| `retrieval-gateway` | SDD 未完成 | 不进入第一条代码切片 |

## 已完成的 message-service 切片

`message-service` 第一阶段已经进入可运行基线，范围是：

```text
message-service SendMessage
-> PostgreSQL local transaction
-> conversation_seq
-> message_log
-> conversation_timeline_events
-> message_outbox
-> outbox relay
-> Kafka publish path
```

边界：

- 只实现普通会话 `LOCAL_ROW_LOCK`。
- `policy-service` 已有第一版独立 gRPC 服务和 message-service 可选 RPC adapter；未配置 `NEXUSIM_POLICY_SERVICE_ADDR` 时仍使用 strict mock port。
- `conversation-service` 已开始替换 strict mock：当前可通过 `GetSendContext` gRPC read path 提供会话发送上下文。
- 不实现热点 sequencer。
- 不实现 delivery、push、RAG、Agent。
- 不绕过 outbox 直接发布 Kafka。

## 当前 conversation-service 切片

当前已完成的主要范围：

```text
conversation-service GetSendContext
-> PostgreSQL conversations / conversation_members
-> gRPC response
-> message-service ConversationQueryPort gRPC client
```

边界：

- 已实现会话发送上下文读取。
- 已实现成员变更 `CreateMemberChange` 最小写路径和 `GetMemberChange` 查询。
- 已实现当前 ACTIVE 成员 `ListConversationMembers` 读接口，并已验证 `JOIN` 后返回 active roster、`LEAVE / REMOVE` 后不再返回目标成员、`ROLE_CHANGED` 后返回更新后的当前角色；它只暴露会话当前 roster，不承担成员历史 / 审计查询。
- 已实现成员边界事件通过统一 outbox relay 发布到 Kafka，并由 worker 推进 saga 到 `DONE`。
- 不在 conversation-service 写消息正文，不直接 publish Kafka；成员变更只通过 shared timeline/outbox 写成员边界事件。
- `message-service` 未配置 `NEXUSIM_CONVERSATION_SERVICE_ADDR` 时仍可使用 strict mock，便于历史压测复现。

## 当前 delivery-service 切片

当前已完成的主要范围：

```text
Kafka conversation.timeline.events
-> delivery-service timeline projection
-> PostgreSQL user_inbox / delivery cursor
-> PullInbox / AckDelivery
-> delivery_outbox
-> Kafka im.delivery.events
```

边界：

- 只实现 durable delivery read model，不实现 WebSocket 长连接。
- `push-gateway` 后续只做连接和在线推送，不写 `user_inbox`。
- `user_inbox` 是可重建投影，不是 message 事实源。
- 已完成小规模 full smoke、delivery outbox smoke 和 LEAVE / REMOVE 负向可见性 smoke。
- 不继续做重型硬件矩阵。

## 当前 push-gateway 设计切片

当前优先范围：

```text
Kafka im.delivery.events
-> push-gateway delivery event consumer
-> online WebSocket notification
-> client PullInbox
-> client AckDelivery through push-gateway
```

边界：

- 只做在线连接、轻量通知、短时 resume buffer 和 ACK/Pull 协调。
- 不拥有 `user_inbox`，不修改 `device_delivery_cursors`。
- 不直接读取 message-service / conversation-service / delivery-service 内部表。
- `delivery.notify` 只是唤醒信号，客户端展示事实仍以 `PullInbox` 为准。
- 第一阶段不新增 PostgreSQL migration；多实例前再接 Redis route。

## 当前 receipt-service / conversation list 切片

当前优先范围：

```text
Kafka im.delivery.events
-> receipt-service delivery event projection
-> received / read cursor
-> MarkRead
-> GetReceiptState
-> receipt_outbox
-> user_conversation_summaries / ListConversations
```

边界：

- `delivery.ack.recorded.v1` 是 received / device persisted 事实，不等于 read。
- read 必须由客户端显式 `MarkRead` 推进。
- receipt-service 不直接读取 `delivery-service` 内部表；它消费 `im.delivery.events` 建自己的 projection。
- receipt event 只能通过 receipt outbox 发布。
- 权限来源必须通过 `ReceiptAccessPort` 表达；不能用 receipt projection 的存在性替代会话访问权限。
- 会话列表 / 未读数复用 receipt-service projection，不新增独立服务，不直接读取 delivery-service 内部表。
- 最小真实进程 smoke 已验证投递后 `unread_count=1`，`MarkRead` 后 `unread_count=0`。
- summary 更新与 `MarkRead` 使用同一 tenant/user/conversation advisory lock，避免并发 projection 覆盖 read cursor。

## 已补齐的工程基线

| 文件 | 状态 | 说明 |
| --- | --- | --- |
| `api/proto/nexusim/message/v1/message_service.proto` | 已存在 | 需要生成 Go 代码 |
| `api/proto/nexusim/message/v1/message_error.proto` | 已存在 | 需要生成 Go 代码 |
| `api/proto/nexusim/conversation/v1/conversation_service.proto` | 已存在 | 定义 `GetSendContext` |
| `schemas/kafka/conversation.timeline.events.proto` | 已存在 | 需要生成 Go 代码或接入发布侧序列化 |
| `migrations/postgres/message/000001_message_core.sql` | 已存在 | 可进入本地事务实现 |
| `migrations/postgres/conversation/000001_conversation_core.sql` | 已存在 | 支撑 `GetSendContext` read path 和后续成员变更 Saga |
| `services/message-service` 六层目录 | 已存在 | 可进入服务实现 |
| `services/conversation-service` 六层目录 | 已存在 | 可进入 `GetSendContext` read path 实现 |
| `deploy/local` 基础设施 | 已存在 | 可启动本地 PostgreSQL / Kafka / Schema Registry / Redis |

## 下一批 SDD

优先级：

1. `receipt-service` 会话列表 / 未读数真实权限校验和产品化字段
2. `timeline-service-sequencer.md`
3. `retrieval-gateway.md`

其中 `push-gateway.md` 已补 v0.1 Draft，下一步应先做阶段评审，再落 WebSocket frame 契约、六层骨架和最小在线通知 smoke。`timeline-service` SDD 不阻塞普通会话当前实现，但阻塞热点会话生产化。
