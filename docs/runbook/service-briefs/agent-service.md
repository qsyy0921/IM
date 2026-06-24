# agent-service

状态：foundation-active / proposal + approval + cross-group temporal stack smoke passed.

定位：受控 Agent proposal 边界。只消费 retrieval-gateway `EvidencePack`，
proposal 前调用 `mcp-gateway.PrepareToolCall` 做 skill / policy / prepare audit。

## 已落

- `CreateAgentProposal`：prepare deny 时 `BLOCKED` 且不检索 evidence；allow 后检索 EvidencePack。
- 默认 extractive provider：`generated_by_llm=false`，citations 经过 verifier。
- `agent_proposals`：保存低敏 proposal / approval / policy metadata，不保存完整 EvidencePack。
- `agent_approval_outbox`：approval 同事务写低敏 `agent.proposal.approved.v1`，不保存正文 / EvidencePack。
- `approval-outbox-relay`：发布低敏 `im.agent.events`，unsupported / malformed fail-closed。
- `ApproveAgentProposal` / `VerifyApprovedAgentProposal`：给 action-executor 校验 approval / prepare audit / skill / tool / resource，不暴露私表。
- `proposal-approval-audit` / approve operator 默认 dry-run，reason 走文件，输出不含正文 / EvidencePack。
- 可选 `python-worker` proposal provider mode：Go 先生成 grounded proposal，Python worker 只返回 proposal hash / citation metadata；hash / citation mismatch 与 worker failure 已有 Agent output regression。
- Agent adapter smoke、Agent -> mcp-gateway smoke、Agent execution eval adapter first path 和 Agent output safety fixture eval 已落。
- `at_conversation_seq` 已透传到 EvidencePack；CI-safe regression 和 2026-06-20 live smoke 均验证 proposal 不引用 stale memory。
- 2026-06-20 cross-group / temporal stack smoke 已验证 proposal path 保留跨群 source refs / speaker attribution，并排除 stale / future memory。
- 2026-06-23 Agent live adapter 已增加 multi-hop actor/source-chain completeness
  断言；仍只提交 proposal，不直接执行 tool / business mutation，不绕过
  mcp-gateway、policy、approval 或 audit。
- 2026-06-24 Agent EvidencePack graph edge 透传已落：retrieval client 会保留
  `EvidenceMemoryGraphEdge`，gRPC response 会继续向 action / UI 调用方返回该字段；
  `loadtest/agent` 会断言跨群 source refs 与 `SUPPORTS` memory graph edge 被保留，
  proposal 仍只基于 EvidencePack 和 citation verifier。
- 2026-06-24 Agent EvidencePack profile evidence 透传已落：retrieval client 会保留
  `PROFILE_AGGREGATE` evidence 的 profile subject、aggregate type/key、
  supporting memory ids 和时间字段；`loadtest/agent` 会断言 profile aggregate
  evidence 被保留。Agent 仍只提交 proposal，不直接读 memory-service 私表。
- 2026-06-24 `loadtest/ragagent` 已提供 RAG-Agent demo first path：复用
  `loadtest/agent` 的 proposal / approval / action-executor audit 校验，并与
  RAG grounded answer 校验组合成同一 tenant / conversation 的低敏总报告；Agent
  仍不直接执行工具或业务 mutation。
- 2026-06-24 `loadtest/ragagent` / `rag-agent-demo` adapter 已把 memory-service
  公开 candidate review 纳入 Agent EvidencePack 断言链路：候选必须经
  `SubmitMemoryCandidate` -> `ReviewMemoryCandidate(APPROVE)` 成为
  `ACTIVE + APPROVED` memory 后才可进入 proposal evidence；Agent 仍不直接读
  memory-service 私表。该断言已在
  `ai-eval-rag-agent-demo-live-20260624-public-candidate-review-v3` 真实 gate 中通过。
- 2026-06-24 Agent action boundary cases 已扩展到 action-executor preflight
  safety eval：approval id、prepared audit id、resource id 与 approved proposal
  绑定不一致会在 verify-approved-proposal 阶段 fail-closed，且不生成 execution
  audit / tool result projection / tool execution。

## 边界

- 不执行真实工具或业务 mutation；不直接读 message / conversation / search / memory / policy 私表。
- 外部 LLM / Python worker / MCP provider 后续必须走 port、verifier、proposal / approval / executor / audit。

## 下一步

- 继续扩展 temporal update / profile recompute 和更完整 group-memory Agent proposal
  场景；真实写动作仍只走 proposal / approval / executor / audit。
