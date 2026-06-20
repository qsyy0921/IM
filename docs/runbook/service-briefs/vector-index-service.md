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
- local / PostgreSQL-backed test vector adapter；Milvus / pgvector / OpenSearch
  vector 后置。
- `vector_outbox -> im.vector.events` 第一版 outbox relay、低敏 Kafka schema、
  PENDING / PUBLISHED / retry / DLQ 状态推进、同 aggregate 顺序阻塞和 focused
  builder / PostgreSQL store 测试。
- 确认 raw text、embedding vector array、source URI、object key 不进入事件 / metrics / relay payload。

后续待办：embedding worker、rebuild worker、真实 Kafka relay smoke、Milvus /
pgvector / OpenSearch backend、vector-index handoff smoke。
