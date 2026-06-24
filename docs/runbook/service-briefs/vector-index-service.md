# vector-index-service

状态：product-active / first-stage vector metadata、outbox relay、embedding queue、
chunk-consumer、embedding-worker 和 rebuild backfill 已落。

设计入口：`docs/sdd/vector-index-service.md`。
Stage-switch：`docs/runbook/stage-switch/vector-index-service.md`。

定位：向量索引写入、provider backend handoff、rebuild 和 backfill 服务。

边界：
- 不直接服务 RAG 请求；retrieval-gateway 仍是唯一检索入口。
- 不保存 raw message body；默认 PostgreSQL metadata / outbox / metrics 不保存 raw text 或 embedding vector array。
- 删除 / 撤回 / retention 必须传播 tombstone 并可验证。
- embedding 必须经 model-gateway；knowledge chunk 必须经 knowledge public API。

已覆盖：
- `UpsertVectorItem` / `TombstoneVectorItem` / `SearchVectors` / `GetVectorIndexJob` / `RequestVectorRebuild`。
- `vector_outbox -> im.vector.events` relay 和真实 Kafka readback smoke。
- `knowledge.chunk.ready.v1 -> chunk-consumer -> vector_embedding_tasks` handoff。
- PostgreSQL embedding task queue、embedding-producer、embedding-worker。
- `postgres-test` provider sink 和 rebuild backfill focused smoke。
- retrieval-gateway 已通过公开 `SearchVectors` adapter first path 消费 vector item
  ref / source ref hash / visibility metadata，并以 `VECTOR_ITEM` EvidencePack source
  暴露给 RAG / summary / Agent；vector-index-service 仍不直接服务 RAG 请求。
- optional pgvector adapter 包和 compose overlay；默认不启用，不进入普通 migration。

证据入口：`docs/runbook/loadtest/vector-index-service/` 和 `loadtest/vectorembedding`。

后续：memory / search chunk consumer、pgvector smoke、Milvus / OpenSearch backend、
provider repair、真 provider backfill smoke。
