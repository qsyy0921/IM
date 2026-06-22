# mcp-gateway SDD

## 目标

`mcp-gateway` 是 NexusIM AI 应用底座中的外部工具调用边界服务。它把 Agent / MCP tool 的调用意图纳入租户、技能合约、policy precheck 和 audit 边界。

第一版只做 prepare / precheck，不执行外部 MCP tool。

## 职责

- 提供 `PrepareToolCall`。
- 读取 `skill-registry.GetSkill`：
  - skill 必须 `ACTIVE`
  - request `tool_name` 必须与 skill contract 匹配
  - request action 必须在 `allowed_actions` 内
- 调用 `policy-service.CheckToolAction`。
- 写入低敏审计：
  - tenant / user / skill / tool / action / resource
  - risk / classification / reason / decision source
  - `input_sha256`
  - 不保存完整 `input_json`

## 非职责

- 不执行 MCP tool。
- 不连接外部 MCP server。
- 不保存 provider credential / access token / raw secret。
- 不审批高风险动作。
- 不替代 `action-executor`。
- 不直接读业务服务私有表。

## 链路

第一版：

```text
agent-service / future caller
-> mcp-gateway.PrepareToolCall
-> skill-registry.GetSkill
-> policy-service.CheckToolAction
-> mcp_gateway_tool_call_audits
```

后续：

```text
agent-service proposal
-> mcp-gateway prepare
-> approval
-> action-executor
-> external tool / MCP server
-> audit
```

## 安全边界

- `AuthContext` 必须包含 tenant / user / device。
- `input_json` 第一版只校验 JSON 和大小，不执行 schema validation。
- audit 表只存低敏 metadata 和 input hash。
- skill contract 的 `requires_approval=false` 不等于自动执行许可。
- policy deny 必须 fail closed。
- external MCP adapter 未接入前不能宣称真实工具执行链路完成。

## Migration

`migrations/postgres/mcp-gateway/000001_mcp_gateway_core.sql` 新增 `mcp_gateway_tool_call_audits`：

- 主键：`tenant_id + audit_id`
- 索引：
  - `tenant_id + tool_name + created_at`
  - `tenant_id + user_id + created_at`
- 状态：`ALLOWED` / `BLOCKED` / `FAILED`

## 后续

- Agent proposal 前调用 mcp-gateway prepare。
- 输入 schema validation。
- per tenant / per tool rate limit。
- real MCP adapter + provider failure handling。
- 与 action-executor 串接 approved action execution。
- 统一外部 audit sink。
