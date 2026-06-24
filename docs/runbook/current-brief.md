# NexusIM Current Brief

本文件只做每轮入口摘要；长历史、smoke 证据和设计细节放到 SDD、service brief、
loadtest report 或 archive。

## 当前主线

- 客户端 Web / PC 已达到演示 MVP；除阻塞演示的问题外，不继续追完整产品级客户端。
- 后端主线已切到 AI / Agent / RAG 演示路径和必要平台能力。
- 当前 active module：EvidencePack-driven RAG / Summary safety first path。

## 当前模块事实

- 上一模块已收口并推送：`action-executor` 支持 controlled `RedriveProviderFailure`
  first path，要求 DLQ source、fresh proposal / approval / prepared audit、匹配
  skill / tool / resource、新 input 和 reason hash，并记录 redrive lineage。
- 当前模块要把 AI / Agent 读取事实的边界收紧到 EvidencePack：RAG、Summary 和后续
  Agent 只能基于可见、可引用、可审计的 evidence 工作。
- group memory / multi-party collaboration 必须保留 source refs、conversation scope、
  member visibility、time/version boundary 和 no-citation refusal。

## 已成型底座

- 9 个核心 IM 服务：api-gateway、identity-service、message-service、
  conversation-service、delivery-service、push-gateway、receipt-service、
  contacts-service、policy-service。
- AI foundation：search-service、memory-service、retrieval-gateway、rag-service、
  summary-service、agent-service、skill-registry、mcp-gateway、action-executor、
  ai-eval-service、Python AI Worker。
- Product-active first paths：admin-service、audit-service、control-plane-service、
  knowledge-ingestion-service、media-service、model-gateway、notification-service、
  presence-service、vector-index-service、workflow-service。

## 工作规则

- 每轮先读 `prompt.md`、`agent.md`、`docs/runbook/current-goal.md`。
- 只按需读取 SDD / service brief / ADR / report，不全文扫长历史文档。
- 一个 goal 必须是可感知功能模块；不要把小字段、小测试、小文档句子当目标。
- 不写隐藏 fallback；不确定时 fail-closed，或显式 repair / retry / redrive。
- 文档只在阶段、公开能力、架构边界、新服务 / 中间件 / provider 变化时同步。

## 下一个方向

- 完成当前 EvidencePack / RAG / Summary safety module 后，进入 Agent proposal /
  approval / action execution demo path，再补 provider-grade redrive / eval 平台能力。
