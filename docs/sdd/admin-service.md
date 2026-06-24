# admin-service SDD v0.1 Draft

## 1. 服务定位

`admin-service` 是 NexusIM 的管理后台 API 和 operator workflow 入口。它负责租户管理、
用户 / 设备 / 会话管理入口、封禁 / 解封请求、配置操作入口、repair 审批、运维操作
编排和低敏管理审计。

职责：

- 拥有 `admin_operation`、`admin_approval`、`admin_operation_result` 和
  `admin_outbox`。
- 为管理后台、CLI operator 和 workflow-service 提供统一管理 API。
- 校验管理身份、角色、scope、二次审批和幂等键。
- 通过公开 API / operator command 调用 identity、contacts、policy、conversation、
  control-plane、audit、notification 等服务。
- 为高风险操作生成 proposal / approval / execution audit 关联。

不负责：

- 不承载普通用户 IM 流量，不替代 api-gateway 的用户入口。
- 不直接写其它服务私有表，不通过 SQL 修业务数据。
- 不拥有 identity、contacts、conversation、policy、delivery、message 或 media facts。
- 不替代 control-plane 的配置版本事实，不替代 workflow-service 的长事务等待。
- 不保存 raw password、token、TOTP、message body、provider body、raw prompt、
  EvidencePack 原文或 operator reason 原文。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游 | admin UI / CLI / api-gateway admin facade | 管理请求、审批、查询 |
| 同步依赖 | identity-service | revoke device/session、user state query、future ban/unban |
| 同步依赖 | contacts-service | contact review、privacy/source policy operator |
| 同步依赖 | policy-service | admin permission precheck、policy operator / relation / quota |
| 同步依赖 | control-plane-service | config publish / rollback / applied status |
| 同步依赖 | audit-service | audit query / export / proof |
| 同步依赖 | workflow-service | long approval / async operation orchestration |
| 异步下游 | audit-service / notification-service | admin operation events / operator notifications |
| 事实源 | PostgreSQL | operation、approval、result、outbox |

第一版可以先提供 internal gRPC API；正式 Web admin UI、session UI 和前端权限面后置。

## 3. 六层 DDD 包结构

```text
services/admin-service/
  cmd/admin-service/
  internal/{api,app,domain,infrastructure,types,trigger}/
```

| 层 | 本服务内容 |
| --- | --- |
| `api` | gRPC adapter，verified admin metadata，稳定错误映射 |
| `app` | CreateAdminOperation、ApproveAdminOperation、ExecuteAdminOperation、QueryAdminOperations |
| `domain` | operation 状态机、risk level、approval policy、scope validation |
| `infrastructure` | PostgreSQL repository、service RPC clients、policy / audit / workflow clients |
| `types` | command、DTO、错误码、枚举、low-sensitive metadata |
| `trigger` | operation worker、outbox relay、cleanup worker |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `AdminOperation` | 一次管理操作请求 | append-only command summary；tenant scoped |
| `AdminApproval` | 审批记录 | high risk 操作必须至少一个有效 approval |
| `AdminOperationResult` | 执行结果投影 | 只保存低敏 outcome 和 target refs |
| `AdminOutboxEvent` | 管理事件 | 只通过 outbox relay 发布 |

Operation 类型第一版：

```text
TENANT_CREATE
TENANT_UPDATE
TENANT_DISABLE
USER_BAN
USER_UNBAN
DEVICE_REVOKE
SESSION_REVOKE
CONTACT_REQUEST_REVIEW
CONTACT_PRIVACY_POLICY_CHANGE
POLICY_RULE_CHANGE
REBAC_RELATION_CHANGE
TENANT_QUOTA_CHANGE
CONFIG_PUBLISH
CONFIG_ROLLBACK
REPAIR_REQUEST
PROVIDER_REPLAY_REQUEST
AUDIT_EXPORT_REQUEST
NOTIFICATION_SUPPRESSION_CHANGE
```

Operation 状态：

