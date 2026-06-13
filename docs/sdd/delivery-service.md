# NexusIM delivery-service SDD v0.1

状态：Draft

本文定义 `delivery-service` 的第一条可编码切片：消费 `conversation.timeline.events`，构建用户收件箱 `user_inbox`，提供离线补拉和设备 ACK。它让 NexusIM 从“消息和成员事件已发布”继续走到“客户端可以按用户/设备拉取缺失消息”。

## 1. 服务定位

`delivery-service` 拥有 durable delivery read model：

- `user_inbox`：用户维度的可投递事件索引。
- `delivery_membership_projection`：delivery 自己的成员可见性投影，用于 fanout，不是成员事实源。
- `device_delivery_cursors`：设备维度的已收游标。
- `delivery_kafka_checkpoints`：consumer group 维度的 Kafka partition checkpoint。
- delivery projection rebuild job：后续租户级重放任务，不和 Kafka offset 混用。
- delivery outbox：后续通知 `push-gateway` 在线推送。

职责：

- 消费 `conversation.timeline.events`。
- 按 timeline event 固化的 `fanout_mode / fanout_policy_version / permission_version` 生成 `user_inbox`。
- 支持客户端重连后的离线补拉。
- 支持设备 ACK，记录 `last_received_seq`。
- 为 `push-gateway` 提供可重放的投递事件，不直接维护 WebSocket 连接。

不负责：

- 不修改 `message_log`、`conversation_timeline_events`、`message_outbox`。
- 不分配 conversation seq。
- 不决定成员事实，成员边界来自 `conversation-service`。
- 不做 WebSocket 连接管理，在线连接归 `push-gateway`。
- 不做全文检索、RAG、Agent。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游事件 | Kafka `conversation.timeline.events` | 消费 message/member boundary 事件 |
| 同步上游 | `api-gateway` / `push-gateway` | 调用 `PullInbox`、`AckDelivery` |
| 同步依赖 | PostgreSQL | 写 `user_inbox`、cursor、checkpoint |
| 同步依赖 | message read source | 第一阶段通过 message payload / timeline payload 满足展示；后续可批量回源 message-service |
| 异步下游 | Kafka `im.delivery.events` | 发布 inbox item created / delivery ack recorded |
| 下游 | `push-gateway` | 后续消费 delivery event 或调用 delivery-service 查询缺失范围 |

## 3. 六层 DDD 包结构

