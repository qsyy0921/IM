# workflow-service SDD v0.1 Draft

## 1. 服务定位

`workflow-service` 是 NexusIM 的长事务、人工审批等待、补偿和异步运维编排服务。
它承接不适合放在 IM 热路径里的审批、repair、retention、外部系统调用补偿和多步骤
operator workflow。

职责：

- 拥有 `workflow_requests`、`workflow_steps`、`workflow_decisions`、
  `workflow_timers`、`workflow_compensations` 和 `workflow_outbox`。
- 提供 action approval、repair approval、config rollout approval、retention cleanup、
  external callback wait 和 compensation request 的统一状态机。
- 为 admin-service、agent-service、action-executor、control-plane-service 提供长等待编排。
- 记录低敏 workflow metadata、decision refs、timer refs、compensation refs 和 audit refs。
- 通过公开 API / event / operator command 驱动下游服务，不直接改业务私表。

不负责：

- 不替代 agent-service proposal，也不决定 Agent 生成内容。
- 不替代 action-executor 执行业务动作或工具调用。
- 不替代 admin-service 管理 API，也不承载普通用户 IM 流量。
- 不替代 audit-service 的长期审计归档。
- 不进入 message / delivery / push / receipt 热路径同步等待。
- 不保存 raw operator reason、EvidencePack 正文、raw prompt、model output、provider body、secret 或业务 payload 原文。
- 不要求第一版绑定 Temporal；Temporal / Cadence / Durable Execution 引擎只是后续候选。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游 | admin-service | repair / config / tenant / user operation workflow |
| 上游 | agent-service | high-risk proposal approval wait |
| 上游 | action-executor | execution retry / compensation / redrive approval |
| 上游 | control-plane-service | publish / rollback approval and staged rollout wait |
| 上游 | operator / CLI | 人工审批、补偿、重试、取消 |
| 同步依赖 | policy-service | workflow create / approve / execute precheck |
| 同步依赖 | notification-service | approval request / timeout / result notification |
| 同步依赖 | audit-service | low-sensitive workflow audit append / query |
| 异步下游 | admin / agent / action / control / audit | workflow state and decision events |
| 事实源 | PostgreSQL | request、step、decision、timer、compensation、outbox |

第一版可以只用 PostgreSQL + worker tick 实现；若未来接 Temporal，只能作为 infrastructure
adapter，不能改变服务边界或把业务状态外包给 engine history。

## 3. 六层 DDD 包结构

```text
services/workflow-service/
  cmd/workflow-service/
  internal/{api,app,domain,infrastructure,types,trigger}/
```

| 层 | 本服务内容 |
| --- | --- |
| `api` | gRPC adapter，verified service/admin metadata，稳定错误映射 |
| `app` | CreateWorkflow、RecordDecision、AdvanceWorkflow、CancelWorkflow、GetWorkflow |
| `domain` | workflow 状态机、approval policy、timer、compensation、risk rule |
| `infrastructure` | PostgreSQL repository、service RPC clients、notification / audit clients |
| `types` | command、DTO、错误码、枚举、低敏 metadata |
| `trigger` | workflow worker、timer worker、outbox relay、compensation worker、cleanup |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `WorkflowRequest` | 一个长事务 / 审批请求 | tenant scoped；带 request type、risk、owner |
| `WorkflowStep` | 工作流步骤 | step order 单调；只能按状态机推进 |
| `WorkflowDecision` | 人工或系统决策 | append-only；separation-of-duty 可验证 |
| `WorkflowTimer` | 等待 / timeout / retry 计时器 | due_at 到期后 worker 推进 |
| `WorkflowCompensation` | 补偿动作请求 | 只保存低敏 target ref 和 outcome |
| `WorkflowOutboxEvent` | 低敏 workflow 事件 | 只通过 outbox relay 发布 |

Workflow 类型：

```text
ACTION_APPROVAL
REPAIR_APPROVAL
CONFIG_ROLLOUT
CONFIG_ROLLBACK
RETENTION_CLEANUP
EXTERNAL_CALLBACK_WAIT
COMPENSATION_REQUEST
ADMIN_OPERATION
```

