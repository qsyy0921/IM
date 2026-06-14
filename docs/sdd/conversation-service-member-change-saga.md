# NexusIM conversation-service member_change_saga SDD v1.0

状态：Frozen

本文冻结 `conversation-service` 成员变更 Saga 的服务级设计。它解决的问题不是“改一行成员表”，而是把成员事实、timeline 边界事件、权限版本和后续 ACL 投影放进一个可解释的失败窗口。

当前文档先冻结设计边界和编码门禁；成员变更 API、Kafka schema 和代码实现后续按本文落地。

## 1. 服务定位

`conversation-service` 是会话和成员事实源。

职责：

- 创建和维护会话成员事实：`conversation_members`。
- 维护 `member_version` 和 `permission_version`。
- 执行成员变更 Saga：加入、退出、移除、角色变更。
- 为成员变更分配 conversation timeline 边界 seq。
- 产生成员边界 timeline event，并保证与消息事件共享同一会话顺序轴。
- 为 ACL / retrieval / delivery 下游提供可重放的成员边界事实。

不负责：

- 不写 `message_log`。
- 不写消息正文。
- 不直接推送 WebSocket。
- 不直接修改 OpenSearch / Milvus 索引。
- 不绕过 outbox relay 直接发布 Kafka。
- 不实现热点会话 sequencer 生产逻辑；`SEQUENCER_BLOCK` 仍等待 timeline-service / sequencer SDD。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游 | Web / desktop / admin API / future control plane | 发起成员变更命令 |
| 同步依赖 | policy-service | 校验操作者是否有成员变更权限，第一版可用 strict mock |
| 同步依赖 | timeline-service / sequencer | 仅热点会话需要；第一版普通会话使用 PostgreSQL `conversation_seq` |
| 事实源 | PostgreSQL | `conversations`、`conversation_members`、`member_change_saga` |
| 异步下游 | Kafka `conversation.timeline.events` | 发布 `conversation.member.*` 边界事件 |
| 异步下游 | delivery-service | 根据成员边界修正 inbox / cursor / fanout |
| 异步下游 | retrieval-gateway / search-service | 更新 ACL 投影；失败时 retrieval 进入 strict ACL 回源校验 |
| 异步下游 | audit-service | 记录成员变更命令、冲突和补偿结果 |

## 3. 六层 DDD 包结构

```text
services/conversation-service/
  cmd/conversation-service
  internal/
    api/
    app/
    domain/
    infrastructure/
    types/
    trigger/
```

| 层 | 本服务内容 |
| --- | --- |
| `api` | `CreateMemberChange` / `GetMemberChange` gRPC handler，request/response/error 转换 |
| `app` | `CreateMemberChangeUseCase`，权限检查，事务编排，Saga 状态推进 |
| `domain` | 成员变更命令、角色/状态规则、版本冲突、Saga 状态机、事件 payload 构造 |
| `infrastructure` | PostgreSQL repository、timeline/outbox append adapter、policy RPC client、audit client |
| `types` | Command、DTO、枚举、错误 sentinel、轻量事件类型 |
| `trigger` | `member_change_saga` retry / compensation worker、DLQ repair worker |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `Conversation` | 会话事实 | `ACTIVE` 才允许成员变更；`DELETED` 禁止任何成员命令 |
| `ConversationMember` | 成员事实 | 同一 `(tenant_id, conversation_id, user_id)` 只有一条当前事实 |
| `MemberChangeSaga` | 成员变更长事务记录 | 每个幂等键只对应一个 `change_id`；状态只允许正向推进或补偿失败 |
| `MemberBoundaryEvent` | 成员边界 timeline event | 必须有全局 conversation seq；必须写入 outbox；不能只更新成员表 |
| `PermissionVersion` | 权限版本 | 成员变更成功后递增，用于 `message-service` 发送前版本校验 |

成员状态：

```text
ACTIVE
LEFT
BANNED
```

成员变更类型：

```text
JOIN
LEAVE
REMOVE
ROLE_CHANGED
```

Saga 状态：

```text
PENDING_BOUNDARY
-> BOUNDARY_ALLOCATED
-> MEMBER_UPDATED
-> OUTBOX_ENQUEUED
-> EVENT_PUBLISHED
-> DONE

any state -> FAILED_COMPENSATED
```

