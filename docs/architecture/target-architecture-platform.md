## 10. 权限、搜索、RAG 与 Agent

### 10.1 权限模型

对外只暴露 policy-service。内部采用 OpenFGA-compatible ReBAC 模型。

```text
Subject: user / device / agent / service_account
Resource: tenant / org / conversation / message / file / chunk / tool / action
Relation: member / owner / admin / viewer / agent_delegate
Action: read / write / search / download / call_tool / approve / execute
```

授权路径：

```text
business service -> policy-service -> relation tuple store / OpenFGA-compatible engine
```

事实源边界：

| 权限事实 | 事实源 | policy-service 角色 |
| --- | --- | --- |
| user / device / session | identity-service | 消费事件构建 subject projection |
| conversation membership | conversation-service | 消费成员事件构建 relation tuples |
| tool policy | tool-service | 消费工具策略事件构建 action relation |
| approval policy | approval-service | 消费审批策略事件构建 risk relation |

policy-service 是 authorization projection + decision center，不是所有权限关系的原始事实源。

Retrieval 权限规则：

- 索引 ACL 只是加速字段，不是最终授权事实。
- retrieval-gateway 必须支持 `strict_acl_mode`。
- `acl_version` 过期、缺失或投影延迟超阈值时，必须回源 policy-service。
- 用户退群后，`leave_seq` 之后的 chunk 不能被召回。
- 成员 join/leave 的可见窗口必须进入搜索和向量检索过滤，不能只看当前成员状态。
- 消息撤回、删除、保留期清理必须产生 tombstone / delete event，搜索和 RAG 在投影完成前进入 strict fallback 或冻结相关结果。
- Agent 继承用户权限，并叠加 `agent_delegate` 与 tool policy。

强一致授权矩阵：

| 场景 | 授权方式 |
| --- | --- |
| 发消息 | 强校验 conversation-service / policy current version |
| 下载文件 | 强校验 policy-service，必要时回源 media/conversation |
| RAG 检索 | 默认 projection，`acl_version` stale 时 strict check |
| Agent 写动作 | 强校验 policy-service + approval-service |
| 普通搜索 | projection filter + 二次校验 |
| 历史补拉 | 按 `join_seq / leave_seq` 强过滤 |

Relation tuple 重建：

```text
freeze affected policy scope
-> replay identity/conversation/tool/approval events
-> rebuild relation tuples
-> compare tuple count and checksum
-> switch policy projection version
-> audit repair result
```

### 10.2 Search/RAG

OpenSearch 文档必须包含：

```text
tenant_id
org_id
conversation_id
seq
acl_scope
permission_version
deleted_at
classification
```

Milvus metadata 必须包含：

```text
tenant_id
org_id
conversation_id
source_id
chunk_id
source_event_id
source_seq
acl_version
visibility_scope
deleted_at
legal_hold
embedding_model
checksum
```

retrieval-gateway 输出 EvidencePack：

```text
evidence_pack_id
chunk_id
source_id
conversation_id
source_seq
source_message_id
snippet
score.recall
score.rerank
checksum
classification
trace_id
```

EvidencePack 必须能追溯到 source message id / seq / event id；AI 回答不得只给模型生成文本而没有可审计证据。

### 10.3 Agent 与工作流引擎

Agent 写动作固定流程：

```text
agent detects write intent
-> create Action Proposal
-> approval-service checks policy and risk
-> workflow waits approval
-> action-executor calls tool-service / internal API
-> audit-service appends result
```

Agent 边界不变量：

- Agent 不直接读 PostgreSQL、OpenSearch 或 Milvus，只能通过 retrieval-gateway、tool-service 或公开业务 API。
- Agent 不直接写业务库；所有写动作必须进入 proposal / approval / executor / audit。
- Agent 输出必须携带 evidence pack id 或明确标记无证据回答，并被 AI eval / safety gate 覆盖。

工作流引擎只用于长事务：

- 审批等待；
- Agent 写动作；
- 文件解析补偿；
- Retention 删除；
- 跨系统工具调用补偿。

不用于发消息热路径、ACK、read cursor、普通在线投递。

## 11. 多 Region 与灾备

目标态采用：

```text
IM source of truth: Region Active-Passive
edge and push: Region Active-Active
tenant_home_region
conversation_home_region
```

写入路由：

- message write 路由到 `conversation_home_region`。
- member boundary 路由到 `conversation_home_region`。
- read/search 优先就近读投影。
- 强一致补拉回源 home region。
- Agent 写动作按资源 home region 执行。

