# action-executor Brief

状态：foundation-active / approved execution audit + local safe adapter + guarded external HTTP adapter.

## 已落

- `ExecuteApprovedAction` gRPC。
- 强制 `proposal_id`、`approval_id`、`prepared_audit_id`。
- 调 `agent-service.VerifyApprovedAgentProposal` 校验 approval / prepare audit / skill / tool / resource。
- 调 `skill-registry.GetSkill` 校验 active skill、tool match、`EXECUTE` action。
- 调 `policy-service.CheckToolAction(action=EXECUTE)` 做执行前 precheck。
- PostgreSQL `action_executor_execution_audits` 和 `action_executor_tool_results`。
- 本地安全 `nexusim.local.echo`：仅 `LOW` risk，deterministic 低敏输出，不回显 raw input。
- 外部 MCP fallback：默认关闭；显式开启后返回稳定低敏失败分类，不落 provider 原文。
- 外部 HTTP provider adapter：默认关闭；显式 `http` mode + allowlist + `LOW` risk 才执行，
  只发送 tool metadata / input hash，provider output 继续走 safety gate 和 output hash projection。
- Tool output safety：malformed / oversize / secret-like / PII-like output fail closed，不入 hash。
- Docker / Prometheus / Grafana wiring、聚焦测试、PG integration、Agent execution eval adapter。

## 边界

- 不执行任意外部 MCP / provider tool；当前只允许显式开启的 LOW-risk HTTP adapter first path。
- 不自动执行高风险 / 真实业务写动作；执行前仍必须经过 proposal / approval / prepare / policy。
- 不保存 raw `input_json`、provider secret 或 provider output。
- 未配置 adapter 的业务 tool 默认 `executed=false`；echo 和 allowlisted HTTP provider tool 可 `SUCCEEDED`，只证明 output hash / result projection。

## 下一步

- external adapter eval / failure smoke cases。
- rate limit、DLQ / repair。
