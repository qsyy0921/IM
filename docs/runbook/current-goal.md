# NexusIM Current Goal

本文件只写当前可执行目标。完整架构见
`docs/architecture/target-architecture-complete.md`，未完成工作见
`docs/runbook/remaining-goals.md`，单服务事实见 `docs/runbook/service-briefs/`。

## Active Module

Agent action boundary / repair cases：在 provider replay admin / workflow handoff
已落的基础上，继续补更完整的 Agent action approval / repair / redrive 边界。

## 当前已收口

- action-executor provider replay operator UI first path 已落：`provider-replay-operator-ui`
  只读 `DLQ` provider failure，输出低敏 batch / candidate / workflow state /
  permission gate / audit contract，不执行 tool、不修改 failure row、不复用旧 approval。
- `RedriveProviderFailure` 仍是唯一执行入口，必须使用 fresh proposal / approval /
  prepared audit、新 input 和 reason hash，并复用正常 action execution 链。
- eval catalog 已新增 provider replay operator UI first-path case。
- group-memory / retrieval / eval 功能包已扩展：profile-agent safety adapter 从 20 个
  active cases 增加到 24 个，新增 asker-bound term ambiguity、visible-chain incomplete
  abstention、missing visibility projection fail-closed、audience-language profile
  overgeneralization cases；覆盖 no unsupported memory hidden path 和 no raw prompt persistence。
- provider replay admin / workflow handoff 已落：`provider-replay-handoff` 只读
  `DLQ` provider failure，输出低敏 admin operation request 和 workflow handoff request；
  admin-service 已接受 `PROVIDER_REPLAY_REQUEST` 并强制路由 workflow-service
  `REPAIR_APPROVAL`，approval policy 指向 `admin.workflow.provider_replay.v1`，target
  service 为 `action-executor`。
- provider replay handoff review page 已落：`write-provider-replay-handoff-review-page.ps1`
  把低敏 handoff artifact 渲染成仓库外 HTML，并重新校验 contract、payload hash、
  workflow request、final execution owner、`direct_execution_allowed=false` 和
  `source_dlq_immutable=true`；页面不提交 admin operation、不创建 workflow、不记录 approval、
  不调用 `RedriveProviderFailure`、不暴露 raw provider artifacts。
- provider replay execution readiness page 已落：`write-provider-replay-readiness-page.ps1`
  将原 handoff、approved admin operation、workflow APPROVE manifest 和 fresh Agent proof
  绑定成最终 redrive 前的低敏 HTML；它只展示 refs / hashes，不执行 redrive、不修改 DLQ row、
  不包含 raw new input / reason / provider artifacts。
- provider replay redrive invocation manifest 已落：
  `write-provider-replay-redrive-invocation.ps1` 将原 handoff、approved admin operation、
  workflow APPROVE manifest 和 fresh Agent proof 绑定成低敏 `RedriveProviderFailure`
  command contract；它不执行 redrive、不修改 DLQ row、不包含 raw resource id / input /
  reason / provider artifacts，operator 必须在仓库外提供 raw 值并核验 hash。
- provider replay 受控 redrive execution operator path 已落：`loadtest/actionexecutor
  -mode provider-replay-redrive` 默认只做 preflight，校验低敏 invocation manifest、
  仓库外 raw resource id / new input / reason 的 hash，显式 `-execute` 才调用
  action-executor 公开 `RedriveProviderFailure` gRPC；输出只保留 refs / hashes /
  result metadata，不打印 raw resource id / input / reason。
- provider replay redrive result manifest 已落：
  `write-provider-replay-redrive-result-manifest.ps1` 读取低敏 invocation manifest 和
  `loadtest/actionexecutor -mode provider-replay-redrive -execute` 的低敏执行 summary，
  重新绑定 fresh proposal / approval / prepared audit、skill / tool / resource hash、
  result refs 和 status；它不执行 redrive、不追加 audit、不修改 DLQ row、不包含 raw
  resource id / input / reason / provider artifact。
