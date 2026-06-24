# NexusIM Current Goal

本文件只写当前可执行目标。完整架构见
`docs/architecture/target-architecture-complete.md`，未完成工作见
`docs/runbook/remaining-goals.md`，单服务事实见 `docs/runbook/service-briefs/`。

## Active Module

AI / Agent operations：action-executor provider replay approval / audit operator UI
first path。

## 目标

- 在已有 provider failure metrics / batch redrive handoff 基础上，补 provider replay
  的审批、审计和 operator UI first path。
- provider replay 不能自动重放旧 raw input、旧 provider output 或旧审批；必须复用
  fresh proposal / approval / prepared audit 和新的 input / reason hash。
- operator UI / API 只能展示低敏状态、hash、reason class、batch id、candidate id 和
  workflow 状态，不暴露 raw provider error、raw tool input / output、secret 或 PII。
- replay / approval / audit 任一状态不确定时 fail-closed；不引入隐藏 fallback，
  不用默认成功、静默降级或本地缓存冒充 replay 成功。
- Python AI Worker 仍只做 candidate algorithm / planner / eval，不拥有业务状态、
  权限、审批或持久化。

## 本轮完成条件

- 先读取并对齐 `action-executor`、`agent-service`、`workflow-service`、
  `admin-service`、`audit-service`、`skill-registry`、`mcp-gateway`、`policy-service`、
  `ai-eval-service` 的 service brief / SDD。
- 做 compact architecture analysis：owner、replay state、operator workflow、
  approval / audit contract、permission、eval gate 和是否需要新中间件。
- 实现一个可演示的 provider replay approval / audit operator UI first path，或补齐其
  阻塞缺口。
- 补 focused tests / eval cases，覆盖 unapproved replay、stale approval、policy denied
  replay、source mismatch、no raw payload boundary 和 audit / result projection。
- Focused checks 通过；若跨 proto / migration / 安全边界再跑完整门禁。
- 必要文档同步后提交并推送到 GitHub。

## 非目标

- 不继续扩完整产品级客户端。
- 不做长压、生产 HA、Docker / 双机基础设施整理。
- 不把 AI Worker 变成业务事实源。
- 不新增服务或中间件，除非架构分析证明当前模块必须新增。
- 不一次性展开完整 provider-grade eval 平台、生产 UI 或自动 replay。

## 后续优先级

1. 本模块完成后，深化 group memory / retrieval / eval 的多跳、时间版本、profile cases。
2. Product-active 服务按需推进，不抢占 AI / Agent 演示主线。
