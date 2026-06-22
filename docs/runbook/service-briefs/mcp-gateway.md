# mcp-gateway

状态：foundation-active / first prepare path.

定位：AI / Agent 工具调用受控入口。第一版只做 prepare / precheck：读
`skill-registry` 合约，调用 `policy-service.CheckToolAction`，写
`mcp_gateway_tool_call_audits` 低敏审计。

## 已落

- `mcp_gateway_service.proto`
- migration / 六层 skeleton / `PrepareToolCall`
- skill-registry RPC client、policy-service RPC client
- PostgreSQL audit repository、真实 PG 集成测试
- `grpc` mode、debug `/metrics`、Docker / compose / Prometheus / Grafana wiring
- agent-service proposal path 已接入 PrepareToolCall，返回 `prepared_audit_id`
- 真实本地 `retrieval-gateway -> agent-service -> mcp-gateway` adapter smoke

## 边界

- 不执行外部 MCP tool。
- 不保存 provider secret、credential、完整 tool input。
- 只保存 `input_sha256` 和低敏 decision / audit metadata。
- 不替代 policy-service，不直接读业务私表。

## 下一步

- 真实 MCP adapter 必须继续走 skill catalog、policy、rate limit、audit 和 failure recovery。
- 与 action-executor 继续串接 approval / result handoff。