RPO/RTO：

| 数据 | RPO | RTO | 恢复方式 |
| --- | ---: | ---: | --- |
| message_log / timeline / outbox | <= 5s / 30s | <= 15min | PostgreSQL replication + WAL |
| conversation_members | <= 5s / 30s | <= 15min | PostgreSQL replication |
| audit_logs | <= 5s | <= 30min | replication + WORM manifest |
| user_inbox | 可重建 | <= 1h | timeline replay |
| OpenSearch | 可重建 | 数小时 | Kafka/PG replay |
| Milvus | 可重建 | 数小时 | rag_chunks replay |
| Redis route | 可丢 | 客户端重连恢复 | 不跨 region 复制 |

Failback Runbook：

```text
freeze writes on recovered primary
-> compare WAL position / Kafka offsets / audit manifest
-> reconcile missing events
-> verify message_log and timeline checksum
-> switch conversation_home_region by tenant/conversation batch
-> drain old active region writes
-> resume normal routing
```

约束：

- 不允许双写窗口。
- `conversation_home_region` 切换必须由 control-plane 灰度执行。
- failback 前必须完成 message/timeline/audit 对账。
- split-brain 以 `sequencer_epoch` 和 home region fencing 拒绝写入。

## 12. 数据生命周期与审计

Retention workflow：

```text
retention scanner
-> data.expire.requested
-> message-service tombstone
-> search-service delete/update index
-> rag-ingest-service delete chunk/vector
-> media-service delete object or mark expired
-> audit-service write delete proof
-> data.expire.completed
```

规则：

- `legal_hold = true` 禁止物理删除。
- `deleted_at` 只代表用户侧不可见。
- audit_logs 不参与普通删除。
- OpenSearch/Milvus 删除必须产生 `delete_proof`。

Audit hash chain：

```text
record_hash = hash(canonical_json(record_json))
prev_hash = previous audit record hash in tenant stream
daily_manifest_hash = hash(all record_hash in day order)
```

Audit manifest 写入 WORM object storage。导出文件必须包含 record count、hash range、checksum、operator signature。

## 13. 观测、容量与成本

核心容量目标：

```text
registered users >= 3,000,000
DAU >= 500,000
peak online WebSocket >= 300,000
peak message persistence >= 50,000 msg/s
peak receipt aggregation >= 500,000 cursor/s
vector scale >= 200,000,000 chunks
Kafka replay window >= 14 days
```

容量公式：

| 资源 | 计算方式 |
| --- | --- |
| push-gateway 实例 | `peak_online_connections / safe_connections_per_instance * redundancy_factor` |
| timeline partition | `peak_timeline_events_per_sec / safe_events_per_partition` |
| message DB shard | `peak_message_writes_per_sec / safe_writes_per_shard` |
| redis-route shard | `peak_connection_route_ops / safe_ops_per_shard` |
| redis-counter shard | `peak_counter_ops / safe_ops_per_shard` |
| Kafka broker | `peak_ingress_bytes_per_sec / safe_broker_ingress`，同时满足副本和保留期容量 |
| OpenSearch data node | `index_size_hot_window / safe_disk_per_node` |
| Milvus query node | `peak_retrieval_qps / safe_qps_per_query_node` |

示例代入：

```text
push_gateway_instances
= 300,000 peak connections / 20,000 safe connections per instance * 1.5 redundancy
= 22.5 -> 24 instances

message_db_shards
= 50,000 msg/s / 5,000 safe writes per shard * 1.5 redundancy
= 15 shards

timeline_partitions
= 50,000 timeline events/s / 100 safe events per partition
= 500 -> choose 512 / 1024 capacity tier
```

核心指标：

```text
message_persist_latency_seconds
conversation_seq_alloc_latency_seconds
sequencer_leader_change_total
timeline_gap_marker_count
message_outbox_oldest_age_seconds
kafka_publish_latency_seconds
kafka_consumer_group_lag
fanout_mode_switch_total
push_queue_depth
delivery_delay_seconds
receipt_flush_lag_seconds
redis_hotkey_hits_total
acl_projection_lag_seconds
search_index_lag_seconds
embedding_backlog
retrieval_latency_seconds
approval_queue_depth
audit_hash_chain_break_total
tenant_cost_budget_usage_ratio
error_budget_burn_rate_1h
error_budget_burn_rate_6h
```

发布门禁：

