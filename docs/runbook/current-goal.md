# NexusIM Current Goal

本文件只写当前可执行目标。完整架构见
`docs/architecture/target-architecture-complete.md`，未完成工作见
`docs/runbook/remaining-goals.md`，单服务事实见 `docs/runbook/service-briefs/`。

## Active Module

AI / Agent demo path：EvidencePack-driven RAG / Summary safety first path。

## 目标

- 以 `EvidencePack` 作为 RAG / Summary / Agent 读取业务事实的唯一输入契约。
- 明确 group memory 场景下的 source refs、conversation scope、member visibility、
  time/version boundary 和 citation requirements。
- 收紧 `retrieval-gateway`、`rag-service`、`summary-service`、`ai-eval-service` 之间的
  contract：不能绕过 EvidencePack 直接读 message / memory / search 私有表。
- 补 focused tests / eval cases，覆盖 missing evidence、visibility mismatch、stale /
  superseded evidence、unsafe no-citation answer 等 fail-closed cases。
- 保持 Python AI Worker 只做 candidate algorithm / eval，不拥有业务状态、权限或审计。
- 不引入隐藏 fallback；证据不足时拒答或返回稳定错误，不用假答案 / stale cache / 默认成功。

## 本轮完成条件

- 先读取并对齐 `retrieval-gateway`、`rag-service`、`summary-service`、`memory-service`、
  `search-service`、`ai-eval-service` 的 service brief / SDD。
- 做 compact architecture analysis：owner、contract、permission、audit、eval gate、
  Python / Go boundary 和是否需要新中间件。
- 实现一个可演示的 EvidencePack -> RAG / Summary safety path 或补齐其阻塞缺口。
- Focused checks 通过；若跨 proto / migration / 安全边界再跑完整门禁。
- 必要文档同步后提交并推送到 GitHub。

## 非目标

- 不继续扩完整产品级客户端。
- 不做长压、生产 HA、Docker / 双机基础设施整理。
- 不把 AI Worker 变成业务事实源。
- 不新增服务或中间件，除非架构分析证明当前模块必须新增。
- 不一次性展开完整 Agent workflow / operator UI / provider-grade eval 平台。

## 后续优先级

1. 本模块完成后，进入 Agent proposal / approval / action execution demo path。
2. 再补 action-executor provider-grade batch redrive、provider replay、operator UI 和 metrics。
3. Product-active 服务按需推进，不抢占 AI / Agent 演示主线。
