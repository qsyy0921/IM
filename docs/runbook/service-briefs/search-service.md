# search-service

## 当前状态

- SDD v0.1 draft 已存在：`docs/sdd/search-service.md`。
- 第一实现切片已开始：`search_service.proto`、PostgreSQL core migration、六层 package skeleton、`SearchMessages` app / domain / gRPC adapter 和 projection usecase skeleton 已落地。
- 当前 active slice 下一步：补 PostgreSQL repository、timeline consumer / decoder、真实 `SearchMessages` 查询和 projection smoke。
- 短期不以生产级完整系统测试或生产级 HA 作为 v0.1 阻塞；v0.1 验证重点是切片级本地检查、projection / visibility / tombstone smoke 和 EvidencePack 前置字段。
- 定位是搜索索引、成员可见窗口过滤、tombstone 和 EvidencePack 前置服务。
- 不绑定具体搜索中间件；索引后端必须通过 port，可本地/PG 起步，后续按 ADR 替换。
- 第一版目标：消费 `conversation.timeline.events`，维护消息索引、可见性窗口和 tombstone。
- 第一版查询：提供 `SearchMessages`，按 tenant / conversation / join_seq / leave_seq / deleted/revoked 状态过滤。
- 第一版不调用 LLM，不做 RAG 回答，不直接读 message-service / conversation-service 私有表。
- 后续 memory / retrieval-gateway / RAG / summary / Agent / skill-registry / MCP gateway / action-executor 都必须复用它的 visibility / tombstone 语义。
- 搜索索引不直接承载每个 viewer 的成员窗口膨胀字段；v0.1 需要保留 message `conversation_seq`、source event、status、permission / visibility version，并配合 membership visibility projection 做查询过滤。
- `memory-service` 会在 search 边界稳定后消费同一事实事件，生成带 source refs 和版本语义的 group memory；search-service 不保存长期 memory。
- Agent / skill-registry / MCP gateway / action-executor 不能绕过 search/retrieval 的 visibility 和 tombstone 语义。真实业务写动作必须通过 policy-service tool policy precheck，再进入 proposal / approval / action-executor / audit。

## 后续

- PostgreSQL repository：消息索引、成员可见窗口、checkpoint 和 tombstone 状态同事务更新。
- timeline projection：message persisted / edited / revoked / deleted + member boundary。
- `SearchMessages` 真实查询：tenant / conversation / keyword / join_seq / leave_seq / tombstone 过滤。
- search smoke：发消息可搜，编辑更新，撤回/删除不可见，退群后不可见。
- EvidencePack 前置 smoke：搜索结果必须带 source message id、conversation seq、source event id 和 visibility version。
- 后续链路：memory / group memory -> retrieval-gateway / EvidencePack -> RAG / summary -> Agent -> skill-registry -> mcp-gateway/tool-gateway -> action-executor。
