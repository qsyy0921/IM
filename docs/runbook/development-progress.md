# NexusIM Development Progress

这份文档只做“当前开发进度总览”。

- 面向人看整体进度，不作为每轮默认入口。
- 每次只在阶段状态真的变化时更新。
- 细节证据仍放在 `loadtest/`、`service-briefs/`、`sdd/` 和 `archive/`。
- 阶段顺序和为什么这么做，见 `development-process.md`。

## 当前快照

截至当前仓库状态，NexusIM 已经不是单体 demo，而是可本地 / 双机运行的最小分布式 IM 后端。

当前已落地的真实服务：

- `api-gateway`
- `identity-service`
- `message-service`
- `conversation-service`
- `delivery-service`
- `push-gateway`
- `receipt-service`
- `contacts-service`
- `policy-service`

当前已开始的 AI 大模型应用底座能力：

- `search-service`
- `memory-service` / group memory projection
- `retrieval-gateway` / EvidencePack
- `rag-service` first read-only answer path + executable RAG adapter runner + real adapter smoke + provider boundary / citation verifier + guarded external HTTP LLM boundary
- `summary-service` first read-only EvidencePack summary path + real adapter smoke + guarded external HTTP LLM boundary
- `agent-service` first proposal-only path + proposal store / approval preflight / approval outbox relay + approval operator + planner Python worker candidate guard
- `skill-registry` first catalog path + PG repository / gRPC runtime / Docker / observability wiring
- `mcp-gateway` first prepare path + skill catalog check / policy precheck / low-sensitive audit
- `action-executor` first execution audit path + proposal / approval / prepare audit linkage + local safe adapter + guarded external HTTP provider adapter + external adapter eval + preflight / rate-limit / DLQ-repair safety eval + provider failure retry/DLQ skeleton + bounded retry worker
- `ai-eval-service` first persistent eval run catalog + low-sensitive `RecordEvalRun` / `GetEvalRun` / `ListEvalRuns` + recorder / policy-driven multi-adapter gate smoke + Python optional adapter path + RAG / Agent service-stack live gate + Summary live negative adapter + CI-safe gate skeleton + first RAG-Agent regression expansion + profile / Agent output safety expansion + service-stack version / hash-only expansion + negative RAG / Agent cases + Python/model-output negative cases + RAG/Summary citation source-ref regression + external MCP fallback eval cases + Agent output regression + action preflight / rate-limit / DLQ-repair safety eval + action provider failure worker / redrive safety eval + memory group source-ref / validity / supersession / cross-group / temporal eval、retrieval smoke、RAG / Summary / Agent stack consumption smoke 和 40/40 optional stack gate
- AI eval harness first-stage case schema / validator + RAG execution adapter + expanded profile / Agent / group-memory safety adapter
- Agent execution eval adapter + low-sensitive tool result projection + local safe tool adapter first path
- Python AI Worker 边界已由 ADR-036 固定，且 foundation first path 已落：`ai/python` 目录、`IM` conda toolchain、candidate contract helpers、低敏 safety guard、contract validator、candidate-only worker CLI、malformed / unsafe output eval adapter、bad model-output rejection adapter、第一条 worker smoke、Go-side Python candidate adapter smoke，以及 `rag-service` / `summary-service` / `agent-service` 服务级 Python worker candidate guard；Python 只做模型 / 算法 / eval 候选层，Go 继续拥有控制面、状态、审计和持久化

当前已开始的后续产品化 / 平台服务：

- `media-service` product-active：SDD v0.1 和 stage-switch review 已通过，第一版
  proto / migration / 六层 skeleton / `grpc` runtime / fake object-store port /
  Docker / Prometheus / Grafana 覆盖已落；真实 PG 集成测试和 object_key 不出
  public response / fake presign URL / outbox payload 的回归门禁已补；最小
  gRPC smoke、`media_outbox -> im.media.events` 真实 Kafka smoke 和本地 mock
  processing worker smoke 已通过。
- `notification-service` product-active：SDD v0.1 和 stage-switch review 已通过，
  第一版 proto / migration / 六层 skeleton / `grpc` runtime / Docker / Prometheus /
  Grafana 覆盖已落，并已通过 focused checks / 完整 `check-local`；当前覆盖
  request 事实源、status 查询、cancel、accepted outbox 和
  `notification_outbox -> im.notification.events` 最小 relay、delivery worker 和
  noop / webhook provider adapter，真实 Kafka / delivery smoke 已通过；
  不宣称 provider-grade email / SMS / APNs / FCM。
- `audit-service` product-active：SDD v0.1 和 stage-switch review 已通过，
  第一版 proto / migration / 六层 skeleton / `grpc` runtime / Docker /
  Prometheus / Grafana 覆盖已落；当前覆盖 `AppendAuditRecord`、
  `QueryAuditRecords`、`VerifyAuditProof`、PostgreSQL append hash chain 和
  低敏 `audit.record.appended.v1` outbox；最小 gRPC smoke 已通过并归档，
  不宣称 Kafka ingestion / export / SIEM / retention cleanup。
- `control-plane-service` product-active：SDD v0.1 和 stage-switch review 已通过，
  第一版 proto / migration / 六层 skeleton / `grpc` runtime / Docker /
  Prometheus / Grafana 覆盖已落；当前覆盖 `PublishConfigVersion`、
  `RollbackConfigVersion`、`GetConfigSnapshot`、`AckAppliedConfigVersion`、
  DB-backed quota / feature snapshot 和低敏 control outbox；最小 gRPC smoke 和
  admin-driven config rollback smoke 已通过并归档。
- `presence-service` product-active：SDD v0.1 和 stage-switch review 已通过，
  第一版 proto / migration / 六层 skeleton / `grpc` runtime / Docker /
  Prometheus / Grafana 覆盖已落；当前覆盖 `UpdatePresence`、`GetPresence`、
  `UpdateTyping`、PostgreSQL user / session / typing projection 和低敏
  presence outbox；最小 gRPC smoke 已通过并归档。
- `model-gateway` product-active：SDD v0.1 和 stage-switch review 已通过，
  第一版 proto / migration / 六层 skeleton / `grpc` runtime / Docker /
  Prometheus / Grafana 覆盖已落；当前覆盖 `InvokeTextGeneration`、
  `GetModelInvocation`、allowlisted deterministic mock provider、低敏
  invocation metadata 和 `model_outbox`；最小 gRPC smoke 已通过并归档。
