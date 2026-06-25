# workflow-service SDD v0.1 Draft

## 1. 服务定位

`workflow-service` 是 NexusIM 的长事务、人工审批等待、补偿和异步运维编排服务。
它承接不适合放在 IM 热路径里的审批、repair、retention、外部系统调用补偿和多步骤
operator workflow。

职责：

- 拥有 `workflow_requests`、`workflow_steps`、`workflow_decisions`、
  `workflow_timers`、`workflow_compensations`、`workflow_external_callback_deliveries`
  和 `workflow_outbox`。
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
| `WorkflowExternalCallbackDelivery` | 外部审批 callback delivery job | 只保存低敏 refs / hashes / retry 状态，不保存 raw URL / provider body |
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
rpc ListWorkflowCompensations(ListWorkflowCompensationsRequest) returns (ListWorkflowCompensationsResponse)
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

Provider replay handoff 使用既有 `REPAIR_APPROVAL` workflow type：

```text
admin-service PROVIDER_REPLAY_REQUEST
-> workflow-service CreateWorkflow(
     workflow_type=REPAIR_APPROVAL,
     target_service=action-executor,
     target_operation=PROVIDER_REPLAY_REQUEST,
     approval_policy_ref=admin.workflow.provider_replay.v1,
     payload_ref_hash=<admin operation payload hash>
   )
```

workflow-service 只记录审批状态和低敏 refs；它不执行 provider replay，也不调用
`RedriveProviderFailure`。workflow decision 完成后，后续仍需要 fresh Agent proposal /
approval / prepared audit / new input / reason hash，再由 action-executor 执行。

第一版 operator queue view 使用 `ListWorkflows` 暴露低敏 workflow metadata，可按
`workflow_type`、`status`、`target_service`、`target_operation` 和
`approval_policy_ref` 过滤。`loadtest/workflow -mode provider-replay-queue` 默认查询：

```text
workflow_type=REPAIR_APPROVAL
status=WAITING_DECISION
target_service=action-executor
target_operation=PROVIDER_REPLAY_REQUEST
approval_policy_ref=admin.workflow.provider_replay.v1
```

