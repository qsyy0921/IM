# NexusIM contacts-service SDD v0.1

状态：Draft；proto / Kafka schema / migration / 六层骨架、PostgreSQL repository 真实事务、contacts outbox relay 和 `ACCEPT / DECLINE / CANCEL` 真实进程 smoke 已落地；联系人删除 / 拉黑 / 解除拉黑 / 备注名 / 分组 v0.2 已完成代码切片；好友申请来源 metadata、`USER -> TENANT_DEFAULT -> SYSTEM_DEFAULT` 三层隐私读取和搜索来源陌生申请 gate 已落地；删除 / 拉黑 / 备注名 / 解除拉黑真实进程 smoke 已通过；删除后重新申请 / 接受恢复联系人关系的 re-add smoke 已通过；first-stage Prometheus text `/metrics`、本地 alert rules 和 Grafana dashboard 原型已落地。

本文定义第三层 IM 产品能力中的“联系人 / 好友关系”最小服务边界。目标是补齐社交关系事实源，同时保持低耦合：不把好友关系塞进 `conversation_members`，也不让会话、消息、投递服务直接读联系人表。

## 1. 服务定位

`contacts-service` 拥有用户之间的联系人关系和好友申请事实。

职责：

- 发起好友申请；
- 接受 / 拒绝 / 取消好友申请；
- 删除联系人、拉黑联系人、解除拉黑、更新本人的联系人备注和分组；
- 读取 / 更新当前用户的联系人申请隐私设置；
- 查询当前用户收到 / 发出的好友申请列表；
- 查询当前联系人列表，并可在当前用户 ACTIVE 联系人的 `contact_user_id / remark / group_name` 内做本地搜索，也可按联系人分组过滤；
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

第二阶段约束：

- 删除 / 拉黑 / 备注名 / 分组只修改 contacts-service 自己的关系 read model 和 outbox，不修改会话、消息、投递或在线状态。
- 删除联系人是当前用户的单向关系操作：只把 `owner_user_id -> contact_user_id` 这条 edge 标为 `DELETED`，不强制删除对方视角的 edge。是否双向解除关系由产品策略或后续 saga 单独设计。
- 拉黑联系人是当前用户的单向关系操作：只把 `owner_user_id -> contact_user_id` 标为 `BLOCKED`，并发布 `contact.edge.blocked.v1`。是否影响发消息权限不由 contacts-service 直接决定，后续由 policy-service / conversation-service 投影消费该事件后统一表达。
- 解除拉黑是当前用户的单向关系操作：只允许 `BLOCKED -> ACTIVE`，并发布 `contact.edge.unblocked.v1`；不能把 `DELETED`、不存在或从未接受的关系恢复成好友。
- `BLOCKED` 会 gate contacts-service 自己的新建好友申请和接受好友申请路径：任一方向已有 `BLOCKED` edge 时，不写新的 `contact_requests`，也不接受 pending request。它仍不直接等同于 message-service 发送权限。
- 备注名和分组是当前用户私有资料：只更新 `owner_user_id -> contact_user_id` 的 `remark / group_name`，不进入对方视图，不复制用户 profile。
- 联系人申请隐私设置只控制“陌生用户能否新建好友申请”；读取优先级为用户显式设置、租户默认、系统默认；其中 `allow_search_contact_requests=false` 只拒绝 `SEARCH` 来源的新建申请，仍可允许 DIRECT / GROUP / INVITE 等其它来源；不删除已有联系人，不影响已存在 pending request，不直接改变 message / conversation / delivery 权限。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游 | api-gateway / client | gRPC/HTTP 调用 `SendContactRequest`、`RespondContactRequest`、`CancelContactRequest`、`ListContactRequests`、`ListContacts` |
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
| `app` | `SendContactRequestUseCase`、`RespondContactRequestUseCase`、`CancelContactRequestUseCase`、`ListContactRequestsUseCase`、`ListContactsUseCase`、`DeleteContactUseCase`、`BlockContactUseCase`、`UpdateContactRemarkUseCase`、`UpdateContactGroupUseCase` |
| `domain` | 好友申请状态机、联系人边不变量、幂等规则 |
| `infrastructure` | PostgreSQL repository、outbox store、Kafka producer |
| `types` | Command、DTO、错误 sentinel、枚举 |
| `trigger` | contacts outbox relay、后续 retry / repair worker |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| ContactRequest | A 向 B 发起的好友申请 | sender != receiver；同一对用户同一时间最多一个 PENDING 请求；requester 只能取消自己的请求；receiver 才能接受或拒绝 |
| ContactEdge | 方向性联系人边 | ACCEPT 后写两条 ACTIVE 边；删除 / 拉黑 / 备注名 / 分组只更新当前 owner 的方向性 edge；不能物理删除历史请求；`owner_user_id + contact_user_id` 唯一 |
| ContactPrivacySettings | 当前用户的联系人申请隐私设置 | 缺省视为 `allow_contact_requests=true, allow_search_contact_requests=true`；只有当前 auth user 可读写自己的设置；更新必须幂等并发布 `contact.privacy.updated.v1` |
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

第一阶段已实现 `PENDING -> ACCEPTED / DECLINED / CANCELED` 和 ACTIVE 联系人列表。第二阶段实现当前用户视角的 `ACTIVE -> DELETED / BLOCKED`、`BLOCKED -> ACTIVE`、`remark` 和 `group_name` 更新；`BLOCKED` / `ACTIVE` 转换只产生联系人事实事件，不直接改 `SendMessage` 权限。

## 5. 同步 API 契约

契约文件：

```text
api/proto/nexusim/contacts/v1/contacts_service.proto
```

第一阶段 RPC：

