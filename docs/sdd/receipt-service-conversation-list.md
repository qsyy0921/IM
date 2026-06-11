# NexusIM receipt-service Conversation List / Unread SDD v0.1 Draft

状态：Draft。本文定义第三层 IM 产品能力中的“会话列表 / 未读数”最小切片。该能力作为 `receipt-service` 的扩展实现，不新增独立 `conversation-list-service`，以降低服务数量、跨服务依赖和维护复杂度。

设计备注：独立 `conversation-list-service` 在职责上也成立，但目标架构已冻结服务集合且当前阶段要求控制复杂度。v0.1 先在 receipt-service 内实现为 projection 模块；只有当列表流量、团队边界或独立扩缩容需求足够明确时，再按服务拆分重构。

## 1. 服务定位

会话列表 / 未读数是用户侧 read model，基于已有投递和回执事件构建：

```text
im.delivery.events
-> receipt-service projection
-> user_conversation_summaries
-> ListConversations
```

职责：

- 为用户生成会话摘要：最近可见消息 seq、message_id、sender_id、更新时间。
- 维护未读数：基于用户可见消息和 `MarkRead` 推进的 read cursor 计算。
- 提供最小 `ListConversations` 查询接口，供客户端首页展示。
- 保持与 `delivery-service user_inbox` 和 `message-service message_log` 解耦，不直接读取其它服务内部表。

不负责：

- 不拥有消息事实，不修改 `message_log`。
- 不拥有会话成员事实，不修改 `conversation_members`。
- 不推进 delivery ACK；设备收到仍由 `delivery-service AckDelivery` 负责。
- 不做 WebSocket 在线推送；push-gateway 仍只做在线唤醒。
- 第一阶段先做最小 archive/unarchive 列表过滤偏好；随后已补最小 pin/unpin 排序偏好和 mute/unmute 静音标志。不做草稿、会话头像或消息 preview 富文本渲染。

## 2. 为什么放在 receipt-service

目标架构已冻结服务集合，不继续为每个 read model 新增微服务。会话列表 / 未读数与 receipt-service 已有状态高度相关：

- `receipt_inbox_projection` 已有用户可见消息索引。
- `user_read_cursors` 已有用户维度已读游标。
- `receipt_outbox` 已发布 `receipt.message.read.v1`，后续可驱动摘要更新或审计。

因此第一阶段把会话摘要 projection 放在 receipt-service 内部，复用当前事件消费和权限边界。后续如果会话列表成为独立高流量域，可以再按“服务拆分”重构，但不能在当前阶段提前引入网状依赖。

## 3. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游事件 | Kafka `im.delivery.events` | 消费 `delivery.inbox_item.created.v1`，按 `source_event_type` 更新用户会话摘要和 unread |
| 上游命令 | `receipt-service MarkRead` | 推进 read cursor 后更新 unread |
| 同步上游 | API gateway / client | 调用 `ListConversations` |
| 同步依赖 | conversation / policy access port | 校验用户可见会话范围；第一阶段可复用 `ReceiptAccessPort` |
| 下游 | 客户端 / push-gateway | 客户端列表展示；push-gateway 只作为唤醒，不承载列表事实 |

## 4. 六层 DDD 包结构

不新增服务目录，沿用：

```text
services/receipt-service/
  internal/
    api/
    app/
    domain/
    infrastructure/
    types/
    trigger/
```

| 层 | 本切片内容 |
| --- | --- |
| `api` | gRPC `ListConversations` adapter |
| `app` | `ListConversationsUseCase`，接收 AuthContext 和分页参数 |
| `domain` | unread count、排序游标、摘要更新规则 |
| `infrastructure` | PostgreSQL summary repository，复用 delivery event projection 事务 |
| `types` | command / result / summary DTO / 错误 sentinel |
| `trigger` | 继续使用 `delivery-consumer`；不新增独立 consumer |

