# NexusIM Service Briefs

短状态索引。当前任务涉及哪个服务，就只读对应 brief；完整架构基线：
`docs/architecture/target-architecture-complete.md`。

## 已进入真实链路的 10 个运行服务

[message](message-service.md) / [conversation](conversation-service.md) / [delivery](delivery-service.md) /
[push](push-gateway.md) / [receipt](receipt-service.md) / [contacts](contacts-service.md) /
[identity](identity-service.md) / [policy](policy-service.md) / [api-gateway](api-gateway.md) /
[timeline](timeline-service.md)

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

## Distributed timeline / hot conversation planning

[timeline](timeline-service.md) is now part of the local core runtime as a
first-stage seq block allocator. It is deployable and observable for hot-group
loadtest topology, can allocate sequence blocks through its own PostgreSQL
state, and is now called by message-service when a conversation enters
`SEQUENCER_BLOCK`. The current write path requests one seq per message; block
cache, gap marker, epoch fencing and repair remain future hardening.

## Client platform

Client platform is tracked in [client platform](../client-platform.md), not in
backend service briefs.
