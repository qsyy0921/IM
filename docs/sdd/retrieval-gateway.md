# retrieval-gateway SDD v0.1

## 定位

`retrieval-gateway` 是 AI 大模型应用底座中的统一检索边界。它位于
`search-service` / `memory-service` 之后，`rag-service` / `summary-service` /
`agent-service` 之前。

第一版目标：

- 对外提供 `RetrieveEvidence`。
- 聚合 `search-service.SearchMessages` 和 `memory-service.QueryMemoryEvents`。
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
-> retrieval-gateway EvidencePack
-> RAG / summary / Agent
```

RAG / summary / Agent 必须消费 `EvidencePack`，不能绕过 retrieval-gateway
直接调用 search / memory 或业务库。

## EvidencePack v0.1

`EvidencePack` 第一版包含：

- `pack_id`：由 tenant / user / query / limit / source flags 派生的稳定请求标识。
- `query`、`conversation_id`。
- `at_conversation_seq`：可选当前会话时点，用于调用 memory-service 的
  `QueryMemoryEvents.at_conversation_seq`；未传时第一版使用返回 search hit 的
  最大 `conversation_seq` 作为 fallback。
- `items`：
  - `SEARCH_MESSAGE`：来自 search-service 的 message hit。
  - `MEMORY_EVENT`：来自 memory-service 的 StructuredMemoryEvent。
- `source_refs`：message id、source event id、conversation id、seq、occurred time。
- `valid_from_seq` / `valid_to_seq`：memory temporal window。
- `temporal_status` / `review_state` / `extraction_version`。
- `rerank_score`：retrieval-gateway 本地统一排序分，第一版使用 search score /
  memory confidence clamp 到 `[0, 1]`。
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
- retrieval-gateway 默认只请求 ACTIVE memory；PENDING / SUPERSEDED 只能由
  调试、review 或指定调用方显式请求。
- retrieval-gateway 调 memory-service 时传递 `at_conversation_seq`，确保
  `valid_from_seq / valid_to_seq` current-only 过滤生效。
- retrieval-gateway 透传 verified auth metadata 和 request body auth context。
- 如果配置 `NEXUSIM_POLICY_GRPC_ADDR`，retrieval-gateway 会在调用 search / memory
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
- `NEXUSIM_POLICY_GRPC_ADDR`（可选；配置后启用 retrieval policy precheck）
- `NEXUSIM_RETRIEVAL_DEPENDENCY_TIMEOUT`

## 后续

- RAG / summary / Agent 接入前继续保持 EvidencePack smoke 和字段兼容性回归。
- policy-service 显式 retrieval check 已有 first-stage 可选 precheck；后续如果
  RAG / Agent 需要更细粒度的 retrieval policy，可在 policy-service 规则侧扩展。
- AI eval harness：retrieval miss、temporal version、attribution、permission leak。
- RAG / summary / Agent 只能在本边界稳定后进入。
