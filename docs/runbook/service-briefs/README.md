# NexusIM Service Briefs

短状态索引。当前任务涉及哪个服务，就只读对应 brief；完整架构基线：
`docs/architecture/target-architecture-complete.md`。

## 已进入真实链路的 9 个服务

[message](message-service.md) / [conversation](conversation-service.md) / [delivery](delivery-service.md) /
[push](push-gateway.md) / [receipt](receipt-service.md) / [contacts](contacts-service.md) /
[identity](identity-service.md) / [policy](policy-service.md) / [api-gateway](api-gateway.md)

## Foundation-active AI 服务

[search](search-service.md) / [memory](memory-service.md) / [retrieval](retrieval-gateway.md) /
[rag](rag-service.md) / [summary](summary-service.md) / [agent](agent-service.md) /
[skill-registry](skill-registry.md) / [mcp](mcp-gateway.md) / [action-executor](action-executor.md) / [ai-eval](ai-eval-service.md)

## Product-active platform / product services

[media](media-service.md) / [notification](notification-service.md) /
[audit](audit-service.md) / [admin](admin-service.md) /
[control-plane](control-plane-service.md) / [presence](presence-service.md) /
[model-gateway](model-gateway.md) /
[knowledge-ingestion](knowledge-ingestion-service.md) /
[workflow](workflow-service.md) / [vector-index](vector-index-service.md)

## Client platform

Client platform is tracked in [client platform](../client-platform.md), not in
backend service briefs.

## 当前推进规则

- 当前 active slice 是 client platform MVP foundation；client 细节看
  `../client-platform.md` 和 `../../sdd/client-platform.md`。
- 现有 9 个服务只做阻塞当前 client / AI 链路的必要收口。
- client platform 只能通过 `api-gateway` / `push-gateway` 使用后端能力。
- memory 必须保留 source refs、speaker / audience、validity、supersedes、confidence、review state。
- Agent 写动作必须走 policy precheck 和 `Proposal -> Approval -> Executor -> Audit`。
