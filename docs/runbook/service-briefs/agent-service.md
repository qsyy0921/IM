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
- 2026-06-24 `ai-eval-rag-agent-demo-live-20260624-temporal-update-v2` 已进一步验证
  public candidate replacement temporal update：旧 memory 被 memory-service 标为
  `SUPERSEDED` 后，Agent proposal EvidencePack 只包含当前 `ACTIVE + APPROVED`
  replacement，不把旧事实作为当前 proposal evidence。
- 2026-06-24 Agent action boundary cases 已扩展到 action-executor preflight
  safety eval：approval id、prepared audit id、resource id 与 approved proposal
  绑定不一致会在 verify-approved-proposal 阶段 fail-closed，且不生成 execution
  audit / tool result projection / tool execution。
- 2026-06-24 `ai-eval-rag-agent-demo-live-20260624-profile-repair-approval-v3`
  已验证 profile repair approval：profile repair 必须经 workflow-service
  `REPAIR_APPROVAL` 审批后，才通过 memory-service 公开
  `RecomputeProfileAggregate` 更新 profile aggregate；修复后的 profile evidence 会进入
  Agent proposal EvidencePack。Agent 仍不直接读 memory-service 私表。
- 2026-06-24 `loadtest/ragagent` 已补 profile repair negative gate：Agent / RAG
  组合 demo 会在正确执行前验证未审批 workflow 不能触发 repair，且审批 payload
  hash 与 batch manifest 不匹配时必须 fail-closed；ai-eval rag-agent adapter 已把
  该 gate 纳入 summary 断言。
- 同日 `ai-eval-rag-agent-demo-live-20260624-profile-repair-negative-v1` 已通过真实
  service-stack gate：4 adapters、27 cases、27 passed、0 failed、0 skipped；该 run
  归档了 profile repair negative gate，并确认修复后的 profile evidence 仍进入 Agent
  proposal EvidencePack。
- 同日 `ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1`
  已通过真实 service-stack gate：Agent proposal 对 `DECISION` / `BLOCKER` /
  `FILE` 三类 group memory 保留 3 条 memory evidence、6 个 source refs 和 3 个
  cross-group source refs；proposal 仍走 approval / action-executor audit，不直接执行
  业务 mutation。
- 同日 `ai-eval-rag-agent-demo-live-20260624-business-proposal-source-chain-gate-v1`
  已通过真实 service-stack gate：Agent 基于 `DECISION` / `TASK` / `STATUS` 三类
  reviewed memory 生成 `conversation.note.create` 业务 proposal，approval 后由
  action-executor 记录 audit。2026-06-25 已补显式 opt-in
  `conversation.note.create` business adapter；配置 conversation-service gRPC 地址后，
  action-executor 可写真实 note fact。未配置时仍要求
  `business_action_executed=false`，证明 source-chain、approval 和 audit 边界，而不是伪造业务写入。
  同日 `loadtest/ragagent` 已补 execute-mode gate：显式开启
  `--expect-business-action-executed` 时必须确认 action-executor 执行成功、输出低敏且不回显
  note body，并验证 `conversation_notes` 中 note fact 与 proposal / approval 绑定一致；
  默认仍保持 audit-only gate。2026-06-25
  `ai-eval-rag-agent-demo-live-20260625-business-mutation-execute-v7` 已通过真实完整
  service-stack opt-in mutation smoke，确认 approved Agent proposal 能经 action-executor
  写入真实 conversation note fact。
- 2026-06-24 retrieval-gateway source-chain-aware rerank first pass 已落后，Agent
  仍只消费 EvidencePack，不自己实现排序或重查 source；proposal path 后续通过
  EvidencePack `rerank_score`、source refs、graph edges 和 profile evidence 继续扩展
  source-chain coverage。该 coverage 已通过
  `ai-eval-service-stack-live-20260624-retrieval-source-chain-rerank-v2` 和后续
  RAG-Agent service-stack gates 归档。

## 边界

- 不直接执行真实工具或业务 mutation；真实写动作必须交给 action-executor 和显式业务 adapter。
- 不直接读 message / conversation / search / memory / policy 私表。
- 外部 LLM / Python worker / MCP provider 后续必须走 port、verifier、proposal / approval / executor / audit。

## 下一步

- 继续消费并校验 EvidencePack source-chain / rerank coverage；下一步扩展其它真实
  mutation 前必须先补公开业务 API、显式 tool adapter、operator policy、低敏输出和
  repair / redrive 边界。
