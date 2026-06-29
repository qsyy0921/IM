# NexusIM Remaining Goals

只记录未完成工作。当前进度见 `development-progress.md`，单服务事实见
`service-briefs/<service>.md`，完整架构见
`docs/architecture/target-architecture-complete.md`。

## 维护规则

- 新发现待办追加到本文件；完成后删除或改写为下一阶段 hardening。
- 不写长历史、完成证据、SDD / ADR 正文或 loadtest report。
- 新功能先做架构分析；新增服务 / 中间件 / provider 时同步 README、目标架构、
  service brief、相关 SDD / ADR、runtime profile 和本文件。
- 隐藏 fallback 按 `docs/architecture/fail-closed-policy.md` 治理；触达的
  fallback-like 代码必须清理或记录 owner、范围和风险。

## 当前优先顺序

1. Hot group fanout / sequencer / projection hardening：小规模 Docker 热点群 smoke 已通过；
   下一步扩大三机压测规模，补趋势图和瓶颈曲线归档。
2. Agent action boundary / repair cases：在 provider replay admin / workflow handoff 已落
   的基础上，继续扩更多需要 proposal / approval / workflow / audit 的 action 与 repair 场景。
3. Product-active 服务按需推进：workflow、audit、admin、notification、media、vector、
   model、knowledge、presence、control-plane。
4. 数据平台和中间件 profile 按完整架构逐步补，不抢占 AI / Agent 演示主线。
5. 10 个运行链路服务（9 个既有 IM 服务 + `timeline-service` seq-block allocator）只回补阻塞
   AI / product platform、热点群压测或用户点名项的 P0/P1。
6. 客户端只作为演示入口维护；除非阻塞演示，不继续扩完整产品级客户端。
7. 热点群 / 分区主线已落 conversation-service scale policy、delivery hybrid/read fanout、
   conversation-level delivery signal first-stage runtime、push-gateway conversation
   subscription / signal broadcast、timeline-service seq-block allocator、message-service
   active `SEQUENCER_BLOCK` 单条 seq block 写路径和 hotgroup runner；2026-06-29 已跑通
   61 人 / 20 消息 / 3 WebSocket subscriber 小规模 smoke，下一步扩大压测规模，并继续做
   block cache / gap marker / epoch fencing / deeper repair。

## Client Demo Backlog

- Web / PC shell：演示 MVP 已达标；后续只修阻塞 AI / Agent 演示入口的问题。
- Windows PC：只要求本地 shell 能打开并演示；release signing / installer 后置。
- Android：后续切回时重新加载 F 盘 toolchain env 或显式 Docker builder，再跑 APK /
  WebView login smoke。
- 入群审批 / 禁言、复杂群管理、完整媒体 UX、移动端体验、SQLite native store 后置。
- 生产 Web 鉴权后续切 httpOnly cookie / provider-grade session 策略。

## AI / Agent Platform

- `search-service`：真实 OpenSearch 进程 smoke、容量曲线、provider-grade 运维。
- `memory-service`：结构过滤、BM25 / vector、rerank、repair audit、更多 group-memory eval。
- `retrieval-gateway`：真实 OpenSearch、pgvector、Milvus provider smoke 和 coverage 深化。
- `rag-service` / `summary-service`：继续扩展 multi-hop / temporal / profile eval、
  provider-specific regression 和更完整 unsafe output cases。
- `agent-service`：proposal risk policy、instruction approval UI、更多真实业务
  proposal 场景。
- `skill-registry` / `mcp-gateway`：tool contract、risk level、tenant allowlist、adapter、
  rate limit。
- `action-executor`：provider replay admin / workflow handoff、review/readiness/redrive
  operator artifacts、external audit append 和 audit result manifest 已落；后续做更多
  action boundary / repair cases 和 provider-grade replay UI。
- `ai-eval-service`：group-memory asker-bound term ambiguity、visible-chain incomplete
  abstention、missing visibility projection fail-closed、audience-language profile negative
  cases 已进入本地低敏 gate；后续继续扩 provider readiness、Agent action boundary 和
  redrive / repair cases。
- Python AI Worker：继续保持 candidate-only；更多 memory extraction、planner、rerank 和
  eval 候选算法。

## Product-Active Services

- `media-service`：真实 S3-compatible adapter、scanner、thumbnail / transcode provider、
  CDN / download policy、retention / delete proof。
- `notification-service`：SMTP / SMS / APNs / FCM adapter、bounce / suppression、
  provider redrive / audit、tenant template policy。
- `audit-service`：action-executor external audit append operator 已通过公开
  `AppendAuditRecord` 接入第一版低敏 operator 追加路径；后续补更多 Kafka ingestion
  source、checkpoint / rewind、export worker、SIEM forwarding、retention cleanup、
  segment sealing。
