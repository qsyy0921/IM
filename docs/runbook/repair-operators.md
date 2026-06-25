# NexusIM Repair Operators

本文件只做现有本地 operator 的统一入口。它不替代服务 SDD、smoke 报告或审批系统，也不代表已经有生产级运维 UI。

机器可读索引见 `repair-operators.catalog.json`。后续跨服务执行编排、审批 UI 或外部审计 sink 应优先消费该 catalog，而不是解析本 Markdown。

本地计划生成入口：

```powershell
.\tools\write-repair-operator-plan.ps1 -Service delivery-service -Mode projection-checkpoint-repair -DryRun -DryRunEnv NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN
```

该脚本只读取 catalog 并输出低敏 JSON plan，不执行 operator、不连接数据库、不读取业务数据。后续审批 / 运维 UI 可以先复用这个计划格式，再接正式执行编排。

`write-repair-operator-plan.ps1 -Env KEY=VALUE` 只用于低敏过滤条件、operator 标识或低敏引用这类执行参数。计划文件会进入审批 / 审计链路，因此脚本会拒绝看起来像 password / secret / token / bearer / API key / session / cookie 的 ad-hoc env key 或 value，也会拒绝直接写入 `*_REASON`。operator reason 原文应放在文件中，并用 catalog 登记过的 reason-file env 引用：

```powershell
.\tools\write-repair-operator-plan.ps1 -Service delivery-service -Mode projection-checkpoint-repair -DryRun -DryRunEnv NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN -ReasonFileEnv NEXUSIM_DELIVERY_PROJECTION_REPAIR_REASON_FILE -ReasonFilePath H:\NexusIM\operator-plans\projection-reason.txt
```

需要使用真实凭据时应走部署环境或正式 secret 管理，不要写入 plan / request / decision / bundle 文件。

审批请求、审批决定、批量 manifest 和 audit bundle 中的 `requested_by` / `decided_by` / `generated_by` 也只允许低敏 operator id，例如 `operator-a` 或 `approver_1`。不要写邮箱、手机号、Bearer token、session id 或自由文本；详细原因放 `ReasonFile`，最终 JSON 只保存 reason hash。`ReasonFile` 是短文本 operator reason，不是日志包或证据附件；本地工具会拒绝超过 64 KiB 的 reason 文件。

新增 repair 交接 writer 时必须复用 `tools/repair-operator-safety.ps1`，并由 `tools/check-repair-operator-safety.ps1` 纳入 `check-local`，避免低敏 actor / ad-hoc env / hash 规则在多个脚本里漂移。

`tools/check-repair-operator-catalog-plannable.ps1` 会遍历 `repair-operators.catalog.json` 中的所有 service / mode，确认它们都能生成非执行 plan，避免新增 operator 后只更新 Markdown、漏更新机器可读 catalog 或 plan writer。

本地审批请求生成入口：

```powershell
.\tools\write-repair-approval-request.ps1 -PlanPath H:\NexusIM\operator-plans\plan.json -RequestedBy operator-a -ReasonFile H:\NexusIM\operator-plans\reason.txt
```

审批请求只保存 `plan_sha256`、`reason_sha256`、service / mode / command 和环境变量名，不保存环境变量值、operator reason 原文或业务数据。它是 first-stage 审批交接文件，不是正式审批系统。

本地审批决定生成入口：

```powershell
.\tools\write-repair-approval-decision.ps1 -RequestPath H:\NexusIM\operator-plans\approval.json -Decision APPROVED -DecidedBy approver-a -ReasonFile H:\NexusIM\operator-plans\decision-reason.txt
```

审批决定只保存 `request_sha256`、`plan_sha256`、`reason_sha256`、service / mode / command 和审批状态，不保存审批 reason 原文、环境变量值或业务数据。它不执行 operator，只为后续正式执行编排 / 运维 UI 提供可校验的低敏交接文件。

本地审批链校验入口：

```powershell
.\tools\validate-repair-approval-chain.ps1 -PlanPath H:\NexusIM\operator-plans\plan.json -RequestPath H:\NexusIM\operator-plans\approval.json -DecisionPath H:\NexusIM\operator-plans\decision.json
```

校验器确认 plan / request / decision 的 hash、`approval_id`、service / mode / command 和 `APPROVED` 状态一致，并回查 `repair-operators.catalog.json`，确保 service / mode / mode env / command 仍属于当前机器可读 catalog。它不执行 operator，也不会复制环境变量值、审批 reason 或业务数据。

本地静态审批 review page 生成入口：

```powershell
.\tools\write-repair-approval-review-page.ps1 -PlanPath H:\NexusIM\operator-plans\plan.json -RequestPath H:\NexusIM\operator-plans\approval.json -DecisionPath H:\NexusIM\operator-plans\decision.json -InvocationSummaryPath H:\NexusIM\operator-plans\invoke-summary.json -AuditBundlePath H:\NexusIM\operator-plans\audit-bundle.json -GeneratedBy operator-a -OutputPath H:\NexusIM\operator-plans\approval-review.html
```

该页面是 first-stage 本地 operator UI artifact。它会先复用 approval-chain validator，再渲染 service / mode / approval id / decision id、artifact hash、path hash、环境变量名、reason hash、preflight hash 和 audit bundle 摘要；不会复制环境变量值、operator reason 原文、manifest 路径、payload ref 文件正文、业务数据或 evidence 原文。它不执行 operator，也不是正式运维 UI / 外部审批系统。

本地审批后执行预检入口：

```powershell
.\tools\invoke-approved-repair-operator.ps1 -PlanPath H:\NexusIM\operator-plans\plan.json -RequestPath H:\NexusIM\operator-plans\approval.json -DecisionPath H:\NexusIM\operator-plans\decision.json -OutputPath H:\NexusIM\operator-plans\invoke-summary.json
```

默认只做审批链校验并输出低敏执行摘要，不执行 operator。显式加 `-Execute` 才会按 plan 设置环境变量并运行服务 operator；如果 plan 没有 `*_DRY_RUN=true`，还必须额外加 `-AllowMutating`，避免误执行 mutate / cleanup。

Provider replay 受控 redrive operator 入口：

```powershell
go run ./loadtest/actionexecutor -mode provider-replay-redrive `
  -manifest H:\NexusIM\operator-plans\provider-replay-redrive-invocation.json `
  -resource-id-file H:\NexusIM\operator-plans\provider-replay-resource-id.txt `
  -input-json-file H:\NexusIM\operator-plans\provider-replay-new-input.json `
  -reason-file H:\NexusIM\operator-plans\provider-replay-reason.txt `
  -operator-user-id operator-a `
  -operator-device-id operator-device-a
```

