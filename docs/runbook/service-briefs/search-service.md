# search-service

## 当前状态

- SDD v0.1 draft 已存在：`docs/sdd/search-service.md`。
- 第一实现切片已继续推进：`search_service.proto`、PostgreSQL core migration、六层 package skeleton、`SearchMessages` app / domain / gRPC adapter、projection usecase skeleton、PostgreSQL repository、`grpc` runtime mode、timeline decoder / consumer、worker overlay 和 projection smoke runner 已落地。
- 2026-06-19 已在 clean commit `f2a57516` 跑通真实 projection smoke：`conversation.timeline.events -> search-service timeline-consumer -> PostgreSQL projection -> SearchMessages`，覆盖 persisted / edited / revoked / deleted / member boundary。
- 2026-06-24 已补 PostgreSQL FTS lexical backend first path：`SearchMessages` 使用
  `plainto_tsquery('simple') + to_tsvector('simple', searchable_text)` 和既有 GIN index，
  不再使用 `ILIKE` substring fallback；新增真实 PG token-search 集成测试用于覆盖
  `launch` 不会命中 `launchable` 这类子串。
- 当前 active slice 已转入 `memory-service v0.1` 设计 / 契约；search-service 后续保留 EvidencePack 前置字段深化和搜索 hardening。
- 短期不以生产级完整系统测试或生产级 HA 作为 v0.1 阻塞；v0.1 验证重点是切片级本地检查、projection / visibility / tombstone smoke 和 EvidencePack 前置字段。
- 定位是搜索索引、成员可见窗口过滤、tombstone 和 EvidencePack 前置服务。
- 不绑定具体搜索中间件；索引后端必须通过 port，可本地/PG 起步，后续按 ADR 替换。
- 第一版目标：消费 `conversation.timeline.events`，维护消息索引、可见性窗口和 tombstone。
- 第一版查询：提供 `SearchMessages`，使用 PostgreSQL FTS 词法匹配，并按 tenant / conversation / join_seq / leave_seq / deleted/revoked 状态过滤。
- 第一版不调用 LLM，不做 RAG 回答，不直接读 message-service / conversation-service 私有表。
- 后续 memory / retrieval-gateway / RAG / summary / Agent / skill-registry / MCP gateway / action-executor 都必须复用它的 visibility / tombstone 语义。
- 搜索索引不直接承载每个 viewer 的成员窗口膨胀字段；v0.1 需要保留 message `conversation_seq`、source event、status、permission / visibility version，并配合 membership visibility projection 做查询过滤。
- `memory-service` 会在 search 边界稳定后消费同一事实事件，生成带 source refs 和版本语义的 group memory；search-service 不保存长期 memory。
- Agent / skill-registry / MCP gateway / action-executor 不能绕过 search/retrieval 的 visibility 和 tombstone 语义。真实业务写动作必须通过 policy-service tool policy precheck，再进入 proposal / approval / action-executor / audit。

## 后续

- projection smoke：真实 timeline event -> PG projection -> `SearchMessages` 已通过。
- search smoke：发消息可搜，编辑更新，撤回/删除不可见，退群后不可见，stranger 不可见已通过。
- PostgreSQL FTS first path：`SearchMessages` 已从 substring match 改为 token-based lexical search；OpenSearch / BM25 provider 仍是后续可替换后端，不在当前切片宣称完成。
- EvidencePack 前置 smoke：搜索结果必须带 source message id、conversation seq、source event id 和 visibility version。
- 后续链路：memory / group memory -> retrieval-gateway / EvidencePack -> RAG / summary -> Agent -> skill-registry -> mcp-gateway/tool-gateway -> action-executor。
