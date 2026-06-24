# NexusIM Current Goal

短入口，只维护当前可执行目标。长事实分别放在：

- 阶段背景：`docs/runbook/current-brief.md`
- 客户端细节：`docs/runbook/client-platform.md`
- 未完成工作：`docs/runbook/remaining-goals.md`
- 长期架构：`docs/architecture/target-architecture-complete.md`

## Active Slice

```text
backend architecture + AI / Agent / RAG demo path
```

目标：

```text
IM messages -> search / memory projection -> EvidencePack -> RAG / Agent answer -> approval / audit
```

## 当前事实

- Web / Windows PC 客户端演示 MVP 已达标；Android 后置到用户明确切回。
- 客户端当前停在“可演示 MVP”，不继续追完整产品级 UI、release signing、
  MSI / NSIS installer、完整移动端、媒体体验或群管理深水区。
- Web / PC / Android 共用 `clients/packages/protocol` 和
  `clients/packages/client-core`；客户端只连 `api-gateway` BFF 和 `push-gateway`。
- Web / PC shell 已接账号密码登录、注册、好友工作台、好友私聊、群聊列表、建群、
  群成员添加 / 退群、从好友列表邀请入群、成员列表、移除成员、角色变更、owner
  transfer、成员搜索 / 角色过滤 / 分页、群资料卡、权限感知群设置入口、邀请来源提示、
  群公告、会话置顶 / 免打扰、归档、标签、草稿、会话筛选、消息列表、发送、PullInbox 和 ACK。
- 群设置 UI 已按资料、成员、操作分区；资料和成员事实仍只来自 conversation-service
  经 api-gateway BFF 暴露的公开接口，不维护本地假成员列表。
- 群标题 / 头像 URI / 群公告已接 first-stage read/update：`conversation-service` 拥有事实，
  `api-gateway` 暴露 BFF，Web / PC shell 只通过 BFF 读写。群头像上传 first path
  已通过 `api-gateway` BFF -> `media-service` 上传会话 / 完成上传 ->
  `conversation-service` profile update 串起来；头像展示会再通过 BFF 校验当前
  profile avatar URI 并向 `media-service` 换取短期 download URL；当前仍是本地 fake object HTTP adapter，
  不宣称真实 S3、缩略图、扫描或 CDN 已完成。
- 会话刷新会保留当前选中；gateway token 过期会清理本地 session / push / 会话展示状态，
  且 Web / PC shell expired-session cleanup contract 已纳入 client focused check。
- 本地发送失败的消息会保留在消息列表中，并提供显式重新编辑或作为新消息重发入口；
  客户端不会把失败缓存项静默标记为成功。
- `plan:browser-multiuser-ui-smoke` 已可从成功的 `loadtest/clientweb`
  `client-web-summary.json` 生成低敏浏览器 / PC 多用户 UI smoke 计划，覆盖直聊、
  群聊、群设置、会话管理（标签 / 草稿 / 归档 / 筛选）和稳定 selector，不保存密码、gateway token、push token 或
  refresh token；这仍是计划 / 契约，不宣称真实浏览器自动化已跑。
- `smoke:browser-multiuser-ui` 已提供显式 opt-in 的真实浏览器 / PC 多用户 UI runner：
  使用两个隔离 Chromium profile 通过 CDP 驱动 Web shell，覆盖登录、点击好友发起直聊、
  UI 建群、邀请成员、群聊发送、会话管理 controls、PullInbox 和 ACK；`loadtest/clientweb/run-local-smoke.ps1`
  可通过 `-RunBrowserMultiuserUISmoke` 调用。2026-06-23 已在 clean commit
  `8782936b` 跑通 direct / group / invite 路径并归档报告；随后 clean commit
  `7e8a890b` 跑通 direct / group / invite + 会话标签 / 草稿 / 归档 round-trip
  的真实浏览器 / PC 多用户 UI smoke；clean commit `05b8aec6` 进一步验证会话
  tag / draft / archived-only 筛选的匹配和排除路径。2026-06-23 追加
  `client-demo-mvp-browser-ui-20260623-231711` 真实验收，verdict 全部为 true：
  direct chat、group chat、group invite、conversation management、receiver ACK。
  默认路径不启动浏览器。