默认只做低敏 preflight，不调用 action-executor。显式加 `-execute` 才会调用
action-executor 公开 `RedriveProviderFailure` gRPC。执行前必须校验低敏 redrive
invocation manifest、仓库外 raw resource id、new input JSON 和 reason 文件 hash；
输出只保留 refs / hashes / result metadata，不打印 raw resource id、input JSON、reason
文本、本机文件路径或 provider artifact。该入口不属于 `repair-operators.catalog.json`
的服务 mode，因为它不是通过 `NEXUSIM_ACTION_EXECUTOR_MODE` 启动的 service operator，
而是最终 operator gRPC caller。

本地批量 repair manifest 生成入口：

```powershell
.\tools\write-repair-batch-manifest.ps1 -InvocationSummaryPath H:\NexusIM\operator-plans\invoke-1.json,H:\NexusIM\operator-plans\invoke-2.json -RequestedBy operator-a -ReasonFile H:\NexusIM\operator-plans\batch-reason.txt -OutputPath H:\NexusIM\operator-plans\batch-manifest.json
```

批量 manifest 只接受默认预检模式生成的 approved invocation summary，要求每个 item 都是 `execute_requested=false`、`executed=false`。它只保存 summary / plan / request / decision hash、service / mode / command、审批 id 和环境变量名，不保存环境变量值、operator reason 原文或业务数据，也不会执行 operator。它是 first-stage 批量交接文件，给后续正式批量执行编排、审批系统、外部 audit sink 或运维 UI 复用。

本地批量 manifest 校验入口：

```powershell
.\tools\validate-repair-batch-manifest.ps1 -ManifestPath H:\NexusIM\operator-plans\batch-manifest.json -OutputPath H:\NexusIM\operator-plans\batch-validation.json
```

校验器会重新读取 manifest 引用的 approved invocation summary，确认 summary hash、approval / decision id、service / mode / command 和 plan / request / decision hash 没有被篡改。它只输出低敏验证摘要，不执行 operator。

本地批量执行预检入口：

```powershell
.\tools\invoke-repair-batch-manifest.ps1 -ManifestPath H:\NexusIM\operator-plans\batch-manifest.json -OutputPath H:\NexusIM\operator-plans\batch-invoke-summary.json
```

默认只对批量 manifest 做校验，并逐条重新校验 approved invocation summary 对应的 plan / request / decision 链路，输出低敏批量预检摘要，不执行 operator。只有显式加 `-Execute` 才会按 item 顺序委托 `invoke-approved-repair-operator.ps1`；如果任一 item 不是 dry-run，还必须额外加 `-AllowMutating`。

本地 repair audit bundle 生成入口：

```powershell
.\tools\write-repair-audit-bundle.ps1 -EvidencePath H:\NexusIM\operator-plans\plan.json,H:\NexusIM\operator-plans\approval.json,H:\NexusIM\operator-plans\decision.json,H:\NexusIM\operator-plans\invoke-summary.json,H:\NexusIM\operator-plans\batch-manifest.json,H:\NexusIM\operator-plans\batch-validation.json,H:\NexusIM\operator-plans\batch-invoke-summary.json -GeneratedBy operator-a -ReasonFile H:\NexusIM\operator-plans\audit-reason.txt -OutputPath H:\NexusIM\operator-plans\audit-bundle.json
```

Audit bundle 只保存 evidence 文件路径、sha256、schema version 和低敏元数据，例如 service / mode / approval id / decision id / batch id / execute flags。它不复制 evidence 原文、不保存环境变量值、operator reason 原文或业务数据。它是 first-stage 本地审计交接 manifest，不是外部 audit sink。

本地 repair audit bundle 校验入口：

```powershell
.\tools\validate-repair-audit-bundle.ps1 -BundlePath H:\NexusIM\operator-plans\audit-bundle.json -OutputPath H:\NexusIM\operator-plans\audit-bundle-validation.json
```

校验器会重新读取 audit bundle 引用的 evidence 文件，验证 sha256 和 kind summary，没有复制 evidence 原文，也不会执行 operator。

## 使用原则

- 先 audit，后 repair；没有明确 event / outbox / checkpoint / failure 范围时不要 redrive。
- 优先使用服务自带 operator，不直接手写 SQL 修改业务表。
- 所有 redrive / cleanup 必须保留服务内 audit 记录。
- `last_error` / `previous_last_error` / provider error 只能保留稳定低敏分类，不能写入 broker body、token、账号、目标地址或原始 provider body。
- operator 只处理本服务拥有的表；不能跨服务读写私有表。

## 通用 outbox 模式

这些服务已有相同形态的 outbox 排障入口：

| 服务 | 环境变量 | 只读审计 | redrive | repair 审计 | cleanup |
| --- | --- | --- | --- | --- | --- |
| `message-service` | `NEXUSIM_MESSAGE_SERVICE_MODE` | `outbox-audit` | `outbox-repair` | `outbox-repair-audit` | `outbox-repair-cleanup` |
| `delivery-service` | `NEXUSIM_DELIVERY_SERVICE_MODE` | `outbox-audit` | `outbox-repair` | `outbox-repair-audit` | `outbox-repair-cleanup` |
| `receipt-service` | `NEXUSIM_RECEIPT_SERVICE_MODE` | `outbox-audit` | `outbox-repair` | `outbox-repair-audit` | `outbox-repair-cleanup` |
| `contacts-service` | `NEXUSIM_CONTACTS_SERVICE_MODE` | `outbox-audit` | `outbox-repair` | `outbox-repair-audit` | `outbox-repair-cleanup` |
| `policy-service` | `NEXUSIM_POLICY_SERVICE_MODE` | `outbox-audit` | `outbox-repair` | `outbox-repair-audit` | `outbox-repair-cleanup` |

示例：

```powershell
$env:NEXUSIM_DELIVERY_SERVICE_MODE = "outbox-audit"
go run ./services/delivery-service/cmd/delivery-service
```

```powershell
$env:NEXUSIM_DELIVERY_SERVICE_MODE = "outbox-repair"
go run ./services/delivery-service/cmd/delivery-service
```

具体过滤参数以对应服务 cmd / service brief / smoke 报告为准。

以下服务的 `outbox-repair` 支持写低敏 JSON summary，便于 redrive 当次执行留存证据；输出只包含输入 ID 数、执行模式和计数，不输出 event id 列表、业务 payload 或 operator reason：

