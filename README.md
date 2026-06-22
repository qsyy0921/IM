# NexusIM

NexusIM 是面向企业协同场景的分布式 IM + AI 协作平台。当前仓库已经从普通 IM demo 推进到：

```text
本地 / 双机可运行的分布式 IM 后端
-> Web / Windows PC / Android client platform first slice
-> group memory / EvidencePack / RAG / summary / Agent 应用底座
-> skill registry / MCP gateway / action executor / proposal approval / audit
-> 完整目标架构：业务平台 + 数据平台 + AI / Agent 平台 + 中间件平台
```

GitHub 首页只放当前总览。Codex 目标框正文只保存在目标框本身，不在仓库里重复维护；
每轮继续开发时按 [prompt.md](prompt.md) 和 [agent.md](agent.md) 路由读取
[current-goal.md](docs/runbook/current-goal.md)、[current-brief.md](docs/runbook/current-brief.md)
和 [remaining-goals.md](docs/runbook/remaining-goals.md)。

长期完整架构以 [target-architecture-complete.md](docs/architecture/target-architecture-complete.md)
为准。后续新增业务服务、数据平台服务、AI / Agent 服务或中间件时，都要按这份蓝图的
所有权、事件、数据、安全和演进规则推进；中间件能力登记见
[middleware-catalog.md](docs/platform/middleware-catalog.md)。

README 是 GitHub 首页和面试前快速入口，必须跟随当前开发进度维护。凡是 active slice、
服务 promotion、客户端能力、AI / Agent 平台边界或“下一步”发生实质变化，都要同步
更新本文件；长历史证据仍放 `docs/runbook/loadtest/` 和 `docs/runbook/archive/`。

## 当前架构设计

NexusIM 当前按“事实源清晰、事件驱动、读模型独立、AI 受控执行”的方式组织。根 README
只放架构总览；完整分层、数据平台和 AI / Agent 设计见
[target-architecture-complete.md](docs/architecture/target-architecture-complete.md)。

### 中文架构示意图

下面几张图用于快速讲解架构设计，采用白皮书 / 白板风格，不作为精确接口契约；
精确的服务边界、字段、事件和运行门禁以本文表格、SDD、ADR 和 runbook 为准。

![NexusIM 完整架构](docs/assets/architecture/nexusim-architecture-cn.png)

![NexusIM 消息主链路](docs/assets/architecture/nexusim-message-flow-cn.png)

![NexusIM AI Agent 受控架构](docs/assets/architecture/nexusim-ai-agent-cn.png)

![NexusIM 技术与中间件能力目录](docs/assets/architecture/nexusim-middleware-cn.png)

```mermaid
flowchart TB
  subgraph Client["客户端层"]
    Web["Web"]
    PC["Windows PC"]
    Android["Android"]
  end

  subgraph Access["接入层"]
    BFF["api-gateway / Client BFF"]
    Push["push-gateway / WebSocket"]
  end

  subgraph Core["IM 核心事实层"]
    Identity["identity-service"]
    Policy["policy-service"]
    Contacts["contacts-service"]
    Conversation["conversation-service"]
    Message["message-service"]
    Delivery["delivery-service"]
    Receipt["receipt-service"]
  end

  subgraph Event["事件与投影层"]
    PG["Service-owned PostgreSQL"]
    Outbox["Transactional Outbox"]
    Kafka["Kafka topics"]
    ReadModel["Durable read models"]
  end

  subgraph Product["产品平台服务"]
    Media["media-service"]
    Notification["notification-service"]
    Presence["presence-service"]
    Admin["admin-service"]
    Audit["audit-service"]
    Control["control-plane-service"]
    Workflow["workflow-service"]
  end

  subgraph AI["AI / Agent 平台"]
    Search["search-service"]
    Memory["memory-service"]
    Retrieval["retrieval-gateway / EvidencePack"]
    RAG["rag-service / summary-service"]
    Agent["agent-service"]
    Skill["skill-registry / mcp-gateway"]
    Executor["action-executor"]
    Eval["ai-eval-service"]
    Python["Python AI Worker"]
  end

  Web --> BFF
  PC --> BFF
  Android --> BFF
  Web --> Push
  PC --> Push
  Android --> Push

  BFF --> Identity
  BFF --> Policy
  BFF --> Contacts
  BFF --> Conversation
  BFF --> Message
  BFF --> Delivery
  BFF --> Receipt
  Push --> Delivery

  Core --> PG
  Product --> PG
  PG --> Outbox
  Outbox --> Kafka
  Kafka --> ReadModel
  Kafka --> Search
  Kafka --> Memory
  Kafka --> Audit

  Search --> Retrieval
  Memory --> Retrieval
  Policy --> Retrieval
  Retrieval --> RAG
  Retrieval --> Agent
  Skill --> Agent
  Agent --> Workflow
  Agent --> Executor
  Executor --> Audit
  Python --> RAG
  Python --> Agent
  Eval --> RAG
  Eval --> Agent
```