- `knowledge-ingestion-service` product-active：SDD v0.1 和 stage-switch review
  已通过，第一版 proto / migration / 六层 skeleton / `grpc` runtime / Docker /
  Prometheus / Grafana 覆盖已落；当前覆盖 `CreateKnowledgeSource`、
  `SubmitIngestionJob`、`GetIngestionJob`、`ListKnowledgeChunks`、本地 metadata +
  chunk manifest、低敏 `knowledge_outbox`；真实 PG 集成测试已验证 source + job +
  chunks + outbox 同事务，且 outbox 不含 source URI、object key、chunk text 或
  parser raw error；`loadtest/knowledgevector` 已通过公开 gRPC 证明 chunk manifest
  可 handoff 到 `vector-index-service`。
- `workflow-service` product-active：SDD v0.1 和 stage-switch review 已通过，
  第一版 proto / migration / 六层 skeleton / `grpc` runtime / Docker /
  Prometheus / Grafana 覆盖已落；当前覆盖 `CreateWorkflow`、
  `RecordWorkflowDecision`、`GetWorkflow`、action / repair / admin operation
  approval 最小状态机和低敏 `workflow_outbox`；已作为 admin-service
  `REPAIR_REQUEST -> REPAIR_APPROVAL` 和 `CRITICAL -> ADMIN_OPERATION` 的长审批
  入口，并支撑 admin operation-specific approval policy / target-service routing；
  focused checks、真实 PostgreSQL integration 和完整 `check-local` 已通过，不宣称
  timer / compensation worker、external callback wait 或 outbox relay。
- `vector-index-service` product-active：SDD v0.1 和 stage-switch review 已通过，
  第一版 proto / migration / 六层 skeleton / `grpc` runtime / Docker /
  Prometheus / Grafana 覆盖已落；当前覆盖 `UpsertVectorItem`、
  `TombstoneVectorItem`、`SearchVectors`、`GetVectorIndexJob`、PostgreSQL metadata
  和 local / PostgreSQL-backed test vector adapter；`vector_outbox -> im.vector.events`
  第一版 outbox relay、低敏 Kafka schema、PENDING / PUBLISHED / retry / DLQ 状态推进和
  focused relay / store tests 已落；`loadtest/vectorindex` 已跑通真实 outbox relay
  + Kafka `im.vector.events` readback；`loadtest/knowledgevector` 已跑通 knowledge
  chunk -> vector upsert -> vector search handoff；不宣称 Milvus / pgvector /
  OpenSearch、embedding worker 或 rebuild worker。
- `admin-service` product-active：SDD v0.1 和 stage-switch review 已通过，
  第一版 proto / migration / 六层 skeleton / `grpc` runtime / Docker /
  Prometheus / Grafana 覆盖已落；当前覆盖 `CreateAdminOperation`、
  `ApproveAdminOperation`、`GetAdminOperation`、`ListAdminOperations`、
  PostgreSQL operation / approval 状态、低敏 `admin_outbox`、
  `admin_outbox -> im.admin.events` outbox relay 和 `operation-worker`
  risk routing 执行闭环；`REPAIR_REQUEST` 已路由到 workflow-service
  `REPAIR_APPROVAL`，其它 `CRITICAL` operation 已路由到 workflow-service
  `ADMIN_OPERATION`；`loadtest/admin` operator CLI 已支持通过公开 gRPC create /
  approve / reject / get / list；非 `CRITICAL` 的 `CONFIG_PUBLISH` 已可通过
  `NEXUSIM_CONTROL_PLANE_GRPC_ADDR` 调用 control-plane public gRPC；非
  `CRITICAL` 的 `CONFIG_ROLLBACK` 已可通过同一公开 gRPC adapter 回滚
  control-plane 配置；非 `CRITICAL` 的 `TENANT_QUOTA_CHANGE` 已可通过同一公开
  gRPC adapter 发布 `API_GATEWAY_TENANT_QUOTA` 配置；本地多进程 config publish /
  rollback / tenant quota smoke 已通过；
  不宣称其它真实下游 mutation、admin UI 或 provider-grade 运维。

当前 Go 侧服务底座、控制面、EvidencePack、proposal / approval / audit、
Python Worker 候选接入边界和低敏 eval 持久化已经足够支撑算法切片；
当前 active slice 已转为 future platform / product services promotion，继续按服务
推进媒体、通知、审计、控制面、presence、model 等产品化 / 平台服务，不再泛化清九服务 P2。

当前 10 个 future platform / product services 均已进入 product-active 第一版实现；
后续继续按 service brief 推进 worker、relay、真实 provider / adapter 和 smoke。

当前可以采用 multi sub-agent 方式加快后续 AI 底座开发，但只允许拆分互不重叠的服务、文档或验证范围；主 agent 保持最终方案、集成和检查责任。

当前面试主线只覆盖：

```text
后端微服务主链路
-> 分布式可靠性
-> 9 个现有服务必要收口
-> search-service v0.1 第一实现切片已跑通 projection smoke
-> memory-service foundation-active projection smoke 已通过
-> retrieval-gateway / EvidencePack 第一轮真实 smoke 已通过，field hardening first pass 已落
-> AI eval harness first pass 已落
-> rag-service first read-only answer path / loadtest runner / eval adapter / real adapter smoke / provider boundary / citation verifier 已落
-> summary-service first read-only summary path + real adapter smoke 已落
-> agent-service first proposal-only path + real adapter smoke + mcp-gateway prepare adapter smoke + proposal store / approval preflight / approval outbox relay + approval operator + planner Python worker candidate guard 已落
-> skill-registry first catalog path 已落
-> mcp-gateway first prepare path 已落
-> action-executor first execution audit path + Agent approved proposal handoff 已落
-> Agent execution eval adapter first path 已落
-> low-sensitive tool result projection first path 已落
-> local safe tool adapter / safe output hash projection first path 已落
-> RAG/Summary guarded external HTTP LLM boundary 已落
-> Python worker output-safety eval + first candidate-only smoke 已落
-> Go-side Python candidate adapter smoke 已落
-> Python worker model-output negative cases 已落
-> rag-service Python worker candidate guard
-> summary-service Python worker candidate guard
-> agent-service planner Python candidate guard
-> guarded external HTTP provider adapter first path 已落
-> external adapter eval / failure smoke cases 已落
-> profile overgeneralization / Agent output safety expanded eval cases 已落
-> action preflight / rate-limit / DLQ-repair safety eval cases 已落
-> ai-eval-service first persistent eval run catalog / RecordEvalRun recorder / policy-driven multi-adapter gate smoke / Python optional adapter path / RAG-Agent service-stack live gate / Summary live negative adapter / CI-safe gate skeleton / first RAG-Agent regression expansion / profile-Agent safety expansion / service-stack version-hash expansion / Python-model-output negative expansion / RAG-Summary citation source-ref regression / current-memory service-stack live gate / cross-group-temporal memory fixture eval / retrieval smoke / RAG-Summary-Agent stack consumption smoke / optional stack gate
-> 安全 / 观测 / repair / 运维 hardening
```

