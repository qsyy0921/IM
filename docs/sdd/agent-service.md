# agent-service SDD

## 目标

`agent-service` 是 NexusIM AI 应用底座中的受控 Agent proposal / approval
边界。第一版创建只读 / proposal-only 的 `ActionProposal`，持久化低敏
proposal 元数据，并提供 approval / execution preflight 校验；它仍不执行真实业务动作。

它必须同时满足两个约束：

- 认知输入只能来自 `retrieval-gateway` 返回的权限过滤 `EvidencePack`。
- 工具动作必须先通过 `mcp-gateway.PrepareToolCall`，由 mcp-gateway 统一完成
  skill catalog 校验、`policy-service.CheckToolAction` 预检和低敏 prepare audit。

## 职责

- 对外提供 `CreateAgentProposal`、`ApproveAgentProposal` 和
  `VerifyApprovedAgentProposal`。
- 调用 `mcp-gateway.PrepareToolCall` 做 tool action prepare / precheck。
- 调用 `retrieval-gateway.RetrieveEvidence` 获取 EvidencePack。
- 第一版通过 `ProposalProvider` port 生成 deterministic extractive proposal；
  默认实现不调用外部 LLM provider。
- response 必须保留 `tool_policy_decision`、`citations`、原始 `EvidencePack`、
  `skill_id`、`prepared_audit_id`、`agent_version` 和 `generated_by_llm=false`。
- mcp prepare deny 时返回 `BLOCKED`，并且不检索 EvidencePack。
- 无可见证据时返回 `INSUFFICIENT_EVIDENCE`，不能编造行动依据。
- provider 输出必须经过 citation verifier；引用无法匹配 EvidencePack 时
  fail closed。
- PostgreSQL `agent_proposals` 只保存 proposal / approval / execution
  preflight 所需字段、proposal 文本、citation 元数据和低敏 policy metadata；
  不保存完整 EvidencePack 正文。
- `VerifyApprovedAgentProposal` 是 `action-executor` 的公开校验边界：
  proposal 必须 `APPROVED`，且 `approval_id`、`prepared_audit_id`、skill、
  tool、resource 字段全部匹配。

## 非职责

- 不直接读 message / conversation / delivery / search / memory / policy 私有表。
- 不执行工具动作，不写业务事实，不发布 Kafka 事件。
- 不实现 executor，不直接调用外部 MCP provider。
- 不在第一版接外部 LLM adapter、tool runtime、prompt template registry 或缓存。

## 链路

```text
client / future API gateway
-> agent-service.CreateAgentProposal
-> mcp-gateway.PrepareToolCall
   -> skill-registry.GetSkill
   -> policy-service.CheckToolAction
   -> mcp_gateway_tool_call_audits
-> retrieval-gateway.RetrieveEvidence
-> EvidencePack
-> ProposalProvider
-> citation verifier
-> agent_proposals
-> proposal response
-> ApproveAgentProposal
-> VerifyApprovedAgentProposal
-> action-executor.ExecuteApprovedAction
```

`skill_id` 是兼容增量字段；旧调用方未传时，第一阶段默认使用
`tool_name` 作为 `skill_id`。后续 `action-executor` 必须使用
`prepared_audit_id` 关联 mcp-gateway prepare 记录。`action-executor` 必须
调用 `VerifyApprovedAgentProposal`，不能直接读取 `agent_proposals` 私表。
Agent 不允许绕过 mcp-gateway / policy，也不允许直接执行 mutation。

## 安全边界

- `AuthContext` 优先使用 verified metadata，本地开发可用 request body。
- tool prepare / policy precheck 失败时 fail closed，返回稳定 public error。
- prepare deny 时不读取 EvidencePack，避免未授权动作继续消耗检索链路。
- Agent 传给 mcp-gateway 的 `input_json` 只包含低敏 metadata，不包含 prompt /
  evidence 原文或业务 payload；mcp-gateway 只保存 input hash。
- `CreateAgentProposal` 不返回没有 EvidencePack 支撑的事实。
- citations 必须可追踪到 evidence item 或 source ref。
- approval 只把 proposal 从 `PROPOSED` 转为 `APPROVED`，不执行业务 mutation。
- executor preflight 失败必须 fail closed；不匹配的 approval / prepare audit /
  skill / tool / resource 不能进入执行边界。

## 后续

- 接 `action-executor` 的真实 MCP / tool adapter，并继续保持 proposal /
  approval / executor / audit 串接。
- 补 approval workflow 的 operator / audit outbox / external review UI。
- 外部 LLM adapter 接入时增加 prompt boundary、token budget、PII / secret
  filter、tool-call schema validation 和 provider failure fallback。
