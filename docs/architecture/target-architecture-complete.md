# NexusIM 完整目标架构蓝图

本文是 NexusIM 的完整目标架构蓝图。它面向项目完善后的形态：不只是一个即时通信
后端，而是一个包含 IM 主链路、业务平台、数据平台、AI / Agent 平台和中间件平台
的分布式系统。

本文不是固定服务数量或中间件产品清单。服务和中间件可以持续演进，但必须遵守
边界、所有权、事件、数据治理、安全审计和 ADR 规则。

## 1. 架构目标

NexusIM 的目标形态是：

```text
高并发即时通信后端
  + 可复用业务平台
  + 可信数据平台
  + AI / RAG / Agent 应用平台
  + 平台工程化中间件与运维体系
```

系统需要支持：

- 实时消息、群聊、回执、durable inbox、在线唤醒；
- Web、Windows PC、Android 客户端通过稳定 BFF / push 边界接入；
- identity、policy、contacts、media、notification、admin、audit、workflow、
  control-plane 等业务能力复用；
- 群组 memory、检索、RAG、总结、多 Agent 协作和真实业务动作；
- 当前本地 / 双机 / 面试级分布式演示；
- 后续生产级 HA、观测、数据治理和中间件替换，而不重写服务边界。

## 2. 设计依据

本架构综合参考以下一手资料和工程实践：

- 微服务按业务能力拆分，并拥有自己的数据边界：
  <https://martinfowler.com/articles/microservices.html>
- SLO / SLI 驱动可靠性判断，而不是模糊地宣称“生产级”：
  <https://sre.google/sre-book/service-level-objectives/>
- 平台工程把通用能力作为内部平台产品提供：
  <https://tag-app-delivery.cncf.io/whitepapers/platform-eng-maturity-model/>
- Zero Trust 要求显式验证、最小权限和不信任默认网络：
  <https://csrc.nist.gov/pubs/sp/800/207/final>
- API 安全需要对象级和字段级授权：
  <https://owasp.org/API-Security/editions/2023/en/0x11-t10/>
- Data Mesh 强调领域拥有数据产品，共享自助式数据基础设施：
  <https://martinfowler.com/articles/data-mesh-principles.html>
- Lakehouse / 开放表格式支撑统一 BI / ML 数据底座：
  <https://www.cidrdb.org/cidr2021/papers/cidr2021_paper17.pdf>,
  <https://iceberg.apache.org/spec/>
- Transactional Outbox / Saga 是微服务一致性的基础模式：
  <https://microservices.io/patterns/data/transactional-outbox.html>
- CloudEvents / AsyncAPI 用于长期事件契约治理：
  <https://cloudevents.io/>, <https://www.asyncapi.com/>
- OpenTelemetry 是可观测性的厂商中立基线：
  <https://opentelemetry.io/docs/>
- MCP 为 AI 工具、资源和 prompt 接入提供标准边界：
  <https://modelcontextprotocol.io/docs/getting-started/intro>
- 长程协作记忆需要处理多人、多群组、时间演化和归因，而不是只做向量召回：
  <https://arxiv.org/abs/2602.01313>
- RAG / Multi-Agent 系统需要检索质量、证据绑定、协作协议、工具安全和评测：
  <https://arxiv.org/abs/2506.00054>,
  <https://arxiv.org/abs/2501.06322>

## 3. 总体分层

```text
客户端层
  Web / Windows PC / Android / future iOS

接入层
  api-gateway / client BFF / push-gateway / auth / rate limit / trusted metadata

IM 核心平台
  identity / policy / contacts / conversation / message / delivery / receipt

产品业务平台
  media / notification / presence / admin / audit / control-plane / workflow

事件与事实层
  per-service PostgreSQL / outbox / Kafka / schema contracts / DLQ / repair

数据平台
  CDC / ingestion / lakehouse / OLAP / data catalog / metrics / feature store

AI 与 Agent 平台
  search / vector-index / memory / retrieval / RAG / summary / agent
  skill-registry / MCP gateway / model-gateway / action-executor / ai-eval

中间件平台
  Redis / Kafka / PostgreSQL / OpenSearch / vector store / MinIO / Vault
  Keycloak / OpenFGA / Temporal / OTel / Prometheus / Grafana / future options
```

服务层拥有业务语义，中间件层提供能力。中间件产品可以替换，但领域所有权不能因为
替换中间件而变化。

## 4. 部署视图

