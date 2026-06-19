# agent-service SDD

## 目标

`agent-service` 是 NexusIM AI 应用底座中的受控 Agent proposal 边界。
第一版只创建只读 / proposal-only 的 `ActionProposal`，不执行真实业务动作。

它必须同时满足两个约束：

- 认知输入只能来自 `retrieval-gateway` 返回的权限过滤 `EvidencePack`。
- 工具动作必须先通过 `policy-service.CheckToolAction` 预检。

## 职责

- 对外提供 `CreateAgentProposal`。
- 调用 `policy-service.CheckToolAction` 做 tool action precheck。
- 调用 `retrieval-gateway.RetrieveEvidence` 获取 EvidencePack。
- 第一版通过 `ProposalProvider` port 生成 deterministic extractive proposal；
  默认实现不调用外部 LLM provider。
- response 必须保留 `tool_policy_decision`、`citations`、原始 `EvidencePack`、
  `agent_version` 和 `generated_by_llm=false`。
- policy deny 时返回 `BLOCKED`，并且不检索 EvidencePack。
- 无可见证据时返回 `INSUFFICIENT_EVIDENCE`，不能编造行动依据。
- provider 输出必须经过 citation verifier；引用无法匹配 EvidencePack 时
  fail closed。

## 非职责

- 不直接读 message / conversation / delivery / search / memory / policy 私有表。
- 不执行工具动作，不写业务事实，不发布 Kafka 事件。
- 不审批 proposal，不实现 executor，不做外部 MCP 调用。
- 不在第一版接外部 LLM adapter、tool runtime、prompt template registry 或缓存。

## 链路

```text
client / future API gateway
-> agent-service.CreateAgentProposal
-> policy-service.CheckToolAction
-> retrieval-gateway.RetrieveEvidence
-> EvidencePack
-> ProposalProvider
-> citation verifier
-> proposal response
```

后续 `skill-registry`、`mcp-gateway` 和 `action-executor` 必须沿用该边界。
Agent 不允许绕过 policy，也不允许直接执行 mutation。

## 安全边界

- `AuthContext` 优先使用 verified metadata，本地开发可用 request body。
- tool policy precheck 失败时 fail closed，返回稳定 public error。
- policy deny 时不读取 EvidencePack，避免未授权动作继续消耗检索链路。
- `CreateAgentProposal` 不返回没有 EvidencePack 支撑的事实。
- citations 必须可追踪到 evidence item 或 source ref。
- 第一版所有 proposal 都要求后续 approval / executor，不执行业务 mutation。

## 后续

- 引入持久 `ActionProposal` store、approval workflow 和 audit outbox。
- 接 `skill-registry` 读取可调用工具合约和权限元数据。
- 接 `mcp-gateway` / tool gateway 做外部工具隔离和租户级限流。
- 接 `action-executor` 执行 approved low-risk 动作，并记录幂等和审计。
- 外部 LLM adapter 接入时增加 prompt boundary、token budget、PII / secret
  filter、tool-call schema validation 和 provider failure fallback。
