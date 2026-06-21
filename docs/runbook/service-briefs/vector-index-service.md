# vector-index-service

状态：product-active / 第一版 implementation slice 已落地。已同步 service registry、
proto、migration、六层 runtime、Docker、Prometheus 和 Grafana。

设计入口：`docs/sdd/vector-index-service.md`。

Stage-switch 记录：`docs/runbook/stage-switch/vector-index-service.md`。

定位：向量索引写入和重建服务。仅当 embedding、Milvus / pgvector / OpenSearch
vector 写入、rebuild 和 backfill 逻辑复杂到影响 retrieval / memory 时才独立。

边界：

- 不直接服务 RAG 请求；retrieval-gateway 仍是唯一检索入口。
- 不保存 raw message body；只保存可删除、可重建、带 visibility metadata 的向量引用。
- 删除 / 撤回 / retention 必须传播 tombstone 并可验证。
- model provider 和 embedding 成本治理可经 model-gateway / control-plane 接入。

当前已覆盖：

- `UpsertVectorItem`、`TombstoneVectorItem`、`SearchVectors`、`GetVectorIndexJob`。
- `RequestVectorRebuild` 第一版 rebuild job / checkpoint request path；first-stage
  `rebuild-worker` 已能 claim PENDING rebuild、推进 checkpoint 并写
  `vector.rebuild.started.v1` / `vector.rebuild.completed.v1` 低敏 outbox event。
- local / PostgreSQL-backed test vector adapter；PostgreSQL backend state adapter
  已显式记录 backend item ACTIVE / DELETED 状态，`SearchVectors` 必须 join ACTIVE
  backend state 才返回 refs；Milvus / pgvector / OpenSearch vector 后置。
- `vector_outbox -> im.vector.events` 第一版 outbox relay、低敏 Kafka schema、
  PENDING / PUBLISHED / retry / DLQ 状态推进、同 aggregate 顺序阻塞和 focused
  builder / PostgreSQL store 测试。
- `loadtest/vectorindex` 已覆盖公开 gRPC upsert / tombstone / search、
  `RequestVectorRebuild -> rebuild-worker -> rebuild started/completed outbox`、
  真实 outbox relay 和 Kafka `im.vector.events` readback；runbook 见
  `docs/runbook/loadtest/vector-index-service/README.md`。
- `loadtest/knowledgevector` 已覆盖 `knowledge-ingestion-service` chunk manifest
  经公开 gRPC handoff 到 vector upsert，再由 vector search 读回。
- 确认 raw text、embedding vector array、source URI、object key 不进入事件 / metrics / relay payload。
- first-stage `embedding-worker` 已落地：支持本地 JSONL 任务源，以及通过
  `knowledge-ingestion-service.ListKnowledgeChunks` 公开 API 拉取 redacted preview 的
  knowledge 任务源；随后调 `model-gateway.InvokeEmbedding`，再调用现有 vector upsert
  写入 hash / refs / visibility metadata。PostgreSQL、outbox、metrics 和 relay payload
  仍不保存 raw text 或 embedding vector array。该模式用于本地 smoke / worker 边界验证，
  不是生产 parser / chunk consumer。
- `loadtest/vectorembedding` 已跑通 embedding worker 真实进程 smoke：公开 gRPC
  准备 knowledge chunk manifest，启动 embedding worker，经 `model-gateway.InvokeEmbedding`
  写入 vector metadata，再通过 `SearchVectors` 验证结果；runner 不手工 upsert，也不读私表。
- first-stage PostgreSQL embedding task queue 已落：`vector_embedding_tasks` 只保存
  redacted preview、input hash 和低敏 source / visibility metadata；`embedding-worker`
  支持 `NEXUSIM_VECTOR_EMBEDDING_SOURCE=postgres`，使用 `FOR UPDATE SKIP LOCKED` claim、
  claim-timeout retry 和 `COMPLETED` 标记。
- first-stage `embedding-producer` 已落：支持从 file / knowledge source 读取
  redacted-preview task 并写入 PostgreSQL queue；producer 不允许使用 postgres source，
  避免 self-loop。`loadtest/vectorembedding` 已跑通 producer -> queue -> worker
  的本地多进程链路。
- first-stage `chunk-consumer` runtime 已落：消费低敏
  `knowledge.chunk.ready.v1` refs 后，通过
  `knowledge-ingestion-service.ListKnowledgeChunks` 公开 API resolve redacted preview，
  再写入 PostgreSQL embedding queue。已支持 schema 化 protobuf `KnowledgeEvent` 与旧
  JSON fallback；已知非 chunk knowledge events 会 skip + commit，unknown / malformed
  仍 fail-closed；focused tests 和真实 Kafka `knowledge_outbox -> im.knowledge.events
  -> chunk-consumer -> vector_embedding_tasks` smoke 已通过。
- vector-index 内部 `ModelGatewayClient` 已保留 `InvokeEmbedding.embedding_values`
  用于后续真实 vector backend handoff；公开 API / metadata PostgreSQL / outbox / metrics
  仍只暴露 hash / ref / dimension 等低敏字段。
- first-stage optional pgvector adapter 包已落在
  `services/vector-index-service/internal/infrastructure/pgvector`：提供 schema 初始化、
  upsert、delete、search 和 focused unit tests。该 adapter 当前不接默认 `cmd`，不进入
  普通 PostgreSQL migration；真实 pgvector profile / smoke 后置。

后续待办：memory / search chunk consumer、pgvector runtime wiring / smoke、
真实 Milvus / OpenSearch backend、provider backend rebuild / backfill worker、
provider backend repair。
