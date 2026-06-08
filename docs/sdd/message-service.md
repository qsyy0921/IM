# message-service SDD v1.0

状态：冻结，可进入 Proto / PostgreSQL migration / Kafka schema / 集成测试阶段。

## 1. 服务定位

message-service 是 NexusIM 消息事实源服务，负责普通会话消息写入、消息变更和消息 outbox。它承载最核心不变量：

```text
普通会话下 seq + message + timeline + outbox 必须同库同分片同事务。
```

职责：

- `SendMessage`
- `EditMessage`
- `RevokeMessage`
- `DeleteMessage`
- `client_msg_id` 幂等
- `message_log` 写入
- `conversation_timeline_events` 写入
- `message_outbox` 写入
- outbox relay 发布 Kafka 事件
- 热点会话 seq block 使用与 journal 标记

不负责：

- WebSocket 在线推送；
- 离线 fanout；
- ACK / read cursor；
- OpenSearch 索引；
- RAG chunk / embedding；
- Agent 执行；
- 成员事实变更。
- 用户个人消息隐藏 / `SELF_VIEW` 可见性。

## 2. 上下游

| 方向 | 服务 | 交互 |
| --- | --- | --- |
| 上游 | api-gateway | 发消息、编辑、撤回、删除 |
| 同步依赖 | policy-service | 发送和变更权限校验 |
| 同步依赖 | timeline-service | 仅热点会话获取 seq block |
| 事实源 | PostgreSQL | message、timeline、outbox 同事务 |
| 异步下游 | Kafka | `conversation.timeline.events` |
| 消费者 | delivery/search/rag/agent/audit | 消费消息事件 |

### 2.1 第一阶段依赖端口

第一阶段代码只能通过 port 调用外部能力，不能把外部服务的领域规则写进 message-service。

| Port | 第一阶段行为 | 禁止事项 |
| --- | --- | --- |
| `PolicyCheckPort` | 返回 allow/deny、`permission_version`、拒绝原因 | 不在 message-service 内硬编码角色、群主、管理员或合规规则 |
| `ConversationQueryPort` | 返回会话存在性、`member_version`、`permission_version`、`conversation_mode`、`fanout_mode`、`fanout_policy_version`、`current_seq_shard` | 不修改成员事实，不生成成员边界事件，不硬编码 fanout 策略 |
| `SequencerPort` | 只为热点会话定义 `AllocateSeqBlock` mock | 不在第一阶段实现 sequencer 状态机、epoch fencing 或 Kubernetes Lease |
| `EventPublisherPort` | 由 outbox relay 发布 Kafka 事件 | 不允许业务事务绕过 outbox 直接发布 |

第一阶段只实现普通会话 `LOCAL_ROW_LOCK`。`SEQUENCER_BLOCK` 只能保留 port、mock 和表契约，等 `timeline-service / sequencer SDD` 冻结后再实现。

第一阶段 `ConversationQueryPort` strict mock 可以返回 `fanout_mode=WRITE_FANOUT`、`fanout_policy_version=1`，但这些值必须经由 port 返回，不能在 SendMessage use case 内硬编码。

## 3. 领域模型

| 模型 | 说明 |
| --- | --- |
| `Message` | 消息事实，包含发送者、设备、类型、正文、状态 |
| `MessageChange` | 编辑、撤回、删除的状态变化历史，用于审计和回滚呈现 |
| `TimelineEvent` | 会话内可见顺序事件 |
| `OutboxEvent` | 事务内待发布事件 |
| `IdempotencyRecord` | `client_msg_id` 或 `idempotency_key` 去重 |
| `SeqAllocation` | 热点会话 seq block 分配记录 |

消息状态：

```text
NORMAL
EDITED
REVOKED
DELETED
```

状态流转：

```text
NORMAL -> EDITED
NORMAL / EDITED -> REVOKED
NORMAL / EDITED / REVOKED -> DELETED
```

## 4. 同步 API 契约

内部 RPC 使用 gRPC + Protobuf；外部 HTTP 由 api-gateway 适配。

### 4.1 SendMessage

```text
rpc SendMessage(SendMessageRequest) returns (SendMessageResponse)
```

Request：

```text
auth_context
conversation_id
client_msg_id
message_type
payload
attachment_ids
```

Response：