```text
DRAFT -> SUBMITTED -> APPROVED -> EXECUTING -> SUCCEEDED
DRAFT/SUBMITTED -> CANCELED
SUBMITTED -> REJECTED
APPROVED/EXECUTING -> FAILED
FAILED -> COMPENSATION_REQUESTED
```

## 5. 同步 API 契约

```text
rpc CreateAdminOperation(CreateAdminOperationRequest) returns (CreateAdminOperationResponse)
rpc ApproveAdminOperation(ApproveAdminOperationRequest) returns (ApproveAdminOperationResponse)
rpc ExecuteAdminOperation(ExecuteAdminOperationRequest) returns (ExecuteAdminOperationResponse)
rpc GetAdminOperation(GetAdminOperationRequest) returns (GetAdminOperationResponse)
rpc ListAdminOperations(ListAdminOperationsRequest) returns (ListAdminOperationsResponse)
```

`CreateAdminOperation` 请求字段：

```text
tenant_id, operator_user_id, operator_role
operation_type, target_ref, risk_level
operation_payload_json
reason_ref, evidence_refs
idempotency_key
correlation_id, causation_id, trace_id
```

`operation_payload_json` 只允许每个 operation type 的低敏 schema 字段，例如：

```text
target_user_ref, device_ref, session_ref, config_bundle_key, config_version,
quota_rps, quota_burst, policy_rule_ref, repair_mode, audit_export_filter_hash,
provider_failure_ref_hash, source_execution_ref_hash, source_result_ref_hash,
redrive_entrypoint, source_dlq_immutable, direct_execution_allowed
```

禁止字段：

```text
password, token, TOTP/recovery code, raw message body, raw prompt,
raw EvidencePack, provider body, SQL error, DSN, private key, object storage key,
operator reason raw text
```

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | operation type、payload、reason/evidence ref 非法 | 否 |
| `PERMISSION_DENIED` | 管理员角色、scope 或 policy precheck 拒绝 | 否 |
| `FAILED_PRECONDITION` | 缺审批、状态不允许、target 当前状态不匹配 | 否 |
| `ALREADY_EXISTS` | idempotency key replay 命令冲突 | 否 |
| `NOT_FOUND` | operation / approval / target ref 不存在 | 否 |
| `UNAVAILABLE` | 下游服务或存储暂不可用 | 是 |

## 6. 执行边界

`admin-service` 不直接执行跨服务 SQL。执行策略：

| Operation | 执行方式 |
| --- | --- |
| `DEVICE_REVOKE` / `SESSION_REVOKE` | 调 identity-service admin RPC |
| `CONTACT_REQUEST_REVIEW` | 调 contacts-service review RPC / operator port |
| `POLICY_RULE_CHANGE` | 第一阶段发布低敏 `POLICY_RULESET_REF` 配置引用到 control-plane-service；后续完整 policy mutation 再接 policy-service operator port |
| `REBAC_RELATION_CHANGE` | 调 policy-service operator port |
| `TENANT_QUOTA_CHANGE` / `CONFIG_PUBLISH` / `CONFIG_ROLLBACK` | 调 control-plane-service |
| `REPAIR_REQUEST` | 创建 workflow request 或生成服务专用 operator command |
| `PROVIDER_REPLAY_REQUEST` | 创建 workflow repair approval；最终执行仍由 action-executor `RedriveProviderFailure` 完成 |
| `AUDIT_EXPORT_REQUEST` | 调 audit-service export API |
| `NOTIFICATION_SUPPRESSION_CHANGE` | 调 notification-service suppression API |

第一版 operation-worker 已支持最小 workflow 路由：

```text
REPAIR_REQUEST -> workflow-service CreateWorkflow(REPAIR_APPROVAL)
PROVIDER_REPLAY_REQUEST -> workflow-service CreateWorkflow(REPAIR_APPROVAL)
CRITICAL non-repair operation -> workflow-service CreateWorkflow(ADMIN_OPERATION)
```

该路径只传 `target_ref_hash`、`payload_hash`、`reason_ref` 和 `evidence_refs` 等低敏
ref/hash；admin-service result 只记录 `workflow:<workflow_id>`。未配置
`NEXUSIM_WORKFLOW_GRPC_ADDR` 时，`REPAIR_REQUEST` / `CRITICAL` operation 不得走
本地 no-op executor，必须 fail-closed。

