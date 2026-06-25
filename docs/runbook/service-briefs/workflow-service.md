# workflow-service Brief

状态：product-active / long-running workflow first paths。

定位：长事务和审批工作流服务，负责 Agent / repair / admin approval、补偿请求、外部 callback 等低敏工作流状态。

## 已落

- `CreateWorkflow` / `RecordWorkflowDecision` / `GetWorkflow` / `ListWorkflows`。
- workflow types：`ACTION_APPROVAL`、`REPAIR_APPROVAL`、`ADMIN_OPERATION`、`COMPENSATION_REQUEST`。
- `timer-worker`：显式 approval timeout -> `TIMED_OUT`，不执行 action。
- compensation：request materialization、instruction import / list、control-plane rollback
  first path，unsupported target fail closed。
- external callback：wait workflow、decision binding、delivery plan、persistent delivery worker、status / redrive path、delivery review page / dashboard、batch redrive invocation / runner / result manifest / audit append handoff / audit append result manifest。
- operator visibility：provider-replay queue、operator queues、compensation review bundle / page、
  approval queue review / batch decision / audit append handoff、`ListWorkflowCompensations`。
- compensation execution artifacts：instruction approval page、readiness、invocation、execution result、audit append handoff 和 append result；只输出低敏 refs / hashes / runtime contract。
- `outbox-relay`：读取 workflow-service 自有 `workflow_outbox`，发布低敏 JSON envelope 到
  `im.workflow.events`，同一 workflow 的旧 `PENDING` / `DLQ` 事件会阻塞后续事件，避免
  repair / redrive 顺序被绕过。
- 已被 admin-service 用于 repair / critical / compensation handoff 和 provider replay workflow handoff。

## 边界

- 不替代 agent-service proposal、action-executor 执行或 audit-service 归档。
- 业务热路径不得同步等待长工作流；高风险动作必须保留 proposal / approval / executor / audit 链。
- Temporal 等工作流引擎只是候选，中间件不写死。

## 后续
- 更多 compensation adapter、provider-grade approval platform、callback persisted dashboard、workflow outbox relay smoke。