状态：

```text
DRAFT -> SUBMITTED -> WAITING_DECISION -> APPROVED -> RUNNING -> SUCCEEDED
DRAFT/SUBMITTED/WAITING_DECISION -> CANCELED
WAITING_DECISION -> REJECTED
RUNNING -> FAILED
FAILED -> COMPENSATION_PENDING -> COMPENSATED
RUNNING/WAITING_DECISION -> TIMED_OUT
```

Step 状态：

```text
PENDING -> READY -> RUNNING -> SUCCEEDED
PENDING/READY/RUNNING -> FAILED
PENDING/READY/RUNNING -> SKIPPED
RUNNING -> WAITING_CALLBACK
WAITING_CALLBACK -> TIMED_OUT
```

## 5. 同步 API 契约

```text
rpc CreateWorkflow(CreateWorkflowRequest) returns (CreateWorkflowResponse)
rpc RecordWorkflowDecision(RecordWorkflowDecisionRequest) returns (RecordWorkflowDecisionResponse)
rpc AdvanceWorkflow(AdvanceWorkflowRequest) returns (AdvanceWorkflowResponse)
rpc CancelWorkflow(CancelWorkflowRequest) returns (CancelWorkflowResponse)
rpc GetWorkflow(GetWorkflowRequest) returns (GetWorkflowResponse)
rpc ListWorkflows(ListWorkflowsRequest) returns (ListWorkflowsResponse)
```

`CreateWorkflow` 请求字段：

```text
tenant_id, requester_ref, requester_service
workflow_type, risk_level
target_ref, target_service, target_operation
approval_policy_ref, timeout_policy_ref, compensation_policy_ref
payload_schema_version, payload_ref_hash
reason_ref, evidence_refs[]
idempotency_key
correlation_id, causation_id, trace_id
```

`payload_ref_hash` 只能是低敏 request summary 的 hash/ref，不能是业务 payload 原文。

`RecordWorkflowDecision` 请求字段：

```text
tenant_id, workflow_id, step_id
decision_type: APPROVE | REJECT | REQUEST_CHANGES | CANCEL
decider_ref, decision_policy_ref
reason_ref, evidence_refs[]
idempotency_key
```

响应字段：

```text
workflow_id, workflow_status, next_step_id, decision_id, outbox_event_id
```

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | workflow type、payload ref、policy ref、timer 非法 | 否 |
| `PERMISSION_DENIED` | policy precheck、decider scope 或 separation-of-duty 拒绝 | 否 |
| `FAILED_PRECONDITION` | 状态不允许、approval 缺失、target 当前状态不匹配 | 否 |
| `ALREADY_EXISTS` | idempotency replay command hash 冲突 | 否 |
| `NOT_FOUND` | workflow / step / decision 不存在或不可见 | 否 |
| `UNAVAILABLE` | policy、notification、audit 或存储暂不可用 | 是 |

## 6. 工作流类别

第一版支持三类最小 workflow：

| 类型 | 入口 | 目标 |
| --- | --- | --- |
| `ACTION_APPROVAL` | agent-service / action-executor | 高风险 tool action 审批等待 |
| `REPAIR_APPROVAL` | admin-service / operator | DLQ、repair、redrive、cleanup 审批 |
| `ADMIN_OPERATION` | admin-service | 非 repair 的 CRITICAL 管理操作审批等待 |