```text
                 +-----------------------------+
                 | Web / PC / Android clients  |
                 +--------------+--------------+
                                |
                 +--------------v--------------+
                 | api-gateway / client BFF    |
                 | auth, quota, trusted meta   |
                 +--------------+--------------+
                                |
          +---------------------+----------------------+
          |                                            |
+---------v----------+                      +----------v---------+
| IM business APIs   |                      | push-gateway       |
| gRPC / HTTP BFF    |                      | WebSocket wakeup   |
+---------+----------+                      +----------+---------+
          |                                            |
          +---------------------+----------------------+
                                |
                 +--------------v--------------+
                 | service-owned PostgreSQL    |
                 | outbox -> Kafka -> workers  |
                 +--------------+--------------+
                                |
          +---------------------+----------------------+
          |                                            |
+---------v----------+                      +----------v---------+
| data platform      |                      | AI / Agent platform|
| analytics / BI     |                      | retrieval / action |
+--------------------+                      +--------------------+
```

本地开发通过 Docker profile 组合启动。未来生产环境可以映射到 Kubernetes、
托管数据库、托管 Kafka、托管对象存储和托管观测系统，但服务边界不应因此改变。

## 5. 核心所有权不变量

1. 每个服务拥有自己的数据库 schema。
2. 生产代码不能读取其他服务的私有表。
3. 跨服务同步调用只能走公开 API 或明确 port。
4. 跨服务异步集成只能走公开事件和 schema 版本。
5. Kafka 是事件传播面，不是权威事实源。
6. 数据平台只消费事实，不执行业务 command。
7. AI / Agent 不能绕过 policy、EvidencePack、approval 和 audit。
8. Python worker 只返回候选结果；Go 服务拥有控制面、事实和审计。
9. 客户端本地存储只是缓存 / 离线队列，不是服务端事实源。
10. 中间件作为平台能力引入，不作为某个服务的私有堆叠。
11. NexusIM 不使用 recovery 作为隐藏业务语义；依赖、权限、事实源或投影不确定时必须
    fail-closed、retry / repair，或回到对应事实源执行 recovery。详细规则见
    `docs/architecture/fail-closed-policy.md`。

## 6. 领域地图

### 6.1 接入域

| 组件 | 职责 |
| --- | --- |
| `api-gateway` | 公共 API facade、client BFF、鉴权、配额、trusted metadata、低敏观测。 |
| `push-gateway` | WebSocket 在线唤醒、Redis route、session 生命周期、best-effort online notify。 |

接入层终结公网信任。下游服务只信任 gateway 生成或内部边界验证过的 metadata，
不能信任客户端任意传入的 header。

### 6.2 IM 核心域

| 服务 | 拥有的事实 |
| --- | --- |
| `identity-service` | 用户、凭证、session、refresh token、MFA、OIDC / JWKS / challenge。 |
| `policy-service` | 授权策略、ReBAC 边、policy decision、risk / moderation policy。 |
| `contacts-service` | 联系人关系、隐私设置、联系人分组。 |
| `conversation-service` | 会话、群、成员、角色、成员边界、owner transfer。 |
| `message-service` | 消息日志、编辑、撤回、删除、附件引用、message outbox。 |
| `delivery-service` | durable user inbox、device cursor、delivery event、projection checkpoint。 |
| `receipt-service` | 已读 / 送达回执、未读基础、会话列表摘要。 |

这些服务构成当前 IM 产品行为的核心底座。

### 6.3 产品业务域

| 服务 | 职责 |
| --- | --- |
| `media-service` | 上传、对象存储、缩略图、病毒扫描、转码、下载策略。 |
| `notification-service` | 邮件、短信、APNs / FCM、模板、provider routing、bounce / suppression。 |
| `presence-service` | 在线状态、输入中、最后在线、设备状态、隐私感知 presence。 |
| `admin-service` | 租户 / 管理 API、repair 审批、用户治理、operator action。 |
| `audit-service` | 登录、安全、管理、Agent action、repair 审计、导出和 retention。 |
| `control-plane-service` | 租户配置、功能开关、灰度、quota、策略 / 配置发布。 |
| `workflow-service` | 长事务、审批、补偿、timer、external callback。 |

这些是业务平台服务。只有产品范围真正需要时才逐个 promotion，不因为架构图好看而提前铺空服务。

### 6.4 AI 与 Agent 域