```text
services/delivery-service/
  cmd/delivery-service
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
| `api` | gRPC handler：`PullInbox`、`AckDelivery`、错误码映射 |
| `app` | `ProjectTimelineEventUseCase`、`PullInboxUseCase`、`AckDeliveryUseCase` |
| `domain` | fanout 决策、inbox item、cursor、ACK 单调性、成员边界可见性 |
| `infrastructure` | PostgreSQL repository、Kafka consumer、delivery outbox publisher |
| `types` | Command、DTO、枚举、错误 sentinel |
| `trigger` | Kafka consumer worker、delivery outbox relay、repair worker |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `InboxItem` | 用户维度可投递事件 | `(tenant_id, user_id, conversation_id, seq)` 唯一 |
| `DeliveryMembership` | 成员可见性投影 | 由 member boundary event 重建，不能作为成员事实源 |
| `DeliveryCursor` | 设备已接收游标 | cursor 只能单调前进 |
| `TimelineProjection` | timeline event 投影进度 | DB 副作用成功后才提交 Kafka offset |
| `FanoutDecision` | 基于 event metadata 的投递策略 | 只能使用事件内固化的 fanout policy，不读取当前策略覆盖历史 |
| `DeliveryOutboxEvent` | 给 push-gateway / audit 的投递事件 | 必须在本地事务内与 inbox/cursor 副作用一起写入 |

## 5. 同步 API 契约

契约文件规划：

```text
api/proto/nexusim/delivery/v1/delivery_service.proto
```

第一阶段 RPC：

```text
rpc PullInbox(PullInboxRequest) returns (PullInboxResponse)
rpc AckDelivery(AckDeliveryRequest) returns (AckDeliveryResponse)
```

`PullInboxRequest`：

```text
auth_context {
  tenant_id
  user_id
  device_id
}
conversation_id
after_seq
limit
request_id
trace_id
```

`PullInboxResponse`：

```text
items[] {
  conversation_id
  conversation_seq
  event_id
  event_type
  message_id
  sender_id
  payload_json
  created_at
}
next_seq
has_more
```

`AckDeliveryRequest`：

```text
auth_context
conversation_id
received_seq
request_id
trace_id
```

错误码：

| 错误码 | gRPC code | 语义 | 是否可重试 |
| --- | --- | --- | --- |
| `INVALID_ARGUMENT` | `InvalidArgument` | 参数缺失或 limit 非法 | 否 |
| `PERMISSION_DENIED` | `PermissionDenied` | 用户无权访问 conversation inbox | 否 |
| `CURSOR_REGRESSION` | `FailedPrecondition` | 严格调试模式下 ACK 小于已记录游标 | 否 |
| `ACK_OUT_OF_VISIBLE_RANGE` | `FailedPrecondition` | ACK 大于该用户已可见最大 seq | 否 |
| `DB_READ_FAILED` | `Unavailable` | 读取 inbox 失败 | 是 |
| `DB_WRITE_FAILED` | `Unavailable` | 写 cursor / outbox 失败 | 是 |
| `SERVICE_OVERLOADED` | `Unavailable` | delivery-service 主动过载保护 | 是 |

## 6. 异步事件契约

### 6.1 消费事件

| 事件 | Topic | 分区键 | 处理 |
| --- | --- | --- | --- |
| `message.persisted.v1` | `conversation.timeline.events` | `tenant_id + conversation_id` | 按 fanout mode 写 inbox |
| `conversation.member.joined.v1` | `conversation.timeline.events` | `tenant_id + conversation_id` | 建立成员可见边界 |
| `conversation.member.left.v1` | `conversation.timeline.events` | `tenant_id + conversation_id` | 截断后续可见性 |
| `conversation.member.removed.v1` | `conversation.timeline.events` | `tenant_id + conversation_id` | 截断后续可见性 |
| `conversation.member.role_changed.v1` | `conversation.timeline.events` | `tenant_id + conversation_id` | 更新投递权限投影 |
| `conversation.member.boundary_cancelled.v1` | `conversation.timeline.events` | `tenant_id + conversation_id` | 记录边界取消事实，第一阶段不自动回滚历史 inbox |

消费规则：

- `conversation.member.*` 事件先更新 `delivery_membership_projection`。
- `message.persisted.v1` 只能 fanout 到 `delivery_membership_projection` 中满足可见窗口的用户：

```text
join_seq <= message_seq
AND (leave_seq IS NULL OR leave_seq >= message_seq)
AND status = ACTIVE
```

- 禁止为了生成历史 inbox 去实时查询 `conversation-service` 当前成员表；当前成员状态不能覆盖旧 timeline event 的可见性。
- unsupported / malformed timeline event 必须 fail-closed：不能误写 inbox，也不能静默提交业务完成状态；第一阶段进入 projection DLQ 或阻塞并报警，repair 后再推进 checkpoint。

### 6.2 发布事件

Topic：

```text
im.delivery.events
```

Schema：

```text
schemas/kafka/delivery/v1/im.delivery.events.proto
```

事件：

| 事件 | 分区键 | 下游 |
| --- | --- | --- |
| `delivery.inbox_item.created.v1` | `tenant_id + conversation_id` | push-gateway、audit |
| `delivery.ack.recorded.v1` | `tenant_id + conversation_id` | receipt-service、audit |

`delivery_outbox` 是 delivery-service 的本地事务 outbox。业务事务只写 `delivery_outbox`，由 `NEXUSIM_DELIVERY_SERVICE_MODE=outbox-relay` 后台进程发布到 Kafka，不允许 `ProjectTimelineEvent` 或 `AckDelivery` 用例直接 publish Kafka。

Envelope 映射：

| `delivery_outbox` 字段 | `DeliveryEvent` 字段 |
| --- | --- |
| `event_id` | `event_id` |
| `event_type` | `event_type` |
| `event_version` | `event_version` |
| `tenant_id` | `tenant_id` |
| `conversation_id` | `aggregate_id` |
| `aggregate_version` | `aggregate_version` |
| `partition_key` | `partition_key` |
| `mapping_version` | `mapping_version` |
| `trace_id` | `trace_id` |
| `correlation_id` | `correlation_id` |
| `causation_id` | `causation_id` |
| `producer` | `producer` |
| `created_at` | `occurred_at` |
| `payload_json` | oneof payload |

Payload 映射：

- `delivery.inbox_item.created.v1` -> `DeliveryInboxItemCreatedV1`，包含 `tenant_id/user_id/conversation_id/conversation_seq/source_event_id/source_event_type/message_id`。
- `delivery.ack.recorded.v1` -> `DeliveryAckRecordedV1`，包含 `tenant_id/user_id/device_id/conversation_id/last_received_seq`。

`delivery.inbox_item.created.v1` 第一阶段定位为在线通知和补拉唤醒信号，不承载完整消息体。push-gateway 收到后只能通知在线客户端有新 inbox item，或调用 delivery-service 查询缺失范围；客户端仍以 `PullInbox` 返回的 durable read model 为准。

Relay 语义：

- 按 `tenant_id + conversation_id + aggregate_version` 保持 fail-closed 顺序保护；低版本 `PENDING` / `DLQ` 会阻塞同会话更高版本 delivery event。
- publish 成功后批量标记 `PUBLISHED`；publish 失败按指数退避写回 `retry_count/next_retry_at/last_error`。
- 超过最大次数进入 `DLQ`，不自动跳过；repair/replay 必须显式审计。
- unsupported / malformed `payload_json` 不允许 publish，也不能误标记 `PUBLISHED`，必须进入 retry / DLQ。
- Kafka publish 是 at-least-once；下游按 `event_id` 幂等去重。

## 7. 数据库设计

Migration 规划：

```text
migrations/postgres/delivery/000001_delivery_core.sql
```

核心表：

```sql
CREATE TABLE user_inbox (
    tenant_id           TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    conversation_seq    BIGINT      NOT NULL,
    event_id            TEXT        NOT NULL,
    event_type          TEXT        NOT NULL,
    message_id          TEXT        NOT NULL DEFAULT '',
    sender_id           TEXT        NOT NULL DEFAULT '',
    payload_json        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    fanout_mode         TEXT        NOT NULL,
    permission_version  BIGINT      NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, conversation_id, conversation_seq),
    UNIQUE (tenant_id, user_id, event_id)
);

