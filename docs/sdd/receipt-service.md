# NexusIM receipt-service SDD v0.1 Draft

状态：Draft，proto / Kafka schema / migration / 六层骨架已落草案，等待 repository / consumer 实现前复核。

本文定义 `receipt-service` 的第一条可编码切片：基于 `delivery-service` 已经产生的 durable delivery 事件，构建消息送达 / 已读回执 read model，并提供最小查询和 `MarkRead` 写入入口。

## 1. 服务定位

`receipt-service` 拥有消息回执 read model：

- `receipt_inbox_projection`：从 `delivery.inbox_item.created.v1` 重建用户可见消息索引，只保存回执所需字段，包括 `sender_id`。
- `device_received_cursors`：从 `delivery.ack.recorded.v1` 投影每个设备的已接收游标。
- `user_read_cursors`：用户维度的已读游标；同一用户多设备读到更高 seq 后单调推进。
- `message_receipt_states`：按 message / conversation seq 聚合用户送达和已读状态。
- `receipt_outbox`：后续向 sender、audit、会话列表或未读数投影发布回执变更事件。

职责：

- 消费 `im.delivery.events`。
- 将 `delivery.inbox_item.created.v1` 投影为回执所需的用户可见消息索引。
- 将 `delivery.ack.recorded.v1` 转换为“设备已接收 / user received”状态。
- 提供 `MarkRead`，由客户端显式报告用户已读到某个 conversation seq。
- 提供 `GetReceiptState`，查询某条消息或某个 seq 的送达 / 已读聚合；批量 `ListReceiptStates` 是后续范围。
- 通过 outbox 发布 receipt event，供 audit、会话列表或未读数投影消费；push-gateway 的在线回执通知需要单独 SDD 和 consumer，不属于第一阶段。

不负责：

- 不修改 `message_log`、`conversation_timeline_events`、`message_outbox`。
- 不修改 `user_inbox`、`device_delivery_cursors`；这些仍归 `delivery-service`。
- 不拥有成员事实，不替代 `conversation-service`；但必须通过 `ReceiptAccessPort` 查询当前用户是否可上报 / 查看指定会话回执。
- 不直接 WebSocket 推送；在线回执通知需要后续扩展 `push-gateway` receipt consumer，本阶段不做。
- 不做 RAG / Agent；RAG 只能在回执、撤回/删除和 ACL 语义更稳定后进入。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游事件 | Kafka `im.delivery.events` | 消费 `delivery.inbox_item.created.v1` 和 `delivery.ack.recorded.v1` |
| 同步上游 | API gateway / client | 调用 `MarkRead`、`GetReceiptState` |
| 同步依赖 | PostgreSQL | 写回执 projection、cursor、outbox、checkpoint |
| 同步依赖 | conversation / policy access port | 校验 `MarkRead` 和 `GetReceiptState` 的会话访问权限、隐私模式和版本 |
| 异步下游 | Kafka `im.receipt.events` | 发布 receipt received/read event |
| 下游 | conversation-list projection / audit | 会话摘要、审计；push-gateway 在线回执提示后置 |

`receipt-service` 不读取 `delivery-service` 内部表。它通过 `im.delivery.events` 重建必要投影；如果 projection lag 或数据缺失，API 必须返回可解释错误或要求客户端回源 `PullInbox`，不能直接跨服务读内部表补洞。

权限来源必须显式化：app 层通过 `ReceiptAccessPort` 调用 conversation / policy 能力，第一阶段可以用本地 mock，但接口必须表达 `CanMarkRead` 和 `CanViewReceiptState` 两种语义。该端口返回 visibility mode、permission version 和必要的 membership window；无权限返回 `PERMISSION_DENIED`。不能用 receipt projection 的存在性替代权限判断。

## 3. 六层 DDD 包结构

