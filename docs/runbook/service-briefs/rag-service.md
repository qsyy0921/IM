# rag-service

状态：foundation-active / provider boundary + cross-group temporal stack smoke passed.

定位：RAG 问答边界服务。它只消费 retrieval-gateway 返回的
`EvidencePack`，不直接读 message / conversation / search / memory 私有表，
不执行 Agent 动作，不绕过 policy / visibility。

当前已落：

- `rag_service.proto`、SDD、六层 skeleton、`grpc` runtime、metrics、Docker / observability wiring
- app usecase 调用 retrieval port 和 `AnswerProvider` port；默认本地
  extractive provider 基于 EvidencePack 生成 deterministic answer
- 已补 guarded external HTTP LLM boundary：只由 EvidencePack 构造 prompt；
  provider failure 返回稳定 unavailable，unsafe / malformed output fail closed
- 已补可选 `python-worker` provider mode：Go 先生成 grounded answer，Python
  worker 只返回 candidate hash / citations；Go 校验 id、hash 和 citation
- response 保留 citations、EvidencePack、`generated_by_llm=false`；provider 输出后统一运行 citation verifier
- retrieval-gateway 公开 proto RPC client、app / gRPC / cmd focused tests
- `loadtest/rag`、`tools/run-ai-eval-rag-adapter.ps1` 和真实本地
  `retrieval-gateway -> rag-service` adapter smoke 已通过
- `at_conversation_seq` 已透传到 EvidencePack current-memory query；CI-safe regression 和 2026-06-20 live smoke 均验证 stale memory 不作为 current citation。
- 2026-06-20 cross-group / temporal stack smoke 已验证跨群 source refs /
  speaker attribution 被保留，expired / superseded / future memory 不进入
  current EvidencePack。
- 2026-06-23 RAG live adapter 已增加 multi-hop actor/source-chain completeness
  断言；仍只基于 retrieval-gateway 返回的 EvidencePack 与 citation verifier
  判断，不直接读 memory / search 私表。
- 2026-06-24 RAG EvidencePack graph edge 透传已落：retrieval client 会保留
  `EvidenceMemoryGraphEdge`，gRPC response 会继续向调用方返回该字段；
  `loadtest/rag` 会断言跨群 source refs 与 `SUPPORTS` memory graph edge 被保留。
- 2026-06-24 RAG EvidencePack profile evidence 透传已落：retrieval client 会保留
  `PROFILE_AGGREGATE` evidence 的 profile subject、aggregate type/key、
  supporting memory ids 和时间字段；`loadtest/rag` 会断言 profile aggregate
  evidence 被保留。RAG 仍只消费 retrieval-gateway 返回的 EvidencePack。
- 2026-06-24 `loadtest/ragagent` 已提供 RAG-Agent demo first path：复用
  `loadtest/rag` 的 grounded answer 校验，并与 Agent proposal / approval /
  action-executor audit 组合成同一 tenant / conversation 的低敏总报告；RAG 仍不直接读
  Agent、action-executor 或任何私表。
- 2026-06-24 `loadtest/ragagent` / `rag-agent-demo` adapter 已把 memory-service
  公开 candidate review 纳入 RAG EvidencePack 断言链路：候选必须经
  `SubmitMemoryCandidate` -> `ReviewMemoryCandidate(APPROVE)` 成为
  `ACTIVE + APPROVED` memory 后才可被 RAG 作为 evidence 消费；该断言已在
  `ai-eval-rag-agent-demo-live-20260624-public-candidate-review-v3` 真实 gate 中通过。
- 2026-06-24 `ai-eval-rag-agent-demo-live-20260624-temporal-update-v2` 已进一步验证
  public candidate replacement temporal update：旧 memory 被 memory-service 标为
  `SUPERSEDED` 后，RAG EvidencePack 只包含当前 `ACTIVE + APPROVED` replacement，
  不把旧事实作为当前 evidence。
- 2026-06-24 `ai-eval-rag-agent-demo-live-20260624-profile-repair-approval-v3`
  已验证 profile repair approval：profile repair 必须经 workflow-service
  `REPAIR_APPROVAL` 审批后，才通过 memory-service 公开
  `RecomputeProfileAggregate` 更新 profile aggregate；修复后的 profile evidence 会进入
  RAG EvidencePack。
- 2026-06-24 `ai-eval-rag-agent-demo-live-20260624-profile-repair-negative-v1`
  已验证 profile repair negative gate：未审批 workflow 和 approval payload hash mismatch
  均 fail closed；同时修复后的 profile evidence 仍进入 RAG EvidencePack。
- 2026-06-24 `ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1`
  已验证 group-memory answer 场景：RAG answer 对 `DECISION` / `BLOCKER` /
  `FILE` 三类 group memory 返回 `GROUNDED`，并保留 3 条 memory evidence、6 个
  source refs 和 3 个 cross-group source refs。

下一步：

- 继续扩展 EvidencePack source-chain coverage 和更真实的业务 proposal 场景，
  provider 仍走 port、guard 和 citation verifier。
