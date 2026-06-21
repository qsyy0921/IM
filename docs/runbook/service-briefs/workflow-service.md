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
- 第一版支持 action approval、repair approval 和 generic admin operation 三类低敏 workflow。
- 已支持 first-stage `COMPENSATION_REQUEST` workflow 类型，供 admin-service
  `compensation-request` operator 通过公开 gRPC 创建 / replay 补偿请求；当前只保存低敏
  target / payload / reason refs，不执行真实补偿 mutation。
- 已新增 first-stage `compensation-worker` runtime：claim 已批准的
  `COMPENSATION_REQUEST` workflow，写 `workflow_compensations`、
  `workflow.compensation.requested.v1` outbox，并把 workflow 推进到
  `COMPENSATION_PENDING`；真实 provider-grade 补偿执行仍后置。
- 已新增 first-stage `compensation-executor` runtime：显式配置
  `control-plane-rollback-file` 或 `control-plane-rollback-store` 后，可把
  `CONFIG_ROLLBACK` compensation 通过 control-plane-service 公开
  `RollbackConfigVersion` 执行；store 模式从 workflow-service 自有
  `workflow_compensation_instructions` 低敏 registry 解析 instruction。无指令 /
  不支持 target fail closed，不读取 admin-service 私有表。
- 已新增 first-stage `compensation-instruction-import` operator mode：从显式 JSON
  instruction file 导入 / replay control-plane rollback instruction 到 workflow DB；
  只保存 environment / config kind / bundle / target version / operator ref /
  reason ref 等低敏字段，不保存 admin payload 原文。
- 已被 admin-service operation worker 用于第一版 `REPAIR_REQUEST ->
  REPAIR_APPROVAL` 长审批入口；该路径只保存低敏 hash/ref，并由 admin-service
  result 记录 `workflow:<workflow_id>`。
- 已通过 focused checks、真实 PostgreSQL integration 和完整 `check-local`。
- 确认 reason / payload / EvidencePack / proposal 正文不进入事件或 metrics。
- 第一版只做 `CreateWorkflow`、`RecordWorkflowDecision`、`GetWorkflow` 和
  compensation request materialization / 显式 control-plane rollback compensation /
  instruction registry；timer worker、更多补偿 adapter、external callback wait 和
  outbox relay 后置。