### 分层职责

| 层级 | 当前设计 |
| --- | --- |
| 客户端层 | Web / Windows PC / Android 共用 TypeScript `protocol` 和 `client-core`；native shell 只做薄平台 bridge。 |
| 接入层 | `api-gateway` 提供 client BFF、鉴权、quota、trusted metadata；`push-gateway` 只做在线唤醒，不拥有 durable inbox。 |
| IM 核心层 | 9 个 IM 服务分别拥有身份、策略、联系人、会话、消息、投递、回执等事实和读模型。 |
| 事件与投影层 | 每个服务拥有自己的 PostgreSQL schema；跨服务事实传播走 outbox -> Kafka -> projection / worker。 |
| 产品平台层 | media、notification、audit、admin、control-plane、presence、workflow 等按独立数据模型和故障边界逐步 promotion。 |
| AI / Agent 层 | search / memory 产出可见投影，retrieval-gateway 构造 EvidencePack，RAG / summary / Agent 只能基于 EvidencePack 工作。 |
| Python AI Worker | 只做模型、算法、embedding、rerank、memory extraction、planner 和 eval 候选；Go 继续拥有权限、状态、审批、审计和持久化。 |
| 中间件平台 | PostgreSQL、Kafka、Redis、OpenSearch / vector store、对象存储、观测、安全组件、数据平台和 AI runtime 都按能力与 runtime profile 引入，不写死产品。 |

### 当前客户端状态

客户端当前主线是 Web / Windows PC 优先、Android 后置。Web / PC shell 已接账号登录、
注册、好友申请、好友列表、点击好友发起私聊、群聊列表、建群、点击群聊进入会话、
群成员添加 / 退群、成员列表、成员搜索 / 角色过滤 / 分页、移除成员、角色变更 / owner transfer 第一路径、
群设置操作区、消息列表、发送后本地状态刷新、PullInbox 和 ACK。2026-06-23 的
clean smoke 已验证双用户好友直聊和群聊 first path；随后 Web / PC shell 又补了
第一版会话展示标题、空态、常见错误中文文案和显式本地启动脚本。Windows desktop
collected package 现在包含低敏 README；standalone exe package 还包含
package-local PowerShell launcher，install plan 会校验这些 support files 并给出人工启动命令；
artifact manifest 现在区分 `desktop-executable` / `desktop-installer`；
`bundle:desktop` 只打包 `desktop-executable`，可以产出 unsigned local portable zip 和低敏 summary；
`plan:desktop-installer` / `plan:desktop-signing` 可以检查 Tauri installer 和显式
签名输入 readiness；`sign:desktop-artifact` 已提供显式 `--execute` 门控的签名
执行入口，默认仍只输出低敏 plan，release signing 可加 `--require-valid` 在签名后
立即执行 fail-closed 验证；`verify:desktop-signature` 已提供只读
Authenticode 验证入口；2026-06-23 已重新 collect 新格式 `desktop-executable`
artifact，manifest 为 `clients/artifacts/2026-06-22T214826Z/manifest.json`，当前签名状态为 `NotSigned`；同日只读 signing plan 已确认本机 Windows Kits
`signtool` 可显式定位，传入 timestamp URL 后剩余 release 阻塞是代码签名证书来源和 valid signature；`build:desktop-installer` 已提供显式 `--execute` 门控的
installer build 包装器，并通过独立仓库 installer profile 调用 Tauri。默认开发
Tauri config 仍保持不打包；installer profile 显式启用 MSI + NSIS targets；signing /
signature verification / installer planner 会按 `artifactKind` 精确选择 collected
artifact，默认 executable，installer 必须显式请求；signing readiness 或 valid
signature 不满足时 fail-closed。generic install plan 也按 `artifactKind`
fail-closed：旧 manifest 缺 kind 时要求重新 collect，installer 不会被当作
portable launcher 或 direct shell-smoke 输入；installer planner 也只接受显式
`desktop-executable` 作为 MSI / NSIS build baseline。