- 2026-06-23 `client-demo-mvp-desktop-login-20260623-232819` 已通过 Windows
  desktop WebView 登录级真实 smoke：Tauri WebView 登录、push connected、direct
  conversation 外部消息触发、PullInbox、message observe、AckDelivery 和
  `tauri-sqlite` native store readiness 全部为 true。此次修正了
  `run-local-smoke.ps1` 桌面 smoke 错用群会话的问题：桌面 smoke 现在从
  `client-web-summary.json` 显式读取双方仍 active 的 `direct_chat.conversation_id`，
  读不到直接失败，不使用群会话兜底。
- 2026-06-23 低敏 collaborative-memory eval 已扩展到 73 个 catalog cases / 20 个
  profile-Agent safety fixture cases；新增 memory-service / retrieval-gateway
  optional live adapters，并让 RAG / Summary / Agent live adapters 断言
  multi-hop actor/source-chain completeness。memory-service adapter 覆盖
  GetMemoryEvent graph edge、reviewed multi-source profile 和 deleted supporting
  memory 排除；retrieval adapter 覆盖 EvidencePack source refs、speaker
  attribution、temporal/current-memory 和 projection version。
- 2026-06-24 `ai-eval-service-stack-live-20260624-collab-memory-v4` 已通过真实
  service-stack gate：8 adapters、51 cases、47 passed、0 failed、4 skipped。
  通过范围包括 required profile / action safety adapters，以及 memory-service、
  retrieval-gateway、rag-service、summary-service、agent-action-executor live
  adapters。4 个 skipped 是当时尚未覆盖的 retrieval-gateway negative / miss
  cases，不计入 positive live EvidencePack smoke 的通过范围。
- 2026-06-24 `ai-eval-service-stack-live-20260624-retrieval-negative` 已补上
  retrieval-gateway negative / miss 专用 adapter，并通过真实 service-stack gate：
  9 adapters、51 cases、51 passed、0 failed、0 skipped。新增覆盖包括
  `source_coverage_status=EMPTY`、superseded memory 排除、source ref / dedupe
  reason 断言和 cross-tenant evidence 不泄漏。
- 2026-06-24 retrieval-gateway EvidencePack 已补 memory graph edge 扩展：
  retrieval-gateway 会通过 memory-service 公开 `GetMemoryEvent` 读取当前 memory
  的 graph edges，并把 `EvidenceMemoryGraphEdge` 透传给 RAG / Agent。RAG /
  Agent RPC mapping、`loadtest/retrieval`、`loadtest/rag`、`loadtest/agent`
  都会断言跨群 source refs 与 `SUPPORTS` graph edge 被保留；memory lookup
  失败时 retrieval-gateway fail-closed，不静默降级为无 graph edge。
- 2026-06-24 retrieval-gateway EvidencePack 已补 profile aggregate evidence：
  retrieval-gateway 会通过 memory-service 公开 `ListProfileAggregates` 查询当前
  `auth.user_id` 的 ACTIVE profile aggregate，并作为 `PROFILE_AGGREGATE`
  evidence 放入 EvidencePack；RAG / Summary / Agent 只透传 / 消费 EvidencePack，
  不直接读 memory-service 私表。`loadtest/retrieval`、`loadtest/rag`、
  `loadtest/agent` 会断言 profile subject、aggregate type/key、supporting memory
  ids 和 source coverage 被保留；profile lookup 失败时 fail-closed。
- 2026-06-24 memory-service 已补公开 `RecomputeProfileAggregate` first path：
  从当前用户可见的多个 ACTIVE / APPROVED `PROFILE_SIGNAL` memory events 重算
  profile aggregate，保留 supporting memory ids；支持数量低于阈值时归档既有
  ACTIVE / PENDING aggregate，避免 stale / deleted support 继续进入 EvidencePack。
  `loadtest/memory` 已改为调用该 RPC 验证 reviewed multi-source profile，而不是
  直接手工写入 active profile。
- 同日 ai-eval memory-service live adapter 已新增
  `must_recompute_profile_via_public_api` 断言，把该 RPC 行为纳入 service-stack
  gate contract。
- 同日 `loadtest/memoryprofile` 已补 first-stage profile repair operator：
  默认 plan-only，显式 `--execute` 才调用公开 `RecomputeProfileAggregate`；
  输出低敏 hash / count 报告，不写 raw profile summary 或 memory text。
