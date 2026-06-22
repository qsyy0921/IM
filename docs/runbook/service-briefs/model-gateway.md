# model-gateway

状态：product-active；SDD v0.1 与 stage-switch review 已通过，第一版
`InvokeTextGeneration` / `InvokeEmbedding` / `GetModelInvocation` gRPC 路径已落地。

设计入口：`docs/sdd/model-gateway.md`；stage-switch / smoke 见对应 runbook。

定位：统一模型 provider 入口，负责 OpenAI / Claude / 本地模型 / embedding /
rerank provider 的路由、限流、成本、fail-closed、prompt policy 和低敏审计。

当前第一版：

- proto / migration / 六层 skeleton / `grpc` runtime / Docker / Prometheus /
  Grafana 覆盖已落。
- `InvokeTextGeneration` 使用 allowlisted deterministic mock provider，证明
  provider route、timeout、token / cost metadata 和 stable error 映射的最小边界。
- `InvokeEmbedding` 使用 allowlisted deterministic mock embedding provider，返回向量给调用方，
  但 PostgreSQL / outbox 只保存 input hash、embedding hash、维度、token / cost /
  latency 等低敏 metadata，不保存 raw input 或 embedding vector array。
- `GetModelInvocation` 只返回低敏 metadata；raw prompt / output 不落库、不进 outbox / metrics。

边界：

- Go 业务服务不直接散落 provider SDK；Python Worker 也不能绕过权限和审计。
- model-gateway 只能处理模型调用，不拥有 IM 事实、EvidencePack 事实或 Agent approval。
- 第一版不宣称真实 provider、rerank、outbox relay、route-refresh、
  budget-reset 或 cleanup worker 完成。
- 成本和配额策略后续可接 control-plane-service。
