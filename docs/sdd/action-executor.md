# action-executor SDD

## 目标

`action-executor` 是 NexusIM AI 应用底座中的受控动作执行边界。它承接 Agent proposal、approval 和 `mcp-gateway` prepare 之后的动作执行请求，把真实写动作纳入 policy precheck、审批关联和低敏 audit。

第一版只记录 approved execution boundary，不连接外部 MCP server，不执行真实业务写动作。

## 职责

- 提供 `ExecuteApprovedAction`。
- 强制请求携带：
  - `proposal_id`
  - `approval_id`
  - `prepared_audit_id`
- 读取 `skill-registry.GetSkill`：
  - skill 必须 `ACTIVE`
  - request `tool_name` 必须匹配 skill contract
  - skill 必须允许 `EXECUTE`
- 调用 `policy-service.CheckToolAction`，且 action 固定为 `EXECUTE`。
- 写入低敏 execution audit：
  - tenant / user / proposal / approval / prepare audit
  - skill / tool / action / resource
  - policy decision metadata
  - `input_sha256`
  - 不保存完整 `input_json`

## 非职责

- 不执行外部 MCP tool。
- 不连接外部 tool provider。
- 不保存 provider credential / access token / raw secret。
- 不替代 approval-service / 审批系统。
- 不直接读 message / conversation / delivery / contacts 私有表。
- 不生成 Agent proposal。

## 链路

第一版：

```text
approved caller
-> action-executor.ExecuteApprovedAction
-> skill-registry.GetSkill
-> policy-service.CheckToolAction(action=EXECUTE)
-> action_executor_execution_audits
```

目标态：

```text
agent-service proposal
-> mcp-gateway prepare
-> approval
-> action-executor
-> external MCP / tool adapter
-> audit / result projection
```

## 安全边界

- `AuthContext` 必须包含 tenant / user / device。
- `proposal_id`、`approval_id`、`prepared_audit_id` 必填。
- 第一版只返回 `RECORDED` / `BLOCKED`，且 `executed=false`。
- `input_json` 只校验 JSON 和大小，audit 只保存 hash。
- policy deny 必须 fail closed 并落低敏 audit。
- 后续接入真实 executor adapter 前，不能宣称外部工具执行链路完成。

## Migration

`migrations/postgres/action-executor/000001_action_executor_core.sql` 新增 `action_executor_execution_audits`：

- 主键：`tenant_id + execution_id`
- 幂等索引：`tenant_id + user_id + idempotency_key`（非空）
- 索引：
  - `tenant_id + tool_name + created_at`
  - `tenant_id + approval_id + created_at`
- 状态：`RECORDED` / `BLOCKED` / `FAILED`

## 后续

- 接入正式 approval / proposal store 校验。
- 接入真实 MCP adapter / tool provider。
- 低风险动作 result projection。
- per tenant / per tool rate limit。
- provider failure fallback / retry / DLQ。
- 外部 audit sink。

