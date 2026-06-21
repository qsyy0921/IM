# NexusIM Remaining Goals

这份文档只记录还没有完成的工作。当前进度总览见
`development-progress.md`，单服务事实见 `service-briefs/<service>.md`。

## 维护规则

- 新发现待办追加到本文件；完成后删除或改写为下一阶段 hardening。
- 不记录已完成证据，不写长历史，不替代 SDD / ADR / loadtest report。
- 当前 active slice：`client platform MVP foundation`。
- 长期完整架构基线：`docs/architecture/target-architecture-complete.md`。
- 中间件引入和替换规则：`docs/platform/middleware-catalog.md`。
- 生产级 HA、长压、sizing 和完整系统测试暂不作为当前阻塞。

## 当前优先顺序

1. 完成 client platform MVP foundation：PC desktop runtime、Android runtime、
   三端共享 smoke、PC / Android 壳层 logout 入口和平台 local store smoke。
2. 回到 AI / Agent 主线：group memory eval、EvidencePack、Agent 真实业务动作、
   Python AI Worker 候选算法。
3. 继续 product-active 服务：workflow / audit / admin / notification / media /
   vector / model / knowledge / presence / control-plane。
4. 按完整架构补数据平台和中间件 profile，但不抢占当前客户端切片。
5. 9 个既有 IM 服务只回补阻塞 client / AI / product platform 的 P0/P1 或用户点名项。

## Client Platform MVP 未完成

- `clients/desktop`：first-stage TypeScript runtime adapter 和 Tauri runner
  skeleton 已落；`createDesktopClientRuntime` 已能组装 shared `BFFClient` /
  `WebSocketPushTransport` / auth / inbox / send / ack queue；继续跑通
  login、conversation list、PullInbox、send、ACK、push notify。
- Windows packaging：产出本地可安装或可运行的 Windows artifact；不要求生产签名。
  当前本机尚缺 Tauri CLI / `cargo-tauri`，需要先补本地构建前置或通过
  Docker / CI builder 产物链路；可用 `npm --prefix clients run check:build-prereqs`
  检查当前机器状态。
- `clients/android`：first-stage TypeScript runtime adapter 和 Kotlin native
  bridge skeleton 已落；`createAndroidClientRuntime` 已能组装 shared
  `BFFClient` / `WebSocketPushTransport` / auth / inbox / send / ack queue；
  继续产出 unsigned APK，Kotlin 只做薄 bridge，业务协议和 sync core 复用 TypeScript。
- Android packaging：产出本地 unsigned APK，并支持局域网 `api-gateway` /
  `push-gateway` 地址配置。当前本机尚缺 Gradle / Android SDK，且 `java`
  指向 JDK 8；需要 JDK 17+ 和 Android build toolchain，或通过
  Docker / CI builder 产物链路；可用 `npm --prefix clients run check:build-prereqs`
  检查当前机器状态。
- 三端 smoke：Web / PC / Android 都只能连 `api-gateway` 和 `push-gateway`；
  PullInbox 是事实源，WebSocket 只做在线唤醒。
- Local store：`IndexedDBMessageStore` 已有 first-stage persistence test；
  desktop / Android 已默认接 shared `KeyValueMessageStore` + WebView
  `localStorage` first-stage durable adapter，并有 cursor replay test；后续在
  native packaging/runtime ready 后替换为 SQLite bridge。
- Auth lifecycle：BFF `/api/auth/logout`、shared runtime logout 编排和 Web
  logout local cleanup 已落；后续在 PC / Android 真实 runtime shell 中接入
  logout 入口并跑平台 smoke。

## AI / Agent Platform 未完成

- `memory-service`：继续 group / collaborative memory 的 source refs、speaker /
  audience scope、validity、supersession、confidence、review state 和 profile
  aggregation。
- `ai-eval-service` / eval harness：继续增加低敏 case，区分 retrieval failure、
  reasoning failure、memory lifecycle failure 和 action boundary failure。
- `retrieval-gateway`：继续结构过滤、BM25 / vector / graph expansion、rerank、
  EvidencePack 覆盖率和 source coverage 口径。
- `rag-service` / `summary-service`：继续拒答、引用校验、source-ref regression、
  provider fallback 和 unsafe output fail-closed cases。
- `agent-service`：真实业务动作继续走
  `policy -> skill contract -> prepare audit -> proposal -> approval -> executor -> audit`；
  不允许 Agent 直接写业务库。
- `skill-registry` / `mcp-gateway`：补真实 MCP/provider tool 接入前的契约版本、
  risk level、tenant allowlist、audit 和 denial reason。
