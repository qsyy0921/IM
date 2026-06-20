# workflow-service

状态：product-active / first path complete。
`services/workflow-service` 第一版实现已覆盖 service registry、proto、migration、
runtime、Docker 和 observability。

设计入口：`docs/sdd/workflow-service.md`。

Stage-switch 记录：`docs/runbook/stage-switch/workflow-service.md`。

定位：长事务和审批工作流服务，负责 Agent approval wait、repair approval、
retention、外部系统调用补偿和人工审批状态。

边界：

- 不替代 agent-service proposal、action-executor 执行或 audit-service 归档。
- 业务服务热路径不得同步等待长工作流。
- 高风险动作必须保留 proposal / approval / executor / audit 链。
- Temporal 等工作流引擎只是候选，中间件不写死。

第一切片范围：

- 已按 SDD 落 proto / migration / 六层 skeleton，并同步
  `docs/runbook/service-registry.json`。
- 第一版支持 action approval 和 repair approval 两类低敏 workflow。
- 已被 admin-service operation worker 用于第一版 `REPAIR_REQUEST ->
  REPAIR_APPROVAL` 长审批入口；该路径只保存低敏 hash/ref，并由 admin-service
  result 记录 `workflow:<workflow_id>`。
- 已通过 focused checks、真实 PostgreSQL integration 和完整 `check-local`。
- 确认 reason / payload / EvidencePack / proposal 正文不进入事件或 metrics。
- 第一版只做 `CreateWorkflow`、`RecordWorkflowDecision`、`GetWorkflow`；
  timer / compensation worker、external callback wait、outbox relay 和通用
  `ADMIN_OPERATION` workflow 类型后置。