第一版 workflow request 会按 operation 类型写入专用 approval policy 和 target
service：`CONFIG_PUBLISH` / `CONFIG_ROLLBACK` / `TENANT_QUOTA_CHANGE` /
`POLICY_RULE_CHANGE` 指向 `control-plane-service`，完整 policy mutation / ReBAC
操作指向 `policy-service`，
`AUDIT_EXPORT_REQUEST` 指向 `audit-service`，`NOTIFICATION_SUPPRESSION_CHANGE`
指向 `notification-service`，`PROVIDER_REPLAY_REQUEST` 指向 `action-executor` 并使用
`admin.workflow.provider_replay.v1`。未映射的 `CRITICAL` operation 仍使用
`admin.workflow.operation.v1` 和 `admin-service` target，等待后续专用 adapter。

第一版真实下游 adapter 已覆盖四类非 `CRITICAL` 的 control-plane operation：

```text
operation-worker
-> parse operation_payload_json as admin.config_publish.v1
-> control-plane-service.PublishConfigVersion
-> admin result downstream_service=control-plane-service

operation-worker
-> parse operation_payload_json as admin.config_rollback.v1
-> control-plane-service.RollbackConfigVersion
-> admin result downstream_service=control-plane-service

operation-worker
-> parse operation_payload_json as admin.tenant_quota_change.v1
-> control-plane-service.PublishConfigVersion(API_GATEWAY_TENANT_QUOTA)
-> admin result downstream_service=control-plane-service

operation-worker
-> parse operation_payload_json as admin.policy_rule_change.v1
-> control-plane-service.PublishConfigVersion(POLICY_RULESET_REF)
-> admin result downstream_service=control-plane-service
```

该 adapter 只在配置 `NEXUSIM_CONTROL_PLANE_GRPC_ADDR` 时启用；未配置时保留
first-stage local no-op recovery。`CRITICAL` risk 的 control-plane operation 仍按上面的
workflow 路由处理，不在 admin-service 内联执行。

高风险 operation 默认走：

```text
policy precheck
-> CreateAdminOperation
-> ApproveAdminOperation
-> ExecuteAdminOperation
-> downstream public API / workflow
-> admin result + audit outbox
```

## 7. 异步事件契约

| 事件 | Topic | 分区键 | 说明 |
| --- | --- | --- | --- |
| `admin.operation.submitted.v1` | `im.admin.events` | `tenant_id:operation_id` | operation 已提交 |
| `admin.operation.approved.v1` | `im.admin.events` | `tenant_id:operation_id` | 审批通过 |
| `admin.operation.executed.v1` | `im.admin.events` | `tenant_id:operation_id` | 执行完成 |
| `admin.operation.failed.v1` | `im.admin.events` | `tenant_id:operation_id` | 执行失败 |
| `admin.operation.compensation_requested.v1` | `im.admin.events` | `tenant_id:operation_id` | 已请求补偿 |
| `admin.operation.canceled.v1` | `im.admin.events` | `tenant_id:operation_id` | 已取消 |

事件 payload 只包含 operation id、operation type、target ref hash、risk level、
status、operator hash、approval hash / approval id、result id、downstream service、
downstream request ref、failure_class、public_error、compensation requester hash、
compensation reason ref、correlation/causation refs。禁止输出 payload_json 原文、
reason 原文、下游 response body 或 secret。

## 8. 数据库设计

第一版表：

```text
admin_operations
admin_approvals
admin_operation_results
admin_outbox
```

关键字段：

```text
admin_operations:
tenant_id, operation_id, idempotency_key, operation_type,
target_ref, target_ref_hash, risk_level, payload_schema_version,
payload_json, payload_hash, reason_ref, evidence_refs_json,
status, requested_by, requested_at, approved_at, executed_at

admin_approvals:
tenant_id, approval_id, operation_id, approver_ref,
approval_decision, approval_policy, reason_ref, created_at

admin_operation_results:
tenant_id, result_id, operation_id, downstream_service,
downstream_request_ref, status, failure_class, public_error,
created_at, completed_at

admin_outbox:
event_id, tenant_id, operation_id, event_type, event_version,
partition_key, payload_json, status, retry_count, next_retry_at, published_at
```

