# NexusIM search-service SDD v0.1

状态：Draft

本文定义 `search-service` 的第一条可编码切片：消费 `conversation.timeline.events`，构建搜索 projection，并提供 `SearchMessages`。它是搜索索引服务，不绑定具体搜索中间件；当前默认 PostgreSQL first path 使用 FTS 词法检索，显式 OpenSearch / BM25 candidate backend first path 已通过 adapter 接入。外部索引只做候选召回，最终可见性、tombstone 和成员窗口过滤仍由 PostgreSQL projection 执行。

## 1. 服务定位

`search-service` 拥有搜索 read model：

- `search_message_documents`：消息搜索文档投影，保存可检索文本、状态和来源事件。
- `search_membership_projection`：成员可见窗口投影，用于查询过滤，不是成员事实源。
- `search_projection_checkpoints`：consumer group + topic + partition 的 next offset checkpoint。
- `search_index_port`：后端索引端口，第一版支持 PostgreSQL FTS 和显式 OpenSearch candidate adapter，后续按 ADR 替换。

职责：

- 消费 `conversation.timeline.events`。
- 将 message persisted / edited / revoked / deleted 投影为搜索文档状态。
- 将 member joined / left / removed / role_changed 投影为可见窗口。
- 提供 `SearchMessages`，按 tenant、用户、会话和可见窗口过滤。
- 在撤回、删除、保留期清理后让搜索结果不可见。

不负责：

- 不修改 `message_log`、`conversation_timeline_events`、`message_outbox`。
- 不分配 conversation seq。
- 不决定成员事实，成员边界来自 `conversation-service`。
- 不做 RAG chunk / embedding / rerank；这些属于后续 `memory-service`、`retrieval-gateway`、`rag-service` 或 `summary-service`。
- 不保存长期 memory、不生成用户画像；group memory 由后续 `memory-service` 消费同一事实事件后生成。
- 不触发 Agent 工具动作；Agent / MCP / Skill 必须通过 `retrieval-gateway` 和 policy tool check。
- 不直接读取其它服务内部表，不直接访问 message-service 或 conversation-service 数据库。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游事件 | Kafka `conversation.timeline.events` | 消费 message/member boundary 事件 |
| 同步入口 | `api-gateway` | 调用 `SearchMessages` |
| 同步依赖 | PostgreSQL | 写 projection、checkpoint、可选第一版索引表 |
| 可选同步依赖 | `policy-service` | projection stale 或 strict mode 时做最终授权；后续 Agent/tool action 做 tool policy precheck |
| 后端端口 | `SearchIndexPort` | 写入 / 删除 / 查询索引，不绑定具体中间件；外部搜索只返回候选 refs |

## 3. 六层 DDD 包结构

```text
services/search-service/
  cmd/search-service
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
| `api` | gRPC handler：`SearchMessages`、错误码映射 |
| `app` | `ProjectTimelineEventUseCase`、`SearchMessagesUseCase` |
| `domain` | 文档状态、可见窗口、tombstone、查询过滤规则 |
| `infrastructure` | PostgreSQL projection repo、Kafka consumer、search index adapter |
| `types` | Command、DTO、错误 sentinel、cursor |
| `trigger` | timeline consumer worker、projection repair worker |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `SearchDocument` | 可检索消息文档 | `(tenant_id, conversation_id, message_id)` 唯一 |
| `SearchVisibilityWindow` | 用户在会话内的可见窗口 | 来自 member boundary event，不能作为成员事实源 |
| `SearchTombstone` | 撤回 / 删除 / 保留期清理状态 | tombstone 后默认不返回文档正文 |
| `SearchProjectionCheckpoint` | Kafka projection 进度 | DB / index 副作用成功后才推进 next offset |
| `SearchHit` | 查询结果 | 必须带 source event id、message id、conversation seq、visibility version |

可见性规则：

```text
document.seq >= membership.join_seq
AND (membership.leave_seq IS NULL OR document.seq <= membership.leave_seq)
AND membership.status = ACTIVE for current open window
AND document.tombstone_status = NONE
```

如果查询时 projection 不确定、checkpoint 明显落后或 policy version stale，第一版必须 fail closed：返回 `SEARCH_UNAVAILABLE` 或走 `policy-service` strict check，不得放宽为“有结果就返回”。

后续 `memory-service`、`retrieval-gateway`、RAG、summary 和 Agent 都必须复用这里的 tombstone / visibility 语义。任何 AI 输出或工具调用不得使用绕过 search visibility 的消息片段。

## 5. 同步 API 契约

契约文件规划：`api/proto/nexusim/search/v1/search_service.proto`。

第一版 RPC：

```text
SearchMessages(SearchMessagesRequest) returns (SearchMessagesResponse)
```

请求字段：

| 字段 | 说明 |
| --- | --- |
| `tenant_id` | 租户 |
| `auth_context` | 由 gateway 注入的认证上下文 |
| `query` | 搜索文本；第一版只支持普通文本查询 |
| `conversation_id` | 可选，会话内搜索 |
| `after_seq` | 可选，分页 / 增量游标 |
| `limit` | 默认 20，上限由配置控制 |

响应字段：

| 字段 | 说明 |
| --- | --- |
| `items` | 命中列表 |
| `next_cursor` | 后续分页游标 |
| `projection_version` | search projection 版本 |

命中项字段：

```text
conversation_id
message_id
conversation_seq
source_event_id
sender_id
message_type
snippet
highlight_ranges
occurred_at
```

错误码：

| 内部错误 | gRPC code | public message |
| --- | --- | --- |
| `INVALID_ARGUMENT` | InvalidArgument | invalid search request |
| `PERMISSION_DENIED` | PermissionDenied | permission denied |
| `SEARCH_UNAVAILABLE` | Unavailable | search unavailable |
| `PROJECTION_STALE` | Unavailable | search projection stale |

## 6. Timeline Projection

第一版处理这些事件：

| Event | 行为 |
| --- | --- |
| `message.persisted.v1` | upsert search document，写入 searchable text、source event id、seq、metadata |
| `message.edited.v1` | 更新 document 内容和 change version；保留 source message id |
| `message.revoked.v1` | 标记 revoked tombstone，查询不返回正文 |
| `message.deleted.v1` | `CONVERSATION_VIEW` 标记 deleted tombstone；`COMPLIANCE_RETENTION` 删除正文并保留最小 tombstone 元数据 |
| `conversation.member.joined.v1` | 建立 / 重开成员可见窗口 |
| `conversation.member.left.v1` / `removed.v1` | 设置 `leave_seq = boundary_seq`，后续消息不可见 |
| `conversation.member.role_changed.v1` | 更新 role / permission version，不重置 join_seq |
| `conversation.member.boundary_cancelled.v1` | 记录边界取消事实；第一版不自动回滚历史索引 |
| `conversation.member.owner_transferred.v1` | 更新 owner / role projection |

消费者语义：

- 消费失败不得提交 Kafka offset。
- malformed / unsupported event fail closed：不写索引、不提交 offset，后续进入 projection DLQ / repair。
- 重放同一 event 必须幂等；以 `event_id` 和 `message_id` 双重防重复。
- checkpoint 的 `offset_value` 表示 next offset to commit。

## 7. 数据设计草案

第一版 migration 只服务本地 projection 和可替换索引端口：

```sql
search_message_documents(
  tenant_id,
  conversation_id,
  message_id,
  conversation_seq,
  source_event_id,
  searchable_text,
  message_type,
  sender_id,
  tombstone_status,
  change_version,
  occurred_at
)