## 5. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `ConversationSummary` | 某用户某会话的列表项 | `(tenant_id, user_id, conversation_id)` 唯一 |
| `LastVisibleMessage` | 用户可见的最新消息索引 | 只能由 `delivery.inbox_item.created.v1` 推进 |
| `UnreadState` | 未读计数和 read cursor 派生状态 | `unread_count >= 0`，`read_seq <= last_visible_seq` |
| `ConversationArchivePreference` | 当前用户是否在默认列表隐藏该会话 | 只影响该用户 `ListConversations` 过滤，不影响 unread、delivery、push 或消息事实 |
| `ConversationPinPreference` | 当前用户是否置顶该会话 | 只影响该用户默认列表排序，不影响 unread、delivery、push 或消息事实 |
| `ConversationMutePreference` | 当前用户是否静音该会话 | v0.1 只作为列表 / 通知策略偏好返回，不改变 unread、delivery、push 或消息事实 |
| `ListCursor` | 分页游标 | 基于 `pinned + sort_updated_at + conversation_id` 或 `sort_updated_at + conversation_id`，避免 offset 深分页 |
| `ProjectionWatermark` | 投影水位 | 表示 summary 至少处理到的 Kafka offset 或本地更新时间 |

第一阶段不追求完整消息 preview。列表项只返回 `last_message_id`、`last_message_seq`、`last_sender_id`、`last_source_event_type` 和 `updated_at`。客户端可以用 `last_source_event_type` 区分新增 / 编辑 / 撤回 / 删除造成的列表刷新，但需要消息正文或 tombstone 详情时仍回源 `PullInbox` 或后续 message query API。

## 6. 同步 API 契约

建议在 `api/proto/nexusim/receipt/v1/receipt_service.proto` 增加：

```text
rpc ListConversations(ListConversationsRequest) returns (ListConversationsResponse)
rpc ArchiveConversation(ArchiveConversationRequest) returns (ArchiveConversationResponse)
rpc PinConversation(PinConversationRequest) returns (PinConversationResponse)
rpc MuteConversation(MuteConversationRequest) returns (MuteConversationResponse)
```

请求字段：

```text
AuthContext auth_context
int32 limit
string page_cursor
ConversationListSort sort
bool include_archived
```

`ConversationListSort` 第一阶段只支持：

```text
CONVERSATION_LIST_SORT_UNSPECIFIED
CONVERSATION_LIST_SORT_UPDATED_AT_DESC
CONVERSATION_LIST_SORT_PINNED_UPDATED_AT_DESC
```

`UNSPECIFIED` 等价于 `PINNED_UPDATED_AT_DESC`。`PINNED_UPDATED_AT_DESC` 按 `pinned desc + updated_at desc + conversation_id asc` 排序；`UPDATED_AT_DESC` 保留纯 `updated_at desc + conversation_id asc` 排序。不要在第一阶段提前加入 unread-first 或自定义排序组合；这些产品字段需要独立设计和索引支持。`muted` 是返回给客户端的偏好标志，不参与 v0.1 排序。`include_archived=false` 是默认列表；`include_archived=true` 用于归档管理视图。

响应字段：

```text
repeated ConversationSummary items
string next_page_cursor
ProjectionWatermark projection_watermark
```

`ConversationSummary`：

```text
string conversation_id
int64 last_visible_seq
string last_message_id
string last_sender_id
string last_source_event_type
int64 unread_count
int64 last_read_seq
int64 updated_at_unix_ms
bool archived
bool pinned
bool muted
```

`ArchiveConversationRequest`：

```text
AuthContext auth_context
string conversation_id
bool archived
```

语义：

- 只修改当前 `auth_context.user_id` 自己的会话摘要行；不修改 message、delivery、conversation 成员事实。
- `archived=true` 后默认 `ListConversations` 不返回该会话；`include_archived=true` 仍可查到。
- `archived=false` 恢复默认列表可见。
- 新消息投影不会自动取消归档；v0.1 将 archive 定义为用户显式列表过滤偏好，不是一次性“清理当前列表”动作。
- 如果本服务尚未投影出该用户的会话摘要，返回 `CONVERSATION_NOT_FOUND`。

