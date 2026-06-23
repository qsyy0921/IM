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

1. Client platform MVP foundation：继续 Web / Windows PC，Android 后置到用户切回。
2. 客户端阶段收口后回到 AI / Agent：group memory、EvidencePack、真实业务动作、
   Python AI Worker 候选算法。
3. Product-active 服务按需推进：workflow、audit、admin、notification、media、
   vector、model、knowledge、presence、control-plane。
4. 数据平台和中间件 profile 按完整架构逐步补，不抢占客户端切片。
5. 9 个既有 IM 服务只回补阻塞 client / AI / product platform 的 P0/P1 或用户点名项。

## Client Platform MVP

- Windows PC：继续真实 signing input、valid signed artifact、MSI / NSIS installer、
  signed installer experience；当前 `signtool` 可定位，但仍缺代码签名证书和 valid
  Authenticode signature。
- Web / PC shell：继续更丰富群设置，把 `plan:browser-multiuser-ui-smoke`
  低敏计划推进成真实浏览器 / PC UI run，继续群头像上传链路；头像上传需要后续接
  media-service，不得直接写 conversation-service 私表。
- Desktop runtime：继续真实 UI lifecycle 和 installer / signing 流水线；portable zip
  与 install plan 已有 first-stage 本地路径。
- Android：后续切回时重新加载 F 盘 toolchain env 或显式 Docker builder，再跑 APK /
  WebView login smoke；不要在当前状态宣称 Android build readiness。
- Local store：后续 native packaging/runtime ready 后把 desktop / Android store 替换为
  SQLite bridge。
- Web hardening：生产 Web 鉴权后续切 httpOnly cookie / provider-grade session 策略。

## AI / Agent Platform

- `search-service`：继续 index projection、visibility filtering、query hardening 和 AI retrieval substrate；`memory-service`：group / collaborative memory 的 source refs、scope、validity、
  supersession、confidence、review state、profile aggregation。
- `ai-eval-service`：区分 retrieval failure、reasoning failure、memory lifecycle
  failure、action boundary failure。
- `retrieval-gateway`：结构过滤、BM25 / vector / graph expansion、rerank、
  EvidencePack coverage。
- `rag-service` / `summary-service`：拒答、引用校验、source-ref regression、unsafe
  output fail-closed cases。
- `agent-service`：真实业务动作继续走 policy、skill contract、proposal、approval、
  executor、audit；Agent 不直接写业务库。
- `skill-registry` / `mcp-gateway` / `action-executor`：补 tool contract、risk level、
  tenant allowlist、adapter、rate limit、DLQ / redrive、repair guard。
- Python AI Worker：只输出候选、hash、citation metadata 和低敏 diagnostics。

## Product-Active Services

- `media-service`：S3-compatible adapter、scanner、thumbnail / transcode provider、
  download policy、retention / delete proof。
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