| 服务 | 职责 |
| --- | --- |
| `search-service` | 搜索投影写入和关键词检索。 |
| `vector-index-service` | 向量投影写入、重建、pgvector / Milvus / OpenSearch adapter。 |
| `memory-service` | 带 source refs 的个人 / 群组 / 项目长期记忆状态。 |
| `retrieval-gateway` | 混合检索、可见性过滤、EvidencePack 构造。 |
| `rag-service` | 基于 EvidencePack 回答，返回引用和不确定性。 |
| `summary-service` | 会话、未读、项目总结，必须带 source references。 |
| `agent-service` | 规划、多 Agent 协作和 Agent run state。 |
| `skill-registry` | 工具 / skill 能力目录、风险等级、调用 metadata。 |
| `mcp-gateway` | MCP tool / resource / prompt 边界和 consent enforcement。 |
| `model-gateway` | LLM、embedding、rerank provider 路由、预算、recovery 和审计。 |
| `action-executor` | 只通过公开 API 执行已审批的业务动作。 |
| `ai-eval-service` | 数据集、回归运行、RAG / memory / Agent 评测、安全门禁。 |

AI 服务消费投影和 EvidencePack，不能成为另一套业务事实源。

### 6.5 数据平台域

当 analytics、RAG、风控、运营和 BI 的需求超过服务本地 debug metrics 时，再引入数据平台服务。

| 服务 | 职责 |
| --- | --- |
| `data-ingestion-service` | 消费公开事件 / CDC，写入治理后的分析记录。 |
| `data-catalog-service` | 登记数据产品、owner、schema、血缘、retention、privacy class。 |
| `analytics-service` | 提供产品、运营和业务指标 API。 |
| `feature-store-service` | 提供低敏 risk / ranking / Agent feature。 |
| `data-quality-service` | 监控 freshness、缺失事件、schema drift、质量检查。 |

数据平台偏读和分析。业务 command 仍必须回到业务服务。

## 7. IM 主链路

### 7.1 发消息链路

```text
client
  -> api-gateway BFF / GatewayService
  -> message-service.SendMessage
  -> conversation-service.GetSendContext
  -> message_log + message_outbox in local transaction
  -> outbox relay -> Kafka conversation.timeline.events
  -> delivery-service projection -> user_inbox + delivery_outbox
  -> delivery outbox relay -> Kafka im.delivery.events
  -> push-gateway -> WebSocket delivery.notify
  -> client PullInbox -> AckDelivery
```

客户端展示消息以 `delivery-service.PullInbox` 为准，不能只依赖 WebSocket payload。
WebSocket 是在线唤醒，不是 durable data。

### 7.2 群成员链路

```text
client/admin
  -> conversation-service.CreateMemberChange
  -> local saga + membership state + timeline event + outbox
  -> Kafka member boundary event
  -> delivery membership projection
  -> search/memory visibility projection
  -> audit / analytics / policy projections
```

成员窗口必须影响 delivery、search、retrieval 和 memory。不能用当前成员状态去重写历史可见性。

### 7.3 回执链路

```text
client AckDelivery / MarkRead
  -> delivery-service / receipt-service
  -> device cursor / receipt facts
  -> receipt events
  -> conversation list / unread projection
  -> notification / analytics / AI summary consumers
```

## 8. 客户端架构

客户端共享 TypeScript 协议和同步核心：

```text
clients/packages/protocol
clients/packages/client-core
clients/web
clients/desktop
clients/android
```

规则：

- 客户端只调用 `api-gateway` BFF 和 `push-gateway`。
- `client-core` 拥有本地同步状态、离线队列语义和 API model。
- 浏览器使用 Web APIs。
- PC 使用 Tauri 作为薄壳。
- Android 使用薄平台 bridge。
- Native bridge 不实现业务决策。
- 本地存储只做缓存和 pending operation state。

## 9. 事件架构

每个产事件服务优先使用本地事务 + outbox，保证第一阶段可靠发布。

事件 envelope 建议：

```text
event_id
tenant_id
producer
aggregate_type
aggregate_id
aggregate_version
event_type
event_version
partition_key
occurred_at
correlation_id
causation_id
trace_id
payload_json/protobuf
```

长期要求：

- Kafka payload 使用 protobuf schema；
- 事件文档逐步向 AsyncAPI 风格 channel 描述收敛；
- 必要时保持 CloudEvents-compatible metadata；
- 每类事件都需要 DLQ / retry / repair / replay 策略；
- 敏感 payload 不能进入报告、metrics、review page 或低敏 manifest。