说明：

- `OUTBOX_ENQUEUED` 表示本地事务已经写入 `conversation_timeline_events` 和 `message_outbox`。
- `EVENT_PUBLISHED` 表示 relay 已经把 outbox 事件发布到 Kafka 并标记 `PUBLISHED`。
- `EVENT_PUBLISHED / DONE` 推进必须 fail-closed：trigger worker 只能观察同 tenant / conversation、`producer='conversation-service'`、event_type 属于 `conversation.member.*` 且 `PUBLISHED + published_at IS NOT NULL` 的 outbox 行；手工 repair、错误 producer 或错误 event type 不得推进成员 saga。
- migration v2 已补齐 `OUTBOX_ENQUEUED` 状态，避免把“已入 outbox”误写成“已发布 Kafka”。

## 5. 同步 API 契约

契约文件规划：

```text
api/proto/nexusim/conversation/v1/conversation_service.proto
```

后续新增 RPC：

```text
rpc CreateMemberChange(CreateMemberChangeRequest) returns (CreateMemberChangeResponse);
rpc GetMemberChange(GetMemberChangeRequest) returns (GetMemberChangeResponse);
rpc ListConversationMembers(ListConversationMembersRequest) returns (ListConversationMembersResponse);
```

`CreateMemberChangeRequest` 必须包含：

```text
auth_context.tenant_id
auth_context.user_id
auth_context.trace_id
auth_context.request_id
conversation_id
target_user_id
change_type
target_role
expected_member_version
idempotency_key
conflict_policy
reason
```

说明：`operator_user_id` 由 `auth_context.user_id` 派生。生产模式不信任 request 里的裸 operator 字段；第一版本地 smoke 也按同一字段模拟认证上下文。

第一版 `conflict_policy` 只接受 `REJECT`。`MERGE` 和 `COMPENSATE` 是协议预留值，真实语义未实现前必须返回 `INVALID_ARGUMENT`，避免调用方误以为系统已经执行自动合并或补偿。

`CreateMemberChangeResponse` 必须包含：

```text
change_id
conversation_id
target_user_id
change_type
status
boundary_seq
member_version
permission_version
idempotent_replay
```

`ListConversationMembers` 第一版只提供当前会话 ACTIVE 成员 roster：

```text
auth_context
conversation_id
page_size
page_token
```

返回：

```text
tenant_id
conversation_id
member_version
permission_version
members(user_id, role, status, join_seq, leave_seq, member_version, permission_version)
next_page_token
```

边界：

- 调用者必须是该会话当前 ACTIVE 成员；否则返回 `PERMISSION_DENIED` / `conversation member is not active`。
- 会话不存在、归档或删除时返回 `CONVERSATION_NOT_FOUND`。
- 第一版只返回 `status=ACTIVE` 的当前成员，不暴露 `LEFT / BANNED` 历史成员；审计 / 管理视角后续单独设计 admin-only 查询，避免把权限矩阵塞进普通成员列表。
- `page_token` 是 opaque token；当前实现内部按 `user_id ASC` keyset 分页，调用方不得解析 token。
- 该接口只读 `conversation-service` 自己的 `conversations / conversation_members` 事实表；其它服务需要成员列表时必须通过正式 API / projection，不得跨服务读取内部表。

错误码：

| 错误码 | gRPC code | 语义 | 是否可重试 |
| --- | --- | --- | --- |
| `INVALID_ARGUMENT` | `InvalidArgument` | 参数缺失、非法 change_type / conflict_policy | 否 |
| `CONVERSATION_NOT_FOUND` | `NotFound` | 会话不存在或不可变更 | 否 |
| `MEMBER_CONFLICT` | `FailedPrecondition` | 版本冲突、状态冲突、角色冲突 | 否 |
| `PERMISSION_DENIED` | `PermissionDenied` | 操作者无权限 | 否 |
| `SEQUENCER_UNAVAILABLE` | `Unavailable` | 热点 seq authority 不可用 | 是 |
| `DB_WRITE_FAILED` | `Unavailable` | 本地事务失败 | 是 |
| `OUTBOX_WRITE_FAILED` | `Unavailable` | 边界事件 outbox 写失败 | 是 |
| `SERVICE_OVERLOADED` | `Unavailable` | admission / backpressure 拒绝 | 是 |

