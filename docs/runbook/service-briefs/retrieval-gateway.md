# retrieval-gateway

状态：foundation-active / first EvidencePack smoke passed / EvidencePack field hardening first pass。

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

下一步：把 selected cross-group / temporal cases 扩到 RAG / summary / Agent
service-stack consumption；后续不绕过 retrieval-gateway。