```text
lint
unit / race / integration test
OpenAPI / Proto / AsyncAPI schema diff
SQL migration check
image scan / SBOM
canary 1% -> 5% -> 20% -> 50% -> 100%
SLO burn-rate check
rollback drill for core schema changes
```

成本指标：

```text
cost_per_1k_messages
embedding_cost_per_day
agent_cost_per_tenant
storage_cost_per_tenant
retrieval_cost_per_query
kafka_retention_cost_per_topic
opensearch_index_cost_per_tenant
milvus_vector_cost_per_tenant
```

超预算先限制低优先级智能任务，不影响 IM 写入主链路。

P0 Runbook 目录：

| 故障 | 首要动作 | 恢复条件 |
| --- | --- | --- |
| message 写入失败 | 冻结发布，检查 PG lock/WAL/outbox | append p99 和错误率恢复 |
| Kafka timeline 积压 | 扩 consumer，限制低优先级下游 | lag 回到基线 |
| sequencer leader 异常 | control-plane 切换 owner，检查 epoch fencing | 无乱序，gap marker 完整 |
| Redis route 故障 | 触发客户端重连和路由重建 | 在线连接恢复到阈值 |
| ACL projection 异常 | retrieval 进入 strict_acl_mode | tuple checksum 通过 |
| OpenSearch/Milvus 删除失败 | 冻结相关检索范围，重放删除事件 | delete_proof 完整 |

## 14. AI Safety Eval

RAG/Agent 发布必须跑安全评测：

| 测试 | 强验收 |
| --- | --- |
| 越权检索 | `permission_violation_rate = 0` |
| 跨租户检索 | `cross_tenant_retrieval = 0` |
| Prompt Injection | `prompt_injection_success_rate = 0` |
| 工具滥用 | `unauthorized_tool_call = 0` |
| 无审批写动作 | `high_risk_action_without_approval = 0` |
| 无证据回答 | `evidence_missing_answer_rate < threshold` |
| 错误引用 | `citation_accuracy > threshold` |

高风险评测失败阻断发布。

评测数据集管理：

| 表 | 职责 |
| --- | --- |
| ai_eval_datasets | 数据集名称、版本、owner、适用范围 |
| ai_eval_cases | 输入、期望策略、标签、风险等级 |
| ai_eval_runs | 模型、prompt、retrieval strategy、tool policy、结果 |
| ai_eval_failures | 失败样本、原因、修复状态、回流版本 |

触发规则：

- prompt template 变更必须跑 safety eval。
- retrieval strategy 变更必须跑 permission eval。
- tool schema/policy 变更必须跑 tool abuse eval。
- 线上人工纠错和事故样本必须回流到 eval dataset。

## 15. ADR

