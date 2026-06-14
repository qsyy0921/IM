# NexusIM receipt-service SDD v0.1 Draft

状态：Draft，proto / Kafka schema / migration / 六层骨架、PostgreSQL repository、delivery event consumer、`MarkRead` 事务、`ListReceiptStates` 薄批量查询、receipt outbox relay、只读 `outbox-audit`、`outbox-repair`、只读 `outbox-repair-audit`、`outbox-repair-cleanup` operator、最小 `ListConversations`、`unread_only` 未读过滤、Archive / Pin / Mute 用户列表偏好、第一阶段 gRPC server TLS / mTLS 配置、`/healthz` / `/readyz` / `/debug/metrics` 低敏观测入口、first-stage OpenTelemetry gRPC server span，以及 receipt / demo smoke runner 的 delivery / receipt client TLS 配置已落地；真实进程 smoke 已覆盖 `im.delivery.events -> receipt projection -> MarkRead -> receipt_outbox -> im.receipt.events` 和会话列表偏好链路。

本文定义 `receipt-service` 的第一条可编码切片：基于 `delivery-service` 已经产生的 durable delivery 事件，构建消息送达 / 已读回执 read model，并提供最小查询和 `MarkRead` 写入入口。

## 1. 服务定位

`receipt-service` 拥有消息回执 read model：

- `receipt_inbox_projection`：从 `delivery.inbox_item.created.v1` 重建用户可见消息索引，只保存回执所需字段，包括 `sender_id` 和 `source_event_type`。
- `device_received_cursors`：从 `delivery.ack.recorded.v1` 投影每个设备的已接收游标。
- `user_read_cursors`：用户维度的已读游标；同一用户多设备读到更高 seq 后单调推进。
- `message_receipt_states`：按 message / conversation seq 聚合用户送达和已读状态。
- `receipt_outbox`：后续向 sender、audit、会话列表或未读数投影发布回执变更事件。

职责：

- 消费 `im.delivery.events`。
- 将 `delivery.inbox_item.created.v1` 投影为回执所需的用户可见消息索引。
- 将 `delivery.ack.recorded.v1` 转换为“设备已接收 / user received”状态。
- 提供 `MarkRead`，由客户端显式报告用户已读到某个 conversation seq。
- 提供 `GetReceiptState`，查询某条消息或某个 seq 的送达 / 已读聚合；提供最小批量 `ListReceiptStates`，用于同一会话内一次查询少量消息回执。
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
| 同步上游 | API gateway / client | 调用 `MarkRead`、`GetReceiptState`、`ListReceiptStates` |
| 同步依赖 | PostgreSQL | 写回执 projection、cursor、outbox、checkpoint |
| 同步依赖 | conversation / policy access port | 校验 `MarkRead`、`GetReceiptState` 和 `ListReceiptStates` 的会话访问权限、隐私模式和版本 |
| 异步下游 | Kafka `im.receipt.events` | 发布 receipt received/read event |
| 下游 | conversation-list projection / audit | 会话摘要、审计；push-gateway 在线回执提示后置 |

`receipt-service` 不读取 `delivery-service` 内部表。它通过 `im.delivery.events` 重建必要投影；如果 projection lag 或数据缺失，API 必须返回可解释错误或要求客户端回源 `PullInbox`，不能直接跨服务读内部表补洞。

权限来源必须显式化：app 层通过 `ReceiptAccessPort` 调用 conversation / policy 能力，第一阶段可以用本地 mock，但接口必须表达 `CanMarkRead` 和 `CanViewReceiptState` 两种语义。该端口返回 visibility mode、permission version 和必要的 membership window；无权限返回 `PERMISSION_DENIED`。不能用 receipt projection 的存在性替代权限判断。

`receipt-service grpc` 支持第一阶段 gateway verified metadata auth mode：`NEXUSIM_RECEIPT_AUTH_MODE=metadata` / `verified-metadata` 时，`MarkRead / GetReceiptState / ListReceiptStates / ListConversations / ArchiveConversation / PinConversation / MuteConversation` 的 `tenant_id / user_id / device_id / session_id` 只来自 gRPC metadata，不信任 request body 中可伪造的身份字段；`trace_id / request_id` 可在 metadata 缺失时从 body 兜底用于排障相关性。默认 `body` 模式仅用于兼容历史 smoke。该模式只定义 receipt-service 如何消费已验证身份，不等同完整 API gateway、token exchange、服务发现或全服务统一身份治理。

`receipt-service grpc` 默认仍以 plaintext 启动，兼容现有 receipt smoke、demo 和内部客户端。第一阶段可选开启静态 TLS / mTLS：