本地调试入口：

```powershell
.\clients\start-local-backend.ps1
.\clients\start-local-web.ps1
```

客户端仍只连接 `api-gateway` BFF 和 `push-gateway`，不直连内部服务；PullInbox 是
消息展示事实源，WebSocket 只做在线唤醒。

### 当前技术 / 中间件能力目录

NexusIM 的中间件和技术栈会随着功能持续增加。README 只记录当前判断和引入规则，
不是终局清单；新增或替换中间件时必须同步
[middleware-catalog.md](docs/platform/middleware-catalog.md)、对应 SDD / ADR、runtime
profile 和最小 smoke。

| 能力 | 当前已用 / 已有路径 | 后续可替换或增强方向 | 边界 |
| --- | --- | --- | --- |
| 交易事实源 | PostgreSQL；每个服务拥有自己的 schema 和 migration | 云 PostgreSQL、分片、只读副本、HA / failover | 业务事实归服务所有，不跨服务读私表。 |
| 事件传播 | Kafka + protobuf schema + transactional outbox relay | Schema Registry、AsyncAPI / CloudEvents 元数据、Pulsar 候选 | Kafka 是传播面，不是权威事实源。 |
| 临时状态 / 路由 / 缓存 | Redis single / Sentinel / Cluster；push route、resume、quota / presence 场景 | Redis Cluster 深化、托管 Redis、局部内存 cache | Redis 不保存 durable business facts。 |
| 搜索 | search-service projection；当前以服务内 adapter / PostgreSQL first path 为主 | OpenSearch、Elasticsearch、Meilisearch | 只有 search-service 写搜索索引。 |
| 向量索引 | vector-index-service；PostgreSQL metadata、pgvector optional adapter、embedding queue | Milvus、Qdrant、Weaviate、OpenSearch vector | 向量是可重建 projection，不是消息事实源。 |
| 对象存储 / 媒体 | media-service fake object-store port；MinIO / S3-compatible 预留 | S3、MinIO、Ceph、病毒扫描、缩略图、转码 provider | 二进制不放 message-service。 |
| 工作流 / 审批 | workflow-service 内部状态机、compensation instruction registry | Temporal、Cadence、Argo Workflows | 审批和补偿审计归 workflow / audit 所有。 |
| 身份 / 联邦 | identity-service、JWKS、OIDC discovery first path | Keycloak、OIDC providers、多 issuer、WebAuthn/passkeys | api-gateway 仍负责 trusted metadata 边界。 |
| 策略 / ReBAC | policy-service、tool policy precheck、关系投影 first path | OpenFGA、OPA、DSL / quota 策略中心 | 外部策略引擎只能作为 adapter，不能绕过 policy-service。 |
| 密钥 / 机密 | 本地 env / 文件 guard；生产语义已预留 | Vault、云 KMS、HSM、密钥轮换 | secret 不写入 docs、metrics、报告或事件 payload。 |
| 可观测性 | `/metrics`、Prometheus rules、Grafana dashboard、OTel first-stage wiring | OTel Collector、Loki、Tempo、Alertmanager、SLO / retention | 本地观测是开发证据，不等于生产 SLO。 |
| 数据平台 | 当前仍是目标架构能力；公开事件 / CDC 消费边界已定义 | Debezium、Flink、Iceberg、Delta、Trino、ClickHouse / Doris | 数据平台不能成为业务 command 写入口。 |
| AI runtime | model-gateway、Python AI Worker、mock / guarded external HTTP provider | OpenAI / Claude / 本地模型、vLLM、Ollama、Triton、LiteLLM | 业务服务不直连模型 provider。 |
| 图 / 知识关系 | memory-service 结构化 memory、source refs、supersedes、profile aggregate | Neo4j、图投影、关系图索引 | 只有关系查询收益明确时才引入独立图能力。 |
| MCP / 工具 | skill-registry、mcp-gateway prepare、action-executor | 更多 MCP tool servers、外部业务工具 | 工具调用必须经过 skill metadata、policy、approval 和 audit。 |