| 编号 | 决策 | 原因 |
| --- | --- | --- |
| ADR-001 | Go + 六层 DDD + gRPC/Protobuf 作为业务服务基线 | 统一服务边界、接口契约和工程目录，框架可按 ADR 渐进引入 |
| ADR-002 | push-gateway 独立实现 | 长连接对性能、慢连接、背压、连接生命周期要求独立 |
| ADR-003 | PostgreSQL 是交易事实源 | 支持事务、分区、PITR、强一致写入 |
| ADR-004 | 普通消息由 message-service 单事务写 `seq + message + timeline + outbox` | 避免 timeline-service 与 message-service 跨服务事务冲突 |
| ADR-005 | timeline-service 只做 sequencer control-plane 和热点 seq authority | 保留热点扩展能力，同时不破坏普通会话本地事务 |
| ADR-006 | Sequencer 使用 Kubernetes Lease + PostgreSQL epoch fencing | Lease 负责选主，epoch 防止旧 leader 写入 |
| ADR-007 | 成员变更必须使用 member_change_saga | 显式管理成员事实和 timeline boundary 的失败窗口 |
| ADR-008 | 事实事件通过契约化事件平台传播，当前推荐 Kafka KRaft + Schema Registry | 支持高吞吐、分区保序、长保留、重放和契约治理 |
| ADR-009 | Timeline topic 使用自定义 virtual partitioner | 避免扩分区导致同会话事件分区不稳定 |
| ADR-010 | Redis 拆 route/counter/cache 三类集群 | 避免连接路由被计数和缓存热点拖垮 |
| ADR-011 | policy-service 封装 ReBAC / ABAC 授权后端 | 统一权限模型，避免业务服务依赖底层授权 SDK |
| ADR-012 | search-service 是搜索索引唯一写入口 | 避免索引多写源造成不可控不一致，具体搜索后端可替换 |
| ADR-013 | retrieval-gateway 是唯一检索入口 | 集中权限过滤、召回、rerank 和 EvidencePack 审计 |
| ADR-014 | 向量检索后端通过 retrieval-gateway 封装 | 支持大规模 ANN、metadata filtering 和水平扩展，后端可替换 |
| ADR-015 | 工作流引擎只承接长事务 | 避免 IM 热路径依赖工作流引擎 |
| ADR-016 | Agent 写动作必须审批和审计 | 防止模型越权和不可追踪副作用 |
| ADR-017 | Region 策略为事实源主备、接入多活 | 保证消息一致性，同时提升接入可用性 |
| ADR-018 | 审计采用 hash chain + WORM manifest | 证明审计未删除、未篡改、导出可校验 |
| ADR-019 | Retention 通过工作流执行 | 保证消息、搜索、向量、对象删除状态可证明 |
| ADR-020 | 成本治理按租户预算执行 | 控制 RAG、Agent、OpenSearch、Milvus、Kafka 单位成本 |
| ADR-021 | 引入 control-plane-service | 所有运行策略、切换、预算、降级和 feature flag 必须版本化、审批、灰度、审计 |
| ADR-022 | Sequencer 支持 LOCAL/SEQUENCER 双模式切换 | 普通会话保持本地事务，热点会话可升级到 seq block 模式 |
| ADR-023 | Kafka virtual partition mapping 由控制面版本化管理 | 支持扩容、迁移、回滚和 producer 热加载 |
| ADR-024 | policy-service 是授权投影和决策中心 | identity/conversation/tool/approval 才是各自权限事实源 |
| ADR-025 | 客户端协议定义连接状态机和本地消息状态机 | 保证断线恢复、重试、乱序和 ACK 丢失有一致语义 |
| ADR-026 | DR 必须包含 failback 和对账流程 | 防止恢复主 Region 时 split-brain 和事件丢失 |
| ADR-027 | AI eval 数据集版本化并纳入发布门禁 | 让模型、prompt、检索和工具策略变更可自动验收 |
| ADR-028 | 热点 seq 使用 seq_allocation_journal | 解释已分配未提交 seq，证明无重复 seq 和无未解释 gap |
| ADR-029 | control-plane rollout 必须收集 applied version ACK | 证明目标实例已应用配置版本，避免只发布未生效 |
| ADR-030 | Replay 遵循 PostgreSQL 事实源、Kafka 优先传播回放 | 修复时明确事实边界，避免 Kafka 与 PG 口径冲突 |
| ADR-031 | 授权按场景区分强校验和投影校验 | 在主链路、下载、Agent 写动作等高风险路径避免投影延迟误判 |
| ADR-032 | push-gateway 支持短断线 resume buffer | 提升移动端短断线体验，同时不改变 delivery 补拉事实源 |
| ADR-033 | api-gateway tenant quota source 由控制面 / 配置源版本化发布 | 避免 user-facing gateway 直连业务内部表，DB-backed quota 需通过服务拥有的配置契约 |
| ADR-034 | PostgreSQL production quorum boundary | 本地 `repmgr + pgpool` 只作为 smoke 拓扑；生产 HA 必须另有 quorum / fencing 证据 |

## 16. 演进结论与下一阶段

本文是目标态架构基线，不是终局服务数量或中间件清单。后续优先进入服务级设计、接口契约、数据库 schema、Kafka schema 和压测验证；新增总架构点必须通过 ADR，并说明为什么不能在现有服务或中间件边界内解决。

优先交付：

| 优先级 | 交付物 | 范围 |
| --- | --- | --- |
| P0 | `message-service SDD` | 已形成基线；发送、编辑、撤回、删除、本地事务、outbox、幂等、Runbook |
| P0 | `timeline-service / sequencer SDD` | seq block、epoch fencing、journal、gap marker、模式切换 |
| P0 | `conversation-service / member_change_saga SDD` | 成员事实、边界 seq、Saga、并发冲突、ACL 投影 |
| P0 | `Proto / OpenAPI / AsyncAPI` | `SendMessage`、`AllocateSeqBlock`、`CreateMemberChange`、`AckDelivery`、`PullOfflineMessages`、`RetrieveEvidence` |
| P0 | `PostgreSQL migration` | `conversation_seq`、`message_log`、`timeline`、`outbox`、`message_change_history`、`member_change_saga`、`inbox`、`cursor` |
| P0 | `Kafka schema and topic config` | timeline、delivery、receipt、media、agent、repair 事件和 DLQ/retry |
| P1 | `push-gateway SDD` | WebSocket 协议、resume buffer、错误码、连接状态机 |
| P1 | `delivery-service SDD` | fanout、offline pull、inbox 重建、投递延迟 |
| P1 | `retrieval-gateway SDD` | strict ACL、EvidencePack、索引版本、shadow rebuild |
| P1 | `第一轮压测脚本` | WS 建连、消息写入、热点群、补拉、ACK、Kafka lag、RAG lag、Agent approval |

