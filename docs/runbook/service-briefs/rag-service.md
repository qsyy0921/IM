# rag-service

状态：foundation-active / provider boundary + citation verifier + Python worker candidate guard first pass.

定位：RAG 问答边界服务。它只消费 retrieval-gateway 返回的
`EvidencePack`，不直接读 message / conversation / search / memory 私有表，
不执行 Agent 动作，不绕过 policy / visibility。

当前已落：

- `rag_service.proto`、SDD、六层 skeleton、`grpc` runtime mode、debug
  `/metrics`、registry / Docker / compose / Prometheus / Grafana wiring
- app usecase 调用 retrieval port 和 `AnswerProvider` port；默认本地
  extractive provider 基于 EvidencePack 生成 deterministic answer
- 已补 guarded external HTTP LLM boundary：只由 EvidencePack 构造 prompt，
  provider failure 回退 extractive，unsafe / malformed output fail closed
- 已补可选 `python-worker` provider mode：Go 先生成 grounded answer，Python
  worker 只返回 candidate hash / citations；Go 校验 id、hash 和 citation
- response 保留 citations、EvidencePack、`generated_by_llm=false`
- provider 输出后统一运行 citation verifier，无法匹配 EvidencePack 则
  fail closed
- retrieval-gateway 公开 proto RPC client、app / gRPC / cmd focused tests
- `loadtest/rag`、`tools/run-ai-eval-rag-adapter.ps1` 和真实本地
  `retrieval-gateway -> rag-service` adapter smoke 已通过
- `AnswerQuestionRequest.at_conversation_seq` 已透传到 EvidencePack current-memory query。
- ai-eval 已补 RAG current-memory consumption CI-safe regression：`at_conversation_seq`
  必须传播，过期和 superseded memory 不得作为 current citation。

下一步：

- 后续 provider 仍走 AnswerProvider port、prompt guard 和 citation verifier。
- 补 current-memory service-stack live smoke，验证真实服务栈不引用 stale memory。
