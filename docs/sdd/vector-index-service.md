# vector-index-service SDD v0.1 Draft

## 1. 服务定位

`vector-index-service` 是 NexusIM AI / 知识基础设施中的向量索引写入、删除、重建和
受控向量查询服务。它把 search / memory / knowledge chunks 的 embedding 写入 Milvus、
pgvector、OpenSearch vector 或本地测试 backend，并维护可删除、可重建、可审计的向量
metadata。

职责：

- 拥有 `vector_collections`、`vector_items`、`vector_index_jobs`、`vector_tombstones`、
  `vector_rebuild_checkpoints` 和 `vector_outbox`。
- 消费 `knowledge.chunk.ready.v1`、memory / search 可索引事件和 tombstone / delete proof。
- 通过 model-gateway 生成 embedding，或接收已授权 embedding vector ref。
- 将 vector refs、source refs、visibility metadata、policy version、delete proof 写入 vector backend。
- 为 retrieval-gateway 提供受控 `SearchVectors` backend API，只返回 refs / scores。

不负责：

- 不直接服务 RAG、summary、Agent 或客户端查询；retrieval-gateway 仍是唯一检索入口。
- 不保存 raw message body、raw chunk text、EvidencePack 正文、raw prompt 或 model output。
- 不拥有 search / memory / knowledge facts；它只保存可重建索引 metadata。
- 不绕过 policy-service、retrieval-gateway、EvidencePack、tombstone 或 delete proof。
- 不决定回答正确性、citation、Agent proposal、approval 或 tool execution。
- 不把 vector backend 当作唯一事实源；PostgreSQL metadata + upstream facts 必须可重建索引。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游事件 | knowledge-ingestion-service | chunk ready / tombstone / delete proof |
| 上游事件 | memory-service | memory event ready / superseded / tombstoned |
| 上游事件 | search-service | searchable document ready / tombstoned |
| 同步依赖 | model-gateway | embedding generation and budget control |
| 同步依赖 | policy-service | retrieval / indexing policy precheck when needed |
| 下游 API | retrieval-gateway | `SearchVectors` restricted backend call |
| 异步下游 | audit-service / ai-eval-service | low-sensitive index and rebuild events |
| 后端 | vector store | Milvus / pgvector / OpenSearch vector / local in-memory |
| 事实源 | PostgreSQL | vector item metadata、job、checkpoint、tombstone、outbox |

第一版可以先使用 PostgreSQL / local vector adapter 做 contract smoke；Milvus / OpenSearch
provider 不写死。

## 3. 六层 DDD 包结构

```text
services/vector-index-service/
  cmd/vector-index-service/
  internal/{api,app,domain,infrastructure,types,trigger}/
```

| 层 | 本服务内容 |
| --- | --- |
| `api` | gRPC adapter，retrieval-gateway / service metadata，稳定错误映射 |
| `app` | UpsertVectorItem、TombstoneVectorItem、SearchVectors、RequestRebuild |
| `domain` | collection、vector item、visibility metadata、tombstone、rebuild state |
| `infrastructure` | PostgreSQL repository、vector backend adapter、model/policy clients |
| `types` | command、DTO、错误码、枚举、低敏 metadata |
| `trigger` | chunk consumer、embedding worker、rebuild worker、outbox relay、cleanup |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `VectorCollection` | 一组向量索引配置 | backend、dimension、model、visibility policy 固定版本 |
| `VectorItem` | 一个可检索向量 metadata | 必须有 source ref、source hash、visibility、delete proof |
| `VectorIndexJob` | upsert / tombstone / rebuild / backfill 任务 | 状态可重试；job id 幂等 |
| `VectorTombstone` | 删除证明和 backend delete 状态 | tombstone 后不得返回 |
| `VectorRebuildCheckpoint` | backfill / rebuild 进度 | partition / cursor 可恢复 |
| `VectorOutboxEvent` | 低敏索引事件 | 只通过 outbox relay 发布 |

Collection 类型：

```text
KNOWLEDGE_CHUNK
MEMORY_EVENT
SEARCH_DOCUMENT
PROFILE_AGGREGATE
EVAL_FIXTURE
```

