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

## 6. 数据库表结构

### 6.1 message_log

```sql
CREATE TABLE message_log (
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    conversation_seq    BIGINT      NOT NULL,
    message_id          TEXT        NOT NULL,
    sender_id           TEXT        NOT NULL,
    device_id           TEXT        NOT NULL,
    client_msg_id       TEXT        NOT NULL,
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

### 6.2 conversation_timeline_events

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

### 6.3 message_outbox

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
```

`message_outbox.status`：

```text
PENDING
PUBLISHED
DLQ
```

### 6.4 message_change_history

```sql
CREATE TABLE message_change_history (
    tenant_id            TEXT        NOT NULL,
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
    PRIMARY KEY (tenant_id, message_id, change_version)
);
```

`message_change_history.change_type`：

```text
EDIT
REVOKE
DELETE
```

`EditMessage`、`RevokeMessage`、`DeleteMessage` 都必须写 `message_change_history`，不能只依赖 Kafka 事件或 audit-service 追溯状态变化。

### 6.5 message_command_idempotency

用于 `EditMessage`、`RevokeMessage`、`DeleteMessage` 等命令幂等。

```sql
CREATE TABLE message_command_idempotency (
    tenant_id        TEXT        NOT NULL,
    command_type     TEXT        NOT NULL,
    idempotency_key  TEXT        NOT NULL,
    message_id       TEXT        NOT NULL,
    command_hash     TEXT        NOT NULL,
    result_json      JSONB       NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, command_type, message_id, idempotency_key)
);
```

### 6.6 seq_allocation_journal

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

### 6.7 timeline_gap_markers

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
```

禁止：

- 普通消息写入跨分片事务。
- message-service 热路径写 `user_inbox`。
- message-service 直写 OpenSearch/Milvus。

## 8. 事务流程

### 8.1 普通会话 SendMessage

```text
begin
  check idempotency by client_msg_id
  check permission_version
  allocate seq from conversation_seq by row lock
  insert message_log
  insert conversation_timeline_events(message.persisted)
  insert message_outbox(message.persisted)
commit
return message_id + seq
```

### 8.2 热点会话 SendMessage

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
  check message_command_idempotency by command_type + message_id + idempotency_key
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
| EditMessage | `tenant_id + message_id + idempotency_key` | 原 version 和 seq |
| RevokeMessage | `tenant_id + message_id + idempotency_key` | 原 revoke seq |
| DeleteMessage | `tenant_id + message_id + idempotency_key` | 原 delete seq |
| Outbox publish | `event_id` | publish once, consume at least once |

command hash 不一致但幂等键相同，返回 `IDEMPOTENCY_CONFLICT`。

## 10. Outbox Relay

Relay 拉取：

```sql
SELECT *
FROM message_outbox
WHERE status = 'PENDING'
  AND published_at IS NULL
  AND COALESCE(next_retry_at, available_at) <= now()
ORDER BY id
LIMIT 500
FOR UPDATE SKIP LOCKED;
```

发布流程：

```text
lock batch
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
outbox_publish_duplicate_rate 可观测但不作为错误
```

## 13. 压测场景

| 场景 | 目标 | 通过标准 |
| --- | --- | --- |
| steady SendMessage | 验证主写链路 | p99 < 120ms |
| duplicate client_msg_id | 验证幂等 | 不重复写 message_log |
| Kafka outage | 验证 outbox 积压 | accepted 正常，outbox 可追平 |
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
| `timeline out-of-order` | partition key -> seq allocation -> transaction log | 冻结写入，按 fact source 修复 |
| `ALLOCATED` 超时 | message-service 实例 -> transaction commit -> journal | commit 确认或 mark gap |
| `IDEMPOTENCY_CONFLICT` 增多 | client version -> retry logic -> command hash | 拦截异常客户端 |
| `Kafka unavailable` | broker ISR -> producer error -> outbox growth | 写入限流，保护 PG |

## 15. 验收标准

- `SendMessage`、`EditMessage`、`RevokeMessage`、`DeleteMessage` 契约冻结。
- 核心表 migration 可执行。
- 普通会话本地事务集成测试通过。
- 热点会话 seq journal 集成测试通过。
- outbox relay 重复发布场景消费幂等通过。
- 压测得到 message 写入基线。

## 16. 编码前契约拆分

第一阶段只落 message-service 主写链路，不扩散到 20 个服务同时开发。

必须先创建的契约文件：

| 文件 | 内容 | 约束 |
| --- | --- | --- |
| `api/proto/nexusim/message/v1/message_service.proto` | `SendMessage`、`EditMessage`、`RevokeMessage`、`DeleteMessage` | request 必须包含幂等键；response 必须返回 `message_id`、`conversation_seq`、`accepted_at` 或变更版本 |
| `api/proto/nexusim/message/v1/message_error.proto` | message-service 错误码 | 与 SDD 错误码表保持一致，不直接暴露数据库错误 |
| `schemas/kafka/conversation.timeline.events.proto` | `message.persisted/edited/revoked/deleted.v1` | envelope 字段与 `message_outbox` 对齐 |
| `migrations/postgres/message/000001_message_core.sql` | 核心表和唯一约束 | 必须包含 `conversation_seq`、`message_log`、`conversation_timeline_events`、`message_outbox`、`message_change_history`、`message_command_idempotency` |

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
| 热点 sequencer | 只定义 port/mock | 不实现 timeline-service |
| delivery / push / rag / agent | 不实现 | 不进入第一阶段 |

第一阶段允许 mock 的依赖：

| 依赖 | mock 行为 | 不能做的事 |
| --- | --- | --- |
| `policy-service` | 返回 allow/deny 和 `permission_version` | 不能在 message-service 里硬编码角色规则 |
| `conversation-service` | 返回会话存在、成员版本、普通/热点模式 | 不能由 message-service 修改成员事实 |
| `timeline-service` | 热点会话返回 seq block；普通会话不调用 | 不能把热点 sequencer 状态写在 message-service 业务层 |
| Kafka | 本地可用真实 broker；无 broker 时只能验证 outbox 积压 | 不能跳过 outbox 直接认为事件已发布 |

代码包边界：

```text
services/message-service/cmd
services/message-service/internal/adapter/grpc
services/message-service/internal/adapter/http
services/message-service/internal/application
services/message-service/internal/domain
services/message-service/internal/port
services/message-service/internal/infrastructure/postgres
services/message-service/internal/infrastructure/kafka
services/message-service/internal/runtime
```

测试门禁：

- `SendMessage` 重复 `client_msg_id` 不重复写 `message_log`。
- 相同幂等键但 command hash 不同返回 `IDEMPOTENCY_CONFLICT`。
- 本地事务失败时不能留下半条 `message_log` 或 `message_outbox`。
- Kafka publish 成功但 mark published 失败时，重复 publish 被 consumer 幂等处理。
- 热点会话 `ALLOCATED` 超时巡检能补 `COMMITTED` 或 `GAP_MARKED`。
- 按 `docs/runbook/local-loadtest.md` 启动本地压测端口后，MacBook 能跑第一轮 SendMessage 压测。