`payload_json` 是服务内部低敏 command summary；高敏操作参数必须以 hash/ref 形式保存。

## 9. 核心流程

创建 operation：

```text
CreateAdminOperation
-> verify admin metadata
-> policy-service admin precheck
-> validate per-operation schema
-> compute payload_hash
-> insert admin_operations(SUBMITTED)
-> write admin.operation.submitted.v1 outbox
```

审批：

```text
ApproveAdminOperation
-> verify approver role and separation-of-duty
-> lock operation
-> insert admin_approvals
-> transition APPROVED or REJECTED
-> write admin.operation.approved/rejected event
```

执行：

```text
ExecuteAdminOperation
-> lock APPROVED operation
-> call downstream public API / workflow
-> record result
-> transition SUCCEEDED / FAILED
-> write admin.operation.executed/failed event
```

## 10. 一致性和事务

强一致边界：

- operation status、approval/result、admin_outbox 在同一 PostgreSQL 事务内更新。
- idempotency replay 不创建重复 operation 或重复 submitted event。
- downstream response 只能在 admin result 中保存低敏 outcome。

最终一致边界：

- 下游服务执行和 admin-service status 之间可能需要 worker 补偿。
- workflow-service 负责长事务等待；admin-service 只保存 operation ref 和当前视图。
- audit-service 通过 `im.admin.events` 最终归档。

## 11. 幂等、审批和补偿

| 场景 | 幂等键 | 重试策略 | 补偿 |
| --- | --- | --- | --- |
| CreateAdminOperation | tenant + operator + idempotency_key | replay 返回 operation | command hash 冲突 fail closed |
| ApproveAdminOperation | operation + approver + decision id | replay 返回 approval | separation-of-duty violation fail closed |
| ExecuteAdminOperation | operation_id | worker retry + downstream idempotency key | compensation request |
| OutboxRelay | event_id | bounded retry + DLQ | outbox repair operator |

Approval rule 第一版：

```text
LOW risk: one admin may approve and execute
MEDIUM risk: one approval, approver can differ from requester
HIGH risk: separation of duty; requester cannot be sole approver
CRITICAL risk: route to workflow-service; admin-service does not execute inline
```

## 12. 权限和安全

- Admin API 只接受 gateway-verified admin metadata 或 mTLS service identity。
- request body 中的 tenant / operator 不能覆盖 trusted metadata。
- policy-service 必须参与 admin precheck；policy unavailable 时 fail closed。
- 高风险操作必须携带 reason_ref 和 evidence_refs，不能把 reason 原文放到 env / shell。
- 所有下游调用都必须携带 correlation_id / causation_id。
- 操作查询默认只返回当前 operator scope 内记录；跨 tenant 查询禁止。
- debug / metrics 不输出 operator、target、payload、reason、tenant 明细或 downstream body。

## 13. 与其它 future 服务关系

- `control-plane-service` 拥有配置版本事实；admin-service 只提供管理入口和审批。
- `workflow-service` 拥有长事务、等待和补偿；admin-service 只创建或展示 workflow ref。
- `audit-service` 拥有统一审计归档；admin-service 只发布低敏 admin event。
- `notification-service` 负责通知管理员 / 用户；admin-service 不直接对接 provider。

## 14. SLO 和指标

第一阶段不写生产 SLO，只保留本地 / 面试展示指标：

```text
admin_operation_total{operation_type,status,risk_level}
admin_approval_total{decision,risk_level}
admin_operation_execution_total{downstream_service,status,failure_class}
admin_outbox_total{status}
admin_policy_precheck_total{operation_type,decision}
```

metrics 禁止输出 tenant_id、operator_ref、target_ref、operation_id、approval_id、
payload_json、reason_ref、evidence refs 或 downstream response。

