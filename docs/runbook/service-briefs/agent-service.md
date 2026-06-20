# agent-service

状态：foundation-active / proposal store + approval outbox relay + planner Python candidate guard.

定位：受控 Agent proposal 边界。只消费 retrieval-gateway `EvidencePack`，
proposal 前调用 `mcp-gateway.PrepareToolCall` 做 skill / policy / prepare audit。

## 已落

- `CreateAgentProposal`：prepare deny 时 `BLOCKED` 且不检索 evidence；allow 后检索 EvidencePack。
- 默认 extractive provider：`generated_by_llm=false`，citations 经过 verifier。
- `agent_proposals`：保存低敏 proposal / approval / policy metadata，不保存完整 EvidencePack。
- `agent_approval_outbox`：approval 同事务写 `agent.proposal.approved.v1` 低敏事件，不保存 proposal 正文、objective、reason 或 EvidencePack 正文。
- `approval-outbox-relay`：发布低敏 `im.agent.events`，unsupported / malformed fail-closed。
- `ApproveAgentProposal` / `VerifyApprovedAgentProposal`：给 action-executor 校验 approval / prepare audit / skill / tool / resource，不暴露私表。
- `proposal-approval-audit` / `proposal-approval-approve`：默认 dry-run，reason 走文件，输出不含正文 / EvidencePack。
- 可选 `python-worker` proposal provider mode：Go 先生成 grounded proposal，Python worker 只返回 proposal hash / citation metadata；hash / citation mismatch 与 worker failure 已有 Agent output regression。
- Agent adapter smoke、Agent -> mcp-gateway smoke、Agent execution eval adapter first path 和 Agent output safety fixture eval 已落。
- `CreateAgentProposalRequest.at_conversation_seq` 已显式透传到 retrieval-gateway
  EvidencePack；prepare deny 仍不检索 EvidencePack。
- ai-eval 已补 Agent current-memory consumption CI-safe regression：`at_conversation_seq`
  必须传播，过期和 superseded memory 不得作为 current citation。

## 边界

- 不执行真实工具或业务 mutation。
- 不直接读 message / conversation / search / memory / policy 私表。
- 外部 LLM / Python worker / MCP provider 后续必须走 ProposalProvider port、citation verifier、proposal / approval / executor / audit。

## 下一步

- 补 current-memory service-stack live smoke，验证真实服务栈的 Agent proposal 不引用 stale memory；仍只提交 proposal。
