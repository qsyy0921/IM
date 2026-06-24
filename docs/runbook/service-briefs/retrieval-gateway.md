# retrieval-gateway

状态：foundation-active / EvidencePack vector source live smoke + configurable graph depth passed。

定位：统一 search + memory 的检索入口，向 RAG / summary / Agent 提供
`EvidencePack`。它不直接读业务库，不调用 LLM，不执行 Agent 动作。

当前已落：

- `docs/sdd/retrieval-gateway.md`
- `api/proto/nexusim/retrieval/v1/retrieval_gateway.proto`
- `services/retrieval-gateway` 六层 skeleton、`grpc` runtime mode、debug `/metrics`
- app usecase：调用 search / memory / vector ports，归一成 EvidencePack
- infrastructure RPC clients：只依赖 search / memory / vector 公开 proto
- 可选 policy-service retrieval precheck：配置 `NEXUSIM_POLICY_GRPC_ADDR` 后，
  app 层在 search / memory 前通过 `CheckToolAction` fail-closed 检查
- registry / Docker runtime / local compose / Prometheus / Grafana foundation-active wiring
- `loadtest/retrieval` 和真实本地 smoke：search + memory projection
  -> retrieval-gateway `RetrieveEvidence` -> `SEARCH_MESSAGE` + `MEMORY_EVENT`；
  2026-06-20 扩展到 cross-group source refs / speaker attribution 和
  expired / superseded / future memory query-seq exclusion。
- EvidencePack 字段 hardening first pass：`rerank_score`、`dedupe_reason`、
  `source_coverage` 已落地，app / gRPC tests 覆盖排序、去重和覆盖统计。
- 2026-06-25 retrieval positive smoke / adapter 已把 `source_coverage` 矩阵纳入
  低敏门禁：search / memory / profile 必须是 `RETURNED`，未启用 vector 时
  `VECTOR_ITEM` 必须是 `NOT_REQUESTED`；summary 只输出 source type、requested、
  candidate / returned / deduped count 和 status。
- EvidencePack -> memory-service current-only query 已落：默认 memory status 收敛为
  ACTIVE，显式 `at_conversation_seq` 透传给 memory-service；未传时使用 search hit
  最大 conversation seq 作为 first-stage recovery。
- 2026-06-23 retrieval-gateway optional live adapter first pass 已落：
  `run-ai-eval-retrieval-adapter.ps1` 运行 `loadtest/retrieval` 并把 EvidencePack
  source type、source refs、speaker attribution、stale/future memory exclusion、
  multi-hop actor/source chain 和 projection version 检查映射到 ai-eval cases。
- 2026-06-24 retrieval-gateway negative / miss adapter 已落：
  `run-ai-eval-retrieval-negative-adapter.ps1` 运行 `loadtest/retrievalnegative`，
  通过真实 `RetrieveEvidence` 覆盖 empty memory `source_coverage=EMPTY`、
  superseded memory 排除、source ref / dedupe reason 和 cross-tenant evidence
  isolation。完整 AI service-stack gate 已达到 51 passed / 0 skipped。
- 2026-06-24 EvidencePack memory graph edge 扩展已落：
  `RetrieveEvidence` 会对 memory hit 调用 memory-service 公开 `GetMemoryEvent`
  读取 current memory graph edges，并把 `EvidenceMemoryGraphEdge` 放入
  EvidencePack。memory lookup 失败会 fail-closed，不静默返回缺少 graph edge
  的证据包。app / gRPC / RPC focused tests 和 `loadtest/retrieval` 已覆盖
  `SUPPORTS` graph edge、跨群 source refs 和 actor attribution。
- 2026-06-24 EvidencePack profile aggregate evidence 已落：
  `RetrieveEvidence` 会调用 memory-service 公开 `ListProfileAggregates` 查询当前
  `auth.user_id` 的 ACTIVE profile aggregate，并作为 `PROFILE_AGGREGATE`
  evidence 放入 EvidencePack。profile lookup 失败会 fail-closed；app / RPC
  focused tests 和 `loadtest/retrieval` 已覆盖 profile subject、aggregate type/key、
  supporting memory ids 和 source coverage。
- 2026-06-24 `ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1`
  已通过真实 service-stack gate：RAG answer 和 Agent proposal 同时验证
  `DECISION` / `BLOCKER` / `FILE` 三类 group memory、6 个 source refs 和 3 个
  cross-group source refs 经 EvidencePack 保留。