- `action-executor`：继续外部 adapter、rate limit、DLQ/redrive、repair guard 和
  low-sensitive result projection。
- Python AI Worker：扩展 embedding / rerank / memory extraction / planner / eval
  候选算法；输出只能是候选、hash、citation metadata 和低敏 diagnostics，Go 继续拥有
  权限、状态和持久化。

## Product-Active 服务未完成

- `media-service`：S3-compatible adapter、scanner、thumbnail / transcode provider、
  download policy、retention / delete proof。
- `notification-service`：SMTP / SMS / APNs / FCM adapter、bounce / suppression、
  provider redrive / audit、tenant template policy。
- `audit-service`：更多 Kafka ingestion source、持久 ingestion checkpoint / rewind
  operator、export worker / manifest、SIEM forwarding、retention cleanup、segment
  sealing、provider-grade audit export。
- `admin-service`：admin UI、更多下游公开 API adapter、更多 compensation adapter、
  provider-grade instruction 审批 / UI。
- `control-plane-service`：outbox relay、drift monitor、expiry / cleanup worker、
  api-gateway quota consumer、provider-grade rollout。
- `presence-service`：push-gateway session event consumer、`SubscribePresence`、
  stale scanner、presence outbox relay、Redis hot-state、privacy / contacts policy。
- `model-gateway`：真实 OpenAI / Claude / local-model provider、真实 embedding、
  rerank、outbox relay、route-refresh、budget-reset、cleanup worker。
- `knowledge-ingestion-service`：parser worker、tombstone / delete proof、真实 connector、
  parser / crawler provider handoff、ingestion repair。
- `workflow-service`：timer worker、更多 compensation adapter、instruction approval
  UI、external approval binding、external callback wait、outbox relay、repair
  operators。
- `vector-index-service`：memory / search chunk consumer、pgvector smoke、Milvus /
  OpenSearch backend、provider repair、真 provider backfill smoke。

## 数据平台 / 中间件平台未完成

- 数据平台：CDC / ingestion、lakehouse、OLAP、data catalog、metrics、feature store
  仍是目标架构能力，尚未作为 active implementation 切片推进。
- 中间件 profile：需要按 `deploy/local/` profile 拆 core、client-demo、
  observability、search-rag、media、workflow-agent、security、data-platform、
  ai-runtime；不要默认启动所有中间件。
- 中间件登记：新增 Redis / Kafka / PostgreSQL / OpenSearch / vector store /
  MinIO / Vault / Keycloak / OpenFGA / Temporal / OTel 等能力前，先登记 capability、
  owner、source-of-truth、runtime profile、健康检查、最小 smoke、迁移和安全边界。
- Adapter 边界：中间件 adapter 只能落在对应服务的
  `internal/infrastructure/<adapter>/`；domain / app 层不能 import 具体中间件 client。
- 数据平台只消费公开事件 / CDC 构建分析和 AI 数据资产，不能成为业务事实写入口。

## 9 个现有 IM 服务必要回补

| 服务 | 未完成工作 |
| --- | --- |
| `api-gateway` | legacy observation evidence、provider-grade 配置中心 quota、灰度治理、生产观测。 |
| `identity-service` | WebAuthn/passkeys、OIDC、多 issuer、KMS/HSM、完整风控、生产级 email/SMS provider。 |
| `message-service` | 删除 / 撤回 / 编辑深化、外部 proof workflow、发送链路生产观测。 |
| `conversation-service` | 群管理、owner transfer、历史窗口 / targeted replay repair。 |
| `delivery-service` | 更多 delivery event 消费方、projection repair、容量曲线。 |
| `push-gateway` | Redis HA、跨实例 resume、长时间在线容量曲线。 |
| `receipt-service` | 会话列表产品能力、更多摘要策略和容量曲线。 |
| `contacts-service` | 组织级策略、租户默认值、来源策略、隐私例外接入 admin/config。 |
| `policy-service` | provider-grade ReBAC / DSL、moderation / risk scoring、tenant quota、外部 audit pipeline。 |

## 后置生产化 Hardening

- 生产级统一观测：collector、Alertmanager、日志汇聚、SLO、retention。
- 分布式 HA / 故障演练：Redis / Kafka / PostgreSQL 更长时长和多故障组合。
- Repair / DLQ / audit 产品化：审批系统、运维 UI、批量 repair、外部审计。
- 容量和复杂度治理：9 服务 `capacity_summary` 长压 campaign、资源曲线、
  生产 sizing、文件拆分。

## 并行开发规则

可使用多个 sub-agent 并行推进，但必须按服务、文档集、测试面或只读审查问题拆分。
禁止同时改同一 proto、migration、service brief、架构章节或同一客户端 package。