Job 状态：

```text
PENDING -> EMBEDDING_REQUESTED -> VECTOR_UPSERTING -> INDEXED
PENDING/EMBEDDING_REQUESTED/VECTOR_UPSERTING -> FAILED
FAILED -> RETRYING
INDEXED -> TOMBSTONED
PENDING/FAILED -> CANCELED
```

## 5. 同步 API 契约

```text
rpc UpsertVectorItem(UpsertVectorItemRequest) returns (UpsertVectorItemResponse)
rpc TombstoneVectorItem(TombstoneVectorItemRequest) returns (TombstoneVectorItemResponse)
rpc SearchVectors(SearchVectorsRequest) returns (SearchVectorsResponse)
rpc RequestVectorRebuild(RequestVectorRebuildRequest) returns (RequestVectorRebuildResponse)
rpc GetVectorIndexJob(GetVectorIndexJobRequest) returns (GetVectorIndexJobResponse)
```

`UpsertVectorItem` 请求字段：

```text
tenant_id, source_service, collection_type
source_ref, source_id, source_version
source_hash, chunk_hash, embedding_model_ref
visibility_scope, data_class, policy_version
delete_proof_id, retention_policy_ref
embedding_input_ref, embedding_vector_ref
idempotency_key
correlation_id, causation_id, trace_id
```

`embedding_input_ref` 是调用 model-gateway 时的受控输入引用；不能传 raw text 到 API。

`SearchVectors` 仅供 retrieval-gateway 调用：

```text
tenant_id, requester_ref, retrieval_request_id
collection_types[], query_embedding_ref or query_text_ref
top_k, min_score, visibility_context
policy_version, at_time
```

响应只返回：

```text
vector_item_ref, source_ref, source_service, score,
visibility_version, tombstone_status, collection_type
```

不得返回 raw chunk text、message body、memory text 或 source URI。

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | dimension、collection、source ref、visibility、top_k 非法 | 否 |
| `PERMISSION_DENIED` | caller 不是允许服务，或 policy precheck 拒绝 | 否 |
| `FAILED_PRECONDITION` | tombstone、delete proof、model mismatch、visibility stale | 否 |
| `ALREADY_EXISTS` | idempotency replay command hash 冲突 | 否 |
| `NOT_FOUND` | collection / item / job 不存在或不可见 | 否 |
| `UNAVAILABLE` | model-gateway、policy、vector backend 或存储暂不可用 | 是 |

## 6. 索引 metadata 和可见性

每个 `VectorItem` 必须携带：

```text
tenant_id
collection_type
source_service
source_ref
source_id
source_version
source_hash
chunk_hash or item_hash
embedding_model_ref
embedding_vector_hash
visibility_scope
visibility_version
policy_version
data_class
delete_proof_id
tombstone_status
retention_policy_ref
```

搜索时必须过滤：

```text
tenant_id matches
AND tombstone_status = NONE
AND delete_proof_id is empty
AND visibility_scope allows requester context
AND policy_version is usable or strict policy check passes
```

如果 visibility metadata 缺失、policy stale、delete proof 未同步或 vector backend metadata 不完整，
`SearchVectors` 必须 fail closed 或返回空结果，不得放宽过滤。

## 7. 异步事件契约

| 事件 | Topic | 分区键 | 说明 |
| --- | --- | --- | --- |
| `vector.item.indexed.v1` | `im.vector.events` | `tenant_id:vector_item_id` | 向量已写入 backend |
| `vector.item.tombstoned.v1` | `im.vector.events` | `tenant_id:vector_item_id` | 向量已 tombstone |
| `vector.rebuild.started.v1` | `im.vector.events` | `tenant_id:rebuild_job_id` | rebuild 开始 |
| `vector.rebuild.completed.v1` | `im.vector.events` | `tenant_id:rebuild_job_id` | rebuild 完成 |
| `vector.index.failed.v1` | `im.vector.events` | `tenant_id:job_id` | 索引失败 |