| 服务 | JSON 输出环境变量 |
| --- | --- |
| `message-service` | `NEXUSIM_MESSAGE_OUTBOX_REPAIR_OUTPUT` |
| `delivery-service` | `NEXUSIM_DELIVERY_OUTBOX_REPAIR_OUTPUT` |
| `receipt-service` | `NEXUSIM_RECEIPT_OUTBOX_REPAIR_OUTPUT` |
| `contacts-service` | `NEXUSIM_CONTACTS_OUTBOX_REPAIR_OUTPUT` |
| `policy-service` | `NEXUSIM_POLICY_OUTBOX_REPAIR_OUTPUT` |

`delivery-service` 的 `outbox-repair` 额外支持 `NEXUSIM_DELIVERY_OUTBOX_REPAIR_DRY_RUN=true`，用于只写 repair audit、不 mutate outbox 状态；并支持 `NEXUSIM_DELIVERY_OUTBOX_REPAIR_REASON_FILE` 读取 repair reason 原文，避免把 reason 写进 operator plan / shell env。

`message-service` 的 `outbox-repair` 额外支持 `NEXUSIM_MESSAGE_OUTBOX_REPAIR_REASON_FILE` 读取 repair reason 原文，避免把 reason 写进 operator plan / shell env。

`receipt-service` 的 `outbox-repair` 额外支持 `NEXUSIM_RECEIPT_OUTBOX_REPAIR_REASON_FILE` 读取 repair reason 原文，避免把 reason 写进 operator plan / shell env。

`contacts-service` 的 `outbox-repair` 额外支持 `NEXUSIM_CONTACTS_OUTBOX_REPAIR_REASON_FILE` 读取 repair reason 原文，避免把 reason 写进 operator plan / shell env。

`policy-service` 的 `outbox-repair` 额外支持 `NEXUSIM_POLICY_OUTBOX_REPAIR_REASON_FILE` 读取 repair reason 原文，避免把 reason 写进 operator plan / shell env。

以下服务的 `outbox-repair-audit` 支持写低敏 JSON 结果，便于 operator 留存证据，不写 Kafka 原始错误正文或业务 payload；并统一支持按 `repaired_at` RFC3339 时间窗口过滤，在 JSON 中写 compacted filters：

| 服务 | JSON 输出环境变量 |
| --- | --- |
| `message-service` | `NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_OUTPUT` |
| `delivery-service` | `NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_OUTPUT` |
| `receipt-service` | `NEXUSIM_RECEIPT_OUTBOX_REPAIR_AUDIT_OUTPUT` |
| `contacts-service` | `NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_OUTPUT` |
| `policy-service` | `NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_OUTPUT` |

以下服务的只读 `outbox-audit` 也支持写低敏 JSON 当前 outbox 结果，便于排障前留存证据；统一支持按 `created_at` RFC3339 时间窗口过滤；输出只包含 outbox 元数据、状态、retry 计数、时间戳、稳定低敏 `last_error` 和 compacted filters，不写业务 payload：

| 服务 | JSON 输出环境变量 |
| --- | --- |
| `message-service` | `NEXUSIM_MESSAGE_OUTBOX_AUDIT_OUTPUT` |
| `delivery-service` | `NEXUSIM_DELIVERY_OUTBOX_AUDIT_OUTPUT` |
| `receipt-service` | `NEXUSIM_RECEIPT_OUTBOX_AUDIT_OUTPUT` |
| `contacts-service` | `NEXUSIM_CONTACTS_OUTBOX_AUDIT_OUTPUT` |
| `policy-service` | `NEXUSIM_POLICY_OUTBOX_AUDIT_OUTPUT` |

以下服务的 `outbox-repair-cleanup` 支持显式 dry-run 和写低敏 JSON summary，便于留存 cleanup 证据；输出只包含删除行数或 dry-run 命中行数、cutoff、retention、batch size、dry-run 标记和过滤条件，不重新输出被清理的历史错误明细。执行正式删除前建议先设置对应 dry-run 环境变量复核范围。

| 服务 | Dry-run 环境变量 | JSON 输出环境变量 |
| --- | --- | --- |
| `message-service` | `NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_DRY_RUN` | `NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_OUTPUT` |
| `delivery-service` | `NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_DRY_RUN` | `NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_OUTPUT` |
| `receipt-service` | `NEXUSIM_RECEIPT_OUTBOX_REPAIR_CLEANUP_DRY_RUN` | `NEXUSIM_RECEIPT_OUTBOX_REPAIR_CLEANUP_OUTPUT` |
| `contacts-service` | `NEXUSIM_CONTACTS_OUTBOX_REPAIR_CLEANUP_DRY_RUN` | `NEXUSIM_CONTACTS_OUTBOX_REPAIR_CLEANUP_OUTPUT` |
| `policy-service` | `NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_DRY_RUN` | `NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OUTPUT` |

`message-service` 还提供只读 `change-history-audit`，用于审计 `message_change_history` 中 `EDIT / REVOKE / DELETE` 变更证明；支持按 tenant / conversation / message / change type / changed_by 缩小范围，可选 `NEXUSIM_MESSAGE_CHANGE_HISTORY_AUDIT_OUTPUT` 写低敏 JSON 结果。输出只包含变更元数据、状态转换、payload 是否存在和 reason 是否存在，不输出消息 payload 或 reason 原文。

`message-service` 还提供只读 `retention-proof-audit`，用于按当前 `message_log` 行聚合删除证明：当前状态、payload 是否仍存在、最新 `DELETE` change history、`message.deleted.v1` timeline / outbox 是否存在。默认审计 `DELETED` 消息，支持按 tenant / conversation / message / status 缩小范围，可选 `NEXUSIM_MESSAGE_RETENTION_PROOF_AUDIT_OUTPUT` 写低敏 JSON 结果；输出不包含消息 payload 或删除 reason 原文。

`message-service` 还提供 `legal-hold-audit` / `legal-hold-set` / `legal-hold-release`，用于审计、设置和释放消息级 legal hold。ACTIVE legal hold 会让 `DeleteMessage` 在事务内 fail-closed，不推进 seq 或写 timeline/outbox。`legal-hold-audit` 可选 `NEXUSIM_MESSAGE_LEGAL_HOLD_AUDIT_OUTPUT` 写低敏 JSON，`legal-hold-set` / `legal-hold-release` 可选 `NEXUSIM_MESSAGE_LEGAL_HOLD_OUTPUT` 写低敏 JSON；`legal-hold-set` 支持 `NEXUSIM_MESSAGE_LEGAL_HOLD_REASON_FILE` 读取 reason 原文，避免把 reason 写进 operator plan / shell env；输出只包含 hold 元数据和 reason-present，不输出 hold reason 原文。