### 关键链路

```text
发消息：
client -> api-gateway -> policy-service -> conversation-service -> message-service
-> PostgreSQL message_log + message_outbox -> Kafka conversation.timeline.events
-> delivery-service projection -> user_inbox -> PullInbox / AckDelivery
-> delivery_outbox -> im.delivery.events -> push-gateway online notify
```

```text
AI / RAG / Agent：
message / conversation / policy events -> search-service + memory-service projections
-> retrieval-gateway policy check + visibility filter + EvidencePack
-> rag-service / summary-service answer with citations
-> agent-service proposal
-> workflow approval
-> action-executor approved action
-> audit-service durable audit
```

### 架构原则

- 不跨服务读私有表；跨服务只走公开 API、事件或明确 port。
- 业务事务不直接 publish Kafka；统一走 transactional outbox。
- 客户端展示消息以 `PullInbox` 为事实源，WebSocket 只做在线通知。
- RAG / summary / Agent 不能直接读 message / conversation 私表，只能消费权限过滤后的 EvidencePack。
- Agent 真实写动作必须经过 policy、proposal、approval、action-executor 和 audit。
- 新服务和中间件不写死，必须满足独立数据模型、独立伸缩、独立故障边界、独立安全边界或明显降低复杂度。
- 每个新功能先做简短架构分析，再编码；先确认 owner、数据所有权、API / 事件、
  权限 / 审计、是否需要新技术 / 新中间件 / 新 provider、平台归属和文档影响。
- 新增中间件进入中间件平台；数据处理进入数据平台；模型、向量、检索、RAG、
  Agent 和 Python worker 进入 AI / Agent 平台；客户端交互进入客户端平台；
  业务产品能力进入业务 / 产品平台；运维控制能力进入 ops / control-plane 平台。
- 开发相关路径时持续清理不符合 fail-closed policy 的隐藏备用路径；新代码不得新增
  隐藏业务备用路径。

## 当前状态

已进入真实链路的 9 个 IM 后端服务：

| 服务 | 作用 |
| --- | --- |
| `api-gateway` | 外部入口、认证透传、租户 quota、legacy descriptor 迁移门禁。 |
| `identity-service` | 登录、Refresh、MFA、recovery code、JWKS、challenge delivery。 |
| `message-service` | 发消息、编辑 / 撤回 / 删除、timeline / outbox。 |
| `conversation-service` | 会话、成员边界、owner transfer、发送上下文。 |
| `delivery-service` | durable inbox、`PullInbox`、`AckDelivery`、delivery outbox。 |
| `push-gateway` | WebSocket 在线通知、Redis route、resume / PullInbox recovery。 |
| `receipt-service` | 已读 / 送达回执、会话列表、未读 / 置顶 / 静音等读模型。 |
| `contacts-service` | 联系人请求、隐私策略、分组 / 搜索、来源风险。 |
| `policy-service` | 策略决策、ReBAC first path、moderation、tenant quota、tool policy precheck。 |

已启动并持续推进的 AI / Agent 应用底座：

| 服务 / 模块 | 当前状态 |
| --- | --- |
| `search-service` | 搜索 projection、visibility / tombstone、`SearchMessages`、timeline consumer、projection smoke。 |
| `memory-service` | group memory projection、StructuredMemoryEvent、source refs、visibility window、revoke hidden。 |
| `retrieval-gateway` | EvidencePack 统一边界，聚合 search / memory / policy precheck，不直接调用 LLM。 |
| `rag-service` | 只读问答 first path、EvidencePack citation verifier、guarded external HTTP LLM boundary。 |
| `summary-service` | 只读摘要 first path、EvidencePack citation verifier、guarded external HTTP LLM boundary。 |
| `agent-service` | proposal-only path、mcp-gateway prepare、approval workflow、approval outbox relay、planner Python candidate guard。 |
| `skill-registry` | 技能目录、输入输出合约、风险等级、审批要求和审计元数据。 |
| `mcp-gateway` | tool prepare 边界、skill catalog check、policy precheck、低敏 audit，不直接执行外部工具。 |
| `action-executor` | approved execution audit、proposal / approval / prepare audit 校验、本地安全 adapter、guarded external HTTP provider adapter、eval smoke。 |
| `ai/python` | Python AI Worker 候选层：contract guard、低敏 safety guard、candidate-only worker CLI、`IM` conda toolchain。 |

