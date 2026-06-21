# NexusIM Current Brief

本文件是每轮低 token 入口，只回答“现在处于什么阶段、下一步去哪里看”。
不要在这里维护长历史或完整待办。

## 按需读取

- 具体执行目标：`docs/runbook/current-goal.md`
- 剩余目标：`docs/runbook/remaining-goals.md`
- 服务细节：先读 `docs/runbook/service-briefs/README.md`，再读对应服务 brief。
- 历史证据：按关键词查 `docs/runbook/loadtest/`、`docs/runbook/archive/`
  或 `docs/runbook/history/`。

## 当前阶段

NexusIM 已有本地 / 双机可运行的最小分布式 IM 后端。
9 个后端服务已进入真实链路：`api-gateway`、`identity-service`、`message-service`、`conversation-service`、`delivery-service`、`push-gateway`、`receipt-service`、`contacts-service`、`policy-service`。

当前 active slice 已切到 `future platform / product services promotion`：

```text
future services -> SDD v0.1 -> stage-switch plan -> service-by-service skeleton
```

AI foundation-active 服务：`search-service`、`memory-service`、`retrieval-gateway`、`rag-service`、`summary-service`、`agent-service`、`skill-registry`、`mcp-gateway`、`action-executor`、`ai-eval-service`。

Go 侧服务底座、EvidencePack、proposal / approval / audit、Python Worker 候选接入边界和低敏 eval 持久化已经足够支撑算法切片；后续 Go 工作围绕候选接入、边界校验和状态流转。

当前下一步：

