# NexusIM Service Briefs

本目录只放短状态索引。默认不要读全部服务文件；当前任务涉及哪个服务，就只读对应文件。

## 已进入真实链路的 9 个服务

[message](message-service.md) / [conversation](conversation-service.md) /
[delivery](delivery-service.md) / [push](push-gateway.md) /
[receipt](receipt-service.md) / [contacts](contacts-service.md) /
[identity](identity-service.md) / [policy](policy-service.md) /
[api-gateway](api-gateway.md)

## Foundation-active AI 服务

[search](search-service.md) / [memory](memory-service.md) /
[retrieval](retrieval-gateway.md) / [rag](rag-service.md) /
[summary](summary-service.md) / [agent](agent-service.md) /
[skill-registry](skill-registry.md) / [mcp](mcp-gateway.md) /
[action-executor](action-executor.md) / [ai-eval](ai-eval-service.md)

## Future platform / product services

[media](media-service.md) / [notification](notification-service.md) / [audit](audit-service.md) /
[admin](admin-service.md) / [control-plane](control-plane-service.md) / [presence](presence-service.md) /
[model-gateway](model-gateway.md) / [workflow](workflow-service.md) / [knowledge-ingestion](knowledge-ingestion-service.md) / [vector-index](vector-index-service.md)

## 当前推进规则

- 现有 9 个服务只做阻塞 AI 链路的必要收口。
- memory 必须按 group memory 设计：source refs、speaker / audience、validity、supersedes、confidence、review state。
- Agent 写动作必须先走 policy tool precheck，默认 `Proposal -> Approval -> Executor -> Audit`。
- sub-agent 可并行推进，但不能同时修改同一服务文件。

## 查询规则

- 当前任务入口：`docs/runbook/current-brief.md`；剩余目标：`docs/runbook/remaining-goals.md`。
- 服务状态读本目录相关 brief；服务设计读对应 `docs/sdd/<service>.md` 相关章节。
- smoke / 压测证据按关键词查 `docs/runbook/loadtest/<service>/`；历史长文档查 archive，不要全文读。