- 同日 `loadtest/memoryprofile` 已补 profile repair batch approval path：
  batch manifest 默认只生成低敏 plan；批量执行必须先通过 workflow-service
  公开 `GetWorkflow` 校验 `REPAIR_APPROVAL/APPROVED`，且 workflow target / payload
  hash 与本次 batch plan 完全匹配；也可显式 `--request-approval` 创建低敏
  repair approval workflow，但该路径不执行 repair。
- 同日 memory-service timeline worker 已升级 `rules-v0.2` group memory extraction：
  只抽取带明确 memory cue 或显式 memory metadata 的群消息；普通聊天不生成
  memory fact；profile / preference / role signal 保持 PENDING + NEEDS_REVIEW。
- 同日 Python AI Worker 已补 `memory-extraction-candidate` first path：
  `ai/python/nexusim_ai_memory` 只从显式低敏 message batch 中抽取
  `decision:` / `task:` / `status:` / `blocker:` / `file:` / `profile_signal:`
  cue；普通聊天输出 0 个候选；结果只返回 candidate hash、source refs、citation refs、
  speaker / message hash、event type 和低敏计数，不返回 raw text、不写 memory fact。
  `profile_signal` 会强制标记 `NEEDS_REVIEW` / `GROUP_SCOPE_PROFILE_SIGNAL`，
  最终入库仍必须由 Go 侧验证、审批、审计和 memory-service 持久化。
- 同日 Go-side memory extraction candidate adapter 和 ai-eval 接入已补齐：
  `internal/ai/memorycandidate` 调用 Python batch CLI，并在 Go 侧校验 request /
  batch result、拒绝 raw text / plaintext id 字段 / final persistence claim、强制
  profile signal review；`tools/memory-extraction-go-adapter-smoke` 和
  `run-ai-eval-memory-extraction-candidate-adapter.ps1` 覆盖 explicit cue hash-only、
  ordinary-chat zero candidates、profile review required 和 unsafe input fail-closed。
  ai-eval catalog 增至 80 个 cases，新增 adapter 为 optional local adapter，不启动服务栈。
- 同日 memory-service 已补公开 candidate review / approval / persistence path：
  `SubmitMemoryCandidate` 校验 conversation-scoped source refs 对 reviewer 可见，
  并要求 Go 侧提交的 `fact_text` 与 Python candidate `fact_sha256` 一致；通过后只写入
  `PENDING + NEEDS_REVIEW`。`ReviewMemoryCandidate` 才能将候选显式推进为
  `ACTIVE + APPROVED` 或 `REJECTED + REJECTED`。PG integration tests 覆盖
  submit -> approve、reject、不可见 source fail-closed；`loadtest/memory` 和
  memory-service ai-eval adapter 已新增 public candidate review 检查；ai-eval catalog
  增至 81 个 cases。
- 同日 Agent action boundary cases 已补齐一轮 action-executor preflight safety：
  `action-preflight-safety` smoke / eval catalog 从 11 个扩到 14 个 case，新增
  approval id、prepared audit id、resource id 与已批准 proposal 绑定不一致时的
  `PROPOSAL_MISMATCH` 断言；这些 case 都要求在 approved-proposal verification
  阶段 fail-closed，不写 execution audit、不写 tool result projection、不调用 tool executor。
- 同日 `loadtest/ragagent` 已提供 RAG-Agent demo first path：编排既有
  `loadtest/rag` 与 `loadtest/agent`，让 RAG answer、Agent proposal、approval
  和 action-executor audit 围绕同一 tenant / conversation 生成一份低敏总报告。
  该 runner 不读取私表、不保存 raw answer / proposal text，只记录 hash、计数和
  EvidencePack / approval / execution 状态。
- 2026-06-24 `ai-eval-rag-agent-demo-live-20260624-current-image-fixed` 已通过
  RAG-Agent demo 真实 service-stack gate：4 adapters、27 cases、27 passed、0 failed、
  0 skipped。该链路确认 RAG grounded answer、Agent proposal / approval、
  action-executor audit、同一 tenant / conversation、cross-group source refs、
  speaker attribution、memory graph edge 和 profile aggregate evidence 均成立。