`PinConversationRequest`：

```text
AuthContext auth_context
string conversation_id
bool pinned
```

语义：

- 只修改当前 `auth_context.user_id` 自己的会话摘要行；不修改 message、delivery、conversation 成员事实。
- `pinned=true` 后默认 `ListConversations` 会把该会话排在非置顶会话之前，同一组内仍按 `updated_at desc + conversation_id asc` 排序。
- `pinned=false` 恢复普通排序。
- pin/unpin 不改变 unread、last_visible_seq、last_source_event_type 或 push 通知。
- 如果本服务尚未投影出该用户的会话摘要，返回 `CONVERSATION_NOT_FOUND`。

`MuteConversationRequest`：

```text
AuthContext auth_context
string conversation_id
bool muted
```

语义：

- 只修改当前 `auth_context.user_id` 自己的会话摘要行；不修改 message、delivery、conversation 成员事实。
- `muted=true` 后 `ListConversations` 返回 `muted=true`，客户端可用于列表 UI 或后续通知策略。
- `muted=false` 恢复普通状态。
- mute/unmute 不改变 unread、last_read_seq、last_visible_seq、last_source_event_type 或 push 通知。
- v0.1 不把 mute 实现为真正 push suppression；后续若要抑制在线通知，应在 push policy / consumer 侧读取偏好或投影，不应改写 durable delivery 事实。
- 如果本服务尚未投影出该用户的会话摘要，返回 `CONVERSATION_NOT_FOUND`。

分页规则：

- v0.1 默认使用 `pinned DESC, sort_updated_at DESC, conversation_id ASC`；显式 `UPDATED_AT_DESC` 使用 `sort_updated_at DESC, conversation_id ASC`。
- `page_cursor` 是服务端签发的 opaque cursor，内部绑定 cursor version、sort、`include_archived`、`pinned`、`sort_updated_at` 和 `conversation_id`。
- 客户端不能自行构造 cursor；cursor 缺字段、版本不匹配、sort 不匹配或 `include_archived` 不匹配时返回 `INVALID_ARGUMENT`。
- 后续新增排序时，必须同步 cursor schema、SQL 条件和索引，不能复用旧 cursor 语义。

`ProjectionWatermark`：

```text
string source
int64 offset_value
int64 updated_at_unix_ms
```

该字段只用于客户端和排障判断“列表投影可能滞后”，不能作为消息事实或权限事实。

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | limit/page_cursor/auth 参数错误 | 否 |
| `PERMISSION_DENIED` | 用户无权列出该租户会话 | 否 |
| `CONVERSATION_NOT_FOUND` | 归档 / 置顶目标会话摘要尚未投影到当前用户 | 否 |
| `DB_READ_FAILED` | 数据库读取失败 | 是 |

## 7. 异步事件契约

第一阶段不新增 Kafka topic，也不新增 outbox 事件。会话列表是 receipt-service 内部 projection。

触发来源：

| 来源 | 处理 |
| --- | --- |
| `delivery.inbox_item.created.v1` | upsert `user_conversation_summaries`，推进 last visible，并按当前 read cursor 计算 unread；只有 `source_event_type=message.persisted.v1` 计入 unread |
| `MarkRead` 事务 | 推进 `last_read_seq`，重算当前会话 unread |

后续如需要客户端在线列表变更提示，可由 receipt-service 通过 `receipt_outbox` 或新 summary outbox 发布轻量事件，但第一阶段不做。

## 8. 数据库设计

新增 receipt-service 自有表：