事件 payload 只包含 vector item id、source ref hash、collection type、model ref、dimension、
visibility version、tombstone status、failure class、trace refs。禁止包含 raw text、embedding
vector 数组、source URI、object key、message body、memory text、provider body 或 secret。

## 8. 数据库设计

第一版表：

```text
vector_collections
vector_items
vector_index_jobs
vector_tombstones
vector_rebuild_checkpoints
vector_outbox
```

关键字段：

```text
vector_collections:
tenant_id, collection_id, collection_type, backend_type,
dimension, embedding_model_ref, route_policy_ref, status,
metadata_schema_version, created_at

vector_items:
tenant_id, vector_item_id, collection_id, source_service,
source_ref_hash, source_id, source_version, source_hash,
chunk_hash, embedding_model_ref, embedding_vector_hash,
backend_vector_id, visibility_scope, visibility_version,
policy_version, data_class, tombstone_status, delete_proof_id,
status, created_at, updated_at

vector_index_jobs:
tenant_id, job_id, collection_id, vector_item_id, job_type,
status, retry_count, next_retry_at, failure_class, public_error,
idempotency_key, created_at, completed_at

vector_tombstones:
tenant_id, tombstone_id, vector_item_id, source_ref_hash,
delete_proof_id, reason_class, backend_delete_status, created_at

vector_rebuild_checkpoints:
tenant_id, rebuild_job_id, collection_id, source_service,
partition_key, cursor_value, status, updated_at
```

Vector metadata PostgreSQL / outbox 不保存 raw vector array；vector backend 保存实际向量
和低敏 metadata。若本地测试 adapter 必须保存 vector array，也必须限制为 test profile，
并禁止进入 events / metrics。

First-stage PostgreSQL backend state adapter:

```text
vector_backend_items:
tenant_id, backend_type, backend_vector_id, vector_item_id, collection_id,
source_ref_hash, embedding_vector_hash, dimension,
status, tombstone_status, indexed_at, deleted_at, updated_at
```

该表不保存 raw text 或 embedding vector array，只记录低敏 backend state。`SearchVectors`
必须同时满足 PostgreSQL metadata 和 backend state 为 ACTIVE；如果 backend state 缺失或
已删除，必须 fail closed / 不返回该 ref。

First-stage pgvector adapter package:

```text
services/vector-index-service/internal/infrastructure/pgvector
```

该 adapter 提供可选 `EnsureSchema` / `Upsert` / `Delete` / `Search` 实现，面向
pgvector profile。它会在 dedicated vector backend table 中保存实际 embedding vector，
但默认普通 PostgreSQL migration 不启用该 adapter，避免没有 `vector` 扩展的本地开发库
被强制要求安装 pgvector。`NEXUSIM_VECTOR_PROVIDER_BACKEND=postgres-test` 可启用本地
metadata-backed provider sink：它只确认 owned `vector_backend_items` ACTIVE 状态和 hash /
dimension，不保存 raw vector array，用于本地 embedding / rebuild backfill smoke。
`embedding-worker` 可通过 `NEXUSIM_VECTOR_PROVIDER_BACKEND=pgvector` 显式启用真实 pgvector
backend sink；未配置时行为保持 metadata-only。`rebuild-worker` 可显式启用 first-stage provider backend backfill：从本服务
自有 `vector_embedding_tasks` 中读取已完成的 redacted-preview task，重新调用
model-gateway，再写入当前配置的 provider backend；它不读取上游私表，也不从 metadata 表伪造
缺失的 vector array。Milvus / OpenSearch adapter、真实 pgvector smoke 和 provider backend
repair 仍是后续项。

## 9. 核心流程

Chunk ready indexing：

```text
knowledge.chunk.ready.v1
-> vector consumer validates source ref / visibility / delete proof
-> request embedding via model-gateway if no authorized embedding ref
-> write vector_items metadata + vector_index_jobs
-> upsert vector backend
-> mark INDEXED + write vector.item.indexed.v1 outbox
```

First-stage embedding worker：

