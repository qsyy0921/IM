# action-executor SDD

## 目标

`action-executor` 是 NexusIM AI 应用底座中的受控动作执行边界。它承接 Agent proposal、approval 和 `mcp-gateway` prepare 之后的动作执行请求，把真实写动作纳入 policy precheck、审批关联和低敏 audit。

当前第一阶段记录 approved execution boundary，并在同一事务内写低敏 tool result projection。已支持本地安全 `nexusim.local.echo` adapter first path，用于验证真实低敏 output hash / result projection；已支持外部 MCP failure 稳定失败分类、tool output safety first path，以及显式开启的外部 HTTP provider adapter guarded first path。`conversation.note.create` 已有显式 opt-in 业务 adapter：配置 `NEXUSIM_ACTION_EXECUTOR_CONVERSATION_GRPC_ADDR` 后，通过 conversation-service 公开 `CreateConversationNote` 写入真实 note fact；未配置时仍保持 unsupported / `executed=false`，不伪造业务成功。默认不连接外部 MCP / provider；外部 HTTP adapter 只允许 allowlist 内的 `LOW` risk tool，只发送 tool metadata / `input_sha256`，不发送 raw `input_json`，provider output 仍必须经过安全门禁后才写 hash / projection。工具 provider 失败和 unsafe output 已有 first-stage `provider_failures` 状态投影和 bounded retry bookkeeping worker，可区分 `RETRY_PENDING` 与 `DLQ`；同时提供 provider failure metrics、provider failure audit、batch redrive-plan operator handoff、provider replay operator UI first path 和 provider replay admin / workflow handoff。redrive plan / operator UI / handoff 只生成低敏审批 artifact，不重放 provider、不执行 tool、不修改失败状态。`RedriveProviderFailure` 已提供受控 redrive API first path：只能针对已有 `DLQ` provider failure，要求 fresh proposal / approval / prepared audit、匹配的 skill / tool / resource、新 `input_json` 和 `reason_sha256`，并复用正常 `ExecuteApprovedAction` 链路执行，同时把 source provider failure id 与 reason hash 写入 execution audit。`loadtest/actionexecutor -mode provider-replay-redrive` 已提供第一版受控 operator path：默认 preflight，校验 invocation manifest 和仓库外 raw resource id / new input / reason hash；显式 `-execute` 时才调用 action-executor 公开 gRPC。`write-provider-replay-redrive-result-manifest.ps1` 会在显式执行后把 invocation 和 execution summary 绑定为低敏 result manifest；它不执行 redrive、不追加 audit、不修改 DLQ row。`write-provider-replay-redrive-audit-append-manifest.ps1` 会从低敏 result manifest 派生 external audit append manifest，绑定 result manifest hash、redrive execution / result refs 和低敏 `attributes_json` hash；它不调用 audit-service、不执行 redrive、不修改 DLQ row。`loadtest/actionexecutor -mode external-audit-append` 已提供第一版受控外部审计追加 operator：默认 preflight，校验仓库外低敏 audit append manifest、`attributes_json` hash、required checks 和 raw provider artifact 禁入；显式 `-execute` 时才调用 audit-service 公开 `AppendAuditRecord`。`write-provider-replay-redrive-audit-append-result-manifest.ps1` 会在显式 append 后把 audit append manifest 和 execution summary 绑定为低敏 result manifest；它不调用 audit-service、不执行 redrive、不修改 DLQ row。自动 provider replay 尚未实现。

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
- 对显式配置的 `conversation.note.create` 业务 adapter：
  - 只接受 `LOW` risk、`resource_type=conversation`、`EXECUTE` skill contract
  - raw `input_json` 只进入一方业务 adapter，用于解析 `body`；外部 provider 仍只拿
    `input_sha256`
  - 只调用 conversation-service 公开 gRPC `CreateConversationNote`，不读写
    conversation-service 私表
  - tool output 只返回 note id / ref / input hash / idempotent flag，不回显 note body
- 对显式启用的外部 MCP failure，只返回 timeout / unavailable / rate-limit /
  permission-denied / failed 等稳定分类，不保存 provider 原始错误。
- 对工具 provider 失败 / unsafe output，同一事务写入低敏
  `action_executor_provider_failures`：
  - retryable：timeout / provider unavailable / provider rate limited ->
    `RETRY_PENDING`
  - non-retryable：permission denied / unsafe output / generic execution failed ->
    `DLQ`
  - 只保存 classification、状态、retry metadata 和 `failure_ref`，不保存
    provider raw error、tool output 或 raw input。
- `provider-failure-worker` 只处理到期 `RETRY_PENDING` 行：
  - 未超过 max attempts 时增加 `retry_count` 并推进 `next_retry_at`
  - 达到 max attempts 时转 `DLQ`
  - 不调用 tool executor，不重放 provider，不需要 raw input