当前 admin-service 已接入第一版 `REPAIR_REQUEST -> REPAIR_APPROVAL`：admin operation
worker 只向 workflow-service 传 low-sensitive ref/hash，并把 workflow id 作为
admin result 的 downstream ref。非 repair 的 `CRITICAL` admin operation 使用
`ADMIN_OPERATION`，不能伪装成 repair approval。
`admin-service compensation-request` operator 已接入第一版
`COMPENSATION_REQUEST` workflow handoff：只传 low-sensitive target / payload /
reason refs，并使用稳定 idempotency key；workflow-service 当前会在审批通过后由
`compensation-worker` 物化 `workflow_compensations` 和
`workflow.compensation.requested.v1` outbox，并把 workflow 推进到
`COMPENSATION_PENDING`。第一版 `compensation-executor` 支持显式
`control-plane-rollback-file` adapter，也支持 `control-plane-rollback-store` 从
workflow-service 自有 `workflow_compensation_instructions` registry 解析 instruction：
operator 先导入低敏 instruction，再用 `payload_ref_hash` 匹配 workflow compensation，
调用 control-plane-service 公开 `RollbackConfigVersion`；缺 instruction 或 unsupported
target fail closed。DB registry instruction 必须绑定具体 `COMPENSATION_REQUEST`
workflow，导入时校验 workflow 已批准或待补偿、target / payload refs 一致；resolve
时只匹配同一 workflow。`ListWorkflowCompensationInstructions` 提供按 workflow 的
低敏 instruction refs / version / status 查询面，供后续 operator UI 使用；它不暴露
payload 原文、reason 原文或 downstream body。更多下游 adapter、provider-grade
instruction UI / external approval binding 和运维后置。

后续扩展：

| 类型 | 入口 | 目标 |
| --- | --- | --- |
| `CONFIG_ROLLOUT` | control-plane-service | staged config rollout / ACK wait |
| `RETENTION_CLEANUP` | admin / policy / media / knowledge | 合规删除、对象清理、delete proof |
| `EXTERNAL_CALLBACK_WAIT` | connector / notification / external system | 等待外部系统 callback |
| `COMPENSATION_REQUEST` | action-executor / admin-service | 失败后补偿动作 |

## 7. 异步事件契约

| 事件 | Topic | 分区键 | 说明 |
| --- | --- | --- | --- |
| `workflow.submitted.v1` | `im.workflow.events` | `tenant_id:workflow_id` | workflow 已提交 |
| `workflow.decision.recorded.v1` | `im.workflow.events` | `tenant_id:workflow_id` | 决策已记录 |
| `workflow.step.ready.v1` | `im.workflow.events` | `tenant_id:workflow_id` | 某步骤可执行 |
| `workflow.completed.v1` | `im.workflow.events` | `tenant_id:workflow_id` | workflow 成功 |
| `workflow.failed.v1` | `im.workflow.events` | `tenant_id:workflow_id` | workflow 失败 |
| `workflow.timed_out.v1` | `im.workflow.events` | `tenant_id:workflow_id` | workflow / step 超时 |
| `workflow.compensation.requested.v1` | `im.workflow.events` | `tenant_id:workflow_id` | 需要补偿 |

事件 payload 只包含 workflow id、type、status、risk、target ref hash、step id、decision id、
failure class、timer refs、correlation refs。禁止包含 reason 原文、业务 payload 原文、
EvidencePack、proposal 正文、provider body、secret、SQL error 或 operator shell 输出。

## 8. 数据库设计

第一版表：

```text
workflow_requests
workflow_steps
workflow_decisions
workflow_timers
workflow_compensations
workflow_compensation_instructions
workflow_outbox
```

关键字段：

```text
workflow_requests:
tenant_id, workflow_id, idempotency_key, workflow_type, risk_level,
requester_ref, requester_service, target_service, target_operation,
target_ref_hash, payload_schema_version, payload_ref_hash,
approval_policy_ref, timeout_policy_ref, compensation_policy_ref,
reason_ref, evidence_refs_json, status, created_at, updated_at, completed_at

workflow_steps:
tenant_id, workflow_id, step_id, step_index, step_type,
target_service, target_operation, status, retry_count,
failure_class, public_error, due_at, created_at, updated_at

workflow_decisions:
tenant_id, workflow_id, decision_id, step_id, decider_ref,
decision_type, decision_policy_ref, reason_ref, evidence_refs_json,
created_at

workflow_timers:
tenant_id, workflow_id, timer_id, step_id, timer_type,
due_at, status, fired_at, created_at

workflow_compensations:
tenant_id, workflow_id, compensation_id, source_step_id,
target_service, target_operation, target_ref_hash, payload_schema_version,
payload_ref_hash, compensation_policy_ref, reason_ref, downstream_service,
downstream_request_ref, status, failure_class, public_error, created_at,
updated_at, completed_at

workflow_compensation_instructions:
tenant_id, instruction_id, workflow_id, payload_ref_hash, target_service,
target_operation, instruction_type, environment, config_kind, bundle_key,
target_version, operator_ref, reason_ref, status, created_at, updated_at
```

