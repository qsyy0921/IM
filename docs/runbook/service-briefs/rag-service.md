# rag-service

状态：foundation-active / first read-only answer path / RAG adapter smoke passed.

定位：RAG 问答边界服务。它只消费 retrieval-gateway 返回的
`EvidencePack`，不直接读 message / conversation / search / memory 私有表，
不执行 Agent 动作，不绕过 policy / visibility。

当前已落：

- `rag_service.proto`、SDD、六层 skeleton、`grpc` runtime mode、debug
  `/metrics`、registry / Docker / compose / Prometheus / Grafana wiring
- app usecase 调用 retrieval port，基于 EvidencePack 生成 deterministic
  extractive answer；无 evidence 返回 `INSUFFICIENT_EVIDENCE`
- response 保留 citations、EvidencePack、`generated_by_llm=false`
- retrieval-gateway 公开 proto RPC client、app / gRPC / cmd focused tests
- `loadtest/rag`、`tools/run-ai-eval-rag-adapter.ps1` 和真实本地
  `retrieval-gateway -> rag-service` adapter smoke 已通过：
  `docs/runbook/loadtest/rag-service/loadtest-report-20260619-rag-adapter-smoke.md`

下一步：

- 设计 LLM provider 边界和 citation verifier，继续保持 EvidencePack 输入
  和 source refs 输出。
- 后续进入 `summary-service` / Agent 时仍只能消费 EvidencePack。
