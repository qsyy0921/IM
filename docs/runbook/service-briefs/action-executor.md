# action-executor Brief

状态：foundation-active / approved execution audit + local safe adapter.

## 已落

- `ExecuteApprovedAction` gRPC。
- 强制 `proposal_id`、`approval_id`、`prepared_audit_id`。
- 调 `agent-service.VerifyApprovedAgentProposal` 校验 approval / prepare audit / skill / tool / resource。
- 调 `skill-registry.GetSkill` 校验 active skill、tool match、`EXECUTE` action。
- 调 `policy-service.CheckToolAction(action=EXECUTE)` 做执行前 precheck。
- PostgreSQL `action_executor_execution_audits` 和 `action_executor_tool_results`。
- 本地安全 `nexusim.local.echo`：仅 `LOW` risk，deterministic 低敏输出，不回显 raw input。
- 外部 MCP fallback：默认关闭；显式开启后返回稳定低敏失败分类，不落 provider 原文。
- Tool output safety：malformed / oversize / secret-like / PII-like output fail closed，不入 hash。
- Docker / Prometheus / Grafana wiring、聚焦测试、PG integration、Agent execution eval adapter。

## 边界

- 不执行外部 MCP / provider tool；fallback 只证明失败审计语义。
- 不执行业务写动作；当前唯一可执行 adapter 是 `nexusim.local.echo`。
- 不保存 raw `input_json`、provider secret 或 provider output。
- 业务 tool 默认 `executed=false`；echo tool 可 `SUCCEEDED`，只证明 output hash / result projection。

## 下一步

- 真实外部 MCP / tool adapter、rate limit、DLQ / repair。
- business tool safety eval adapter cases。
