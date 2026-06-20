# knowledge-ingestion-service

状态：future / SDD v0.1 draft exists。当前不得创建
`services/knowledge-ingestion-service` 目录，直到完成 stage switch。

设计入口：`docs/sdd/knowledge-ingestion-service.md`。

定位：企业知识库导入服务，负责文件解析、网页导入、chunking、embedding
pipeline、权限 metadata、增量重建和导入审计。

边界：

- 不替代 media-service 的对象存储，也不替代 retrieval-gateway 的查询入口。
- 原始文件访问必须经过 policy / tenant / owner 校验。
- chunk / embedding 必须带 source ref、版本、visibility 和 delete proof。
- Python parser / embedding worker 只能返回候选，Go 服务拥有状态和审计。

第一切片建议：

- 先按 SDD 落 proto / migration / 六层 skeleton。
- 先支持本地文档 metadata + chunk manifest，不接真实 provider。
- 输出低敏 ingestion event，后续交给 vector-index / retrieval。
- 确认 source URI、object key、chunk text、parser raw error 不进入事件 / metrics。
