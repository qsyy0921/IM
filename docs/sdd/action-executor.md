# action-executor SDD

## 目标

`action-executor` 是 NexusIM AI 应用底座中的受控动作执行边界。它承接 Agent proposal、approval 和 `mcp-gateway` prepare 之后的动作执行请求，把真实写动作纳入 policy precheck、审批关联和低敏 audit。

当前第一阶段记录 approved execution boundary，并在同一事务内写低敏 tool result projection。已支持本地安全 `nexusim.local.echo` adapter first path，用于验证真实低敏 output hash / result projection；不连接外部 MCP server，不执行真实业务写动作。

## 职责

- 提供 `ExecuteApprovedAction`。
- 强制请求携带：
  - `proposal_id`
  - `approval_id`
  - `prepared_audit_id`
- 调用 `agent-service.VerifyApprovedAgentProposal`：
  - proposal 必须已 `APPROVED`
  - `approval_id`、`prepared_audit_id`、skill、tool、resource 必须匹配
  - 不直接读取 `agent_proposals` 私表
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
- 写入低敏 tool result projection：
  - `result_id`
  - `execution_id`
  - `proposal_id` / `approval_id` / `prepared_audit_id`
  - `status`
  - `result_ref`
  - `output_sha256`，当前未执行外部工具时为空
- 对 `nexusim.local.echo` 这类本地安全 adapter，可生成低敏 output JSON、
  `output_sha256` 和 `SUCCEEDED` result projection。

## 非职责

- 不执行外部 MCP tool。
- 不执行业务写动作；当前唯一可执行 adapter 是本地 deterministic
  `nexusim.local.echo`。
- 不连接外部 tool provider。
- 不保存 provider credential / access token / raw secret。
- 不替代 approval-service / 审批系统。
- 不直接读 message / conversation / delivery / contacts 私有表。
- 不生成 Agent proposal。

## 链路

第一版默认业务 tool：

```text
approved caller
-> action-executor.ExecuteApprovedAction
-> agent-service.VerifyApprovedAgentProposal
-> skill-registry.GetSkill
-> policy-service.CheckToolAction(action=EXECUTE)
-> action_executor_execution_audits
```

本地安全 output 验证路径：

```text
approved caller
-> action-executor.ExecuteApprovedAction(tool=nexusim.local.echo)
-> proposal / skill / policy checks
-> local safe echo adapter
-> action_executor_execution_audits(output_sha256 only)
-> action_executor_tool_results(status=SUCCEEDED)
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
- proposal / approval 校验失败时不写 execution audit。
- 业务 tool 第一版只返回 `RECORDED` / `BLOCKED`，且 `executed=false`。
- `nexusim.local.echo` 可返回 `executed=true`，但 output 只包含低敏 metadata
  和 `input_sha256`，不回显 raw input。
- `input_json` 只校验 JSON 和大小，audit 只保存 hash。
- policy deny 必须 fail closed 并落低敏 audit。
- tool result projection 不是 provider 输出存储；当前只记录
  `NOT_EXECUTED` / `BLOCKED` / `SUCCEEDED` 等低敏状态引用和 output hash。
- 后续接入真实 executor adapter 前，不能宣称外部工具执行链路完成。

## Migration

`migrations/postgres/action-executor/000001_action_executor_core.sql` 新增 `action_executor_execution_audits`：

- 主键：`tenant_id + execution_id`
- 幂等索引：`tenant_id + user_id + idempotency_key`（非空）
- 索引：
  - `tenant_id + tool_name + created_at`
  - `tenant_id + approval_id + created_at`
- 状态：`RECORDED` / `BLOCKED` / `FAILED`

`migrations/postgres/action-executor/000002_action_executor_tool_results.sql` 新增 `action_executor_tool_results`：

- 主键：`tenant_id + result_id`
- 唯一索引：`tenant_id + execution_id`
- 外键：`tenant_id + execution_id -> action_executor_execution_audits`
- 状态：`NOT_EXECUTED` / `BLOCKED` / `SUCCEEDED` / `FAILED`
- 当前 first path 对业务 tool 只生成 `NOT_EXECUTED` 或 `BLOCKED` 低敏投影；
  对 `nexusim.local.echo` 生成 `SUCCEEDED` + `output_sha256`，不保存 raw
  provider output

## 后续

- 接入外部 MCP adapter / tool provider。
- per tenant / per tool rate limit。
- provider failure fallback / retry / DLQ。
- 外部 audit sink。
