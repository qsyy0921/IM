# NexusIM Current Goal

本文件只写当前可执行目标。完整架构见
`docs/architecture/target-architecture-complete.md`，未完成工作见
`docs/runbook/remaining-goals.md`，单服务事实见 `docs/runbook/service-briefs/`。

## Active Module

Agent action boundary / repair cases：继续补需要 proposal、approval、prepared audit、
workflow handoff、operator redrive 和最终执行归属证明的 Agent action / repair 场景。

## 当前已收口摘要

- action-executor 已覆盖 provider failure lifecycle、operator handoff / review /
  readiness / invocation、受控 redrive execution、result manifest、audit append handoff
  和 audit append result manifest；最终执行仍只在 action-executor。
- admin-service 已覆盖 provider replay request submit / list / approve / reject operator
  bridge，只创建和审批低敏 admin operation，不执行 redrive。
- workflow-service 已覆盖 provider replay queue、approval timeout、external approval
  binding、operator queues、external callback wait / delivery / redrive / review page /
  dashboard / batch redrive invocation / runner / result manifest、approval queue review page /
  batch decision manifest / runner / result review page / audit append handoff /
  audit append result manifest、
  compensation review / instruction approval page / readiness / invocation /
  result visibility / audit append handoff /
  audit append result manifest。
- ai-eval 已覆盖 group-memory / retrieval / RAG / Agent safety 和 action boundary
  本地低敏 gates。
- 详细能力和历史证据见 `docs/runbook/current-brief.md`、相关 service brief 和 SDD。

## 目标

- 继续补更多 Agent action boundary / repair cases。
- 保持 admin / workflow 只做请求、审批、状态和运维视图，不能直接执行工具、
  provider replay、compensation 或业务 mutation。
- 任何 repair / redrive 必须使用 fresh proposal / approval / prepared audit、新输入或低敏 refs，
  并经 final execution owner 的公开 API 执行。
- 不恢复 raw provider input / output，不修改 DLQ failure row 来伪造完成。

## 本轮完成条件

- 从 `remaining-goals.md` 选择下一个完整可感知功能模块。
- 先做 compact architecture analysis：owner、state machine、approval boundary、
  audit contract、event / API contract、是否需要新中间件。
- Focused tests / eval cases 覆盖 no direct execution、fresh approval、prepared audit、
  no raw payload、fact source immutable、final execution owner。
- Focused checks 通过；跨 proto / migration / 安全边界再跑完整门禁。
- 必要文档同步后提交并推送到 GitHub。

## 非目标

- 不继续扩完整产品级客户端。
- 不做长压、生产 HA、Docker / 双机基础设施整理。
- 不把 AI Worker 变成业务事实源。
- 不新增服务或中间件，除非架构分析证明当前模块必须新增。
- 不一次性展开 provider-grade 运维 UI、生产审批平台或真实模型长评测。

## 后续优先级

1. 按需推进更多 Agent action boundary / repair cases。
2. Product-active 服务按需推进，不抢占 AI / Agent 演示主线。