```text
message_id
conversation_seq
accepted_at
```

约束：

- deadline：100ms。
- 写请求不透明自动重试。
- 客户端重试必须复用同一 `client_msg_id`。
- 幂等键：`tenant_id + sender_id + device_id + client_msg_id`。
- `client_msg_id` 是 device scoped globally unique UUID。同一 `tenant_id + sender_id + device_id` 下不能跨会话复用。

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `PERMISSION_DENIED` | 无发送或变更权限 | 否 |
| `CONVERSATION_NOT_FOUND` | 会话不存在或不可见 | 否 |
| `MESSAGE_TOO_LARGE` | 消息体超过限制 | 否 |
| `UNSUPPORTED_MESSAGE_TYPE` | 不支持的消息类型 | 否 |
| `IDEMPOTENCY_CONFLICT` | 幂等键相同但 command hash 不同 | 否 |
| `SEQUENCER_UNAVAILABLE` | 热点会话 sequencer 不可用 | 是 |
| `SEQ_BLOCK_EXHAUSTED` | 本地 seq block 用尽且暂时无法续租 | 是 |
| `DB_WRITE_FAILED` | 本地事务失败 | 是 |
| `OUTBOX_WRITE_FAILED` | outbox 写入失败，事务回滚 | 是 |
| `SERVICE_OVERLOADED` | 服务过载或连接池保护性拒绝 | 是 |

`SERVICE_OVERLOADED` 的 gRPC response 必须附带标准 `RetryInfo` detail；当前第一阶段使用固定 `500ms` retry delay，客户端仍必须叠加指数退避和 jitter，不能立即重试。

### 4.2 同步调用治理

| 调用 | deadline | retry | fallback |
| --- | ---: | --- | --- |
| api-gateway -> message-service.SendMessage | 100ms | 服务端不自动重试写请求；客户端复用 `client_msg_id` 重试 | 返回 retryable error |
| message-service -> policy-service | 30ms | 短重试 1 次，仅限幂等读取 | fail closed，返回 `PERMISSION_DENIED` 或 retryable policy error |
| message-service -> conversation-service | 30ms | 短重试 1 次，仅限会话/成员版本读取 | 返回 `CONVERSATION_NOT_FOUND` 或 retryable dependency error |
| message-service -> timeline-service | 20ms | 仅热点会话可重试 | 返回 `SEQUENCER_UNAVAILABLE` |
| outbox-relay -> Kafka | producer config | 指数退避，更新 `retry_count`、`last_error`、`next_retry_at` | 留在 outbox；超过上限进入 `DLQ` |

约束：

- 写请求不做透明服务端重试，避免放大重试风暴。
- 所有 retry 必须带 trace，并可从 metrics 区分依赖失败和业务失败。
- fallback 不能绕过权限、成员版本或 seq 分配事实源。

### 4.3 EditMessage

Request：

```text
auth_context
conversation_id
message_id
idempotency_key
payload
```

Response：

```text
message_id
conversation_seq
version
```

约束：

- 只允许有权限的操作者编辑。
- 每次编辑写 `message_change_history`。
- 下游 search/rag 必须通过事件更新或重建。

### 4.4 RevokeMessage

Request：

```text
auth_context
conversation_id
message_id
idempotency_key
reason
```

Response：

```text
message_id
conversation_seq
```

约束：

- 补拉只返回 tombstone。
- audit 记录操作者、原因、trace。

### 4.5 DeleteMessage

Request：

```text
auth_context
conversation_id
message_id
idempotency_key
delete_scope
```

Response：

```text
message_id
conversation_seq
```

约束：

- 个人视图隐藏不进入 message-service，由 api-gateway 路由到 delivery-service / user-inbox 读模型。
- 会话级删除和合规删除分开。
- 合规删除必须进入 Retention workflow。

`delete_scope`：

```text
CONVERSATION_VIEW
COMPLIANCE_RETENTION
```

语义：

- `CONVERSATION_VIEW` 对会话成员返回 tombstone。
- `COMPLIANCE_RETENTION` 进入 Retention workflow，并按 legal hold / delete proof 约束执行。

`SELF_VIEW` 属于 delivery-service / user-inbox 视图状态，不产生 `message.deleted.v1`，不改变 `message_log`、`conversation_timeline_events`、`message_outbox`。

