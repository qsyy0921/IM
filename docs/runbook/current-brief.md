# NexusIM Current Brief

本文件只做每轮入口摘要；长历史、smoke 证据和设计细节放到 SDD、service brief、
loadtest report 或 archive。

## 当前主线

- 客户端 Web / PC 已达到演示 MVP；除阻塞演示的问题外，不继续追完整产品级客户端。
- 后端主线已切到 AI / Agent / RAG 演示路径和必要平台能力。
- 当前 active module：action-executor provider failure metrics / batch redrive
  operator handoff。

## 当前模块事实

- 上一模块已收口：RAG / Summary 的生成用 text evidence 必须带 `source_ref`；
  search / memory evidence 必须带 `visibility_version`；memory / profile evidence
  必须满足 active / approved；citation verifier 不再用 item 顶层字段兜底。
- Agent demo path 已进入可演示闭环：EvidencePack -> RAG -> Agent proposal ->
  approval -> action-executor -> conversation-service public API，已覆盖
  `conversation.note.create` 和 `conversation.profile.update` 两类业务 mutation；
  eval catalog 已补 source-chain audit boundary 和 approved business mutation cases。
- 当前模块继续补 action-executor 的 provider failure 可观测与 batch redrive
  operator handoff：只输出低敏状态 / hash / reason class，不自动 replay 旧 input /
  provider output，真正 redrive 仍需 fresh proposal / approval / prepared audit。
- group memory / multi-party collaboration 必须继续保留 source refs、conversation scope、
  member visibility、time/version boundary、citations 和 no-citation refusal。

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

- 完成当前 action-executor provider failure metrics / batch redrive handoff 后，再补
  provider replay 的 operator UI / 审批 / 审计闭环。