```text
services/receipt-service/
  cmd/receipt-service
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
| `api` | gRPC handler：`MarkRead`、`GetReceiptState`、错误码映射 |
| `app` | `ProjectDeliveryEventUseCase`、`MarkReadUseCase`、`GetReceiptStateUseCase`、`ReceiptAccessPort` |
| `domain` | received/read cursor 单调性、message receipt 聚合、权限窗口校验 |
| `infrastructure` | PostgreSQL repository、Kafka delivery consumer、receipt outbox relay |
| `types` | Command、DTO、错误 sentinel、枚举 |
| `trigger` | Kafka consumer worker、receipt outbox relay、repair worker |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `ReceiptInboxItem` | 回执服务自己的可见消息索引 | 来自 `delivery.inbox_item.created.v1`，`(tenant_id,user_id,conversation_id,conversation_seq)` 唯一，必须保存 `sender_id` |
| `DeviceReceivedCursor` | 某设备已接收 / 已持久化游标 | 只由 `delivery.ack.recorded.v1` 推进，单调递增 |
| `UserReceivedCursor` | 用户维度已接收游标 | 可由多个设备 received cursor 聚合；第一阶段取该用户任一设备 ack 到的最大 seq |
| `UserReadCursor` | 用户维度已读游标 | 只由 `MarkRead` 推进，不能超过该用户已接收 / 已可见最大 seq |
| `MessageReceiptState` | 某条消息的回执状态 | sender 自己不计入“对方已读”；成员可见性以 delivery event 投影为准 |
| `ReceiptOutboxEvent` | 回执变更事件 | 必须与 cursor / state 更新在同一 PostgreSQL 事务内写入 |

第一版优先使用 cursor 模型表达 received / read。不要一开始就为每条消息和每个成员生成无限增长的强一致明细状态；`message_receipt_states` 只作为查询和 smoke 的可解释投影，后续可按会话规模、群规模和产品展示方式改成按 cursor 计算或按区间聚合。

## 5. 回执语义

### 5.1 送达 / received

`AckDelivery` 已经表示客户端设备确认 `delivery-service` 的 inbox item 已被本地接收或持久化。因此：

```text
delivery.ack.recorded.v1
-> receipt-service ProjectDeliveryEvent
-> device_received_cursors
-> user_received_cursors
-> message_receipt_states.received
```

第一阶段 `received` 是“至少一个设备已 ACK 到该 seq”。如果后续产品要求“所有在线设备均收到”或“每设备送达状态”，需要扩展查询模型，但不能改变第一阶段事件含义。

### 5.2 已读 / read

`read` 必须由客户端显式动作触发，不能把 `AckDelivery` 自动等同于已读。

第一阶段提供：

```text
MarkRead(conversation_id, read_seq)
```

规则：

- `read_seq` 必须大于当前 `user_read_cursors.last_read_seq`，低于或等于当前值视为幂等成功。
- `read_seq` 不能超过该用户在 `receipt_inbox_projection` 中已可见的最大 seq。
- 更保守地，`read_seq` 不能超过 `user_received_cursors.last_received_seq`；如果 projection lag 导致 received cursor 不足，返回 retryable dependency / projection lag 错误。
- read cursor 是用户维度，不是设备维度；同一用户任一设备读到更高 seq 后，用户级 read cursor 单调推进。
- sender 自己发送的消息默认不产生“自己已读”对外回执，避免会话列表误读。

### 5.3 隐私和可见性

第一阶段默认启用 read receipt，但必须在 SDD 层预留隐私和产品策略钩子：

- 用户可关闭“向他人展示已读”，关闭后仍可本地推进 read cursor，但对外 `GetReceiptState` 不展示该用户 read 明细。
- 群聊可以按规模降级：小群展示用户列表，大群只展示计数或摘要。
- sender 只能查看自己有权访问的 conversation / message 的回执；被移出会话后，不能查看离开后消息的回执。sender 判断来自 `delivery.inbox_item.created.v1.sender_id`，不能回读 message-service 内部表。
- message 撤回 / 删除上线后，回执查询必须遵守消息可见性，不得因为 receipt projection 保留历史状态而泄露被删除消息。
- receipt-service 可以保留 audit 级内部状态，但对普通 API 返回必须经过权限和隐私过滤。

## 6. 同步 API 契约

契约文件规划：

```text
api/proto/nexusim/receipt/v1/receipt_service.proto
```

第一阶段 RPC：

```text
rpc MarkRead(MarkReadRequest) returns (MarkReadResponse)
rpc GetReceiptState(GetReceiptStateRequest) returns (GetReceiptStateResponse)
```

`MarkReadRequest`：

```text
auth_context {
  tenant_id
  user_id
  device_id
  session_id
  request_id
  trace_id
}
conversation_id
read_seq
```

`MarkReadResponse`：

```text
tenant_id
user_id
conversation_id
last_read_seq
```

`GetReceiptStateRequest` 第一阶段建议支持按 message_id 或 conversation seq 查询：

```text
auth_context
conversation_id
message_id
conversation_seq
```

`GetReceiptStateResponse`：

```text
conversation_id
conversation_seq
message_id
received_user_count
read_user_count
visibility_mode
receivers[] {
  user_id
  received_seq
  received_at_unix_ms
  read_seq
  read_at_unix_ms
}
```

错误码：

| 错误码 | gRPC code | 语义 | 是否可重试 |
| --- | --- | --- | --- |
| `INVALID_ARGUMENT` | `InvalidArgument` | 参数缺失、seq 非法、message_id / seq 二选一不合法 | 否 |
| `PERMISSION_DENIED` | `PermissionDenied` | 当前用户无权读取或上报该 conversation 的回执 | 否 |
| `READ_OUT_OF_VISIBLE_RANGE` | `FailedPrecondition` | `read_seq` 大于用户可见最大 seq | 否 |
| `READ_OUT_OF_RECEIVED_RANGE` | `FailedPrecondition` | `read_seq` 大于用户已接收 seq | 是 |
| `RECEIPT_NOT_FOUND` | `NotFound` | 查询的 message / seq 尚无回执投影 | 否 |
| `PROJECTION_LAGGING` | `Unavailable` | delivery event 尚未投影完成 | 是 |
| `DB_READ_FAILED` | `Unavailable` | 读取回执失败 | 是 |
| `DB_WRITE_FAILED` | `Unavailable` | 写 cursor / outbox 失败 | 是 |
| `SERVICE_OVERLOADED` | `Unavailable` | 服务主动过载保护 | 是 |

## 7. 异步事件契约

### 7.1 消费事件

Topic：

```text
im.delivery.events
```

| 事件 | 处理 |
| --- | --- |
| `delivery.inbox_item.created.v1` | 写 `receipt_inbox_projection`，建立 `(user, conversation_seq) -> message_id/source_event_id` 映射 |
| `delivery.ack.recorded.v1` | 推进 `device_received_cursors`，再推进 user received 聚合和 message received 状态 |

消费规则：

- Kafka offset 只在 PostgreSQL 事务提交后推进。
- receipt-service 是可靠 projection，新的 consumer group 必须从 topic earliest / `FirstOffset` 开始构建 read model；不能照抄 push-gateway 的 latest offset 策略。
- 如果 topic retention 不足以重建 projection，本阶段直接返回 `PROJECTION_LAGGING` / 运维告警；后续再设计受控 backfill，不允许通过读取 delivery-service 内部表临时补洞。
- 消费 `delivery.ack.recorded.v1` 时，如果对应 inbox projection 尚未到达，必须 fail-closed 或进入可重试 lag 状态，不能丢弃 ack。
- duplicate delivery event 必须幂等：按 `event_id` 或唯一键去重。
- unsupported / malformed event 不得静默 commit；第一阶段可以 fail-closed 阻塞并报警，后续补 projection DLQ / repair。

### 7.2 发布事件

Topic 规划：

```text
im.receipt.events
```

第一阶段事件：

| 事件 | 分区键 | 下游 |
| --- | --- | --- |
| `receipt.message.received.v1` | `tenant_id + conversation_id` | conversation-list projection、audit |
| `receipt.message.read.v1` | `tenant_id + conversation_id` | conversation-list projection、audit |

Payload 建议：

```text
tenant_id
conversation_id
conversation_seq
message_id
user_id
device_id
cursor_seq
occurred_at
source_event_id
```

发布规则：

- `MarkRead` 和 receipt projection 只写 `receipt_outbox`，不得直接 publish Kafka。
- outbox relay 保持 at-least-once，下游按 `event_id` 幂等。
- `receipt.message.read.v1` 优先按 cursor 范围或 seq 区间合并，避免每条消息 x 每个成员的事件爆炸；第一阶段 smoke 可以按单条 message / seq 粒度发布，但报告必须说明这不是大群生产模型。

## 8. 数据库设计草案

Migration 规划：

```text
migrations/postgres/receipt/000001_receipt_core.sql
```

核心表：

```sql
CREATE TABLE receipt_inbox_projection (
    tenant_id           TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    conversation_seq    BIGINT      NOT NULL,
    source_event_id     TEXT        NOT NULL,
    delivery_event_id   TEXT        NOT NULL,
    message_id          TEXT        NOT NULL,
    sender_id           TEXT        NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, conversation_id, conversation_seq),
    UNIQUE (tenant_id, user_id, delivery_event_id)
);

