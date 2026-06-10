# NexusIM contacts-service SDD v0.1

状态：Draft；proto / Kafka schema / migration / 六层骨架、PostgreSQL repository 真实事务、contacts outbox relay 和 `ACCEPT / DECLINE` 真实进程 smoke 已落地。

本文定义第三层 IM 产品能力中的“联系人 / 好友关系”最小服务边界。目标是补齐社交关系事实源，同时保持低耦合：不把好友关系塞进 `conversation_members`，也不让会话、消息、投递服务直接读联系人表。

## 1. 服务定位

`contacts-service` 拥有用户之间的联系人关系和好友申请事实。

职责：

- 发起好友申请；
- 接受 / 拒绝好友申请；
- 查询当前联系人列表；
- 查询两名用户之间的联系人状态；
- 通过 outbox 发布联系人事件，供通知、审计、推荐或后续搜索投影消费。

不负责：

- 不创建会话；
- 不写 `conversation_members`；
- 不决定 `SendMessage` 是否允许；
- 不维护在线状态、设备状态或登录 token；
- 不负责群成员、群 owner transfer、群邀请审批；
- 不把联系人列表作为权限服务的唯一来源。

第一阶段约束：

- 好友申请接受后只写 contacts-service 自己的关系表，不自动创建 direct conversation。
- 如果后续要“加好友后自动创建私聊”，必须通过 app 层显式调用 conversation-service port 或异步 saga 设计，不能在 SQL 层跨库写 `conversations`。
- message-service 不直接同步调用 contacts-service；好友可发消息策略后续由 policy-service 或 conversation-service 成员关系统一表达。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游 | api-gateway / client | gRPC/HTTP 调用 `SendContactRequest`、`RespondContactRequest`、`ListContacts` |
| 同步依赖 | identity-service（后续） | 校验 user 是否存在、是否禁用；第一阶段可用 strict mock / 本地端口 |
| 异步下游 | push-gateway / notification-service（后续） | 消费 `im.contact.events` 做轻量通知 |
| 异步下游 | audit-service / recommendation（后续） | 消费联系人事件做审计和推荐 |
| 事实源 | PostgreSQL | `contact_requests`、`contact_edges`、`contacts_outbox` |

## 3. 六层 DDD 包结构

```text
services/contacts-service/
  cmd/contacts-service
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
| `api` | gRPC handler、request/response 转换、稳定错误映射 |
| `app` | `SendContactRequestUseCase`、`RespondContactRequestUseCase`、`ListContactsUseCase` |
| `domain` | 好友申请状态机、联系人边不变量、幂等规则 |
| `infrastructure` | PostgreSQL repository、outbox store、Kafka producer |
| `types` | Command、DTO、错误 sentinel、枚举 |
| `trigger` | contacts outbox relay、后续 retry / repair worker |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| ContactRequest | A 向 B 发起的好友申请 | sender != receiver；同一对用户同一时间最多一个 PENDING 请求；requester 只能取消自己的请求；receiver 才能接受或拒绝 |
| ContactEdge | 方向性联系人边 | ACCEPT 后写两条 ACTIVE 边；删除 / 拉黑后不能物理删除历史请求；`owner_user_id + contact_user_id` 唯一 |
| ContactEvent | 联系人边界事件 | 只通过 `contacts_outbox` 发布；event_id 幂等 |

状态：

```text
ContactRequestStatus:
PENDING
ACCEPTED
DECLINED
CANCELED
EXPIRED

