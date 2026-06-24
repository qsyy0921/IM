# NexusIM Remaining Goals

只记录未完成工作。当前进度见 `development-progress.md`，单服务事实见
`service-briefs/<service>.md`，完整架构见
`docs/architecture/target-architecture-complete.md`。

## 维护规则

- 新发现待办追加到本文件；完成后删除或改写为下一阶段 hardening。
- 不写长历史、完成证据、SDD / ADR 正文或 loadtest report。
- 新功能先做架构分析；若新增服务 / 中间件 / provider，同步 README、目标架构、
  service brief、相关 SDD / ADR、runtime profile 和本文件。
- 中间件归中间件平台；数据处理归数据平台；模型 / 检索 / Agent / Python worker 归 AI / Agent 平台；业务能力归业务 / 产品平台；客户端交互归客户端平台。
- 隐藏 fallback 按 `docs/architecture/fail-closed-policy.md` 治理；当前切片触达的
  fallback-like 代码必须清理或在本文件记录 owner、文件范围和风险。

## 当前优先顺序

1. AI / Agent demo path：group memory、EvidencePack、真实业务动作、
   Python AI Worker 候选算法和 eval gate。
2. Product-active 服务按需推进：workflow、audit、admin、notification、media、
   vector、model、knowledge、presence、control-plane。
3. 数据平台和中间件 profile 按完整架构逐步补，不抢占 AI / Agent 演示主线。
4. 9 个既有 IM 服务只回补阻塞 AI / product platform 的 P0/P1 或用户点名项。
5. 客户端只作为演示入口维护；除非阻塞演示，不继续扩完整产品级客户端。

## Client Demo Backlog

- Web / PC shell：演示 MVP 已达标。后续只修阻塞 AI / Agent 演示入口的问题。
- Windows PC：只要求本地 PC shell 能打开并演示；release signing、MSI / NSIS installer、
  signed installer experience 不作为当前主线。
- Android：后续切回时重新加载 F 盘 toolchain env 或显式 Docker builder，再跑 APK /
  WebView login smoke；不要在当前状态宣称 Android build readiness。
- Web / PC shell 的入群审批 / 禁言、复杂群管理、完整媒体 UX 和移动端体验后置。
- Local store：后续 native packaging/runtime ready 后把 desktop / Android store 替换为
  SQLite bridge。
- Web hardening：生产 Web 鉴权后续切 httpOnly cookie / provider-grade session 策略。

## AI / Agent Platform

- `search-service`：继续 index projection、visibility filtering、query hardening 和 AI retrieval substrate；`memory-service`：group / collaborative memory 的 source refs、scope、validity、
  supersession、confidence、review state、profile aggregation。
- `ai-eval-service`：已用低敏 fixture 覆盖 multi-hop actor chain、temporal version、
  profile aggregation 正 / 负路径和 profile delete propagation；下一步把这些要求接到
  memory-service / retrieval / RAG / Agent live stack adapter，并继续区分 retrieval failure、
  reasoning failure、memory lifecycle failure、action boundary failure。
- `ai-eval-service`：retrieval negative / miss adapter 已补齐；memory-service public
  candidate review 已进入 live adapter case；继续扩展 EvidencePack source-chain
  和 RAG-Agent demo runner 后续回归 cases。
- `memory-service` / `retrieval-gateway`：memory graph edge、profile aggregate
  evidence、公开 profile recompute first path 和 first-stage profile repair operator
  以及 profile repair batch approval path、rules-v0.2 group memory extraction
  已进入主链路；Python memory extraction candidate first path 已输出 hash-only
  candidates，Go-side adapter / eval 接入和公开 candidate review / approval /
  persistence path 已落；继续做真实 service-stack 归档、结构过滤、BM25 / vector、
  rerank 和 EvidencePack coverage 深化。
