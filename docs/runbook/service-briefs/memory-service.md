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

下一步：真实服务栈启动后运行 memory-service optional adapter / loadtest 并归档报告；
后续继续做 Python memory extraction candidate 的 Go-side adapter / ai-eval 接入和
RAG-Agent demo module 集成，而不是把单条群消息直接升级为 ACTIVE profile fact。
