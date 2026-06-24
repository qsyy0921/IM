# NexusIM Current Goal

本文件只写当前可执行目标。完整架构见
`docs/architecture/target-architecture-complete.md`，未完成工作见
`docs/runbook/remaining-goals.md`，单服务事实见 `docs/runbook/service-briefs/`。

## Active Module

AI / Agent eval：group memory / retrieval / eval multi-hop temporal profile cases。

## 当前已收口

- action-executor provider replay operator UI first path 已落：`provider-replay-operator-ui`
  只读 `DLQ` provider failure，输出低敏 batch / candidate / workflow state /
  permission gate / audit contract，不执行 tool、不修改 failure row、不复用旧 approval。
- `RedriveProviderFailure` 仍是唯一执行入口，必须使用 fresh proposal / approval /
  prepared audit、新 input 和 reason hash，并复用正常 action execution 链。
- eval catalog 已新增 provider replay operator UI first-path case。

## 目标

- 深化 group memory / retrieval / eval 的多跳、时间版本和 profile cases，让 AI / Agent
  演示链路能解释多人、多群、跨时间版本的协作记忆，而不是只做单条事实检索。
- Memory / retrieval / RAG / Agent 必须保留 source refs、conversation scope、
  speaker attribution、member visibility、validity window、supersession、profile
  review 和 citation boundary。
- Python AI Worker 仍只做 candidate extraction / planner / rerank / eval 候选；
  Go 服务继续拥有权限、状态、审批、审计和持久化。

## 本轮完成条件

- 先读取并对齐 `memory-service`、`search-service`、`retrieval-gateway`、`rag-service`、
  `summary-service`、`agent-service`、`ai-eval-service` 和 Python AI Worker brief / SDD。
- 做 compact architecture analysis：case owner、source chain、temporal version、
  profile boundary、visibility gate、eval adapter 和是否需要新中间件。
- 补一个可感知的 group-memory / retrieval / eval 功能包，而不是只加单条 case。
- Focused tests / eval cases 覆盖 multi-hop actor chain、temporal update、profile
  overgeneralization negative gate、retrieval miss / insufficient evidence 和 no raw prompt /
  no unsupported fallback。
- Focused checks 通过；若跨 proto / migration / 安全边界再跑完整门禁。
- 必要文档同步后提交并推送到 GitHub。

## 非目标

- 不继续扩完整产品级客户端。
- 不做长压、生产 HA、Docker / 双机基础设施整理。
- 不把 AI Worker 变成业务事实源。
- 不新增服务或中间件，除非架构分析证明当前模块必须新增。
- 不一次性展开完整 provider-grade eval 平台、生产 UI 或真实模型长评测。

## 后续优先级

1. group memory / retrieval / eval 功能包完成后，再按需推进正式 provider replay admin /
   workflow handoff。
2. Product-active 服务按需推进，不抢占 AI / Agent 演示主线。