## 5. 异步事件契约

所有事件写入 `conversation.timeline.events`。

| 事件 | 触发 | 分区键 | 下游 |
| --- | --- | --- | --- |
| `message.persisted.v1` | SendMessage 成功 | `tenant_id + conversation_id` | delivery/search/rag/agent/audit |
| `message.edited.v1` | EditMessage 成功 | `tenant_id + conversation_id` | delivery/search/rag/audit |
| `message.revoked.v1` | RevokeMessage 成功 | `tenant_id + conversation_id` | delivery/search/rag/audit |
| `message.deleted.v1` | DeleteMessage 成功 | `tenant_id + conversation_id` | delivery/search/rag/audit |

Envelope 必须包含：

```text
event_id
event_type
event_version
tenant_id
aggregate_type = conversation
aggregate_id = conversation_id
aggregate_version = conversation_seq
partition_key
mapping_version
trace_id
correlation_id
causation_id
producer
payload
```

Timeline event metadata 必须包含：

```text
fanout_mode
fanout_policy_version
permission_version
classification
mapping_version
```

## 6. 数据库表结构

### 6.1 conversation_seq

普通会话 seq 事实源。必须与 `message_log`、`conversation_timeline_events`、`message_outbox` 同库同分片。

```sql
CREATE TABLE conversation_seq (
    tenant_id        TEXT        NOT NULL,
    conversation_id  TEXT        NOT NULL,
    current_seq      BIGINT      NOT NULL DEFAULT 0,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id)
);
```

普通会话分配 seq：

```sql
UPDATE conversation_seq
SET current_seq = current_seq + 1,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
RETURNING current_seq;
```

初始化责任：

```text
conversation-service 创建会话时初始化 conversation_seq(current_seq=0)。
message-service 首次发送发现 conversation_seq 缺失时，可以在同事务内幂等补建，但必须记录 metric 和 repair log。
兜底补建只能在 `ConversationQueryPort` 确认 conversation 存在且 `PolicyCheckPort` 确认当前操作者可发送后执行。
```

兜底补建 SQL：

```sql
INSERT INTO conversation_seq (tenant_id, conversation_id, current_seq)
VALUES ($1, $2, 0)
ON CONFLICT (tenant_id, conversation_id) DO NOTHING;
```

### 6.2 message_log

```sql
CREATE TABLE message_log (
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    conversation_seq    BIGINT      NOT NULL,
    message_id          TEXT        NOT NULL,
    sender_id           TEXT        NOT NULL,
    device_id           TEXT        NOT NULL,
    client_msg_id       TEXT        NOT NULL,
    command_hash        TEXT        NOT NULL,
    message_type        TEXT        NOT NULL,
    payload_json        JSONB       NOT NULL,
    status              TEXT        NOT NULL,
    permission_version  BIGINT      NOT NULL,
    classification      TEXT        NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    edited_at           TIMESTAMPTZ,
    revoked_at          TIMESTAMPTZ,
    deleted_at          TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, conversation_id, conversation_seq),
    UNIQUE (tenant_id, message_id),
    UNIQUE (tenant_id, sender_id, device_id, client_msg_id)
);
```

`command_hash` 是 SendMessage 命令的规范化哈希，用于判断同一 `client_msg_id` 下请求内容是否一致。重复请求命中唯一键时，如果 `command_hash` 不一致，必须返回 `IDEMPOTENCY_CONFLICT`。

`command_hash` 计算规则：

```text
SHA256(canonical_json({
  tenant_id,
  conversation_id,
  sender_id,
  device_id,
  client_msg_id,
  message_type,
  payload,
  sorted_attachment_ids
}))
```

不进入 `command_hash` 的字段：

```text
request_id
trace_id
accepted_at
client_send_time
```

### 6.3 conversation_timeline_events

```sql
CREATE TABLE conversation_timeline_events (
    tenant_id          TEXT        NOT NULL,
    conversation_id    TEXT        NOT NULL,
    seq                BIGINT      NOT NULL,
    event_id           TEXT        NOT NULL,
    event_type         TEXT        NOT NULL,
    event_version      TEXT        NOT NULL,
    message_id         TEXT,
    actor_id           TEXT        NOT NULL,
    fanout_mode        TEXT        NOT NULL,
    fanout_policy_version BIGINT      NOT NULL,
    permission_version BIGINT,
    classification     TEXT,
    mapping_version    TEXT        NOT NULL,
    trace_id           TEXT        NOT NULL,
    payload_json       JSONB       NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, seq),
    UNIQUE (tenant_id, event_id)
);
```