search_membership_projection(
  tenant_id,
  conversation_id,
  user_id,
  role,
  status,
  join_seq,
  leave_seq,
  member_version,
  permission_version,
  updated_by_event_id
)

search_projection_checkpoints(
  consumer_group,
  topic,
  partition_id,
  offset_value
)
```

后续如果接入外部搜索后端，PostgreSQL 表仍保留 projection / audit / rebuild 所需的最小状态；外部索引可以重建，不作为事实源。

当前 PostgreSQL first path 使用 `plainto_tsquery('simple')` 匹配
`to_tsvector('simple', searchable_text)`，并复用
`search_message_documents` 上的 GIN expression index。该路径是 token-based lexical
search，不保留 `ILIKE` substring fallback。

显式 OpenSearch / BM25 candidate backend first path：

- `NEXUSIM_SEARCH_BACKEND=postgres|opensearch`，默认 `postgres`。
- `opensearch` backend 需要 `NEXUSIM_SEARCH_OPENSEARCH_ENDPOINT` 和
  `NEXUSIM_SEARCH_OPENSEARCH_INDEX`；可选 basic auth / API key / timeout / overfetch
  配置只用于该 backend。
- OpenSearch query 使用官方 `_search` API 和 `match` query，对
  `searchable_text` 做 `operator=and` 的候选召回，并只读取 `_source` 中的
  `conversation_id` / `message_id`。
- PostgreSQL `SearchMessagesByCandidates` 会按候选 rank hydration，并重新执行
  tenant、user membership window、tombstone、after_seq 和 conversation filter。
- OpenSearch 配置错误、非 2xx、malformed hit、dependency error 都返回
  `SEARCH_UNAVAILABLE`，不静默回退 PostgreSQL FTS。
- 当前只宣称 adapter first path 和 focused tests；真实 OpenSearch 进程 smoke、
  mapping / rebuild operator、容量曲线和 provider-grade 运维仍是后续项。

## 8. 第一版验收

编码门禁：

- `search_service.proto` 定义 `SearchMessages` 和稳定错误码。
- migration 落 `search_message_documents`、`search_membership_projection`、`search_projection_checkpoints`。
- `SearchIndexPort` 存在，app/domain 不依赖具体搜索后端。
- timeline consumer 支持 message persisted / edited / revoked / deleted 和 member joined / left / removed。
- repository PG 集成测试覆盖可见窗口、tombstone、checkpoint、replay 幂等。
- repository PG 集成测试覆盖 PostgreSQL FTS token search：查询 token 不应命中仅包含该 token 子串的文档。
- OpenSearch adapter 单元测试覆盖请求 DSL、candidate hydration、非 2xx fail-closed
  和 malformed hit fail-closed。
- PostgreSQL candidate hydration 集成测试覆盖外部候选仍受 visibility / tombstone
  过滤，不允许外部索引绕过搜索投影边界。

最小 smoke：

```text
member joined
-> SendMessage
-> timeline consumer projects document
-> SearchMessages returns message
-> EditMessage updates hit
-> Revoke/Delete hides hit
-> member leave/remove
-> later message is not visible to that user
```

本轮不是容量压测，不宣称 OpenSearch 集群、Milvus / RAG 或 provider-grade BM25
运维已完成。
