# knowledge-ingestion-service SDD v0.1 Draft

## 1. 服务定位

`knowledge-ingestion-service` 是 NexusIM AI / 知识基础设施中的企业知识导入服务。
它负责外部文件、网页、知识库记录和管理上传文档的导入状态、解析、chunk manifest、
source refs、可见性 metadata、delete proof、增量重建和导入审计。

职责：

- 拥有 `knowledge_sources`、`knowledge_documents`、`knowledge_chunks`、
  `knowledge_ingestion_jobs`、`knowledge_delete_proofs` 和 `knowledge_outbox`。
- 从 media-service / object storage ref、网页 ref、admin upload ref 或 connector ref 创建导入任务。
- 管理 parser worker candidate、chunking、metadata extraction、embedding 请求和 vector rebuild handoff。
- 为每个 chunk 维护 source ref、version、visibility、tenant scope、retention、delete proof 和 audit refs。
- 向 vector-index-service / retrieval-gateway / audit-service 发布低敏 ingestion events。

不负责：

- 不保存媒体二进制或对象原文件；原始文件由 media-service / object storage 拥有。
- 不直接服务 RAG / search 查询；retrieval-gateway 仍是唯一检索入口。
- 不直接写 vector store；vector-index-service 或 retrieval 内部 port 负责向量索引。
- 不直接调用外部模型 provider；embedding / extraction / rerank 通过 model-gateway 或 parser port。
- 不绕过 policy-service、retrieval-gateway、EvidencePack、tombstone 或 delete proof。
- 不执行 Agent action，不决定回答，不持久化 raw prompt / model output。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游 | admin-service / control-plane-service | 创建知识源、发布 ingest policy、触发 rebuild |
| 上游 | media-service | 提供对象 ref、MIME、hash、size、virus scan / transcode status |
| 上游 | external connector / operator | 网页、企业知识库、手工上传导入请求 |
| 同步依赖 | policy-service | source access、tenant scope、data class、ingestion action precheck |
| 同步依赖 | model-gateway | 可选 embedding、metadata extraction、chunk summary |
| 同步依赖 | parser worker / Python worker | 文件解析和 chunk candidate；只返回候选 |
| 异步下游 | vector-index-service | chunk indexed / tombstone / rebuild handoff |
| 异步下游 | retrieval-gateway / audit-service | 低敏 source / chunk / delete-proof events |
| 事实源 | PostgreSQL | source、document、chunk、job、delete proof、outbox |

第一版可先只接本地 operator / admin source ref；生产 connector、云盘、网页爬虫和 provider-grade
parser 后置。

## 3. 六层 DDD 包结构

```text
services/knowledge-ingestion-service/
  cmd/knowledge-ingestion-service/
  internal/{api,app,domain,infrastructure,types,trigger}/
```

| 层 | 本服务内容 |
| --- | --- |
| `api` | gRPC adapter，verified admin/service metadata，稳定错误映射 |
| `app` | CreateKnowledgeSource、SubmitIngestionJob、GetIngestionJob、ListChunks、RequestRebuild |
| `domain` | source 状态机、chunk manifest、visibility、delete proof、parser candidate rules |
| `infrastructure` | PostgreSQL repository、media/policy/model/vector clients、parser adapters |
| `types` | command、DTO、错误码、枚举、低敏 metadata |
| `trigger` | ingestion worker、parser worker adapter、embedding worker、outbox relay、cleanup |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `KnowledgeSource` | 一个知识来源，如 media object、web page、connector doc | tenant scoped；带 owner 和 policy |
| `KnowledgeDocument` | 一次 source version 的解析结果 | source hash + version 幂等 |
| `KnowledgeChunk` | 可检索知识片段 | 必须有 source ref、version、visibility、delete proof 状态 |
| `IngestionJob` | 导入 / rebuild / delete 任务 | 状态 append-only；worker 可重试 |
| `DeleteProof` | 删除 / 撤回 / retention 证明 | tombstone 后不得再索引 |
| `KnowledgeOutboxEvent` | 低敏事件 | 只通过 outbox relay 发布 |

Source 类型：

```text
MEDIA_OBJECT
WEB_PAGE
ADMIN_UPLOAD
CONNECTOR_RECORD
MANUAL_MARKDOWN
```

Job 类型：

```text
INGEST
REINGEST
REBUILD_CHUNKS
REFRESH_METADATA
TOMBSTONE
DELETE_PROOF_REPAIR
```

Job 状态：

