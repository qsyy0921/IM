# action-executor Brief

## 当前状态

- 阶段：`foundation-active`
- 当前能力：first approved execution audit path
- 运行模式：
  - `noop`
  - `grpc`

## 已完成

- `ExecuteApprovedAction` proto / gRPC adapter。
- 强制 `proposal_id`、`approval_id`、`prepared_audit_id`。
- 通过 `skill-registry.GetSkill` 校验 skill active、tool name 匹配和 `EXECUTE` allowed action。
- 通过 `policy-service.CheckToolAction(action=EXECUTE)` 做执行前 policy precheck。
- PostgreSQL `action_executor_execution_audits` 低敏审计表。
- 本地 Docker / Prometheus / Grafana wiring。
- 聚焦单测和 PG integration test。

## 边界

- 不执行外部 tool。
- 不连接 MCP server。
- 不保存 raw `input_json` 或 provider secret。
- 不读其它服务私有表。
- 第一版返回 `executed=false`，只证明执行边界和 audit 已建立。

## 下一步

- 接 proposal / approval store 校验。
- 接真实 MCP / tool adapter。
- 增加 rate limit、failure fallback、result projection 和 DLQ / repair。
- 接 AI eval 的 Agent action safety adapter。
