# skill-registry

状态：foundation-active / first catalog path.

定位：AI / Agent 可调用技能的合约目录服务。它登记技能、工具名、允许动作、输入输出 JSON schema、风险等级、审批要求、owner service、tags 和审计事件类型，供后续 `agent-service`、`mcp-gateway`、`action-executor` 做受控发现和校验。

当前已落：

- `skill_registry_service.proto`
- `migrations/postgres/skill-registry/000001_skill_registry_core.sql`
- 六层 skeleton：api / app / domain / infrastructure / types / cmd
- `UpsertSkill`、`GetSkill`、`ListSkills`
- PostgreSQL repository、真实 PG 集成测试
- `grpc` runtime mode、debug `/metrics`
- Docker runtime、local compose、Prometheus rules、Grafana dashboard、`service-registry.json` wiring

边界：

- 不执行工具动作。
- 不调用外部 MCP server。
- 不审批 proposal。
- 不写 message / conversation / delivery / policy 私有表。
- 不替代 policy-service；`requires_approval` / `risk_level` / `allowed_actions` 是 discovery contract，真实动作仍必须走 policy precheck 和后续 approval / executor / audit。

下一步：

- 让 `agent-service` 在 proposal 前读取 skill-registry，校验 tool name / action / schema 元数据。
- `mcp-gateway` 和 `action-executor` first path 已读取 skill contract；后续补 schema validation、version compatibility 和真实 tool adapter metadata。
