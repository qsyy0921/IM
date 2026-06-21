# NexusIM Remaining Goals

这份文档只记录还没有完成的工作。当前进度总览见
`development-progress.md`，单服务事实见 `service-briefs/<service>.md`。

维护规则：

- 新发现待办追加到本文件；已完成后移除或改写为后续项。
- 不记录已完成证据，不写长历史，不替代 SDD / ADR。
- 当前 active slice：client platform MVP foundation。
- 长期完整架构基线：`docs/architecture/target-architecture-complete.md`。
- 生产级 HA、长压、sizing 和完整系统测试暂不作为当前阻塞。

## 当前未完成重点

0. Client platform first slice：`api-gateway` client BFF first-stage HTTP/JSON
   surface 已落，`clients/web` 的真实 BFF fetch adapter、push WebSocket adapter
   和 IndexedDB local store 已接入；`loadtest/clientweb` 已提供脚本化 BFF + push
   client-path smoke runner 和本地私有启动脚本；第一轮本地 Web MVP smoke 已通过
   并归档，提交后的 clean baseline 也已通过并归档；runner 已支持
   `-BindHost` / `-ClientHost`，第一轮 `172.31.50.1` wired-address WIP smoke 和
   clean committed baseline 均已通过并归档；BFF HTTP-layer route metrics /
   rate-limit adapter 已落。剩余：接 PC desktop Tauri runner 和 Android runtime
   shell，产出本地 Windows installer / unsigned APK；
   客户端只连 `api-gateway` / `push-gateway`，不能直连内部服务。`/api/auth/logout`
   仍等待 identity user self-session revoke 契约。
1. AI eval 回归扩展：继续增加低敏 case，区分 retrieval、reasoning、action
   boundary 和 memory lifecycle failure。
2. Memory / retrieval 深化：继续 group / collaborative memory 的 source refs、
   speaker / audience scope、validity、supersession、confidence、review state。
3. Agent 真实业务动作扩展：真实 MCP / provider tool / 业务写动作必须走
   policy -> skill contract -> prepare audit -> proposal -> approval -> executor -> audit。
4. Python AI Worker 扩展：embedding / rerank / memory extraction / planner / eval
   只能返回候选和 hash / citation metadata；Go 拥有权限、状态和持久化。

Active AI foundation backlog 覆盖：`action-executor`、`agent-service`、
`ai-eval-service`、`mcp-gateway`、`memory-service`、`rag-service`、
`retrieval-gateway`、`search-service`、`skill-registry`、`summary-service`。

## 平台能力 / 中间件待办

- 新增服务、数据平台能力、AI / Agent 能力或中间件前，先确认是否符合完整架构的
  service split / data ownership / security / event boundary；不符合时先补 ADR / SDD。
- 后续新增中间件前，先按 `docs/platform/middleware-catalog.md` 登记 capability、
  source-of-truth 语义、依赖服务、runtime profile、健康检查、最小 smoke、替换 /
  迁移策略和安全边界。
- 需要按 profile 拆 `deploy/local/`：core、client-demo、observability、search-rag、
  media、workflow-agent、security、data-platform、ai-runtime；不要默认启动所有
  中间件。
- 中间件 adapter 只能落在对应服务的 `internal/infrastructure/<adapter>/`；domain /
  app 层不能 import Kafka、Redis、OpenSearch、Milvus、MinIO、Temporal、Vault 等
  具体 client。
- 数据平台中间件只能消费公开事件 / CDC 构建分析和 AI 数据资产，不能成为业务事实
  写入口。

## 9 个现有服务必要收口

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

## Product-active 平台 / 产品化服务

- `media-service`：S3-compatible adapter、scanner、thumbnail / transcode provider、
  download policy。
- `notification-service`：SMTP / SMS / APNs / FCM adapter、bounce / suppression、
  provider redrive / audit。
- `audit-service`：更多 Kafka ingestion source、持久 ingestion checkpoint / rewind
  operator、export worker / manifest、SIEM forwarding、retention cleanup、segment
  sealing、provider-grade audit export。
- `admin-service`：admin UI、更多下游公开 API adapter、更多
  compensation adapter、provider-grade instruction 审批 / UI。
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

## 后置平台 / 产品化服务

新增服务必须满足独立模型 / 伸缩 / 故障 / 安全边界或明显降复杂度，并通过
ADR / SDD v0.1。

## 后置 Hardening

- 生产级统一观测：collector、Alertmanager、日志汇聚、SLO、retention。
- 分布式 HA / 故障演练：Redis / Kafka / PostgreSQL 更长时长和多故障组合。
- Repair / DLQ / audit 产品化：审批系统、运维 UI、批量 repair、外部审计。
- 容量和复杂度治理：9 服务 `capacity_summary` 长压 campaign、资源曲线、生产 sizing、文件拆分。

可使用多个 sub-agent 并行推进，但必须按服务、文档集、测试面或只读审查问题拆分；
禁止同时改同一 proto、migration、service brief 或架构章节。