```sql
CREATE TABLE user_conversation_summaries (
    tenant_id         TEXT NOT NULL,
    user_id           TEXT NOT NULL,
    conversation_id   TEXT NOT NULL,
    last_visible_seq  BIGINT NOT NULL,
    last_message_id   TEXT NOT NULL,
    last_sender_id    TEXT NOT NULL,
    last_source_event_type TEXT NOT NULL DEFAULT 'message.persisted.v1',
    last_read_seq     BIGINT NOT NULL DEFAULT 0,
    unread_count      BIGINT NOT NULL DEFAULT 0,
    sort_updated_at   TIMESTAMPTZ NOT NULL,
    archived          BOOLEAN NOT NULL DEFAULT FALSE,
    archived_at       TIMESTAMPTZ,
    pinned            BOOLEAN NOT NULL DEFAULT FALSE,
    pinned_at         TIMESTAMPTZ,
    muted             BOOLEAN NOT NULL DEFAULT FALSE,
    muted_at          TIMESTAMPTZ,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, conversation_id)
);

CREATE INDEX idx_user_conversation_summaries_list
ON user_conversation_summaries (tenant_id, user_id, archived, sort_updated_at DESC, conversation_id);

CREATE INDEX idx_user_conversation_summaries_list_pinned
ON user_conversation_summaries (tenant_id, user_id, archived, pinned DESC, sort_updated_at DESC, conversation_id);

CREATE TABLE conversation_summary_checkpoints (
    consumer_group TEXT NOT NULL,
    topic          TEXT NOT NULL,
    partition_id   INT NOT NULL,
    offset_value   BIGINT NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_group, topic, partition_id)
);
```

第一阶段 unread 计算可以直接使用：

```text
unread_count = count(receipt_inbox_projection where source_event_type = message.persisted.v1 and conversation_seq > last_read_seq)
```

为了控制复杂度，写路径可以先在 projection / MarkRead 事务内针对单会话重算 unread，而不是引入 Redis counter 或异步批处理。Kafka checkpoint 语义与 delivery / receipt 保持一致：`offset_value` 表示 next offset to commit，且只能在 PostgreSQL 事务成功后推进。

`message.edited.v1`、`message.revoked.v1`、`message.deleted.v1` 仍然可以推进 `last_visible_seq` 和列表排序，让客户端知道会话有状态变化；但第一阶段不把这些变更事件当成新未读消息，也不为它们创建新的 read receipt state。这样可以避免用户已读原消息后，因为编辑 / 撤回 / 删除 tombstone 又出现未读数回升。

## 9. 核心流程

### 9.1 消息进入 inbox

```text
delivery.inbox_item.created.v1
-> receipt-service delivery consumer
-> upsert receipt_inbox_projection
-> upsert user_conversation_summaries
-> commit receipt_kafka_checkpoints
```

### 9.2 用户 MarkRead

```text
MarkRead
-> validate access
-> lock user_read_cursors
-> update user_read_cursors
-> update message_receipt_states
-> insert receipt_outbox read event
-> update user_conversation_summaries.last_read_seq / unread_count
```

### 9.3 客户端拉会话列表

```text
ListConversations
-> validate auth
-> query user_conversation_summaries by tenant/user
-> return cursor page
```

## 10. 一致性和事务

强一致边界：

- `receipt_inbox_projection` 与 `user_conversation_summaries` 在同一 delivery event projection 事务中更新。
- `user_read_cursors`、`message_receipt_states`、`receipt_outbox` 和 `user_conversation_summaries` 在同一 `MarkRead` 事务中更新。
- `conversation_summary_checkpoints` 与 summary 更新在同一事务内推进；Kafka offset 只能在事务提交后 commit。

最终一致边界：

- message-service 写入到 delivery-service projection 是 Kafka 最终一致。
- delivery-service 到 receipt-service projection 是 Kafka 最终一致。
- 客户端收到 push notify 后仍以 `PullInbox` 和 `ListConversations` 的最终投影为准。

## 11. 幂等、重试和补偿