```text
rpc SendContactRequest(SendContactRequestRequest) returns (SendContactRequestResponse)
rpc RespondContactRequest(RespondContactRequestRequest) returns (RespondContactRequestResponse)
rpc CancelContactRequest(CancelContactRequestRequest) returns (CancelContactRequestResponse)
rpc ListContactRequests(ListContactRequestsRequest) returns (ListContactRequestsResponse)
rpc ListContacts(ListContactsRequest) returns (ListContactsResponse)
rpc GetContactState(GetContactStateRequest) returns (GetContactStateResponse)
rpc DeleteContact(DeleteContactRequest) returns (DeleteContactResponse)
rpc BlockContact(BlockContactRequest) returns (BlockContactResponse)
rpc UpdateContactRemark(UpdateContactRemarkRequest) returns (UpdateContactRemarkResponse)
rpc UpdateContactGroup(UpdateContactGroupRequest) returns (UpdateContactGroupResponse)
```

`SendContactRequestRequest`：

```text
auth_context
target_user_id
idempotency_key
message
source_type = DIRECT / SEARCH / GROUP / INVITE_LINK / QR_CODE / IMPORT
source_ref
```

`source_type` 缺省为 `DIRECT`，`source_ref` 是低敏来源引用，例如搜索关键词 hash、群 / 会话引用或邀请链接 hash / id。`source_ref` 会持久化到 `contact_requests`，并进入 `contacts_outbox` / 下游策略和审计事件；调用方不得把原始邀请 token、手机号、邮箱、认证 token、provider secret、完整搜索词或其它用户敏感内容放入该字段。第一版已支持租户级来源策略：没有显式策略时各来源默认允许；若某租户禁用 `SEARCH / INVITE_LINK` 等来源，则 `SendContactRequest` 返回 `PERMISSION_DENIED`，不写 `contact_requests` 或 outbox。

`RespondContactRequestRequest`：

```text
auth_context
request_id
decision = ACCEPT / DECLINE
idempotency_key
```

`CancelContactRequestRequest`：

```text
auth_context
request_id
idempotency_key
```

`CancelContactRequest` 只能由原 sender 对 `PENDING` 申请执行，成功后状态变为 `CANCELED`，不创建联系人边。

`GetContactPrivacyRequest`：

```text
auth_context
```

缺省没有用户级 `contact_privacy_settings` 行时，读取租户级 `contact_tenant_privacy_defaults`；如果租户默认也不存在，则返回 `allow_contact_requests=true, allow_search_contact_requests=true, version=0, policy_source=SYSTEM_DEFAULT`。有租户默认时返回 `policy_source=TENANT_DEFAULT`，有用户显式设置时返回 `policy_source=USER`。

`SetContactPrivacyRequest`：

```text
auth_context
allow_contact_requests
allow_search_contact_requests (optional)
idempotency_key
```

`SetContactPrivacy` 只允许当前 auth user 修改自己的设置。`allow_search_contact_requests` 是兼容增量字段：未传时保留当前有效值，传入后可单独控制 `SEARCH` 来源的陌生申请。更新使用 `SET_CONTACT_PRIVACY` 幂等键；同一命令 replay 返回原设置快照，不重复发布事件；相同 idempotency key 但参数不同返回 conflict。

`ListContactsRequest`：

```text
auth_context
page_size
page_token
query
group_name
```

`query` 为可选字段，只在 contacts-service 自有 `contact_edges` read model 中按 `contact_user_id / remark / group_name` 做大小写不敏感包含匹配；不查询 identity/profile，不返回全站用户搜索结果。`group_name` 为可选字段，只按当前用户 ACTIVE edge 上的私有分组做精确过滤；`page_token` 必须绑定 `tenant_id / owner_user_id / page_size / query / group_name / last_contact_user_id`。

`ListContactRequestsRequest`：

```text
auth_context
direction = INCOMING | OUTGOING, default INCOMING
status = PENDING | ACCEPTED | DECLINED | CANCELED | EXPIRED, default PENDING
page_size
page_token
```

`ListContactRequestsResponse` 返回当前用户视角的好友申请列表，按 `created_at DESC, request_id ASC` keyset 分页。`page_token` 绑定 `tenant_id / user_id / direction / status / page_size / last_created_at / last_request_id`，不能跨用户、跨方向、跨状态或跨 page size 复用。

`DeleteContactRequest`：

```text
auth_context
contact_user_id
idempotency_key
```

`BlockContactRequest`：

```text
auth_context
contact_user_id
idempotency_key
reason
```

`UpdateContactRemarkRequest`：

```text
auth_context
contact_user_id
remark
idempotency_key
```

`UpdateContactGroupRequest`：

```text
auth_context
contact_user_id
group_name
idempotency_key
```

删除 / 拉黑 / 备注名 / 分组返回当前 owner 视角的 `ContactItem` 或 `ContactState`，不返回对方状态。

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | 参数缺失、自己加自己、page token 非法 | 否 |
| `CONTACT_REQUEST_NOT_FOUND` | request 不存在或对当前用户不可见 | 否 |
| `CONTACT_ALREADY_EXISTS` | 双方已经是 ACTIVE 联系人 | 否 |
| `CONTACT_NOT_FOUND` | 当前用户没有对应联系人 edge，或操作对象不可见 | 否 |
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
| `contact.edge.deleted.v1` | `im.contact.events` | `tenant_id:canonical_user_pair` | push / audit / recommendation |
| `contact.edge.blocked.v1` | `im.contact.events` | `tenant_id:canonical_user_pair` | push / audit / policy projection |
| `contact.edge.remark_updated.v1` | `im.contact.events` | `tenant_id:canonical_user_pair` | audit / user preference projection |
| `contact.edge.group_updated.v1` | `im.contact.events` | `tenant_id:canonical_user_pair` | audit / user preference projection |

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

`contact.request.canceled.v1` 使用同一组申请索引字段，但不带完整申请 message；它只表达 sender 撤销已发出的 pending 申请，不表示 conversation membership 或消息权限变化。

