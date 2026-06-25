# workflow-service Brief

状态：product-active / long-running workflow first paths。

定位：长事务和审批工作流服务，负责 Agent approval wait、repair approval、admin operation approval、
补偿请求、外部 callback 等低敏工作流状态。

## 已落

- `CreateWorkflow` / `RecordWorkflowDecision` / `GetWorkflow` / `ListWorkflows`。
- workflow types：`ACTION_APPROVAL`、`REPAIR_APPROVAL`、`ADMIN_OPERATION`、
  `COMPENSATION_REQUEST`。
- `timer-worker`：显式 approval timeout -> `TIMED_OUT`，不执行 action。
- compensation：request materialization、instruction import / list、control-plane rollback
  first path，unsupported target fail closed。
- external callback：wait workflow、decision binding、delivery plan、persistent delivery worker、status / redrive path。
- operator visibility：provider-replay queue、operator queues、compensation review bundle / page、
  `ListWorkflowCompensations` execution result query。
- compensation execution artifacts：readiness、invocation、execution result、audit append handoff 和 append result；
  只输出低敏 refs / hashes / runtime contract，不执行 compensation / decision / 下游调用。
- 已被 admin-service 用于 repair / critical / compensation handoff 和 provider replay workflow handoff。

## 边界

- 不替代 agent-service proposal、action-executor 执行或 audit-service 归档。
- 业务热路径不得同步等待长工作流；高风险动作必须保留 proposal / approval / executor / audit 链。
- Temporal 等工作流引擎只是候选，中间件不写死。

## 后续
- 更多 compensation adapter、provider-grade instruction / approval UI、callback UI、workflow outbox relay。