```text
NEXUSIM_VECTOR_INDEX_SERVICE_MODE=chunk-consumer
-> consume knowledge.chunk.ready.v1 from im.knowledge.events
-> event carries low-sensitive refs only, no chunk preview / raw text
-> resolve target chunk via knowledge-ingestion-service.ListKnowledgeChunks
-> enqueue redacted-preview task into vector_embedding_tasks

NEXUSIM_VECTOR_INDEX_SERVICE_MODE=embedding-producer
-> choose NEXUSIM_VECTOR_EMBEDDING_PRODUCER_SOURCE=file|knowledge
-> file source reads controlled JSONL embedding tasks from NEXUSIM_VECTOR_EMBEDDING_TASKS_FILE
-> knowledge source calls knowledge-ingestion-service.ListKnowledgeChunks and uses redacted preview
-> enqueue redacted-preview tasks into vector_embedding_tasks

NEXUSIM_VECTOR_INDEX_SERVICE_MODE=embedding-worker
-> choose NEXUSIM_VECTOR_EMBEDDING_SOURCE=file|knowledge|postgres
-> postgres source claims persisted vector_embedding_tasks rows with FOR UPDATE SKIP LOCKED
-> verify input_hash matches in-memory input_text before model call
-> call model-gateway InvokeEmbedding
-> keep embedding_values only inside vector-index internal worker/backend handoff
-> write vector_items / vector_index_jobs / vector_outbox through existing UpsertVectorItem
-> only persist source refs, input hash, embedding hash, dimension, model ref and visibility metadata
```

该 chunk-consumer / producer / worker 是 first-stage worker 边界验证入口。`chunk-consumer`
不从事件中获取 preview，而是用低敏 event refs 调 `ListKnowledgeChunks` resolve
redacted preview；JSONL / knowledge source 不得把 `input_text` 或 embedding vector array 写入
PostgreSQL、outbox、metrics、logs 或 Kafka payload；PostgreSQL task source 只允许持久化
`input_preview_redacted`、input hash 和低敏 source / visibility metadata，不允许保存 raw
document、source URI、object key 或 embedding vector array。

Tombstone：

```text
knowledge.chunk.tombstoned.v1 or memory/search tombstone
-> lock vector_items by source ref
-> write vector_tombstones
-> delete / mask backend vectors
-> mark TOMBSTONED
-> write vector.item.tombstoned.v1
```

Retrieval query：

```text
retrieval-gateway SearchVectors
-> verify caller service identity
-> policy / visibility guard
-> query vector backend
-> post-filter metadata against vector_items
-> return source refs + scores only
```

Rebuild：

```text
RequestVectorRebuild
-> create rebuild job and checkpoints
-> optional first-stage backfill from completed vector_embedding_tasks
-> re-embed redacted preview through model-gateway
-> upsert configured provider backend partition by partition
-> verify tombstones and visibility metadata
```

第一版 provider backfill 支持 checkpoint cursor 分页续跑：每批最多处理
`NEXUSIM_VECTOR_REBUILD_BACKFILL_BATCH_SIZE` 条 matching completed embedding task；如果还有
下一页，会推进 `vector_rebuild_checkpoints.cursor_value` 并让后续 RunOnce 继续 claim
RUNNING rebuild；只有没有下一页时才把 rebuild job 标记完成。生产级 rebuild / backfill 仍
需要 provider repair、更多 upstream replay 和真实 provider smoke。

定向 repair / 本地 smoke 可设置 `NEXUSIM_VECTOR_REBUILD_TENANT_ID`，让 rebuild worker
只 claim 指定 tenant 的 rebuild job；默认不设置时仍按全局 worker 语义 claim。PostgreSQL
embedding task 新入队使用数据库 `now()` 写 `available_at / created_at / updated_at`，避免
双机 / 多进程环境中应用时钟和数据库时钟轻微漂移导致刚入队任务短暂不可 claim。

## 10. 与 retrieval-gateway 的边界

`retrieval-gateway` 是唯一面向 RAG / summary / Agent 的检索入口。

```text
RAG / summary / Agent
-> retrieval-gateway
-> search-service / memory-service / vector-index-service
-> EvidencePack
```