- `admin-service`：admin UI、provider-grade provider replay request UI、更多下游公开 API
  adapter、compensation adapter、instruction approval UI。
- `control-plane-service`：outbox relay、drift monitor、expiry / cleanup worker、
  api-gateway quota consumer、provider-grade rollout。
- `presence-service`：push-gateway session consumer、`SubscribePresence`、stale scanner、
  outbox relay、Redis hot-state、privacy / contacts policy。
- `model-gateway`：provider routing、budget、fallback policy as explicit config、audit。
- `knowledge-ingestion-service`：file/web imports、chunking pipeline、PII scan、rebuild jobs。
- `vector-index-service`：real pgvector / OpenSearch vector / Milvus smoke、provider repair。
- `workflow-service`：external callback delivery / redrive、approval queue review / batch
  decision、compensation review / execution artifacts、audit append handoff 和 workflow outbox
  relay first path 已落；后续继续补 workflow outbox relay smoke、更多 compensation adapter、
  callback delivery provider-grade persisted dashboard / provider-grade approval platform。

## 核心 IM 运行链路 P2

- `api-gateway`：legacy observation evidence、provider-grade quota、gray rollout、OTel stack。
- Cross-service loadtest：继续维护 `capacity_summary`，形成容量曲线和瓶颈说明；新增
  `loadtest/hotgroup` 业务压测 runner，覆盖热点群聊 fanout、Kafka lag、delivery
  projection、push notify storm、PullInbox / ACK 追平、成员变更和故障恢复，不用单接口
  QPS 替代真实业务链路；正式压测必须配套 Prometheus / Grafana 趋势图。当前已新增
  `NexusIM Hot Group Loadtest` first-stage dashboard，后续还需补 fanout-mode distribution、
  Kafka consumer lag、delivery timeline item insert rate、inbox rows per message 和
  PostgreSQL lock / WAL / dead tuple time-series exporter。
- `identity-service`：WebAuthn/passkeys、OIDC、多 issuer、KMS/HSM、完整风控、生产级
  email/SMS provider。
- `message-service`：删除 / 撤回 / 编辑深化、外部 proof workflow、发送链路生产观测。
- `message-service`：`SEQUENCER_BLOCK` 已接 timeline-service 单条 seq block active 写路径；
  61 人热点群小规模 smoke 已通过；后续补本地 block cache、gap marker、epoch fencing
  观测和发送链路生产观测；删除 / 撤回 / 编辑深化、外部 proof workflow 后置。
- `conversation-service`：群规模策略已进入 domain 层，medium / large 策略已转 active
  first-stage；hot group 策略已与 message-service `SEQUENCER_BLOCK` 单条 seq block
  active 写路径和 timeline lease 绑定；后续继续补 control-plane rollout、群管理深化、
  历史窗口 / targeted replay repair。
- `timeline-service`：已进入本地运行链路的 seq-block allocator，具备 PostgreSQL
  sequence state / block lease、`AllocateSeqBlock` gRPC API、Docker / Prometheus /
  Grafana 观测；message-service 已只在 valid block lease 下取号；下一步补 block cache、
  sequencer epoch fencing、gap marker、virtual partition mapping 和 repair operator。
- `delivery-service`：projection DLQ / repair 深化、更多 delivery event consumer；
  `WRITE_FANOUT`、`HYBRID_FANOUT`、`READ_FANOUT` 和 conversation-level signal 已有
  first-stage runtime，materialized `user_inbox` 已改成批量 insert，`timeline-consumer`
  已支持按 Kafka partition 安全并行的 multi-worker runtime；后续补 timeline item repair、动态 read
  fanout 容量曲线，重点验证 Kafka lag、projection backlog、PullInbox / ACK 追平时间
  和 push notify storm；delivery outbox relay SQL / worker / Kafka batch hardening 已落，
  2026-06-28 hotgroup QPS step 已证明 `delivery_outbox -> Kafka im.delivery.events`
  不再是 100 人群 150 QPS 内的首个瓶颈；下一步转向 delivery timeline projection /
  single hot conversation fanout 策略、projection lag metrics、inbox rows per message metrics 和
  WebSocket notify storm 覆盖。
- `push-gateway`：conversation subscribe / unsubscribe 与 conversation signal fanout
  已进入服务端 first path；hotgroup runner 已验证 3 个 WebSocket subscriber 共收到
  60 条 conversation signal；后续补 Redis 网络分区 smoke、跨实例 resume、容量测试。
- `receipt-service`：会话列表产品能力、更多摘要策略和容量曲线。
- `contacts-service`：组织级策略、租户默认值、来源策略、隐私例外。
- `policy-service`：provider-grade ReBAC / DSL、moderation / risk scoring、tenant quota、
  external audit pipeline。