- 2026-06-24 `ai-eval-rag-agent-demo-live-20260624-public-candidate-review-v3`
  已通过真实 service-stack gate：4 adapters、27 cases、27 passed、0 failed、
  0 skipped。该链路进一步确认 memory-service 公开
  `SubmitMemoryCandidate -> ReviewMemoryCandidate(APPROVE)` 产生的
  `ACTIVE + APPROVED` memory 会进入 RAG / Agent EvidencePack，并保留 source refs。
- 2026-06-24 `ai-eval-rag-agent-demo-live-20260624-temporal-update-v2` 已通过真实
  service-stack gate：4 adapters、27 cases、27 passed、0 failed、0 skipped。
  该链路把 public candidate replacement 接入 RAG-Agent demo：replacement 经公开
  `SubmitMemoryCandidate -> ReviewMemoryCandidate(APPROVE)` 审批后 supersede 旧
  memory，旧 memory 变为 `SUPERSEDED` 且 `valid_to_seq` 截断到 replacement 前一 seq；
  RAG / Agent EvidencePack 只保留当前 `ACTIVE + APPROVED` replacement，不包含旧
  memory。
- 2026-06-24 `loadtest/ragagent` 已把 profile repair approval 纳入组合演示断言：
  demo 会通过公开 `SubmitMemoryCandidate -> ReviewMemoryCandidate(APPROVE)` 构造
  `PROFILE_SIGNAL`，用 `loadtest/memoryprofile --request-approval` 创建
  `REPAIR_APPROVAL` workflow，经 workflow-service 公开审批后再执行 batch
  recompute，并断言修复后的 profile aggregate 同时进入 RAG / Agent EvidencePack。
- 同日 `ai-eval-rag-agent-demo-live-20260624-profile-repair-approval-v3` 已通过真实
  service-stack gate：4 adapters、27 cases、27 passed、0 failed、0 skipped。
  该链路确认 profile repair 必须先创建并审批 `REPAIR_APPROVAL` workflow，再通过
  memory-service 公开 `RecomputeProfileAggregate` 执行 batch recompute；修复后的
  profile aggregate 会同时进入 RAG / Agent EvidencePack。过程中还修复了
  memory-service 对既有 non-deterministic `profile_id` 但相同 subject/type/key
  的 profile aggregate 重算唯一约束问题，新增真实 PostgreSQL 集成测试覆盖。
- 2026-06-24 `loadtest/ragagent` 已把 profile repair 负向门禁纳入组合报告：
  在正确审批执行前，demo 会先验证未审批 `REPAIR_APPROVAL` workflow 不能执行
  batch recompute，并验证已审批 workflow 的 payload hash 与当前 batch manifest
  不匹配时必须 fail-closed。`run-ai-eval-ragagent-adapter.ps1` 已把
  `profile_repair_negative_cases_verified` 和 case count 纳入必检断言。
- 同日 `ai-eval-rag-agent-demo-live-20260624-profile-repair-negative-v1` 已通过真实
  service-stack gate：4 adapters、27 cases、27 passed、0 failed、0 skipped。
  该 run 确认 profile repair negative gate 已归档：未审批 workflow 和 approval
  payload hash mismatch 均 fail-closed，之后匹配的已审批 workflow 才会执行
  memory-service public recompute，并把修复后的 profile aggregate 放入 RAG / Agent
  EvidencePack。
- 同日 `loadtest/ragagent` 已补 group-memory answer / proposal 场景：
  通过 memory-service 公开 `SubmitMemoryCandidate -> ReviewMemoryCandidate(APPROVE)`
  构造 `DECISION`、`BLOCKER`、`FILE` 三类协作记忆，再要求 RAG answer 和 Agent
  proposal 同时保留这些 evidence、source refs 和跨群 source refs。真实
  `ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1`
  service-stack gate 已通过：4 adapters、27 cases、27 passed、0 failed、0 skipped。
- 同日 `loadtest/ragagent` 已补真实业务 proposal source-chain 场景：
  通过公开 memory candidate review 构造 `DECISION`、`TASK`、`STATUS` 三类业务证据，
  Agent 基于 EvidencePack 生成 `conversation.note.create` proposal，经
  `ApproveAgentProposal` 审批后调用 action-executor 记录执行审计。由于该业务 tool
  没有配置真实 mutation adapter，本场景要求 `business_action_executed=false`，
  只证明 approval / action audit 边界、source-chain evidence 和低敏 input hash 成立。
  真实 `ai-eval-rag-agent-demo-live-20260624-business-proposal-source-chain-gate-v1`
  service-stack gate 已通过：4 adapters、27 cases、27 passed、0 failed、0 skipped。
