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
- `ai-eval-service`：retrieval negative / miss adapter 已补齐；继续扩展
  profile repair batch、Python memory extraction candidate、EvidencePack
  source-chain 和 RAG-Agent demo runner 后续回归 cases。
- `memory-service` / `retrieval-gateway`：memory graph edge、profile aggregate
  evidence、公开 profile recompute first path 和 first-stage profile repair operator
  以及 rules-v0.2 group memory extraction 已进入主链路；继续做 profile repair
  batch / approval、Python memory extraction candidate、结构过滤、BM25 / vector、
  rerank 和 EvidencePack coverage 深化。
- `loadtest/ragagent`：first-stage RAG-Agent demo runner 已提供低敏总报告；
  `rag-agent-demo` 已接入 ai-eval optional service-stack adapter / gate policy /
  service-stack preflight，且真实服务栈 gate 已通过并归档；下一步围绕 profile
  repair batch / approval、Python memory extraction candidate 和更多 Agent action
  boundary cases 扩展该演示路径。
- `rag-service` / `summary-service`：拒答、引用校验、source-ref regression、unsafe
  output fail-closed cases。
- `agent-service`：真实业务动作继续走 policy、skill contract、proposal、approval、
  executor、audit；Agent 不直接写业务库。
- `skill-registry` / `mcp-gateway` / `action-executor`：补 tool contract、risk level、
  tenant allowlist、adapter、rate limit、DLQ / redrive、repair guard。
- Python AI Worker：只输出候选、hash、citation metadata 和低敏 diagnostics。

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