## 6. 异步事件契约

Topic：

```text
conversation.timeline.events
```

分区键：

```text
tenant_id + ":" + conversation_id
```

事件类型：

| 事件 | event_type | aggregate_version | 下游 |
| --- | --- | --- | --- |
| 成员加入 | `conversation.member.joined.v1` | `boundary_seq` | delivery、retrieval/search、audit |
| 成员退出 | `conversation.member.left.v1` | `boundary_seq` | delivery、retrieval/search、audit |
| 成员移除 | `conversation.member.removed.v1` | `boundary_seq` | delivery、retrieval/search、audit |
| 角色变更 | `conversation.member.role_changed.v1` | `boundary_seq` | policy/retrieval/search、audit |
| 边界取消 | `conversation.member.boundary_cancelled.v1` | `boundary_seq` | audit、repair |

事件 payload 必须包含：

```text
change_id
conversation_id
target_user_id
operator_user_id
change_type
old_role
new_role
old_status
new_status
member_version
permission_version
reason
occurred_at
```

成员边界 outbox envelope 映射必须固定：

| 字段 | 来源 / 规则 |
| --- | --- |
| `event_id` | saga 创建时生成并持久化到 `member_change_saga.outbox_event_id`；同一幂等键 replay 必须复用 |
| `event_version` | `v1` |
| `mapping_version` | 等于具体事件类型，例如 `conversation.member.joined.v1` |
| `correlation_id` | 优先 `auth_context.request_id`，为空时使用 `change_id` |
| `causation_id` | `change_id` |
| `producer` | `conversation-service` |
| `trace_id` | `auth_context.trace_id`，为空时使用 `auth_context.request_id` |
| `payload_json` | oneof 业务 payload 的 JSON 形状，不保存完整 envelope |

`conversation_timeline_events.event_id` 和 `message_outbox.event_id` 第一版都使用同一个 `outbox_event_id`，并把该值保存到 `member_change_saga.timeline_event_id` / `member_change_saga.outbox_event_id`。这样 trigger worker 可以用稳定 `outbox_event_id` 查询 outbox 发布状态，再把 saga 推进到 `EVENT_PUBLISHED / DONE`；不得只依赖 `(tenant_id, conversation_id, boundary_seq, event_type)` 反查。

Kafka schema `schemas/kafka/conversation.timeline.events.proto` 已包含 member boundary oneof payload：`ConversationMemberJoinedV1` / `ConversationMemberLeftV1` / `ConversationMemberRemovedV1` / `ConversationMemberRoleChangedV1` / `ConversationMemberBoundaryCancelledV1`。

现有统一 outbox relay 已升级为支持 `conversation.member.*`：

- `services/message-service/internal/trigger/outbox/relay.go` 可以 build message/member 两类 conversation timeline event。
- unsupported / malformed event 必须 fail-closed：不崩进程、不误标记 `PUBLISHED`，进入 retry / DLQ，并继续按同 conversation 顺序阻塞，除非受控 repair / audit 显式 skip。
- 真实成员变更写路径仍必须通过 shared timeline/outbox append port 在同一事务内写 `conversation_timeline_events` 和 `message_outbox`，不得在 API / usecase 中拼接跨表 SQL 或直接 publish Kafka。

## 7. 数据库设计

已有 migration：

```text
migrations/postgres/conversation/000001_conversation_core.sql
```

已有表：

- `conversations`
- `conversation_members`
- `member_change_saga`

编码前需要新增或确认的字段：