ContactEdgeStatus:
ACTIVE
DELETED
BLOCKED
```

第一阶段只实现 `PENDING -> ACCEPTED / DECLINED` 和 ACTIVE 联系人列表。`BLOCKED` 先保留，不做消息权限联动。

## 5. 同步 API 契约

契约文件：

```text
api/proto/nexusim/contacts/v1/contacts_service.proto
```

第一阶段 RPC：

```text
rpc SendContactRequest(SendContactRequestRequest) returns (SendContactRequestResponse)
rpc RespondContactRequest(RespondContactRequestRequest) returns (RespondContactRequestResponse)
rpc ListContacts(ListContactsRequest) returns (ListContactsResponse)
rpc GetContactState(GetContactStateRequest) returns (GetContactStateResponse)
```

`SendContactRequestRequest`：

```text
auth_context
target_user_id
idempotency_key
message
```

`RespondContactRequestRequest`：

```text
auth_context
request_id
decision = ACCEPT / DECLINE
idempotency_key
```

`ListContactsRequest`：

```text
auth_context
page_size
page_token
```

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | 参数缺失、自己加自己、page token 非法 | 否 |
| `CONTACT_REQUEST_NOT_FOUND` | request 不存在或对当前用户不可见 | 否 |
| `CONTACT_ALREADY_EXISTS` | 双方已经是 ACTIVE 联系人 | 否 |
| `CONTACT_REQUEST_CONFLICT` | 幂等键复用但 command hash 不同，或已有反向 pending 需要先处理 | 否 |
| `PERMISSION_DENIED` | 非 receiver 响应请求，或当前用户无权读取 | 否 |
| `DB_WRITE_FAILED` | PostgreSQL 写失败 | 是 |
| `OUTBOX_WRITE_FAILED` | contacts outbox 写失败 | 是 |
| `SERVICE_OVERLOADED` | admission / repository 保护触发 | 是 |

## 6. 异步事件契约

Kafka topic：

```text
im.contact.events
```

事件：

| 事件 | Topic | 分区键 | 下游 |
| --- | --- | --- | --- |
| `contact.request.created.v1` | `im.contact.events` | `tenant_id:canonical_user_pair` | push / audit |
| `contact.request.accepted.v1` | `im.contact.events` | `tenant_id:canonical_user_pair` | push / audit / recommendation |
| `contact.request.declined.v1` | `im.contact.events` | `tenant_id:canonical_user_pair` | push / audit |

Envelope 字段与现有 outbox 口径保持一致：

```text
event_id
tenant_id
aggregate_type = CONTACT_REQUEST / CONTACT_EDGE
aggregate_id = request_id or owner_user_id:contact_user_id
aggregate_version
event_type
event_version
mapping_version
partition_key
correlation_id
causation_id
producer = contacts-service
trace_id
payload_json
```

其中 `canonical_user_pair` 使用两个 user id 字典序拼接：

```text
min(sender_user_id, receiver_user_id) + ":" + max(sender_user_id, receiver_user_id)
```

涉及同一对用户的关系事实事件必须使用同一 partition key，避免 accepted / declined / 后续 removed / blocked 在 Kafka 分区上乱序。通知类下游如果需要按单个用户路由，可在消费后再 fanout，不改变关系事实事件的分区键。

第一阶段事件 payload 只包含联系人索引信息，不承载完整用户资料：

```text
request_id
sender_user_id
receiver_user_id
status
message
occurred_at
```

用户昵称、头像、组织信息由 profile/identity 投影补充，contacts-service 不复制用户资料。

## 7. 数据库设计

Migration：

```text
migrations/postgres/contacts/000001_contacts_core.sql
```

核心表：

```sql
CREATE TABLE contact_requests (
    request_id        TEXT        PRIMARY KEY,
    tenant_id         TEXT        NOT NULL,
    sender_user_id    TEXT        NOT NULL,
    receiver_user_id  TEXT        NOT NULL,
    status            TEXT        NOT NULL,
    idempotency_key   TEXT        NOT NULL,
    command_hash      TEXT        NOT NULL,
    message           TEXT        NOT NULL DEFAULT '',
    decided_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, sender_user_id, idempotency_key),
    CHECK (sender_user_id <> receiver_user_id),
    CHECK (status IN ('PENDING', 'ACCEPTED', 'DECLINED', 'CANCELED', 'EXPIRED'))
);

CREATE UNIQUE INDEX uq_contact_requests_pending_pair
    ON contact_requests (tenant_id, LEAST(sender_user_id, receiver_user_id), GREATEST(sender_user_id, receiver_user_id))
    WHERE status = 'PENDING';

