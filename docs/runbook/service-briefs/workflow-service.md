# workflow-service

状态：product-active / first path complete。

定位：长事务和审批工作流服务，负责 Agent approval wait、repair approval、
admin operation approval、补偿请求、外部 callback 等低敏工作流状态。

边界：

- 不替代 agent-service proposal、action-executor 执行或 audit-service 归档。
- 业务服务热路径不得同步等待长工作流。
- 高风险动作必须保留 proposal / approval / executor / audit 链。
- Temporal 等工作流引擎只是候选，中间件不写死。

当前能力：

- `CreateWorkflow` / `RecordWorkflowDecision` / `GetWorkflow` / `ListWorkflows`。
- `ACTION_APPROVAL`、`REPAIR_APPROVAL`、`ADMIN_OPERATION`、`COMPENSATION_REQUEST`。
- `timer-worker`：显式 approval timeout -> `TIMED_OUT`，不执行 action。
- compensation：request materialization、instruction import / list、file / DB instruction 驱动的
  `compensation-executor` control-plane rollback first path，unsupported target fail closed。
- external callback：wait workflow、decision manifest binding、delivery plan、persistent delivery worker、
  delivery status / redrive plan、redrive operator path；不记录 decision、不执行 target。
- operator visibility：provider-replay queue、operator queues、compensation review bundle / review page、
  `ListWorkflowCompensations` execution result query。
- compensation execution operator artifacts：execution readiness manifest 和 execution invocation manifest；
  只输出低敏 refs / hashes / runtime contract，不记录 decision、不执行 compensation、不调用 control-plane。
- compensation execution result manifest：绑定 execution invocation 和 workflow-service 公开
  compensation result summary；只输出 terminal status / downstream refs / public error，不执行
  compensation、不记录 decision、不调用下游服务。
- 已被 admin-service 用于 repair / critical / compensation handoff 和 provider replay workflow handoff。

后续：更多 compensation adapter、provider-grade instruction / approval UI、callback delivery provider-grade UI、
workflow outbox relay、repair operators。