所有 reason / evidence / payload 都通过 ref 或 hash 保存；原文由 admin UI、artifact store
或 audit system 按独立 retention 管理。

## 9. 核心流程

Action approval：

```text
agent-service creates proposal
-> action-executor detects approval workflow required
-> CreateWorkflow(ACTION_APPROVAL)
-> notification-service notifies approver
-> RecordWorkflowDecision(APPROVE)
-> workflow emits workflow.step.ready.v1
-> action-executor rechecks agent approval / policy / skill / prepare audit
-> execute or fail closed
```

Repair approval：

```text
operator/admin creates repair request
-> CreateWorkflow(REPAIR_APPROVAL)
-> policy precheck + separation-of-duty
-> approver decision
-> workflow emits approved event
-> admin-service or service-specific operator executes repair with workflow_ref
-> workflow records result / compensation if needed
```

Timer / timeout：

```text
workflow_timers(due_at)
-> timer worker locks due timers
-> transition step/workflow TIMED_OUT or RETRYING
-> write low-sensitive event
```

## 10. 一致性和事务

强一致边界：

- workflow request / first steps / submitted outbox 同事务。
- decision / status transition / decision outbox 同事务。
- timer fire / step transition / timeout outbox 同事务。
- compensation request / failed workflow state / outbox 同事务。

最终一致边界：

- 下游服务执行通过公开 API / event 触发，必须自带 idempotency key。
- notification delivery 不阻塞 workflow 状态；失败进入 notification-service retry / DLQ。
- audit-service 通过 `im.workflow.events` 或 internal API 最终归档。

## 11. 权限、审批和隔离

- 所有 create / decision / advance 必须走 policy-service precheck。
- 高风险 workflow 必须 separation-of-duty：requester 不能作为唯一 approver。
- decider scope 必须覆盖 target tenant / target service / operation type。
- request body 不能覆盖 trusted tenant / decider metadata。
- `CRITICAL` risk workflow 不允许 inline auto-advance，必须等待人工审批或外部 approval proof。
- workflow 查询默认只返回 caller scope 内的 workflow；跨 tenant 查询禁止。

Approval rule 第一版：

```text
LOW: system auto approve allowed if policy permits
MEDIUM: one approver
HIGH: separation-of-duty
CRITICAL: admin-service / external approval proof required
```

## 12. 与其它服务关系

- `agent-service` 仍拥有 proposal 和 approval preflight；workflow-service 只等待或记录长审批过程。
- `action-executor` 仍拥有 execution audit 和 adapter result；workflow-service 不执行 tool。
- `admin-service` 仍是管理 API 入口；workflow-service 不接普通 admin UI 查询业务事实。
- `control-plane-service` 仍拥有 config version facts；workflow-service 只编排 rollout wait。
- `audit-service` 仍拥有长期归档和 hash-chain；workflow-service 只发布低敏 events。
- `notification-service` 负责通知 approver；workflow-service 不接 email/SMS/APNs provider。

## 13. 幂等、重试和补偿

| 场景 | 幂等键 | 重试策略 | 补偿 |
| --- | --- | --- | --- |
| CreateWorkflow | tenant + requester + idempotency_key | replay 返回 workflow | command hash 冲突 fail closed |
| RecordDecision | workflow + decider + decision key | replay 返回 decision | duplicate decision fail closed |
| Timer worker | timer_id | SKIP LOCKED + bounded retry | timeout transition |
| Advance step | workflow + step_id + target operation | downstream idempotency | compensation request |
| OutboxRelay | event_id | bounded retry + DLQ | outbox repair |

Compensation 不能自动执行高风险业务 mutation；它只能创建 `COMPENSATION_REQUEST` 或调用
明确 allowlisted 的低风险 public API。
第一阶段 `compensation-executor` 只允许 `CONFIG_ROLLBACK -> control-plane-service`
公开 API，且必须由 file 或 DB registry 提供显式 instruction；registry 只保存低敏
refs / version 字段，不保存 admin payload 原文。DB registry mode 还必须绑定具体
workflow id，避免同一 payload hash 的 instruction 被复用到其它补偿。