`vector-index-service` 不返回 EvidencePack，不返回文本，不做 citation verifier。它只返回
source refs、scores 和 metadata。EvidencePack 合成、dedupe、citation refs 和最终可见性收敛仍由
retrieval-gateway 完成。

## 11. 与 model-gateway / knowledge-ingestion 的边界

- `knowledge-ingestion-service` 拥有 source / chunk lifecycle 和 delete proof。
- `model-gateway` 拥有 embedding provider route、budget、fallback 和 provider metadata。
- `vector-index-service` 拥有 vector backend metadata、upsert/delete/rebuild 状态。

Embedding 输入必须来自受控 source / chunk refs；vector-index 不接受任意 caller 提交 raw text。

## 12. 一致性和事务

强一致边界：

- vector item metadata、index job 和 outbox 同事务。
- tombstone、backend delete request metadata 和 outbox 同事务。
- rebuild checkpoint 和 job status 同事务。

最终一致边界：

- vector backend upsert/delete 与 PostgreSQL metadata 之间可能短暂不一致；worker / repair 必须补偿。
- model-gateway embedding 失败会暂停 job，不得写半成品 ACTIVE vector。
- retrieval-gateway query 必须 post-filter PostgreSQL metadata，不能只信 backend metadata。

## 13. 幂等、重试和 repair

| 场景 | 幂等键 | 重试策略 | repair |
| --- | --- | --- | --- |
| UpsertVectorItem | source_id + source_version + model_ref | replay 返回 vector item | metadata audit |
| Embedding request | chunk_hash + model_ref | model-gateway retry / pause | embedding rebuild |
| Backend upsert | vector_item_id + model_ref | bounded retry + DLQ | backend repair |
| Tombstone | vector_item_id + delete_proof_id | replay returns tombstone | tombstone repair |
| Rebuild | collection + model_ref + rebuild id | checkpoint resume | rebuild audit |
| OutboxRelay | event_id | bounded retry + DLQ | outbox repair |

## 14. 安全边界

- `SearchVectors` 只允许 retrieval-gateway 或 explicitly allowlisted internal service identity。
- Upsert / tombstone caller 必须是 knowledge-ingestion、memory、search 或 controlled worker。
- request body 不能覆盖 trusted tenant / caller metadata。
- raw text、embedding vector arrays、source URI、object key、private connector ids 不进入 events / metrics / logs。
- provider secret 和 vector backend credential 只能来自 secret manager / env ref。
- public debug endpoint 不输出 collection ids、source ids、vector ids、hashes、tenant labels 或 backend endpoint。
- delete proof 一旦记录，后续 query 必须 fail closed，直到 rebuild 证明无 stale vector。

## 15. SLO 和指标

第一阶段不写生产 SLO，只保留本地 / 面试展示指标：

```text
vector_item_total{collection_type,status,tombstone_status}
vector_index_job_total{job_type,status,failure_class}
vector_backend_upsert_total{backend_type,status}
vector_backend_delete_total{backend_type,status}
vector_search_total{collection_type,status}
vector_rebuild_checkpoint_total{collection_type,status}
vector_outbox_total{status}
```

metrics label 禁止使用 tenant_id、collection_id、source_id、chunk_id、vector_item_id、
source_ref_hash、chunk_hash、backend_vector_id、trace_id 或 request_id。

## 16. 测试方案

| 测试 | 目标 |
| --- | --- |
| domain unit | vector item state、tombstone invariant、visibility fail closed |
| app unit | caller allowlist、policy deny、embedding failure、idempotency |
| backend adapter | local vector adapter upsert/search/delete、malformed metadata fail closed |
| PostgreSQL integration | item + job + tombstone + outbox transaction |
| event builder / outbox relay | 不输出 raw text、vector array、source URI、object key、secret；只发布低敏 ref / hash metadata |
| query test | retrieval caller only，post-filter tombstone / visibility |
| smoke | chunk.ready -> vector indexed -> SearchVectors refs -> tombstone -> no result |

## 17. Runbook

运行模式：

