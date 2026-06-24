# memory-service

状态：foundation-active / first projection smoke passed。`memory_service.proto`、PostgreSQL projection migration、六层 runtime 和 focused projection smoke 已落。

定位：group memory / StructuredMemoryEvent / Memory Graph / ProfileAggregate 投影服务。它消费 message / member / domain events，维护带 source refs、speaker / audience scope、validity window、supersession、confidence 和 review state 的协作记忆，不做 RAG 回答，不执行 Agent 动作，不替代 search / policy / message facts。

当前已落：

- `docs/sdd/memory-service.md`
- `api/proto/nexusim/memory/v1/memory_service.proto`
- `migrations/postgres/memory/000001_memory_core.sql`
- `services/memory-service` 六层 skeleton、`grpc` runtime mode、`timeline-consumer` runtime mode、debug `/metrics`
- domain / types / app validation、PostgreSQL repository first pass、timeline projection usecase、PG integration tests、timeline worker tests
- registry / Docker runtime / local compose / Prometheus / Grafana foundation-active wiring
- `loadtest/memory` 和 clean projection smoke：member join -> message persisted -> PENDING StructuredMemoryEvent + source ref -> Query/Get -> revoke hidden
- profile overgeneralization eval case + local fixture adapter：单条群聊事实不能升级为 ACTIVE profile，profile candidate 必须保留 GROUP scope 且 PENDING_REVIEW
- group memory fixture eval：ACTIVE 需要 source refs，validity window 必须保留，superseded memory 不能作为 current fact
- runtime current-only query semantics：`QueryMemoryEvents.at_conversation_seq` 会过滤
  `valid_from_seq / valid_to_seq`，默认 ACTIVE query 不返回 `SUPERSEDED`
  memory，且返回项保留 source refs 和 `supersedes_event_ids`
- `loadtest/memory` 已增加 source-ref、validity window、supersession 的 runtime
  smoke checks；真实进程 smoke 仍按需要手动运行
- RAG / Summary / Agent current-memory consumption CI-safe regression 已接入
  ai-eval gate，验证过期和 superseded memory 不应作为 current citation。
- memory extraction confidence / review eval、cross-group attribution-chain 和
  temporal query-seq version selection eval 已接入 ai-eval gate。
- 2026-06-23 低敏 collaborative-memory eval 继续扩展：multi-hop actor-chain completeness、
  workstream / decision dependency edge、reviewed multi-source profile activation、
  supporting-memory delete 后 profile recompute 已纳入 profile / Agent safety fixture；
  该覆盖仍是本地 eval gate，不调用模型、数据库或业务服务。
- 2026-06-23 memory-service optional live adapter first pass 已落：
  `ListProfileAggregates` 只返回 supporting memory 全部仍为 ACTIVE / APPROVED
  的 ACTIVE profile aggregate；`loadtest/memory` 增加 GetMemoryEvent graph edge、
  reviewed multi-source profile、supporting evidence 保留和 deleted support profile
  排除检查；`run-ai-eval-memory-adapter.ps1` 可把这些结果映射到 ai-eval cases。
- 2026-06-24 retrieval-gateway 已通过 memory-service 公开 `ListProfileAggregates`
  把当前用户 ACTIVE profile aggregate 放入 EvidencePack；RAG / Summary / Agent
  只通过 EvidencePack 消费 profile evidence，不直接读 memory-service 私表。
- 2026-06-24 memory-service 已新增公开 `RecomputeProfileAggregate` first path：
  它从当前用户可见的多个 ACTIVE / APPROVED `PROFILE_SIGNAL` memory events 重算
  `ProfileAggregate`，保留 supporting memory ids；支持数量不足时会归档既有
  ACTIVE / PENDING profile aggregate，避免 deleted / rejected supporting memory
  继续暴露为 profile evidence。PG integration test 和 `loadtest/memory` 已改为
  通过该 API 验证 reviewed multi-source profile，而不是手工写入 active profile。
- 2026-06-24 `loadtest/memoryprofile` 已提供 first-stage profile repair operator：
  默认 plan-only，只有显式 `--execute` 才调用 memory-service 公开
  `RecomputeProfileAggregate`；输出低敏报告，只保留 aggregate key / supporting memory
  ids / summary text 的 hash 和计数，不写 raw profile summary 或 memory text。
- 2026-06-24 `loadtest/memoryprofile` 已补 profile repair batch approval path：
  batch manifest 默认仍只生成低敏 plan；批量执行必须提供已批准的 workflow-service
  `REPAIR_APPROVAL` workflow id，并校验 workflow status、target service / operation、
  payload schema、payload hash 和 target hash 全部匹配后才逐项调用公开
  `RecomputeProfileAggregate`。也可显式 `--request-approval` 创建低敏 repair
  approval workflow；该路径不执行 repair、不读 memory-service 私表。
- 2026-06-24 timeline worker 已升级 `rules-v0.2` group memory extraction：
  只抽取带明确 `decision:` / `task:` / `status:` / `blocker:` / `file:` /
  `profile_signal:` 等 cue 或显式 memory metadata 的消息；普通聊天只推进 checkpoint，
  不被泛化成 StructuredMemoryEvent；profile / preference / role signal 保持
  PENDING + NEEDS_REVIEW，不直接升级个人画像。