Web / App / 桌面端属于后续产品化展示层，暂不纳入当前开发主线。

## 总体进度

### 1. IM 主链路

当前 9 个服务已经覆盖 IM 主链路：

- 注册 / 登录 / refresh / MFA 基础能力
- 会话和成员读写
- 发送消息
- timeline / outbox / Kafka 传播
- durable inbox / `PullInbox` / `AckDelivery`
- WebSocket 在线通知
- receipt / contacts / policy 基础链路

可以把当前系统表述为：

```text
本地 / 双机可运行的最小分布式 IM 后端
```

还不能表述为：

```text
生产级完整分布式 IM 平台
```

### 2. 分布式与可靠性

已经完成的关键分布式证据：

- 本地多进程 distributed smoke
- Win / Mac Docker cross-instance smoke
- Redis route / Redis-backed resume
- Redis stop/start fault fallback
- Redis Sentinel discovery / failover / master-stop / quorum-loss fallback
- Redis Sentinel network-partition fallback smoke
- Redis Cluster 本地三节点 topology smoke
- Redis Cluster node-stop fallback smoke
- Redis Cluster 六节点自动 failover smoke
- Redis Cluster 六节点短容量基线
- Redis Sentinel / Cluster smoke summary 离线 validator
- PostgreSQL `repmgr + pgpool` local failover smoke
- PostgreSQL quorum observation smoke、summary 离线 validator，以及 ADR-034 production quorum boundary
- 分布式 smoke 证据低敏 manifest：集中索引 Redis / PostgreSQL / Kafka 本地 summary 路径，支持 schema-only / H 盘真实文件复核和 Markdown report，且可用 `tools/add-distributed-smoke-evidence.ps1` 追加新故障 smoke 证据，避免手改 JSON
- 安全启动门禁 catalog：集中索引 DDD / cross-service table / future service boundary / debug listener / public listener auth / TLS / gateway / api-gateway legacy / quota 子门禁 / api-gateway legacy evidence / repair operator safety gates，并校验已接入 `check-local` 或由父 check 覆盖
- Kafka KRaft 3 broker local failover / controller-switch / ISR observation smoke，且 ISR observation raw summary 已有可复用 JSON / Markdown summary validator
- Kafka KRaft repeated ISR flapping smoke：本地 2 轮 broker stop/start 均验证 ISR 从 3 收缩到 2、恢复到 3，且 `acks=all` probe 在降级和恢复阶段均可写入；这是本地 flapping 观察，不是生产 Kafka HA 或 rebalance storm 证明
- outbox Kafka producer first-stage `acks=all` / bounded retry-backoff 配置、本地门禁、7 个 producer package 配置单测、producer config summary 和 Kafka producer hardening evaluation；当前 `kafka-go` writer 明确不声明 idempotent / transactional producer 语义，可靠业务边界仍是 outbox / event_id 幂等
- 本地 `kafka-go` producer in-flight broker-fault observation：120 条 records 在 broker stop/restore 窗口内全部 ack，消费侧 unique 120、missing acknowledged 0、observed duplicate 0；这是一次本地观察，不证明 exactly-once producer 语义
- push-gateway delivery-consumer 本地 Kafka consumer group rebalance smoke：2 个 consumer 进入同一 group 后停止 1 个，Kafka 将 `im.delivery.events` 3 个 partition 重新分配到剩余 consumer
- push-gateway delivery-consumer 本地 Kafka consumer churn smoke：2 轮 leave / rejoin、8 个 transition 均回到 Stable，且 `im.delivery.events` 3 个 partition 每次都已分配；这是本地 churn 观察，不是生产 rebalance storm SLO
- push-gateway delivery-consumer 本地 Kafka consumer churn probe smoke：在 8 个 transition 后共写入 24 条合法 `delivery.inbox_item.created.v1` probe，全部 ack，consumer group 每次 post-probe lag 回到 0；这是本地消息连续性观察，不是生产 rebalance storm SLO

当前已经证明：

- 在线通知层可以跨实例工作
- Redis 故障时 durable `PullInbox + AckDelivery` 可以兜底
- PostgreSQL / Kafka 单点切换后最小链路仍可恢复
- Kafka consumer group 能完成第一阶段本地 rebalance 观察
- Kafka consumer group 能完成第一阶段本地 repeated leave / rejoin churn 观察
- Kafka consumer group 在本地 churn 后还能消费合法 delivery probe 并回到 zero lag

当前还没有证明：

- 生产级 Redis HA / Redis Cluster 长时间容量曲线和跨机器治理
- 生产级 PostgreSQL HA / split-brain fencing / quorum write guard
- 生产级 Kafka multi-failure / long-duration ISR flapping / rebalance storm 治理
- 完整部署编排、服务发现、统一观测、灰度发布

这些都是后置 hardening gap，不是当前转进 `search-service` / memory / retrieval 的短期阻塞条件。

### 3. 安全与运维

当前已经落地的共性 hardening：