用户昵称、头像、组织信息由 profile/identity 投影补充，contacts-service 不复制用户资料。

第二阶段事件 payload 继续只表达 contacts-service 自己的关系事实：

```text
contact.edge.deleted.v1:
  owner_user_id
  contact_user_id
  previous_status
  status = DELETED
  edge_version
  occurred_at

contact.edge.blocked.v1:
  owner_user_id
  contact_user_id
  previous_status
  status = BLOCKED
  edge_version
  reason
  occurred_at

contact.edge.remark_updated.v1:
  owner_user_id
  contact_user_id
  status
  edge_version
  remark
  occurred_at

contact.edge.group_updated.v1:
  owner_user_id
  contact_user_id
  status
  edge_version
  group_name
  occurred_at

contact.privacy.updated.v1:
  user_id
  allow_contact_requests
  allow_search_contact_requests
  privacy_version
  occurred_at
```

这些事件不表示 conversation membership 已变化，也不表示 message-service 发送权限立即变化。policy-service / conversation-service 后续可以消费 `contact.edge.blocked.v1` 建立自己的权限投影，但 contacts-service 不直接写其它服务内部表。`contact.privacy.updated.v1` 只表示该用户是否接受新的联系人申请，以及是否接受 `SEARCH` 来源的新申请；不表示已有联系人或已有 pending request 被撤销。

## 7. 数据库设计

Migration：

```text
migrations/postgres/contacts/000001_contacts_core.sql
migrations/postgres/contacts/000002_contact_edge_management.sql
migrations/postgres/contacts/000006_contact_groups.sql
migrations/postgres/contacts/000007_contact_privacy_settings.sql
migrations/postgres/contacts/000008_contact_request_source.sql
migrations/postgres/contacts/000009_contact_tenant_privacy_defaults.sql
migrations/postgres/contacts/000010_contact_request_source_policies.sql
migrations/postgres/contacts/000011_contact_search_privacy.sql
```

以下 DDL 表示 v0.2 后目标结构。`000001_contacts_core.sql` 已作为第一阶段基线存在；第二阶段只能通过 expand-only migration 增加 `remark / group_name`、`contact_privacy_settings` 和扩展 command type check，不回写旧 migration。

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
    source_type       TEXT        NOT NULL DEFAULT 'DIRECT',
    source_ref        TEXT        NOT NULL DEFAULT '',
    decided_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, sender_user_id, idempotency_key),
    CHECK (sender_user_id <> receiver_user_id),
    CHECK (status IN ('PENDING', 'ACCEPTED', 'DECLINED', 'CANCELED', 'EXPIRED')),
    CHECK (source_type IN ('DIRECT', 'SEARCH', 'GROUP', 'INVITE_LINK', 'QR_CODE', 'IMPORT')),
    CHECK (char_length(source_ref) <= 256)
);

CREATE UNIQUE INDEX uq_contact_requests_pending_pair
    ON contact_requests (tenant_id, LEAST(sender_user_id, receiver_user_id), GREATEST(sender_user_id, receiver_user_id))
    WHERE status = 'PENDING';

