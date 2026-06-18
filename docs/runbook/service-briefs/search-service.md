# search-service

## 当前状态

- SDD v0.1 draft 已存在：`docs/sdd/search-service.md`。
- 服务代码尚未实现；`search-service` v0.1 是 9 个服务必要收口后，向 AI 大模型应用后端转进的第一步。
- 当前 active slice：收敛 SDD，落 proto / migration / 六层 skeleton，完成 timeline projection 和 `SearchMessages` 最小查询。
- 短期不以生产级完整系统测试或生产级 HA 作为 v0.1 阻塞；v0.1 验证重点是切片级本地检查、projection / visibility / tombstone smoke 和 EvidencePack 前置字段。
- 定位是搜索索引、成员可见窗口过滤、tombstone 和 EvidencePack 前置服务。
- 不绑定具体搜索中间件；索引后端必须通过 port，可本地/PG 起步，后续按 ADR 替换。
- 第一版目标：消费 `conversation.timeline.events`，维护消息索引、可见性窗口和 tombstone。
- 第一版查询：提供 `SearchMessages`，按 tenant / conversation / join_seq / leave_seq / deleted/revoked 状态过滤。
- 第一版不调用 LLM，不做 RAG 回答，不直接读 message-service / conversation-service 私有表。
- 后续 memory / retrieval-gateway / RAG / Agent / MCP / Skill 都必须复用它的 visibility / tombstone 语义。

## 后续

- `search_service.proto`、migration、六层 skeleton。
- timeline projection：message persisted / edited / revoked / deleted + member boundary。
- search smoke：发消息可搜，编辑更新，撤回/删除不可见，退群后不可见。
- EvidencePack 前置 smoke：搜索结果必须带 source message id、conversation seq、source event id 和 visibility version。
- 后续链路：memory / group memory -> retrieval-gateway / EvidencePack -> RAG / summary -> Agent -> MCP tool surface -> Skill runtime / registry。
