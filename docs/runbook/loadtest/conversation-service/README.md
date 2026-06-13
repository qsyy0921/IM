# Conversation Service 压测报告入口

本目录保存 `conversation-service` 的小规模验证报告和阶段总报告。

当前阶段重点不是做容量压测，而是证明第二个真实微服务已经接入：

```text
message-service SendMessage
-> ConversationQueryPort
-> conversation-service gRPC GetSendContext
-> PostgreSQL conversations / conversation_members
-> message-service PostgreSQL local transaction
```

```text
conversation-service CreateMemberChange
-> shared timeline/outbox
-> message-service outbox relay
-> Kafka conversation.timeline.events
-> conversation-service member-change-worker
-> GetMemberChange(DONE)
```

## 当前结论

- `conversation-service` 已有独立六层 DDD 骨架。
- `GetSendContext` 已经能从 PostgreSQL 读取会话状态、成员状态、版本号、会话模式和 fanout 策略。
- `message-service` 已支持通过 `NEXUSIM_CONVERSATION_SERVICE_ADDR` 切到真实 conversation-service，不再只能依赖 strict mock。
- 第一轮真实进程 smoke 结果：`725 / 725` 成功，p95 `10.36ms`，p99 `13.26ms`。
- 本轮 smoke 没启动 outbox relay，因此 summary 中 `outbox_pending_count=725` 是预期现象；测试结束后已删除本次 tenant 数据，避免污染后续压测。
- `CreateMemberChange` 最小写路径已完成真实进程 smoke：`279 / 279` 成功，p99 `24.95ms`，`outbox_published_count=279`，`outbox_pending_count=0`。
- `CreateMemberChange -> outbox relay -> member-change-worker -> GetMemberChange(DONE)` 完整 smoke 已通过：`350 / 350` 成功，p99 `40.90ms`，`saga_done_count=350`，`sample_get_status=MEMBER_CHANGE_STATUS_DONE`。
- `ListConversationMembers` 最小 roster smoke 已通过：3 条 `JOIN` 后，真实 gRPC 读取到 `member_list_count=4`，成员为 seed owner + 3 个 active target；`LEAVE` / `REMOVE` 后 roster smoke 也已通过，目标成员变为 `LEFT` 后不再出现在普通 ACTIVE roster；`ROLE_CHANGED` 后 roster smoke 已通过，目标成员仍为 ACTIVE 且 role 更新为 `ADMIN`；该接口只返回当前 ACTIVE 成员和当前角色，不承担成员历史 / 审计视图。
- `TransferConversationOwner` 最小真实进程 smoke 已通过：1 次 owner transfer 成功，旧 owner 仍为 ACTIVE 但降级为 `ADMIN`，新 owner 为唯一 ACTIVE `OWNER`，`saga_done_count=1`，`outbox_pending_count=0`，`outbox_published_count=1`。
- `loadtest/memberchange` smoke runner 默认仍使用 plaintext 和 body auth；如 conversation-service gRPC server 开启第一阶段静态 TLS / mTLS，可通过 `--conversation-tls-ca-file`、`--conversation-tls-server-name`、`--conversation-tls-client-cert-file`、`--conversation-tls-client-key-file`，或对应 `NEXUSIM_CONVERSATION_TLS_*` 环境变量配置 client 侧 TLS。如需验证 gateway verified metadata auth，可用 `--verified-auth-metadata` 或 `run-local-smoke.ps1 -VerifiedAuthMetadata`，脚本会同时把 conversation-service 切到 `NEXUSIM_CONVERSATION_AUTH_MODE=metadata`。这些能力只覆盖 runner 到 conversation-service 的 gRPC transport / auth smoke，不包含证书签发、轮换、分发或完整 API gateway。

## 报告列表

| 类型 | 文件 |
| --- | --- |
| 阶段总报告 | `loadtest-report-20260609-conversation-service-consolidated.md` |
| 第一轮跨服务 smoke | `loadtest-report-20260609-send-context-smoke.md` |
| 成员变更写路径 smoke | `loadtest-report-20260609-member-change-smoke.md` |
| 成员变更完整 smoke | `loadtest-report-20260609-member-change-full-smoke.md` |
| 当前成员列表 smoke | `loadtest-report-20260610-list-conversation-members-smoke.md` |
| LEAVE 后成员列表过滤 smoke | `loadtest-report-20260610-list-conversation-members-leave-smoke.md` |
| REMOVE 后成员列表过滤 smoke | `loadtest-report-20260610-list-conversation-members-remove-smoke.md` |
| ROLE_CHANGED 后成员角色更新 smoke | `loadtest-report-20260610-list-conversation-members-role-smoke.md` |
| owner transfer smoke | `loadtest-report-20260610-owner-transfer-smoke.md` |

## 面试可讲点

- 这一步把系统从“一个 message-service + mock”推进到“两个真实微服务之间的 gRPC 依赖”。
- `conversation-service` 是会话和成员事实源，`message-service` 只读取发送上下文，不写成员事实。
- 版本号通过 `member_version` / `permission_version` 返回给 `message-service`，用于发送前的一致性校验。
- 成员变更写路径采用 saga + timeline/outbox：先写成员事实和边界事件，再由统一 outbox relay 发布 Kafka。
- saga completion worker 已经能观察 outbox `PUBLISHED` 并把 `member_change_saga` 推进到 `DONE`，`GetMemberChange` 能读到完成态。
- `ListConversationMembers` 给第三层群管理提供低耦合 roster API：其它服务不跨表读取 `conversation_members`，普通列表只暴露当前 ACTIVE 成员。
- 当前已验证保守权限矩阵下的最小 `JOIN`、`LEAVE / REMOVE` 后普通 roster 过滤、`ROLE_CHANGED` 后普通 roster role 更新，以及 owner transfer 专用 RPC 的单事务 / 单 seq / 单事件闭环；ACL 投影、DLQ repair、历史成员审计和更完整权限负例还在后续阶段。