CREATE TABLE device_received_cursors (
    tenant_id           TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    device_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    last_received_seq   BIGINT      NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, device_id, conversation_id)
);

CREATE TABLE user_received_cursors (
    tenant_id           TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    last_received_seq   BIGINT      NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, conversation_id)
);

CREATE TABLE user_read_cursors (
    tenant_id           TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    last_read_seq       BIGINT      NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, conversation_id)
);

CREATE TABLE message_receipt_states (
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    conversation_seq    BIGINT      NOT NULL,
    message_id          TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    received_at         TIMESTAMPTZ,
    read_at             TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, conversation_seq, user_id)
);
```

还需要：

- `receipt_kafka_checkpoints(consumer_group, topic, partition_id, offset_value, updated_at)`。
- `receipt_outbox`，字段对齐现有 `delivery_outbox` envelope：`event_id / event_type / event_version / tenant_id / conversation_id / aggregate_version / partition_key / mapping_version / correlation_id / causation_id / producer / trace_id / payload_json / status / retry_count / next_retry_at / dead_lettered_at / published_at`。

## 9. 第一条可编码切片

当前草案文件已经包括：

- `api/proto/nexusim/receipt/v1/receipt_service.proto`：`MarkRead` 和 `GetReceiptState`。
- `schemas/kafka/receipt/v1/im.receipt.events.proto`：`receipt.message.received.v1` / `receipt.message.read.v1`。
- `migrations/postgres/receipt/000001_receipt_core.sql`。
- `services/receipt-service/internal/{api,app,domain,infrastructure,types,trigger}` 六层骨架。

下一步按下面顺序推进：

1. 实现 `ReceiptAccessPort` 第一版 mock / adapter，冻结 `CanMarkRead` 和 `CanViewReceiptState` 语义。
2. 实现 PostgreSQL repository：消费 `delivery.inbox_item.created.v1` / `delivery.ack.recorded.v1`，建立 received projection。
3. 实现 `MarkReadUseCase`：校验 read_seq 不超过 visible / received 范围，推进 user read cursor，写 receipt outbox。
4. 实现最小 smoke：

```text
SendMessage
-> delivery projection
-> AckDelivery
-> receipt-service projects delivery.ack.recorded
-> MarkRead
-> GetReceiptState shows received/read
```

第一条代码切片可以先只覆盖单会话、小群、单条消息：

- 两个成员可见同一条消息。
- receiver `AckDelivery` 后，receipt projection 显示 received。
- receiver `MarkRead` 后，receipt projection 显示 read。
- sender 查询 `GetReceiptState` 能看到 received/read；无关用户查询返回 `PERMISSION_DENIED`。

## 10. 当前不做

- 不做群已读完整 UI。
- 不做每设备细粒度展示。
- 不做大规模 receipt fanout 压测。
- 不做跨租户聚合报表。
- 不做 RAG / Agent 读取 receipt 状态。
- 不把 `AckDelivery` 自动视为 read。
- 不允许 receipt-service 直接读取 delivery-service 内部表绕过事件投影。
- 不把 first-slice 的 per-message 明细投影表述为大群生产模型。

## 11. 风险和门禁

- `delivery.ack.recorded.v1` 只有 cursor，没有 message_id；receipt-service 必须先投影 `delivery.inbox_item.created.v1` 建立 seq 到 message 的映射。
- 如果 ack event 先于 inbox projection 到达，必须可重试或 fail-closed，不能丢 ack。
- read cursor 必须同时受 visible seq 和 received seq 限制，防止客户端把未投递消息标成已读。
- 未来撤回 / 删除上线后，receipt query 必须遵守 message visibility，不得展示已删除消息的回执详情给无权用户。
- 未来 RAG / search 使用 receipt 信息时，必须先通过 conversation membership / ACL 过滤。
