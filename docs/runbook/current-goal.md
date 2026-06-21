# NexusIM Current Goal

本文件只维护当前可执行目标。Codex 目标框短 prompt 见根目录 `prompt.md`，
不要把长 prompt 复制到这里。

## 当前 Active Slice

```text
future platform / product services promotion
```

用户已点名把 goal 指向全部待开发 future 微服务。当前目标不是只做
`media-service`，也不是一次性把全部服务目录铺开，而是把这批服务按统一边界推进到
可逐个实现的 promotion plan。

## 当前目标服务

```text
media / notification / audit / admin / control-plane / presence
model-gateway / workflow / knowledge-ingestion / vector-index
```

## 默认推进方式

1. 每轮先读 `prompt.md`、`agent.md`、本文件和相关 service brief。
2. 先冻结组合边界：哪些服务先做、哪些只保留 port / adapter、哪些需要 ADR。
   组合边界文档见 `docs/sdd/future-platform-services.md`。
3. 对每个服务按顺序推进：SDD v0.1 -> proto / migration -> 六层 skeleton
   -> cmd runtime -> Docker / Prometheus / Grafana -> focused smoke。
4. 第一组优先级建议：
   `media-service` -> `notification-service` -> `audit-service`
   -> `control-plane-service` -> `presence-service`
   -> `model-gateway` / `knowledge-ingestion-service` / `workflow-service`
   -> `vector-index-service`。
5. 只有完成对应服务 SDD v0.1 和门禁影响确认后，才把该服务从 `future`
   stage switch 到 active，并创建 `services/<service>`。

## 当前进展

- 组合 promotion 边界见 `docs/sdd/future-platform-services.md`。
- 10 个目标服务的 SDD v0.1 draft 已存在，单服务状态见 service brief。
- `media-service`、`notification-service`、`audit-service`、
  `control-plane-service`、`presence-service`、`model-gateway`、
  `knowledge-ingestion-service` 已 product-active 并通过各自第一版 focused
  checks / smoke。
- `workflow-service` 已从 stage-switch 进入 product-active 第一版 implementation
  slice，`CreateWorkflow`、`RecordWorkflowDecision`、`GetWorkflow` 最小路径已落地并
  支持 `ACTION_APPROVAL`、`REPAIR_APPROVAL` 和 `ADMIN_OPERATION`，通过 focused
  checks / 完整 `check-local`。
- `vector-index-service` 已从 stage-switch 进入 product-active 第一版 implementation
  slice，覆盖 `UpsertVectorItem`、`TombstoneVectorItem`、`SearchVectors`、
  `GetVectorIndexJob`、PostgreSQL metadata、local / PostgreSQL-backed test vector adapter
  和 `RequestVectorRebuild` rebuild job / checkpoint API；PostgreSQL backend state
  adapter 已显式记录 backend item ACTIVE / DELETED 状态，Search 必须 join ACTIVE
  backend state 才返回 refs；first-stage
  `rebuild-worker` 已能 claim / complete rebuild checkpoint，并写
  `vector.rebuild.started.v1` / `vector.rebuild.completed.v1` 低敏 outbox event。并已接
  `vector_outbox -> im.vector.events` 第一版 outbox relay。当前 relay 已覆盖低敏
  Kafka schema、event builder、PostgreSQL outbox store、PENDING / PUBLISHED / retry /
  DLQ 状态推进、focused tests 和 `loadtest/vectorindex` 真实 Kafka relay smoke；
  `loadtest/knowledgevector` 已跑通 knowledge chunk manifest -> vector upsert ->
  vector search 的公开 API handoff。
- `model-gateway` 已补 `InvokeEmbedding` 第一版公开 gRPC 路径：deterministic mock
  embedding provider 会把向量返回给调用方，但 PostgreSQL / outbox 只保存 input hash、
  embedding hash、维度、token / cost / latency 等低敏 metadata，不保存 raw input 或
  embedding vector array。这解除后续 `vector-index-service` embedding worker 不能绕过
  model-gateway 的边界问题。
