# NexusIM Remaining Goals

这份文档只记录当前还没有完成的工作。当前进度总览见
`development-progress.md`，单服务事实见 `service-briefs/<service>.md`。

维护规则：

- 新发现的待完成工作追加到本文件。
- 已完成工作从本文件移除，并同步到 service brief / progress / smoke report。
- 不记录已完成证据，不写长历史，不替代 SDD / ADR。

## 当前默认主线

当前 active slice 是 future platform / product services promotion。AI 底座和
9 个既有 IM 服务只做阻塞该主线的必要收口；生产级 HA、长压、sizing 和完整系统测试
暂不作为当前阻塞。
Foundation backlog 锚点：`search-service`、`memory-service`、`retrieval-gateway`、
`rag-service`、`summary-service`、`agent-service`、`skill-registry`、`mcp-gateway`、
`action-executor`、`ai-eval-service`。

## 当前未完成重点

1. AI eval 回归扩展：
   继续扩展低敏 case，区分 retrieval failure、reasoning failure、action
   boundary failure 和 memory lifecycle failure。不得保存 raw prompt、
   EvidencePack、model output、用户正文、secret 或 tool input。

2. Memory / retrieval 深化：
   `memory-service` 继续按 group / collaborative memory 论文方向深化
   source refs、speaker / audience scope、valid_from / valid_to、
   supersedes / contradicts、confidence、PENDING / ACTIVE / SUPERSEDED /
   REJECTED 状态。优先 multi-hop、temporal update、profile aggregation，
   保留 visibility、review state 和 source-ref 边界。

3. Agent 真实业务动作扩展：
   `agent-service`、`skill-registry`、`mcp-gateway`、`action-executor`
   后续接真实 MCP / provider tool 或业务写动作时，仍必须走：
   policy precheck -> skill contract -> prepare audit -> proposal -> approval
   -> executor -> low-sensitive result projection -> audit。

4. Python AI Worker 扩展：
   可扩 embedding / rerank / memory extraction / planner / eval 候选，但
   Python 只返回候选和 hash / citation metadata；Go 继续拥有权限、审计、
   状态和持久化。

## 9 个现有服务必要收口

| 服务 | 未完成工作 |
| --- | --- |
| `api-gateway` | 目标环境 legacy observation evidence、provider-grade 配置中心 quota 控制面、灰度治理、生产观测。 |
| `identity-service` | WebAuthn/passkeys、OIDC federation、多 issuer、KMS/HSM、完整风控、生产级 email/SMS provider。 |
| `message-service` | 删除 / 撤回 / 编辑语义深化、外部 proof workflow、发送链路生产观测；媒体二进制交给 future media 能力。 |
| `conversation-service` | 更完整群管理、owner transfer 策略深化、完整历史窗口 / targeted replay repair。 |
| `delivery-service` | 更多 delivery event 消费方、projection repair 深化、容量曲线。 |
| `push-gateway` | 生产级 Redis HA、跨实例 resume 深化、长时间在线容量曲线。 |
| `receipt-service` | 会话列表更多产品能力、更多摘要策略和容量曲线。 |
| `contacts-service` | 组织级策略、租户默认值、来源策略、隐私例外接入 admin/config service。 |
| `policy-service` | provider-grade ReBAC graph / DSL、moderation / risk scoring、tenant DSL / quota、外部 audit pipeline。 |

## Product-active 平台 / 产品化服务

- `media-service`：后续仍需真实 S3-compatible adapter、scanner、thumbnail /
  transcode provider 和更完整 download policy。
- `notification-service`：后续仍需 SMTP / SMS / APNs / FCM adapter、bounce /
  suppression worker、provider redrive / audit 产品化。
- `audit-service`：后续仍需 Kafka ingestion、export worker、SIEM forwarding、
  retention cleanup、segment sealing 和 provider-grade audit export。
- `control-plane-service`：后续仍需 outbox relay、drift monitor、
  expiry / cleanup worker、api-gateway quota consumer 和 provider-grade rollout。
- `presence-service`：后续仍需 push-gateway session event consumer、
  `SubscribePresence`、stale scanner、presence outbox relay、Redis hot-state
  integration 和 provider-grade privacy / contacts policy integration。