CREATE TABLE contact_edges (
    tenant_id        TEXT        NOT NULL,
    owner_user_id    TEXT        NOT NULL,
    contact_user_id  TEXT        NOT NULL,
    status           TEXT        NOT NULL,
    source_request_id TEXT       NOT NULL,
    version          BIGINT      NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, owner_user_id, contact_user_id),
    CHECK (owner_user_id <> contact_user_id),
    CHECK (version > 0),
    CHECK (status IN ('ACTIVE', 'DELETED', 'BLOCKED'))
);

CREATE TABLE contact_command_idempotency (
    tenant_id        TEXT        NOT NULL,
    user_id          TEXT        NOT NULL,
    idempotency_key  TEXT        NOT NULL,
    command_type     TEXT        NOT NULL,
    command_hash     TEXT        NOT NULL,
    result_id        TEXT        NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, idempotency_key),
    CHECK (command_type IN ('SEND_CONTACT_REQUEST', 'RESPOND_CONTACT_REQUEST'))
);

CREATE TABLE contacts_outbox (
    id                BIGSERIAL   PRIMARY KEY,
    event_id          TEXT        NOT NULL UNIQUE,
    tenant_id         TEXT        NOT NULL,
    aggregate_type    TEXT        NOT NULL,
    aggregate_id      TEXT        NOT NULL,
    aggregate_version BIGINT      NOT NULL,
    event_type        TEXT        NOT NULL,
    event_version     TEXT        NOT NULL,
    mapping_version   INT         NOT NULL,
    partition_key     TEXT        NOT NULL,
    producer          TEXT        NOT NULL,
    correlation_id    TEXT        NOT NULL DEFAULT '',
    causation_id      TEXT        NOT NULL DEFAULT '',
    trace_id          TEXT        NOT NULL DEFAULT '',
    payload_json      JSONB       NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'PENDING',
    retry_count       INT         NOT NULL DEFAULT 0,
    available_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at     TIMESTAMPTZ,
    published_at      TIMESTAMPTZ,
    dead_lettered_at  TIMESTAMPTZ,
    last_error        TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ'))
);
```

## 8. 核心流程

### 8.1 发起好友申请

```text
SendContactRequest
-> validate auth / target / idempotency
-> lock idempotency key
-> if contact_requests has same sender idempotency key and same command hash, replay
-> if same idempotency key but different command hash, conflict
-> check existing ACTIVE contact edge
-> check pending request pair
-> insert contact_requests(PENDING)
-> insert contacts_outbox(contact.request.created.v1)
-> commit
```

### 8.2 接受好友申请

```text
RespondContactRequest(ACCEPT)
-> validate receiver auth
-> lock contact_command_idempotency(tenant, receiver, idempotency_key)
-> lock request row FOR UPDATE
-> if same idempotency key and same command hash, replay
-> if request already has the same terminal status, return existing result
-> if request has the opposite terminal status, conflict
-> update request ACCEPTED
-> upsert contact_edges(sender -> receiver ACTIVE)
-> upsert contact_edges(receiver -> sender ACTIVE)
-> insert / reuse contact_command_idempotency result_id=request_id
-> insert contacts_outbox(contact.request.accepted.v1)
-> commit
```

### 8.3 查询联系人列表

```text
ListContacts
-> validate auth
-> query contact_edges where owner_user_id = auth.user_id and status = ACTIVE
-> keyset page by contact_user_id
-> page_token binds tenant_id / owner_user_id / page_size / last_contact_user_id
```

## 9. 一致性和事务

强一致边界：

```text
contact_requests + contact_edges + contacts_outbox
```

最终一致边界：

```text
contacts_outbox -> Kafka im.contact.events -> push / audit / recommendation
```

不使用分布式事务。后续如果要联动创建 direct conversation，必须使用 saga 或显式 API 编排，不在 contacts-service transaction 中写 conversation-service 表。

## 10. 幂等、重试和补偿

| 场景 | 幂等键 | 重试策略 | 补偿 |
| --- | --- | --- | --- |
| SendContactRequest | `tenant_id + sender_user_id + idempotency_key`，事实源为 `contact_requests` 唯一键和 `command_hash` | 同 command hash replay；不同 hash 返回 conflict；不额外写 `contact_command_idempotency` | 无需补偿 |
| RespondContactRequest | `tenant_id + receiver_user_id + idempotency_key`，落到 `contact_command_idempotency` | 同 command hash replay；不同 hash 返回 conflict；已完成且同 decision 返回既有 result；已完成但相反 decision 返回 conflict | 后续通过重新申请恢复 |
| contacts outbox publish | `event_id` | at-least-once retry，max attempts 后 DLQ；relay 必须按 `partition_key + aggregate_version` fail-closed 阻塞低版本 PENDING/DLQ，避免 accepted 早于 created 发布 | repair/replay worker 后续实现 |

Command hash 规则：

- 不包含 `idempotency_key`、`request_id`、`trace_id`、`session_id`、`device_id`。
- `SendContactRequest` 包含 command type、tenant、sender、target、message 原文。
- `RespondContactRequest` 包含 command type、tenant、receiver、request_id、decision。
- 第一阶段不 trim message；消息长度和敏感词等内容治理后续接 policy/identity 端口。

## 11. 权限和安全

- `tenant_id / user_id / device_id / session_id / trace_id / request_id` 来自 `AuthContext`。
- 发送申请只能以当前 auth user 作为 sender。
- 响应申请只能由 receiver 执行。
- `ListContacts` 只能查询当前 auth user 的联系人列表，第一阶段不提供 admin 查询。
- 不在事件 payload 里暴露私密用户资料，只放 user id 和关系状态。
- 用户存在性、封禁状态、组织策略后续通过 identity/policy port 接入；第一阶段先保留端口边界或 strict mock。

## 12. SLO 和指标

第一阶段只做本地 smoke，不做容量承诺。

建议指标：

```text
contacts_request_created_total
contacts_request_accepted_total
contacts_request_declined_total
contacts_request_conflict_total
contacts_list_latency_ms
contacts_outbox_pending_count
contacts_outbox_dlq_count
```

## 13. 测试方案

| 测试 | 目标 |
| --- | --- |
| domain unit | 自己加自己、重复 pending、accept 写双向 edge、非 receiver 响应 |
| app unit | command validation、repository error propagation |
| api unit | gRPC request/response 转换、稳定错误映射 |
| postgres integration | SendContactRequest / RespondContactRequest / ListContacts 真实事务、幂等 replay、并发首次申请、反向 pending、终态相反 decision、分页 token 绑定 |
| outbox integration | contacts_outbox retry / DLQ / mark PUBLISHED |
| smoke | `SendContactRequest -> RespondContactRequest(ACCEPT) -> ListContacts` |

## 14. Runbook

运行模式：

```text
NEXUSIM_CONTACTS_SERVICE_MODE=grpc
NEXUSIM_CONTACTS_SERVICE_MODE=outbox-relay
```

本地 smoke：

```text
contacts-service grpc
-> loadtest/contacts SendContactRequest
-> loadtest/contacts RespondContactRequest(ACCEPT)
-> loadtest/contacts ListContacts
```

完整事件 smoke 后续再加：

```text
contacts-service outbox-relay
-> Kafka im.contact.events
-> push-gateway / audit consumer
```

## 15. 验收标准

进入第一轮编码前：

- SDD 已评审无 P0/P1；
- `contacts_service.proto` 存在并生成 Go 代码；
- `000001_contacts_core.sql` 存在；
- 六层目录存在；
- 第一轮代码只实现 `SendContactRequest / RespondContactRequest / ListContacts`，不自动创建会话；
- `go test ./services/contacts-service/...` 通过；
- 真实 PostgreSQL integration 覆盖 request、accept、list；
- smoke 报告归档到 `docs/runbook/loadtest/contacts-service/`。