`message-service` 还提供 `compliance-proof-audit` / `compliance-proof-register` / `compliance-proof-revoke`，用于登记、吊销和审计外部合规 proof 的低敏引用。`compliance-proof-register` 只保存 `external_proof_ref`、provider 和 proof hash，不保存 proof 正文；审批创建和最终 `COMPLIANCE_RETENTION` 删除都会要求 proof ref 仍为 `VERIFIED`。`compliance-proof-audit` 支持按 tenant / proof ref / status / provider / `updated_at` RFC3339 时间窗口过滤，可选 `NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_OUTPUT` 写低敏 JSON，`compliance-proof-register` / `compliance-proof-revoke` 可选 `NEXUSIM_MESSAGE_COMPLIANCE_PROOF_OUTPUT` 写低敏 JSON。

`compliance-proof-register` 的 manifest provider 模式可先用 `tools/validate-message-compliance-proof-manifest.ps1` 校验外部 proof manifest。该 validator 比 runtime parser 更严格：只允许 `external_proof_ref`、`provider`、`proof_hash`、`status` 字段，拒绝 proof 正文、token、secret 或未知字段，并可校验指定 proof ref 必须是 `VERIFIED`。

`message-service` 还提供 `compliance-approval-audit` / `compliance-approval-create` / `compliance-approval-cancel`，用于审计、创建和取消合规删除审批。`COMPLIANCE_RETENTION` 删除必须提交匹配的 approval id 和 external proof ref，并在事务内把 `APPROVED` approval 消费为 `CONSUMED`。`compliance-approval-create` 只能引用已 `VERIFIED` 的 external proof ref，并支持 `NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_REASON_FILE` 读取审批 reason 原文，避免把 reason 写进 operator plan / shell env。`compliance-approval-audit` 支持按 tenant / conversation / message / approval / status / `updated_at` RFC3339 时间窗口过滤，可选 `NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_AUDIT_OUTPUT` 写低敏 JSON，`compliance-approval-create` / `compliance-approval-cancel` 可选 `NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_OUTPUT` 写低敏 JSON；输出只包含审批元数据、低敏 proof ref 和 reason-present，不输出审批 reason 或外部 proof 正文。

`policy-service` 还提供只读 `decision-audit-export`，用于把 `policy_decision_audit_outbox` 中的低敏决策审计行导出为 JSON，支持按 tenant / event / action / allowed / classification / reason_code / outbox status / created_at RFC3339 时间窗口过滤；可选 `NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_OUTPUT` 写结果。输出只包含 stable key、决策元数据、trace/request id 和 outbox 发布状态，不输出消息正文、provider body、payload_json 或用户原始标识。它是 first-stage 本地审计交接文件。

`policy-service` 还提供只读 `decision-audit-forward`，用于把同一批低敏决策审计行 POST 到外部 audit sink。`NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ENDPOINT` 默认必须是 HTTPS；`NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_DRY_RUN=true` 可只生成低敏 summary，不发网络请求；可选 `NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_OUTPUT` 写 forward summary。该模式不 mutate `policy_decision_audit_outbox`、不持久化 provider response body、不输出 bearer token / provider body / payload_json / 用户原始标识；它是 first-stage 外部审计交接，不等于带 checkpoint、retry、DLQ 或 exactly-once 语义的 provider-grade audit pipeline。

`policy-service` 还提供 `tenant-quota-audit` / `tenant-quota-set`，用于审计和设置 first-stage tenant action quota。quota 按 tenant / action / window_seconds 统计已允许的历史 policy decision，达到 `max_decisions` 后 fail-closed deny；`tenant-quota-audit` 可选 `NEXUSIM_POLICY_TENANT_QUOTA_AUDIT_OUTPUT` 写低敏 JSON，`tenant-quota-set` 可选 `NEXUSIM_POLICY_TENANT_QUOTA_SET_OUTPUT` 写低敏 JSON。`tenant-quota-set` 支持 `NEXUSIM_POLICY_TENANT_QUOTA_SET_REASON_FILE` 读取 operator reason 原文，避免把 reason 写进 operator plan / shell env；输出只包含配置元数据和 reason-present，不输出 operator reason 原文。该能力不是 provider-grade tenant DSL / billing quota。

`api-gateway` 提供 `tenant-quota-audit` / `tenant-quota-set`，用于审计和设置 api-gateway 自有 `api_gateway_rate_limit_tenant_plans` DB-backed rate-limit plan。环境变量为 `NEXUSIM_API_GATEWAY_MODE`；`tenant-quota-audit` 可选 `NEXUSIM_API_GATEWAY_TENANT_QUOTA_AUDIT_OUTPUT` 写低敏 JSON，`tenant-quota-set` 可选 `NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_OUTPUT` 写低敏 JSON，并支持 `NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_DRY_RUN=true` 只生成 planned row、不连接数据库、不写表。非 dry-run 可配置 `NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_REQUIRE_APPROVAL=true` 和 `NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_APPROVAL_PATH`，执行前会校验 `nexusim.api_gateway.tenant_quota_approval.v1` 低敏审批 manifest 与本次 tenant / RPS / burst / enabled / source 精确匹配且未过期；`tools/write-api-gateway-tenant-quota-approval.ps1` 可生成仓库外审批文件，`tools/validate-api-gateway-tenant-quota-approval.ps1` 可离线校验。DB DSN 优先级为 `NEXUSIM_API_GATEWAY_TENANT_QUOTA_DB_DSN`、`NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_DB_DSN`、`NEXUSIM_PG_DSN`。输出只包含 tenant id、RPS、burst、enabled、source、更新时间和低敏审批摘要，不输出 DSN、token、业务 payload 或配置源凭据。该能力是 first-stage 本地 quota 控制面 / 审批包入口，不是完整配置中心、灰度发布、计费或多环境发布系统。

`admin-service` 提供 `compensation-request`，用于把已失败的 admin operation 标记为
`COMPENSATION_REQUESTED`，并写低敏
`admin.operation.compensation_requested.v1` outbox event。环境变量为
`NEXUSIM_ADMIN_SERVICE_MODE`；必须提供
`NEXUSIM_ADMIN_COMPENSATION_TENANT_ID` 和
`NEXUSIM_ADMIN_COMPENSATION_OPERATION_ID`，可用
`NEXUSIM_ADMIN_COMPENSATION_REQUESTED_BY` 标记低敏 operator id。默认
`NEXUSIM_ADMIN_COMPENSATION_DRY_RUN=true`，只检查目标 operation 是否可以请求补偿；
显式设为 false 才会 mutate。正式 mutate 必须提供
`NEXUSIM_ADMIN_COMPENSATION_REASON_REF` 或
`NEXUSIM_ADMIN_COMPENSATION_REASON_FILE`；reason file 只计算 sha256 并生成
`reason-sha256:<hash>`，不把 reason 原文写入数据库、outbox 或 JSON summary。可选
`NEXUSIM_ADMIN_COMPENSATION_OUTPUT` 写低敏 JSON summary，输出只包含 tenant /
operation / dry-run / status / reason ref / reason hash / operator hash，不输出
operation payload、reason 原文、EvidencePack 或下游 response body。
设置 `NEXUSIM_WORKFLOW_GRPC_ADDR` 后，非 dry-run 会通过 workflow-service public gRPC
创建 / replay `COMPENSATION_REQUEST` workflow；该 workflow 只携带低敏 target /
payload / reason refs，不代表真实补偿 mutation 已执行。

