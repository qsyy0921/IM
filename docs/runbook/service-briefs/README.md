# NexusIM Service Briefs

本目录只放短状态索引。默认不要读全部服务文件；当前任务涉及哪个服务，就只读对应文件。

## 已进入真实链路的 9 个服务

- [message-service](message-service.md)
- [conversation-service](conversation-service.md)
- [delivery-service](delivery-service.md)
- [push-gateway](push-gateway.md)
- [receipt-service](receipt-service.md)
- [contacts-service](contacts-service.md)
- [identity-service](identity-service.md)
- [policy-service](policy-service.md)
- [api-gateway](api-gateway.md)

## 下一阶段 planned service brief

- [search-service](search-service.md)
- [memory-service](memory-service.md)
- [retrieval-gateway](retrieval-gateway.md)
- [rag-service](rag-service.md)

## 当前新增服务顺序

- 先把现有 9 个服务做必要收口，补齐 search / memory / retrieval / RAG / summary / Agent 依赖的事实、权限、事件和 tombstone 边界。
- `search-service` / `memory-service` / `retrieval-gateway` / `rag-service` 是 AI 底座第一组 foundation-active 服务。
- 短期不以生产级完整系统测试或生产级 HA 作为转进阻塞；后续顺序是 summary / Agent / skill-registry / MCP gateway / action-executor。
- `memory-service` 第一版 contracts 已开始落地，必须按 group memory 设计，而不是普通摘要缓存：source refs、speaker / audience、validity window、supersedes、confidence、review state 是基本字段。
- Agent / skill-registry / MCP gateway / action-executor 接真实业务动作时必须先走 policy-service tool policy precheck；写动作默认 `Proposal -> Approval -> Executor -> Audit`，低风险 allowlist 也必须可审计和幂等。
- 可以用 sub-agent 并行推进服务 brief / SDD / 测试缺口，但不同 agent 不能同时修改同一个服务文件。

## 查询规则

- 当前任务入口：`docs/runbook/current-brief.md`
- 剩余目标：`docs/runbook/remaining-goals.md`
- 服务当前状态：只读本目录下相关服务文件。
- 服务设计：读对应 `docs/sdd/<service>.md` 的相关章节。
- smoke / 压测证据：按关键词查 `docs/runbook/loadtest/<service>/`。
- 历史长文档：按关键词查 `docs/runbook/archive/`，不要全文读。
