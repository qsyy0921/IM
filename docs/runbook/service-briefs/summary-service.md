# summary-service

状态：foundation-active / provider boundary + citation verifier + Python worker candidate guard first pass.

定位：会话摘要边界服务。它只消费 retrieval-gateway 返回的 `EvidencePack`，
不直接读 message / conversation / search / memory 私有表，不执行 Agent 动
作，不绕过 policy / visibility。

当前已落：

- `summary_service.proto`、SDD、六层 skeleton、`grpc` runtime mode、debug
  `/metrics`、registry / Docker / compose / Prometheus / Grafana wiring
- app usecase 调用 retrieval port 和 `SummaryProvider` port；默认本地
  extractive provider 基于 EvidencePack 生成 deterministic summary
- 已补 guarded external HTTP LLM boundary：只由 EvidencePack 构造 prompt，
  provider failure 回退 extractive，unsafe / malformed output fail closed
- 已补可选 `python-worker` provider mode：Go 先生成 grounded summary，Python
  worker 只返回 candidate hash / citations；Go 校验 id、hash 和 citation
- response 保留 citations、EvidencePack、`generated_by_llm=false`
- provider 输出后统一运行 citation verifier，无法匹配 EvidencePack 则
  fail closed
- retrieval-gateway 公开 proto RPC client、app / gRPC / cmd focused tests
- `loadtest/summary`、`tools/run-summary-adapter-smoke.ps1` 和真实本地
  `retrieval-gateway -> summary-service` adapter smoke 已通过
- `GenerateConversationSummaryRequest.at_conversation_seq` 已透传到 EvidencePack current-memory query。
- ai-eval 已补 Summary current-memory consumption CI-safe regression：`at_conversation_seq`
  必须传播，过期和 superseded memory 不得作为 current citation。
- `loadtest/summary` 已加入 expired / superseded 低敏 decoy memory seed，并在
  summary JSON 输出 stale memory 排除结果，供 service-stack live adapter 断言。

下一步：

- 后续 provider 仍走 SummaryProvider port、prompt guard、hash / citation 校验和 verifier。
- 运行 current-memory service-stack live smoke，验证真实服务栈不引用 stale memory。