### 6.4 message_outbox

```sql
CREATE TABLE message_outbox (
    id                BIGSERIAL   PRIMARY KEY,
    event_id          TEXT        NOT NULL UNIQUE,
    tenant_id         TEXT        NOT NULL,
    conversation_id   TEXT        NOT NULL,
    aggregate_version BIGINT      NOT NULL,
    event_type        TEXT        NOT NULL,
    event_version     TEXT        NOT NULL,
    partition_key     TEXT        NOT NULL,
    mapping_version   TEXT        NOT NULL,
    correlation_id    TEXT        NOT NULL,
    causation_id      TEXT        NOT NULL,
    producer          TEXT        NOT NULL DEFAULT 'message-service',
    payload_json      JSONB       NOT NULL,
    trace_id          TEXT        NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'PENDING',
    available_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at     TIMESTAMPTZ,
    published_at      TIMESTAMPTZ,
    retry_count       INT         NOT NULL DEFAULT 0,
    last_error        TEXT,
    dead_lettered_at  TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_message_outbox_ready
ON message_outbox (COALESCE(next_retry_at, available_at), id)
WHERE status = 'PENDING' AND published_at IS NULL;

CREATE INDEX idx_message_outbox_dlq
ON message_outbox (dead_lettered_at, id)
WHERE status = 'DLQ';

CREATE INDEX idx_message_outbox_conversation_order
ON message_outbox (tenant_id, conversation_id, aggregate_version, status);
```

`message_outbox.status`：

```text
PENDING
PUBLISHED
DLQ
```

### 6.5 message_change_history

```sql
CREATE TABLE message_change_history (
    tenant_id            TEXT        NOT NULL,
    conversation_id      TEXT        NOT NULL,
    message_id           TEXT        NOT NULL,
    change_version       INT         NOT NULL,
    change_type          TEXT        NOT NULL,
    before_payload_json  JSONB,
    after_payload_json   JSONB,
    before_status        TEXT        NOT NULL,
    after_status         TEXT        NOT NULL,
    changed_by           TEXT        NOT NULL,
    reason               TEXT,
    trace_id             TEXT        NOT NULL,
    changed_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, message_id, change_version)
);
```

`message_change_history.change_type`：

```text
EDIT
REVOKE
DELETE
```

`EditMessage`、`RevokeMessage`、`DeleteMessage` 都必须写 `message_change_history`，不能只依赖 Kafka 事件或 audit-service 追溯状态变化。

### 6.6 message_command_idempotency

用于 `EditMessage`、`RevokeMessage`、`DeleteMessage` 等命令幂等。

```sql
CREATE TABLE message_command_idempotency (
    tenant_id        TEXT        NOT NULL,
    conversation_id  TEXT        NOT NULL,
    command_type     TEXT        NOT NULL,
    idempotency_key  TEXT        NOT NULL,
    message_id       TEXT        NOT NULL,
    command_hash     TEXT        NOT NULL,
    result_json      JSONB       NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, command_type, message_id, idempotency_key)
);
```

### 6.7 seq_allocation_journal

仅热点会话使用。

```sql
CREATE TABLE seq_allocation_journal (
    tenant_id          TEXT        NOT NULL,
    conversation_id    TEXT        NOT NULL,
    sequencer_epoch    BIGINT      NOT NULL,
    seq                BIGINT      NOT NULL,
    allocation_id      TEXT        NOT NULL,
    allocated_to       TEXT        NOT NULL,
    status             TEXT        NOT NULL,
    allocated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    committed_at       TIMESTAMPTZ,
    gap_marked_at      TIMESTAMPTZ,
    reason             TEXT,
    PRIMARY KEY (tenant_id, conversation_id, seq)
);
```

巡检补偿：

```text
ALLOCATED 超过 30s 未 COMMITTED:
  1. 查询 message_log 是否已有 tenant_id + conversation_id + seq
  2. 有则补 COMMITTED
  3. 无则写 GAP_MARKED + timeline_gap_markers
```