`workflow-service` 提供本地 `loadtest/workflow` operator CLI，用于通过公开 gRPC
get workflow、record decision 和查询 compensation instruction metadata，不读
workflow-service PostgreSQL 私表：

```powershell
.\tools\write-workflow-decision-manifest.ps1 -OutputPath H:\NexusIM\operator-plans\workflow-decision.json -WorkflowID wf_123 -StepID wfs_1 -Decision APPROVE -DeciderRef operator-a -ReasonFile H:\NexusIM\operator-plans\workflow-decision-reason.txt -EvidenceRef evidence:ticket-123
.\tools\validate-workflow-decision-manifest.ps1 -ManifestPath H:\NexusIM\operator-plans\workflow-decision.json -ExpectedWorkflowID wf_123 -ExpectedStepID wfs_1 -ExpectedDecision APPROVE
.\tools\write-workflow-compensation-instruction-manifest.ps1 -OutputPath H:\NexusIM\operator-plans\workflow-compensation-instruction.json -WorkflowID wf_123 -PayloadRefFile H:\NexusIM\operator-plans\rollback-payload-ref.txt -Environment local -ConfigKind API_GATEWAY_TENANT_QUOTA -BundleKey tenant-a -TargetVersion quota-v1 -OperatorRef operator:rollback -ReasonFile H:\NexusIM\operator-plans\rollback-reason.txt
.\tools\validate-workflow-compensation-instruction-manifest.ps1 -ManifestPath H:\NexusIM\operator-plans\workflow-compensation-instruction.json -ExpectedWorkflowID wf_123 -ExpectedTargetVersion quota-v1
go run ./loadtest/workflow -mode get -workflow-id wf_123
go run ./loadtest/workflow -mode provider-replay-queue
go run ./loadtest/workflow -mode list-workflows -workflow-type REPAIR_APPROVAL -status WAITING_DECISION -target-service action-executor -target-operation PROVIDER_REPLAY_REQUEST -approval-policy-ref admin.workflow.provider_replay.v1
go run ./loadtest/workflow -mode record-decision -workflow-id wf_123 -step-id wfs_1 -decision APPROVE -decider-ref operator:a
go run ./loadtest/workflow -mode record-decision -decision-manifest H:\NexusIM\operator-plans\workflow-decision.json
go run ./loadtest/workflow -mode list-compensation-instructions -workflow-id wf_123 -status ACTIVE
go run ./loadtest/workflow -mode compensation-review-bundle -workflow-id wf_123 > H:\NexusIM\operator-plans\workflow-compensation-review-bundle.json
.\tools\write-workflow-compensation-review-page.ps1 -BundlePath H:\NexusIM\operator-plans\workflow-compensation-review-bundle.json -GeneratedBy operator-a -OutputPath H:\NexusIM\operator-plans\workflow-compensation-review.html
```

可用 `-target` 或 `NEXUSIM_WORKFLOW_GRPC_ADDR` 指向 workflow-service gRPC，
并支持 `NEXUSIM_WORKFLOW_TLS_*` / `-workflow-tls-*` 配置 TLS / mTLS。
`provider-replay-queue` 是 `list-workflows` 的显式 provider replay 队列视图，默认只列
`REPAIR_APPROVAL`、`WAITING_DECISION`、`target_service=action-executor`、
`target_operation=PROVIDER_REPLAY_REQUEST`、`approval_policy_ref=admin.workflow.provider_replay.v1`
的 workflow；它只辅助 operator 找到待审批工作流，不执行 redrive、不调用
action-executor、不修改 provider failure / DLQ row。输出只包含 workflow metadata、
decision refs、instruction id、payload ref hash、target service /
operation、version 和状态等低敏字段，不输出 workflow payload、instruction payload、
operator reason 原文或 downstream response body。它是 first-stage ops visibility，
不是正式审批 UI 或执行器。`record-decision` 还会在本地拒绝看起来像 secret /
token / password / raw body / DSN 的 decider、policy、reason 或 evidence ref；
真实原因和证据原文应留在独立 artifact / audit 系统中，只把低敏 ref 或 hash 传给
workflow-service。`-decision-manifest` 可作为第一版外部审批系统交接文件，schema
version 必须是 `nexusim.workflow.external_decision_manifest.v1`；manifest 除 workflow /
step / decision / decider / policy / reason ref / evidence refs / idempotency /
correlation refs 外，还必须包含 expected workflow type、status、target service /
operation、target ref hash、payload schema version、payload ref hash 和 approval policy
ref。CLI 在记录 decision 前会调用 workflow-service `GetWorkflow` 绑定校验；任何
mismatch 都 fail-closed，不会调用 `RecordWorkflowDecision`，也不会执行 provider
replay。manifest 不保存审批 comment、EvidencePack、payload 或 provider body。
`write-workflow-decision-manifest.ps1` 可从 reason 文件生成 `reason-sha256:<hash>`，
`validate-workflow-decision-manifest.ps1` 只做 schema / 低敏字段校验，不读取数据库。
`loadtest/workflow -mode operator-queues` 可列出 action approval、repair approval、
provider replay、admin operation、compensation request 和 compensation pending 的低敏
workflow queue summary；它只调用 `ListWorkflows`，不记录 decision、不修改 workflow、
不执行 provider replay。
`loadtest/workflow -mode external-callback-wait` 可创建一个等待外部 callback decision 的
低敏 workflow，并输出 external decision manifest template。该 template 只绑定 workflow /
step / target / payload / approval policy refs；外部系统必须补全 explicit decision /
decider / reason / evidence 后再走 `record-decision -decision-manifest`。创建 wait workflow
不会记录 decision、不会调用目标服务、不会执行 provider replay 或任何 action。
`write-workflow-compensation-instruction-manifest.ps1` /
`validate-workflow-compensation-instruction-manifest.ps1` 可生成和校验 workflow
compensation executor 使用的 control-plane rollback instruction JSON。该 manifest
只包含 workflow id、payload hash、config target、operator ref 和 reason ref；payload
/ reason 文件只用于计算 hash，不会被复制进 manifest，也不会保存本机路径。它是
first-stage operator handoff，不是正式 instruction approval UI，也不调用
workflow-service、control-plane-service 或数据库。
`write-workflow-compensation-review-page.ps1` 可把
`loadtest/workflow -mode compensation-review-bundle` 的低敏审查包渲染成仓库外 HTML。
页面只展示 workflow / instruction refs、hash、policy、status 和审查边界；它不会记录
decision、不会创建 approval、不会执行 compensation、不会调用 control-plane-service /
action-executor，也不会输出原始 payload、reason、provider body、EvidencePack 或本机路径。