- `vector-index-service` 已补 first-stage `embedding-worker`：支持本地 JSONL 任务源，
  以及通过 `knowledge-ingestion-service.ListKnowledgeChunks` 公开 API 拉取 redacted
  preview 的 knowledge 任务源；随后调用 `model-gateway.InvokeEmbedding`，再经现有 app
  usecase 写 vector item metadata。该切片用于验证 worker / model-gateway /
  knowledge public API / vector upsert 边界，不新增 raw text 公共 API，也不把 raw text
  或 embedding vector array 落入 metadata PostgreSQL / outbox / metrics。
- `vector-index-service` 已跑通 `loadtest/vectorembedding` embedding worker 真实进程
  smoke：公开 gRPC 准备 knowledge source / job / chunk manifest，启动
  `embedding-worker` 通过 `ListKnowledgeChunks` 拉 redacted preview，经
  `model-gateway.InvokeEmbedding` 写 vector metadata，并用 `SearchVectors` 验证。该 runner
  不手工 upsert、不读私表，原始 summary 写入 `H:\NexusIM\loadtest-results`。
- `vector-index-service` 已新增 first-stage PostgreSQL embedding task queue：
  `vector_embedding_tasks` 只持久化 redacted preview、input hash 和低敏 refs / visibility
  metadata；`embedding-worker` 支持 `NEXUSIM_VECTOR_EMBEDDING_SOURCE=postgres`，用
  `FOR UPDATE SKIP LOCKED` claim、claim-timeout retry 和 complete 标记。
- `vector-index-service` 已新增 first-stage `embedding-producer`：
  `NEXUSIM_VECTOR_INDEX_SERVICE_MODE=embedding-producer` 从 file / knowledge source 读取
  redacted-preview task 并写入 PostgreSQL queue；producer 不允许使用 postgres source。
  `loadtest/vectorembedding` 已跑通 producer -> queue -> embedding-worker 链路，
  最近结果：`H:\NexusIM\loadtest-results\vector-embedding-producer-smoke-20260621-081520`。
- `vector-index-service` 已新增 first-stage `chunk-consumer` runtime：
  `NEXUSIM_VECTOR_INDEX_SERVICE_MODE=chunk-consumer` 消费低敏
  `knowledge.chunk.ready.v1` refs 后，通过
  `knowledge-ingestion-service.ListKnowledgeChunks` 公开 API resolve redacted preview，
  再写入 PostgreSQL embedding queue；当前有 focused tests，并已跑通
  `knowledge_outbox -> im.knowledge.events -> chunk-consumer -> vector_embedding_tasks`
  真实 Kafka smoke。
- `vector-index-service` 已补内部 embedding handoff：`ModelGatewayClient` 现在保留
  `model-gateway.InvokeEmbedding` 返回的 `embedding_values`，但公开 API / PostgreSQL
  metadata / outbox / metrics 仍只保存 hash / refs / dimension。已新增
  `NEXUSIM_VECTOR_PROVIDER_BACKEND=postgres-test` 本地 provider sink：它只确认
  `vector_backend_items` ACTIVE 状态和 hash / dimension，不保存 raw vector array，用于
  本地 embedding / rebuild backfill 验证。已新增 optional
  `internal/infrastructure/pgvector` adapter 包，覆盖 schema 初始化、upsert、delete、
  search 和 focused unit tests；`embedding-worker` 可通过
  `NEXUSIM_VECTOR_PROVIDER_BACKEND=pgvector` 显式启用 pgvector backend sink。该 adapter
  默认不启用，也不进入普通 migration，因为默认本地 PostgreSQL 镜像没有 `vector`
  扩展。已新增可选 `deploy/local/docker-compose.pgvector.yml` profile，单独启动
  `nexusim-pgvector`，不替换默认 `nexusim-postgres`。已新增
  `loadtest/vectorembedding/run-local-pgvector-smoke.ps1`；使用 `-StartPgVector` 时脚本会
  检查本机镜像但不自动拉取。本机当前未发现 `pgvector/pgvector:pg16` 镜像，因此真实
  pgvector smoke 仍未执行。