```text
NEXUSIM_VECTOR_INDEX_SERVICE_MODE=grpc
NEXUSIM_VECTOR_INDEX_SERVICE_MODE=rebuild-worker
NEXUSIM_VECTOR_INDEX_SERVICE_MODE=outbox-relay
NEXUSIM_VECTOR_INDEX_SERVICE_MODE=chunk-consumer
NEXUSIM_VECTOR_INDEX_SERVICE_MODE=embedding-producer
NEXUSIM_VECTOR_INDEX_SERVICE_MODE=embedding-worker
```

当前已实现 runtime mode：`noop`、`grpc`、`rebuild-worker`、`outbox-relay`、
`chunk-consumer`、`embedding-producer`、`embedding-worker`。`backend-worker`、`cleanup` 仍是目标态规划，
不应写入当前本地 smoke 命令。

`chunk-consumer` 第一版必需配置：

```text
NEXUSIM_KAFKA_BROKERS=127.0.0.1:9092
NEXUSIM_VECTOR_CHUNK_TOPIC=im.knowledge.events
NEXUSIM_VECTOR_CHUNK_CONSUMER_GROUP=nexusim-vector-index-chunk-consumer
NEXUSIM_KNOWLEDGE_INGESTION_GRPC_ADDR=127.0.0.1:10740
NEXUSIM_VECTOR_EMBEDDING_MODEL_REF=deterministic-embedding-v1
NEXUSIM_VECTOR_EMBEDDING_DIMENSION=8
```

`chunk-consumer` 当前只支持 `knowledge.chunk.ready.v1`，unsupported / malformed
event fail-closed，不 commit Kafka offset。由于 `knowledge-ingestion-service`
尚无独立 Kafka schema / relay smoke，本模式当前只有 focused tests，不宣称端到端
Kafka chunk-consumer smoke 已完成。

`embedding-producer` 第一版必需配置：

```text
NEXUSIM_PG_DSN=...
NEXUSIM_VECTOR_EMBEDDING_PRODUCER_SOURCE=file|knowledge
NEXUSIM_VECTOR_EMBEDDING_TASKS_FILE=H:\NexusIM\loadtest-results\vector-embedding-tasks.jsonl
NEXUSIM_VECTOR_EMBEDDING_BATCH_SIZE=50
```

Producer 不允许使用 `postgres` 作为 source，避免 self-loop；它只能从 file /
knowledge 等 upstream source 读取 redacted-preview task 并写入 PostgreSQL queue。

`embedding-worker` 第一版必需配置：

```text
NEXUSIM_PG_DSN=...
NEXUSIM_MODEL_GATEWAY_GRPC_ADDR=127.0.0.1:10770
NEXUSIM_VECTOR_EMBEDDING_SOURCE=file|knowledge|postgres
NEXUSIM_VECTOR_EMBEDDING_TASKS_FILE=H:\NexusIM\loadtest-results\vector-embedding-tasks.jsonl
NEXUSIM_VECTOR_EMBEDDING_BATCH_SIZE=50
NEXUSIM_VECTOR_EMBEDDING_MODEL_TIMEOUT=5s
```

Optional provider backend 配置：

```text
# Local metadata-backed verification backend; no raw vector array is persisted.
NEXUSIM_VECTOR_PROVIDER_BACKEND=postgres-test

# Real optional pgvector backend.
NEXUSIM_VECTOR_PROVIDER_BACKEND=pgvector
NEXUSIM_VECTOR_PGVECTOR_DSN=postgres://...   # optional; empty uses NEXUSIM_PG_DSN
NEXUSIM_VECTOR_PGVECTOR_TABLE=vector_embedding_items
NEXUSIM_VECTOR_PGVECTOR_DIMENSION=8
NEXUSIM_VECTOR_PGVECTOR_ENSURE_SCHEMA=true
```

该模式要求目标 PostgreSQL 已安装 `vector` extension，或者允许 `EnsureSchema` 执行
`CREATE EXTENSION IF NOT EXISTS vector`。默认本地 `postgres:16-alpine` 不带 pgvector，
所以普通开发 / CI 不应默认开启该 profile。可选本地 overlay 为
`deploy/local/docker-compose.pgvector.yml`，会在 `localhost:15432` 暴露独立
`nexusim-pgvector`，不替换默认 `nexusim-postgres`。