`seq_allocation_journal` 是热点 seq 解释事实源；`message_log/timeline/outbox` 是消息事实源。

### 6.8 timeline_gap_markers

用于解释热点会话已分配但未形成消息事实的 seq，保证补拉、审计和修复任务看到的是显式 gap，而不是未知缺口。

```sql
CREATE TABLE timeline_gap_markers (
    tenant_id        TEXT        NOT NULL,
    conversation_id  TEXT        NOT NULL,
    seq              BIGINT      NOT NULL,
    allocation_id    TEXT        NOT NULL,
    sequencer_epoch  BIGINT      NOT NULL,
    reason           TEXT        NOT NULL,
    detected_by      TEXT        NOT NULL,
    detected_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, seq)
);
```

## 7. 分片与事务边界

分片键：

```text
hash(tenant_id + conversation_id)
```

必须同分片：

```text
conversation_seq
message_log
conversation_timeline_events
message_outbox
message_change_history
message_command_idempotency
```

禁止：

- 普通消息写入跨分片事务。
- message-service 热路径写 `user_inbox`。
- message-service 直写 OpenSearch/Milvus。

## 8. 事务流程

### 8.1 普通会话 SendMessage

```text
outside transaction:
  validate request
  compute command_hash
  query ConversationQueryPort
  call PolicyCheckPort
  if conversation_mode == SEQUENCER_BLOCK:
    return SEQUENCER_UNAVAILABLE(reason=sequencer_not_implemented_in_phase_1)
  if checked_permission_version != current_permission_version:
    retry dependency read once or return retryable dependency error
  pre-check idempotency by client_msg_id and command_hash if possible

begin
  re-check idempotency by client_msg_id and command_hash
  re-check expected permission_version / member_version snapshot
  insert conversation_seq if missing only after conversation and permission were validated
  allocate seq from conversation_seq by row lock
  insert message_log
  insert conversation_timeline_events(message.persisted)
  insert message_outbox(message.persisted)
commit
return message_id + seq
```

事务边界约束：

- DB transaction 内不能调用 `ConversationQueryPort`、`PolicyCheckPort`、Kafka、Redis 或任何外部 RPC。
- 外部依赖抖动只能影响事务开始前的等待时间，不能拉长 `conversation_seq` 行锁持有时间。
- `PolicyCheckPort.checked_permission_version` 与 `ConversationQueryPort.current_permission_version` 不一致时，只允许短重试一次；仍不一致则返回 retryable dependency error。

### 8.2 热点会话 SendMessage

第一阶段不实现该流程。代码必须在识别 `conversation_mode=SEQUENCER_BLOCK` 时直接返回 `SEQUENCER_UNAVAILABLE`，不能写 `seq_allocation_journal`、`timeline_gap_markers`，也不能真实调用 `AllocateSeqBlock`。

目标态流程：

```text
pre-check idempotency by client_msg_id
if hit:
  return old message_id + seq
get seq from local seq block cache
insert seq_allocation_journal(ALLOCATED)
begin
  check idempotency by client_msg_id
  check permission_version
  insert message_log
  insert conversation_timeline_events(message.persisted)
  insert message_outbox(message.persisted)
commit
mark seq_allocation_journal(COMMITTED)
```

并发兜底：

```text
if two requests pass pre-check concurrently:
  one transaction commits successfully
  the other hits unique(tenant_id, sender_id, device_id, client_msg_id)
  failed request queries existing message_log and returns old message_id + seq
  already allocated seq is marked GAP_MARKED
```

事务失败：

```text
mark seq_allocation_journal(GAP_MARKED)
write timeline_gap_markers
return retryable error if client can retry same client_msg_id
```

### 8.3 Edit/Revoke/Delete

`EditMessage`、`RevokeMessage`、`DeleteMessage` 与 `SendMessage` 共享 seq 分配策略：普通会话使用 `conversation_seq` row lock；热点会话使用 seq block + `seq_allocation_journal`。

```text
begin
  check message_command_idempotency by conversation_id + command_type + message_id + idempotency_key
  return IDEMPOTENCY_CONFLICT if same key maps to different command_hash
  lock message_log row
  validate operator permission
  update message_log status or payload
  insert message_change_history
  insert message_command_idempotency with command_hash and result_json
  allocate new timeline seq
  insert conversation_timeline_events
  insert message_outbox
commit
```