```text
PENDING -> PARSING -> CHUNKING -> EMBEDDING_REQUESTED -> INDEX_HANDOFF -> DONE
PENDING/PARSING/CHUNKING/EMBEDDING_REQUESTED/INDEX_HANDOFF -> FAILED
FAILED -> RETRYING
PENDING/PARSING/CHUNKING -> CANCELED
DONE/FAILED -> TOMBSTONED
```

## 5. 同步 API 契约

```text
rpc CreateKnowledgeSource(CreateKnowledgeSourceRequest) returns (CreateKnowledgeSourceResponse)
rpc SubmitIngestionJob(SubmitIngestionJobRequest) returns (SubmitIngestionJobResponse)
rpc GetIngestionJob(GetIngestionJobRequest) returns (GetIngestionJobResponse)
rpc GetKnowledgeDocument(GetKnowledgeDocumentRequest) returns (GetKnowledgeDocumentResponse)
rpc ListKnowledgeChunks(ListKnowledgeChunksRequest) returns (ListKnowledgeChunksResponse)
rpc RequestKnowledgeRebuild(RequestKnowledgeRebuildRequest) returns (RequestKnowledgeRebuildResponse)
rpc TombstoneKnowledgeSource(TombstoneKnowledgeSourceRequest) returns (TombstoneKnowledgeSourceResponse)
```

`CreateKnowledgeSource` 请求字段：

```text
tenant_id, requester_user_id, source_type
source_ref, source_uri_hash, media_object_ref
owner_ref, visibility_scope, data_class
content_hash, mime_type, size_bytes
source_version, retention_policy_ref
idempotency_key
correlation_id, causation_id, trace_id
```

`SubmitIngestionJob` 请求字段：

```text
tenant_id, source_id, source_version
job_type, parser_profile, chunk_profile
embedding_policy_ref, vector_policy_ref
requested_by, idempotency_key
```

`ListKnowledgeChunks` 默认只返回低敏 metadata：

```text
chunk_id, source_id, document_id, chunk_index
chunk_hash, chunk_preview_redacted
source_ref, visibility_scope, data_class
chunk_version, status, tombstone_status
```

完整 chunk text 只能在受控 internal flow 中传给 model-gateway / vector-index adapter，且必须经过
policy 和 retention check。普通管理查询默认不返回 chunk 正文。

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | source ref、MIME、chunk profile、version、data class 非法 | 否 |
| `PERMISSION_DENIED` | source / owner / tenant / policy 不允许导入 | 否 |
| `FAILED_PRECONDITION` | media 未扫描、source 已 tombstone、parser profile 不兼容 | 否 |
| `ALREADY_EXISTS` | idempotency replay command hash 冲突 | 否 |
| `NOT_FOUND` | source / document / job / chunk 不存在或不可见 | 否 |
| `UNAVAILABLE` | media、policy、parser、model 或存储暂不可用 | 是 |

## 6. 数据和可见性边界

`knowledge-ingestion-service` 拥有知识导入事实，但不拥有消息事实或媒体二进制。

Chunk 必须携带：

```text
tenant_id
source_id
document_id
source_type
source_ref
source_version
chunk_index
chunk_hash
visibility_scope
data_class
owner_ref
policy_version
delete_proof_id
retention_policy_ref
```

可见性来源：

| source_type | 可见性来源 |
| --- | --- |
| `MEDIA_OBJECT` | media-service object ACL + policy-service |
| `WEB_PAGE` | admin / connector policy + crawl scope |
| `ADMIN_UPLOAD` | admin operation scope + tenant policy |
| `CONNECTOR_RECORD` | connector source ACL + mapped group/user refs |
| `MANUAL_MARKDOWN` | owner / tenant / explicit visibility scope |

如果 visibility metadata 缺失、policy version stale 或 source ACL 无法确认，导入 worker 必须 fail closed，不得产出 ACTIVE chunk。

## 7. 异步事件契约

| 事件 | Topic | 分区键 | 说明 |
| --- | --- | --- | --- |
| `knowledge.source.created.v1` | `im.knowledge.events` | `tenant_id:source_id` | source 已登记 |
| `knowledge.document.parsed.v1` | `im.knowledge.events` | `tenant_id:document_id` | document metadata / chunks manifest 已生成 |
| `knowledge.chunk.ready.v1` | `im.knowledge.events` | `tenant_id:chunk_id` | chunk 可进入 embedding / index |
| `knowledge.chunk.tombstoned.v1` | `im.knowledge.events` | `tenant_id:chunk_id` | chunk 已 tombstone |
| `knowledge.ingestion.failed.v1` | `im.knowledge.events` | `tenant_id:job_id` | 导入失败 |
| `knowledge.delete_proof.recorded.v1` | `im.knowledge.events` | `tenant_id:source_id` | delete proof 已记录 |

