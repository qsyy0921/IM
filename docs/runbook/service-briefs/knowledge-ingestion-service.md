# knowledge-ingestion-service

状态：product-active / first-stage metadata、chunk manifest 和 knowledge outbox relay 已落。

定位：企业知识库导入服务，负责文件 / 网页导入、chunking、权限 metadata、
增量重建和导入审计。

边界：
- 不替代 media-service 对象存储，也不替代 retrieval-gateway 查询入口。
- 原始文件访问必须经过 policy / tenant / owner 校验。
- chunk / embedding 必须带 source ref、版本、visibility 和 delete proof。
- Python parser / embedding worker 只能返回候选；Go 拥有状态和审计。

已覆盖：
- `CreateKnowledgeSource` / `SubmitIngestionJob` / `GetIngestionJob` /
  `ListKnowledgeChunks`。
- 本地 document metadata + chunk manifest；不接真实 parser / crawler / vector provider。
- PG integration 覆盖 source + job + chunks + low-sensitive outbox transaction。
- outbox 不含 source URI、object key、chunk text、parser raw error。
- `knowledge_outbox -> im.knowledge.events` first-stage relay 和低敏 schema。
- `loadtest/knowledgevector` 和 vector chunk-consumer handoff smoke 已通过。

后续：parser worker、tombstone / delete proof、真实 connector、provider parser /
crawler handoff、完整 ingestion repair。
