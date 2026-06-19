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

下一步：

- provider-specific LLM / Python worker 后续仍必须走 SummaryProvider port、
  prompt guard、hash / citation metadata 校验和 citation verifier。
- `agent-service` 服务级 planner Python candidate guard 和 action-executor guarded external HTTP adapter first path 已落；下一步默认推进 external adapter eval / failure smoke cases。summary / Agent 仍只能消费 EvidencePack。