`workflow-service` 的 `compensation-instruction-import` 已纳入
`repair-operators.catalog.json`，可被本地 repair approval request / decision /
invocation 链路引用。环境变量为 `NEXUSIM_WORKFLOW_SERVICE_MODE`；计划文件需要通过低敏 env 指定
`NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_TENANT_ID` 和
`NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_FILE`，后者应指向上面生成并校验过的
仓库外 instruction manifest；该 operator 导入 instruction metadata，不直接执行
rollback mutation。
`invoke-approved-repair-operator.ps1` 会在执行该 mode 前再次校验 instruction
manifest；preflight summary 只记录 manifest hash / path hash / instruction count，
不输出 manifest 路径、payload ref 文件正文或 operator reason 原文。

`agent-service` 提供 `proposal-approval-audit` / `proposal-approval-approve`，用于审计和审批 Agent proposal。环境变量为 `NEXUSIM_AGENT_SERVICE_MODE`；`proposal-approval-audit` 默认只列 `PROPOSED` proposal，可按 tenant / proposal / user / status / tool / resource_type 过滤，可选 `NEXUSIM_AGENT_PROPOSAL_APPROVAL_AUDIT_OUTPUT` 写低敏 JSON；`proposal-approval-approve` 需要 `NEXUSIM_AGENT_PROPOSAL_APPROVAL_TENANT_ID`、`NEXUSIM_AGENT_PROPOSAL_APPROVAL_PROPOSAL_ID`、`NEXUSIM_AGENT_PROPOSAL_APPROVAL_APPROVED_BY_USER_ID`，默认 `NEXUSIM_AGENT_PROPOSAL_APPROVAL_DRY_RUN=true`，只有显式设为 false 才会调用服务内 approval workflow 并同事务写 approval outbox。审批 reason 应通过 `NEXUSIM_AGENT_PROPOSAL_APPROVAL_REASON_FILE` 读取；输出可选 `NEXUSIM_AGENT_PROPOSAL_APPROVAL_OUTPUT`，只包含 proposal / approval / skill / tool / resource / status 元数据和 `reason_present`，不输出 objective、proposal_text、citations、EvidencePack 或 reason 原文。

`policy-service` 还提供 `rebac-relation-audit` / `rebac-relation-set`，用于审计和设置 first-stage ReBAC relation gate 规则。规则按 tenant / action / relation_type / conversation_scope 要求 `DIRECT_CONTACT_ACTIVE` 或 `CONVERSATION_MEMBER_ACTIVE` 关系，在 exact / tenant allow 规则前 fail-closed deny；`rebac-relation-audit` 可选 `NEXUSIM_POLICY_REBAC_RELATION_AUDIT_OUTPUT` 写低敏 JSON，`rebac-relation-set` 可选 `NEXUSIM_POLICY_REBAC_RELATION_SET_OUTPUT` 写低敏 JSON。`rebac-relation-set` 支持 `NEXUSIM_POLICY_REBAC_RELATION_SET_REASON_FILE` 读取 operator reason 原文，避免把 reason 写进 operator plan / shell env；输出只包含规则元数据和 reason-present，不输出 operator reason 原文。该能力不是 provider-grade ReBAC graph / policy DSL。

`action-executor` 提供 `provider-failure-audit` / `provider-failure-redrive-plan` /
`provider-replay-operator-ui` / `provider-replay-handoff`，用于只读审计 provider failure
投影、生成低敏 redrive plan、生成 provider replay 人工审批视图，以及生成 admin /
workflow handoff request。环境变量为 `NEXUSIM_ACTION_EXECUTOR_MODE`；
`provider-failure-audit` 可选 `NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_AUDIT_OUTPUT`
写低敏 JSON；`provider-failure-redrive-plan` 必须使用
`NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_DRY_RUN=true` 和
`NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_REASON_FILE`，可选
`NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_PLAN_OUTPUT` 写低敏 plan；
`provider-replay-operator-ui` 只读取 `DLQ` candidates，并通过
`NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_UI_OUTPUT` 写低敏 UI artifact。
`provider-replay-handoff` 只读取 `DLQ` candidates，并通过
`NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_OUTPUT` 写低敏 handoff artifact；它要求
`NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_OPERATOR_REF`、
`NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_REASON_REF` 和
`NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_EVIDENCE_REFS`，可选
`NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_OPERATOR_ROLE` /
`CORRELATION_ID` / `TRACE_ID`。输出包含 `PROVIDER_REPLAY_REQUEST` admin operation request
和 `REPAIR_APPROVAL` workflow handoff request，只带 hash/ref、candidate id 和 required gates；
不包含 raw provider input / output / provider error / operator reason。上述模式都不修改
provider failure row、不调用 tool executor、不重放 provider output。
可用以下命令把 handoff artifact 渲染成仓库外低敏 HTML 审查页：

```powershell
.\tools\write-provider-replay-handoff-review-page.ps1 -HandoffPath H:\NexusIM\operator-plans\provider-replay-handoff.json -GeneratedBy operator-a -OutputPath H:\NexusIM\operator-plans\provider-replay-handoff-review.html
```

该页面只展示 handoff contract、admin / workflow refs、payload hash、candidate refs 和
required gates；不提交 admin operation、不创建 workflow、不记录 approval、不调用
`RedriveProviderFailure`、不修改 DLQ row，也不输出 raw provider input / output、provider
error、new input、operator reason、EvidencePack 或本机路径。

admin operation 和 workflow decision 都完成后，可用以下命令把 approved admin operation、
workflow APPROVE manifest、fresh Agent proof 和原 handoff 绑定成最终 redrive 前的低敏
readiness 审查页：

```powershell
.\tools\write-provider-replay-readiness-page.ps1 -HandoffPath H:\NexusIM\operator-plans\provider-replay-handoff.json -AdminOperationPath H:\NexusIM\operator-plans\provider-replay-admin-approved.json -WorkflowDecisionManifestPath H:\NexusIM\operator-plans\workflow-decision.json -FreshProofPath H:\NexusIM\operator-plans\provider-replay-fresh-proof.json -GeneratedBy operator-a -OutputPath H:\NexusIM\operator-plans\provider-replay-readiness.html
```