- `vector-index-service` `rebuild-worker` 已支持 first-stage provider backend backfill：
  `NEXUSIM_VECTOR_REBUILD_BACKFILL_SOURCE=embedding-tasks` 会从本服务已完成
  `vector_embedding_tasks` 读取 redacted preview，重新经 model-gateway embedding，再写入
  显式配置的 provider backend；默认不启用，未配置 provider backend fail-fast。backfill
  已支持 checkpoint cursor 分页续跑：每批推进 `vector_rebuild_checkpoints.cursor_value`，
  后续 RunOnce 继续 claim RUNNING rebuild，直到没有下一页才标记 completed。
- `vector-index-service` 已跑通本地 `postgres-test` provider rebuild backfill focused smoke：
  `loadtest/vectorembedding/run-local-smoke.ps1 -IncludeRebuildBackfill` 覆盖
  `embedding-producer -> vector_embedding_tasks -> embedding-worker -> rebuild-worker
  -> provider backend`，并通过 `NEXUSIM_VECTOR_REBUILD_TENANT_ID` 限定当前 run tenant，
  避免本地历史 rebuild job 干扰 focused smoke。最近结果：
  `H:\NexusIM\loadtest-results\vector-embedding-producer-smoke-20260621-101900`。
- `knowledge-ingestion-service` 已新增低敏 `im.knowledge.events` Kafka schema 和
  `knowledge_outbox -> im.knowledge.events` 第一版 `outbox-relay` runtime。relay 覆盖
  source-created、document-parsed、chunk-ready 现有 outbox 事件和 SDD 预留的
  tombstone / failed / delete-proof 事件 schema；payload 不包含 source URI、object key、
  chunk preview / chunk text、connector secret 或 parser raw error。`vector-index-service`
  `chunk-consumer` 已支持 protobuf `KnowledgeEvent`，同时保留 JSON fallback。
- `admin-service` 已从 stage-switch 进入 product-active 第一版 implementation
  slice，覆盖 `CreateAdminOperation`、`ApproveAdminOperation`、
  `GetAdminOperation`、`ListAdminOperations`、PostgreSQL operation / approval
  状态、低敏 admin outbox、`admin_outbox -> im.admin.events` outbox relay 和
  `operation-worker` risk routing 第一版执行闭环。`REPAIR_REQUEST` 已接入
  `workflow-service` 创建 `REPAIR_APPROVAL`，其它 `CRITICAL` operation 已接入
  `ADMIN_OPERATION`，并为 config / quota / policy / audit / notification 类操作写入
  第一版专用 approval policy 和 target service；未配置 workflow 时
  `REPAIR_REQUEST` / `CRITICAL` 操作 fail-closed，不再被本地 no-op executor 标记成功。
- `admin-service` 已新增 `loadtest/admin` operator CLI，用公开 gRPC 完成 create /
  approve / reject / get / list，输出低敏 JSON，不读取私表。
- `admin-service` 已新增第一条真实下游公开 API adapter：非 `CRITICAL` 的
  `CONFIG_PUBLISH` 可在配置 `NEXUSIM_CONTROL_PLANE_GRPC_ADDR` 后由
  operation-worker 调 `control-plane-service.PublishConfigVersion`；critical 操作
  仍走 workflow。
- `admin-service` 已跑通第一条真实下游 adapter 的本地多进程 smoke：公开 gRPC
  `CreateAdminOperation -> operator approve -> operation-worker ->
  control-plane PublishConfigVersion -> GetConfigSnapshot`。