- `model-gateway`：`InvokeEmbedding` 第一版 mock provider path 已落；后续仍需真实
  OpenAI / Claude / local-model HTTP provider、真实 embedding provider、rerank、outbox relay、
  route-refresh worker、budget-reset worker 和 cleanup worker。
- `knowledge-ingestion-service`：`im.knowledge.events` schema 和
  `knowledge_outbox -> im.knowledge.events` first-stage relay 已落；后续仍需 parser
  worker、tombstone / delete proof、真实 connector、provider parser / crawler handoff
  和完整 ingestion repair。
- `vector-index-service`：first-stage rebuild checkpoint worker、JSONL task source 和
  `knowledge-ingestion-service.ListKnowledgeChunks` 公开 API source 的
  `embedding-worker -> model-gateway.InvokeEmbedding -> vector upsert` 已落，并已补
  `loadtest/vectorembedding` 真实进程 smoke；first-stage PostgreSQL embedding task queue
  已支持 claim / complete / claim-timeout retry；first-stage `embedding-producer` 已支持
  file / knowledge source -> PostgreSQL queue；first-stage `chunk-consumer` runtime 已支持
  `knowledge.chunk.ready.v1` refs -> public `ListKnowledgeChunks` resolve -> embedding queue。
  `knowledge_outbox -> im.knowledge.events -> chunk-consumer -> embedding queue` 真实
  Kafka smoke 已通过；PostgreSQL backend state adapter 已显式记录 backend item
  ACTIVE / DELETED 状态并让 Search fail-closed；内部 RPC client 已保留
  `InvokeEmbedding.embedding_values`，新增 `postgres-test` 本地 provider sink 和 optional
  pgvector adapter 包，并让 `embedding-worker` 可通过
  `NEXUSIM_VECTOR_PROVIDER_BACKEND=postgres-test` 做 metadata-backed provider sink 验证，
  或通过 `NEXUSIM_VECTOR_PROVIDER_BACKEND=pgvector` 显式启用 pgvector backend sink；
  已新增可选 `docker-compose.pgvector.yml` overlay 和
  `run-local-pgvector-smoke.ps1` wrapper，使用 `-StartPgVector` 时脚本默认不拉镜像。
  `rebuild-worker` 已支持显式 `embedding-tasks` provider backfill 和 checkpoint cursor 分页续跑；
  `loadtest/vectorembedding -IncludeRebuildBackfill` 已跑通 `postgres-test` 本地 provider
  backfill focused smoke，并通过 tenant-scoped rebuild claim 避免历史 rebuild job 干扰。
  后续仍需 memory / search chunk consumer、镜像可用后的 focused pgvector smoke、真实 Milvus /
  OpenSearch backend、provider backend repair，以及真 provider backfill smoke。
- `admin-service`：`REPAIR_REQUEST -> workflow-service REPAIR_APPROVAL`、
  `CRITICAL -> workflow-service ADMIN_OPERATION` 和第一版 operation-specific
  approval policy / target-service routing 已接；`CONFIG_PUBLISH` /
  `CONFIG_ROLLBACK` / `TENANT_QUOTA_CHANGE` 已接 control-plane 公开 API；
  后续仍需 audit ingestion / export、admin UI、更多下游公开 admin API adapter、compensation operator 和
  provider-grade 运维。

## 后置平台 / 产品化服务

后置服务暂无新条目。新增服务必须满足独立模型 / 伸缩 / 故障 / 安全边界或明显降复杂度，
并通过 ADR / SDD v0.1。

`workflow-service`：第一版 `CreateWorkflow` / `RecordWorkflowDecision` /
`GetWorkflow` 后，继续补 timer worker、compensation worker、external callback
wait、outbox relay、workflow repair operators 和完整 action / repair / admin
operation smoke。

## 后置 Hardening

- 生产级统一观测：collector、Alertmanager、日志汇聚、SLO、retention。
- 分布式 HA / 故障演练：Redis / Kafka / PostgreSQL 更长时长和多故障组合。
- Repair / DLQ / audit 产品化：审批系统、运维 UI、批量 repair、外部审计。
- 容量和复杂度治理：保留 9 服务 `capacity_summary` 统一口径；后续再做
  9 服务长压 campaign、资源曲线、生产 sizing、文件拆分。

可使用多个 sub-agent 并行推进，但必须按服务、文档集、测试面或只读审查问题拆分互不重叠职责；禁止同时改同一 proto、migration、service brief 或架构章节。