CREATE TABLE delivery_membership_projection (
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    role                TEXT        NOT NULL,
    status              TEXT        NOT NULL,
    join_seq            BIGINT      NOT NULL,
    leave_seq           BIGINT,
    member_version      BIGINT      NOT NULL,
    permission_version  BIGINT      NOT NULL,
    updated_by_event_id TEXT        NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, user_id)
);

CREATE TABLE device_delivery_cursors (
    tenant_id           TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    device_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    last_received_seq   BIGINT      NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, device_id, conversation_id)
);

CREATE TABLE delivery_kafka_checkpoints (
    consumer_group      TEXT        NOT NULL,
    topic               TEXT        NOT NULL,
    partition_id        INT         NOT NULL,
    offset_value        BIGINT      NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_group, topic, partition_id)
);

CREATE TABLE delivery_outbox (
    id                  BIGSERIAL   PRIMARY KEY,
    event_id            TEXT        NOT NULL UNIQUE,
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    aggregate_version   BIGINT      NOT NULL,
    event_type          TEXT        NOT NULL,
    event_version       TEXT        NOT NULL,
    partition_key       TEXT        NOT NULL,
    mapping_version     BIGINT      NOT NULL,
    correlation_id      TEXT        NOT NULL DEFAULT '',
    causation_id        TEXT        NOT NULL DEFAULT '',
    producer            TEXT        NOT NULL,
    trace_id            TEXT        NOT NULL DEFAULT '',
    payload_json        JSONB       NOT NULL,
    status              TEXT        NOT NULL DEFAULT 'PENDING',
    retry_count         INT         NOT NULL DEFAULT 0,
    last_error          TEXT        NOT NULL DEFAULT '',
    available_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at       TIMESTAMPTZ,
    dead_lettered_at    TIMESTAMPTZ,
    published_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`delivery_kafka_checkpoints.offset_value` 固定表示 next offset to commit。处理 Kafka record offset `N` 成功后，本表写入 `N + 1`；Kafka commit 也提交同一个 next offset 语义。

约束：

- `user_inbox` 是可重建投影，不是 message 事实源。
- `delivery_membership_projection` 是可重建投影，不是 member 事实源。
- timeline event 必须先持久化投影副作用，再提交 Kafka offset。
- Kafka checkpoint 是 `consumer_group + topic + partition` 维度，不按 tenant 维度记录；同一 partition 可能混有多个 tenant。
- `delivery_outbox` 与 `user_inbox` / cursor 更新在同一个 PostgreSQL 事务内写入。
- `delivery_outbox.last_error` 不直接返回给普通客户端。

## 8. 核心流程

### 8.1 Timeline Projection

```text
Kafka conversation.timeline.events
-> trigger timeline consumer
-> app ProjectTimelineEventUseCase
-> domain fanout decision
-> PostgreSQL transaction:
   delivery_membership_projection update for member boundary
   user_inbox upsert
   delivery_kafka_checkpoints update
   delivery_outbox insert
-> commit
-> commit Kafka offset
```

### 8.2 Pull Inbox

```text
Client reconnect / sync
-> push-gateway or api-gateway
-> delivery-service PullInbox
-> query user_inbox after_seq limit
-> return ordered items
```

### 8.3 ACK

```text
Client durable local write
-> delivery-service AckDelivery
-> PostgreSQL transaction:
   lock current device_delivery_cursors
   query max visible seq from user_inbox
   if received_seq > max_visible_seq -> ACK_OUT_OF_VISIBLE_RANGE
   if received_seq <= current cursor -> idempotent success
   else update cursor to received_seq
   delivery_outbox insert delivery.ack.recorded.v1
-> response
```

## 9. 一致性和事务

强一致边界：

```text
同一 timeline event 的 inbox 写入、projection offset 更新、delivery_outbox 写入必须同事务。
同一 ACK 的 cursor 更新、delivery_outbox 写入必须同事务。
```

Kafka offset 处理：

```text
DB transaction commit succeeds
-> commit Kafka offset
```

如果进程在 DB commit 后、Kafka commit 前崩溃，重放同一 event 必须依赖 `tenant_id + user_id + event_id` 和 checkpoint 幂等去重。

最终一致边界：

```text
message-service / conversation-service 发布 timeline event
-> delivery-service 投影
-> delivery_outbox relay
-> push-gateway 在线推送
```

客户端收到 `SendMessage` 成功不代表接收方已投递；接收方以 `PullInbox` / push event / ACK 为准。

## 10. 幂等、重试和补偿

| 场景 | 幂等键 | 重试策略 | 补偿 |
| --- | --- | --- | --- |
| timeline event projection | `tenant_id + user_id + event_id` | consumer 可重复处理，upsert 去重 | Kafka replay 或 PostgreSQL projection rebuild |
| PullInbox | request 无副作用 | 客户端可重试 | 无 |
| AckDelivery | `tenant_id + user_id + device_id + conversation_id` | `received_seq <= current` 视为幂等成功，`current < received_seq <= max_visible_seq` 才推进 | `received_seq > max_visible_seq` 返回 `ACK_OUT_OF_VISIBLE_RANGE` |
| delivery outbox publish | `event_id` | at-least-once publish | 下游按 event_id 去重 |

## 11. 权限和安全

- `PullInbox` 和 `AckDelivery` 必须使用 authenticated `tenant_id/user_id/device_id`，不信任请求体裸 user。
- gRPC API 支持第一阶段 gateway verified metadata auth mode：`NEXUSIM_DELIVERY_AUTH_MODE=metadata` / `verified-metadata` 时，`PullInbox` 和 `AckDelivery` 的 `tenant_id / user_id / device_id / session_id` 只来自 gRPC metadata，不信任 request body 中可伪造的身份字段；`trace_id / request_id` 可在 metadata 缺失时从 body 兜底用于排障相关性。默认 `body` 模式仅用于兼容历史 smoke。
- 第一阶段可以通过 `user_inbox` 是否存在判断可见性；没有 inbox item 不等于 conversation 不存在。
- 成员边界事件决定投递可见窗口，不能用当前成员表回写历史可见性。
- `delivery_membership_projection` 只能由 Kafka timeline event 重建；manual repair 必须留审计。
- 普通客户端不返回内部 projection / outbox raw error。
- repair / replay 必须有审计记录。

## 12. SLO 和指标

第一阶段本地目标：

| 指标 | 目标 |
| --- | --- |
| `PullInbox` p95 | `< 50ms` |
| `AckDelivery` p95 | `< 50ms` |
| timeline projection lag | 小规模 smoke 后最终清零 |
| projection error rate | `0` |
| outbox DLQ | `0` |

必须打点：

```text
delivery_pull_latency_ms
delivery_ack_latency_ms
delivery_projection_latency_ms
delivery_inbox_write_count
delivery_projection_lag
delivery_outbox_pending_count
delivery_outbox_dlq_count
delivery_cursor_regression_count
```

## 13. 测试方案

| 测试 | 目标 |
| --- | --- |
| unit | fanout decision、ACK 单调性、权限错误映射 |
| integration | PostgreSQL 写 user_inbox / cursor / delivery_outbox 同事务 |
| contract | Proto request/response 和错误码 |
| server security | gRPC server TLS / mTLS env config、cert/key 成对校验、client DNS / URI SAN allowlist |
| consumer smoke | 构造 timeline event，投影到 user_inbox |
| full smoke | `SendMessage -> Kafka timeline -> delivery-service projection -> PullInbox -> AckDelivery` |
| loadtest | 小规模 `PullInbox/AckDelivery`，不做硬件极限矩阵 |

## 14. Runbook

运行模式规划：

```text
NEXUSIM_DELIVERY_SERVICE_MODE=grpc
NEXUSIM_DELIVERY_SERVICE_MODE=timeline-consumer
NEXUSIM_DELIVERY_SERVICE_MODE=outbox-relay
```

当前 delivery-service 已具备 `delivery_outbox -> Kafka im.delivery.events` 最小 relay 链路；push-gateway 已完成单实例最小在线通知 smoke，可以消费 `im.delivery.events` 并把在线唤醒交给 WebSocket 客户端。后续多实例在线路由仍需要 Redis route / resume buffer / slow session active close。

`projection-checkpoint-repair` 当前支持两类安全入口：

- `audit` / `rewind-next-offset`：显式指定 target checkpoint。
- `rewind-unresolved-failure`：显式指定 unresolved projection failure 的 Kafka raw offset，先锁定该 failure row，再把 checkpoint 回调到同一 offset 做 replay。
- `rewind-earliest-unresolved-failure`：不要求手填 offset，直接锁定当前最早 unresolved projection failure，再把 checkpoint 回调到该 offset 做 replay。

另有只读 `projection-failure-audit` 运维模式，用于直接列出 unresolved projection failure，并可按 `failure_class` 缩小排障范围；第一阶段它只做只读排障，不负责自动 repair / retry。

`projection-failure-cleanup` 只删除超过 retention 的 resolved failure 审计行，不允许触碰 unresolved blocker；operator 可按 `consumer_group / topic / partition_id / failure_class` 缩小 cleanup 范围，避免跨 consumer 误清历史证据。

`delivery-service grpc` 的身份来源模式：

```text
NEXUSIM_DELIVERY_AUTH_MODE=body
NEXUSIM_DELIVERY_AUTH_MODE=metadata
NEXUSIM_DELIVERY_AUTH_MODE=verified-metadata
```

`body` 是本地 smoke 兼容模式；`metadata` / `verified-metadata` 要求上游 gateway / trusted proxy 注入 `x-nexusim-tenant-id`、`x-nexusim-user-id`、`x-nexusim-device-id` 等 verified gRPC metadata，并忽略请求体里的身份字段。该模式只定义 delivery-service 如何消费已验证身份，不等同完整 API gateway、token exchange、服务发现或全服务统一身份治理。

`delivery-service grpc` 默认仍以 plaintext 启动，兼容本地 smoke 和现有 push-gateway ACK 转发。第一阶段可选开启静态 TLS / mTLS：

```text
NEXUSIM_DELIVERY_GRPC_TLS_CERT_FILE=...
NEXUSIM_DELIVERY_GRPC_TLS_KEY_FILE=...
NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_CA_FILE=...
NEXUSIM_DELIVERY_GRPC_TLS_REQUIRE_CLIENT_CERT=true
NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES=push-gateway.nexusim.local
NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_URIS=spiffe://nexusim/push-gateway
```

开启 allowlist 时按客户端证书 DNS SAN 小写 exact-match 或 URI SAN exact-match 校验。push-gateway 调 `AckDelivery` 的 delivery client 已支持第一阶段静态 TLS / mTLS 出站配置：

```text
NEXUSIM_DELIVERY_SERVICE_TLS_CA_FILE=...
NEXUSIM_DELIVERY_SERVICE_TLS_SERVER_NAME=delivery-service.nexusim.local
NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_CERT_FILE=...
NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_KEY_FILE=...
```

未配置上述 client TLS env 时 push-gateway 仍使用 plaintext，兼容现有本地 smoke。配置任一 client TLS env 后必须提供 CA file，client cert/key 必须成对配置。当前仍不包含证书签发 / 轮换 / 分发、动态服务身份治理和全服务 mTLS rollout。

本地最小 smoke：

```text
message-service grpc
conversation-service grpc
message-service outbox-relay
delivery-service timeline-consumer
delivery-service grpc
delivery-service outbox-relay
loadtest sendmessage
loadtest delivery-pull
```

常见故障：

| 故障 | 排查 |
| --- | --- |
| inbox 缺消息 | 查 Kafka consumer lag、projection offset、user_inbox 唯一冲突 |
| ACK 不前进 | 查 device_id、conversation_id、cursor regression |
| push 不到 | 先确认 user_inbox 和 delivery_outbox，再排 push-gateway route |
| 重复投递 | 客户端按 `conversation_id + seq` 去重，服务端查 `event_id` 幂等 |

## 15. 验收标准

进入编码前：

- `delivery_service.proto` 存在并生成 Go 代码。
- `migrations/postgres/delivery/000001_delivery_core.sql` 存在。
- `services/delivery-service/internal/{api,app,domain,infrastructure,types,trigger}` 存在。
- 第一阶段只实现 `PullInbox / AckDelivery / timeline projection`，不实现 WebSocket。
- `go test ./...` 通过。

第一阶段完成：

- `SendMessage -> conversation.timeline.events -> delivery-service projection -> PullInbox -> AckDelivery` 真实进程 smoke 通过。
- `delivery_outbox -> im.delivery.events` relay 单元测试和 PostgreSQL 集成测试通过。
- 小规模压测报告归档到 `docs/runbook/loadtest/delivery-service/`。
- 不把 smoke 结果表述为完整 push 闭环或生产容量结论。