`provider-replay-fresh-proof.json` 只允许低敏 refs / hashes：schema version、
replay candidate id、provider failure ref hash、admin operation id / payload hash、
workflow decision manifest sha256、fresh proposal / approval / prepared audit id、
skill / tool / resource hash、new input sha256 和 reason sha256。readiness page 不调用
`RedriveProviderFailure`，只证明最终执行前的 admin approval、workflow approval 和 fresh
Agent proof 已绑定；raw new input、operator reason、provider artifacts 和本机路径都不得进入页面。

readiness 审查通过后，可生成最终执行前的低敏 redrive invocation manifest：

```powershell
.\tools\write-provider-replay-redrive-invocation.ps1 -HandoffPath H:\NexusIM\operator-plans\provider-replay-handoff.json -AdminOperationPath H:\NexusIM\operator-plans\provider-replay-admin-approved.json -WorkflowDecisionManifestPath H:\NexusIM\operator-plans\workflow-decision.json -FreshProofPath H:\NexusIM\operator-plans\provider-replay-fresh-proof.json -GeneratedBy operator-a -OutputPath H:\NexusIM\operator-plans\provider-replay-redrive-invocation.json
```

该 manifest 只输出 `RedriveProviderFailure` 的低敏 command contract：source failure id、
admin / workflow / fresh proposal / approval / prepared audit refs、resource id hash、
new input sha256 和 reason sha256。它不调用 action-executor、不修改 DLQ row、不包含
raw `resource_id`、raw `input_json`、operator reason、provider artifact 或 EvidencePack；
operator 真正执行 redrive 前必须在仓库外提供 raw resource id / new input，并重新计算 hash
与 manifest 中的 refs 对齐。

`loadtest/admin` 提供 provider replay handoff operator bridge：`provider-replay-submit`
读取上面的 handoff artifact，校验 `PROVIDER_REPLAY_REQUEST` contract、payload hash、
低敏 refs、`direct_execution_allowed=false` 和 `source_dlq_immutable=true` 后，调用
admin-service `CreateAdminOperation` 创建管理操作；`provider-replay-list` 只列
`PROVIDER_REPLAY_REQUEST`；`provider-replay-approve` / `provider-replay-reject` 使用
`admin.workflow.provider_replay.v1` 审批 policy。该 bridge 不调用 workflow-service、
不调用 action-executor `RedriveProviderFailure`、不修改 DLQ row；后续仍由
admin-service operation-worker 路由 workflow，并由 fresh proposal / approval /
prepared audit / new input / reason hash 触发最终 redrive。

provider replay 最终执行后，如需把低敏外部审计事实追加到 `audit-service`，使用
`loadtest/actionexecutor` 的 external audit append operator：

```powershell
go run ./loadtest/actionexecutor -mode external-audit-append -audit-manifest H:\NexusIM\operator-plans\action-executor-audit-append.json -operator-user-id operator-a -operator-device-id operator-device-a
```

默认只做 preflight：校验仓库外低敏 manifest、`attributes_json` hash、required checks、
operator identity 和敏感字段禁入，不调用 audit-service。显式加 `-execute` 时才通过
audit-service 公开 gRPC `AppendAuditRecord` 追加审计；operator 不直接写 audit-service
私表，不打印 manifest 本机路径、raw provider input / output、provider body、raw
attributes JSON 或 credential-like 内容。

## Delivery Projection

`delivery-service` 额外拥有 projection 排障入口：

| 模式 | 作用 |
| --- | --- |
| `projection-failure-audit` | 只读列出 unresolved projection failure，支持按 offset / event / failure class / `last_seen_at` RFC3339 时间窗口缩小范围；可选 `NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_OUTPUT` 写低敏 JSON 结果。 |
| `projection-checkpoint-repair` | 带审计回调 checkpoint 做 replay；只允许回调，不允许前跳跳过事件；支持 `NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN=true` 只写 repair audit、不 mutate checkpoint；可选 `NEXUSIM_DELIVERY_PROJECTION_REPAIR_OUTPUT` 写低敏 JSON summary。 |
| `projection-checkpoint-repair-audit` | 只读列出 checkpoint repair audit 历史；可选 `NEXUSIM_DELIVERY_PROJECTION_REPAIR_AUDIT_OUTPUT` 写低敏 JSON 结果。 |
| `projection-checkpoint-repair-cleanup` | 清理超过保留期的 checkpoint repair audit 历史；支持 `NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_DRY_RUN=true` 只统计候选行不删除；可选 `NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_OUTPUT` 写低敏 JSON summary。 |
| `projection-failure-resolve` | 人工确认指定 unresolved failure 已外部补偿或不再作为 blocker 后，带 operator / reason / dry-run 审计地标记 resolved；支持 `NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_DRY_RUN=true` 只写 resolution audit、不 mutate failure 状态；不移动 Kafka checkpoint；可选 `NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_OUTPUT` 写低敏 JSON summary。 |
| `projection-failure-cleanup` | 只清理 resolved 且超过保留期的 failure 审计行，不触碰 unresolved blocker；支持 `NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_DRY_RUN=true` 只统计候选行不删除；可选 `NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_OUTPUT` 写低敏 JSON summary。 |

`projection-checkpoint-repair` 支持 `NEXUSIM_DELIVERY_PROJECTION_REPAIR_REASON_FILE`，`projection-failure-resolve` 支持 `NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_REASON_FILE`，用于从文件读取 operator reason 原文，避免把 reason 写进 operator plan / shell env。

## Conversation Member Change

`conversation-service` 当前提供：

环境变量：`NEXUSIM_CONVERSATION_SERVICE_MODE`

