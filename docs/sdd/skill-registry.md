# skill-registry SDD

## 目标

`skill-registry` 是 NexusIM AI 应用底座中的技能合约目录服务。它把 Agent 可调用的技能从 `agent-service` 代码中解耦出来，形成可审计、可查询、可治理的 catalog。

第一版只做 catalog，不执行工具、不审批、不调用 MCP。

## 职责

- 提供 `UpsertSkill`、`GetSkill`、`ListSkills`。
- 持久化技能定义：
  - `skill_id`
  - `display_name`
  - `version`
  - `tool_name`
  - `allowed_actions`
  - `input_schema_json`
  - `output_schema_json`
  - `permission_scope`
  - `risk_level`
  - `requires_approval`
  - `audit_event_type`
  - `owner_service`
  - `tags`
  - `metadata_json`
- 用 PostgreSQL 作为 catalog 事实源。
- 对外只暴露低敏技能合约，不暴露 provider secret、MCP credential 或运行时 token。

## 非职责

- 不执行工具动作。
- 不调用外部 MCP server。
- 不保存 tool runtime credential。
- 不审批或拒绝动作执行。
- 不替代 `policy-service.CheckToolAction`。
- 不直接读 message / conversation / delivery / policy 私有表。

## 链路

第一版：

```text
operator / future admin surface
-> skill-registry.UpsertSkill
-> skill_registry_definitions

agent-service / future mcp-gateway
-> skill-registry.Get/ListSkills
-> skill contract metadata
```

后续：

```text
agent-service
-> skill-registry.GetSkill
-> policy-service.CheckToolAction
-> proposal
-> approval
-> action-executor
-> audit
```

## 安全边界

- `AuthContext` 必须包含 tenant / user / device；后续可收紧到 admin / operator role。
- 第一版不把 skill registry 的 `requires_approval=false` 解释为自动执行许可。
- 高风险动作仍必须走 proposal / approval / executor / audit。
- JSON schema 是 contract 元数据，不执行动态代码。
- `metadata_json` 必须保持低敏，不存 secret、token、provider credential。

## Migration

`migrations/postgres/skill-registry/000001_skill_registry_core.sql` 新增 `skill_registry_definitions`：

- 主键：`tenant_id + skill_id`
- 唯一键：`tenant_id + display_name + version`
- JSONB 字段：
  - `allowed_actions_json`
  - `input_schema_json`
  - `output_schema_json`
  - `tags_json`
  - `metadata_json`
- 状态：`ACTIVE` / `DISABLED`

## 后续

- Agent proposal 前读取 skill contract，避免调用未登记 tool。
- MCP gateway 使用 skill contract 约束 external tool adapter。
- action-executor 根据 skill contract 做输入 schema validation、幂等和 audit。
- provider-grade 管理面后续接 admin/config service，不把手工 upsert 当长期方案。