```text
NEXUSIM_RECEIPT_GRPC_TLS_CERT_FILE=...
NEXUSIM_RECEIPT_GRPC_TLS_KEY_FILE=...
NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_CA_FILE=...
NEXUSIM_RECEIPT_GRPC_TLS_REQUIRE_CLIENT_CERT=true
NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES=api-gateway.nexusim.local
NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_URIS=spiffe://nexusim/api-gateway
```

开启 allowlist 时按客户端证书 DNS SAN 小写 exact-match 或 URI SAN exact-match 校验。`loadtest/receipt` 和 `loadtest/demo` 已支持对 delivery / receipt gRPC client 配置 CA、server name 和 client cert/key，用于本地 smoke 验证。其它客户端 TLS 迁移、证书签发 / 轮换 / 分发、动态服务身份治理和全服务 mTLS rollout 仍是后续项。

当 `NEXUSIM_RECEIPT_AUTH_MODE=metadata|verified-metadata` 时，如果 gRPC 监听地址不是 loopback / RFC1918 私网，且服务端没有启用 mTLS client cert 校验，receipt-service 必须在启动前直接失败，避免把第一阶段 trusted metadata 模式暴露到公网监听面。

`receipt-service` 的 `/healthz` / `/readyz` / `/debug/metrics` 仅用于本地 smoke 和低敏排障。debug HTTP 监听地址来自 `NEXUSIM_RECEIPT_DEBUG_ADDR`，未配置时可回退 `NEXUSIM_DEBUG_ADDR`；默认只允许 loopback / RFC1918 私网地址。若确需绑定公网或 unspecified 地址，必须显式设置 `NEXUSIM_RECEIPT_DEBUG_ALLOW_PUBLIC=true`，避免未认证 debug endpoint 被误暴露。

`receipt-service grpc` 已支持第一阶段 OpenTelemetry gRPC server span，默认关闭。开启后只记录低敏低基数服务侧属性：gRPC full method、status code、latency，并从 `traceparent` 继承 W3C trace context；不得写入 token、tenant/user/device/session id、trace_id、request_id、conversation id、message id、payload 或回执状态详情。`x-nexusim-trace-id` / `x-nexusim-request-id` 仍用于 metadata / access log correlation，但不作为 span attribute 导出。

```text
NEXUSIM_RECEIPT_OTEL_TRACES_ENABLED=true
NEXUSIM_RECEIPT_OTEL_SERVICE_NAME=receipt-service
NEXUSIM_RECEIPT_OTEL_TRACES_EXPORTER=stdout|otlp-grpc
NEXUSIM_RECEIPT_OTEL_TRACES_OTLP_ENDPOINT=otel-collector:4317
NEXUSIM_RECEIPT_OTEL_TRACES_OTLP_INSECURE=true
NEXUSIM_RECEIPT_OTEL_TRACES_SAMPLING_RATIO=1
```

`stdout` 适合本地 smoke；`otlp-grpc` 必须显式配置 endpoint。OTel collector、采样策略治理、alerting 和 dashboard 仍属于后续统一观测治理，不在本切片内宣称完成。

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
| `api` | gRPC handler：`MarkRead`、`GetReceiptState`、`ListReceiptStates`、错误码映射 |
| `app` | `ProjectDeliveryEventUseCase`、`MarkReadUseCase`、`GetReceiptStateUseCase`、`ListReceiptStatesUseCase`、`ReceiptAccessPort` |
| `domain` | received/read cursor 单调性、message receipt 聚合、权限窗口校验 |
| `infrastructure` | PostgreSQL repository、Kafka delivery consumer、receipt outbox relay |
| `types` | Command、DTO、错误 sentinel、枚举 |
| `trigger` | Kafka consumer worker、receipt outbox relay、repair worker |

第一阶段运维面补充：`receipt-service` 提供只读 `outbox-audit`、`outbox-repair`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运行模式，用于直接按 `outbox_id / event_id / tenant_id / conversation_id / status / event_type` 审计 `receipt_outbox` 当前状态，把指定 `DLQ` event redrive 回 `PENDING`，并按 retention / scope 清理 repair audit 历史。当前 repair audit 仍是轻量历史，只记录 `previous_status / previous_retry_count / previous_last_error / previous_dead_lettered_at / repair_reason / repaired_at`，更细 operator / outcome 语义后续再补。

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `ReceiptInboxItem` | 回执服务自己的可见消息索引 | 来自 `delivery.inbox_item.created.v1`，`(tenant_id,user_id,conversation_id,conversation_seq)` 唯一，必须保存 `sender_id` 和 `source_event_type` |
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
rpc ListReceiptStates(ListReceiptStatesRequest) returns (ListReceiptStatesResponse)
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