- 2026-06-24 retrieval-gateway 已补 vector-index-service `SearchVectors` adapter
  first path：`RetrieveEvidence` 支持显式 `include_vector`，调用方必须提供低敏
  `query_embedding_ref`、明确的 `vector_collection_types`、`vector_visibility_scope`
  和 `vector_policy_version`；
  retrieval-gateway 只把 vector item ref、source ref hash、source service、
  collection type、visibility version 和 tombstone status 放入 `VECTOR_ITEM`
  EvidencePack，不传 raw text 或 embedding vector。vector lane 独立参与 RRF 风格融合；
  未配置 vector port、vector dependency error 或 malformed vector result 均 fail-closed。
- 同日 `retrieval-vector-backend-smoke-20260624-223543` 已通过真实 opt-in smoke：
  `loadtest/retrieval/run-local-smoke.ps1 -IncludeVectorBackend` 启动 search-service、
  memory-service、vector-index-service 和 retrieval-gateway；runner 通过
  vector-index-service 公开 `UpsertVectorItem` seed 低敏 vector metadata，再要求
  retrieval-gateway 通过公开 `SearchVectors` 返回 refs-only `VECTOR_ITEM`。EvidencePack
  source counts 为 search / memory / profile / vector 各 1 条，vector evidence 不携带
  raw text 或 embedding vector。
- 2026-06-24 search-service 已补 PostgreSQL FTS lexical backend first path：
  `SearchMessages` 使用 `plainto_tsquery('simple') + to_tsvector('simple')` 和既有
  GIN index 做 token-based search，不再使用 `ILIKE` substring fallback。该能力只宣称
  PostgreSQL FTS first path。
- 2026-06-24 search-service 已补显式 OpenSearch / BM25 candidate backend first path：
  `NEXUSIM_SEARCH_BACKEND=opensearch` 时通过 OpenSearch `_search` + `match`
  召回 `conversation_id/message_id` 候选，再回到 PostgreSQL projection 做
  membership visibility、tombstone、after_seq 和 conversation filter hydration；
  OpenSearch 配置 / 请求 / malformed result 均 fail-closed，不静默回退 PostgreSQL FTS。
- 同日 search-service 已补 opt-in OpenSearch backend smoke 入口：
  `loadtest/search/run-local-smoke.ps1 -UseOpenSearchBackend` 会启动 search-service
  `opensearch` backend，并由 runner 将已投影的低敏 search document 写入临时
  OpenSearch index 后，再通过 search-service gRPC 验证候选召回和 PostgreSQL
  visibility / tombstone hydration。当前机器 Docker API 和本地 OpenSearch 端口不可用，
  因此本轮只验证 focused tests / build / script 入口；真实 OpenSearch 进程 smoke
  尚未归档通过报告。
- 2026-06-25 search-service 已补 OpenSearch rebuild operator first path：
  `NEXUSIM_SEARCH_SERVICE_MODE=opensearch-rebuild` 会从 search-service 自有
  PostgreSQL projection 按 tenant 批量读取非 tombstone / 非空 searchable_text
  document；默认 dry-run，只有 `NEXUSIM_SEARCH_REBUILD_EXECUTE=true` 才写 OpenSearch。
  写入使用 create-index / NDJSON bulk / refresh，并且错误 fail-closed；该 operator
  不跨服务读私表，不把 OpenSearch 变成权限事实源。真实 OpenSearch 进程验证仍待
  Docker / OpenSearch runtime 恢复。
- 同日 search-service 已补 OpenSearch index contract hardening：
  rebuild operator 创建 index 时写入 `nexusim.search.messages.v1` strict mapping 和
  `_meta` owner / source projection；已有 index 会读取 `_mapping` 校验 mapping version、
  `dynamic=strict` 和必需字段类型，drift 时 fail-closed 为 `SEARCH_UNAVAILABLE`，
  不继续 bulk 写入。
