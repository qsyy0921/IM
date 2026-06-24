# retrieval-gateway SDD v0.1

## 定位

`retrieval-gateway` 是 AI 大模型应用底座中的统一检索边界。它位于
`search-service` / `memory-service` / `vector-index-service` 之后，`rag-service` /
`summary-service` / `agent-service` 之前。

第一版目标：

- 对外提供 `RetrieveEvidence`。
- 聚合 `search-service.SearchMessages`、`memory-service.QueryMemoryEvents` 和
  `vector-index-service.SearchVectors`。
- 返回统一 `EvidencePack`，保留 source refs、conversation seq、visibility
  version、memory temporal status、review state 和 projection version。
- 不直接读 message / conversation / delivery / memory / search 的私有表。
- 不调用 LLM，不生成回答，不执行 Agent 动作。

## 边界

`retrieval-gateway` 只做受控证据入口，不做事实源：

```text
conversation.timeline.events
-> search-service projection
-> memory-service projection
-> vector-index-service projection
-> retrieval-gateway EvidencePack
-> RAG / summary / Agent
```

RAG / summary / Agent 必须消费 `EvidencePack`，不能绕过 retrieval-gateway
直接调用 search / memory / vector 或业务库。

## EvidencePack v0.1

`EvidencePack` 第一版包含：

- `pack_id`：由 tenant / user / query / limit / source flags 派生的稳定请求标识。
- `query`、`conversation_id`。
- `at_conversation_seq`：可选当前会话时点，用于调用 memory-service 的
  `QueryMemoryEvents.at_conversation_seq`；未传时第一版使用返回 search hit 的
  最大 `conversation_seq` 作为显式查询边界。
- `items`：
  - `SEARCH_MESSAGE`：来自 search-service 的 message hit。
  - `MEMORY_EVENT`：来自 memory-service 的 StructuredMemoryEvent。
  - `PROFILE_AGGREGATE`：来自 memory-service 的当前用户 profile aggregate。
  - `VECTOR_ITEM`：来自 vector-index-service 的低敏 vector item 引用。
- `VECTOR_ITEM` 只包含 `vector_item_ref`、`vector_source_ref_hash`、
  `vector_source_service`、`vector_collection_type`、`vector_tombstone_status`、
  `visibility_version` 和 score；不包含 raw message / raw document / raw embedding。
- vector retrieval 必须显式请求 `include_vector=true`，并提供低敏
  `query_embedding_ref`、明确的 `vector_collection_types`、`vector_visibility_scope`、
  `vector_policy_version`。未配置 vector port、vector 依赖不可用或返回 malformed
  result 时 fail-closed，不静默降级或宽搜默认 collection。
- `source_refs`：message id、source event id、conversation id、seq、occurred time。
- `valid_from_seq` / `valid_to_seq`：memory temporal window。
- `temporal_status` / `review_state` / `extraction_version`。
- `rerank_score`：retrieval-gateway 本地统一排序分。当前策略版本为
  `retrieval-gateway.v1.hybrid-source-vector-rrf-graph-depth<N>`：先保留各 source 的本地分数
  clamp 到 `[0, 1]`，再叠加 source-chain 信号和 RRF 风格 lane fusion；`<N>`
  是本次运行实际 graph expansion depth。
  lane 包括 lexical search、vector item、memory event、profile aggregate、
  source chain、memory graph、actor attribution、profile support。该策略不直接比较
  BM25、vector、graph provider 的原始分数；新增 provider 必须以独立 lane / rank
  进入融合。
- `MEMORY_EVENT` graph expansion：第一阶段允许配置 depth 0..3，默认 depth=1。
  retrieval-gateway 对
  `QueryMemoryEvents` 命中的 memory event 调用 memory-service 公开
  `GetMemoryEvent` 读取 graph edges，并按 bounded BFS 沿 graph edge 读取相邻
  memory event；
  相邻 event 必须继续满足当前请求的 memory status 过滤。相邻 event 查不到、
  不可见、权限不满足或 graph edge 与 source event 不一致时 fail-closed，不返回
  缺失依赖的 EvidencePack。depth=0 表示不做相邻 event expansion，但仍保留当前
  memory hit 的 graph edges。该路径不直接读取 memory-service 私表。
- `dedupe_reason`：证据去重语义，第一版按 `source_type + source_id` 去重，保留
  first duplicate source。
- `source_coverage`：按 source type 返回 requested、candidate_count、
  returned_count、deduped_count 和状态（not requested / empty / returned /
  filtered），用于 RAG / Agent 判断 evidence 缺口，而不是把空结果误判成事实不存在。
- `search_projection_version` / `memory_projection_version`。
- `retrieval_version`。

## 权限与可见性

第一版依赖下游服务各自的 visibility / tombstone / member-window projection：

- search-service 负责消息可见性和 tombstone 过滤。
- memory-service 负责 memory event 可见性和 revoked / deleted 隐藏。
- vector-index-service 负责 vector item tombstone、source hash 和 visibility metadata；
  retrieval-gateway 仍只把 vector 作为可重建 projection evidence lane，不把它当事实源。
- retrieval-gateway 默认只请求 ACTIVE memory；PENDING / SUPERSEDED 只能由
  调试、review 或指定调用方显式请求。
- retrieval-gateway 调 memory-service 时传递 `at_conversation_seq`，确保
  `valid_from_seq / valid_to_seq` current-only 过滤生效。
- retrieval-gateway 透传 verified auth metadata 和 request body auth context。
- 如果配置 `NEXUSIM_POLICY_GRPC_ADDR`，retrieval-gateway 会在调用 search / memory / vector
  前通过 policy-service `CheckToolAction` 执行显式 retrieval precheck：
  `tool_name=retrieval.evidence`、`action=CALL`、`risk_level=LOW`。
  policy deny、requires approval 或 policy 依赖不可用时 fail-closed，不继续查询
  search / memory。

该 precheck 只通过 policy-service 公开 gRPC 契约完成，不直接读其它服务内部表。
未配置 `NEXUSIM_POLICY_GRPC_ADDR` 时，第一阶段仍可依赖 search / memory 下游
visibility projection 跑本地 smoke。

## 运行模式

第一版只有：

- `noop`
- `grpc`

环境变量：

- `NEXUSIM_RETRIEVAL_GATEWAY_MODE`
- `NEXUSIM_RETRIEVAL_GRPC_ADDR`
- `NEXUSIM_RETRIEVAL_DEBUG_ADDR`
- `NEXUSIM_SEARCH_GRPC_ADDR`
- `NEXUSIM_MEMORY_GRPC_ADDR`
- `NEXUSIM_VECTOR_GRPC_ADDR`（可选；配置后允许显式 vector evidence retrieval）
- `NEXUSIM_POLICY_GRPC_ADDR`（可选；配置后启用 retrieval policy precheck）
- `NEXUSIM_RETRIEVAL_DEPENDENCY_TIMEOUT`
- `NEXUSIM_RETRIEVAL_GRAPH_EXPANSION_DEPTH`（默认 1，允许 0..3；非法值启动失败）

## 后续

- RAG / summary / Agent 接入前继续保持 EvidencePack smoke 和字段兼容性回归。
- policy-service 显式 retrieval check 已有 first-stage 可选 precheck；后续如果
  RAG / Agent 需要更细粒度的 retrieval policy，可在 policy-service 规则侧扩展。
- AI eval harness：retrieval miss、temporal version、attribution、permission leak。
- RAG / summary / Agent 只能在本边界稳定后进入。