Optional rebuild provider backfill 配置：

```text
NEXUSIM_VECTOR_INDEX_SERVICE_MODE=rebuild-worker
NEXUSIM_VECTOR_REBUILD_BACKFILL_SOURCE=embedding-tasks
NEXUSIM_VECTOR_REBUILD_BACKFILL_BATCH_SIZE=100
NEXUSIM_VECTOR_REBUILD_TENANT_ID=tenant_1 # optional scoped claim for focused smoke / repair
NEXUSIM_MODEL_GATEWAY_GRPC_ADDR=127.0.0.1:10770
NEXUSIM_VECTOR_PROVIDER_BACKEND=postgres-test

# Or for real pgvector backend:
# NEXUSIM_VECTOR_PROVIDER_BACKEND=pgvector
NEXUSIM_VECTOR_PGVECTOR_DSN=postgres://...
```

该模式只读取本服务 PostgreSQL queue 中 `COMPLETED` 的 `vector_embedding_tasks`，并重新
embedding redacted preview 后写 provider backend。未配置 provider backend 时 fail-fast；
`postgres-test` 只确认 metadata backend state，不保存 raw vector array；默认
`rebuild-worker` 不启用该 backfill。`loadtest/vectorembedding/run-local-smoke.ps1
-IncludeRebuildBackfill` 已覆盖本地 `postgres-test` provider backfill；真 pgvector /
Milvus / OpenSearch provider backfill smoke 后置。

Knowledge source 配置：

```text
NEXUSIM_VECTOR_EMBEDDING_SOURCE=knowledge
NEXUSIM_KNOWLEDGE_INGESTION_GRPC_ADDR=127.0.0.1:10740
NEXUSIM_VECTOR_EMBEDDING_TENANT_ID=tenant_1
NEXUSIM_VECTOR_EMBEDDING_SOURCE_ID=ksrc_1
NEXUSIM_VECTOR_EMBEDDING_DOCUMENT_ID=kdoc_1
NEXUSIM_VECTOR_EMBEDDING_MODEL_REF=deterministic-embedding-v1
NEXUSIM_VECTOR_EMBEDDING_DIMENSION=8
```

JSONL 任务文件和 knowledge redacted-preview source 只用于 first-stage worker 验证；
PostgreSQL task source 是第一版持久 task queue，可 claim / complete / claim-timeout retry，
并已有 first-stage `embedding-producer` 能把 file / knowledge source 转入该 queue。
first-stage `chunk-consumer` 能消费低敏 `knowledge.chunk.ready.v1` refs 并通过公开 API
resolve redacted preview 入队；完整 Kafka / outbox chunk consumer smoke 仍依赖
knowledge outbox relay / schema 后续收口，不能宣称为生产 chunk consumer。

PostgreSQL task source 配置：

```text
NEXUSIM_VECTOR_EMBEDDING_SOURCE=postgres
NEXUSIM_VECTOR_EMBEDDING_TENANT_ID=tenant_1   # optional local scope
NEXUSIM_VECTOR_EMBEDDING_CLAIM_TIMEOUT=30s
```

operator：

```text
vector-item-audit
vector-backend-audit
vector-tombstone-audit
vector-rebuild-request
vector-rebuild-audit
vector-outbox-repair
```

## 18. 验收标准

进入编码前：

- 本 SDD v0.1 draft 被复核，无 P0/P1。
- `vector-index-service` brief 指向本 SDD。
- 明确它不直接服务 RAG，不保存 raw text，不绕过 retrieval / policy / tombstone。

进入 first smoke 前：

- proto / migration / 六层 skeleton / cmd runtime 已落。
- local vector backend adapter、metadata repository 和 tombstone tests 通过。
- retrieval-gateway 调用 `SearchVectors` 的 refs-only smoke 通过。
- tombstone / delete proof 后不再返回 stale vector refs。
