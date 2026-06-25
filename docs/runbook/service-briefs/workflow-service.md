# workflow-service

状态：product-active / first path complete，已覆盖 approval workflow、
compensation request materialization 和 instruction ops visibility。

定位：长事务和审批工作流服务，负责 Agent approval wait、repair approval、
admin operation approval、补偿请求和人工审批状态。

边界：

- 不替代 agent-service proposal、action-executor 执行或 audit-service 归档。
- 业务服务热路径不得同步等待长工作流。
- 高风险动作必须保留 proposal / approval / executor / audit 链。
- Temporal 等工作流引擎只是候选，中间件不写死。

- `CreateWorkflow` / `RecordWorkflowDecision` / `GetWorkflow`。
- `ACTION_APPROVAL`、`REPAIR_APPROVAL`、`ADMIN_OPERATION`、`COMPENSATION_REQUEST`。
- `compensation-worker`：approved compensation request -> `workflow_compensations`
  -> low-sensitive outbox -> `COMPENSATION_PENDING`。
- `compensation-executor`：显式 file / DB instruction 驱动 control-plane rollback，
  缺指令或 unsupported target fail closed。
- `compensation-instruction-import`：导入 / replay 低敏 rollback instruction 并绑定 workflow。
- `ListWorkflowCompensationInstructions`：按 workflow 查询低敏 instruction metadata。
- `loadtest/workflow` operator CLI：公开 gRPC get workflow、record decision、查询低敏 instruction metadata。
- `ListWorkflows`：按 type / status / target / approval policy 查询低敏 workflow metadata。
- `loadtest/workflow provider-replay-queue`：默认列出等待
  `admin.workflow.provider_replay.v1` 审批的 action-executor
  `PROVIDER_REPLAY_REQUEST` workflow；不执行 redrive、不修改 DLQ、不暴露 raw payload。
- manifest / review-page scripts：生成和校验仓库外低敏 decision、instruction 和 HTML review page。
- 已被 admin-service 用于 repair / critical / compensation handoff。
- 已被 admin-service 用于 provider replay handoff：`PROVIDER_REPLAY_REQUEST` 创建
  `REPAIR_APPROVAL` workflow，target service 为 `action-executor`；workflow-service 只记录
  低敏审批状态，不执行 provider replay。

后续：timer worker、更多 compensation adapter、instruction approval UI、
external approval binding、external callback wait、outbox relay、provider-grade approval UI、
repair operators。