- 2026-06-25 retrieval positive smoke / adapter 已把 EvidencePack `source_coverage`
  矩阵纳入低敏门禁：search / memory / profile 必须 `RETURNED`，未启用 vector 时
  `VECTOR_ITEM` 必须 `NOT_REQUESTED`；summary 只输出 source type、requested、
  candidate / returned / deduped count 和 status。
- 同日 vector-index-service pgvector provider smoke 已补 preflight gate：
  `loadtest/vectorembedding --phase preflight-pgvector` 会验证 pgvector PostgreSQL
  连接、`vector` extension 可用性和 table identifier 配置；不可用时快速 fail-closed
  并写低敏 summary，不进入长链路或伪造真实 provider smoke。当前本机 `localhost:15432`
  不可用，因此只完成 preflight 负向验证，真实 pgvector smoke 仍待 runtime 恢复。
- 同日 vector-index-service OpenSearch vector provider smoke 已补 preflight gate：
  `loadtest/vectorembedding --phase preflight-opensearch-vector` 会验证 OpenSearch
  endpoint、index 存在、mapping 中指定字段为 `knn_vector` 且 dimension 匹配；
  endpoint 不允许 credentials / query / fragment；不可用、index 缺失或 mapping drift
  都 fail-closed 并写低敏 summary，不写 OpenSearch，也不伪造真实 provider smoke。
- 同日 vector-index-service Milvus provider smoke 已补 preflight gate：
  `loadtest/vectorembedding --phase preflight-milvus-vector` 会通过 Milvus REST v2
  验证 endpoint、collection 存在、vector field dense type 和 dimension contract；
  endpoint 不允许 credentials / query / fragment，token 只走 Authorization header；
  不可用、collection 缺失或 schema drift 都 fail-closed 并写低敏 summary，不写
  Milvus，也不伪造真实 provider smoke。
- 同日 vector-index-service provider readiness matrix 已补：
  `loadtest/vectorembedding --phase preflight-provider-readiness` 可一次性检查
  pgvector / OpenSearch vector / Milvus 的 readiness，并输出 `provider_readiness[]` 低敏矩阵；
  任一 requested provider 不满足 contract 时整体 fail-closed，但保留每个 provider
  的低敏状态用于排障；该矩阵仍不是 provider 数据写入 / 搜索 smoke。
- 同日 provider preflight / readiness 已补显式短超时门禁：pgvector、OpenSearch
  vector 和 Milvus 的探测都使用 `request-timeout` 作为单 provider 硬超时，并在
  summary 输出 `provider_request_timeout_ms`；runtime 不可用时快速 fail-closed，
  不等待全局 verify timeout，也不切换到其它 provider。
- 同日 retrieval provider coverage contract 已补：
  `loadtest/retrieval --provider-readiness-summary <path>` 可读取上述
  `preflight-provider-readiness` summary，并把 pgvector / OpenSearch vector / Milvus
  readiness 作为低敏 `provider_coverage[]` 挂到 EvidencePack smoke summary；
  只输出 provider、configured / available / status、error class 和 VECTOR_ITEM lane
  状态。启用 `--include-vector-backend` 时，任一 requested provider readiness 不是
  `READY` 会 fail-closed，不把 vector evidence 当成 provider-backed 证据。
- 已有 clean smoke 覆盖真实双用户好友直聊、群聊 first path、群资料 BFF
  read/update 和群成员动作链路；证据见 `docs/runbook/client-platform.md`。