事件 payload 只包含 source / document / chunk / job id、hash refs、visibility version、
data class、failure class、trace refs。禁止包含原文件、chunk 正文、网页正文、provider body、
parser raw error、private URL query、credential 或 token。

## 8. 数据库设计

第一版表：

```text
knowledge_sources
knowledge_documents
knowledge_chunks
knowledge_ingestion_jobs
knowledge_delete_proofs
knowledge_outbox
```

关键字段：

```text
knowledge_sources:
tenant_id, source_id, source_type, source_ref, source_ref_hash,
owner_ref, visibility_scope, data_class, content_hash,
source_version, retention_policy_ref, status, created_at, updated_at

knowledge_documents:
tenant_id, document_id, source_id, source_version,
parser_profile, mime_type, size_bytes, page_count, language,
document_hash, parse_status, parser_failure_class, created_at

knowledge_chunks:
tenant_id, chunk_id, document_id, source_id, source_version,
chunk_index, chunk_hash, chunk_text_encrypted_ref,
chunk_preview_redacted, visibility_scope, data_class,
policy_version, tombstone_status, delete_proof_id,
embedding_status, vector_status, created_at, updated_at

knowledge_ingestion_jobs:
tenant_id, job_id, source_id, source_version, job_type,
status, retry_count, next_retry_at, failure_class, public_error,
idempotency_key, requested_by, created_at, completed_at

knowledge_delete_proofs:
tenant_id, delete_proof_id, source_id, source_version,
proof_type, proof_ref_hash, requested_by, reason_class, created_at

knowledge_outbox:
event_id, tenant_id, aggregate_id, event_type, event_version,
partition_key, payload_json, status, retry_count, next_retry_at, published_at
```

`chunk_text_encrypted_ref` 是可选字段。第一版可以只保存 chunk hash / redacted preview /
manifest；若保存 chunk text，必须使用独立加密和 retention policy，并禁止进入 events / metrics。

## 9. 核心流程

Create source：

```text
CreateKnowledgeSource
-> verify trusted metadata
-> policy-service ingestion precheck
-> validate source ref / media scan / data class
-> insert knowledge_sources
-> write knowledge.source.created.v1 outbox
```

Ingest job：

```text
SubmitIngestionJob
-> lock source version
-> create job PENDING
-> ingestion worker loads source ref via allowed adapter
-> parser worker returns parse candidate
-> Go app validates parser output schema / size / data class
-> chunker creates deterministic chunk manifest
-> write documents + chunks + outbox in transaction
-> request embedding via model-gateway or mark pending for vector-index
```

Tombstone:

```text
TombstoneKnowledgeSource
-> policy/admin precheck
-> record delete proof
-> mark source/document/chunks TOMBSTONED
-> write knowledge.chunk.tombstoned.v1 / delete_proof event
-> vector-index consumes tombstone and removes vectors
```

## 10. Parser / Python Worker 边界

Python parser / algorithm worker 只能返回候选：

```text
parsed_text_ref or page_text candidates
metadata candidates
chunk boundary candidates
language / title / section candidates
confidence
parser_version
```

Go service 必须负责：

- source authorization and policy precheck。
- job state transition。
- candidate schema validation。
- chunk hash / version / source ref assignment。
- delete proof / tombstone propagation。
- outbox / audit / retry / DLQ。

Python worker 禁止：

- 直接写 PostgreSQL、Kafka、vector store 或 object store。
- 直接调用 model provider 绕过 model-gateway。
- 直接读取 message / conversation / search / memory 私有表。
- 把 raw parser error、secret、credential 或 private URL 写入事件 / metrics。

## 11. 与 vector-index / retrieval 的边界

`knowledge-ingestion-service` 只产生可索引 chunks 和 lifecycle events。

```text
knowledge-ingestion-service
-> knowledge.chunk.ready.v1
-> vector-index-service or retrieval internal vector port
-> retrieval-gateway
-> EvidencePack
```

它不直接服务 `RetrieveEvidence`，也不绕过 retrieval-gateway 给 RAG / Agent 提供 chunks。

Vector index 必须能够根据以下字段删除或重建：

```text
source_id
document_id
chunk_id
source_version
chunk_hash
delete_proof_id
tombstone_status
visibility_scope
policy_version
```

## 12. 一致性和事务

强一致边界：

