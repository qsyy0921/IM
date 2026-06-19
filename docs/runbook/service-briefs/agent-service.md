# agent-service

状态：foundation-active / proposal store + approval preflight.

定位：受控 Agent proposal 边界服务。只消费 retrieval-gateway 的 `EvidencePack`，
并通过 `mcp-gateway.PrepareToolCall` 触发 skill catalog / policy precheck /
prepare audit。第一版持久化低敏 proposal / approval 元数据，给
`action-executor` 提供 approved proposal preflight；仍不执行业务动作。

当前已落：

- `CreateAgentProposal` 先调用 mcp-gateway prepare；deny 时返回 `BLOCKED`
  且不检索证据；allow 后通过 retrieval 获取 EvidencePack。
- 默认本地 extractive provider 保留 citations / EvidencePack，
  `generated_by_llm=false`，并统一跑 citation verifier。
- `agent_proposals` migration / PostgreSQL repository 已落，保存 proposal
  文本、citation 元数据、skill / prepare / policy metadata 和 approval 状态，
  不保存完整 EvidencePack 正文。
- `ApproveAgentProposal` 可把 `PROPOSED` 转为 `APPROVED`；
  `VerifyApprovedAgentProposal` 供 `action-executor` 校验 approval / prepare
  audit / skill / tool / resource 匹配，不暴露私表。
- 旧 Agent adapter smoke 和新的 `retrieval-gateway -> agent-service ->
  mcp-gateway` adapter smoke 已通过。
- Agent execution eval adapter 已覆盖 proposal -> approve -> action-executor
  execution audit 的 first path，断言 approved proposal / approval / prepare
  audit 关联成立，且不执行外部 tool。

下一步：

- 与 `action-executor` 的 approved proposal handoff 已有 first path；后续补
  real tool adapter、approval operator / audit outbox 和真实 tool result。
- 外部 LLM adapter 后续仍必须走 ProposalProvider port 和 citation verifier。
