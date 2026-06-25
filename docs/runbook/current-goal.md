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
- workflow compensation review bundle 已落：`loadtest/workflow compensation-review-bundle`
  只读 `COMPENSATION_PENDING` workflow 和 `ACTIVE` instruction refs，校验 workflow id /
  payload hash / target 绑定后输出低敏审查包；不记录 decision、不创建 approval、
  不执行 compensation、不调用下游服务。
- workflow compensation review page 已落：`write-workflow-compensation-review-page.ps1`
  只接受低敏 compensation review bundle，重新校验 workflow / instruction 绑定后渲染
  仓库外 HTML；不记录 decision、不创建 approval、不执行 compensation、不暴露 raw
  payload / reason / path / provider body / EvidencePack。

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