## 14. 安全边界

- API 只接受 verified service/admin metadata 或 mTLS identity。
- reason 原文、approval comment、operator shell 输出、repair command body 不入库。
- target refs 只保存 hash / stable ref；不保存 raw SQL、DSN、object key、token、secret。
- event / metrics / debug 输出不包含 tenant 明细、operator、reason、payload、EvidencePack、proposal 正文或 downstream response。
- workflow engine adapter 不允许持久化比本服务 DB 更多的敏感业务 payload。
- workflow cancellation 必须保留 audit trail，不能 hard delete 状态。

## 15. SLO 和指标

第一阶段不写生产 SLO，只保留本地 / 面试展示指标：

```text
workflow_request_total{workflow_type,status,risk_level}
workflow_decision_total{decision_type,risk_level}
workflow_step_total{step_type,status,failure_class}
workflow_timer_total{timer_type,status}
workflow_compensation_total{status,failure_class}
workflow_outbox_total{status}
```

metrics label 禁止使用 tenant_id、workflow_id、step_id、decision_id、operator_ref、
target_ref、reason_ref、trace_id 或 request_id。

## 16. 测试方案

| 测试 | 目标 |
| --- | --- |
| domain unit | workflow 状态机、decision rule、timer transition、compensation guard |
| app unit | policy deny、separation-of-duty、idempotency、timeout、cancel |
| PostgreSQL integration | request + steps + decision + timer + outbox 同事务 |
| event builder | 不输出 reason / payload / proposal / downstream body |
| worker test | timer due SKIP LOCKED、step advance retry、compensation request |
| smoke | CreateWorkflow -> Approve -> StepReady -> Complete fake target |

## 17. Runbook

运行模式：

```text
NEXUSIM_WORKFLOW_SERVICE_MODE=grpc
NEXUSIM_WORKFLOW_SERVICE_MODE=workflow-worker
NEXUSIM_WORKFLOW_SERVICE_MODE=timer-worker
NEXUSIM_WORKFLOW_SERVICE_MODE=compensation-worker
NEXUSIM_WORKFLOW_SERVICE_MODE=compensation-executor
NEXUSIM_WORKFLOW_SERVICE_MODE=compensation-instruction-import
NEXUSIM_WORKFLOW_SERVICE_MODE=outbox-relay
NEXUSIM_WORKFLOW_SERVICE_MODE=cleanup
```

operator：

```text
workflow-audit
workflow-decision-record
workflow-cancel
workflow-retry-step
workflow-compensation-audit
workflow-compensation-instruction-list
workflow-outbox-repair
```

第一版本地 workflow operator 入口：

```powershell
go run ./loadtest/workflow -mode get -workflow-id wf_123
go run ./loadtest/workflow -mode record-decision -workflow-id wf_123 -step-id wfs_1 -decision APPROVE -decider-ref operator:a
go run ./loadtest/workflow -mode list-compensation-instructions -workflow-id wf_123 -status ACTIVE
```

该 CLI 只通过 workflow-service 公开 gRPC get workflow、record decision 和查询
低敏 instruction refs / version / status，不读 PostgreSQL 私表，不输出 workflow
payload、instruction payload、reason 原文或 downstream response body。

## 18. 验收标准

进入编码前：

- 本 SDD v0.1 draft 被复核，无 P0/P1。
- `workflow-service` brief 指向本 SDD。
- 明确 workflow-service 不替代 Agent proposal、action execution、admin API 或 audit归档。

进入 first smoke 前：

- proto / migration / 六层 skeleton / cmd runtime 已落。
- `ACTION_APPROVAL`、`REPAIR_APPROVAL`、`ADMIN_OPERATION` 和
  `COMPENSATION_REQUEST` 物化最小 workflow tests 通过。
- 高风险 workflow 无审批时 fail closed。
- event、metrics、audit summary 不包含 reason 原文、payload 原文、EvidencePack、
  proposal 正文或 downstream response。