权限 action 粒度：

```text
message.send
message.edit.own
message.edit.any
message.revoke.own
message.revoke.any
message.delete.own
message.delete.compliance
```

- `message.delete.own` 仅用于授权作者在允许窗口内执行 `CONVERSATION_VIEW` 删除。
- `message.delete.compliance` 用于 `COMPLIANCE_RETENTION`。
- `SELF_VIEW` 使用 delivery-service 的个人可见性权限，不属于本服务 action。

## 9. 幂等策略

| 操作 | 幂等键 | 返回 |
| --- | --- | --- |
| SendMessage | `tenant_id + sender_id + device_id + client_msg_id` | 原 `message_id + seq` |
| EditMessage | `tenant_id + conversation_id + message_id + idempotency_key` | 原 version 和 seq |
| RevokeMessage | `tenant_id + conversation_id + message_id + idempotency_key` | 原 revoke seq |
| DeleteMessage | `tenant_id + conversation_id + message_id + idempotency_key` | 原 delete seq |
| Outbox publish | `event_id` | publish once, consume at least once |

command hash 不一致但幂等键相同，返回 `IDEMPOTENCY_CONFLICT`。

## 10. Outbox Relay

Relay 拉取：

```sql
SELECT *
FROM message_outbox mo
WHERE mo.status = 'PENDING'
  AND mo.published_at IS NULL
  AND COALESCE(mo.next_retry_at, mo.available_at) <= now()
  AND NOT EXISTS (
      SELECT 1
      FROM message_outbox prev
      WHERE prev.tenant_id = mo.tenant_id
        AND prev.conversation_id = mo.conversation_id
        AND prev.aggregate_version < mo.aggregate_version
        AND prev.status IN ('PENDING', 'DLQ')
  )
ORDER BY mo.id
LIMIT 500
FOR UPDATE SKIP LOCKED;
```

发布流程：

```text
lock batch
-> acquire per conversation single-flight lock
-> skip event if lower aggregate_version is still PENDING or DLQ
-> publish to Kafka with partition key tenant_id + conversation_id
-> mark status=PUBLISHED and published_at
-> on failure update retry_count, last_error, next_retry_at
-> retry exceeded -> mark status=DLQ and dead_lettered_at
-> DLQ repair workflow reviews and requeues by setting status=PENDING
```

约束：

- producer `acks=all`。
- producer `enable.idempotence=true`。
- `partition_key = tenant_id + conversation_id`。
- `event_id` 全局唯一。
- Kafka publish 成功但 mark published 失败，允许重复发布。
- 消费者必须用 `event_id + handler_name` 幂等。
- Relay lag 不能影响已提交消息的客户端 accepted 响应。
- 同一 `tenant_id + conversation_id` 的事件必须按 `aggregate_version` 严格发布。
- 如果同会话存在更小 `aggregate_version` 的 `PENDING` 或 `DLQ` 事件，Relay 不能发布后续事件。
- Relay worker 必须对同一会话 single-flight，避免多 worker 并发发布同一 conversation 的事件。

DLQ repair 操作：

```text
ReplayOutboxEvent(repair_id, event_id)
ReplayOutboxBatch(repair_id, tenant_id, event_type, time_range)
SkipOutboxEvent(repair_id, event_id, reason)
```

约束：

- `repair_id` 必填，全链路写入 trace 和 audit。
- 只有 `audit-service` 授权的运维角色可以执行 replay/skip。
- replay 只能将 `status=DLQ` 的事件改回 `PENDING`，不能直接标记 `PUBLISHED`。
- replay 前必须检查同一 `tenant_id + conversation_id` 是否存在更小 `aggregate_version` 的 `PENDING` 或 `DLQ` 事件；存在则拒绝 replay 当前事件。
- batch replay 必须支持 tenant、event_type、time_range 过滤，并按 tenant 限速。
- skip timeline 事件必须保留 `last_error`、`reason` 和操作者，发布 `audit.repair.events`，并标记 downstream 需要按 repair event 处理。

## 11. 失败补偿