CREATE TABLE contact_edges (
    tenant_id        TEXT        NOT NULL,
    owner_user_id    TEXT        NOT NULL,
    contact_user_id  TEXT        NOT NULL,
    status           TEXT        NOT NULL,
    remark           TEXT        NOT NULL DEFAULT '',
    group_name       TEXT        NOT NULL DEFAULT '',
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
    CHECK (command_type IN (
        'SEND_CONTACT_REQUEST',
        'RESPOND_CONTACT_REQUEST',
        'CANCEL_CONTACT_REQUEST',
        'DELETE_CONTACT',
        'BLOCK_CONTACT',
        'UNBLOCK_CONTACT',
        'UPDATE_CONTACT_REMARK',
        'UPDATE_CONTACT_GROUP'
    ))
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

第二阶段 migration 必须保持 expand-only，不修改 `000001_contacts_core.sql`：

```text
migrations/postgres/contacts/000002_contact_edge_management.sql
migrations/postgres/contacts/000006_contact_groups.sql
migrations/postgres/contacts/000007_contact_privacy_settings.sql
```

计划变更：

- `contact_edges.remark TEXT NOT NULL DEFAULT ''`；
- `contact_edges.group_name TEXT NOT NULL DEFAULT ''`；
- 新增 `contact_privacy_settings(tenant_id, user_id, allow_contact_requests, allow_search_contact_requests, version, created_at, updated_at)`；
- 新增 `contact_requests.source_type / source_ref`，用于审计和后续策略输入；`source_ref` 只允许低敏业务引用，不保存原始 invite token、手机号 / 邮箱、认证 token 或 provider secret；
- 新增 `contact_tenant_privacy_defaults(tenant_id, allow_contact_requests, allow_search_contact_requests, version, created_at, updated_at)`，用于用户未显式设置时的租户默认；
- 新增 `contact_tenant_request_source_policies(tenant_id, source_type, allow_contact_requests, version, created_at, updated_at)`，用于租户级好友申请来源 allow / deny；
- 扩展 `contact_command_idempotency.command_type` check，加入 `CANCEL_CONTACT_REQUEST / DELETE_CONTACT / BLOCK_CONTACT / UNBLOCK_CONTACT / UPDATE_CONTACT_REMARK / UPDATE_CONTACT_GROUP / SET_CONTACT_PRIVACY`；
- 不新增跨服务外键，不引用 `conversation_members`、`message_log` 或 delivery / receipt 内部表。

## 8. 核心流程

### 8.1 发起好友申请

```text
SendContactRequest
-> validate auth / target / idempotency
-> lock idempotency key
-> if contact_requests has same sender idempotency key and same command hash, replay
-> if same idempotency key but different command hash, conflict
-> check existing ACTIVE contact edge
-> check any BLOCKED edge in the pair; if found, return PERMISSION_DENIED
-> check pending request pair
-> check target user's contact_privacy_settings; missing row means allow; SEARCH source additionally checks allow_search_contact_requests
-> normalize source_type/source_ref; source_ref must be a low-sensitivity reference, not raw token / phone / email / auth secret
-> check contact_tenant_request_source_policies for normalized source_type; missing row means allow
-> insert contact_requests(PENDING, source_type, source_ref)
-> insert contacts_outbox(contact.request.created.v1, includes source_type/source_ref)
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

### 8.3 取消好友申请

```text
CancelContactRequest
-> validate sender auth / request_id / idempotency_key
-> lock contact_command_idempotency(tenant, sender, idempotency_key)
-> lock request row FOR UPDATE
-> if same idempotency key and same command hash, replay
-> if request sender != auth.user_id, permission denied
-> if request already CANCELED, replay existing canceled result
-> if request already ACCEPTED / DECLINED / EXPIRED, conflict
-> update request CANCELED
-> insert / reuse contact_command_idempotency result_id=request_id
-> insert contacts_outbox(contact.request.canceled.v1)
-> commit
```

### 8.4 查询联系人列表

```text
ListContacts
-> validate auth
-> query contact_edges where owner_user_id = auth.user_id and status = ACTIVE
-> if query is non-empty, filter current user's ACTIVE edge by contact_user_id / remark / group_name
-> if group_name is non-empty, filter current user's ACTIVE edge by exact group_name
-> keyset page by contact_user_id
-> page_token binds tenant_id / owner_user_id / page_size / query / group_name / last_contact_user_id
```

```text
ListContactRequests
-> validate auth
-> normalize direction/status
-> query contact_requests by receiver_user_id(INCOMING) or sender_user_id(OUTGOING)
-> filter by exact request status, default PENDING
-> keyset page by created_at DESC + request_id ASC
-> page_token binds tenant_id / user_id / direction / status / page_size / cursor
```

### 8.5 删除联系人

```text
DeleteContact
-> validate auth / contact_user_id / idempotency_key
-> lock contact_command_idempotency(tenant, owner, idempotency_key)
-> lock contact_edges(tenant, owner, contact_user_id) FOR UPDATE
-> if same idempotency key and same command hash, replay
-> if edge missing or status != ACTIVE, return CONTACT_NOT_FOUND or replay existing terminal result
-> update current owner edge status = DELETED, version = version + 1
-> insert / reuse contact_command_idempotency result_id = owner_user_id + ":" + contact_user_id
-> insert contacts_outbox(contact.edge.deleted.v1)
-> commit
```

删除联系人是当前 owner 的单向列表偏好和关系事实，不删除对方视角 edge，不删除历史 contact request，也不影响已有会话、消息或投递事实。

### 8.6 拉黑联系人

```text
BlockContact
-> validate auth / contact_user_id / idempotency_key / reason
-> lock contact_command_idempotency(tenant, owner, idempotency_key)
-> lock contact_edges(tenant, owner, contact_user_id) FOR UPDATE
-> if same idempotency key and same command hash, replay
-> if edge missing, return CONTACT_NOT_FOUND
-> if edge already BLOCKED and command hash matches, replay
-> update current owner edge status = BLOCKED, version = version + 1
-> insert / reuse contact_command_idempotency result_id = owner_user_id + ":" + contact_user_id
-> insert contacts_outbox(contact.edge.blocked.v1)
-> commit
```

BLOCKED 是 contacts-service 的当前 owner 关系状态。它不直接拒绝 `SendMessage`；发送权限必须由 policy-service / conversation-service 的权限投影或正式检查表达，避免 message-service 同步依赖 contacts-service。

### 8.7 更新备注名

```text
UpdateContactRemark
-> validate auth / contact_user_id / idempotency_key / remark
-> lock contact_command_idempotency(tenant, owner, idempotency_key)
-> lock contact_edges(tenant, owner, contact_user_id) FOR UPDATE
-> if same idempotency key and same command hash, replay
-> if edge missing or status != ACTIVE, return CONTACT_NOT_FOUND
-> update current owner edge remark, version = version + 1
-> insert / reuse contact_command_idempotency result_id = owner_user_id + ":" + contact_user_id
-> insert contacts_outbox(contact.edge.remark_updated.v1)
-> commit
```

备注名只属于当前 owner 的联系人视图，不复制到对方，不写 profile/identity，不进入消息事实。

### 8.8 更新联系人分组

```text
UpdateContactGroup
-> validate auth / contact_user_id / idempotency_key / group_name
-> trim group_name
-> lock contact_command_idempotency(tenant, owner, idempotency_key)
-> lock contact_edges(tenant, owner, contact_user_id) FOR UPDATE
-> if same idempotency key and same command hash, replay original snapshot
-> if edge missing or status != ACTIVE, return CONTACT_NOT_FOUND
-> update current owner edge group_name, version = version + 1
-> insert / reuse contact_command_idempotency result_id = owner_user_id + ":" + contact_user_id
-> insert contacts_outbox(contact.edge.group_updated.v1)
-> commit
```

联系人分组只属于当前 owner 的联系人视图，不复制到对方，不写 profile/identity，不进入消息事实。空分组表示未分组；列表过滤使用精确 `group_name`，不做全站用户搜索或 profile 搜索。

### 8.9 更新联系人申请隐私

```text
SetContactPrivacy
-> validate auth / idempotency_key
-> lock contact_command_idempotency(tenant, user, idempotency_key)
-> if same idempotency key and same command hash, replay original snapshot
-> if same idempotency key but different command hash, conflict
-> lock contact_privacy_settings(tenant, user) by transaction advisory lock
-> if row missing, insert version = 1
-> if row exists and value changes, update allow_contact_requests / allow_search_contact_requests and version = version + 1
-> if row exists and value is unchanged, keep version and do not publish duplicate event
-> insert / reuse contact_command_idempotency result_id = user_id
-> insert contacts_outbox(contact.privacy.updated.v1) only when setting changed
-> commit
```

`GetContactPrivacy` 缺省返回 `allow_contact_requests=true, allow_search_contact_requests=true, version=0`。该设置只影响新建好友申请：关闭 `allow_contact_requests` 后所有来源的 `SendContactRequest` 返回 `PERMISSION_DENIED`；只关闭 `allow_search_contact_requests` 时，`SEARCH` 来源返回 `PERMISSION_DENIED`，DIRECT / GROUP / INVITE 等其它来源仍继续按总开关和租户来源策略判断；拒绝时不写 `contact_requests` 或 outbox。已存在联系人、已存在 pending request、消息发送权限和会话成员事实不被 contacts-service 直接修改。

租户默认值第一阶段只作为 contacts-service 自有 read path 的 fallback policy：当用户没有 `contact_privacy_settings` 行时，`GetContactPrivacy` 和 `SendContactRequest` 会读取 `contact_tenant_privacy_defaults`；用户一旦执行 `SetContactPrivacy`，用户级设置优先于租户默认。当前通过本地 operator mode 审计 / 设置租户默认值，不开放普通 gRPC admin RPC；正式 admin/config service 权限面后续接入，当前不新增跨服务同步依赖。

租户来源策略第一阶段只作为 contacts-service 自有 `SendContactRequest` gate：`contact_tenant_request_source_policies` 缺省允许所有来源，显式禁用某个 `source_type` 后，该来源的新建申请返回 `PERMISSION_DENIED`，不影响其它来源、不删除已有申请、不修改联系人事实，也不直接改变 message / conversation / delivery 权限。当前通过本地 operator mode 审计 / 设置来源策略；正式 admin/config service 权限面后续接入。

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
| CancelContactRequest | `tenant_id + sender_user_id + idempotency_key`，落到 `contact_command_idempotency` | 同 command hash replay；不同 hash 返回 conflict；`PENDING -> CANCELED`；已 CANCELED 返回既有 result；已 ACCEPTED / DECLINED / EXPIRED 返回 conflict | 可重新发起申请 |
| DeleteContact | `tenant_id + owner_user_id + idempotency_key`，落到 `contact_command_idempotency` | 同 command hash replay；不同 hash 返回 conflict；只更新 owner -> contact 单向 edge | 可通过重新申请或对方仍 ACTIVE edge 恢复 |
| BlockContact | `tenant_id + owner_user_id + idempotency_key`，落到 `contact_command_idempotency` | 同 command hash replay；不同 hash 返回 conflict；重复 BLOCKED 返回既有 result | 通过 UnblockContact 从 BLOCKED 恢复 ACTIVE；不混入删除 |
| UnblockContact | `tenant_id + owner_user_id + idempotency_key`，落到 `contact_command_idempotency` | 同 command hash replay；不同 hash 返回 conflict；只允许 `BLOCKED -> ACTIVE`，并 replay 原始 result snapshot | 不能恢复 DELETED；重新加好友仍走申请 / 接受链路 |
| UpdateContactRemark | `tenant_id + owner_user_id + idempotency_key`，落到 `contact_command_idempotency` | 同 command hash replay；不同 hash 返回 conflict；remark 相同可 replay | 再次 UpdateContactRemark 覆盖 |
| UpdateContactGroup | `tenant_id + owner_user_id + idempotency_key`，落到 `contact_command_idempotency` | 同 command hash replay；不同 hash 返回 conflict；group_name 相同可 replay | 再次 UpdateContactGroup 覆盖 |
| SetContactPrivacy | `tenant_id + user_id + idempotency_key`，落到 `contact_command_idempotency` | 同 command hash replay；不同 hash 返回 conflict；同值更新不重复发布事件 | 再次 SetContactPrivacy 覆盖 |
| contacts outbox publish | `event_id` | at-least-once retry，max attempts 后 DLQ；relay 必须按 `partition_key + aggregate_version` fail-closed 阻塞低版本 PENDING/DLQ，避免 accepted 早于 created 发布 | 已有按 `event_id` 受控 repair 入口，可把 DLQ 重置为 PENDING 后重新进入 relay，并写 `contacts_outbox_repair_audit`；批量 repair 平台和审批 UI 后续实现 |

Command hash 规则：

- 不包含 `idempotency_key`、`request_id`、`trace_id`、`session_id`、`device_id`。
- `SendContactRequest` 包含 command type、tenant、sender、target、message 原文；当 `source_type != DIRECT` 或 `source_ref != ''` 时额外包含 normalized source_type / source_ref。默认 `DIRECT` + 空 `source_ref` 不写入 hash，保证迁移前已存在的默认申请 idempotency replay 不被误判为 conflict。
- `RespondContactRequest` 包含 command type、tenant、receiver、request_id、decision。
- `CancelContactRequest` 包含 command type、tenant、sender、request_id。
- `DeleteContact` 包含 command type、tenant、owner、contact。
- `BlockContact` 包含 command type、tenant、owner、contact、reason 原文。
- `UnblockContact` 包含 command type、tenant、owner、contact。
- `UpdateContactRemark` 包含 command type、tenant、owner、contact、remark 原文。
- `UpdateContactGroup` 包含 command type、tenant、owner、contact、trim 后的 group_name。
- `SetContactPrivacy` 包含 command type、tenant、user、allow_contact_requests；若请求显式携带 `allow_search_contact_requests`，也纳入 command hash，未携带时保留当前有效值。
- 第一阶段不 trim message；消息长度和敏感词等内容治理后续接 policy/identity 端口。
- 第二阶段不 trim remark / reason；`group_name` 会 trim 后写入；长度上限、敏感词、profile 展示规则后续接 policy/identity 端口。第一版实现必须至少拒绝过长输入，避免写入无限 payload。

## 11. 权限和安全

- 生产模式必须设置 `NEXUSIM_CONTACTS_AUTH_MODE=metadata`，由 gRPC metadata 中的 gateway verified identity 派生 `tenant_id / user_id`，并忽略请求体中伪造的身份字段。
- 默认 `body` auth mode 只保留给本地 smoke / 兼容旧 runner 使用，不作为生产安全边界。
- 受信 metadata key：`x-nexusim-tenant-id`、`x-nexusim-user-id`、`x-nexusim-device-id`、`x-nexusim-session-id`、`x-nexusim-trace-id`、`x-nexusim-request-id`。
- 发送申请只能以当前 auth user 作为 sender。
- 响应申请只能由 receiver 执行。
- 取消申请只能由原 sender 执行，receiver 不能取消对方发来的申请。
- `ListContactRequests` 只能查询当前 auth user 收到或发出的好友申请列表；第一阶段不提供 admin 查询或全站搜索。
- `ListContacts` 只能查询当前 auth user 的 ACTIVE 联系人列表；`query` 和 `group_name` 只过滤当前用户已有联系人，不提供 admin 查询、全站用户搜索或 profile 搜索。
- 删除 / 拉黑 / 解除拉黑 / 备注名 / 分组只能操作当前 auth user 自己的 `owner_user_id -> contact_user_id` edge。
- `GetContactPrivacy` / `SetContactPrivacy` 只能读写当前 auth user 自己的联系人申请隐私设置；缺省允许好友申请和搜索来源申请。该设置只 gate 新建 `SendContactRequest`，不直接撤销已有申请或联系人事实。
- `source_ref` 会进入本服务持久表和 outbox payload，只能作为低敏业务引用；不得存放原始邀请 token、手机号 / 邮箱、认证 token、provider secret、完整搜索词或其它敏感载荷。
- 租户默认隐私和来源策略当前只通过本地 operator mode 修改，不暴露普通 gRPC admin RPC；后续接入 admin/config service 前，不能把用户请求体里的字段当成 admin 授权。
- `BLOCKED` 会阻止 contacts-service 新建 / 接受好友申请，但不直接等同于消息发送权限拒绝；其它服务必须通过正式 policy / projection 使用该事实，不能同步读 contacts-service 内部表。
- 不在事件 payload 里暴露私密用户资料，只放 user id 和关系状态。
- 用户存在性、封禁状态、组织策略后续通过 identity/policy port 接入；第一阶段先保留端口边界或 strict mock。
- gRPC server 支持第一阶段静态 TLS / mTLS 配置：`NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE`、`NEXUSIM_CONTACTS_GRPC_TLS_KEY_FILE` 启用 server TLS；`NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE` 或 `NEXUSIM_CONTACTS_GRPC_TLS_REQUIRE_CLIENT_CERT=true` 启用客户端证书校验；`NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES` / `NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_URIS` 可做 client certificate DNS / URI SAN exact-match allowlist。默认仍是 plaintext，方便本地 smoke；这不是证书签发、轮换、分发、动态 SPIFFE 身份或全服务 mTLS rollout。
- 当 `NEXUSIM_CONTACTS_AUTH_MODE=metadata|verified-metadata` 时，非 loopback / 非 RFC1918 的 gRPC 监听地址若没有启用 mTLS client-certificate verification，必须在启动前直接失败；第一阶段 trusted metadata 只允许私网 listener 在无 mTLS 下运行。

## 12. SLO 和指标

第一阶段只做本地 smoke，不做容量承诺。

建议指标：

```text
contacts_request_created_total
contacts_request_accepted_total
contacts_request_declined_total
contacts_request_conflict_total
contacts_edge_deleted_total
contacts_edge_blocked_total
contacts_remark_updated_total
contacts_group_updated_total
contacts_list_latency_ms
contacts_outbox_pending_count
contacts_outbox_dlq_count
contacts_grpc_requests_total
contacts_grpc_errors_total
contacts_grpc_latency_ms
```

生产化基础观测入口已接入 `NEXUSIM_CONTACTS_DEBUG_ADDR`：

```text
GET /healthz
GET /readyz
GET /debug/metrics
```

`/readyz` 会检查 PostgreSQL ping；`/debug/metrics` 第一版输出 pgx pool 状态、低敏联系人聚合快照，以及 `contacts_outbox` 的 total / pending / published / DLQ / ready_pending / oldest age。联系人聚合只包含 `contact_requests` 的总量和各状态计数、`contact_edges` 的总量和 `ACTIVE / DELETED / BLOCKED / with_remark` 聚合计数，以及 `contact_command_idempotency` 总行数；不暴露 user_id、request_id、remark 内容、message 内容或 command hash。gRPC interceptor 会输出 JSON 结构化请求日志，包含 service、method、code、latency_ms。该入口只暴露本服务自己的健康、关系聚合与 outbox 状态，不读取其它服务内部表。debug HTTP 监听地址默认只允许 loopback / RFC1918 私网；公网或 unspecified 地址必须显式设置 `NEXUSIM_CONTACTS_DEBUG_ALLOW_PUBLIC=true`。

同一 debug HTTP server 已提供第一阶段 Prometheus text `/metrics`，复用 `/debug/metrics` 的低敏 snapshot。本地默认 scrape 目标为 `host.docker.internal:11915/metrics`，对应本地进程监听 `NEXUSIM_CONTACTS_DEBUG_ADDR=127.0.0.1:11915`。当前 `/metrics` 只覆盖 gRPC request / error / latency、contact request / edge 聚合、contacts outbox、outbox relay、PG pool 和 OTel trace config 聚合指标；labels 只允许 method、code、state、status、exporter 等低基数字段，不得输出 token、tenant_id、user_id、device_id、session_id、request_id、trace_id、remark、message body、command hash 或 payload。该 endpoint、Prometheus alert rules 和 Grafana dashboard 只用于本地开发 / 面试展示，不代表生产 Prometheus、Alertmanager、SLO 或容量观测已完成。

first-stage OpenTelemetry trace 默认关闭，仅覆盖 contacts-service gRPC server span。启用后从 incoming metadata 提取 W3C `traceparent`，只记录 service / method / gRPC status / latency 等低敏低基数属性，不记录 token、tenant/user/device/session id、trace_id、request_id、remark、payload 或 command hash。`x-nexusim-trace-id` / `x-nexusim-request-id` 仍用于 metadata / access log correlation，但不作为 span attribute 导出。支持 exporter：

```text
NEXUSIM_CONTACTS_OTEL_TRACES_ENABLED=true
NEXUSIM_CONTACTS_OTEL_SERVICE_NAME=contacts-service
NEXUSIM_CONTACTS_OTEL_TRACES_EXPORTER=stdout|otlp-grpc
NEXUSIM_CONTACTS_OTEL_TRACES_OTLP_ENDPOINT=otel-collector:4317
NEXUSIM_CONTACTS_OTEL_TRACES_OTLP_INSECURE=true
NEXUSIM_CONTACTS_OTEL_TRACES_SAMPLING_RATIO=1
```

`/debug/metrics` 会暴露低敏 trace runtime snapshot，便于确认 contacts-service 是否启用 trace、使用哪个 exporter 和采样率。当前仍是本地 debug/运维入口，不等同于完整 Prometheus / OpenTelemetry collector / alertmanager 生产栈。

## 13. 测试方案

| 测试 | 目标 |
| --- | --- |
| domain unit | 自己加自己、重复 pending、accept 写双向 edge、非 receiver 响应 |
| app unit | command validation、repository error propagation |
| api unit | gRPC request/response 转换、稳定错误映射 |
| postgres integration | SendContactRequest / RespondContactRequest / CancelContactRequest / ListContactRequests / ListContacts / ListContacts(query/group_name) / DeleteContact / BlockContact / UpdateContactRemark / UpdateContactGroup 真实事务、申请来源 metadata、租户默认隐私 operator、来源策略 gate / operator、BLOCKED gate、幂等 replay、并发首次申请、反向 pending、终态相反 decision、分页 token 绑定、搜索 / 分组 token 绑定、单向 edge 变更 |
| outbox integration | contacts_outbox retry / DLQ / mark PUBLISHED / 按 event_id repair DLQ 后恢复顺序发布 |
| smoke | `SendContactRequest -> ListContactRequests(PENDING) -> RespondContactRequest(ACCEPT) -> ListContactRequests(ACCEPTED) -> ListContacts` |
| cancel smoke | `SendContactRequest -> ListContactRequests(INCOMING,PENDING) -> CancelContactRequest -> ListContactRequests(INCOMING,PENDING)=0 -> ListContactRequests(OUTGOING,CANCELED)=1` |
| v0.2 smoke | `ACCEPT -> DeleteContact`、`ACCEPT -> BlockContact`、`ACCEPT -> BlockContact -> UnblockContact`、`ACCEPT -> UpdateContactRemark`、`ACCEPT -> DeleteContact -> SendContactRequest -> ACCEPT`，分别验证 contacts_outbox / Kafka / ListContacts / GetContactState |

## 14. Runbook

运行模式：

```text
NEXUSIM_CONTACTS_SERVICE_MODE=grpc
NEXUSIM_CONTACTS_SERVICE_MODE=outbox-relay
NEXUSIM_CONTACTS_SERVICE_MODE=outbox-repair
NEXUSIM_CONTACTS_SERVICE_MODE=tenant-privacy-default-audit
NEXUSIM_CONTACTS_SERVICE_MODE=tenant-privacy-default-set
NEXUSIM_CONTACTS_SERVICE_MODE=source-policy-audit
NEXUSIM_CONTACTS_SERVICE_MODE=source-policy-set
NEXUSIM_CONTACTS_AUTH_MODE=metadata   # production / gateway verified identity
NEXUSIM_CONTACTS_AUTH_MODE=body       # local smoke compatibility only
NEXUSIM_CONTACTS_DEBUG_ADDR=0.0.0.0:10501
NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE=certs/contacts-server.crt
NEXUSIM_CONTACTS_GRPC_TLS_KEY_FILE=certs/contacts-server.key
NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE=certs/gateway-client-ca.crt
NEXUSIM_CONTACTS_GRPC_TLS_REQUIRE_CLIENT_CERT=true
NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES=api-gateway.nexusim.local
NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_URIS=spiffe://nexusim/api-gateway
```

租户默认隐私 operator：

```powershell
$env:NEXUSIM_CONTACTS_SERVICE_MODE='tenant-privacy-default-audit'
$env:NEXUSIM_CONTACTS_TENANT_PRIVACY_TENANT_ID='tenant-demo'
.\contacts-service.exe

$env:NEXUSIM_CONTACTS_SERVICE_MODE='tenant-privacy-default-set'
$env:NEXUSIM_CONTACTS_TENANT_PRIVACY_TENANT_ID='tenant-demo'
$env:NEXUSIM_CONTACTS_TENANT_PRIVACY_ALLOW_CONTACT_REQUESTS='true'
$env:NEXUSIM_CONTACTS_TENANT_PRIVACY_ALLOW_SEARCH_CONTACT_REQUESTS='false'
.\contacts-service.exe
```

该 operator mode 只读写 `contact_tenant_privacy_defaults`，不发布 Kafka 事件，不绕过用户级隐私设置优先级；正式多租户 admin API / 审批 / audit sink 后续由 admin/config 能力补齐。

租户来源策略 operator：

```powershell
$env:NEXUSIM_CONTACTS_SERVICE_MODE='source-policy-audit'
$env:NEXUSIM_CONTACTS_SOURCE_POLICY_TENANT_ID='tenant-demo'
$env:NEXUSIM_CONTACTS_SOURCE_POLICY_SOURCE_TYPE='SEARCH'
.\contacts-service.exe

$env:NEXUSIM_CONTACTS_SERVICE_MODE='source-policy-set'
$env:NEXUSIM_CONTACTS_SOURCE_POLICY_TENANT_ID='tenant-demo'
$env:NEXUSIM_CONTACTS_SOURCE_POLICY_SOURCE_TYPE='SEARCH'
$env:NEXUSIM_CONTACTS_SOURCE_POLICY_ALLOW_CONTACT_REQUESTS='false'
.\contacts-service.exe
```

该 operator mode 只读写 `contact_tenant_request_source_policies`，不发布 Kafka 事件，不影响已有申请和联系人事实；正式多租户 admin API / 审批 / audit sink 后续由 admin/config 能力补齐。

受控 outbox repair：

```powershell
$env:NEXUSIM_CONTACTS_SERVICE_MODE='outbox-repair'
$env:NEXUSIM_CONTACTS_OUTBOX_REPAIR_EVENT_IDS='evt_contact_1,evt_contact_2'
$env:NEXUSIM_CONTACTS_OUTBOX_REPAIR_REASON='operator retried after kafka recovery'
.\contacts-service.exe
```

另有只读 `outbox-audit` 运维模式，可直接查询 `contacts_outbox` 当前行，并按 `outbox_id / event_id / tenant_id / aggregate_id / status / event_type` 缩小排障范围；它不 redrive，不修改当前 outbox 状态。

`outbox-repair` 只处理明确列出的 `contacts_outbox.status='DLQ'` 事件，把它们重置为 `PENDING`、清理 retry / error / DLQ 时间字段，并写入 `contacts_outbox_repair_audit` 保存原状态、原 retry/error 和 repair reason。随后事件交回普通 outbox relay 按 `partition_key + aggregate_version` 顺序发布；不会直接 publish Kafka，也不会跳过低版本阻塞。`PUBLISHED`、仍在 `PENDING` 或不存在的 event 会计入 skipped。

另有只读 `outbox-repair-audit` 运维模式，可直接查询 `contacts_outbox_repair_audit` 历史，并按 `event_id / tenant_id` 缩小排障范围；它不 redrive，也不修改 `contacts_outbox` 当前状态。`outbox-repair-cleanup` 则按 `retention + batch_size` 清理过期 `contacts_outbox_repair_audit` 行，并支持 `event_id / tenant_id` 范围收窄；它只删除 repair 历史，不改写当前 outbox 状态。

本地容器编排：

```powershell
$env:GOOS='linux'; $env:GOARCH='amd64'
. .\tools\go-env.ps1
go build -o bin\linux\contacts-service ./services/contacts-service/cmd/contacts-service
docker compose -f deploy\local\docker-compose.yml -f deploy\local\docker-compose.contacts-service.yml up -d postgres kafka contacts-service-grpc contacts-service-outbox-relay
```

`contacts-service-grpc` 和 `contacts-service-outbox-relay` 分进程运行，共用同一个 runtime image；`contacts-service-outbox-repair` 放在 `contacts-repair` profile 下，只用于一次性人工修复，不应常驻运行。生产环境还需要镜像签名、灰度发布、配置中心和正式观测栈，本 compose 只作为本地/双机 smoke 的最小编排样例。

本地 smoke：

```text
contacts-service grpc
-> loadtest/contacts SendContactRequest
-> loadtest/contacts ListContactRequests(PENDING)
-> loadtest/contacts RespondContactRequest(ACCEPT)
-> loadtest/contacts ListContactRequests(ACCEPTED)
-> loadtest/contacts ListContacts
```

取消申请 smoke：

```text
loadtest/contacts --scenario cancel
```

第二阶段 smoke：

```text
loadtest/contacts --scenario delete
loadtest/contacts --scenario block
loadtest/contacts --scenario remark
```

这些场景必须继续使用 `H:\NexusIM\loadtest-results` 保存原始 summary；E 盘仓库只保存报告和索引文档。

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
- 第一轮代码实现 `SendContactRequest / RespondContactRequest / CancelContactRequest / ListContactRequests / ListContacts`，不自动创建会话；
- 第二轮代码已实现 `DeleteContact / BlockContact / UpdateContactRemark` 的 proto / schema / migration / repository / relay builder / smoke runner；
- `go test ./services/contacts-service/...` 通过，带 `NEXUSIM_PG_DSN` 的 contacts PostgreSQL 集成测试通过；
- 真实 PostgreSQL integration 覆盖 request、accept、cancel、list；
- smoke 报告归档到 `docs/runbook/loadtest/contacts-service/`。

进入第二轮联系人管理编码前：

- SDD 明确删除 / 拉黑 / 解除拉黑 / 备注名是当前 owner 单向 edge 操作；
- migration v0.2 只扩展 contacts-service 自己的表；
- proto / Kafka schema 增量添加 `DeleteContact / BlockContact / UnblockContact / UpdateContactRemark` 和 `contact.edge.*` 事件，不复用旧 tag；
- 不新增 message-service / conversation-service 对 contacts-service 的同步依赖；
- 真实 PostgreSQL 测试覆盖三类操作的幂等、状态转换、outbox 和单向可见性；
- 三条真实进程 smoke 报告归档到 `docs/runbook/loadtest/contacts-service/`。
