# NexusIM Current Goal

本文件只写当前可执行目标。完整架构见
`docs/architecture/target-architecture-complete.md`，未完成工作见
`docs/runbook/remaining-goals.md`，单服务事实见 `docs/runbook/service-briefs/`。

## Active Module

AI / Agent operations：action-executor provider failure metrics / batch redrive
operator handoff。

## 目标

- 将 action-executor 的 provider failure / DLQ / retry-pending 状态变成可观测、
  可筛选、可交接给 operator 的低敏控制面能力。
- batch redrive 只能生成 operator handoff / redrive plan，不能自动重放旧 raw input、
  旧 provider output 或旧审批。
- 真正 redrive 必须重新走 fresh proposal / approval / prepared audit，并继续校验
  skill / tool / resource / policy；失败时 fail-closed。
- metrics / report 只能输出聚合计数、状态、hash、reason class 和低敏 id，不暴露
  raw provider error、raw tool input / output、secret 或 PII。
- 不引入隐藏 fallback；provider、审批、权限、redrive source 或 tool policy 状态不确定时
  fail-closed，或走显式 operator handoff。
- Python AI Worker 仍只做 candidate algorithm / planner / eval，不拥有业务状态、
  权限、审批或持久化。

## 本轮完成条件

- 先读取并对齐 `action-executor`、`agent-service`、`skill-registry`、
  `mcp-gateway`、`policy-service`、`ai-eval-service` 的 service brief / SDD。
- 做 compact architecture analysis：owner、failure state、redrive contract、
  permission、approval、audit、metrics、eval gate 和是否需要新中间件。
- 实现一个可演示的 provider failure metrics / batch redrive handoff 模块，或补齐其
  阻塞缺口。
- 补 focused tests / eval cases，覆盖 retryable failure、DLQ failure、unsafe output、
  redrive plan、unapproved redrive、policy denied redrive 和 no raw payload boundary。
- Focused checks 通过；若跨 proto / migration / 安全边界再跑完整门禁。
- 必要文档同步后提交并推送到 GitHub。

## 非目标

- 不继续扩完整产品级客户端。
- 不做长压、生产 HA、Docker / 双机基础设施整理。
- 不把 AI Worker 变成业务事实源。
- 不新增服务或中间件，除非架构分析证明当前模块必须新增。
- 不一次性展开完整 operator UI / provider-grade eval 平台。
- 不做自动 provider replay；本模块只做低敏观测和 operator handoff first path。

## 后续优先级

1. 本模块完成后，继续深化 provider replay 的审批 / 审计 / operator UI。
2. 再深化 group memory / retrieval / eval 的多跳、时间版本、profile cases。
3. Product-active 服务按需推进，不抢占 AI / Agent 演示主线。