- `loadtest/ragagent`：first-stage RAG-Agent demo runner 已提供低敏总报告；
  `rag-agent-demo` 已接入 ai-eval optional service-stack adapter / gate policy /
  service-stack preflight，且真实服务栈 gate 已通过并归档；public candidate
  review 现在也会通过 memory-service 公开 submit / review API 写入
  `ACTIVE + APPROVED` memory，并被 RAG / Agent EvidencePack 断言消费；该断言已通过
  `ai-eval-rag-agent-demo-live-20260624-public-candidate-review-v3` 真实 gate 归档。
  public candidate replacement temporal update 也已通过
  `ai-eval-rag-agent-demo-live-20260624-temporal-update-v2` 真实 gate 归档：replacement
  审批后 supersede 旧 memory，当前 RAG / Agent EvidencePack 只保留 active replacement。
  profile repair approval 已进入 `loadtest/ragagent` 组合断言并通过
  `ai-eval-rag-agent-demo-live-20260624-profile-repair-approval-v3` 真实 gate 归档：
  公开 candidate review
  写入 `PROFILE_SIGNAL` 后，必须经 workflow-service `REPAIR_APPROVAL` 审批才执行
  `memoryprofile` batch recompute，并要求修复后的 profile aggregate 同时进入 RAG /
  Agent EvidencePack。profile repair negative gate 已接入 runner / adapter contract：
  未审批 workflow 和 approval payload hash mismatch 均必须 fail-closed；该负向门禁
  已通过 `ai-eval-rag-agent-demo-live-20260624-profile-repair-negative-v1` 真实 gate
  归档。group-memory answer / proposal 场景已通过
  `ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1` 真实 gate
  归档：`DECISION` / `BLOCKER` / `FILE` 三类 group memory 同时进入 RAG / Agent
  EvidencePack。真实业务 proposal source-chain 场景已通过
  `ai-eval-rag-agent-demo-live-20260624-business-proposal-source-chain-gate-v1` 真实 gate
  归档：`DECISION` / `TASK` / `STATUS` 三类 reviewed memory 驱动
  `conversation.note.create` proposal，并经 approval / action-executor audit 记录，
  未配置真实 mutation adapter 时不执行业务写动作。EvidencePack source-chain-aware
  rerank first pass 已进入 retrieval-gateway，并已通过
  `ai-eval-service-stack-live-20260624-retrieval-source-chain-rerank-v2`
  真实 service-stack gate；`retrieval-gateway.v1.hybrid-source-vector-rrf-graph-depth<N>` 已补
  lane-aware RRF 风格融合边界、可配置 graph expansion depth 0..3 和
  vector-index-service `SearchVectors` adapter first path；retrieval vector backend opt-in
  live smoke 已通过，证明 refs-only `VECTOR_ITEM` 能经真实 vector-index-service gRPC
  进入 EvidencePack。search-service PostgreSQL FTS lexical backend first path 已落，
  不再使用 substring fallback；显式 OpenSearch / BM25 candidate backend first path 已落，
  不会绕过 PostgreSQL visibility / tombstone hydration；OpenSearch opt-in backend
  smoke 入口和 service-owned rebuild operator first path 已补齐但本机尚未归档真实
  OpenSearch 进程通过报告；下一步继续补真实 OpenSearch 进程 smoke、mapping hardening、
  pgvector / Milvus / OpenSearch vector provider smoke
  与 EvidencePack coverage。
  真实 mutation 场景必须等显式业务 adapter 和
  operator policy 就绪。
- `rag-service` / `summary-service`：拒答、引用校验、source-ref regression、unsafe
  output fail-closed cases。
- `agent-service`：真实业务动作继续走 policy、skill contract、proposal、approval、
  executor、audit；Agent 不直接写业务库。
- `skill-registry` / `mcp-gateway` / `action-executor`：补 tool contract、risk level、
  tenant allowlist、adapter、rate limit、DLQ / redrive、repair guard；Agent action
  approval / prepared-audit / resource binding mismatch 已进入 preflight safety eval。
- Python AI Worker：只输出候选、hash、citation metadata 和低敏 diagnostics；
  memory extraction candidate first path 和 Go-side adapter / eval gate 已落；
  后续只通过 Go-owned review / approval / memory-service 持久化路径进入最终 memory，
  不直接持久化最终 memory；该公开 review path 已落地，后续补真实服务栈归档和
  RAG-Agent 演示消费。

## Product-Active Services

- `media-service`：真实 S3-compatible adapter、scanner、thumbnail / transcode provider、
  CDN / download policy、retention / delete proof。
- `notification-service`：SMTP / SMS / APNs / FCM adapter、bounce / suppression、
  provider redrive / audit、tenant template policy。
- `audit-service`：更多 Kafka ingestion source、checkpoint / rewind、export worker、
  SIEM forwarding、retention cleanup、segment sealing。
- `admin-service`：admin UI、更多下游公开 API adapter、compensation adapter、
  instruction approval UI。
- `control-plane-service`：outbox relay、drift monitor、expiry / cleanup worker、
  api-gateway quota consumer、provider-grade rollout。
- `presence-service`：push-gateway session consumer、`SubscribePresence`、stale scanner、
  outbox relay、Redis hot-state、privacy / contacts policy。
- `model-gateway`、`knowledge-ingestion-service`、`workflow-service`、
  `vector-index-service`：补真实 provider / worker / repair / backfill smoke。

## 9 个现有 IM 服务必要回补

| 服务 | 未完成工作 |
| --- | --- |
| `api-gateway` | legacy observation evidence、provider-grade 配置中心 quota、灰度治理、生产观测。 |
| `identity-service` | WebAuthn/passkeys、OIDC、多 issuer、KMS/HSM、完整风控、生产级 email/SMS provider。 |
| `message-service` | 删除 / 撤回 / 编辑深化、外部 proof workflow、发送链路生产观测。 |
| `conversation-service` | 群管理深化、历史窗口 / targeted replay repair。 |
| `delivery-service` | 更多 delivery event 消费方、projection repair、容量曲线。 |
| `push-gateway` | Redis HA、跨实例 resume、长时间在线容量曲线。 |
| `receipt-service` | 会话列表产品能力、更多摘要策略和容量曲线。 |
| `contacts-service` | 组织级策略、租户默认值、来源策略、隐私例外接入 admin/config。 |
| `policy-service` | provider-grade ReBAC / DSL、moderation / risk scoring、tenant quota、外部 audit pipeline。 |

## 数据平台 / 中间件 / 后置生产化

- 数据平台：CDC / ingestion、lakehouse、OLAP、catalog、metrics、feature store。
- 中间件 profile：按 core、client-demo、observability、search-rag、media、
  workflow-agent、security、data-platform、ai-runtime 拆，不默认启动全部中间件。
- Adapter 边界：中间件 client 只能在对应服务 `internal/infrastructure/<adapter>/`。
- 生产级观测、分布式 HA、`capacity_summary` 长压 sizing、repair / DLQ / audit 产品化均后置。

## 并行开发规则

可用多个 sub-agent，但必须按服务、文档集、测试面或只读审查问题拆分；禁止并发改同一
proto、migration、service brief、架构章节或同一客户端 package。