## 10. 数据平台架构

数据平台基于公开事件和 CDC，不直接 join 业务私表。

```text
Public events / CDC
  -> ingestion validation
  -> raw event store
  -> domain normalized tables
  -> aggregate data products
  -> metrics / BI / risk / feature / RAG use cases
```

数据产品契约：

```text
name
owner
source events / source systems
schema version
freshness target
privacy class
retention
quality checks
consumer list
repair / backfill procedure
```

AI 检索不能直接构建在任意 operational table 上。必须通过明确的 search / vector /
memory projection，并带上可见性、删除、撤回和 supersession 语义。

## 11. AI / Memory / RAG 架构

### 11.1 群组 Memory 模型

Memory record 必须保留上下文：

```text
source_ref
tenant_id
conversation_id / group_id / project_id
speaker_id
audience_scope
fact_type
fact_text
status: draft | active | superseded | archived | deleted
valid_from / valid_to
supersedes
related_events
visibility_window
confidence
review_state
evidence_refs
```

规则：

- 群聊中的一句话不能自动升级成某个人的个人偏好。
- 旧决策必须被 supersede，不能只是把新旧事实并列塞进向量库。
- 删除、撤回、成员离开必须影响 search、vector 和 memory 的可见性。
- 检索必须返回 source refs 和纳入原因。

### 11.2 Retrieval 和 EvidencePack

```text
request
  -> policy check
  -> structural filters: tenant / group / user / time / visibility
  -> keyword search
  -> vector search
  -> graph / related-event expansion
  -> rerank
  -> EvidencePack
  -> RAG / summary / Agent
```

EvidencePack 包含：

```text
evidence_id
source_refs
conversation_seq / message_id / object refs
visibility proof
retrieval score
version / valid time
redaction profile
```

当 policy-aware retrieval gateway 已存在时，RAG 和 summary 不能直接调用 search /
vector store。

### 11.3 Agent 架构

```text
Agent request
  -> intent classification
  -> retrieval EvidencePack
  -> plan candidate
  -> policy precheck
  -> proposal
  -> approval if needed
  -> action-executor
  -> audit
  -> event
```

可以使用 multi-agent 角色，但协作必须由服务编排：

- planner；
- retrieval specialist；
- risk / policy reviewer；
- tool / action executor；
- evaluator / critic。

只有审批通过的 action 才能修改业务状态。

## 12. 中间件平台

中间件按能力管理。完整 catalog 和引入 checklist 见：
`../platform/middleware-catalog.md`。

Runtime profile：

| Profile | 范围 |
| --- | --- |
| `core` | PostgreSQL、Kafka、Redis 和 IM core services。 |
| `client-demo` | client BFF、push 和客户端 smoke 链路。 |
| `observability` | Prometheus、Grafana、Alertmanager、OTel collector。 |
| `search-rag` | OpenSearch / vector store / retrieval / RAG / model gateway。 |
| `media` | MinIO 和 media processing 依赖。 |
| `workflow-agent` | workflow、Agent、skill、MCP、action execution。 |
| `security` | OIDC provider、Vault/KMS emulator、OpenFGA/OPA。 |
| `data-platform` | CDC、lakehouse、OLAP、analytics。 |
| `ai-runtime` | 本地或远程模型 provider proxy。 |

不要默认启动所有中间件。每个 active slice 只选择它的最小 runtime profile。

## 13. 安全架构

安全边界：

- 公网边界：`api-gateway`、`push-gateway`、client assets。
- 身份边界：`identity-service`、OIDC / JWKS、session / MFA。
- 授权边界：`policy-service`、gateway metadata、每个服务自己的 ownership check。
- 数据边界：服务自有 PostgreSQL、事件契约、数据产品隐私等级。
- AI action 边界：retrieval、policy、workflow approval、action executor、audit。

安全规则：

1. 公网 listener 不能使用 mock auth 或明文 secret，除非显式限定 local / private。
2. Trusted metadata 只能由 gateway 或已验证内部边界生成。
3. 每个 public API 都必须做对象级授权。
4. 敏感值不能进入日志、导出、metrics、报告或 Git 提交。
5. Tool / MCP / Agent action 必须有 capability metadata、policy 和 audit。
6. KMS/HSM/Vault 是 adapter 后面的能力，不是写死在 domain 里的假设。

