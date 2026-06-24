# search-service

## 当前状态

- SDD v0.1 draft 已存在：`docs/sdd/search-service.md`。
- 第一实现切片已继续推进：`search_service.proto`、PostgreSQL core migration、六层 package skeleton、`SearchMessages` app / domain / gRPC adapter、projection usecase skeleton、PostgreSQL repository、`grpc` runtime mode、timeline decoder / consumer、worker overlay 和 projection smoke runner 已落地。
- 2026-06-19 已在 clean commit `f2a57516` 跑通真实 projection smoke：`conversation.timeline.events -> search-service timeline-consumer -> PostgreSQL projection -> SearchMessages`，覆盖 persisted / edited / revoked / deleted / member boundary。
- 2026-06-24 已补 PostgreSQL FTS lexical backend first path：`SearchMessages` 使用
  `plainto_tsquery('simple') + to_tsvector('simple', searchable_text)` 和既有 GIN index，
  不再使用 `ILIKE` substring fallback；新增真实 PG token-search 集成测试用于覆盖
  `launch` 不会命中 `launchable` 这类子串。
- 2026-06-24 已补显式 OpenSearch / BM25 candidate backend first path：
  `NEXUSIM_SEARCH_BACKEND=opensearch` 时 `SearchMessages` 会通过 OpenSearch
  `POST /<index>/_search` + `match(searchable_text, operator=and)` 召回
  `conversation_id/message_id` 候选，再回到 PostgreSQL projection 做 membership
  visibility、tombstone、after_seq 和 conversation filter hydration；OpenSearch
  配置错误、请求失败或返回 malformed hit 时返回 `SEARCH_UNAVAILABLE`，不静默回退 PG。
- 2026-06-24 已补 opt-in OpenSearch backend smoke 入口：
  `loadtest/search/run-local-smoke.ps1 -UseOpenSearchBackend` 会把 search-service
  gRPC 切到 `opensearch` backend，并由 runner 把已投影的低敏 search document
  同步进临时 OpenSearch index，再通过 search-service gRPC 验证候选召回 +
  PostgreSQL visibility / tombstone hydration。该路径不改变默认 PostgreSQL FTS
  smoke，不把 OpenSearch 当权限事实源。
- 当前机器 Docker API 和 `127.0.0.1:9200` OpenSearch 均不可用，因此本轮只验证到
  runner / 脚本 / adapter focused tests；真实 OpenSearch 进程 smoke 尚未归档通过报告。
- 2026-06-25 已补 search-service 自有 OpenSearch rebuild operator first path：
  `NEXUSIM_SEARCH_SERVICE_MODE=opensearch-rebuild` 只读取 search-service 的
  PostgreSQL projection，按 tenant 批量扫描非 tombstone / 非空 searchable_text 文档；
  默认 dry-run，只有 `NEXUSIM_SEARCH_REBUILD_EXECUTE=true` 才通过 OpenSearch
  create-index / NDJSON bulk / refresh 写外部 index。该 operator 不跨服务读私表，
  不把 OpenSearch 当事实源，真实 OpenSearch 进程验证仍待运行环境恢复。
- 2026-06-25 已补 OpenSearch index contract hardening：rebuild operator 创建 index 时写入
  `nexusim.search.messages.v1` strict mapping 和 `_meta` owner / source projection；
  已存在 index 会先读取 `_mapping` 校验 mapping version、`dynamic=strict` 和必需字段类型，
  drift 时返回 `SEARCH_UNAVAILABLE`，不继续 bulk 写入。
- 当前 active slice 已转入 backend architecture + AI / Agent / RAG demo path；search-service 后续保留 EvidencePack 前置字段深化和搜索 hardening。
- 短期不以生产级完整系统测试或生产级 HA 作为 v0.1 阻塞；v0.1 验证重点是切片级本地检查、projection / visibility / tombstone smoke 和 EvidencePack 前置字段。
- 定位是搜索索引、成员可见窗口过滤、tombstone 和 EvidencePack 前置服务。
- 不绑定具体搜索中间件；索引后端必须通过 port，可本地/PG 起步，后续按 ADR 替换。
- 第一版目标：消费 `conversation.timeline.events`，维护消息索引、可见性窗口和 tombstone。
- 第一版查询：提供 `SearchMessages`，默认使用 PostgreSQL FTS 词法匹配；显式
  OpenSearch backend 只做候选召回，仍按 tenant / conversation / join_seq /
  leave_seq / deleted/revoked 状态在 PostgreSQL 投影层过滤。
- 第一版不调用 LLM，不做 RAG 回答，不直接读 message-service / conversation-service 私有表。
- 后续 memory / retrieval-gateway / RAG / summary / Agent / skill-registry / MCP gateway / action-executor 都必须复用它的 visibility / tombstone 语义。
- 搜索索引不直接承载每个 viewer 的成员窗口膨胀字段；v0.1 需要保留 message `conversation_seq`、source event、status、permission / visibility version，并配合 membership visibility projection 做查询过滤。
- `memory-service` 会在 search 边界稳定后消费同一事实事件，生成带 source refs 和版本语义的 group memory；search-service 不保存长期 memory。
- Agent / skill-registry / MCP gateway / action-executor 不能绕过 search/retrieval 的 visibility 和 tombstone 语义。真实业务写动作必须通过 policy-service tool policy precheck，再进入 proposal / approval / action-executor / audit。

## 后续

- projection smoke：真实 timeline event -> PG projection -> `SearchMessages` 已通过。
- search smoke：发消息可搜，编辑更新，撤回/删除不可见，退群后不可见，stranger 不可见已通过。
- PostgreSQL FTS first path：`SearchMessages` 已从 substring match 改为 token-based lexical search。
- OpenSearch / BM25 candidate backend first path：显式 `NEXUSIM_SEARCH_BACKEND=opensearch`
  已接入；opt-in OpenSearch smoke 入口和 service-owned rebuild operator first path
  已补齐；mapping drift focused hardening 已补齐；后续仍需在可用 OpenSearch 进程上
  归档真实 smoke，再做容量曲线和 provider-grade 运维。
- EvidencePack 前置 smoke：搜索结果必须带 source message id、conversation seq、source event id 和 visibility version。
- 后续链路：memory / group memory -> retrieval-gateway / EvidencePack -> RAG / summary -> Agent -> skill-registry -> mcp-gateway/tool-gateway -> action-executor。
