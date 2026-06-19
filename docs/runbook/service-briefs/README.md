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

## 当前新增服务顺序

- 现有 9 个服务只做阻塞 AI 链路的必要收口。
- 第一组 AI 服务为 search、memory、retrieval、RAG、summary、Agent、skill-registry、mcp-gateway、action-executor、ai-eval-service。
- memory 必须按 group memory 设计：source refs、speaker / audience、validity、supersedes、confidence、review state。
- Agent 写动作必须先走 policy tool precheck，默认 `Proposal -> Approval -> Executor -> Audit`。
- sub-agent 可并行推进，但不能同时修改同一服务文件。

## 查询规则

- 当前任务入口：`docs/runbook/current-brief.md`
- 剩余目标：`docs/runbook/remaining-goals.md`
- 服务当前状态：只读本目录下相关服务文件。
- 服务设计：读对应 `docs/sdd/<service>.md` 的相关章节。
- smoke / 压测证据：按关键词查 `docs/runbook/loadtest/<service>/`。
- 历史长文档：按关键词查 `docs/runbook/archive/`，不要全文读。