| 表 | 字段 / 索引 | 说明 |
| --- | --- | --- |
| `member_change_saga` | `completed_at` | Saga 成功完成时间 |
| `member_change_saga` | `dead_lettered_at` | 补偿失败或人工处理时间 |
| `member_change_saga` | `next_retry_at` | trigger worker 重试调度 |
| `member_change_saga` | `timeline_event_id` | 关联 `conversation_timeline_events.event_id` |
| `member_change_saga` | `outbox_event_id` | 关联 `message_outbox.event_id`，trigger / repair 以它为主键 |
| `member_change_saga` | `metadata_json` | 保存旧角色、旧状态、策略版本等扩展字段 |
| `member_change_saga` | status check 增加 `OUTBOX_ENQUEUED` | 区分已入 outbox 和已发布 Kafka |
| `conversation_members` | `join_seq` / `leave_seq` | 已存在，用于解释成员边界 |
| `conversation_members` | `(tenant_id, conversation_id, status)` index | delivery/retrieval 查询活跃成员 |

重要约束：

- `conversation_members.member_version` 和 `conversations.member_version` 必须在同一个本地事务内推进。
- `permission_version` 变化必须与角色/状态变化同事务提交。
- `boundary_seq` 一旦分配，必须能在 timeline 上解释；如果成员更新失败，必须写 cancelled boundary 或保留 saga repair 证据。
- `member_change_saga.boundary_seq == conversation_timeline_events.seq == message_outbox.aggregate_version`。
- 成员边界 timeline 行不写 `message_log`；`conversation_timeline_events.message_id` 必须为 `NULL`，`actor_id` 写 `operator_id`。
- 成员边界 timeline 行必须写非空 `permission_version`。当前 outbox store 会把该字段 scan 到 `int64`，不能依赖 nullable 语义。

## 8. 核心流程

第一版普通会话流程选定目标架构中的 **方案 C**：

```text
所有 timeline event 进入同一张 conversation_timeline_events 和同一条 outbox 流。
```

选择原因：

- 当前 `message-service` 已经验证 `conversation_seq + timeline + outbox` 同事务模型。
- 第一批生产化没有 timeline-service authority，方案 A 需要额外服务。
- 方案 B 需要全局 publish cursor，治理成本更高。
- 方案 C 能在本地事务内让 message event 和 member boundary event 共享 `conversation_seq` 顺序轴。

普通会话 `JOIN / LEAVE / REMOVE / ROLE_CHANGED` 流程：

```text
api CreateMemberChange
-> app validate command
-> policy check operator permission
-> db tx begin
-> lock conversations row
-> idempotency lookup by (tenant_id, conversation_id, idempotency_key)
-> lock target conversation_member row or create placeholder
-> domain validate status / role / expected_member_version / conflict_policy
-> ensure conversation_seq row after conversation ACTIVE is verified
-> allocate boundary_seq through conversation_seq row using UPDATE ... RETURNING
-> generate stable outbox_event_id / timeline_event_id for this change_id
-> insert / update member_change_saga(status=BOUNDARY_ALLOCATED)
-> update conversation_members
-> increment conversations.member_version / permission_version
-> insert conversation_timeline_events(member boundary)
-> insert message_outbox(event_type=conversation.member.*)
-> update member_change_saga(status=OUTBOX_ENQUEUED)
-> commit
-> outbox relay publishes Kafka
-> trigger worker marks saga EVENT_PUBLISHED / DONE after outbox PUBLISHED is observed
```

说明：

- 第一版允许在同一 PostgreSQL 事务内写 `conversation_timeline_events` 和 `message_outbox`，但必须通过 conversation-service 自己的 repository / port 完成，不能直接复用 message-service 的 domain 模型。
- 代码落地前应把 timeline append / outbox append 抽成明确 port，避免成员变更逻辑散落在 repository SQL 中。
- 这不是让 conversation-service 修改 message-service 的消息事实；它只追加 conversation timeline 边界事件。
- 不允许启动第二套 relay 竞争同一张 `message_outbox`；必须升级现有统一 outbox relay，使 message/member timeline event 经同一条发布路径输出。
- 如果暂时没有扩展 Kafka schema 和 relay builder，`member_change_saga` 最多只能记录命令，不得声明成员事件已进入 `conversation.timeline.events` 全序流。
- `DONE` 是 conversation-service 本地 saga 完成态，只表示成员事实更新完成且边界事件已经通过 outbox 发布到 Kafka；它不表示 delivery、retrieval/search ACL projection、audit sink 都已完成。下游 projection lag / checksum mismatch 由各 consumer、strict ACL fallback 和独立 repair 处理。

## 9. 一致性和事务