- `provider-failure-audit` / `provider-failure-redrive-plan`：
  - 只读取 action-executor 自有 `action_executor_provider_failures` 投影
  - 输出低敏 JSON artifact；user / resource / failure_ref 只输出 hash
  - redrive-plan 输出 batch id、candidate count 和 fresh proposal / approval /
    prepared-audit requirements
  - redrive plan 必须显式 dry-run 和 reason file，挂入 repair operator approval chain
  - 不修改 provider failure row，不调用 tool executor，不重放 provider output
- `provider-replay-operator-ui`：
  - 只读取 `DLQ` provider failure，输出低敏 replay candidate / batch / workflow state
  - candidate 行只暴露 user / resource / failure_ref hash，不暴露 raw input / output /
    provider raw error、secret 或 PII
  - 输出 permission gate、audit contract、eval gate、fresh proposal / approval /
    prepared audit / new input / reason hash requirements
  - 它是 operator review artifact，不执行 replay、不调用 tool、不修改 provider failure row
- `provider-replay-handoff`：
  - 只读取 `DLQ` provider failure，输出低敏 `PROVIDER_REPLAY_REQUEST` admin operation
    request 和 `REPAIR_APPROVAL` workflow handoff request
  - request 只携带 target / payload hash、candidate id、reason ref、evidence refs 和
    required gates，不携带 raw provider input / output / provider error / operator reason
  - admin-service / workflow-service 只做请求、审批和状态；最终执行仍只能走
    action-executor `RedriveProviderFailure`
  - 它不执行 replay、不调用 tool、不修改 provider failure row
- `write-provider-replay-handoff-review-page.ps1`：
  - 只读取仓库外 handoff artifact，重新校验 handoff contract、admin request payload hash、
    workflow request 和 `direct_execution_allowed=false` / `source_dlq_immutable=true`
  - 输出低敏 HTML 审查页，不提交 admin operation、不创建 workflow、不记录 approval、
    不调用 `RedriveProviderFailure`、不修改 DLQ row
  - 不嵌入 raw provider input / output、provider error、new input、operator reason、
    EvidencePack、本机路径或 credential-like 字段
- `write-provider-replay-readiness-page.ps1`：
  - 在 admin operation 已 `APPROVED`、workflow decision manifest 为 `APPROVE` 后使用
  - 重新绑定原 handoff candidate、approved admin operation、workflow manifest 和 fresh Agent proof
  - fresh proof 只允许 proposal / approval / prepared audit refs、skill / tool / resource hash、
    new input sha256 和 reason sha256；不得包含 raw new input、raw reason 或 raw provider artifact
  - 输出低敏 HTML readiness page，不调用 `RedriveProviderFailure`、不修改 DLQ row、
    不提交 admin operation、不记录 workflow decision
- `loadtest/actionexecutor -mode provider-replay-redrive`：
  - 读取仓库外低敏 redrive invocation manifest 和仓库外 raw resource id / new input /
    reason 文件
  - 校验 `resource_id_hash`、`new_input_sha256` 和 `reason_sha256` 后构造
    `RedriveProviderFailure` request
  - 默认只输出低敏 preflight summary；显式 `-execute` 时才调用 action-executor
    `RedriveProviderFailure`
  - 输出不打印 raw resource id、raw input、reason text、本机文件路径或 provider artifact
- `write-provider-replay-redrive-result-manifest.ps1`：
  - 读取仓库外低敏 redrive invocation manifest 和显式 redrive execution JSON summary
  - 重新校验 invocation、request、response 中的 provider failure、fresh proposal /
    approval / prepared audit、skill / tool / resource hash、new input hash、reason hash、
    result refs 和 status
  - 输出低敏 result manifest，用作 external audit append 前的证据绑定
  - 不调用 action-executor、不追加 audit、不修改 provider failure / DLQ row、不创建
    admin / workflow decision
  - 不输出 raw resource id、raw input、reason text、本机文件路径或 provider artifact
- `write-provider-replay-redrive-audit-append-manifest.ps1`：
  - 读取仓库外低敏 provider replay redrive result manifest
  - 重新校验 result manifest 的 execution boundary、fresh proposal / approval /
    prepared audit、skill / tool / resource hash、execution result refs 和 status
  - 输出现有 external audit append operator 可消费的低敏 audit append manifest
  - 不调用 audit-service、不调用 action-executor、不执行 redrive、不修改 provider failure /
    DLQ row、不创建 admin / workflow decision
  - 不输出 raw resource id、raw input、reason text、本机文件路径或 provider artifact
