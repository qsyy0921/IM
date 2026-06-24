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
- 2026-06-24 retrieval vector backend opt-in smoke 已通过：
  `loadtest/retrieval/run-local-smoke.ps1 -IncludeVectorBackend` 启动真实
  vector-index-service gRPC，runner 通过公开 `UpsertVectorItem` 写入低敏
  `MEMORY_EVENT` vector metadata，retrieval-gateway 再通过公开 `SearchVectors`
  获取 refs-only `VECTOR_ITEM`。该 smoke 使用 PostgreSQL metadata / `postgres-test`
  backend state，不代表 pgvector / Milvus / OpenSearch provider 已完成。
- optional pgvector adapter 包和 compose overlay；默认不启用，不进入普通 migration。
- 2026-06-25 `loadtest/vectorembedding` 已补 `preflight-pgvector` phase，
  `run-local-pgvector-smoke.ps1` 会在启动完整链路前先验证 pgvector PostgreSQL
  连接、`vector` extension 可用性和 table identifier 配置；不可用时快速
  fail-closed 并写低敏 summary，不伪造 provider smoke 通过。
- 2026-06-25 `loadtest/vectorembedding` 已补 `preflight-opensearch-vector` phase 和
  `run-local-opensearch-vector-preflight.ps1`：先验证 OpenSearch endpoint、index
  是否存在，以及 mapping 中指定字段是否为 `knn_vector` 且 dimension 匹配；endpoint
  禁止携带 credentials / query / fragment；不可用或 mapping drift 时 fail-closed
  并写低敏 summary，不写入 OpenSearch，也不伪造真实 provider smoke。

证据入口：`docs/runbook/loadtest/vector-index-service/` 和 `loadtest/vectorembedding`。

后续：memory / search chunk consumer、真实 pgvector smoke、真实 OpenSearch vector
backend smoke、Milvus backend、provider repair、真 provider backfill smoke。
