# search-service Brief

状态：foundation-active / message search projection and visibility filter。
First slice：`search_service.proto`、projection usecase skeleton、PostgreSQL projection。

## 已落

- `SearchMessages`、timeline consumer、PostgreSQL projection、projection smoke。
- persisted / edited / revoked / deleted / member boundary projection。
- PostgreSQL FTS lexical backend；旧 substring path 已移除。
- Explicit OpenSearch / BM25 candidate backend first path；OpenSearch 只召回候选，
  PostgreSQL projection 做最终 visibility / tombstone hydration。
- OpenSearch smoke preflight、service-owned rebuild operator、mapping drift hardening。

## 边界

- 不调用 LLM，不做 RAG 回答，不直接读 message / conversation 私表。
- Search index 不是权限事实源；visibility window、tombstone、membership boundary 必须保留。
- Memory / retrieval / RAG / Agent 必须复用 search visibility / tombstone 语义。
- 不绑定终局搜索中间件；外部 backend 通过 port / ADR 接入。

## 下一步

- 真实 OpenSearch 进程 smoke、capacity curve、EvidencePack 前置字段深化、
  provider-grade 运维。