- `loadtest/actionexecutor -mode external-audit-append`：
  - 读取仓库外低敏 audit append manifest
  - 校验 `attributes_json` canonical hash、manifest required checks、operator identity
    和 `source_service=action-executor`
  - 拒绝 top-level raw input / raw output / provider body / provider artifact 字段，并
    只允许白名单低敏 audit attributes
  - 默认只输出低敏 preflight summary；显式 `-execute` 时才通过 audit-service 公开
    `AppendAuditRecord` 追加审计
- `write-provider-replay-redrive-audit-append-result-manifest.ps1`：
  - 读取仓库外低敏 audit append manifest 和显式 append execution JSON summary
  - 重新校验 request、response 中的 audit stream、source event、record type、
    attributes hash、idempotency key、audit id 和 record hash
  - 输出低敏 audit append result manifest，用作 operator 最终审计证据绑定
  - 不调用 audit-service、不调用 action-executor、不执行 redrive、不修改 provider failure /
    DLQ row、不创建 admin / workflow decision
  - 不输出 raw attributes JSON、本机文件路径、raw provider artifact 或 credential-like 内容
- `/metrics` / `/debug/metrics`：
  - 输出 provider failure status、retryable、due retry 和 classification 聚合计数
  - 不输出 tenant / user / resource / provider raw error / tool input / output
  - 查询失败时返回稳定 unavailable 语义，不泄漏数据库或 provider 错误
- `RedriveProviderFailure`：
  - 只接受 source status 为 `DLQ` 的 provider failure
  - source skill / tool / resource 必须与本次 redrive command 完全匹配
  - proposal / approval / prepared audit 必须是新的，不能复用 source failure 的旧引用
  - 调用方必须提交新的 `input_json` 和 64 位小写 hex `reason_sha256`
  - 执行路径复用 `ExecuteApprovedAction` 的 proposal / skill / policy / adapter / audit
    校验，不从旧失败行恢复 raw input，也不自动重放旧 provider output
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

未配置业务 tool 默认路径：

```text
approved caller
-> action-executor.ExecuteApprovedAction
-> agent-service.VerifyApprovedAgentProposal
-> skill-registry.GetSkill
-> policy-service.CheckToolAction(action=EXECUTE)
-> action_executor_execution_audits
```

Conversation note 业务 adapter 路径：

```text
approved caller
-> action-executor.ExecuteApprovedAction(tool=conversation.note.create)
-> proposal / skill / policy checks
-> conversation-service.CreateConversationNote
-> action_executor_execution_audits(output_sha256 only)
-> action_executor_tool_results(status=SUCCEEDED)
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
-> action_executor_provider_failures(RETRY_PENDING or DLQ when provider fails)
```

Provider failure redrive first path：

```text
operator / approved caller
-> action-executor.RedriveProviderFailure(provider_failure_id, reason_sha256, fresh proposal refs, new input_json)
-> load action_executor_provider_failures(status=DLQ)
-> source skill / tool / resource match check
-> ExecuteApprovedAction normal chain
-> action_executor_execution_audits(redrive_provider_failure_id, redrive_reason_sha256)
-> action_executor_tool_results / provider_failures according to execution outcome
```

Provider replay operator UI first path：

```text
operator
-> action-executor provider-replay-operator-ui
-> read action_executor_provider_failures(status=DLQ)
-> low-sensitive operator UI artifact(batch_id, candidate_id, workflow state, hashes, required gates)
-> fresh proposal / approval / prepared audit / new input / reason hash
-> action-executor.RedriveProviderFailure
```

External audit append operator first path：

```text
operator
-> loadtest/actionexecutor -mode external-audit-append
-> low-sensitive audit append manifest preflight
-> explicit -execute
-> audit-service.AppendAuditRecord
```

Provider replay admin / workflow handoff：

