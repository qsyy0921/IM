# rag-service

状态：foundation-active / provider boundary + citation verifier first pass.

定位：RAG 问答边界服务。它只消费 retrieval-gateway 返回的
`EvidencePack`，不直接读 message / conversation / search / memory 私有表，
不执行 Agent 动作，不绕过 policy / visibility。

当前已落：

- `rag_service.proto`、SDD、六层 skeleton、`grpc` runtime mode、debug
  `/metrics`、registry / Docker / compose / Prometheus / Grafana wiring
- app usecase 调用 retrieval port 和 `AnswerProvider` port；默认本地
  extractive provider 基于 EvidencePack 生成 deterministic answer
- response 保留 citations、EvidencePack、`generated_by_llm=false`
- provider 输出后统一运行 citation verifier，无法匹配 EvidencePack 则
  fail closed
- retrieval-gateway 公开 proto RPC client、app / gRPC / cmd focused tests
- `loadtest/rag`、`tools/run-ai-eval-rag-adapter.ps1` 和真实本地
  `retrieval-gateway -> rag-service` adapter smoke 已通过：
  `docs/runbook/loadtest/rag-service/loadtest-report-20260619-rag-adapter-smoke.md`

下一步：

- 外部 LLM adapter 后续仍必须走 AnswerProvider port 和 citation verifier。
- `summary-service` 已进入 foundation-active；summary / Agent 仍只能消费 EvidencePack。