- source create 和 `knowledge.source.created.v1` outbox 同事务。
- document / chunks manifest 和 `knowledge.document.parsed.v1` / `knowledge.chunk.ready.v1` 同事务。
- tombstone / delete proof / chunk status / outbox 同事务。
- idempotency replay 不创建重复 source、job 或 chunk manifest。

最终一致边界：

- parser、embedding、vector index 是异步 worker；失败进入 retry / DLQ / repair。
- media-service object update / retention delete 通过事件或 repair 对齐。
- retrieval-gateway 只看已 indexed 且未 tombstone 的可见 chunk。

## 13. 幂等、重试和 repair

| 场景 | 幂等键 | 重试策略 | repair |
| --- | --- | --- | --- |
| CreateKnowledgeSource | tenant + source_ref + source_version | replay 返回 source | source audit |
| SubmitIngestionJob | tenant + source_id + job_type + source_version | replay 返回 job | job retry / cancel |
| Parser worker | job_id + parser_version | bounded retry + DLQ | parser failure repair |
| Embedding request | chunk_hash + embedding_model | retry later | embedding rebuild |
| Tombstone | source_id + delete_proof_id | replay returns tombstone | delete proof repair |
| OutboxRelay | event_id | bounded retry + DLQ | outbox repair |

## 14. 安全边界

- API 只接受 verified admin / service metadata；request body 不能覆盖 tenant / requester。
- source URI 和 object key 不能进入 events、metrics、logs；只保存 hash/ref。
- 私有网页 / connector credential 只存在 secret manager / connector adapter，不进入本服务 DB。
- chunk preview 必须 redacted 且长度受限；默认管理查询不返回 chunk text。
- `data_class=SECURITY_SENSITIVE` 默认禁止外部 parser / external provider。
- parser worker output size、page count、chunk count 必须有上限。
- delete proof 记录后，chunk 不得再次进入 ACTIVE / INDEXED，除非有新 source_version 和新 proof 链。

## 15. SLO 和指标

第一阶段不写生产 SLO，只保留本地 / 面试展示指标：

```text
knowledge_source_total{source_type,status,data_class}
knowledge_ingestion_job_total{job_type,status,failure_class}
knowledge_chunk_total{status,tombstone_status,data_class}
knowledge_parser_latency_ms{parser_profile}
knowledge_embedding_request_total{status}
knowledge_outbox_total{status}
```

metrics label 禁止使用 tenant_id、source_id、document_id、chunk_id、owner_ref、URI、object key、
content hash、chunk hash、trace_id 或 request_id。

## 16. 测试方案

| 测试 | 目标 |
| --- | --- |
| domain unit | source/version state、chunk manifest、tombstone invariant |
| app unit | policy deny、media not scanned、idempotency、parser malformed fail closed |
| parser adapter | size limit、schema validation、raw error redaction |
| PostgreSQL integration | source + job + chunks + outbox transaction |
| event builder | 不输出 chunk text、source URI、object key、parser body |
| worker test | parser retry/DLQ、embedding request handoff、tombstone propagation |
| smoke | source ref -> chunk manifest -> chunk.ready event -> tombstone event |

## 17. Runbook

运行模式：

```text
NEXUSIM_KNOWLEDGE_INGESTION_SERVICE_MODE=grpc
NEXUSIM_KNOWLEDGE_INGESTION_SERVICE_MODE=ingestion-worker
NEXUSIM_KNOWLEDGE_INGESTION_SERVICE_MODE=parser-worker
NEXUSIM_KNOWLEDGE_INGESTION_SERVICE_MODE=embedding-worker
NEXUSIM_KNOWLEDGE_INGESTION_SERVICE_MODE=outbox-relay
NEXUSIM_KNOWLEDGE_INGESTION_SERVICE_MODE=cleanup
```

operator：

```text
knowledge-source-audit
knowledge-job-audit
knowledge-job-retry
knowledge-job-cancel
knowledge-delete-proof-audit
knowledge-outbox-repair
```

## 18. 验收标准

进入编码前：

- 本 SDD v0.1 draft 被复核，无 P0/P1。
- `knowledge-ingestion-service` brief 指向本 SDD。
- 明确它不拥有 media binary、不直接服务 retrieval、不直接写 vector store、不绕过 model-gateway / policy。

进入 first smoke 前：

- proto / migration / 六层 skeleton / cmd runtime 已落。
- 本地 document metadata + chunk manifest tests 通过。
- parser malformed / oversized / policy deny fail closed。
- chunk.ready / tombstone events 不包含 chunk text、source URI、object key 或 raw parser error。
