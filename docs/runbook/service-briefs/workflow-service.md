# workflow-service

状态：product-active / first path complete，已覆盖 approval workflow、first-stage
compensation request materialization 和 instruction ops visibility。

定位：长事务和审批工作流服务，负责 Agent approval wait、repair approval、
admin operation approval、补偿请求和人工审批状态。

边界：
- 不替代 agent-service proposal、action-executor 执行或 audit-service 归档。
- 业务服务热路径不得同步等待长工作流。
- 高风险动作必须保留 proposal / approval / executor / audit 链。
- Temporal 等工作流引擎只是候选，中间件不写死。

已覆盖：
- `CreateWorkflow` / `RecordWorkflowDecision` / `GetWorkflow`。
- `ACTION_APPROVAL`、`REPAIR_APPROVAL`、`ADMIN_OPERATION`、`COMPENSATION_REQUEST`。
- `compensation-worker`：approved compensation request -> `workflow_compensations`
  -> low-sensitive outbox -> `COMPENSATION_PENDING`。
- `compensation-executor`：显式 file / DB instruction 驱动 control-plane rollback；
  缺指令或 unsupported target fail closed，不读 admin-service 私表。
- `compensation-instruction-import`：导入 / replay 低敏 rollback instruction，
  并绑定具体 workflow / 校验 refs。
- `ListWorkflowCompensationInstructions`：按 workflow 查询低敏 instruction refs /
  version / status，供后续 operator UI 使用，不暴露 payload / reason 原文。
- `loadtest/workflow` operator CLI：通过 workflow-service 公开 gRPC get workflow、
  record decision、查询低敏 instruction metadata，作为 workflow ops / UI 的
  first-stage 本地入口；`record-decision` 本地拒绝明显敏感的 decider / policy /
  reason / evidence ref marker，并支持低敏 `decision-manifest` 作为第一版
  external approval binding。
- `write-workflow-decision-manifest.ps1` / `validate-workflow-decision-manifest.ps1`：
  生成和校验仓库外低敏 decision manifest，不保存审批 comment、EvidencePack 或
  payload 原文。
- `write-workflow-compensation-instruction-manifest.ps1` /
  `validate-workflow-compensation-instruction-manifest.ps1`：生成和校验仓库外
  低敏 control-plane rollback instruction JSON，只保存 workflow id、payload hash、
  config target、operator ref 和 reason hash，不保存 rollback payload、operator
  reason 原文或本机文件路径。
- `compensation-instruction-import` 已纳入 `repair-operators.catalog.json`，可被
  本地 approval request / decision / invocation 链路引用；导入只写 workflow-service
  自有 instruction metadata，不执行 downstream rollback。
- approved repair invocation 在执行 import 前会校验 instruction manifest，并且
  invocation summary 只记录 manifest hash / instruction count，不输出 manifest 路径、
  payload ref 文件正文或 operator reason 原文。
- `write-repair-approval-review-page.ps1` 可把 workflow import 的 plan / request /
  decision / invocation / audit bundle 渲染为低敏静态 HTML review page；页面只展示
  hash、path hash、env key、preflight 和 audit 摘要，不复制 payload / reason /
  evidence 原文或本机路径。
- 已被 admin-service 用于 repair / critical / compensation handoff。

后续：timer worker、更多 compensation adapter、instruction approval UI /
external approval binding、external callback wait、outbox relay、repair operators。