| 模式 | 作用 |
| --- | --- |
| `member-change-audit` | 只读审计 `member_change_saga`，可按 tenant / conversation / target user / operator / change type / status / change_id / outbox event / `updated_at` RFC3339 时间窗口缩小范围；可选 `NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_OUTPUT` 写低敏 JSON 结果。 |
| `member-window-audit` | 只读审计 `conversation_members` 当前窗口 cache 异常，覆盖 ACTIVE 无 join_seq、ACTIVE 带 leave_seq、inactive 无 leave_seq、leave_seq 早于 join_seq、成员版本高于会话版本、非 ACTIVE 会话仍有 ACTIVE 成员，以及 ACTIVE 会话无 ACTIVE OWNER / 多个 ACTIVE OWNER 等 issue class；支持 `updated_at` RFC3339 时间窗口过滤，可选 `NEXUSIM_CONVERSATION_MEMBER_WINDOW_AUDIT_OUTPUT` 写低敏 JSON 结果。 |
| `member-window-repair` | 保守修复成员窗口 cache：`ACTIVE_WITHOUT_JOIN_SEQ` 只会在 ACTIVE、无 `leave_seq`、`member_version` 合法且不超过会话版本时把 `join_seq` 补为 `member_version`；`ACTIVE_WITH_LEAVE_SEQ` 会清空 ACTIVE 成员残留的历史 `leave_seq`；`INACTIVE_WITHOUT_LEAVE_SEQ` 会在 inactive 成员有合法 `join_seq` / `member_version` 时把 `leave_seq` 补为 `member_version`；inactive `LEAVE_BEFORE_JOIN` 会把早于 `join_seq` 的 `leave_seq` clamp 到 `join_seq`；`MEMBER_VERSION_AHEAD_CONVERSATION` / `PERMISSION_VERSION_AHEAD_CONVERSATION` 会把 conversation 版本 floor 提升到当前成员最大版本；`ACTIVE_MEMBER_IN_INACTIVE_CONVERSATION` 会把非 ACTIVE 会话内仍 ACTIVE 且窗口合法的成员标为 `LEFT` 并补 `leave_seq = member_version`；默认 `NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_DRY_RUN=true`，只有显式设为 false 才 mutate；可选 `NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_OUTPUT` 写低敏 JSON summary。 |
| `member-window-repair-audit` | 只读审计 `member-window-repair` 历史；可按 tenant / conversation / user / issue class / outcome / `repaired_at` RFC3339 时间窗口缩小范围；可选 `NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_AUDIT_OUTPUT` 写低敏 JSON 结果和 compacted filters。 |

`member-window-repair` 支持 `NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_REASON_FILE` 读取 operator reason 原文，避免把 reason 写进 operator plan / shell env；输出仍只写低敏 repair 元数据。

完整历史窗口 / targeted replay repair 仍是后续工作；执行 mutate 前仍应先用 `member-window-audit` 缩小范围并留存证据，不能用手写 SQL 直接改成员事实。

## Identity Challenge / Session

`identity-service` 当前提供：

环境变量：`NEXUSIM_IDENTITY_SERVICE_MODE`

| 模式 | 作用 |
| --- | --- |
| `session-mfa-proof-audit` | 只读发现历史 session MFA proof 脏数据；可选 `NEXUSIM_IDENTITY_SESSION_MFA_PROOF_AUDIT_OUTPUT` 写低敏聚合 JSON 结果。 |
| `challenge-delivery-repair` | 处理 challenge delivery outbox / retry / expire / DLQ 相关修复；支持 `NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_DRY_RUN=true` 只写 repair audit、不 mutate delivery / challenge 状态；可选 `NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_OUTPUT` 写低敏 JSON summary。 |
| `challenge-delivery-repair-audit` | 只读审计 challenge delivery repair 历史；支持按 delivery / tenant / user / challenge / mode / outcome / failure class / `repaired_at` RFC3339 时间窗口缩小范围；可选 `NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_OUTPUT` 写低敏 JSON 结果和 compacted filters。 |
| `challenge-delivery-repair-cleanup` | 按 retention / scope 清理 challenge delivery repair audit 历史；支持 `NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_DRY_RUN=true` 只统计候选行不删除；可选 `NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_OUTPUT` 写低敏 JSON summary。 |
| `challenge-request-limit-cleanup` | 清理 verification / password reset request limit 历史；支持 `NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_DRY_RUN=true` 只统计候选行不删除；可选 `NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_OUTPUT` 写低敏 JSON summary。 |
| `gateway-token-keyring-rotate` | 轮换本地 RS256 gateway token keyring 文件；生成新当前私钥，把旧当前 key 降级为 public-only overlap，并按 old-key limit 保留旧公钥；可选 `NEXUSIM_IDENTITY_GATEWAY_TOKEN_KEYRING_ROTATE_OUTPUT` 写不含 JWK 材料的低敏 JSON summary。 |

`challenge-delivery-repair` 支持 `NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_REASON_FILE` 读取 operator reason 原文，避免把 reason 写进 operator plan / shell env；输出仍只写低敏 delivery / repair 元数据。

`gateway-token-keyring-rotate` 只处理本地 secret-bearing JSON 文件。它不是 KMS / HSM、不会跨主机分发密钥，也不替代正式密钥管理审批。

## Contacts Policy Operators

`contacts-service` 额外拥有租户默认值和来源策略 operator：

| 模式 | 作用 |
| --- | --- |
| `tenant-privacy-default-audit` | 只读审计租户联系人隐私默认值；可选 `NEXUSIM_CONTACTS_TENANT_PRIVACY_AUDIT_OUTPUT` 写低敏 JSON 结果。 |
| `tenant-privacy-default-set` | 设置租户联系人隐私默认值；可选 `NEXUSIM_CONTACTS_TENANT_PRIVACY_SET_OUTPUT` 写低敏 JSON 结果。 |
| `source-policy-audit` | 只读审计联系人来源策略；可选 `NEXUSIM_CONTACTS_SOURCE_POLICY_AUDIT_OUTPUT` 写低敏 JSON 结果。 |
| `source-policy-set` | 设置联系人来源策略；可选 `NEXUSIM_CONTACTS_SOURCE_POLICY_SET_OUTPUT` 写低敏 JSON 结果。 |
| `contact-request-review` | 审批 `REVIEW_REQUIRED` 联系人申请；可选 `NEXUSIM_CONTACTS_REQUEST_REVIEW_OUTPUT` 写低敏 JSON 结果，只写 reason-present，不写审核 reason 原文。 |
| `contact-request-review-audit` | 只读导出联系人申请审核审计；按 tenant / request / operator / decision / next_status / source_type / risk_level / review_required / `reviewed_at` RFC3339 时间窗口过滤；可选 `NEXUSIM_CONTACTS_REQUEST_REVIEW_AUDIT_OUTPUT` 写低敏 JSON 结果和 compacted filters，只写 reason-present，不写审核 reason 原文。 |

`contact-request-review` 支持 `NEXUSIM_CONTACTS_REQUEST_REVIEW_REASON_FILE` 读取审核 reason 原文，避免把 reason 写进 operator plan / shell env。输出仍只写 `reason_present`。

这些仍是本地 operator 形态；后续 admin/config service 接入后，应迁移到正式权限面和审批流。

## 仍未完成

- 跨服务统一 repair runbook 的执行编排。
- 正式批量 repair 执行编排、审批系统和运维 UI。
- provider-grade 外部 audit sink。
- 更细 poison payload 分类和长期 retention 策略。
