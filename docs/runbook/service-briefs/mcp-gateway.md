# mcp-gateway

状态：foundation-active / first prepare path.

定位：AI / Agent 外部工具调用的受控入口。第一版只做 tool call prepare / precheck：读取 `skill-registry` 公开技能合约，调用 `policy-service.CheckToolAction`，并把低敏审计写入 `mcp_gateway_tool_call_audits`。

当前已落：

- `mcp_gateway_service.proto`
- `migrations/postgres/mcp-gateway/000001_mcp_gateway_core.sql`
- 六层 skeleton：api / app / domain / infrastructure / types / cmd
- `PrepareToolCall`
- skill-registry gRPC client、policy-service gRPC client
- PostgreSQL audit repository、真实 PG 集成测试
- `grpc` runtime mode、debug `/metrics`
- Docker runtime、local compose、Prometheus rules、Grafana dashboard、`service-registry.json` wiring

边界：

- 不执行外部 MCP tool。
- 不保存 provider secret、MCP credential 或完整 tool input。
- 只保存 `input_sha256` 和低敏 decision / audit metadata。
- 不替代 `policy-service`，真实写动作仍要走 proposal / approval / executor / audit。
- 不直接读 message / conversation / delivery / policy 私有表。

下一步：

- 让 `agent-service` proposal 前调用 mcp-gateway prepare，而不是只直接调用 policy-service。
- 后续实现真实 MCP adapter 时必须继续走 skill catalog、policy、rate limit、audit 和 failure fallback。
- 与 `action-executor` 的 first audit path 已具备基础串接点；后续补正式 approval store 校验和真实 tool result handoff。
