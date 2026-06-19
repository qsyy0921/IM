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
- 可选 `python-worker` proposal provider mode：Go 先生成 grounded proposal，Python worker 只返回 proposal hash / citation metadata；最终 proposal、citation verifier、approval 和 audit 仍由 Go 控制。
- Agent adapter smoke、Agent -> mcp-gateway smoke 和 Agent execution eval adapter first path 已落。

## 边界

- 不执行真实工具或业务 mutation。
- 不直接读 message / conversation / search / memory / policy 私表。
- 外部 LLM / Python worker / MCP provider 后续必须走 ProposalProvider port、citation verifier、proposal / approval / executor / audit。

## 下一步

- external MCP / provider tool guarded adapter first path。
