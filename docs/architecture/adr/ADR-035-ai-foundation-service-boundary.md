# ADR-035: AI Foundation Service Boundary

## Status

Accepted

## Context

NexusIM 已有 9 个真实后端服务：

```text
api-gateway
identity-service
message-service
conversation-service
delivery-service
push-gateway
receipt-service
contacts-service
policy-service
```

当前阶段从必要收口转向 AI 大模型应用底座。相关能力包括搜索、群组记忆、检索、RAG、摘要、Agent、Skill、MCP 工具接入、动作执行和 AI eval。服务数量和中间件不能写死，但后续拆分必须有清晰数据所有权、伸缩需求、故障边界、安全边界或复杂度收益。

2025/2026 memory benchmark 和论文暴露出多人协作场景的关键风险：speaker attribution、group / audience scope、temporal update、superseded fact、profile overgeneralization、permission leak 和 abstention failure。因此 group memory 不能实现成普通摘要缓存或向量库。

## Decision

NexusIM AI 底座采用以下边界：

```text
IM facts
-> outbox / Kafka event plane
-> search-service / memory-service projections
-> retrieval-gateway / EvidencePack
-> rag-service / summary-service
-> agent-service proposal
-> policy / approval
-> skill-registry / mcp-gateway
-> action-executor
-> audit / eval
```

第一批服务按依赖顺序推进：

1. `search-service v0.1`
2. `memory-service v0.1`
3. `retrieval-gateway v0.1`
4. `rag-service` / `summary-service`
5. `agent-service`
6. `skill-registry` / `mcp-gateway`
7. `action-executor`
8. `ai-eval-service` 或先以 `tools/ai-eval` 形式落地

关键不变量：

- `search-service` 是搜索投影写入口，不调用 LLM，不直接读其他服务私有表。
- `memory-service` 管理结构化长期记忆，不作为消息事实源，不替代 policy。
- `retrieval-gateway` 是 AI 唯一读入口，负责 policy check、visibility filter、temporal version 和 EvidencePack。
- `rag-service` / `summary-service` 只能消费 EvidencePack。
- `agent-service` 第一阶段 read-only / proposal-only；高风险动作不能自动执行。
- `skill-registry` 只登记 skill / tool schema、版本、owner、risk tier 和策略引用，不执行工具。
- `mcp-gateway` 是工具调用安全边界，必须接入 policy-service `CheckToolAction`、rate limit、tenant isolation 和 audit。
- `action-executor` 只执行低风险 allowlist 或已审批动作，通过公开业务 API 写入，必须幂等和审计。

`memory-service` 的长期 memory 至少必须包含：

```text
source_refs
speaker_ids
audience_scope
scope_type / scope_id
valid_from / valid_to
supersedes / contradicts
confidence
status: PENDING / ACTIVE / SUPERSEDED / REJECTED
visibility_policy_version
```

无 source ref 的长期 memory 不能进入 ACTIVE；用户画像不能从单条群消息直接升级；群组共识、个人偏好、项目事实和过程性知识必须分类型治理。

## Multi Sub-Agent Development Rule

允许用多个 sub-agent 加快 AI 底座开发，但必须满足：

- 每个 sub-agent 有单一 owner scope：一个服务、一个文档集、一个测试面或一个只读审查问题。
- 不允许两个 sub-agent 同时修改同一 proto、migration、service brief、architecture 章节或同一代码文件。
- 主 agent 负责最终集成、冲突解决、检查、提交和关闭 stale sub-agent。
- sub-agent 的发现如形成新工作，必须写回 `docs/runbook/remaining-goals.md`。

## Consequences

好处：

- AI 能力沿着 search -> memory -> retrieval -> RAG / Agent 的依赖顺序推进，不会直接变成“大 AI 服务”。
- RAG / Agent 不能绕过 IM 权限、成员可见窗口、tombstone 和 audit。
- group memory 可支持多人、多群、多时间版本协作语义，而不是简单摘要或向量堆积。
- 多 sub-agent 开发可以提速，但不牺牲主线一致性。

代价：

- 前期需要先落 search / memory / retrieval 契约，RAG / Agent demo 会后置。
- Memory schema、EvidencePack 和 eval harness 需要更严格的版本和测试。
- 新服务会增加本地运行和门禁维护成本，必须通过 ADR 和切片证据逐步推进。

## Validation

进入每个后续阶段前至少验证：

- search: tombstone、visibility window、source refs。
- memory: source-backed candidate、supersedes、profile overgeneralization 防护、permission leak 防护。
- retrieval: EvidencePack、citation、ACL denial、temporal version。
- RAG: no-evidence abstention、citation coverage。
- Agent/tool: policy precheck、approval bypass prevention、idempotent execution、audit linkage。