- action-executor external audit append operator path 已落：`loadtest/actionexecutor
  -mode external-audit-append` 默认只做 preflight，校验仓库外低敏 audit append
  manifest、`attributes_json` hash、required checks、operator identity 和 raw provider
  artifact 禁入；显式 `-execute` 才调用 audit-service 公开 `AppendAuditRecord`
  gRPC；输出不打印 manifest path、raw attributes JSON、raw provider artifact 或
  credential-like 内容。
- provider replay admin operator bridge 已落：`loadtest/admin provider-replay-submit`
  校验 handoff contract / payload hash / 低敏 refs 后创建 `PROVIDER_REPLAY_REQUEST`；
  `provider-replay-list` / `provider-replay-approve` / `provider-replay-reject` 只做
  admin operation 列表和审批，不执行 redrive。
- workflow provider replay 队列视图已落：workflow-service `ListWorkflows` 可按
  workflow type / status / target service / target operation / approval policy 查询低敏
  workflow metadata；`loadtest/workflow provider-replay-queue` 默认列出等待
  `admin.workflow.provider_replay.v1` 审批的 action-executor
  `PROVIDER_REPLAY_REQUEST`，不执行 redrive、不修改 DLQ row、不暴露 raw payload。
- workflow approval timeout handling 已落：`timer-worker` 消费显式
  `workflow_timers(APPROVAL_TIMEOUT)` 到期事实，将仍在 `WAITING_DECISION` 的 workflow
  推进到 `TIMED_OUT` 并写低敏 `workflow.timed_out.v1` outbox；审批已终结时取消 pending
  timer；不执行 action、不创建隐式 approval、不根据命名 `timeout_policy_ref` 猜默认 due_at。
- workflow external approval binding 已落：`loadtest/workflow record-decision
  -decision-manifest` 使用仓库外低敏
  `nexusim.workflow.external_decision_manifest.v1` manifest，记录 decision 前先调用
  `GetWorkflow` 校验 workflow type、step、target、payload hash 和 approval policy；
  任何 mismatch 都 fail-closed，不调用 `RecordWorkflowDecision`，不执行 provider replay。
- workflow operator queues 已落：`loadtest/workflow operator-queues` 通过公开
  `ListWorkflows` 列出 action approval、repair approval、provider replay、admin operation、
  compensation request 和 compensation pending 的低敏 refs / counts；不记录 decision、
  不修改 workflow 状态、不执行 provider replay。
- workflow external callback wait 已落：`loadtest/workflow external-callback-wait` 创建显式
  `WAITING_DECISION` workflow，并输出低敏 external decision manifest template；外部系统仍需
  补全 explicit decision 后走 `record-decision -decision-manifest` 绑定校验，不执行 action。
- workflow external callback delivery plan 已落：
  `write-workflow-external-callback-delivery-plan.ps1` 将低敏 external decision manifest
  template 绑定到 endpoint ref、delivery queue ref 和 retry policy refs，输出仓库外
  `nexusim.workflow.external_callback_delivery_plan.v1`；它不调用外部 provider、不记录
  decision、不执行 target action，也不接受 raw callback URL / provider body。
- workflow external callback delivery status / redrive plan 已落：
  `write-workflow-external-callback-delivery-status.ps1` 记录 `DELIVERED` /
  `RETRY_PENDING` / `DLQ` 低敏 attempt status；
  `write-workflow-external-callback-redrive-plan.ps1` 只从 `RETRY_PENDING` 或 `DLQ`
  status 生成 redrive handoff；两者都绑定 delivery plan 和仍在 `WAITING_DECISION`
  的 workflow，不调用 provider、不记录 decision、不执行 target action。