已进入 product-active first-stage 的平台 / 产品服务：

| 服务 | 当前状态 |
| --- | --- |
| `media-service` | 上传会话、完成上传、资产查询、下载 URL、删除、media outbox relay、mock processing worker first path。 |
| `notification-service` | 通知请求事实源、状态查询、取消、accepted outbox、notification outbox relay、noop / webhook delivery worker first path。 |
| `audit-service` | append / query、hash-chain proof、admin-event ingestion、first-stage export job metadata。 |
| `admin-service` | 管理操作创建 / 审批 / 查询、operation worker、outbox relay、control-plane / audit adapter、workflow compensation request handoff。 |
| `control-plane-service` | config publish / rollback / snapshot / ACK、DB-backed quota / feature snapshot。 |
| `presence-service` | `UpdatePresence`、`GetPresence`、`UpdateTyping`、PostgreSQL presence projection、低敏 presence outbox。 |
| `model-gateway` | text generation / embedding invocation metadata、mock provider、低敏 invocation outbox，不持久化 raw prompt 或 embedding vector。 |
| `knowledge-ingestion-service` | knowledge source、ingestion job、chunk manifest、knowledge outbox relay、vector handoff first path。 |
| `workflow-service` | workflow creation / decision、approval状态机、compensation request / instruction registry / rollback adapter first path。 |
| `vector-index-service` | vector metadata、tombstone、search、rebuild request / checkpoint、embedding queue / worker、knowledge chunk consumer first path。 |

已启动的客户端平台：

| 模块 | 当前状态 |
| --- | --- |
| `clients/` | Browser / Windows PC / Android client platform first slice：`protocol`、`client-core`、Web shell、PC desktop shell contract 和 Android runtime contract 已建立并通过 focused validation；`api-gateway` client BFF first-stage HTTP/JSON surface 已落；Web / PC shell 已接账号密码登录、注册、好友列表、好友申请、点击好友发起私聊、群聊列表、建群、点击群聊进入会话、群成员添加 / 退群、成员列表、成员搜索 / 角色过滤 / 分页、移除成员、角色变更 / owner transfer 第一路径、群设置操作区、消息列表、发送后本地状态刷新、PullInbox / AckDelivery；真实双用户 direct + group client smoke 已通过；PC standalone exe、package-local README / launcher support files、unsigned local portable zip bundle、desktop installer / signing readiness plans、显式 `--execute` 门控 signing wrapper、只读 signature verifier、显式 `--execute` 门控 installer build 包装器和 Android debug APK baseline 已产出；artifact install / signing / verification / installer plans 已按 `artifactKind` 区分 executable / installer，不把 installer 当作 portable direct-launch 输入，也不把 mixed manifest 里的 installer 当作 MSI / NSIS executable baseline。客户端只连 `api-gateway` / `push-gateway`，PullInbox 是消息事实源，WebSocket 只做在线唤醒。 |

当前默认主线不是继续泛化清理 9 服务 P2 backlog，也不是做生产级 HA 长测，而是先把
Web / Windows PC 客户端的 IM MVP 交互做实：账号注册登录、好友关系、好友私聊、群聊、
群成员管理第一路径、消息列表、发送、PullInbox / AckDelivery 和局域网可运行体验。本地 / 局域网 Web smoke、
PC WebView login smoke、真实双用户 direct + group client smoke、PC standalone exe package、
unsigned local desktop bundle、desktop installer / signing readiness plans、显式 signing wrapper、只读 signature verifier、显式 installer build 包装器、artifactKind-aware install / signing / verification / installer plans 和 Android debug APK baseline 已有；`loadtest/clientweb` 已扩展到群成员列表、移除、
角色变更和 owner transfer，并已通过 clean committed 真实 smoke；Web / PC shell 已补
BFF-backed 成员搜索 / 角色过滤 / 分页。下一步是
Windows PC MSI / NSIS installer 与真实 signing input / valid signed artifact、群标题 / 头像 read model
和更完整群设置。Android 真机 WebView login smoke 和正式移动端发布链路后置到用户明确切回。

