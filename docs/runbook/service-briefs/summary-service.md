# summary-service

状态：foundation-active / first read-only summary path / adapter smoke passed.

定位：会话摘要边界服务。它只消费 retrieval-gateway 返回的 `EvidencePack`，
不直接读 message / conversation / search / memory 私有表，不执行 Agent 动
作，不绕过 policy / visibility。

当前已落：

- `summary_service.proto`、SDD、六层 skeleton、`grpc` runtime mode、debug
  `/metrics`、registry / Docker / compose / Prometheus / Grafana wiring
- app usecase 调用 retrieval port 和 `SummaryProvider` port；默认本地
  extractive provider 基于 EvidencePack 生成 deterministic summary
- response 保留 citations、EvidencePack、`generated_by_llm=false`
- provider 输出后统一运行 citation verifier，无法匹配 EvidencePack 则
  fail closed
- retrieval-gateway 公开 proto RPC client、app / gRPC / cmd focused tests
- `loadtest/summary`、`tools/run-summary-adapter-smoke.ps1` 和真实本地
  `retrieval-gateway -> summary-service` adapter smoke 已通过：
  `docs/runbook/loadtest/summary-service/loadtest-report-20260619-summary-adapter-smoke.md`

下一步：

- 外部 LLM adapter 后续仍必须走 SummaryProvider port 和 citation verifier。
- 进入 `agent-service`；summary / Agent 仍只能消费 EvidencePack。