- `admin-service` / `control-plane-service` 已新增 config rollback path：非
  `CRITICAL` 的 `CONFIG_ROLLBACK` 通过 operation-worker 调
  `control-plane-service.RollbackConfigVersion`，并已跑通本地多进程
  `publish v1 -> publish v2 -> rollback to v1 -> GetConfigSnapshot` smoke。
- `admin-service` / `control-plane-service` 已新增 tenant quota path：非
  `CRITICAL` 的 `TENANT_QUOTA_CHANGE` 通过 operation-worker 调
  `control-plane-service.PublishConfigVersion(API_GATEWAY_TENANT_QUOTA)`，并已跑通
  本地多进程 `Create -> approve -> tenant quota publish -> GetConfigSnapshot` smoke。
- `admin-service` / `control-plane-service` 已新增 policy ruleset ref path：非
  `CRITICAL` 的 `POLICY_RULE_CHANGE` 通过 operation-worker 调
  `control-plane-service.PublishConfigVersion(POLICY_RULESET_REF)` 发布低敏规则集引用，
  并已跑通本地多进程 `Create -> approve -> policy ruleset publish ->
  GetConfigSnapshot` smoke。
- `admin-service` 已新增 first-stage `compensation-request` 本地 operator：默认 dry-run；
  显式关闭 dry-run 后只允许把 `FAILED` operation 标记为
  `COMPENSATION_REQUESTED`，并写低敏
  `admin.operation.compensation_requested.v1` outbox。operator reason file 只落
  sha256 hash / ref，不落 reason 原文。
- `admin-service` / `workflow-service` 已新增 first-stage compensation workflow handoff：
  `compensation-request` 在设置 `NEXUSIM_WORKFLOW_GRPC_ADDR` 后会创建 / replay
  `COMPENSATION_REQUEST` workflow；workflow-service 只保存低敏 target / payload /
  reason refs，不执行真实补偿 mutation。
- `workflow-service` 已新增 first-stage `compensation-worker`：claim 已批准的
  `COMPENSATION_REQUEST` workflow，写 `workflow_compensations` 和低敏
  `workflow.compensation.requested.v1` outbox，并把 workflow 推进到
  `COMPENSATION_PENDING`；真实 provider-grade compensation execution 仍后置。
- `workflow-service` 已新增 first-stage `compensation-executor`：
  `control-plane-rollback-file` adapter 只在显式配置 instruction file 时执行
  control-plane-service 公开 `RollbackConfigVersion`；缺失 instruction 或 unsupported
  target 会 fail closed，不读取 admin-service 私有表，也不把 raw payload 写入
  workflow DB / outbox。

## 下一步

- 默认继续更多明确下游补偿 adapter / compensation instruction 管理，或补其它明确下游公开 admin API adapter。
- 默认下一步可继续 vector-index provider backend：在镜像可用后跑 focused pgvector smoke、
  真实 Milvus / OpenSearch backend、provider backend repair / 真 provider backfill smoke，
  或继续 active future service 的 focused checks。
- 也可以继续 notification SMTP / SMS / APNs / FCM adapter 或 bounce-suppression。

## 硬边界

- 不一次性 promotion 全部 future 服务。
- 不把媒体二进制塞回 message-service。
- 不把 identity 局部 webhook / SMTP 扩成完整 notification-service 前的生产承诺。
- 不让 admin / control-plane / workflow 直接改其它服务私有表。
- 不让 model-gateway / vector-index / knowledge-ingestion 绕过 retrieval /
  policy / EvidencePack 边界。
- 不回滚用户已有修改。
- 小改跑 focused checks；涉及 service-registry / Docker / compose / proto /
  migration / 安全边界时再扩大门禁。

## 文档路由

- 当前阶段背景：`docs/runbook/current-brief.md`
- 剩余待办：`docs/runbook/remaining-goals.md`
- 服务入口：`docs/runbook/service-briefs/<service>.md`
- 新发现待办写入 `docs/runbook/remaining-goals.md`
