# rag-service

状态：foundation-active / first read-only answer path.

定位：RAG 问答边界服务。它只消费 retrieval-gateway 返回的
`EvidencePack`，不直接读 message / conversation / search / memory 私有表，
不执行 Agent 动作，不绕过 policy / visibility。

当前已落：

- `api/proto/nexusim/rag/v1/rag_service.proto`
- `docs/sdd/rag-service.md`
- `services/rag-service` 六层 skeleton、`grpc` runtime mode、debug `/metrics`
- app usecase：调用 retrieval port，基于 EvidencePack 生成 deterministic
  extractive answer
- 无 evidence 时返回 `INSUFFICIENT_EVIDENCE`，不编造答案
- response 保留 citations、EvidencePack、`generated_by_llm=false`
- infrastructure RPC client：只依赖 retrieval-gateway 公开 proto
- registry / Docker runtime / local compose / Prometheus / Grafana
  foundation-active wiring
- app / gRPC / cmd focused tests

下一步：

- 跑真实 `retrieval-gateway -> rag-service AnswerQuestion` 本地 smoke。
- 补 AI eval execution adapter，让低敏 case 真正调用 RAG。
- 后续如果接 LLM provider，必须保持 EvidencePack 输入和 source refs 输出。
