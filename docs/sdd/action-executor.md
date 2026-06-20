# action-executor SDD

## 目标

`action-executor` 是 NexusIM AI 应用底座中的受控动作执行边界。它承接 Agent proposal、approval 和 `mcp-gateway` prepare 之后的动作执行请求，把真实写动作纳入 policy precheck、审批关联和低敏 audit。

当前第一阶段记录 approved execution boundary，并在同一事务内写低敏 tool result projection。已支持本地安全 `nexusim.local.echo` adapter first path，用于验证真实低敏 output hash / result projection；已支持外部 MCP fallback 稳定失败分类、tool output safety first path，以及显式开启的外部 HTTP provider adapter guarded first path。默认不连接外部 MCP / provider；外部 HTTP adapter 只允许 allowlist 内的 `LOW` risk tool，只发送 tool metadata / `input_sha256`，不发送 raw `input_json`，provider output 仍必须经过安全门禁后才写 hash / projection。

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
  - `output_sha256`；未执行或 unsafe output 时为空
- 对 `nexusim.local.echo` 这类本地安全 adapter，可生成低敏 output JSON、
  `output_sha256` 和 `SUCCEEDED` result projection。
- 对显式开启的外部 HTTP provider adapter：
  - 只接受 allowlist tool，排除 `nexusim.local.echo`
  - 只执行 `LOW` risk
  - 只发送 `tool_name`、`action=EXECUTE`、resource metadata、risk、intent 是否存在、
    `input_sha256`
  - 不发送 raw `input_json`、provider secret、用户 PII 或业务私表内容
  - provider 返回 body 必须是安全 JSON object，才会生成 `output_sha256`
- 对显式启用的外部 MCP fallback，只返回 timeout / unavailable / rate-limit /
  permission-denied / failed 等稳定分类，不保存 provider 原始错误。
- 所有 adapter output 进入响应 / hash 前必须经过安全门禁：
  valid JSON object、大小限制、无 secret-like / PII-like key 或 value。
- 明显的 repair / redrive / DLQ / dead-letter 类 tool / resource 元数据会被
  first-stage repair guard 拦截为 `ACTION_REPAIR_REQUIRES_OPERATOR`，不能通过
  通用 action adapter 执行；后续真实 repair 仍走专门 operator / approval
  workflow。

## 非职责

- 不执行任意外部 MCP tool；第一阶段只允许显式配置的 HTTP provider adapter
  guarded path。
- 不自动执行高风险 / 真实业务写动作；`LOW` risk provider tool 也必须先经过
  proposal / approval / mcp-gateway prepare / skill registry / policy precheck。
- 不默认连接外部 tool provider。
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

外部 HTTP provider guarded first path：

```text
approved caller
-> action-executor.ExecuteApprovedAction(tool=allowlisted LOW-risk provider tool)
-> proposal / skill / policy checks
-> external HTTP adapter
   (metadata + input_sha256 only; no raw input_json)
-> tool output safety gate
-> action_executor_execution_audits(output_sha256 only)
-> action_executor_tool_results(status=SUCCEEDED or FAILED)
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
- 外部 HTTP adapter 必须通过
  `NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_ADAPTER_MODE=http` 显式开启；缺少
  endpoint 或 allowlist 时 fail closed。
- 外部 HTTP adapter 的 plain HTTP endpoint 只允许 loopback / private 地址；
  公网 endpoint 必须使用 HTTPS。
- 外部 HTTP adapter 只发送 provider 所需的低敏 metadata 和 `input_sha256`，
  不发送 raw `input_json`。
- 外部 MCP fallback 使用 `NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_FALLBACK_MODE`
  显式开启；默认 `disabled`，保持不执行外部工具。
- unsafe tool output 必须 fail closed 为 `TOOL_OUTPUT_UNSAFE`，不写
  `output_sha256`，响应只返回 `{}`。
- `input_json` 只校验 JSON 和大小，audit 只保存 hash。
- policy deny 必须 fail closed 并落低敏 audit。
- repair / redrive / DLQ / dead-letter 类动作即使 proposal 和 policy 允许，
  第一阶段也必须停在 `ACTION_REPAIR_REQUIRES_OPERATOR`，不调用通用 tool
  adapter、不写 output hash。
- tool result projection 不是 provider 输出存储；当前只记录
  `NOT_EXECUTED` / `BLOCKED` / `SUCCEEDED` 等低敏状态引用和 output hash。
- 当前外部 HTTP adapter 只证明 guarded first path，不代表任意 MCP server、
  高风险业务写动作、provider retry / DLQ 或生产级 tool gateway 完成。

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
- 当前 first path 对未配置 adapter 的业务 tool 只生成 `NOT_EXECUTED` 或
  `BLOCKED` 低敏投影；对 `nexusim.local.echo` 和显式 allowlist 的外部 HTTP
  provider tool 可生成 `SUCCEEDED` + `output_sha256`，不保存 raw provider output

## 后续

- 接入更完整的真实外部 MCP adapter / tool provider。
- per tenant / per tool rate limit。
- provider retry / DLQ 和正式 repair operator handoff。
- 外部 audit sink。