## 14. 可靠性和运维

第一阶段本地证据不是生产 SLO 证明。走向生产需要：

- 为用户可感知路径定义 SLI，例如 login、send、PullInbox、push wakeup、
  search retrieval、Agent action latency；
- 积累足够运行数据后再定义 SLO；
- 为关键链路提供 dashboard、alert、runbook 和 error budget；
- 每条事件驱动 workflow 都有 DLQ、repair、replay、audit；
- 每个 durable store 都有 backup、restore、failover 和数据完整性演练；
- 配置和服务变更有 canary、rollout、rollback gate。

## 15. 代码组织

```text
services/<service>/
  cmd/
  internal/api/
  internal/app/
  internal/domain/
  internal/infrastructure/
  internal/trigger/
  internal/types/

clients/
  packages/protocol/
  packages/client-core/
  web/
  desktop/
  android/

ai/python/
  workers/
  eval/
  algorithms/
  tools/

deploy/
  local/
  docker/

docs/
  architecture/
  platform/
  sdd/
  runbook/
```

共享包必须至少有两个真实调用方和稳定契约。不要为了让架构图对称而提前抽象。

## 16. 语言边界

| 范围 | 语言 |
| --- | --- |
| 后端服务、BFF、控制面、审计、持久事实 | Go |
| Web / PC / Android 共享客户端核心和 UI | TypeScript |
| Tauri 桌面桥 | Rust，只做薄桥 |
| Android 平台桥 | Kotlin，只做薄桥 |
| iOS 未来平台桥 | Swift，只做薄桥 |
| AI worker、模型算法、离线 eval | Python |

Python 不能拥有业务事实、安全决策或审计真相。

## 17. 演进路线

### Phase A：稳定 IM 和客户端 MVP

- 保持当前 IM 服务作为可运行后端。
- 完成 Web / PC / Android 客户端 shell，共用 client core。
- BFF 和 push 是唯一客户端后端入口。

### Phase B：产品业务平台完善

- 按产品需要逐步 promotion media、notification、presence、admin、audit、
  control-plane、workflow。
- 每个 promotion 都按服务切片闭环：SDD、migration、API、smoke、docs。

### Phase C：AI 数据边界

- 强化 search、vector-index、memory、retrieval。
- 固定 visibility、delete、supersession 和 EvidencePack 契约。

### Phase D：RAG 和 Agent 应用

- RAG 和 summary 只能基于 EvidencePack。
- Agent action 必须走 workflow approval 和 action-executor。
- 扩 autonomous action 前先扩 ai-eval。

### Phase E：数据平台

- 从公开事件 / CDC 构建 ingestion 和 analytics。
- 增加 data catalog、quality checks、feature products。

### Phase F：生产化运维

- 按需要把 local-only 中间件替换成 managed 或 HA profile。
- 补 SLO、告警、备份恢复、故障切换和 rollout 治理。

## 18. 服务 / 中间件新增规则

新增服务或中间件至少满足一个条件：

1. 拥有独立数据模型。
2. 拥有独立伸缩特征。
3. 拥有独立故障或安全边界。
4. 多个服务需要同一种能力。
5. 能显著降低现有服务复杂度。

每次新增必须包含：

- ADR 或 SDD 章节；
- public API / event contract；
- 数据所有权说明；
- runtime profile 或明确 deferred-runtime note；
- focused validation；
- rollback / migration / compatibility note。

## 19. 本架构避免什么

- 一个巨大的“中台服务”。
- AI 服务直接读 operational private tables。
- 客户端直接调用内部微服务。
- 数据平台变成隐藏 command side。
- domain / app 层依赖具体中间件 client。
- Python worker 拥有持久业务状态。
- 把服务数量或中间件产品写成终局答案。

## 20. 面试讲述口径

NexusIM 可以这样描述：

```text
NexusIM 是一个以即时通信为核心的分布式后端系统。它先完成消息、会话、投递、
在线通知和回执主链路，再沉淀身份、权限、媒体、通知、审计、控制面和 workflow
等业务平台能力；同时通过公开事件和 CDC 构建数据平台；最后在权限过滤的
EvidencePack 之上构建群组 memory、RAG、总结和可审计的 Agent action。

系统把业务事实放在 Go 服务中，把分析数据放在治理后的数据产品中，把 AI 证据放在
retrieval gateway 生成的 EvidencePack 中，把真实写动作放在 approval 和 audit 之后。
```