- 2026-06-24 Python AI Worker 已补 `memory-extraction-candidate` first path：
  `ai/python/nexusim_ai_memory` 从显式低敏 message batch 中识别同一组 memory cue，
  输出 hash-only `MEMORY_EVENT_CANDIDATE`、source refs、citation refs、event type、
  speaker / message hash 和低敏 report；普通聊天输出 0 个候选；`profile_signal`
  强制 `NEEDS_REVIEW` / `GROUP_SCOPE_PROFILE_SIGNAL`。该路径不写 memory-service
  数据库、不返回 raw text、不绕过 Go-side validation / approval / audit。
- 2026-06-24 Go-side memory extraction candidate adapter 已接入：
  `internal/ai/memorycandidate` 负责调用 Python batch CLI、校验低敏 request /
  batch result、拒绝 raw text / plaintext id 字段 / final persistence claim，并强制
  `profile_signal` 保持 review required；`tools/memory-extraction-go-adapter-smoke`
  和 `run-ai-eval-memory-extraction-candidate-adapter.ps1` 覆盖 explicit cue hash-only、
  ordinary chat zero candidates、profile review 和 unsafe input fail-closed。
- 2026-06-24 memory-service 已补公开 candidate review / approval / persistence path：
  `SubmitMemoryCandidate` 要求 candidate id、conversation scope、source refs、fact text
  与 Python candidate `fact_sha256` 匹配，并先写入 `PENDING + NEEDS_REVIEW`；
  `ReviewMemoryCandidate` 只允许可见 source refs 的 reviewer 将 pending candidate
  显式推进为 `ACTIVE + APPROVED` 或 `REJECTED + REJECTED`。PG integration tests
  覆盖 submit -> approve、reject、不可见 source fail-closed；`loadtest/memory`
  和 memory-service ai-eval adapter 已新增 public candidate review 检查。
- 2026-06-24 public candidate temporal update 已补齐：当 replacement candidate 带
  `supersedes_event_ids` 并被公开 `ReviewMemoryCandidate(APPROVE)` 审批通过时，
  memory-service 会在同一 PostgreSQL 事务内校验被 supersede memory 可见、同 scope、
  已 `ACTIVE + APPROVED` 且时间顺序正确，然后把旧 memory 标为 `SUPERSEDED` 并把
  `valid_to_seq` 截断到 replacement 前一 seq；失败时整次审批 fail-closed。
- 2026-06-24 profile repair approval 已通过真实 RAG-Agent service-stack gate：
  `loadtest/ragagent` 会通过公开 candidate review path 写入 `PROFILE_SIGNAL`，
  再经 workflow-service `REPAIR_APPROVAL` 审批后调用公开
  `RecomputeProfileAggregate`。修复后的 profile aggregate 同时进入 RAG / Agent
  EvidencePack。repository 同步修复了既有 non-deterministic `profile_id` 但
  subject/type/key 相同的 profile aggregate 重算唯一约束问题，并新增真实 PG
  集成测试。
- 2026-06-24 `loadtest/ragagent` 已把 profile repair 负向门禁接入 demo summary：
  未审批 workflow 不能执行 batch recompute，已审批 workflow 的 payload hash 与
  当前 batch manifest 不匹配时必须 fail-closed；ai-eval rag-agent adapter 已把
  该负向门禁纳入必检断言。
- 同日 `ai-eval-rag-agent-demo-live-20260624-profile-repair-negative-v1` 已通过真实
  service-stack gate：4 adapters、27 cases、27 passed、0 failed、0 skipped；该 run
  归档了上述负向门禁，并确认匹配审批后才执行 memory-service public recompute。
- 同日 `ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1`
  已通过真实 service-stack gate：通过公开 candidate review path 构造的
  `DECISION` / `BLOCKER` / `FILE` group memory 会同时进入 RAG answer 和 Agent
  proposal EvidencePack，并保留跨群 source refs。
- 同日 `ai-eval-rag-agent-demo-live-20260624-business-proposal-source-chain-gate-v1`
  已通过真实 service-stack gate：`DECISION` / `TASK` / `STATUS` 三类 reviewed memory
  驱动 `conversation.note.create` 业务 proposal，并经 approval / action-executor audit
  记录。2026-06-25 已补显式 opt-in conversation note business adapter；配置
  conversation-service gRPC 地址后可写真实 note fact，未配置时仍不执行业务写动作。

下一步：`loadtest/ragagent` / `rag-agent-demo` adapter 已把 public candidate
review、temporal update 和 profile repair approval 纳入 RAG / Agent EvidencePack 断言链路，并已通过
`ai-eval-rag-agent-demo-live-20260624-public-candidate-review-v3` 和
`ai-eval-rag-agent-demo-live-20260624-temporal-update-v2`、`ai-eval-rag-agent-demo-live-20260624-profile-repair-approval-v3`
真实 gate 归档；profile repair negative gate 也已通过
`ai-eval-rag-agent-demo-live-20260624-profile-repair-negative-v1` 真实 gate 归档。
group-memory answer / proposal gate 也已通过
`ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1` 真实 gate
归档。business proposal source-chain gate 也已通过
`ai-eval-rag-agent-demo-live-20260624-business-proposal-source-chain-gate-v1` 真实 gate
归档。之后继续做结构过滤、BM25 / vector、rerank 和更细 source-chain coverage。仍不得把
单条群消息直接升级为 ACTIVE profile fact。
