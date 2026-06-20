# model-gateway

状态：future / SDD v0.1 draft 已存在；stage-switch approved。当前不得单独
创建 `services/model-gateway` 目录；下一实现切片必须同步切换
`service-registry.json`、proto、migration、runtime、Docker / observability。

设计入口：`docs/sdd/model-gateway.md`。
Stage-switch 记录：`docs/runbook/stage-switch/model-gateway.md`。

定位：统一模型 provider 入口，负责 OpenAI / Claude / 本地模型 / embedding /
rerank provider 的路由、限流、成本、fallback、prompt policy 和低敏审计。

边界：

- Go 业务服务不直接散落 provider SDK；Python Worker 也不能绕过权限和审计。
- model-gateway 只能处理模型调用，不拥有 IM 事实、EvidencePack 事实或 Agent approval。
- 不持久化 raw prompt / model output，除非有明确低敏 eval / audit contract。
- 成本和配额策略可接 control-plane-service。

第一切片建议：

- 先按 SDD 落 proto / migration / 六层 skeleton，并优先实现
  `InvokeTextGeneration` / `GetModelInvocation`。
- 抽 RAG / Summary guarded external HTTP provider 的公共 contract。
- 增加 provider allowlist、timeout、budget key 和 failure classification。
- 确认 raw prompt / model output 不落库，调用结果只返回给同步 caller。
