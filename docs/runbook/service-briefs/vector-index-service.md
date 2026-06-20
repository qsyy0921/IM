# vector-index-service

状态：future / SDD v0.1 draft / stage-switch approved。当前不得创建
`services/vector-index-service` 目录，直到实现切片同步切换 service registry、proto、
migration、runtime、Docker 和 observability。

设计入口：`docs/sdd/vector-index-service.md`。

Stage-switch 记录：`docs/runbook/stage-switch/vector-index-service.md`。

定位：向量索引写入和重建服务。仅当 embedding、Milvus / pgvector / OpenSearch
vector 写入、rebuild 和 backfill 逻辑复杂到影响 retrieval / memory 时才独立。

边界：

- 不直接服务 RAG 请求；retrieval-gateway 仍是唯一检索入口。
- 不保存 raw message body；只保存可删除、可重建、带 visibility metadata 的向量引用。
- 删除 / 撤回 / retention 必须传播 tombstone 并可验证。
- model provider 和 embedding 成本治理可经 model-gateway / control-plane 接入。

第一切片建议：

- 先按 SDD 落 proto / migration / 六层 skeleton。
- 第一版只做 `UpsertVectorItem`、`TombstoneVectorItem`、`SearchVectors`、
  `GetVectorIndexJob`。
- 先使用 local / PostgreSQL-backed test vector adapter；Milvus / pgvector /
  OpenSearch vector 后置。
- 确认 raw text、embedding vector array、source URI、object key 不进入事件 / metrics。