当前工程缺口和代码边界：

| 缺口 | 对第一阶段代码的影响 | 边界约束 |
| --- | --- | --- |
| `timeline-service / sequencer SDD` 未完成 | 不阻塞普通会话 `SendMessage`；阻塞热点会话生产实现 | 第一阶段只实现 `LOCAL_ROW_LOCK`，`SEQUENCER_BLOCK` 只定义 port 和 mock |
| `conversation-service / member_change_saga SDD` 未完成 | 不阻塞 `GetSendContext` 会话发送上下文 read path；阻塞真实成员变更、群主/管理员规则和 ACL 投影 | message-service 只能依赖 `ConversationQueryPort`，并从 port 读取 `fanout_mode`、`fanout_policy_version`，不能写成员事实、角色规则或硬编码 fanout 策略 |
| Proto / OpenAPI / AsyncAPI 未落文件 | 阻塞正式业务代码 | 先确定 `message_service.proto`、错误码和事件契约，再创建 service skeleton |
| PostgreSQL migration 未落文件 | 阻塞本地事务代码 | 先落 `conversation_seq + message_log + timeline + outbox` 同分片约束 |
| Kafka schema 未落文件 | 阻塞 outbox relay 对外发布 | 先落 `message.persisted.v1` 和 envelope，再实现 producer |
| Outbox DLQ repair 契约文件未落地 | 不阻塞第一阶段 `SendMessage`，但阻塞后续运维闭环 | SDD 已定义 replay/skip 语义；后续必须落 Proto/AsyncAPI 和 `audit.repair.events` schema |
| 跨服务 timeline append / publish ordering 未落地 | 不阻塞第一阶段只有 message event 的 `SendMessage`；阻塞成员边界、gap marker、repair event 生产化 | `conversation-service / member_change_saga SDD` 必须选择统一 timeline append / publish 机制 |

本地开发和双机压测配置属于运行手册，不固化在目标态架构正文中。当前机器 IP、端口、防火墙和代理约定见 `docs/runbook/local-loadtest.md`。

本地/双机分布式 smoke 与生产 HA 的区别：

| 已验证 | 未宣称 |
| --- | --- |
| 双机 Docker、多实例 push route、durable PullInbox fallback、基础 Redis route smoke | Kafka 多 broker HA、PostgreSQL 主从/failover、Redis Sentinel quorum/网络分区、Kubernetes 灰度和自动恢复 |

工程落地基线：

| 目录 | 职责 | 约束 |
| --- | --- | --- |
| `api/proto` | gRPC 服务和事件 Protobuf 契约 | 先定义 `nexusim.message.v1`，字段只增不删，破坏性变更必须升版本 |
| `api/openapi` | HTTP/OpenAPI 契约 | 由 gateway 适配生成，不能手写偏离 Proto 语义的接口 |
| `api/asyncapi` | Kafka topic 事件契约 | topic、partition key、DLQ/retry/replay policy 必须显式声明 |
| `migrations/postgres` | PostgreSQL schema migration | 按服务分目录；所有变更遵循 `expand -> migrate -> contract` |
| `schemas/kafka` | Schema Registry 输入文件 | Protobuf 为主；事件 envelope 与 outbox 表字段保持一致 |
| `services/<service>` | 服务实现 | `api / app / domain / infrastructure / types / trigger` |
| `pkg` | 跨服务公共库 | 只允许放日志、错误码、trace、配置、测试工具；禁止放业务领域模型 |
| `deploy/local` | 本地开发依赖 | PostgreSQL、Kafka、Redis 等基础设施；本地可用单 Redis namespace 简化三集群；本机端口以 runbook 为准 |
| `loadtest` | MacBook/服务器压测脚本 | 每个脚本必须写目标、参数、通过标准和结果输出路径 |

代码依赖规则：