| 失败点 | 结果 | 补偿 |
| --- | --- | --- |
| 权限校验失败 | 不写库 | 返回 final error |
| seq 分配失败 | 不写消息 | 客户端可重试 |
| 本地事务失败 | 不写 outbox | 普通会话可重试；热点会话 gap marker |
| outbox publish 失败 | 消息已 accepted | relay 重试 |
| Kafka 长时间不可用 | outbox 积压 | 限流写入，保护 PG |
| search/rag 下游失败 | 不影响消息事实 | 下游 DLQ/replay |

## 12. SLO

```text
SendMessage success rate >= 99.99%
SendMessage p95 < 80ms
SendMessage p99 < 120ms
Edit/Revoke/Delete p99 < 150ms
duplicate message by client_msg_id = 0
timeline out-of-order = 0
unexplained seq gap = 0
seq_allocation_journal_allocated_stale_count = 0
outbox oldest pending age < 5s
outbox_conversation_order_violation = 0
outbox_blocked_conversation_count 可观测
outbox_blocked_event_count 可观测
outbox_blocked_oldest_age_seconds 可观测
outbox_dlq_blocking_conversation_count 可观测
outbox_publish_duplicate_rate 可观测但不作为错误
```

## 13. 压测场景

| 场景 | 目标 | 通过标准 |
| --- | --- | --- |
| steady SendMessage | 验证主写链路 | p99 < 120ms |
| duplicate client_msg_id | 验证幂等 | 不重复写 message_log |
| Kafka outage | 验证 outbox 积压 | accepted 正常，outbox 可追平 |
| outbox ordered publish | 验证同会话 Kafka 顺序 | seq 较小事件未发布时，后续 seq 不发布 |
| hot conversation | 验证 seq block | 无乱序，gap 有 journal |
| local-to-sequencer switch | 验证普通会话升级热点会话 | 无重复 seq、无乱序、gap 均有 journal |
| sequencer-to-local switch | 验证热点会话降级 | block drain 正确，next_seq 对齐 |
| edit/revoke storm | 验证 timeline 变更 | search/rag 事件完整 |
| PG lock contention | 验证行锁瓶颈 | 可触发热点升级 |

## 14. Runbook

| 告警 | 排查顺序 | 修复 |
| --- | --- | --- |
| `SendMessage p99` 升高 | PG lock -> seq alloc -> outbox insert -> policy latency | 扩容、限流、热点升级 |
| `outbox_oldest_age > 5s` | Kafka publish -> relay worker -> DB lock | 扩 relay，检查 Kafka |
| `outbox_blocked_conversation_count` 上升 | 最小 aggregate_version 的 PENDING/DLQ -> relay error -> repair 状态 | 禁止继续扩大写入压力，按 repair_id replay 或 skip |
| `outbox_dlq_blocking_conversation_count` 上升 | DLQ root cause -> 同会话后续事件阻塞 -> downstream lag | 修复 DLQ，skip 必须发 `audit.repair.events` |
| `timeline out-of-order` | partition key -> seq allocation -> transaction log | 冻结写入，按 fact source 修复 |
| `ALLOCATED` 超时 | message-service 实例 -> transaction commit -> journal | commit 确认或 mark gap |
| `IDEMPOTENCY_CONFLICT` 增多 | client version -> retry logic -> command hash | 拦截异常客户端 |
| `Kafka unavailable` | broker ISR -> producer error -> outbox growth | 写入限流，保护 PG |

## 15. 验收标准

- `SendMessage`、`EditMessage`、`RevokeMessage`、`DeleteMessage` 契约冻结。
- 核心表 migration 可执行。
- 普通会话本地事务集成测试通过。
- outbox relay 重复发布场景消费幂等通过。
- outbox relay 同会话按 `aggregate_version` 有序发布。
- 压测得到 message 写入基线。

第二阶段门禁：

- `timeline-service / sequencer SDD` 冻结后，热点会话 seq journal 集成测试通过。
- `ALLOCATED` 超时巡检能补 `COMMITTED` 或 `GAP_MARKED`。

## 16. 编码前契约拆分

第一阶段只落 message-service 主写链路，不扩散到 20 个服务同时开发。

第一阶段硬冻结工程栈：

```text
Go
Kratos
gRPC + Protobuf
HTTP/OpenAPI via api-gateway
pgx + sqlc
PostgreSQL
Kafka + Schema Registry
Transactional Outbox
六层 DDD: api / app / domain / infrastructure / types / trigger
```