AI 大模型应用底座作为后续主线保留：

```text
group memory
-> EvidencePack
-> RAG / summary
-> multi-agent
-> skill-registry
-> MCP/tool gateway
-> action-executor
-> proposal / approval / audit
-> ai-eval
```

下一步默认看 [current-goal.md](docs/runbook/current-goal.md)。截至当前主线，下一步是围绕
Windows PC MSI / NSIS installer 与真实 signing input / valid signed artifact 继续收口；若继续客户端产品能力，则优先补群成员
列表 / 移除 / 角色 / owner transfer 的真实多用户 smoke、群标题 /
头像 read model 和更完整群设置。Android APK /
真机 smoke 不作为当前默认阻塞。

## 不变量

- PostgreSQL 是交易事实源；Kafka 是事件传播面；业务事务不能直接 publish Kafka，必须走 outbox relay。
- RAG / summary / Agent 只能消费 EvidencePack，不能直接读 message / conversation / private tables。
- Agent 写动作必须走 policy、tool policy、proposal、approval、action-executor 和 audit。
- Python 只做 LLM / embedding / rerank / memory extraction / planner / eval 候选层；Go 负责控制面、权限、状态、审计和持久化。
- push-gateway 不拥有 durable inbox；断线和慢连接恢复以 delivery-service `PullInbox` 为准。
- 新服务和中间件不写死；只有当独立数据模型、伸缩、故障、安全边界或复杂度收益成立时才通过 ADR 新增。
- 新功能、新服务和新中间件先做架构分析再编码，并同步 README、目标架构、
  service brief、SDD / ADR、middleware catalog 和进度文档中的相关事实。
- 不新增隐藏兜底路径；触达相关旧路径时优先删除或改成显式 fail-closed、
  retry / repair / redrive / local-test adapter。
- 压测 / smoke 原始输出放 `H:\NexusIM\loadtest-results`，仓库只保存低敏报告、summary 和索引。

## 目录结构

| 目录 | 作用 |
| --- | --- |
| `api/` | 同步接口契约。`api/proto/` 存放 gRPC Protobuf。 |
| `schemas/` | 异步事件契约。`schemas/kafka/` 存放 Kafka topic 的 Protobuf schema。 |
| `services/` | Go 服务实现。每个服务统一使用 `api / app / domain / infrastructure / types / trigger` 六层目录。 |
| `ai/python/` | Python AI Worker 候选层代码和 `IM` conda 环境配置。 |
| `migrations/` | PostgreSQL migration，按服务归档。 |
| `deploy/` | 本地 Docker、观测、服务编排和运行配置。 |
| `loadtest/` | smoke / loadtest runner。原始结果默认写到 H 盘。 |
| `docs/` | 架构、SDD、ADR、runbook、面试叙事和证据索引。 |
| `tools/` | 生成、门禁、smoke、evidence manifest、capacity / observability 辅助脚本。 |

## 文档入口

| 文档 | 用途 |
| --- | --- |
| [prompt.md](prompt.md) | Codex 文档路由入口；不保存目标框正文。 |
| [agent.md](agent.md) | Codex / sub-agent 每轮读取和维护文档的路由规则。 |
| [docs/runbook/current-brief.md](docs/runbook/current-brief.md) | 低 token 当前阶段入口。 |
| [docs/runbook/current-goal.md](docs/runbook/current-goal.md) | 当前 active slice。 |
| [docs/runbook/development-progress.md](docs/runbook/development-progress.md) | 当前开发进度总览。 |
| [docs/runbook/remaining-goals.md](docs/runbook/remaining-goals.md) | 只记录还没有完成的工作。 |
| [docs/runbook/service-briefs/README.md](docs/runbook/service-briefs/README.md) | 服务 brief 索引。 |
| [docs/architecture/target-architecture.md](docs/architecture/target-architecture.md) | 总体目标架构。 |
| [docs/architecture/target-architecture-complete.md](docs/architecture/target-architecture-complete.md) | 完整目标架构蓝图：业务平台、数据平台、AI / Agent 平台、中间件平台和演进路线。 |
| [docs/architecture/target-architecture-ai.md](docs/architecture/target-architecture-ai.md) | AI / RAG / Agent 目标架构。 |
| [docs/platform/middleware-catalog.md](docs/platform/middleware-catalog.md) | 中间件能力分类、runtime profile 和引入规则。 |
| [docs/runbook/ai-eval/README.md](docs/runbook/ai-eval/README.md) | AI eval case schema、adapter 和运行入口。 |

