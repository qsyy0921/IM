# NexusIM Current Brief

本文件是低 token 阶段入口，只回答“现在在哪个阶段、下一步读哪里”。不要在这里维护
长历史、完整证据或全部待办。

## 按需读取

- 当前执行目标：`docs/runbook/current-goal.md`
- 剩余目标：`docs/runbook/remaining-goals.md`
- 单服务事实：`docs/runbook/service-briefs/README.md`，再读对应 service brief。
- 客户端细节：`docs/runbook/client-platform.md`
- 完整目标架构：`docs/architecture/target-architecture-complete.md`
- Fail-closed 规则：`docs/architecture/fail-closed-policy.md`
- 历史证据：按关键词查 `docs/runbook/loadtest/`、`docs/runbook/archive/`
  或 `docs/runbook/history/`。

## 当前阶段

```text
client platform MVP foundation
```

NexusIM 已有本地 / 双机可运行的最小分布式 IM 后端，并已扩展到 AI foundation
和 product-active 服务 first paths。当前用户已切入客户端平台，短线优先浏览器端
和 Windows PC 端；Android 后置到用户明确切回。

## 已有服务层级

- Core IM services：api-gateway、identity-service、message-service、
  conversation-service、delivery-service、push-gateway、receipt-service、
  contacts-service、policy-service。
- AI foundation：search-service、memory-service、retrieval-gateway、rag-service、
  summary-service、agent-service、skill-registry、mcp-gateway、action-executor、
  ai-eval-service。
- Product-active first paths：admin-service、audit-service、control-plane-service、
  knowledge-ingestion-service、media-service、model-gateway、notification-service、
  presence-service、vector-index-service、workflow-service。
- Client platform：Web / Windows PC / Android 共用 TypeScript protocol 和
  client-core；native shell 只做薄平台 bridge。

## 当前短线

1. 收口 Web / Windows PC 客户端：登录、好友、好友直聊、建群、消息列表、发送、
   PullInbox / ACK 和本机可运行包体验。
2. 所有客户端能力只走 api-gateway BFF 和 push-gateway，不直连内部服务。
3. 不引入隐藏 recovery；依赖、权限、事实源或投影不确定时 fail-closed、repair /
   retry，或回到对应事实源 recovery。

## 不变量

- PullInbox 是消息展示事实源，WebSocket 只是在线唤醒。
- RAG / summary / Agent 只能消费权限过滤后的 EvidencePack。
- 真实写动作必须走 policy、proposal / approval、executor 和 audit。
- Python AI Worker 只做候选算法和 eval；Go 拥有控制面、状态和审计。
- 新发现待完成工作写入 `docs/runbook/remaining-goals.md`。