| 场景 | 幂等键 | 策略 |
| --- | --- | --- |
| 重复 `delivery.inbox_item.created.v1` | `tenant_id + user_id + conversation_id + conversation_seq` | `receipt_inbox_projection` upsert，summary 使用最大 seq |
| 重复 `MarkRead` | `tenant_id + user_id + conversation_id + read_seq` | `read_seq <= current` 视为幂等成功 |
| 重复 `ArchiveConversation` | `tenant_id + user_id + conversation_id` | 幂等设置 `archived`，不改变 unread 和 last visible |
| 重复 `PinConversation` | `tenant_id + user_id + conversation_id` | 幂等设置 `pinned`，不改变 unread 和 last visible |
| 重复 `MuteConversation` | `tenant_id + user_id + conversation_id` | 幂等设置 `muted`，不改变 unread、read cursor 和 last visible |
| projection lag | Kafka offset 不提交 | fail-closed，等待重放 |
| malformed / unsupported event | `event_id` | 不静默 commit；后续 repair/backfill 处理 |
| summary 损坏 | tenant/user/conversation | 后续 repair 从 `receipt_inbox_projection` 和 `user_read_cursors` 重建 |

## 12. 权限和安全

- `AuthContext` 的 `tenant_id/user_id/device_id` 必须来自 gateway / identity，不能信任裸请求参数。
- `ListConversations` 只返回当前用户自己的会话摘要，不接受请求体指定其它 user_id。
- 第一阶段可以只按 `tenant_id + auth.user_id` 查询本服务 projection；后续接入 policy-service 后，需要过滤无权访问或已退出窗口之外的会话。
- 摘要不返回消息正文，避免绕过 message visibility / revoke / delete 规则。
- 无权限不能靠摘要表泄露最后消息。未来撤回 / 删除上线后，summary 必须按 message visibility 更新或隐藏对应字段。

## 13. SLO 和指标

| 指标 | 目标 |
| --- | --- |
| `ListConversations` p95 | `< 50ms` 本地小规模 |
| projection lag | 可观测，不作为第一阶段容量目标 |
| unread negative count | 必须为 0 |

建议指标：

```text
conversation_summary_upsert_count
conversation_summary_recompute_count
conversation_summary_unread_count
list_conversations_latency_ms
list_conversations_page_size
```

## 14. 测试方案

| 测试 | 目标 |
| --- | --- |
| unit | unread 计算、sort 校验、cursor 分页、read_seq 幂等 |
| PostgreSQL integration | inbox event upsert summary、MarkRead 清零 unread、重复 event 不重复计数、edit/revoke/delete 不增加 unread、多会话 keyset 分页、非法 cursor、archive 默认隐藏 / include_archived 可见 / 新投影保留 archive、pinned-first 排序、显式 updated sort、跨 pinned/unpinned 分页、mute/unmute 不改变 unread/read cursor |
| gRPC contract | 参数校验、sort/page_cursor/include_archived 映射、archive/pin/mute request/response、分页响应、只返回当前 auth user |
| smoke | `SendMessage -> PullInbox -> ListConversations(unread=1) -> MarkRead -> ListConversations(unread=0)` |

## 15. 当前不做

- 不新增独立 `conversation-list-service`。
- 不直接读取 `delivery-service.user_inbox` 或 `message-service.message_log`。
- 不返回 message payload / preview 富文本。
- 不做 draft；archive、pin 和 mute v0.1 只作为当前用户的列表偏好，不影响通知或 unread。mute 当前不等于真正 push suppression。
- 不引入 Redis unread counter。
- 不做大规模压测矩阵。
- 不把 `delivery.ack.recorded.v1` 当成 read；ACK 只说明 received。
- 不做跨 topic 全局严格排序；跨 conversation 只承诺分页排序稳定。

## 16. 验收标准

- SDD 经过评审，无 P0/P1。
- Proto 增量兼容，生成代码入库。
- migration expand-only。
- `go test ./services/receipt-service/...` 通过。
- PostgreSQL integration 覆盖 unread 从 1 到 0。
- 真实进程 smoke 归档到 `docs/runbook/loadtest/receipt-service/`。