- 2026-06-24 `ai-eval-rag-agent-demo-live-20260624-business-proposal-source-chain-gate-v1`
  已通过真实 service-stack gate：Agent 业务 proposal 对 `DECISION` / `TASK` /
  `STATUS` 三类 reviewed memory 保留 3 条 memory evidence、6 个 source refs 和
  3 个 cross-group source refs，且后续 approval / action audit 不绕过 EvidencePack。
- 2026-06-24 EvidencePack source-chain-aware rerank first pass 已落：
  `rerank_score` 不再只等同单条 source score；多来源 source refs、跨群 source refs、
  多 actor attribution、audience、memory graph edges 和 profile supporting memory
  ids 会提高 rerank score。focused app test 覆盖 limit 截断前多来源 memory chain
  优先于单条 search hit；`loadtest/retrieval` 低敏 summary 和 retrieval positive
  adapter 已新增 `source_chain_rerank_preserved` / `must_preserve_source_chain_rerank`
  断言。2026-06-24
  `ai-eval-service-stack-live-20260624-retrieval-source-chain-rerank-v2`
  已通过真实 service-stack gate：4 adapters / 27 cases / 27 passed / 0 failed /
  0 skipped，retrieval case 的 9 个断言全通过，`memory_rerank_score=1.29`
  高于 single search baseline。
- 2026-06-24 retrieval source-chain rerank first pass 已落：
  `retrieval-gateway.v1.hybrid-source-chain-rrf` 先收齐 search /
  memory / profile candidates，再按 lexical search、memory event、profile aggregate、
  source chain、memory graph、actor attribution、profile support 等 lane 做 RRF
  风格融合，最后叠加 source-chain bonus 后截断 limit。该实现为后续 BM25 /
  vector / graph provider 接入提供边界，不引入新中间件，也不把原始 provider
  score 直接跨 lane 比较。
- 2026-06-24 EvidencePack graph expansion 已从固定 depth=1 升级为可配置 depth：
  `NEXUSIM_RETRIEVAL_GRAPH_EXPANSION_DEPTH` 默认 1，允许 0..3，非法配置启动失败；
  strategy version 使用实际 depth，例如
  `retrieval-gateway.v1.hybrid-source-vector-rrf-graph-depth2`。retrieval-gateway
  会沿当前 memory hit 的 graph edge 通过 memory-service 公开 `GetMemoryEvent`
  按 bounded BFS 拉取相邻 memory event，并在 rerank / limit 截断前纳入候选；
  相邻 memory 必须满足当前请求 memory status，lookup 失败、不可见或 edge 不引用
  source memory 时 fail-closed。focused app tests 覆盖 depth=0 禁用、depth=2 二跳扩展、
  相邻 lookup fail-closed、superseded 邻接 memory 默认过滤和 malformed graph edge
  fail-closed。
- 2026-06-24 EvidencePack vector source adapter first path 已落：
  默认 strategy version 为 `retrieval-gateway.v1.hybrid-source-vector-rrf-graph-depth1`，
  配置 graph expansion depth 后会记录实际 depth。
  `RetrieveEvidence` 支持显式 `include_vector`，并要求调用方提供低敏
  `query_embedding_ref`、明确的 `vector_collection_types`、`vector_visibility_scope`
  和 `vector_policy_version`；
  retrieval-gateway 通过 vector-index-service 公开 `SearchVectors` 获取
  `VECTOR_ITEM` evidence，只返回 vector item ref、source ref hash、source service、
  collection type、visibility version 和 tombstone status，不传 raw text 或 embedding
  vector。vector lane 独立参与 RRF 风格融合，不与 BM25 / vector 原始分数直接比较；
  未配置 vector port、vector 依赖错误或 malformed vector result 均 fail-closed。
- 2026-06-24 `retrieval-vector-backend-smoke-20260624-223543` 已通过真实本地
  opt-in smoke：`run-local-smoke.ps1 -IncludeVectorBackend` 启动 search-service、
  memory-service、vector-index-service 和 retrieval-gateway，runner 通过
  vector-index-service 公开 `UpsertVectorItem` seed 低敏 vector metadata，并要求
  retrieval-gateway 通过公开 `SearchVectors` 返回 refs-only `VECTOR_ITEM`。
  EvidencePack source counts 为 search / memory / profile / vector 各 1 条，vector
  evidence 不携带 raw text 或 embedding vector。

下一步：继续把真实 BM25 backend、pgvector / Milvus / OpenSearch vector provider
smoke 和更细 source-chain / vector coverage 通过
EvidencePack 暴露给 RAG / summary / Agent，仍不绕过 retrieval-gateway。