该视图只展示 workflow id、目标、policy、payload/ref hash、status、step 和时间戳等
低敏字段，不读取 action-executor provider failure raw payload，不修改 DLQ row，也不执行
`RedriveProviderFailure`。

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
payload 原文、reason 原文或 downstream body。`ListWorkflowCompensations` 提供按
workflow / status 查询 execution result 的低敏公开查询面，只返回 compensation id、
target refs、payload hash、downstream service / request ref、terminal status、failure
class 和 stable public error；它不读取下游服务私表，不暴露 payload / reason 原文，
也不调用下游服务。第一版外部审批 manifest binding 已由
operator CLI 负责当前 workflow 绑定校验；第一版 external callback wait 已由
`loadtest/workflow external-callback-wait` 创建低敏等待 workflow 和 decision manifest
template；`write-workflow-external-callback-delivery-plan.ps1` 把 template 绑定为低敏
callback provider / endpoint / queue / retry refs；
`write-workflow-external-callback-delivery-status.ps1` 记录 `DELIVERED` /
`RETRY_PENDING` / `DLQ` attempt status；
`write-workflow-external-callback-redrive-plan.ps1` 只从 retry / DLQ status 生成 redrive
handoff。`write-workflow-external-callback-delivery-review-page.ps1` 生成静态低敏
operator review page，重新校验 delivery plan、delivery status 和 redrive plan 的
hash / workflow binding / no-execution contract；页面只展示 refs、hashes、attempt、
failure class、redrive queue 和 owner，不调用 provider、不重新入队、不记录 decision、
不执行 target action，也不输出 raw callback URL、provider body、本地路径或 payload
正文。`write-workflow-external-callback-delivery-dashboard.ps1` 提供第一版本地批量
delivery triage dashboard：它读取仓库外一组 `external_callback_delivery_status` 和可选
`external_callback_redrive_plan` artifact，重新校验 status / redrive 绑定、workflow
仍为 `WAITING_DECISION`、redrive plan 只对应 retry / DLQ status，并输出状态计数、
redrive candidate 和低敏 refs / hashes。dashboard 不调用 provider、不记录 decision、
不执行 redrive、不执行 target action，也不输出本机路径、provider material、payload
material、model input 或 auth material。
`write-workflow-external-callback-batch-redrive-invocation.ps1` 提供第一版本地批量
redrive invocation manifest：它读取仓库外一组
`nexusim.workflow.external_callback_redrive_plan.v1` artifact，重新校验每个 redrive
plan 的 workflow binding、retry / DLQ source status、no-execution contract、
dedupe key 和 low-sensitive refs，并可绑定 dashboard hash 作为人工 review evidence。
manifest 只枚举 `external-callback-delivery-redrive` runtime contract 和每个 plan 的
hash / ref，不调用 workflow-service、不重新入队、不调用 provider、不记录 decision、
不执行 target action，也不输出本机路径、provider material、payload material、model
input 或 auth material。当前 first path 已新增
`workflow_external_callback_deliveries` 持久 job 和
`external-callback-delivery-import` / `external-callback-delivery-worker` /
`external-callback-delivery-redrive` 运行模式：
import 锁定 workflow-service 自有 workflow fact，校验仍为 `WAITING_DECISION` 且
workflow type / step / target / payload hash / approval policy 绑定一致；worker 只按
runtime endpoint ref 调用 provider，并推进
`PENDING -> IN_FLIGHT -> DELIVERED / RETRY_PENDING / DLQ`；redrive 读取低敏
`nexusim.workflow.external_callback_redrive_plan.v1`，重新锁定 workflow / delivery，
只允许 `RETRY_PENDING` 或 `DLQ` 重新入队为 `PENDING` 并写 low-sensitive redriven
outbox。上述入口不记录
decision、不执行 target action、不保存 raw callback URL / provider body。更多下游
adapter、provider-grade instruction / approval UI 和正式运维后置。

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
| `workflow.external_callback.redriven.v1` | `im.workflow.events` | `tenant_id:workflow_id` | external callback delivery 被 operator 显式重新入队 |
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
-> transition waiting workflow to TIMED_OUT
-> write low-sensitive event
```

当前实现只消费显式写入 `workflow_timers` 的 `APPROVAL_TIMEOUT` timer fact。
`timeout_policy_ref` 是低敏策略引用，不会被 workflow-service 猜测成默认 due_at；
命名策略要产生超时必须由对应创建方 / policy catalog 显式写入 timer。

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
| worker test | timer due SKIP LOCKED、workflow TIMED_OUT、compensation request |
| smoke | CreateWorkflow -> Approve -> StepReady -> Complete fake target |

## 17. Runbook

运行模式：

```text
NEXUSIM_WORKFLOW_SERVICE_MODE=grpc
NEXUSIM_WORKFLOW_SERVICE_MODE=timer-worker
NEXUSIM_WORKFLOW_SERVICE_MODE=compensation-worker
NEXUSIM_WORKFLOW_SERVICE_MODE=compensation-executor
NEXUSIM_WORKFLOW_SERVICE_MODE=compensation-instruction-import
NEXUSIM_WORKFLOW_SERVICE_MODE=external-callback-delivery-import
NEXUSIM_WORKFLOW_SERVICE_MODE=external-callback-delivery-redrive
NEXUSIM_WORKFLOW_SERVICE_MODE=external-callback-delivery-worker
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
.\tools\write-workflow-decision-manifest.ps1 -OutputPath H:\NexusIM\operator-plans\workflow-decision.json -WorkflowID wf_123 -StepID wfs_1 -Decision APPROVE -DeciderRef operator-a -ReasonFile H:\NexusIM\operator-plans\workflow-decision-reason.txt -EvidenceRef evidence:ticket-123
.\tools\validate-workflow-decision-manifest.ps1 -ManifestPath H:\NexusIM\operator-plans\workflow-decision.json -ExpectedWorkflowID wf_123 -ExpectedStepID wfs_1 -ExpectedDecision APPROVE
.\tools\write-workflow-compensation-instruction-manifest.ps1 -OutputPath H:\NexusIM\operator-plans\workflow-compensation-instruction.json -WorkflowID wf_123 -PayloadRefFile H:\NexusIM\operator-plans\rollback-payload-ref.txt -Environment local -ConfigKind API_GATEWAY_TENANT_QUOTA -BundleKey tenant-a -TargetVersion quota-v1 -OperatorRef operator:rollback -ReasonFile H:\NexusIM\operator-plans\rollback-reason.txt
.\tools\validate-workflow-compensation-instruction-manifest.ps1 -ManifestPath H:\NexusIM\operator-plans\workflow-compensation-instruction.json -ExpectedWorkflowID wf_123 -ExpectedTargetVersion quota-v1
go run ./loadtest/workflow -mode get -workflow-id wf_123
go run ./loadtest/workflow -mode provider-replay-queue
go run ./loadtest/workflow -mode list-workflows -workflow-type REPAIR_APPROVAL -status WAITING_DECISION -target-service action-executor -target-operation PROVIDER_REPLAY_REQUEST -approval-policy-ref admin.workflow.provider_replay.v1
go run ./loadtest/workflow -mode record-decision -workflow-id wf_123 -step-id wfs_1 -decision APPROVE -decider-ref operator:a
go run ./loadtest/workflow -mode record-decision -decision-manifest H:\NexusIM\operator-plans\workflow-decision.json
go run ./loadtest/workflow -mode operator-queues
.\tools\write-workflow-approval-queue-review-page.ps1 -QueueSummaryPath H:\NexusIM\operator-plans\workflow-operator-queues.json -GeneratedBy operator-a -OutputPath H:\NexusIM\operator-plans\workflow-approval-queue-review.html
.\tools\write-workflow-external-callback-delivery-dashboard.ps1 -DeliveryStatusRootPath H:\NexusIM\operator-plans\workflow-callback-statuses -RedrivePlanRootPath H:\NexusIM\operator-plans\workflow-callback-redrives -GeneratedBy operator-a -OutputPath H:\NexusIM\operator-plans\workflow-callback-delivery-dashboard.html
.\tools\write-workflow-external-callback-batch-redrive-invocation.ps1 -RedrivePlanRootPath H:\NexusIM\operator-plans\workflow-callback-redrives -DashboardPath H:\NexusIM\operator-plans\workflow-callback-delivery-dashboard.html -PreparedBy operator-a -OutputPath H:\NexusIM\operator-plans\workflow-callback-batch-redrive-invocation.json
go run ./loadtest/workflow -mode list-compensation-instructions -workflow-id wf_123 -status ACTIVE
go run ./loadtest/workflow -mode list-compensations -workflow-id wf_123 -status SUCCEEDED
go run ./loadtest/workflow -mode compensation-review-bundle -workflow-id wf_123
.\tools\write-workflow-compensation-review-page.ps1 -BundlePath H:\NexusIM\operator-plans\workflow-compensation-review-bundle.json -GeneratedBy operator-a -OutputPath H:\NexusIM\operator-plans\workflow-compensation-review.html
.\tools\write-workflow-compensation-execution-result-manifest.ps1 -InvocationPath H:\NexusIM\operator-plans\workflow-compensation-execution-invocation.json -CompensationSummaryPath H:\NexusIM\operator-plans\workflow-compensation-summary.json -GeneratedBy operator-a -OutputPath H:\NexusIM\operator-plans\workflow-compensation-execution-result.json
.\tools\write-workflow-compensation-execution-audit-append-manifest.ps1 -ResultManifestPath H:\NexusIM\operator-plans\workflow-compensation-execution-result.json -GeneratedBy operator-a -TenantID tenant_123 -OutputPath H:\NexusIM\operator-plans\workflow-compensation-audit-append.json
```

external callback delivery worker 运行依赖：

```text
NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_DELIVERY_TENANT_ID
NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_DELIVERY_PLAN_FILE
NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_PLAN_FILE
NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_ENDPOINTS_FILE
```

`NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_ENDPOINTS_FILE` 只存在运行时，用 endpoint ref
映射 URL；URL 不进入 PostgreSQL、outbox、报告或 delivery plan。
`external-callback-delivery-redrive` 不需要 endpoint map，只读取
`NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_PLAN_FILE` 指向的低敏 redrive plan，
重新校验 workflow / delivery fact 后把 delivery 重新入队；它不调用 provider、不记录
decision、不执行 target action。

该 CLI 只通过 workflow-service 公开 gRPC get workflow、list workflows、record decision
和查询低敏 instruction refs / version / status，不读 PostgreSQL 私表，不输出 workflow
payload、instruction payload、reason 原文或 downstream response body。`record-decision`
模式会在本机拒绝看起来像 secret / token / password / raw body / DSN 的
`decider-ref`、`decision-policy-ref`、`reason-ref` 或 `evidence-refs`，避免
operator 把敏感原文送入 gRPC 请求。`-decision-manifest` 是第一版 external
approval binding：manifest 只允许
`nexusim.workflow.external_decision_manifest.v1`，除 workflow id、step id、
decision、低敏 reason/evidence refs、idempotency key 和 correlation refs 外，还必须
绑定 expected workflow type、status、target service / operation、target ref hash、
payload schema version、payload ref hash 和 approval policy ref。record decision 前 CLI
必须先调用 workflow-service `GetWorkflow` 校验这些字段，任何 mismatch 都 fail-closed，
且不调用 `RecordWorkflowDecision`。manifest 不保存审批 comment、payload 原文或 provider
body。writer / validator 只处理仓库外低敏 JSON artifact，不读取数据库。

`operator-queues` 是第一版多队列 operator visibility：它复用 `ListWorkflows`，按固定
低敏队列列出 action approval、repair approval、provider replay、admin operation、
compensation request 和 compensation pending workflow refs / hash / counts。它不记录
decision、不修改 workflow 状态、不调用 action-executor，也不执行 provider replay；队列
结果只作为人工审批和 repair triage 入口。

`write-workflow-approval-queue-review-page.ps1` 提供第一版本地 approval queue review
HTML：它只接受 `loadtest/workflow -mode operator-queues` 或
`-mode provider-replay-queue` 的低敏 summary，重新校验 queue / workflow 绑定、
`WAITING_DECISION` 状态、approval policy、target service / operation、workflow count 和
no-decision contract，再渲染 workflow refs / hashes / step id / reason ref。页面不创建
approval、不记录 decision、不调用 action-executor、不执行 compensation、不 redrive provider
work，也不输出 raw payload、provider body、本机路径或 credential-like 字段。真正审批仍必须
通过 `workflow-service.RecordWorkflowDecision` 和 decision manifest 绑定校验完成。

`external-callback-wait` 是第一版外部回调等待入口：它通过 workflow-service
`CreateWorkflow` 创建一个显式 `WAITING_DECISION` workflow，并输出
`nexusim.workflow.external_decision_manifest.v1` 低敏 template。template 只携带
workflow id、step id、target / payload hash、approval policy 和 correlation refs；外部系统
仍必须填入 explicit decision / decider / reason / evidence 后，再通过
`record-decision -decision-manifest` 进行绑定校验。该模式不记录 decision、不调用
action-executor、不执行 target operation，也不把外部回调当成最终执行证明。

`write-workflow-external-callback-delivery-plan.ps1` 是第一版外部回调交付计划：
它只接受仓库外低敏 `nexusim.workflow.external_decision_manifest.v1` template，
要求 workflow 仍绑定 `WAITING_DECISION`，且 decision / decider 仍为空，然后输出
`nexusim.workflow.external_callback_delivery_plan.v1`。plan 只保存 workflow / target /
payload / approval policy 绑定、decision manifest hash、callback provider ref、endpoint
ref、delivery queue ref 和显式 retry / backoff / timeout policy refs。它不接受 raw
callback URL，不保存 provider body，不调用外部 provider，不记录 decision，不执行 target
action。外部系统完成审批后仍必须产出 explicit decision manifest，并通过
`record-decision -decision-manifest` 再次绑定校验。

`compensation-review-bundle` 是第一版 compensation instruction 审查包入口：它先调用
`GetWorkflow`，默认只接受 `COMPENSATION_REQUEST / COMPENSATION_PENDING` workflow，
再调用 `ListWorkflowCompensationInstructions(status=ACTIVE)` 查询低敏 instruction refs。
CLI 会校验 workflow id、payload ref hash、target service 和 target operation 绑定一致；
任何 mismatch 都 fail closed，不输出审查包。审查包只包含 workflow / instruction refs、
hash、version 和审查边界，不记录 decision、不创建 approval、不修改 instruction 状态、
不调用 compensation-executor、control-plane-service 或 action-executor。正式 provider-grade
instruction approval UI 仍是后续项。
`write-workflow-compensation-review-page.ps1` 提供第一版本地 compensation review HTML：
它只接受 `nexusim.workflow.compensation_review_bundle.v1`，重新校验 workflow type /
status、payload hash、target 和 ACTIVE instruction 绑定，再渲染低敏 refs / hash /
policy / status / boundary。页面不记录 decision、不创建 approval、不执行 compensation、
不调用下游服务，也不嵌入原始 payload、operator reason、provider body、EvidencePack、
本机路径或 credential-like 字段。

`write-workflow-compensation-execution-readiness.ps1` 提供第一版本地 compensation
execution readiness manifest：它只接受低敏 compensation review bundle，校验
`COMPENSATION_PENDING` workflow、`ACTIVE` instruction refs、payload hash、target、
control-plane rollback instruction type 和显式 executor mode，然后输出
`nexusim.workflow.compensation_execution_readiness.v1`。该 manifest 只绑定
workflow-service `compensation-executor` 的最终执行契约，不记录 decision、不创建或复用
approval、不执行 compensation、不调用 control-plane-service / action-executor，也不嵌入原始
payload、operator reason、provider body、EvidencePack、本机路径或 credential-like 字段。

`write-workflow-compensation-execution-invocation.ps1` 提供 readiness 之后、真正启动
`workflow-service` `compensation-executor` 之前的低敏 invocation manifest：它只接受
`nexusim.workflow.compensation_execution_readiness.v1`，重新校验 workflow 仍是
`COMPENSATION_PENDING`、instruction refs 仍为 `ACTIVE`、target 为
control-plane `CONFIG_ROLLBACK`，且 executor owner / mode / final execution owner 都绑定到
workflow-service。输出 `nexusim.workflow.compensation_execution_invocation.v1` 只包含
runtime env 名称、owner、hash 和 required checks；该 manifest 不记录 decision、不执行
compensation、不调用 control-plane-service、不修改 workflow / compensation rows，也不包含原始
payload、operator reason、provider artifact、EvidencePack、本机路径或凭证。

`loadtest/workflow -mode list-compensations` 是第一版 compensation execution result
visibility：它只调用 workflow-service 公开 `ListWorkflowCompensations`，按 workflow id
和可选 status 输出低敏 compensation refs / terminal result，不记录 decision、不执行
compensation、不调用下游服务、不读取 PostgreSQL 私表。
`write-workflow-compensation-execution-result-manifest.ps1` 将低敏 invocation manifest
与 `list-compensations` summary 绑定为
`nexusim.workflow.compensation_execution_result.v1`，要求 compensation row 与 invocation
的 workflow / payload / target refs 一致，且 status 为 `SUCCEEDED` 或 `FAILED`。该
manifest 只用于 operator 结果归档和后续 audit / repair handoff，不修改 workflow /
compensation rows，也不保存 raw payload、operator reason、provider body、本机路径或凭证。

`write-workflow-compensation-execution-audit-append-manifest.ps1` 将低敏 execution result
manifest 派生为 `nexusim.audit.external_append.v1`，供外部 audit append operator 通过
audit-service 公开 `AppendAuditRecord` 追加审计。该脚本必须显式传入 tenant id，只输出
workflow id、compensation id、payload/ref hash、downstream ref、terminal status 和
attributes hash；它不调用 audit-service、不执行 compensation、不记录 workflow decision、
不调用下游服务、不修改 workflow / compensation rows，也不保存 raw payload、operator
reason、provider body、本机路径或凭证。

`write-workflow-compensation-execution-audit-append-result-manifest.ps1` 读取低敏
audit append manifest 和 `loadtest/actionexecutor -mode external-audit-append -execute`
summary，重新绑定 audit-service 返回的 audit id、record hash、previous record hash 和
idempotency key，输出 `nexusim.workflow.compensation_audit_append_result.v1`。该 manifest
只证明外部 audit append execution summary 与 workflow compensation audit handoff
一致；它不调用 audit-service、不执行 compensation、不记录 workflow decision、不调用下游服务、
不修改 workflow / compensation rows，也不保存 raw payload、raw attributes JSON、provider
body、本机路径或凭证。

`write-workflow-compensation-instruction-manifest.ps1` /
`validate-workflow-compensation-instruction-manifest.ps1` 是第一版 compensation
instruction handoff：它复用 runtime 的 `instructions` JSON 形状，生成 / 校验
control-plane rollback instruction 所需的 workflow id、payload ref hash、
environment、config kind、bundle key、target version、operator ref 和 reason ref。
payload / reason 可从仓库外文件计算 hash，但 manifest 不复制 payload、reason 原文
或本机路径。validator 只做 schema / 低敏字段校验，不调用 workflow-service、
control-plane-service 或数据库。

`compensation-instruction-import` 已纳入机器可读 `repair-operators.catalog.json`，
因此可复用本地 repair approval request / decision / invocation 链路生成低敏执行
计划。该 operator 只导入 workflow-service 自有 instruction metadata，不直接执行
control-plane rollback mutation。
approved repair invocation 会在执行 import 前重新调用 instruction manifest validator；
summary 只保存 manifest hash、path hash 和 instruction count，不输出 manifest 路径、
payload ref 文件正文或 operator reason 原文。
`write-repair-approval-review-page.ps1` 提供第一版本地静态 operator review page：
它复用 approval-chain validator，并从 plan / request / decision / invocation /
audit bundle 渲染低敏 HTML，只展示 service / mode / approval refs、artifact hash、
path hash、env key、preflight hash 和 audit 摘要；不复制环境变量值、operator
reason 原文、manifest 路径、payload ref 文件正文、业务数据或 evidence 原文，也不执行
operator。该页面是 first-stage operator UI artifact，不是正式审批系统。
`write-workflow-compensation-instruction-approval-page.ps1` 是 workflow compensation
instruction import 的专用 approval page：它读取 instruction manifest、repair approval
request / decision 和可选 invocation summary，重新校验 plan mode、manifest path binding、
manifest hash 和 invocation preflight hash，只渲染 tenant / workflow / instruction refs、
approval refs、reason hash 和 artifact hash。该页面不创建 approval、不记录 decision、
不导入 instruction、不执行 compensation、不调用 workflow-service / control-plane-service，
也不输出 raw payload、reason text、本机路径、provider body、credential 或 evidence body。

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