`ListReceiptStatesRequest`：

```text
auth_context
conversation_id
items[] {
  message_id
  conversation_seq
}
```

规则：

- 每个 item 必须且只能提供 `message_id` 或 `conversation_seq`。
- 第一阶段最多 50 个 item，响应顺序必须与请求顺序一致。
- `ListReceiptStates` 只做同一会话内薄批量查询：app 层只调用一次 `ReceiptAccessPort.CanViewReceiptState`，随后按请求顺序复用既有 `GetReceiptState` repository 读模型；不新增批量 SQL、不跨服务读内部表、不新增公共抽象。
- 第一阶段采用 whole-request failure：任一 item 参数错误、无权限、projection lag、not found 或 DB 读取失败，整个 RPC 返回现有稳定错误码；不做 item 级 error 协议。若未来需要部分成功，必须先扩展 proto 契约。

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
| `delivery.inbox_item.created.v1` | 写 `receipt_inbox_projection`，建立 `(user, conversation_seq) -> message_id/source_event_id/source_event_type` 映射 |
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
source_event_type
```

发布规则：

- `MarkRead` 和 receipt projection 只写 `receipt_outbox`，不得直接 publish Kafka。
- outbox relay 保持 at-least-once，下游按 `event_id` 幂等。
- publish 失败只允许写入稳定低敏 `last_error`，不得持久化 broker / provider raw error。
- 第一阶段 `receipt_outbox.aggregate_version` 使用 cursor seq，表达“该用户推进到的 conversation seq”，不承诺同 conversation 所有用户回执事件形成全局严格递增版本。下游不得依赖该字段做唯一排序；如需生产级全序，后续引入独立 receipt event sequence。
- `receipt.message.read.v1.device_id` 表示触发 `MarkRead` 的设备，不表示 read cursor 是设备维度；权威 read cursor 仍是用户维度。
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
    source_event_type   TEXT        NOT NULL DEFAULT 'message.persisted.v1',
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

当前已落地文件包括：

- `api/proto/nexusim/receipt/v1/receipt_service.proto`：`MarkRead`、`GetReceiptState`、`ListReceiptStates`。
- `schemas/kafka/receipt/v1/im.receipt.events.proto`：`receipt.message.received.v1` / `receipt.message.read.v1`。
- `migrations/postgres/receipt/000001_receipt_core.sql`。
- `services/receipt-service/internal/{api,app,domain,infrastructure,types,trigger}` 六层骨架。
- `services/receipt-service/internal/infrastructure/postgres`：receipt projection、received/read cursor、receipt outbox 写入。
- `services/receipt-service/internal/trigger/delivery`：消费 `im.delivery.events` 并在 PostgreSQL 事务提交后 commit Kafka offset。
- `services/receipt-service/internal/trigger/outbox`：发布 `receipt.message.received.v1` / `receipt.message.read.v1` 到 `im.receipt.events`。
- `services/receipt-service/cmd/receipt-service`：gRPC server 第一阶段可选 TLS / mTLS env 配置和 cmd 层配置测试。

下一步按下面顺序推进：

1. 已跑真实进程 smoke：

```text
SendMessage
-> delivery projection
-> AckDelivery
-> receipt-service projects delivery.ack.recorded
-> MarkRead
-> GetReceiptState shows received/read
-> receipt outbox relay publishes im.receipt.events
```

2. 后续替换 `StaticAllowAccess` 为 conversation / policy adapter。
3. 后续按产品优先级接入 receipt event 下游消费者，例如会话摘要、审计或在线回执提示。

第一条 smoke 可以先只覆盖单会话、小群、单条消息：

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
- 不把第一阶段静态 TLS / mTLS 配置表述为证书生命周期、服务身份治理或全服务 mTLS rollout。

## 11. 风险和门禁

- `delivery.ack.recorded.v1` 只有 cursor，没有 message_id；receipt-service 必须先投影 `delivery.inbox_item.created.v1` 建立 seq 到 message 的映射。
- 如果 ack event 先于 inbox projection 到达，必须可重试或 fail-closed，不能丢 ack。
- read cursor 必须同时受 visible seq 和 received seq 限制，防止客户端把未投递消息标成已读。
- gRPC TLS / mTLS 必须保持 opt-in；无 TLS env 时仍为 plaintext，避免破坏现有 receipt smoke 和 demo。cert/key 必须成对配置，mTLS 或 client allowlist 开启时必须配置 client CA，并 fail fast。
- 未来撤回 / 删除上线后，receipt query 必须遵守 message visibility，不得展示已删除消息的回执详情给无权用户。
- 未来 RAG / search 使用 receipt 信息时，必须先通过 conversation membership / ACL 过滤。
