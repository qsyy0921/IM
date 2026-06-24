# retrieval-gateway

状态：foundation-active / EvidencePack graph edge + profile evidence expansion first pass passed。

定位：统一 search + memory 的检索入口，向 RAG / summary / Agent 提供
`EvidencePack`。它不直接读业务库，不调用 LLM，不执行 Agent 动作。

当前已落：

- `docs/sdd/retrieval-gateway.md`
- `api/proto/nexusim/retrieval/v1/retrieval_gateway.proto`
- `services/retrieval-gateway` 六层 skeleton、`grpc` runtime mode、debug `/metrics`
- app usecase：调用 search / memory ports，归一成 EvidencePack
- infrastructure RPC clients：只依赖 search / memory 公开 proto
- 可选 policy-service retrieval precheck：配置 `NEXUSIM_POLICY_GRPC_ADDR` 后，
  app 层在 search / memory 前通过 `CheckToolAction` fail-closed 检查
- registry / Docker runtime / local compose / Prometheus / Grafana foundation-active wiring
- `loadtest/retrieval` 和真实本地 smoke：search + memory projection
  -> retrieval-gateway `RetrieveEvidence` -> `SEARCH_MESSAGE` + `MEMORY_EVENT`；
  2026-06-20 扩展到 cross-group source refs / speaker attribution 和
  expired / superseded / future memory query-seq exclusion。
- EvidencePack 字段 hardening first pass：`rerank_score`、`dedupe_reason`、
  `source_coverage` 已落地，app / gRPC tests 覆盖排序、去重和覆盖统计。
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

下一步：继续把 group memory extraction、profile recompute / repair 和后续 rerank /
coverage 增强通过 EvidencePack 暴露给 RAG / summary / Agent，仍不绕过 retrieval-gateway。