- 当前 Go module 为 `github.com/qsyy0921/IM`。
- 服务之间不能直接 import 对方的 `internal` 或业务实现，只能通过 Protobuf 契约、事件契约或明确的 port interface 交互。
- `domain` 层不依赖 SQL、Kafka、Redis、OpenSearch、Milvus、Temporal、Kratos SDK。
- `app` 层只编排 use case、事务和 port interface，不写协议解析和具体存储细节。
- `infrastructure` 层承接 pgx/SQL repository、Kafka producer/consumer、Redis、OpenTelemetry exporter。
- `api` 层只做对外接口适配和 request/response 转换，不写业务规则。
- `trigger` 层只做后台触发、消费、巡检和补偿任务，不写业务规则。
- `types` 层只放稳定基础类型，不允许变成全局工具箱或业务模型包。
- 第一阶段允许 `policy-service`、`conversation-service`、`timeline-service` 使用 strict mock，但 mock 必须实现同名 port，不能把权限、成员、seq 逻辑硬编码进 message-service。

代码复杂度治理：

- 生产手写文件接近 2500 行必须优先同 package 拆分；测试和 runner 接近 3000 行必须拆 helper。
- 公共包至少需要两个以上真实调用方；禁止为了架构好看提前抽象。
- 优先按已有端口、事实流、projection 和 read model 扩展；新抽象必须降低实际复杂度或隔离故障边界。

第一条代码切片：

```text
message-service SendMessage
-> gRPC/HTTP contract
-> PostgreSQL migration
-> external dependency reads before DB transaction
-> local transaction: conversation_seq + message_log + conversation_timeline_events + message_outbox
-> outbox relay publishes or records publish attempt
-> integration test proves idempotency and transaction atomicity
-> service listens on configured local load-test port
-> load client runs first baseline from local-loadtest runbook
-> record SendMessage baseline before expanding service scope
```

第一阶段实现范围只包含：

```text
SendMessage
PostgreSQL local transaction
message_log
conversation_timeline_events
message_outbox
outbox relay
Kafka publish path
```

第一阶段只实现普通会话本地行锁模式：

```text
LOCAL_ROW_LOCK
```

热点会话只保留契约和 mock：

```text
SEQUENCER_BLOCK
AllocateSeqBlock port
seq_allocation_journal table contract
timeline_gap_markers table contract
```

第一阶段只定义契约、不实现业务闭环：

```text
EditMessage
RevokeMessage
DeleteMessage
hot sequencer
delivery-service
push-gateway
RAG
Agent
```

第一阶段采用边搭建边压测：

```text
small smoke load
-> SendMessage baseline
-> idempotency and conflict test
-> Kafka outage / outbox backlog test
-> short duration stability test
```

不要等所有目标态服务全部部署后再压测；每完成一条真实链路就记录容量基线和瓶颈。

进入编码前的门禁：

- `message-service.proto` 必须先确定 `SendMessage`、`EditMessage`、`RevokeMessage`、`DeleteMessage` 和错误码。
- `EditMessage`、`RevokeMessage`、`DeleteMessage` request 必须携带 `conversation_id`，不能只靠 `message_id` 路由分片。
- `SendMessage` 必须明确 `command_hash` canonical 规则和 `client_msg_id` device scope。
- `SendMessage` 外部依赖读取必须在 DB transaction 外完成，事务内只做本地事实写入。
- PostgreSQL migration 必须覆盖 message-service SDD 中的核心表和唯一约束，尤其是 `conversation_seq` DDL、`message_log.command_hash` 和同分片事务约束。
- Kafka schema 必须覆盖 `message.persisted.v1`、`message.edited.v1`、`message.revoked.v1`、`message.deleted.v1`。
- `conversation.timeline.events` 的跨服务 append / publish 顺序机制必须在成员边界生产化前确定。
- 本地集成测试必须能一键启动依赖并清理数据。
- 第一轮压测只接受真实服务进程，不接受仅返回固定字符串的 toy endpoint。

落地顺序：

```text
service SDD first
-> Proto / OpenAPI / AsyncAPI contract first
-> PostgreSQL migration first
-> Kafka schema first
-> code skeleton
-> integration test baseline
-> load test baseline
-> fault drill
```

核心验收：

- 普通会话 `seq + message + timeline + outbox` 同库同分片同事务。
- Kafka 事件契约可注册、可兼容检查、可重放。
- 客户端断线、重复发送、ACK 丢失、离线补拉均有确定协议。
- RAG/Agent 无越权检索、无审批写动作、无证据回答可被评测阻断。
- 压测输出真实容量基线和瓶颈表。