强一致边界：

```text
member_change_saga
conversation_members
conversations.member_version / permission_version
conversation_seq boundary allocation
conversation_timeline_events
message_outbox
```

这些必须在同一个 PostgreSQL transaction 内提交或回滚。

物理表所有权说明：

- 当前 `conversation_seq`、`conversation_timeline_events`、`message_outbox` 由 message migration 创建，这是第一阶段工程落地结果。
- 成员变更编码前应把这些表在文档和 migration 目录上标记为 conversation timeline shared store，或至少在 SDD 中明确两类服务只能通过 timeline/outbox append port 写入。
- 不允许 conversation-service 直接修改 `message_log`，也不允许 message-service 修改 `conversation_members`。
- 当前阶段统一 outbox relay 仍运行在 `message-service/internal/trigger/outbox`，这是工程过渡安排；它承担 shared conversation timeline outbox relay 职责，不代表 member event 归 message-service 所有。生产化时应评估独立为 `timeline-outbox-relay`，或在 TADD 中明确保留现有部署口径。

最终一致边界：

```text
Kafka publish
delivery inbox rebuild / fanout correction
retrieval/search ACL projection
audit sink
```

这些通过 outbox relay / consumer / trigger worker 重试，不能进入成员变更业务事务。

顺序不变量：

- 同一 conversation 的 message event 和 member boundary event 共享 `conversation_seq`。
- 同一 `(conversation_id, user_id)` 的成员变更串行执行。
- 同一 idempotency key 重试必须返回同一 `change_id` 和同一 `boundary_seq`。
- 边界 event 的 `aggregate_version` 必须等于 `boundary_seq`。
- 边界 event 不允许跳过 outbox 直接 publish Kafka。
- unsupported event 必须 fail-closed：relay 不崩溃，但事件保持 PENDING retry 或进入 DLQ，并继续阻塞同 conversation 后续更高版本事件；只有受控 repair / audit 流程可以显式 skip。

## 10. 幂等、重试和补偿

| 场景 | 幂等键 | 重试策略 | 补偿 |
| --- | --- | --- | --- |
| 同一请求重复提交 | `(tenant_id, conversation_id, idempotency_key)` | 返回已有 saga | 不重复分配 seq |
| 版本冲突 | `expected_member_version` | 不自动重试 | 返回 `MEMBER_CONFLICT` |
| boundary 分配失败 | `change_id` | 可重试整个事务 | saga 保持 `PENDING_BOUNDARY` 或失败 |
| 成员更新失败 | `change_id` | 回滚事务 | 不产生边界 event |
| outbox 写失败 | `change_id` | 回滚事务 | 不更新成员事实 |
| Kafka publish 失败 | `event_id` | outbox relay retry | 超限 DLQ，saga 不进入 `DONE` |
| ACL 投影失败 | `change_id` | 下游 consumer retry | retrieval-gateway strict ACL fallback |

Outbox / DLQ repair：

- 成员已更新但 outbox 未发布时，不能回滚成员事实。
- outbox DLQ 需要 repair command：`replay`、`skip_with_audit`、`mark_compensated`。
- `skip_with_audit` 只能由人工或受控 runbook 执行，并必须写 audit / repair event。
- retrieval/search 看到 ACL 投影延迟或 checksum 不一致时，进入 strict ACL 回源校验。

补偿策略：

- 如果事务内失败，回滚即可，不推进成员事实。
- 如果事务已提交但 outbox 迟迟未发布，trigger worker 负责重试/告警。
- 如果边界 event 已发布但 ACL 投影失败，不能回滚成员事实；必须让 retrieval 回源校验。
- 如果人工跳过 DLQ，必须写 audit 和 repair event。

## 11. 权限和安全

权限规则：

