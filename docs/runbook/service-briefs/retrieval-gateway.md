# retrieval-gateway

状态：foundation-active / first EvidencePack smoke passed。

定位：统一 search + memory 的检索入口，向 RAG / summary / Agent 提供
`EvidencePack`。它不直接读业务库，不调用 LLM，不执行 Agent 动作。

当前已落：

- `docs/sdd/retrieval-gateway.md`
- `api/proto/nexusim/retrieval/v1/retrieval_gateway.proto`
- `services/retrieval-gateway` 六层 skeleton、`grpc` runtime mode、debug `/metrics`
- app usecase：调用 search / memory ports，归一成 EvidencePack
- infrastructure RPC clients：只依赖 search / memory 公开 proto
- 可选 policy-service retrieval precheck：配置 `NEXUSIM_POLICY_GRPC_ADDR` 后，
  app 层在 search / memory 前通过本地 `PolicyPort` 调用 policy-service
  `CheckToolAction`，deny / approval-required / dependency unavailable 均 fail-closed
  且不会继续查下游
- registry / Docker runtime / local compose / Prometheus / Grafana foundation-active wiring
- `loadtest/retrieval` 和真实本地 smoke：seed search / memory projection rows
  -> search-service grpc + memory-service grpc -> retrieval-gateway grpc
  -> `RetrieveEvidence` -> EvidencePack 同时包含 `SEARCH_MESSAGE` 和
  `MEMORY_EVENT`

下一步：

- focused tests 和 `check-local` 收口。
- EvidencePack 字段 hardening：rerank score、source coverage、dedupe reason。
- 后续由 `rag-service` / `summary-service` / `agent-service` 消费 EvidencePack，不绕过 retrieval-gateway。
