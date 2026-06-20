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
- local / PostgreSQL-backed test vector adapter；Milvus / pgvector / OpenSearch
  vector 后置。
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
- first-stage `embedding-worker` 已落地：从本地 JSONL 任务源读取受控输入文本，
  调 `model-gateway.InvokeEmbedding`，再调用现有 vector upsert 写入 hash / refs /
  visibility metadata；PostgreSQL、outbox、metrics 和 relay payload 仍不保存 raw text
  或 embedding vector array。该模式用于本地 smoke / worker 边界验证，不是生产
  parser / chunk consumer。

后续待办：真实 knowledge / memory / search chunk consumer、真实 Milvus / pgvector /
OpenSearch backend、provider backend rebuild / backfill worker、embedding task 持久队列。
