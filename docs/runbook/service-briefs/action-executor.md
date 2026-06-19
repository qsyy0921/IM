# action-executor Brief

## 当前状态

- 阶段：`foundation-active`
- 当前能力：first approved execution audit path + Agent approval preflight + low-sensitive tool result projection + local safe tool adapter
- 运行模式：
  - `noop`
  - `grpc`

## 已完成

- `ExecuteApprovedAction` proto / gRPC adapter。
- 强制 `proposal_id`、`approval_id`、`prepared_audit_id`。
- 通过 `agent-service.VerifyApprovedAgentProposal` 校验 proposal 已批准，并
  核对 approval / prepared audit / skill / tool / resource，失败时不写
  execution audit。
- 通过 `skill-registry.GetSkill` 校验 skill active、tool name 匹配和 `EXECUTE` allowed action。
- 通过 `policy-service.CheckToolAction(action=EXECUTE)` 做执行前 policy precheck。
- PostgreSQL `action_executor_execution_audits` 低敏审计表。
- PostgreSQL `action_executor_tool_results` 低敏结果投影表，与 execution audit 同事务写入。
- 本地安全 `nexusim.local.echo` adapter first path：只支持 `LOW` risk、
  deterministic 低敏输出，不回显 raw input，不连接外部 MCP/provider。
- 本地 Docker / Prometheus / Grafana wiring。
- 聚焦单测和 PG integration test。
- Agent execution eval adapter first path：通过 `loadtest/agent` 建立 proposal、
  approve 后调用 `ExecuteApprovedAction`，并校验 execution audit 低敏字段。

## 边界

- 不执行外部 MCP / provider tool。
- 不执行业务写动作；当前可真实执行的只有 `nexusim.local.echo` 本地安全工具。
- 不连接 MCP server。
- 不保存 raw `input_json` 或 provider secret。
- 不读其它服务私有表。
- 业务 tool 默认保持 `executed=false`；本地安全 echo tool 可返回 `executed=true`
  和 `SUCCEEDED`，但只证明 output hash / result projection 边界。

## 下一步

- 接外部 MCP / tool adapter。
- 增加 rate limit、failure fallback、业务 tool output projection 和 DLQ / repair。
- 扩展 AI eval 的外部 MCP failure fallback / business tool safety cases。
