# vector-index-service Brief

状态：product-active / vector metadata and provider handoff。

## 已落

- `UpsertVectorItem`、`TombstoneVectorItem`、`SearchVectors`、`GetVectorIndexJob`、
  `RequestVectorRebuild`。
- `vector_outbox -> im.vector.events` relay、Kafka readback smoke。
- `knowledge.chunk.ready.v1 -> chunk-consumer -> vector_embedding_tasks`。
- PostgreSQL embedding task queue、embedding-producer、embedding-worker。
- `postgres-test` provider sink、rebuild backfill focused smoke。
- Retrieval-gateway 已通过公开 `SearchVectors` 消费 refs-only `VECTOR_ITEM`。
- pgvector / OpenSearch vector / Milvus preflight 和 provider readiness matrix。

## 边界

- 不直接服务 RAG 请求；retrieval-gateway 是唯一检索入口。
- 不保存 raw message body；metadata / outbox / metrics 不保存 raw text 或 embedding vector。
- 删除 / 撤回 / retention 必须传播 tombstone。
- embedding 必须经 model-gateway；knowledge chunk 必须经 knowledge public API。

## 下一步

- 真实 pgvector、OpenSearch vector、Milvus provider smoke、provider repair、real backfill。
