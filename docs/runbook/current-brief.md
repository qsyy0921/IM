# NexusIM Current Brief

本文件只做每轮入口摘要；长历史、smoke 证据和设计细节放到 SDD、service
brief、loadtest report、development-progress 或 archive。

## 当前主线

- 客户端 Web / PC 已达到演示 MVP；除阻塞演示的问题外，不继续追完整产品级客户端。
- 后端主线已切到 AI / Agent / RAG 演示路径和必要平台能力。
- 当前 active module：Agent action boundary / repair cases。

## 最近收口

- Agent demo path 已能演示 EvidencePack -> RAG -> Agent proposal -> approval
  -> action-executor -> conversation-service public API，覆盖 conversation note
  和 conversation profile mutation。
- action-executor 已覆盖 provider failure metrics、batch handoff、provider replay
  operator UI、admin / workflow handoff、review / readiness / invocation manifest、
  controlled redrive execution、redrive result manifest 和 external audit append
  operator path。
- workflow-service 已覆盖 provider replay queue、approval timeout、external
  approval binding、operator queues、external callback wait、external callback
  delivery plan、delivery status / redrive plan、external callback delivery
  persistent worker first path、external callback delivery redrive operator path、
  compensation review bundle / page 和 execution
  readiness / invocation manifest。
- group memory / retrieval / eval 持续保留 source refs、conversation scope、
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
- 一个 goal 必须是可感知功能模块；不要把小字段、小测试、小文档句子当目标。
- 不写隐藏 alternate path；不确定时 fail-closed，或显式 repair / retry / redrive。
- 文档只在阶段、公开能力、架构边界、新服务 / 中间件 / provider 变化时同步。

## 下一个方向

- 继续补 Agent action boundary / repair cases；正式生产级运维 UI、provider-grade
  长周期平台和 provider replay 批量审批 UI 仍后置。