禁止在第一阶段引入：

```text
GORM 或其他 ORM 替代 sqlc
NATS / RocketMQ / Pulsar 替代 Kafka
REST-only 内部服务通信
跨服务共享业务 domain package
绕过 outbox 的直接 Kafka publish
```

必须先创建的契约文件：

| 文件 | 内容 | 约束 |
| --- | --- | --- |
| `api/proto/nexusim/message/v1/message_service.proto` | `SendMessage`、`EditMessage`、`RevokeMessage`、`DeleteMessage` | request 必须包含幂等键；`Edit/Revoke/Delete` 必须包含 `conversation_id`；`SendMessage` 必须明确 `client_msg_id` scope 和 `command_hash` canonical 规则 |
| `api/proto/nexusim/message/v1/message_error.proto` | message-service 错误码 | 与 SDD 错误码表保持一致，不直接暴露数据库错误 |
| `schemas/kafka/conversation.timeline.events.proto` | `message.persisted/edited/revoked/deleted.v1` | envelope 字段与 `message_outbox` 对齐；metadata 包含 fanout/permission/mapping 版本 |
| `migrations/postgres/message/000001_message_core.sql` | 核心表和唯一约束 | 必须包含 `conversation_seq`、`message_log`、`conversation_timeline_events`、`message_outbox`、`message_change_history`、`message_command_idempotency`；变更表必须带 `conversation_id` |

第一阶段实现范围：

| 项 | 状态 | 说明 |
| --- | --- | --- |
| `SendMessage` | 实现 | 打穿主写链路和压测基线 |
| PostgreSQL 本地事务 | 实现 | `conversation_seq + message_log + timeline + outbox` 同事务 |
| Outbox Relay | 实现 | 支持 pending、retry、DLQ 状态 |
| Kafka publish path | 实现 | 可用真实 broker；不可用时保留 outbox 积压测试 |
| `EditMessage` | 只定义契约 | 不进入第一条代码切片 |
| `RevokeMessage` | 只定义契约 | 不进入第一条代码切片 |
| `DeleteMessage` | 只定义契约 | 不进入第一条代码切片 |
| 热点 sequencer | 只定义 port/mock | 不实现 timeline-service；不实现 `SEQUENCER_BLOCK` 生产逻辑 |
| delivery / push / rag / agent | 不实现 | 不进入第一阶段 |

第一阶段允许 mock 的依赖：

| 依赖 | mock 行为 | 不能做的事 |
| --- | --- | --- |
| `policy-service` | 返回 allow/deny 和 `permission_version` | 不能在 message-service 里硬编码角色规则 |
| `conversation-service` | 返回会话存在、成员版本、权限版本、普通/热点模式、fanout 模式、fanout 策略版本 | 不能由 message-service 修改成员事实，不能由 message-service 硬编码 fanout 策略 |
| `timeline-service` | 热点会话返回 seq block；普通会话不调用 | 不能把热点 sequencer 状态写在 message-service 业务层 |
| Kafka | 本地可用真实 broker；无 broker 时只能验证 outbox 积压 | 不能跳过 outbox 直接认为事件已发布 |

代码包边界：

```text
services/message-service/cmd
services/message-service/internal/api/grpc
services/message-service/internal/api/http
services/message-service/internal/app
services/message-service/internal/domain
services/message-service/internal/infrastructure/postgres
services/message-service/internal/infrastructure/kafka
services/message-service/internal/infrastructure/rpc
services/message-service/internal/types
services/message-service/internal/trigger/outbox
services/message-service/internal/trigger/repair
```

测试门禁：

- `SendMessage` 重复 `client_msg_id` 不重复写 `message_log`。
- 相同幂等键但 command hash 不同返回 `IDEMPOTENCY_CONFLICT`。
- 本地事务失败时不能留下半条 `message_log` 或 `message_outbox`。
- Kafka publish 成功但 mark published 失败时，重复 publish 被 consumer 幂等处理。
- 热点会话 `ALLOCATED` 超时巡检能补 `COMMITTED` 或 `GAP_MARKED`。
- 按 `docs/runbook/local-loadtest.md` 启动本地压测端口后，MacBook 能跑第一轮 SendMessage 压测。
