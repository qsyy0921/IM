# NexusIM Current Goal

本文件只写当前可执行目标。完整架构见
`docs/architecture/target-architecture-complete.md`，未完成工作见
`docs/runbook/remaining-goals.md`，单服务事实见 `docs/runbook/service-briefs/`。

## Active Module

AI / Agent demo path：Agent proposal / approval / action execution demo path。

## 目标

- 让 `agent-service` 只基于 `EvidencePack` 生成 proposal，不直接读 message /
  memory / search 私有表，也不直接执行业务动作。
- proposal 必须保留 citations、risk level、policy metadata、version metadata 和
  low-sensitive audit trail；证据不足时拒绝生成。
- approval 是写动作前置边界：未审批 proposal 不能进入 `action-executor`。
- `action-executor` 只执行已批准、policy 允许、skill / tool / resource 匹配的动作，
  并记录 execution audit / result projection。
- Python AI Worker 只做 candidate algorithm / planner / eval，不拥有业务状态、
  权限、审批或持久化。
- 不引入隐藏 fallback；审批、权限、证据、tool policy 或 provider 状态不确定时
  fail-closed，或走显式 repair / redrive。

## 本轮完成条件

- 先读取并对齐 `agent-service`、`action-executor`、`skill-registry`、
  `mcp-gateway`、`retrieval-gateway`、`ai-eval-service` 的 service brief / SDD。
- 做 compact architecture analysis：owner、contract、permission、approval、audit、
  eval gate、Python / Go boundary 和是否需要新中间件。
- 实现一个可演示的 Agent proposal -> approval -> action execution 安全路径，
  或补齐其阻塞缺口。
- 补 focused tests / eval cases，覆盖 missing evidence、unapproved proposal、
  policy denied action、tool mismatch、unsafe output、redrive / repair 边界。
- Focused checks 通过；若跨 proto / migration / 安全边界再跑完整门禁。
- 必要文档同步后提交并推送到 GitHub。

## 非目标

- 不继续扩完整产品级客户端。
- 不做长压、生产 HA、Docker / 双机基础设施整理。
- 不把 AI Worker 变成业务事实源。
- 不新增服务或中间件，除非架构分析证明当前模块必须新增。
- 不一次性展开完整 operator UI / provider-grade eval 平台。

## 后续优先级

1. 本模块完成后，补 action-executor provider-grade batch redrive、provider replay、
   operator UI 和 metrics。
2. 再深化 group memory / retrieval / eval 的多跳、时间版本、profile cases。
3. Product-active 服务按需推进，不抢占 AI / Agent 演示主线。
