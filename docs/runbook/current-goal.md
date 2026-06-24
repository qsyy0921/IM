# NexusIM Current Goal

本文件只写当前可执行目标。完整架构见
`docs/architecture/target-architecture-complete.md`，未完成工作见
`docs/runbook/remaining-goals.md`，单服务事实见 `docs/runbook/service-briefs/`。

## Active Module

action-executor / workflow：provider replay admin / workflow handoff。

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
  overgeneralization cases；覆盖 no unsupported memory fallback 和 no raw prompt persistence。

## 目标

- 把 provider failure replay 从本地 operator UI first path 推进到更接近正式运维的
  admin / workflow handoff：operator 可以创建低敏 replay request / workflow，审批后仍走
  `RedriveProviderFailure` 的 fresh proposal / approval / prepared audit / new input /
  reason hash 链。
- 不允许 admin / workflow 直接执行 provider replay，不允许复用旧 approval，不允许恢复 raw
  provider input / output，不允许修改 DLQ failure row 来伪造完成。
- action-executor 继续拥有最终执行边界；workflow / admin 只做请求、审批、状态和运维视图。

## 本轮完成条件

- 读取并对齐 `action-executor`、`workflow-service`、`admin-service`、`audit-service` 和
  `ai-eval-service` brief / SDD。
- 做 compact architecture analysis：owner、state machine、approval boundary、audit contract、
  event / API contract、是否需要新中间件。
- 补一个可感知的 provider replay admin / workflow handoff 功能包，而不是只加单条 case。
- Focused tests / eval cases 覆盖 request creation、fresh approval requirement、prepared audit、
  no direct execution、no raw provider payload、DLQ row immutable、redrive still goes through
  action-executor。
- Focused checks 通过；若跨 proto / migration / 安全边界再跑完整门禁。
- 必要文档同步后提交并推送到 GitHub。

## 非目标

- 不继续扩完整产品级客户端。
- 不做长压、生产 HA、Docker / 双机基础设施整理。
- 不把 AI Worker 变成业务事实源。
- 不新增服务或中间件，除非架构分析证明当前模块必须新增。
- 不一次性展开完整 provider-grade 运维 UI、生产审批平台或真实模型长评测。

## 后续优先级

1. provider replay admin / workflow handoff 完成后，再按需推进更多 Agent action boundary /
   repair cases。
2. Product-active 服务按需推进，不抢占 AI / Agent 演示主线。
