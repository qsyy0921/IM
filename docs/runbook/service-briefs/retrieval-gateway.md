# retrieval-gateway

状态：foundation-active / first EvidencePack boundary implementation in progress。

定位：统一 search + memory 的检索入口，向 RAG / summary / Agent 提供
`EvidencePack`。它不直接读业务库，不调用 LLM，不执行 Agent 动作。

当前已落：

- `docs/sdd/retrieval-gateway.md`
- `api/proto/nexusim/retrieval/v1/retrieval_gateway.proto`
- `services/retrieval-gateway` 六层 skeleton、`grpc` runtime mode、debug `/metrics`
- app usecase：调用 search / memory ports，归一成 EvidencePack
- infrastructure RPC clients：只依赖 search / memory 公开 proto
- registry / Docker runtime / local compose / Prometheus / Grafana foundation-active wiring

下一步：

- focused tests 和 `check-local` 收口。
- 真实 `search-service + memory-service -> retrieval-gateway RetrieveEvidence` smoke。
- 后续由 `rag-service` / `summary-service` / `agent-service` 消费 EvidencePack，不绕过 retrieval-gateway。