## 15. 测试方案

| 测试 | 目标 |
| --- | --- |
| domain unit | operation 状态机、risk/approval rule、payload schema |
| app unit | policy deny、idempotency、separation-of-duty、downstream failure |
| PostgreSQL integration | operation + approval + result + outbox 同事务 |
| RPC adapter | admin metadata、稳定错误映射、trusted metadata override 防护 |
| event builder | 不输出 payload_json / reason / downstream body |
| smoke | Create -> Approve -> Execute fake downstream -> Query result |

## 16. Runbook

运行模式：

```text
NEXUSIM_ADMIN_SERVICE_MODE=grpc
NEXUSIM_ADMIN_SERVICE_MODE=operation-worker
NEXUSIM_ADMIN_SERVICE_MODE=outbox-relay
NEXUSIM_ADMIN_SERVICE_MODE=cleanup
NEXUSIM_ADMIN_SERVICE_MODE=compensation-request
```

`operation-worker` 第一版配置：

```text
NEXUSIM_WORKFLOW_GRPC_ADDR=127.0.0.1:10820
NEXUSIM_ADMIN_WORKFLOW_RPC_TIMEOUT=1s
NEXUSIM_CONTROL_PLANE_GRPC_ADDR=127.0.0.1:10760
NEXUSIM_ADMIN_CONTROL_PLANE_RPC_TIMEOUT=1s
```

未设置 `NEXUSIM_WORKFLOW_GRPC_ADDR` 时，`REPAIR_REQUEST` / `CRITICAL` operation 会
fail-closed 并记录失败结果，不会被本地 no-op executor 标记为成功。
未设置 `NEXUSIM_CONTROL_PLANE_GRPC_ADDR` 时，非 critical `CONFIG_PUBLISH` /
`CONFIG_ROLLBACK` / `TENANT_QUOTA_CHANGE` / `POLICY_RULE_CHANGE` 仍保持第一阶段
local executor recovery；
设置后才调用 control-plane public gRPC。

operator：

```text
loadtest/admin -mode create
loadtest/admin -mode approve
loadtest/admin -mode reject
loadtest/admin -mode get
loadtest/admin -mode list
admin-outbox-repair
NEXUSIM_ADMIN_SERVICE_MODE=compensation-request
```

`loadtest/admin` 只调用公开 admin gRPC，不读私表，不输出 payload 原文、reason
原文、EvidencePack 正文或 downstream response body。
`compensation-request` 是 first-stage 本地 operator，默认 dry-run。正式执行要求
`NEXUSIM_ADMIN_COMPENSATION_REASON_REF` 或
`NEXUSIM_ADMIN_COMPENSATION_REASON_FILE`；reason file 只计算 hash / ref，不会把
reason 原文写入数据库、outbox 或 summary。
设置 `NEXUSIM_WORKFLOW_GRPC_ADDR` 后，`compensation-request` 会创建 / replay
workflow-service `COMPENSATION_REQUEST`，用于后续人工审批和 workflow-service
`compensation-worker` 物化补偿请求；admin-service 不在该 mode 内联执行真实补偿
mutation。workflow-service 第一版 `compensation-executor` 可通过显式 instruction file
或 workflow-service 自有 instruction registry 执行 control-plane rollback
compensation；DB registry instruction 必须绑定具体 compensation workflow 并校验低敏
refs。admin-service 仍只提供 operation 入口和低敏 refs。

## 17. 验收标准

进入编码前：

- 本 SDD v0.1 draft 被复核，无 P0/P1。
- `admin-service` brief 指向本 SDD。
- 明确第一版只做管理入口、审批和受控下游调用，不直接修业务表。

进入 first smoke 前：

- proto / migration / 六层 skeleton / cmd runtime 已落。
- PostgreSQL operation / approval / result / outbox tests 通过。
- fake downstream execution smoke 通过。
- high-risk operation 没有 approval 时 fail closed。
- payload、reason 原文、下游 response body 和 secret 不会出现在事件、metrics、
  audit 或 repair summary。