- 各核心服务已补 `/healthz`、`/readyz`、`/debug/metrics`
- 公网地址 + 弱鉴权 / 明文入口的启动门禁
- trusted metadata / mTLS 边界的第一阶段收口
- 项目命名门禁覆盖 Go / Markdown / PowerShell / Bash 等文本文件，并带 shell fixture 自测，防止旧项目名回流
- 六层 DDD 反向依赖门禁，生产代码禁止 `api/app/domain/trigger/types` 直接 import `internal/infrastructure`
- 跨服务私有表访问门禁，生产代码禁止直接 SQL 访问其他服务私有表，只保留已冻结的共享 timeline / outbox 例外
- 文件大小预算门禁，手写 Go / Markdown / PowerShell / Bash 文件继续按生产代码、测试 / runner、文档和脚本分档控复杂度；`tools/check-file-size-budget.ps1` 可按需输出 JSON / Markdown hotspot summary，当前持久基线见 `docs/runbook/file-size-hotspot-baseline.json` 和 `docs/runbook/file-size-hotspots.md`，且摘要格式 / 持久基线均已有 `check-local` 自测门禁；`loadtest/pushgateway` 已按 config / model / auth / scenario / util 同 package 文件拆分，避免在线通知 / Redis route / slow-client / resume smoke 继续堆进单个 `main.go`；`loadtest/receipt`、`loadtest/policyintegration`、`loadtest/sendmessage` 已按 config / model / auth / util 等同 package 文件拆分；`contacts-service` PostgreSQL privacy / source-policy 集成测试已拆到同 package 测试文件；`message-service` PostgreSQL revoke / edit / delete mutation 集成测试已拆出同 package 测试文件；`identity-service` PostgreSQL challenge command methods 已拆出，核心 repository 文件降到约 1.4k 行，app 层登录 / MFA / Refresh / Challenge 测试和 cmd 层 challenge / MFA / gateway-token / env 配置 helper 也已按主题拆分；`api-gateway` cmd 层 rate-limit / tenant-plan 配置测试已从 `main_test.go` 拆到同 package 测试文件，继续降低启动配置测试文件复杂度
- PowerShell / Bash 脚本解析门禁，`tools` 和 `loadtest` 下的 `.ps1` / `.sh` 都会进入本地检查，避免 smoke / 运维脚本语法回归
- `check-local` 覆盖门禁，新增 `tools/check-*.ps1` 默认必须接入主检查；间接或手动检查必须显式列为例外
- future service boundary 门禁仍保护未授权服务目录；`search-service v0.1`、`memory-service v0.1`、`retrieval-gateway`、`rag-service`、`summary-service`、`agent-service`、`skill-registry`、`mcp-gateway` 和 `action-executor` 已作为 AI 底座 foundation-active 服务落地，后续外部 MCP / provider tool adapter 不能绕过 search / memory / retrieval / policy / skill registry / mcp-gateway / approval 直接落 demo；当前 action-executor guarded HTTP provider adapter 也只能显式 allowlist + LOW-risk 执行
- 本地 Prometheus / Grafana / Alertmanager 覆盖门禁，已实现服务目录必须有 scrape / alert rules / dashboard 配置；`tools/run-local-observability-smoke.ps1` 可在本机已有镜像时验证 Prometheus rules、Grafana 9 服务 dashboard 和可选本地 Alertmanager null route 已由真实进程加载，也可按需把本地观测 smoke summary / report 写到 `H:\NexusIM\loadtest-results`；`tools/run-observability-target-smoke.ps1` 可对已有 Prometheus / Grafana 端点做目标环境 dashboard smoke，summary / validation 格式已有 `check-local` 自测门禁；`docs/runbook/observability-evidence.json` 已提供低敏观测证据索引，`tools/add-observability-evidence.ps1` 可追加本地 / 目标环境 smoke evidence，validator 支持 schema / H 盘文件复核；当前索引包含 policy-service debug metrics smoke 和本地观测镜像准备 dry-run 计划（`observability-image-prepare-plan`），不把目标环境 9 服务 dashboard smoke 写成已完成
- 服务 cmd 层启动配置测试门禁，已实现服务必须保留 `main_test.go` 覆盖启动 / 监听 / TLS / auth guard 配置
- 服务 cmd 构建门禁，当前 active 服务的 `services/<service>/cmd/<service>` 必须能通过 `go build`
- 服务 Linux 构建门禁，当前 active 服务的 cmd 包必须能以 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64/arm64` 交叉编译，保证本地 / Mac Docker runtime 的二进制基础不漂移
- 服务包级测试门禁，`go test ./services/...` 默认进入 `check-local`，覆盖当前 active 服务的轻量单测 / 跳过型集成测试
- 服务运行态端点门禁，已实现服务必须保留 `/healthz`、`/readyz`、`/debug/metrics` 和 `/metrics`
- Docker runtime / 本机镜像构建 / Mac 镜像同步 / 本地服务 compose 覆盖门禁，当前 active 服务必须都有 `deploy/docker/<service>.runtime.Dockerfile` 和 `nexusim/<service>:local` 编排入口，本机构建脚本和双机镜像同步脚本默认从 `services/` 推导完整服务集合；`tools/run-local-service-health-smoke.ps1` 可启动本地服务 compose 并检查 active 服务的 `/healthz` / `/readyz`，也可按需把 Docker resource snapshot 写到 `H:\NexusIM\loadtest-results`，再用 `tools/summarize-local-service-resource-snapshot.ps1` 生成健康态资源摘要；摘要格式和 `docs/runbook/resource-snapshot-evidence.json` 低敏证据索引、validator、追加工具均已有 `check-local` 自测门禁
- runbook consistency 门禁，防止 `development-progress.md` / service brief 已标记完成的事项继续残留在 `remaining-goals.md`
- 压测原始输出路径门禁，loadtest / smoke 默认结果不能写回仓库内 `loadtest/results`，原始数据默认落 `H:\NexusIM\loadtest-results`；9 服务 `capacity_summary` 合约、`tools/summarize-loadtest-capacity-baselines.ps1` 容量基线汇总器、`tools/run-loadtest-capacity-baseline-suite.ps1` dry-run / 顺序执行入口、`tools/write-capacity-longrun-campaign-plan.ps1` 30m+ 长压 campaign 计划入口、`tools/test-capacity-longrun-campaign-preflight.ps1` plan-driven 长压就绪预检入口、`tools/invoke-capacity-longrun-campaign.ps1` plan-driven 长压执行 / dry-run 入口、`tools/summarize-capacity-longrun-campaign.ps1` 长压完成结果汇总 / report 生成入口，以及 `docs/runbook/capacity-baseline-evidence.json` / `docs/runbook/capacity-longrun-campaign-evidence.json` 低敏证据索引、validator 和追加工具已有本地自测门禁，suite 会区分 direct runner、需要后台角色的 stack runner 与 seeded-only runner；`deploy/local/docker-compose.service-workers.yml` 已提供本地 relay / consumer worker overlay，`loadtest/capacityseed` 已提供 message / conversation / delivery seeded runner fixture，且本地 seeded 短基线已覆盖 message / conversation / delivery；contacts stack 短基线已覆盖 contacts outbox relay 和 Kafka readback，且 contacts runner 已支持 `--duration` / `--vus` 容量模式；identity stack 短基线已覆盖临时 webhook fixture 与 challenge-delivery-worker，且 identity runner 已支持 `--duration` / `--vus` Login/Refresh 热路径容量模式；receipt stack 短基线已覆盖 message / delivery / receipt relay-consumer 链路和 receipt Kafka readback；api-gateway stack 短基线已覆盖 secure mTLS + HMAC GatewayService facade、push WebSocket、delivery / receipt / policy Kafka readback；push-gateway stack 短基线已覆盖 full 场景在线 notify / PullInbox / ACK / delivery_outbox；policy-service 已有本地 direct 短基线和一条 clean commit direct 30m 长跑切片；delivery-service、message-service、conversation-service 已有本地 seeded 30m 长跑切片；identity-service、contacts-service 和 receipt-service 已有本地 stack 30m 长跑切片；9 个服务的短基线证据已覆盖，且 9 服务 30m+ planned long-run campaign 已登记到 `capacity-longrun-campaign-evidence.json`，后续仍需完整 9 服务长时间运行、资源曲线和生产 sizing
- outbox / projection / challenge delivery 等 repair / audit / cleanup operator，并通过 `docs/runbook/repair-operators.md` 提供统一入口；本地门禁会校验文档中的 operator mode 与对应服务 cmd 入口一致
- `check-local` 会显式检查子门禁脚本和原生命令 exit code，避免出现打印 `FAIL` 但总检查仍返回成功的假绿。
- worker / relay 非取消错误退避重试
- identity-service 已补 opt-in production key guard，并已纳入 `check-local` 与 security gate catalog；生产样式启动会拒绝 legacy / HS256 gateway token 和 MFA / recovery / challenge token 的 local fallback key；这只是本地启动安全门禁，不等同于 KMS/HSM。

更完整的 trace / alert / structured logging、故障演练和运维 workflow 属于后续目标，统一维护在 `remaining-goals.md`。

## 服务进度矩阵

| 服务 | 当前状态 | 最近进展 / 证据 | 详情入口 |
| --- | --- | --- | --- |
| `api-gateway` | 已落地、已接主链路 | quota source guard、file / URL / DB tenant plan snapshot source、first-stage tenant quota audit / set operator、tenant quota approval manifest 强制校验、versioned-required / checksum-required snapshot policy、future snapshot timestamp fail-closed、legacy quiet-window gate / observation window gate、legacy observation/removal-plan 低敏 evidence manifest、OTel / Prometheus 本地观测、cmd rate-limit 配置测试拆分、secure stack 短基线、`loadtest/demo --gateway-facade` `capacity_summary` 和 `--duration` / `--vus` facade 容量循环入口 | `service-briefs/api-gateway.md` |
| `identity-service` | 已落地、已接登录主链路 | login / refresh / MFA / recovery code / JWKS / opt-in OIDC discovery / production-like key guard / challenge delivery、SMTP template、session MFA proof audit / challenge delivery repair audit / cleanup / gateway keyring rotate JSON 留存、repository / cmd helper 和 app 测试拆分、loadtest `capacity_summary` 口径和本地 Login/Refresh 30m 长跑切片 | `service-briefs/identity-service.md` |
| `message-service` | 已落地、已接主链路 | `SendMessage` / 编辑 / 撤回 / 删除、delete scope fail-closed 错误语义、合规删除 external proof manifest verifier、`TEXT` + `IMAGE` / `FILE` / `VOICE` 附件引用消息、`LOCATION` / `CARD` 结构化 payload 消息、outbox / Kafka timeline、outbox audit / repair / repair audit / cleanup JSON 留存、first-stage `/metrics` 和 OTel server span、mutation repository 测试拆分、loadtest `capacity_summary` 口径和本地 seeded 30m 长跑切片 | `service-briefs/message-service.md` |
| `conversation-service` | 已落地、已接主链路 | `GetSendContext` / member change / owner transfer / owner transfer 负向 PG 回归 / ACTIVE roster 分页与单 role / 多 role 过滤、user / role-first 排序、`user_id_prefix` 轻量前缀过滤 / saga / member-change audit JSON 留存、member-window audit（含 ACTIVE 会话 owner 数量异常）/ repair / repair audit（含 ACTIVE 缺 `join_seq` 当前窗口修复、inactive `LEAVE_BEFORE_JOIN`、conversation version floor、非 ACTIVE 会话内 ACTIVE 成员转 LEFT 的保守修复）、first-stage `/metrics` 和 OTel server span、loadtest `capacity_summary` 口径和本地 seeded 30m 长跑切片 | `service-briefs/conversation-service.md` |
| `delivery-service` | 已落地、已接主链路 | projection / `PullInbox` / `AckDelivery` / hide inbox / delivery outbox、outbox audit / repair / repair audit / cleanup JSON 留存、projection failure audit / checkpoint rewind / failure resolve / cleanup JSON 留存、loadtest `capacity_summary` 口径和本地 seeded 30m 长跑切片 | `service-briefs/delivery-service.md` |
| `push-gateway` | 已落地、已接主链路 | notify / ACK / resume / Redis route / Redis resume negative fallback / Win-Mac / Sentinel / network-partition / Redis Cluster topology / Redis Cluster node-stop fallback / Redis Cluster failover / Redis Cluster 短容量基线 / TLS smoke、stack 短基线、loadtest `capacity_summary` 和 `--duration` / `--vus` full 场景容量循环入口 | `service-briefs/push-gateway.md` |
| `receipt-service` | 已落地、已接主链路 | receipt projection / outbox / audit / repair、outbox audit / repair / repair audit / cleanup JSON 留存、`ListReceiptStates` repository 级批量查询、低敏 `received_device_count` 聚合和 opt-in capped device details、会话列表 archived-only / unread / pinned / muted / legacy tag / multi-tag all-match / draft-only / last-source-event-type 过滤、draft-first 和 unread-first 排序、first-stage `/metrics` 和 OTel server span、loadtest `capacity_summary` 口径和本地 stack 30m 长跑切片 | `service-briefs/receipt-service.md` |
| `contacts-service` | 已落地、已接主链路 | request source metadata / source_ref 低敏 fail-fast 校验 / source policy gate / first-stage risk_level + `REVIEW_REQUIRED` operator 审批状态机 / ListContactRequests source-risk-review 过滤与分页 token 绑定 / first-stage ALLOW-DENY privacy exception set / list / delete / search-source privacy gate / profile visibility 总开关和字段级白名单 / contacts search / group filter / USER-TENANT-SYSTEM privacy / tenant privacy operator / outbox / audit / repair、outbox audit / repair / repair audit / cleanup 与 privacy / source policy audit / set / contact-request-review / contact-request-review-audit JSON 留存、repository 同 package 拆分、loadtest `capacity_summary` 口径和本地 stack 30m 长跑切片 | `service-briefs/contacts-service.md` |
| `policy-service` | 已落地、已接主链路 | decision / user action restriction / first-stage ReBAC decision source / first-stage relationship gate + 本地低敏 relation operator / first-stage keyword + HTTP content moderation / first-stage tenant action quota / first-stage tool policy precheck + low-sensitive local audit / projection / outbox / audit / repair、outbox audit / repair / repair audit / cleanup JSON 留存、低敏 decision audit export / forward、本地 direct 短基线和 clean commit direct 30m 长跑切片 | `service-briefs/policy-service.md` |
| `search-service` | 已跑通第一轮 projection smoke | `search_service.proto`、PostgreSQL core migration、六层 skeleton、`SearchMessages` app / domain / gRPC adapter、projection usecase skeleton、PostgreSQL repository、真实 PG visibility / tombstone 集成测试、`grpc` runtime mode、timeline decoder / consumer 和 clean projection smoke 已落地；定位为 search projection / visibility / tombstone / EvidencePack 前置，不做 LLM demo | `service-briefs/search-service.md` |
| `memory-service` | 已跑通第一轮 projection smoke | `memory_service.proto`、PostgreSQL memory core migration、六层 skeleton、`QueryMemoryEvents` / `GetMemoryEvent` / `ListProfileAggregates` gRPC adapter、domain/types/app validation、PostgreSQL repository first pass、timeline projection usecase、`grpc` / `timeline-consumer` runtime、Docker / compose / Prometheus / Grafana wiring、PG integration tests、timeline worker tests 和 clean projection smoke 已落地；定位为 group memory / StructuredMemoryEvent / source refs / visibility window，不做 LLM demo | `service-briefs/memory-service.md` |
| `retrieval-gateway` | 已跑通 EvidencePack smoke，cross-group / temporal retrieval smoke 已落 | `retrieval_gateway.proto`、SDD、六层 skeleton、`RetrieveEvidence` app / gRPC adapter、search / memory RPC clients、可选 policy-service retrieval precheck、EvidencePack `rerank_score` / `dedupe_reason` / `source_coverage`、`grpc` runtime mode、本地 Docker / compose / Prometheus / Grafana wiring、真实本地 `search + memory -> RetrieveEvidence` smoke，以及 2026-06-20 cross-group source refs / speaker attribution / stale-future memory exclusion smoke 已落地；定位为 EvidencePack 统一检索边界，不读业务库、不调用 LLM、不执行 Agent 动作 | `service-briefs/retrieval-gateway.md` |
| `rag-service` | 已落第一版只读问答路径、真实 adapter smoke、citation verifier、guarded external HTTP LLM boundary 和 cross-group / temporal stack smoke | `rag_service.proto`、SDD、六层 skeleton、`AnswerQuestion` app / gRPC adapter、retrieval-gateway RPC client、`grpc` runtime mode、本地 Docker / compose / Prometheus / Grafana wiring 已落地；第一版 deterministic extractive provider，保留 citations / EvidencePack，`generated_by_llm=false`，无 evidence 时拒答；provider 输出后统一通过 citation verifier；真实本地 adapter smoke 已验证 current-memory stale exclusion 与 cross-group source refs / speaker attribution / future memory exclusion；可选 `external-http` provider 只能用 EvidencePack 构造 prompt，provider failure 回退 extractive，unsafe / malformed output fail closed | `service-briefs/rag-service.md` |
| `summary-service` | 已落第一版只读摘要路径、真实 adapter smoke、Summary negative eval adapter、guarded external HTTP LLM boundary 和 cross-group / temporal stack smoke | `summary_service.proto`、SDD、六层 skeleton、`GenerateConversationSummary` app / gRPC adapter、retrieval-gateway RPC client、`grpc` runtime mode、本地 Docker / compose / Prometheus / Grafana wiring、`loadtest/summary` 和真实本地 adapter smoke 已落地；第一版 deterministic extractive provider，保留 citations / EvidencePack，`generated_by_llm=false`，无 evidence 时拒绝摘要；真实本地 stack smoke 已验证 cross-group source refs / speaker attribution 和 stale / future memory exclusion；provider 输出后统一通过 citation verifier；可选 `external-http` provider 只能用 EvidencePack 构造 prompt，provider failure 回退 extractive，unsafe / malformed output fail closed | `service-briefs/summary-service.md` |
| `agent-service` | 已落第一版 proposal store / approval preflight / approval outbox relay / approval operator / planner Python candidate guard / cross-group temporal stack smoke | `agent_service.proto`、SDD、六层 skeleton、`CreateAgentProposal` / `ApproveAgentProposal` / `VerifyApprovedAgentProposal` app 和 gRPC adapter、PostgreSQL `agent_proposals` repository、低敏 `agent_approval_outbox`、`approval-outbox-relay`、`proposal-approval-audit` / `proposal-approval-approve` 本地 operator、`im.agent.events` schema、retrieval-gateway RPC client、mcp-gateway prepare RPC client、可选 `python-worker` proposal provider mode、`grpc` runtime mode、本地 Docker / compose / Prometheus / Grafana wiring、`loadtest/agent` 和真实本地 adapter smoke 已落地；当前 proposal path 已验证 cross-group source refs / speaker attribution 和 stale / future memory exclusion，且继续只生成 proposal、不执行业务 mutation；approval operator 默认 dry-run，reason 走文件且输出不含 proposal 正文 / EvidencePack / reason 原文；默认 deterministic extractive proposal，Python worker 只校验 proposal hash / citation metadata，Go 仍拥有最终 proposal / approval / audit，prepare / policy deny 时不检索证据，不执行工具动作 | `service-briefs/agent-service.md` |
| `skill-registry` | 已落第一版技能合约目录 | `skill_registry_service.proto`、SDD、migration、六层 skeleton、`UpsertSkill` / `GetSkill` / `ListSkills` app / gRPC adapter、PostgreSQL repository、`grpc` runtime mode、本地 Docker / compose / Prometheus / Grafana wiring 已落地；第一版只登记技能合约，不执行工具、不调用 MCP、不替代 policy-service | `service-briefs/skill-registry.md` |
| `mcp-gateway` | 已落第一版工具调用 prepare 边界 | `mcp_gateway_service.proto`、SDD、migration、六层 skeleton、`PrepareToolCall` app / gRPC adapter、skill-registry RPC client、policy-service RPC client、PostgreSQL 低敏 audit repository、`grpc` runtime mode、本地 Docker / compose / Prometheus / Grafana wiring 已落地；第一版只做 skill contract 校验、policy precheck 和 audit，不执行外部 MCP tool | `service-briefs/mcp-gateway.md` |
| `action-executor` | 已落第一版 approved execution audit + Agent approval preflight + result projection + local safe tool adapter + guarded external HTTP adapter + external adapter eval + preflight / rate-limit / DLQ-repair safety eval + provider failure worker | `action_executor_service.proto`、SDD、migration、六层 skeleton、`ExecuteApprovedAction` app / gRPC adapter、agent-service proposal verification RPC client、skill-registry RPC client、policy-service RPC client、PostgreSQL 低敏 execution audit repository、低敏 `action_executor_tool_results` projection、低敏 `action_executor_provider_failures` projection、bounded retry bookkeeping worker、`nexusim.local.echo` 本地安全 adapter、显式 allowlist + `LOW` risk 外部 HTTP provider adapter、外部 adapter eval / failure smoke、preflight safety smoke、rate-limited action blocked、rate-limiter unavailable fail-closed、repair/DLQ/redrive operator guard smoke、`grpc` runtime mode、本地 Docker / compose / Prometheus / Grafana wiring 已落地；第一版强制 proposal / approval / prepare audit 关联，并通过 `agent-service.VerifyApprovedAgentProposal` 校验 proposal 已批准且字段匹配；未配置 adapter 的业务 tool 仍 `executed=false`，repair/DLQ/redrive 类动作必须走 operator workflow，echo 和 allowlisted HTTP provider tool 可 `SUCCEEDED` 并只记录 output hash，provider failure 只记录 retry/DLQ 状态，不保存 raw input / provider output / provider 原始错误 | `service-briefs/action-executor.md` |
| `ai-eval-service` | 已落第一版持久化 eval run catalog、RecordEvalRun recorder、policy-driven multi-adapter gate smoke、Python optional adapter path、RAG / Agent service-stack live gate、Summary live negative adapter、CI-safe gate skeleton、第一批 RAG-Agent regression expansion、profile / Agent output safety expansion、service-stack version / hash-only expansion、negative RAG / Agent cases、Python/model-output negative cases、RAG/Summary citation source-ref regression、external MCP fallback eval cases、Agent output regression、action preflight / rate-limit / DLQ-repair safety eval、action provider failure worker / redrive safety eval、memory group source-ref / validity / supersession / cross-group / temporal eval、RAG / Summary / Agent current-memory consumption regression、memory extraction confidence / review eval、current-memory service-stack live smoke、cross-group / temporal retrieval smoke、RAG / Summary / Agent stack consumption smoke 和 40/40 optional stack gate | `ai_eval_service.proto`、SDD、migration、六层 skeleton、`RecordEvalRun` / `GetEvalRun` / `ListEvalRuns` app / gRPC adapter、PostgreSQL repository、`grpc` runtime mode、本地 Docker / compose / Prometheus / Grafana wiring、`ai-eval-record-smoke`、`run-ai-eval-record-run-smoke.ps1`、`gate-policy.local.json`、`validate-ai-eval-gate-policy.ps1`、`run-ai-eval-regression-gate-smoke.ps1`、`run-ai-eval-service-stack-gate-smoke.ps1` 和 `check-ai-eval-regression-gate.ps1` 已落地；case catalog 66 个 case，CI-safe fixture adapter 16 cases，optional stack gate 6 adapters / 40 cases passed；retrieval 和 RAG / Summary / Agent stack smokes 已验证跨群 source refs / speaker attribution 和 stale / future memory exclusion；CI-safe gate 只跑不依赖服务栈的 required adapters；第一版不保存 raw prompt / EvidencePack / model output | `service-briefs/ai-eval-service.md` |
| `media-service` | product-active，第一版 skeleton + PG hardening + gRPC smoke + outbox relay + processing worker 已落 | `media_service.proto`、PostgreSQL core migration、六层 skeleton、`CreateUploadSession` / `CompleteUpload` / `GetMediaAsset` / `GetMediaDownloadURL` / `DeleteMediaAsset` app + gRPC adapter、fake object-store port、PostgreSQL repository first pass、真实 PG upload / complete / delete / outbox / processing 集成测试、object_key 不出 public response / fake presign URL / outbox payload 的回归门禁、`grpc` / `processing-worker` runtime mode、Docker / Prometheus / Grafana wiring、`loadtest/media` 和 2026-06-20 最小 gRPC smoke 已落；`media_outbox -> im.media.events` Kafka schema、outbox store、Kafka producer、trigger relay、`outbox-relay` runtime mode、真实 PG relay 顺序 / retry / DLQ 测试、真实 Kafka readback smoke 和 mock scanner / thumbnail / transcode worker smoke 已落；第一版仍不做真实 S3、真实 scanner、真实 thumbnail/transcode provider 或 provider-grade download policy | `service-briefs/media-service.md` |
| `notification-service` | product-active，第一版 skeleton + accepted outbox + outbox relay + delivery worker 已落 | `notification_service.proto`、PostgreSQL core migration、六层 skeleton、`CreateNotificationRequest` / `GetNotificationStatus` / `CancelNotificationRequest` app + gRPC adapter、destination hash port、PostgreSQL repository first pass、accepted outbox 敏感字段不泄露回归测试、无 secret payload 普通请求写库回归测试、`grpc` / `delivery-worker` / `outbox-relay` runtime mode、Docker / Prometheus / Grafana wiring、`im.notification.events` Kafka schema、outbox store、Kafka producer、trigger relay、真实 PG relay publish / retry / DLQ / request-order blocker 测试、delivery store success / DLQ 测试、noop provider adapter、webhook provider adapter 和 2026-06-20 真实 Kafka / delivery smoke 已落；第一版仍不做 provider-grade email / SMS / APNs / FCM、bounce / suppression worker | `service-briefs/notification-service.md` |
| `audit-service` | product-active，第一版 append / query / proof 和最小 gRPC smoke 已落 | `audit_service.proto`、PostgreSQL core migration、六层 skeleton、`AppendAuditRecord` / `QueryAuditRecords` / `VerifyAuditProof` app + gRPC adapter、PostgreSQL repository、hash-chain proof、低敏 `audit.record.appended.v1` outbox、`grpc` runtime mode、Docker / Prometheus / Grafana wiring、最小 gRPC smoke 报告已落；第一版仍不做 Kafka ingestion、export worker、SIEM forwarding、segment sealing 或 retention cleanup | `service-briefs/audit-service.md` |
| `admin-service` | product-active，第一版 operation / approval 管理入口 + outbox relay + operation worker + operator CLI + config publish / rollback smoke 已落 | `admin_service.proto`、PostgreSQL core migration、六层 skeleton、`CreateAdminOperation` / `ApproveAdminOperation` / `GetAdminOperation` / `ListAdminOperations` app + gRPC adapter、PostgreSQL repository、high-risk separation-of-duty、低敏 `admin_outbox`、`grpc` / `operation-worker` / `outbox-relay` runtime mode、`im.admin.events` Kafka schema、outbox store、Kafka producer、trigger relay、本地 no-op executor、risk routing、`REPAIR_REQUEST -> workflow-service REPAIR_APPROVAL`、`CRITICAL -> workflow-service ADMIN_OPERATION`、operation-specific approval policy / target-service routing、非 critical `CONFIG_PUBLISH -> control-plane-service.PublishConfigVersion` adapter、非 critical `CONFIG_ROLLBACK -> control-plane-service.RollbackConfigVersion` adapter、`admin_operation_results` result 投影、`loadtest/admin` 公开 gRPC create / approve / reject / get / list operator CLI、本地多进程 config publish / rollback smoke、Docker / Prometheus / Grafana / worker compose wiring 已落；第一版仍不做其它真实下游 mutation、admin UI 或 provider-grade 运维 | `service-briefs/admin-service.md` |
| `control-plane-service` | product-active，第一版 config publish / rollback / snapshot / ACK 和最小 gRPC smoke 已落 | `control_plane_service.proto`、PostgreSQL core migration、rollback migration、六层 skeleton、`PublishConfigVersion` / `RollbackConfigVersion` / `GetConfigSnapshot` / `AckAppliedConfigVersion` app + gRPC adapter、PostgreSQL repository、DB-backed quota / feature snapshot、低敏 `control_outbox`、`grpc` runtime mode、Docker / Prometheus / Grafana wiring、最小 gRPC smoke 和 admin-driven config rollback smoke 报告已落；第一版仍不做 Kafka relay、drift monitor、expiry / cleanup worker 或 api-gateway consumer | `service-briefs/control-plane-service.md` |

## 剩余目标入口

剩余目标、P2 hardening、收口门禁和逐服务 backlog 已拆到 `remaining-goals.md`。

本文只回答“现在开发到哪一步”；不要在这里继续堆待办长句。

## 当前阶段判断

当前最准确的阶段判断是：

```text
前 9 个微服务已经能跑通 IM 主链路，
现在处于“Go 底座已支撑 AI 算法/eval，正式推进 collaborative-memory 算法切片”，
search-service v0.1 第一实现切片已继续推进到 PG repository / SearchMessages / grpc runtime / timeline consumer，并已跑通 clean projection smoke；
memory-service 已从 contract 切到 foundation-active implementation 并跑通 clean projection smoke；
retrieval-gateway / EvidencePack 第一轮真实 smoke、policy precheck、EvidencePack 字段 hardening 和 cross-group / temporal retrieval smoke 已落；
rag-service / summary-service / agent-service 已落 first path、真实 adapter smoke 和 Python worker candidate guard；
skill-registry、mcp-gateway、action-executor 和 ai-eval-service 已形成 Agent controlled execution / eval first-stage 闭环；
external MCP fallback、guarded external HTTP provider、tool output safety、profile / Agent output safety、RAG / Summary citation regression、Agent output regression、action preflight / rate-limit / DLQ-repair safety eval、action provider failure worker / redrive safety eval、memory group fixture eval、current-memory service-stack live gate、cross-group / temporal memory fixture eval、retrieval smoke、RAG / Summary / Agent stack consumption smoke 和 optional stack gate 已落；
后续继续扩展低敏 collaborative-memory 算法/eval，优先 multi-hop / temporal update / profile aggregation 边界。
短期生产级测试、完整 HA、长压和 sizing 不再作为当前转进阻塞，但仍留在 hardening backlog。
```

AI 底座采用 v3.0 口径：能力面固定、服务边界 ADR 演进。最关键的不变量是 facts / projections / retrieval / controlled execution 分层；`memory-service` 已把 group memory、source refs、speaker / audience scope、validity windows、supersedes、confidence 和 review state 推进到 first-stage source-backed projection，后续仍不能把群聊内容直接持久化成个人偏好或未经证据支持的 active memory。

下一步优先级和剩余目标统一看 `remaining-goals.md`，不要在本页重复维护。

## 维护规则

- 这里只写阶段结论，不堆长历史。
- 新增真实里程碑时才更新本页。
- 具体 smoke 证据写入 `docs/runbook/loadtest/<service>/`。
- 服务细节变化写入 `docs/runbook/service-briefs/<service>.md`。