- Windows desktop 已有 artifact / signing / installer plan first paths；签名 / installer
  工具已支持显式 local signing profile 输入，并有只读 release readiness report 汇总
  签名输入、低敏 `signtool` 候选提示、签名验证和 installer 阻塞；候选工具不会
  自动用于 readiness；timestamp URL 禁止携带账号密码、query 或 fragment；
  仓库已有 PFX 和 Windows cert-store 两个低敏 signing profile 示例，实际发布时需复制成
  untracked 本机 profile 并填入真实本机路径 / thumbprint；`init:desktop-signing-profile`
  是显式 copy helper，要求 `--source` 和 `--output`，不读取证书 / 密钥 / 密码；
  PFX 输入会做只读可读性 / signing key / 过期检查；Windows cert-store thumbprint
  会做只读本机证书 / signing key / 过期检查；profile 可声明预期公开 signer subject，
  valid signature 必须匹配该发布者策略；signing plan 可携带 CLI、
  `NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT` 或 profile 中的公开 signer subject policy，但不宣称已验证；
  `verify:desktop-signature` 也可通过 CLI、
  `NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT` 或 profile 读取公开 signer subject policy，
  但不会使用证书源签名或修改 artifact；signing executor 也可通过 CLI、
  `NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT` 或 profile 读取公开 signer subject policy；
  其低敏 execution policy 会声明 profile 读取和 `--require-valid` 下的 signer subject enforcement；
  release readiness report 的 top-level 和 nested signing execution policy 也会声明 profile
  读取 / signer subject policy 检查，并输出低敏 executable / installer `signaturePolicy`
  摘要以表明公开 signer policy 是否配置和匹配；同时会对已收集的
  `desktop-installer` artifact 做独立 post-build 签名验证；install plan 也会在
  `desktop-installer` 未通过 read-only Authenticode 验证时 fail-closed，且 installer
  安装路径必须显式请求 `--artifact-kind desktop-installer`，并会在通过 CLI 或
  `NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT` 传入公开 expected signer subject policy 时声明该检查，
  且 installer 签名摘要会输出低敏 `signaturePolicy`；`build:desktop-installer`
  执行后的 artifact 收集只读取选中 `bundle/<target>` 目录并要求
  `desktop-installer` kind，避免 standalone exe 混入 installer manifest；installer plan / builder
  的低敏 execution policy 也会声明 signing profile 读取、显式 expected signer subject policy 检查、artifact
  collection 和 manifest 写入；installer plan 的 signing summary 也会携带低敏
  `signaturePolicy`；当前不宣称已有生产签名 installer。
- Windows desktop 已有显式本地开发签名 smoke：默认 plan-only；只有
  `--execute --allow-local-trust-store --require-valid` 才会签名临时 artifact 副本，
  临时创建 / 信任 / 清理 CurrentUser code-signing certificate，并验证 Authenticode
  `Valid`。2026-06-23 本机运行已得到 `validSignedArtifactCopy=true`，确认临时证书剩余
  `0` 且原 collected artifact hash 未变。该能力只证明本地开发签名机制，不宣称生产签名或
  installer 签名完成。
- Android 已有 WebView shell / bridge / APK 历史产物和 metadata smoke；后续切回时重新加载 toolchain env 或 Docker builder。

## 客户端演示 MVP 收口状态

客户端已经达到以下阶段性标准，当前阶段不再继续追完整产品级客户端：

1. Web / Windows PC 能登录、注册和恢复会话。
2. 能展示好友列表、群聊列表和消息列表。
3. 能发起私聊、进入群聊、发送中文消息，Web / PC 双向不乱码。
4. 能看到 PullInbox、ACK、push 连接状态和失败提示。
5. Windows PC 端能打开并用于局域网演示。

## 下一步优先级

1. 从客户端主线切到后端架构完善与 AI / Agent / RAG 演示主线。
2. 下一模块先做架构分析，再做代码：读取
   `docs/architecture/target-architecture-complete.md`、
   `docs/architecture/target-architecture-ai.md`、`docs/runbook/service-briefs/search-service.md`、
   `memory-service.md`、`retrieval-gateway.md`、`rag-service.md`、`agent-service.md`
   和 `action-executor.md`。
3. 后端 / AI 演示主线已完成 collaborative-memory eval 到 memory-service /
   retrieval-gateway / RAG / Summary / Agent optional live adapter 的第一轮提升，
   且完整 live service-stack gate 已归档低敏报告。
