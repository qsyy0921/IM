# NexusIM Current Brief

本文件只做每轮入口摘要；长历史、smoke 证据和设计细节放到 SDD、service brief、
loadtest report 或 archive。

## 当前主线

- 客户端 Web / PC 已达到演示 MVP；除阻塞演示的问题外，不继续追完整产品级客户端。
- 后端主线已切到 AI / Agent / RAG 演示路径和必要平台能力。
- 当前 active module：`action-executor` provider failure redrive first path 收口。

## 当前模块事实

- `action-executor` 已有 approved execution audit、tool result projection、本地安全
  adapter、guarded external HTTP provider adapter、conversation note / profile opt-in
  business adapters、provider failure projection 和 bounded retry bookkeeping。
- 本轮新增目标是 `RedriveProviderFailure`：只能 redrive 已有 `DLQ` provider failure；
  必须使用 fresh proposal / approval / prepared audit、匹配 skill / tool / resource、
  新 `input_json` 和 `reason_sha256`。
- Redrive 复用 `ExecuteApprovedAction` 正常链路；execution audit 只保存 source
  provider failure id 和 reason hash，不恢复旧 raw input，不重放旧 provider output。

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

- 完成当前 redrive module 后，继续 AI / Agent demo path：EvidencePack、RAG /
  Summary safety、Agent proposal / approval / action execution、eval gate。