```text
future platform / product services 的 10 个 SDD draft 已存在；`media-service`、
`notification-service`、`audit-service`、`control-plane-service`、`presence-service`
和 `model-gateway` 已进入 product-active 并通过各自第一版 smoke。
`knowledge-ingestion-service` 已完成第一版 metadata + chunk manifest path；
`workflow-service` 已完成 `CreateWorkflow`、`RecordWorkflowDecision`、`GetWorkflow`
最小审批等待路径，并通过 focused checks / 完整 `check-local`。
`model-gateway` 已补第一版 `InvokeEmbedding` mock provider 路径；`vector-index-service`
已完成第一版 `UpsertVectorItem` / `TombstoneVectorItem` /
`SearchVectors` / `GetVectorIndexJob` / `RequestVectorRebuild` path、
first-stage rebuild checkpoint worker、`vector_outbox -> im.vector.events` 第一版 relay、
真实 Kafka relay smoke、knowledge chunk -> vector upsert 公开 API handoff smoke，以及
first-stage `embedding-worker`：本地 JSONL 任务源或
`knowledge-ingestion-service.ListKnowledgeChunks` redacted preview 公开 API 任务源 ->
`model-gateway.InvokeEmbedding` -> vector upsert hash / metadata。`loadtest/vectorembedding`
真实进程 smoke 已跑通，用公开 gRPC 准备 knowledge chunk manifest，
再启动 embedding worker 经 `model-gateway.InvokeEmbedding` 写 vector metadata，并通过
`SearchVectors` 验证；该 smoke 入口不手工 upsert、不读私表。PostgreSQL embedding
task queue 已新增，`embedding-worker` 可用 `NEXUSIM_VECTOR_EMBEDDING_SOURCE=postgres`
claim redacted-preview task 并 complete；`embedding-producer` 可从 file / knowledge
source 读取 redacted-preview task 写入该 queue，`loadtest/vectorembedding` 已跑通
producer -> queue -> worker 链路；`chunk-consumer` runtime 已能消费低敏
`knowledge.chunk.ready.v1` refs，经 `ListKnowledgeChunks` 公开 API resolve redacted
preview 后入 embedding queue，当前覆盖 focused tests，并已支持 `im.knowledge.events`
protobuf `KnowledgeEvent` 与旧 JSON fallback；`knowledge-ingestion-service` 已补
`knowledge_outbox -> im.knowledge.events` first-stage relay 和低敏 schema，并已跑通
真实 Kafka chunk-consumer 联调 smoke，把 2 个 knowledge chunk refs 写入
`vector_embedding_tasks`；PostgreSQL backend state adapter 已显式记录 backend item
ACTIVE / DELETED 状态，`SearchVectors` 必须 join ACTIVE backend state 才返回 refs；
vector-index 内部 RPC client 已保留 `InvokeEmbedding.embedding_values`，并新增
`postgres-test` 本地 provider sink，可确认 owned backend state ACTIVE 但不保存 raw vector
array；同时新增 optional pgvector adapter 包；`embedding-worker` 可通过
`NEXUSIM_VECTOR_PROVIDER_BACKEND=pgvector`
显式启用 pgvector backend sink，且已有可选 `docker-compose.pgvector.yml` overlay。
`run-local-pgvector-smoke.ps1` wrapper 已准备；使用 `-StartPgVector` 时默认不拉镜像。
本机未发现 `pgvector/pgvector:pg16` 镜像，所以真实 pgvector smoke 尚未执行。
`rebuild-worker` 已新增显式 `embedding-tasks` provider backfill：只读取本服务 completed
queue 中 redacted preview，重新 embedding 后写 provider backend；未配置 backend fail-fast，
并已支持 checkpoint cursor 分页续跑。`postgres-test` provider backfill focused smoke 已通过：
`loadtest/vectorembedding/run-local-smoke.ps1 -IncludeRebuildBackfill` 覆盖 producer / queue /
worker / rebuild-worker，本地通过 `NEXUSIM_VECTOR_REBUILD_TENANT_ID` 限定 run tenant，避免历史
rebuild job 干扰。公开 API / PostgreSQL metadata / outbox / metrics 仍不暴露 raw vector array。
`admin-service` 已完成第一版
`CreateAdminOperation` / `ApproveAdminOperation` / `GetAdminOperation` /
`ListAdminOperations` path、`admin_outbox -> im.admin.events` outbox relay 和
`operation-worker` risk routing 执行闭环；`REPAIR_REQUEST` 已接入
workflow-service `REPAIR_APPROVAL`，其它 `CRITICAL` operation 已接入
workflow-service `ADMIN_OPERATION`，并已写入第一版 operation-specific approval
policy / target service；`loadtest/admin` operator CLI 已支持公开 gRPC create /
approve / reject / get / list；第一条真实下游 adapter 已支持非 `CRITICAL`
`CONFIG_PUBLISH -> control-plane-service.PublishConfigVersion`；第二条 control-plane
adapter 已支持非 `CRITICAL` `CONFIG_ROLLBACK ->
control-plane-service.RollbackConfigVersion`；第三条 control-plane adapter 已支持非
`CRITICAL` `TENANT_QUOTA_CHANGE ->
control-plane-service.PublishConfigVersion(API_GATEWAY_TENANT_QUOTA)`；第四条
control-plane adapter 已支持非 `CRITICAL` `POLICY_RULE_CHANGE ->
control-plane-service.PublishConfigVersion(POLICY_RULESET_REF)`。
admin `Create -> operator approve -> operation-worker -> control-plane` 本地多进程
publish / rollback / tenant quota / policy ruleset smoke 已通过。`admin-service`
已新增 first-stage `compensation-request` 本地 operator：默认 dry-run，正式执行只把
`FAILED` operation 标记为 `COMPENSATION_REQUESTED`，并写低敏
`admin.operation.compensation_requested.v1` outbox，reason file 只落 hash / ref；
设置 `NEXUSIM_WORKFLOW_GRPC_ADDR` 时会创建 / replay workflow-service
`COMPENSATION_REQUEST` workflow；workflow-service `compensation-worker` 已能物化
已批准补偿请求到 `workflow_compensations` 和低敏 outbox。`compensation-executor`
已支持显式 instruction file 驱动的 control-plane rollback adapter，缺失 instruction /
unsupported target fail closed；并已补 workflow-service 自有
`workflow_compensation_instructions` 低敏 registry、`compensation-instruction-import`
operator mode 和 `control-plane-rollback-store` resolver；store instruction 已强制绑定
具体 `COMPENSATION_REQUEST` workflow 并校验 refs。下一步默认继续更多明确下游补偿 adapter /
provider-grade compensation instruction 审批 / UI 管理、其它明确下游公开 admin API adapter，或在镜像可用后继续 focused pgvector smoke、Milvus /
OpenSearch backend / provider repair / 真 provider backfill smoke。
```

系统测试 / HA / 长压 / sizing 后置；总览、待办、单服务状态分别看
`development-progress.md`、`remaining-goals.md`、`service-briefs/<service>.md`。

- RAG / summary / Agent 只能消费权限过滤后的 EvidencePack。
- 真实写动作必须走 policy、proposal / approval、executor 和 audit。
- Python AI Worker 只做模型 / 算法 / eval 候选层，Go 负责控制面、状态和审计。
- future 服务 promotion 期间不得一次性创建全部服务目录。
- 媒体、通知、审计、控制面、presence、model、workflow、ingestion、vector 等边界必须继续通过公开 API、事件或明确 port 串联。
- 不回滚用户已有修改。
- 新发现的待完成工作写入 `docs/runbook/remaining-goals.md`。