- workflow external callback delivery persistent worker first path 已落：
  workflow-service 新增 `workflow_external_callback_deliveries` 持久 job 状态机和
  `external-callback-delivery-import` / `external-callback-delivery-worker` 运行模式；
  import 会锁定并校验仍处于 `WAITING_DECISION` 的 workflow 绑定，worker 只按 endpoint ref
  调用 runtime provider，更新 `PENDING` / `IN_FLIGHT` / `DELIVERED` /
  `RETRY_PENDING` / `DLQ`，并只写低敏 delivered / DLQ outbox；不记录 decision、
  不执行 target action、不保存 raw callback URL / provider body。
- workflow external callback delivery redrive operator path 已落：
  `external-callback-delivery-redrive` 读取低敏 redrive plan，重新锁定 workflow / delivery fact，
  只允许仍处于 `WAITING_DECISION` 的 `RETRY_PENDING` / `DLQ` delivery 重新入队为
  `PENDING` 并写 redriven outbox；不调用 provider、不记录 decision、不执行 target。
- workflow compensation review bundle 已落：`loadtest/workflow compensation-review-bundle`
  只读 `COMPENSATION_PENDING` workflow 和 `ACTIVE` instruction refs，校验 workflow id /
  payload hash / target 绑定后输出低敏审查包；不记录 decision、不创建 approval、
  不执行 compensation、不调用下游服务。
- workflow compensation review page 已落：`write-workflow-compensation-review-page.ps1`
  只接受低敏 compensation review bundle，重新校验 workflow / instruction 绑定后渲染
  仓库外 HTML；不记录 decision、不创建 approval、不执行 compensation、不暴露 raw
  payload / reason / path / provider body / EvidencePack。
- workflow compensation execution readiness 已落：`write-workflow-compensation-execution-readiness.ps1`
  只接受低敏 compensation review bundle，校验 `COMPENSATION_PENDING` workflow、
  `ACTIVE` instruction refs、payload hash、target 和 executor mode 后输出低敏
  readiness manifest；它只绑定 workflow-service `compensation-executor` 执行契约，
  不记录 decision、不创建 approval、不执行 compensation、不调用 control-plane 或其它下游服务。
- workflow compensation execution invocation manifest 已落：
  `write-workflow-compensation-execution-invocation.ps1` 只接受低敏 readiness manifest，
  重新校验 workflow / instruction / executor contract 后输出
  `nexusim.workflow.compensation_execution_invocation.v1`；它只给 operator 明确
  `workflow-service` `compensation-executor` runtime env / owner / preflight checks，
  不执行 compensation、不调用 control-plane、不记录 decision、不修改 workflow 或 compensation rows。

## 目标

- 在现有 Agent / RAG demo path 上继续补 action boundary / repair cases：更多需要
  proposal、approval、prepared audit、workflow handoff 和 action-executor final execution 的
  场景。
- 保持不变量：admin / workflow 不能直接执行工具或 provider replay；不能复用旧 approval；
  不能恢复 raw provider input / output；不能修改 DLQ failure row 来伪造完成。
- action-executor 继续拥有最终执行边界；workflow / admin 只做请求、审批、状态和运维视图。

## 本轮完成条件

- 从 `remaining-goals.md` 选择下一个完整可感知功能模块，优先 action boundary / repair /
  redrive 相关；不要把单个字段或单条文档作为 goal。
- 做 compact architecture analysis：owner、state machine、approval boundary、audit contract、
  event / API contract、是否需要新中间件。
- Focused tests / eval cases 覆盖 no direct execution、fresh approval、prepared audit、
  no raw payload、fact source immutable、final execution owner。
- Focused checks 通过；若跨 proto / migration / 安全边界再跑完整门禁。
- 必要文档同步后提交并推送到 GitHub。

## 非目标

- 不继续扩完整产品级客户端。
- 不做长压、生产 HA、Docker / 双机基础设施整理。
- 不把 AI Worker 变成业务事实源。
- 不新增服务或中间件，除非架构分析证明当前模块必须新增。
- 不一次性展开完整 provider-grade 运维 UI、生产审批平台或真实模型长评测。

## 后续优先级

1. 按需推进更多 Agent action boundary / repair cases。
2. Product-active 服务按需推进，不抢占 AI / Agent 演示主线。