- `JOIN`：第一版只允许 `OWNER` 添加 `ADMIN / MEMBER`，允许 `ADMIN` 添加普通 `MEMBER`；不支持通过 `JOIN` 创建新 `OWNER`，OWNER 转移后续用专用流程。
- `LEAVE`：只允许用户本人触发；管理员代办退群在第一版使用 `REMOVE`，避免 `LEAVE` 同时表达本人退出和管理移除。
- `REMOVE`：`OWNER` 可以移除 `ADMIN / MEMBER`，不能移除另一个 `OWNER`；`ADMIN` 只能移除普通 `MEMBER`，不能移除 `ADMIN / OWNER`。第一版 `REMOVE` 写入状态为 `LEFT`，表示踢出但后续可再加入；永久封禁后续用 `BANNED` 或独立 ban 流程。
- `ROLE_CHANGED`：第一版只允许 `OWNER` 在 `ADMIN / MEMBER` 之间调整角色；不允许把任何人改成 `OWNER`，也不允许调整已有 `OWNER`。

Owner transfer v0.1 采用专用流程，不复用 `ROLE_CHANGED`：

- API 使用独立 `TransferConversationOwner` RPC，避免把“双成员角色变更”伪装成单目标 `CreateMemberChange`。
- 第一版只允许当前 `OWNER` 主动把所有权转给当前 `ACTIVE` 的 `ADMIN / MEMBER`；不允许转给非成员、`LEFT/BANNED` 成员、自己或已有 `OWNER`。
- 转移成功后，新 owner 变为 `OWNER`，原 owner 保留在会话内并降级为 `ADMIN`。后续如需 “transfer and leave” 或 owner 多人制，必须另写 SDD，不塞进 v0.1。
- 事务必须在同一个 conversation 本地事务中完成：锁 conversation、锁当前 owner 和目标成员、校验 `expected_member_version`、分配一个 `conversation_seq`、更新两条 `conversation_members`、只推进一次 `member_version / permission_version`、写一条 timeline/outbox 事件。
- Kafka 事件使用专用 `conversation.member.owner_transferred.v1`，payload 必须同时包含 `previous_owner_user_id`、`new_owner_user_id`、`previous_owner_new_role`、`new_owner_old_role`、`new_owner_new_role`、`member_version`、`permission_version`、`reason` 和 `occurred_at`。
- `member_change_saga` 可以通过 expand-only migration 增加 `OWNER_TRANSFER` change_type，并把 `user_id` 解释为 new owner；previous owner 信息必须进入 `metadata_json` 和 event payload。不要新增独立 `owner_transfer_saga`，除非实现时证明复用会显著增加复杂度。
- 下游 projection 必须显式支持该事件：delivery membership projection 至少要把新 owner / 原 owner 的 role 更新到最新；unsupported owner transfer event 必须 fail-closed，不得被误标记为 published / committed。
- 实现必须分阶段：先冻结 proto/schema/migration/relay builder，再做 repository/usecase/RPC，最后跑 owner transfer roster smoke。不要在一个提交里同时完成所有生产 hardening。

安全要求：

- API 层只接受 authenticated operator context，不信任 request 里的裸 operator 字段。
- 第一版如果 operator 来自 request，必须在 SDD / runbook 标记为本地开发模式，生产前接入 identity/policy。
- 所有冲突、拒绝、补偿和人工 repair 必须进入 audit。
- 对外错误 message 使用稳定文案，不暴露 SQL、constraint、内部状态机细节。
- OpenSearch / Milvus 中的 ACL 字段只作为加速投影，不是最终授权事实。
- 用户退群后，`leave_seq` 之后的消息和 RAG chunk 不得通过检索返回；投影不确定时必须走 strict ACL fallback。

## 12. SLO 和指标

目标：

| 指标 | 目标 |
| --- | --- |
| `CreateMemberChange` success p95 | 小规模 smoke 下 `< 50ms` |
| `CreateMemberChange` error rate | 正常 smoke 为 `0` |
| saga stuck age | `PENDING_*` 超过 60s 必须告警 |
| outbox pending for member events | smoke 结束后应可 drain 到 0 |
| ACL projection lag | 超过阈值进入 strict ACL mode |
| ACL tuple checksum mismatch | 必须触发回源校验和修复告警 |

必须打点：

```text
member_change_request_latency_ms
member_change_tx_latency_ms
member_change_boundary_alloc_latency_ms
member_change_saga_state_count
member_change_conflict_count
member_change_outbox_pending_count
member_change_dlq_count
member_change_retry_count
acl_projection_lag_seconds
acl_projection_checksum_mismatch_count
```