4. 后端 / AI 演示主线优先做：
   `IM 消息 -> search / memory projection -> EvidencePack -> RAG / Agent answer -> approval / audit`。
   retrieval negative / miss adapter、EvidencePack memory graph edge 和 profile evidence 已补齐；
   profile recompute first path、first-stage operator 和 `rules-v0.2` group memory
   extraction 已补齐；RAG-Agent demo runner first path 和真实服务栈 smoke 报告已补齐；
   profile repair batch approval path、Agent action boundary cases、Python memory
   extraction candidate first path、Go-side adapter / ai-eval 接入以及 memory-service
   公开 candidate review / approval / persistence path 已补齐；`loadtest/ragagent`
   与 `rag-agent-demo` adapter 已把 public candidate review 和 temporal update
   纳入 RAG / Agent EvidencePack 断言链路，并已通过真实 service-stack gate 归档；
   profile repair approval、negative gate、group-memory answer / proposal gate 和
   business proposal source-chain gate 均已通过真实 service-stack gate 归档。
   EvidencePack source-chain-aware rerank first pass 已落到 retrieval-gateway：
   多来源 / 跨群 / actor attribution / graph-edge / profile-supporting evidence 会
   增加 `rerank_score`，并纳入 retrieval positive adapter 断言；当前已通过
   focused app / loadtest checks 和真实 service-stack gate
   `ai-eval-service-stack-live-20260624-retrieval-source-chain-rerank-v2`
   归档：4 adapters / 27 cases / 27 passed / 0 failed / 0 skipped，
   `memory_rerank_score=1.29` 高于 single search baseline。显式业务
   mutation 场景必须等业务 adapter 准备好后再扩展。retrieval strategy version
   已推进为 `retrieval-gateway.v1.hybrid-source-vector-rrf-graph-depth<N>`：rerank 在截断 limit
   前按 lexical search、memory event、profile aggregate、source chain、
   vector item、memory graph、actor attribution 和 profile support lane 做 RRF 风格融合，
   并沿 memory graph edge 做可配置 depth 0..3 相邻 memory expansion，默认 depth=1，
   strategy version 记录实际 depth；相邻 memory 继续通过 memory-service 公开
   `GetMemoryEvent` 和当前 memory status 过滤，lookup / visibility / malformed edge
   失败时 fail-closed，非法 depth 配置启动失败；显式 vector retrieval 会通过
   vector-index-service 公开 `SearchVectors` 返回 `VECTOR_ITEM` source。retrieval
   vector backend opt-in live smoke 已通过；search-service PostgreSQL FTS lexical
   backend 和显式 OpenSearch / BM25 candidate backend first paths 已补齐；OpenSearch
   opt-in smoke 入口、service-owned rebuild operator first path 和 mapping drift
   hardening 已补齐；retrieval `source_coverage` 和 provider `provider_coverage`
   矩阵已进入 positive smoke / adapter 侧契约；pgvector、OpenSearch vector 和
   Milvus provider preflight gate 及 readiness matrix 已补齐，但尚未
   在真实 OpenSearch / pgvector 进程上归档通过 provider smoke 报告；后续真实
   OpenSearch 进程 smoke、真实 pgvector /
   Milvus / OpenSearch vector provider smoke 仍保持在
   retrieval-gateway 边界内。
5. 客户端只作为演示入口；除非阻塞上述演示，不继续扩 UI 产品化。
6. Windows release signing / MSI / NSIS installer、完整 Android、完整移动端发布、
   复杂 UI、群管理深水区和真实 media provider 链路全部后置到 backlog。
7. 新发现待办写入 `docs/runbook/remaining-goals.md`，不要把长待办复制回本文件。

## Focused Checks

客户端后续小修优先跑：

```powershell
npm --prefix clients run check:no-toolchain
git diff --check; git diff --cached --check
```

AI / Agent 或后端跨服务模块按影响面选择相关 Go / Python / TypeScript checks；
跨服务、生成代码、migration、service-registry、Docker / compose、安全边界、
提交推送前或用户明确要求时，扩大到完整 `.\tools\check-local.ps1`。

## 硬边界

- 每个新客户端功能先做简短架构分析再编码。
- 客户端 local store 只做缓存 / 离线队列，不成为服务端事实源。
- 客户端不得直接调用内部微服务，不读取任何服务私表。
- PullInbox 是消息展示事实源；WebSocket 只做在线唤醒。
- 不引入隐藏 fallback；依赖、权限、事实源、投影或 provider 不确定时 fail-closed。
- TypeScript 负责三端共享客户端协议、同步核心和 UI；Rust / Kotlin 只做薄平台桥。
- Python 只做 AI worker / eval / 离线工具，不接管客户端主链路或业务事实源。
- 边界变化必须同步 README、service brief、相关 SDD / ADR、current-brief 和 remaining-goals。
- 不回滚用户已有修改。
