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

1. Agent action boundary / repair cases：在 provider replay admin / workflow handoff 已落
   的基础上，继续扩更多需要 proposal / approval / workflow / audit 的 action 与 repair 场景。
2. Product-active 服务按需推进：workflow、audit、admin、notification、media、vector、
   model、knowledge、presence、control-plane。
3. 数据平台和中间件 profile 按完整架构逐步补，不抢占 AI / Agent 演示主线。
4. 9 个既有 IM 服务只回补阻塞 AI / product platform 的 P0/P1 或用户点名项。
5. 客户端只作为演示入口维护；除非阻塞演示，不继续扩完整产品级客户端。

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
- `action-executor`：provider replay admin / workflow handoff 已落；后续再做更多
  action boundary / repair cases、external audit integration 和 provider-grade replay UI。
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
- `audit-service`：更多 Kafka ingestion source、checkpoint / rewind、export worker、
  SIEM forwarding、retention cleanup、segment sealing。
- `admin-service`：admin UI、provider-grade provider replay request UI、更多下游公开 API
  adapter、compensation adapter、instruction approval UI。
- `control-plane-service`：outbox relay、drift monitor、expiry / cleanup worker、
  api-gateway quota consumer、provider-grade rollout。
- `presence-service`：push-gateway session consumer、`SubscribePresence`、stale scanner、
  outbox relay、Redis hot-state、privacy / contacts policy。
- `model-gateway`：provider routing、budget、fallback policy as explicit config、audit。
- `knowledge-ingestion-service`：file/web imports、chunking pipeline、PII scan、rebuild jobs。
- `vector-index-service`：real pgvector / OpenSearch vector / Milvus smoke、provider repair。
- `workflow-service`：compensation review bundle / review page 已落；后续继续补更多
  compensation adapter、provider-grade instruction / approval UI、external callback delivery /
  retry hardening。

## 9 个核心 IM 服务 P2

- `api-gateway`：legacy observation evidence、provider-grade quota、gray rollout、OTel stack。
- Cross-service loadtest：继续维护 `capacity_summary`，形成容量曲线和瓶颈说明。
- `identity-service`：WebAuthn/passkeys、OIDC、多 issuer、KMS/HSM、完整风控、生产级
  email/SMS provider。
- `message-service`：删除 / 撤回 / 编辑深化、外部 proof workflow、发送链路生产观测。
- `conversation-service`：群管理深化、历史窗口 / targeted replay repair。
- `delivery-service`：projection DLQ / repair 深化、更多 delivery event consumer。
- `push-gateway`：Redis 网络分区 smoke、跨实例 resume、容量测试。
- `receipt-service`：会话列表产品能力、更多摘要策略和容量曲线。
- `contacts-service`：组织级策略、租户默认值、来源策略、隐私例外。
- `policy-service`：provider-grade ReBAC / DSL、moderation / risk scoring、tenant quota、
  external audit pipeline。
