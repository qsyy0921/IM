# knowledge-ingestion-service

状态：product-active，第一版 metadata + chunk manifest path 已落。

设计入口：`docs/sdd/knowledge-ingestion-service.md`。

Stage-switch 记录：`docs/runbook/stage-switch/knowledge-ingestion-service.md`。

定位：企业知识库导入服务，负责文件解析、网页导入、chunking、embedding
pipeline、权限 metadata、增量重建和导入审计。

边界：

- 不替代 media-service 的对象存储，也不替代 retrieval-gateway 的查询入口。
- 原始文件访问必须经过 policy / tenant / owner 校验。
- chunk / embedding 必须带 source ref、版本、visibility 和 delete proof。
- Python parser / embedding worker 只能返回候选，Go 服务拥有状态和审计。

已落第一切片：

- `knowledge_ingestion_service.proto`、PostgreSQL core migration、六层 skeleton、
  `grpc` runtime、Docker / Prometheus / Grafana wiring 已落。
- 当前覆盖 `CreateKnowledgeSource`、`SubmitIngestionJob`、`GetIngestionJob`、
  `ListKnowledgeChunks`。
- 第一版只支持本地 document metadata + chunk manifest，不接真实 parser /
  embedding / crawler / vector provider。
- 真实 PG 集成测试覆盖 source + job + chunks + low-sensitive outbox transaction。
- outbox payload 不包含 source URI、object key、chunk text、parser raw error。
- `loadtest/knowledgevector` 已通过公开 gRPC 证明 knowledge source / chunk
  manifest 可 handoff 到 `vector-index-service.UpsertVectorItem`，并由
  `SearchVectors` 搜到。
- 后续补 parser worker、embedding handoff、tombstone/delete proof、outbox relay
  和真实 connector。