## 六层 DDD 约束

服务目录统一为：

```text
services/<service-name>/
  cmd/
  internal/
    api/
    app/
    domain/
    infrastructure/
    types/
    trigger/
```

允许依赖方向：

```text
api -> app/types
trigger -> app/types
app -> domain/infrastructure/types
domain -> types
infrastructure -> domain/types
```

禁止方向：

```text
domain -> infrastructure/api/trigger
infrastructure -> api/trigger
types -> app/domain/infrastructure/api/trigger
```

说明：

- `api` 只做 gRPC/HTTP 适配和 request/response 转换。
- `app` 编排 use case、事务和 port。
- `domain` 表达领域规则，不依赖 SQL、Kafka、Redis 或外部 SDK。
- `infrastructure` 实现 PostgreSQL、Kafka、Redis、外部 RPC client 和 provider adapter。
- `trigger` 放 outbox relay、Kafka consumer、定时巡检和补偿任务。

## 常用命令

生成 Protobuf：

```powershell
. .\tools\go-env.ps1
.\tools\gen-proto.ps1
```

启动 / 停止本地依赖：

```powershell
make local-up
make local-down
```

运行聚焦 Go 测试：

```powershell
. .\tools\go-env.ps1
go test ./services/action-executor/... -count=1
```

使用 Python AI Worker 环境：

```powershell
conda activate IM
cd ai\python
python -m pytest
python -m ruff check .
python -m mypy nexusim_ai_common scripts tests
```

运行当前 AI eval adapter：

```powershell
. .\tools\go-env.ps1
.\tools\validate-ai-eval-cases.ps1
.\tools\run-ai-eval-action-external-adapter.ps1
.\tools\run-ai-eval-profile-agent-safety.ps1
```

完整本地门禁只在跨服务、生成代码、migration、service-registry、Docker/compose、安全边界或提交推送前需要：

```powershell
.\tools\check-local.ps1
```

## 当前不是已完成项

以下内容仍属于后续 hardening 或产品化，不要在面试或文档里说成已经生产级完成：

- 生产级 Redis / Kafka / PostgreSQL HA、长时间 fault campaign、split-brain fencing、生产 sizing。
- 统一生产观测平台、Alertmanager 路由、日志汇聚、SLO 和长期 retention。
- provider-grade OIDC / KMS / HSM / email / SMS / WebAuthn / complete risk engine。
- provider-grade ReBAC DSL、外部 audit sink、运维 UI、批量 repair 审批系统。
- 完整 Web / App / 桌面客户端；当前 Web / Windows PC shell 已有账号登录、注册、好友、
  群聊、群成员添加 / 退群、成员列表、成员搜索 / 角色过滤 / 分页、移除成员、角色变更 / owner transfer
  第一路径和消息 first path，`api-gateway` client BFF first-stage surface、Web adapters first path、
  本地 / wired LAN smoke、BFF HTTP metrics / rate-limit adapter、PC standalone exe、
  package-local README / launcher support files、unsigned local portable zip bundle、desktop installer / signing readiness plans、独立 installer profile、显式 signing wrapper、只读 signature verifier、显式 installer build 包装器、artifactKind-aware install / signing / verification / installer plans、Android debug APK baseline 已落，真实双用户 direct + group client smoke 已通过；仍缺
  Windows signed installer / MSI / NSIS、真实 signing input 和 valid signed artifact、Android 真机 smoke、正式移动端发布链路以及
  群标题 / 头像 read model 和更完整群设置。
- 完整 media / notification / admin / audit / workflow / control-plane 等产品化平台能力；
  当前这些服务已有 first-stage 路径，但 provider-grade adapter、UI、长周期运维和生产化
  仍未完成。

当前最准确表述：

```text
NexusIM 已完成 9 个 IM 后端服务的主链路和一批本地 / 双机分布式 smoke，
已形成 AI / RAG / Agent first-stage 应用底座和 product-active 平台服务 first paths，
当前正优先收口 Web / Windows PC 客户端 IM MVP。
```