## 13. 测试方案

| 测试 | 目标 |
| --- | --- |
| domain unit | 状态机、角色规则、冲突策略、幂等 replay |
| app unit | policy denied、version conflict、dependency retry、不进入 repository |
| repository integration | 同事务更新 saga/member/version/timeline/outbox |
| concurrency integration | 同一 user 串行、同一 idempotency replay、不同 user 可并发 |
| contract | gRPC code / stable message / proto generated |
| relay integration | member boundary outbox 可发布 Kafka，DLQ 可观察 |
| smoke | `CreateMemberChange -> GetSendContext -> SendMessage` 串联 |
| roster smoke | `JOIN -> ListConversationMembers includes target`；`LEAVE / REMOVE -> ListConversationMembers excludes target`；`ROLE_CHANGED -> ListConversationMembers returns updated role` |
| owner transfer contract | `OWNER_TRANSFER` 不复用 `ROLE_CHANGED`；proto / Kafka schema / relay builder / delivery projection 明确支持 `conversation.member.owner_transferred.v1` |
| owner transfer transaction | 一个 transfer 只分配一个 seq、一条 saga、一条 timeline、一条 outbox；两条 member row 同事务更新；conversation version 只递增一次 |
| owner transfer smoke | `TransferConversationOwner -> ListConversationMembers` 返回新 owner 为 `OWNER`、旧 owner 为 `ADMIN`、owner 数量为 1；outbox drain 到 0 |

第一轮 smoke 不做大规模压测，只验证：

```text
JOIN user
-> GetSendContext returns ACTIVE
-> SendMessage succeeds
LEAVE user
-> GetSendContext returns PermissionDenied
-> ListConversationMembers no longer returns left target
REMOVE user
-> ListConversationMembers no longer returns removed target
ROLE_CHANGED user MEMBER -> ADMIN
-> ListConversationMembers returns ADMIN role
OWNER_TRANSFER owner -> active member
-> ListConversationMembers returns target OWNER and previous owner ADMIN
```

## 14. Runbook

编码后需要新增：

```text
docs/runbook/conversation-service-member-change-local.md
docs/runbook/loadtest/conversation-service/loadtest-report-YYYYMMDD-member-change-smoke.md
docs/runbook/loadtest/conversation-service/loadtest-report-YYYYMMDD-list-conversation-members-leave-smoke.md
docs/runbook/loadtest/conversation-service/loadtest-report-YYYYMMDD-list-conversation-members-remove-smoke.md
docs/runbook/loadtest/conversation-service/loadtest-report-YYYYMMDD-list-conversation-members-role-smoke.md
docs/runbook/loadtest/conversation-service/loadtest-report-YYYYMMDD-owner-transfer-smoke.md
```

Runbook 必须覆盖：

- migration；
- seed 会话；
- 启动 conversation-service；
- 执行 `CreateMemberChange`；
- 查询 `member_change_saga`；
- 查询 `conversation_members`；
- 查询 timeline / outbox；
- outbox relay drain；
- DLQ / retry / repair；
- 清理 smoke 数据。

## 15. 验收标准

进入编码前必须满足：

- 本 SDD Frozen。
- `conversation_service.proto` 增加成员变更 RPC。
- `schemas/kafka/conversation.timeline.events.proto` 增加 member boundary oneof payload。
- outbox relay builder 已支持 `conversation.member.*`，并覆盖 unsupported event fail-closed：不崩进程、不误标记 `PUBLISHED`、继续按同 conversation 顺序阻塞，除非受控 repair/audit 显式 skip。
- migration 补齐 saga retry / DLQ / metadata / `outbox_event_id` / `timeline_event_id` 字段或明确不需要。
- migration / 文档明确 `conversation_seq`、`conversation_timeline_events`、`message_outbox` 是 conversation timeline shared store。
- 明确 timeline append / outbox append port，不在 API/usecase 里拼 SQL。
- 单元测试覆盖角色规则和状态机。
- 集成测试覆盖同事务 saga/member/timeline/outbox。
- 本地 smoke 报告归档到 `docs/runbook/loadtest/conversation-service/`。

不满足这些条件前，不实现真实成员变更生产代码。