```text
operator
-> action-executor provider-replay-handoff
-> low-sensitive PROVIDER_REPLAY_REQUEST admin operation request
-> admin-service.CreateAdminOperation / ApproveAdminOperation
-> admin-service operation-worker -> workflow-service.CreateWorkflow(REPAIR_APPROVAL)
-> workflow decision approved
-> fresh Agent proposal / approval / prepared audit / new input / reason hash
-> action-executor.RedriveProviderFailure
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
- 未配置显式 adapter 的业务 tool 只返回 `RECORDED` / `BLOCKED`，且 `executed=false`。
- `conversation.note.create` 只有配置 conversation-service gRPC adapter 后才能写真实 note；
  未配置时不执行、不伪造成功。
- `nexusim.local.echo` 可返回 `executed=true`，但 output 只包含低敏 metadata
  和 `input_sha256`，不回显 raw input。
- 外部 HTTP adapter 必须通过
  `NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_ADAPTER_MODE=http` 显式开启；缺少
  endpoint 或 allowlist 时 fail closed。
- 外部 HTTP adapter 的 plain HTTP endpoint 只允许 loopback / private 地址；
  公网 endpoint 必须使用 HTTPS。
- 外部 HTTP adapter 只发送 provider 所需的低敏 metadata 和 `input_sha256`，
  不发送 raw `input_json`。
- 外部 MCP failure 使用 `NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_FAILURE_MODE`
  显式开启；默认 `disabled`，保持不执行外部工具。
- unsafe tool output 必须 fail closed 为 `TOOL_OUTPUT_UNSAFE`，不写
  `output_sha256`，响应只返回 `{}`。
- `input_json` 只校验 JSON 和大小，audit 只保存 hash。
- policy deny 必须 fail closed 并落低敏 audit。
- repair / redrive / DLQ / dead-letter 类动作即使 proposal 和 policy 允许，
  第一阶段也必须停在 `ACTION_REPAIR_REQUIRES_OPERATOR`，不调用通用 tool
  adapter、不写 output hash。
- provider failure redrive 只能先生成低敏 `provider-failure-redrive-plan` operator
  artifact，并通过通用 repair approval chain 审批；该 plan 不代表 replay 已执行。
- `provider-replay-operator-ui` / `provider-replay-handoff` 只能生成低敏人工审批视图或
  admin / workflow request artifact；它们不代表 replay 已执行，不复用旧 approval，
  不恢复旧 raw input / provider output，也不修改 DLQ failure row。
- `write-provider-replay-redrive-invocation.ps1` 只能生成仓库外低敏 invocation manifest：
  它绑定 approved admin operation、workflow approval manifest、fresh Agent proof 和 source
  handoff，并输出 `RedriveProviderFailure` command contract；它不调用 RPC、不修改 DLQ row、
  不保存 raw resource id / input / reason / provider artifact。
- `loadtest/actionexecutor -mode provider-replay-redrive` 是第一版受控 operator 执行入口：
  它默认只做 preflight，不调用 RPC；显式 `-execute` 时必须先校验低敏 invocation manifest、
  仓库外 raw resource id / new input / reason hash，并且只能调用 action-executor
  `RedriveProviderFailure` 公开 gRPC。它不读取其它服务私表，不绕过 admin / workflow /
  fresh Agent proof，不打印 raw input、reason 或 provider artifact。
- `RedriveProviderFailure` 是专用 redrive RPC，不属于通用 tool adapter。它要求
  DLQ source、fresh proposal / approval / prepared audit、匹配 skill / tool / resource、
  新 `input_json` 和 `reason_sha256`，并保留 source lineage；它不保存或恢复旧 raw
  input / provider output。
- tool result projection 不是 provider 输出存储；当前只记录
  `NOT_EXECUTED` / `BLOCKED` / `SUCCEEDED` 等低敏状态引用和 output hash。
- 当前外部 HTTP adapter 只证明 guarded first path；provider failure worker 只证明
  bounded retry bookkeeping，batch redrive plan 只证明低敏 operator handoff，不代表
  任意 MCP server、高风险业务写动作、自动 provider replay 或生产级 tool
  gateway 完成。

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
  `BLOCKED` 低敏投影；对 `conversation.note.create`、`nexusim.local.echo`
  和显式 allowlist 的外部 HTTP provider tool 可生成 `SUCCEEDED` +
  `output_sha256`，不保存 raw provider output / raw business input

`migrations/postgres/action-executor/000003_action_executor_provider_failures.sql` 新增 `action_executor_provider_failures`：

- 主键：`tenant_id + provider_failure_id`
- 唯一索引：`tenant_id + execution_id`
- 外键：
  - `tenant_id + execution_id -> action_executor_execution_audits`
  - `tenant_id + result_id -> action_executor_tool_results`
- 状态：`RETRY_PENDING` / `DLQ`
- retry state check：
  - `RETRY_PENDING` 必须 `retryable=true` 且有 `next_retry_at`
  - `DLQ` 必须 `retryable=false` 且有 `dead_lettered_at`
- 当前只做低敏状态投影、bounded retry bookkeeping、provider failure metrics、
  provider failure audit、redrive-plan operator handoff 和 provider replay operator UI；
  不保存 raw provider output / raw input。

`migrations/postgres/action-executor/000004_action_executor_redrive_metadata.sql` 给 `action_executor_execution_audits` 增加受控 redrive lineage：

- `redrive_provider_failure_id`：可空，引用同租户 `action_executor_provider_failures`
- `redrive_reason_sha256`：无 redrive 时为空；有 redrive 时必须是 64 位小写 hex
- 索引：`tenant_id + redrive_provider_failure_id + created_at`
- 只保存 source id 和 reason hash，不保存旧 raw input、provider raw error 或 provider output。

## 后续

- 接入更完整的真实外部 MCP adapter / tool provider。
- 正式 admin / workflow provider replay approval UI、外部 audit sink。
