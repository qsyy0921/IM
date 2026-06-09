# NexusIM Current Goal

本文是 NexusIM 的完整持续目标档案，包含历史事实、风险、评审规则和报告索引。为节省 token，每轮默认先读短入口 `docs/runbook/current-brief.md`；只有需要细节、历史证据或风险上下文时，才按关键词查询本文。

## 0. 可复制短 Goal Prompt

```text
持续推进 E:\development\IM 的 NexusIM 项目。

每轮开始：
1. 执行 git status --short --branch。
2. 读取 docs/runbook/current-brief.md。
3. 只在需要细节时，用 Select-String 按关键词查询 docs/runbook/current-goal.md，不要每轮全文读取。
4. 按 brief/current-goal 的当前目标、硬边界、下一步优先级继续工作。
5. 不回滚用户已有修改。

工作原则：
1. 优先把系统链路做完整，不把主要时间消耗在重型压测矩阵上。
2. 每个微服务独立使用六层 DDD：api / app / domain / infrastructure / types / trigger。
3. 优先降低微服务耦合、控制代码复杂度：不跨服务读取内部表，不引入网状依赖，不为短期功能增加不必要同步 RPC、公共包或抽象层。
4. 复杂度控制是硬约束：一个切片如果需要同时改多个服务、多条 Kafka 事件、多张核心表或多种用户语义，先拆小；一个 helper / port / 公共包如果没有两个以上真实调用方或不能明显降低复杂度，就留在单服务内。
5. 新能力优先复用已有事实流、outbox、projection、read model 和端口；只有能减少重复、稳定边界或支撑真实链路时才新增服务、表或抽象。
6. 单个切片保持小闭环：契约 / migration / 本地事务 / consumer 或 relay / smoke 分阶段推进，不一次性横跨多个产品能力。实现方案明显变复杂时，先补 SDD/契约并让 sub-agent 复核，再编码。
7. 开发过程中主动使用可用 sub-agent 做设计、实现、测试、文档或风险复核，不等到最后才集中评审。
8. sub-agent 完成任务后及时关闭，避免线程池被历史任务占满；如果线程池已满，优先复用或关闭不再需要的 sub-agent。
9. 不把具体任务写死在 prompt 里；具体下一步以 current-brief.md 为准，current-goal.md 只作长档案和按需索引。
10. 公共契约、migration、事务、幂等、消息顺序、错误码、可运行链路完成时，再按 current-goal.md 的评审规则邀请独立评审。
11. 有意义的切片完成后运行必要检查，更新 current-brief.md；阶段状态变化时同步 current-goal.md 和对应 runbook/loadtest 报告。
12. 按 current-goal.md 的 GitHub 同步策略批量提交和推送，不为低风险小改动频繁推送。
```

## 1. 当前目标

持续推进 `E:\development\IM` 的 NexusIM 项目落地。`message-service`、`conversation-service`、`delivery-service`、`push-gateway` 已完成最小真实闭环和本地 / 双机分布式 smoke；当前不再把重型基础设施矩阵作为主线，优先按 `docs/runbook/current-brief.md` 推进下一条产品能力或可靠性切片。

```text
current-brief.md
-> current slice design / implementation / smoke
-> docs and report update
```

## 2. 长期路线图

NexusIM 按四层逐步推进。每层都必须建立在前一层可运行、可验证、可解释的基础上；不要为了追求亮点而跳过核心 IM 链路，也不要在单个基础设施维度上无限压测。

### 第一层：最小可运行 IM 主链路

目标是证明用户发出的消息可以可靠进入系统、形成会话时间线、投递到接收方，并通过在线通道通知客户端。

核心能力：

- `SendMessage`：普通会话发消息，写入 `message_log`、`conversation_timeline_events`、`message_outbox`。
- 会话上下文：`conversation-service` 提供发送所需的会话模式、成员版本、fanout 策略和 seq shard。
- 本地事务 + outbox：业务事务只写本地 PostgreSQL 和 outbox，不在事务内直接 publish Kafka。
- timeline event：通过 outbox relay 发布到 `conversation.timeline.events`。
- delivery projection：`delivery-service` 消费 timeline，生成 durable `user_inbox`。
- `PullInbox` / `AckDelivery`：客户端可以补拉投递结果，并推进设备级 ACK cursor。
- 在线通知：`push-gateway` 消费 `im.delivery.events`，通过 WebSocket 发送轻量 `delivery.notify`，客户端展示事实仍以 `PullInbox` 为准。

当前状态：这一层已经形成最小闭环，四个真实微服务均有 smoke 证据。后续只在新增核心业务语义时补小规模 smoke，不再围绕第一层做重型压测矩阵。

### 第二层：分布式与可靠性

目标是把第一层从“单进程能跑”提升为“多服务、多实例、跨机器、可降级恢复”的最小分布式系统。

核心能力：

- outbox relay：所有跨服务事件通过 outbox 发布，保持至少一次投递和同会话顺序保护。
- Kafka 事件流：`conversation.timeline.events` 承载会话事实流，`im.delivery.events` 承载投递通知流。
- durable inbox：可靠投递事实在 `delivery-service user_inbox`，不是 WebSocket 内存。
- Redis route：`push-gateway` 多实例用 Redis 维护在线 session route，通过 Pub/Sub 转发在线通知。
- Redis-backed resume buffer：短断线可跨 gateway best-effort replay 最近轻量 notify；失败时回退 `PullInbox`。
- 故障恢复：Redis stop/start、Sentinel discovery、Sentinel failover、慢连接主动关闭等场景要用 smoke 证明降级边界。
- 双机分布式：Windows / Mac 通过有线 `172.31.50.*` 运行跨机器 smoke，优先使用本地网线传输，避免外网流量。
- 观测和报告：每个关键 smoke 都保留原始 result JSON、runbook 报告、成功条件和限制，方便面试复盘。

当前状态：本机多进程、Win/Mac 双机 Docker、Redis route、cross-instance resume、Redis Sentinel discovery / 手动 failover / 停止当前 master 后自动切主 recovery 已跑通；分布式证据已经够用于面试讲“最小分布式 IM 后端”，下一步应转回第三层 IM 产品能力。

注意边界：第二层的目标是“面试可讲、能运行、能解释故障降级”的分布式能力，不是完整生产级基础设施平台。Kafka HA、PostgreSQL 主从/故障切换、服务发现、配置中心、统一 tracing/metrics/alert 和部署编排属于后续生产化项，除非它们阻塞下一层业务能力，否则不要长期停留在重型基础设施矩阵。

### 第三层：完整 IM 产品能力

目标是从“主链路可运行”扩展到更完整的 IM 产品语义。第三层优先补真实用户会遇到的核心功能，而不是继续扩大压测范围。

候选能力：

- 已读回执 / 送达回执：基于 `AckDelivery`、设备 cursor 和后续 receipt event，形成用户可见的阅读状态。
- 消息编辑 / 撤回 / 删除：补齐 `EditMessage`、`RevokeMessage`、`DeleteMessage` 的事实写入、timeline event、delivery projection 和客户端可见性。
- 会话列表 / 未读数：基于 durable inbox、conversation timeline 和 cursor 生成用户侧会话摘要。
- 联系人 / 群管理：完善成员变更、角色、邀请、退出、移除、owner transfer 等业务语义。
- 真实鉴权：push-gateway 已先落第一版 signed gateway token 校验；后续继续从本地 mock auth 过渡到 gateway / identity / device 状态 / session revoke，统一 `AuthContext`。
- policy-service：把当前 mock permission check 收敛为真实权限服务或本地投影。
- 客户端 UI：桌面端或 Web 端只在后端链路稳定后接入，展示事实必须来自 `PullInbox` / 查询 API。
- repair / audit：为 outbox、projection、DLQ、member_change_saga 增加可解释的修复入口和审计记录。

推进原则：优先选择“能串起已有链路、能体现 IM 产品完整性”的功能。当前更适合做已读/送达回执、撤回/编辑/删除、会话列表/未读数，而不是继续做大规模硬件压测。

### 第四层：智能化扩展

目标是在稳定的消息事实、权限边界和可重建投影之上增加 AI 能力。RAG / Agent 不应早于核心 IM 语义上线，否则容易出现权限泄露、数据不完整或演示与主系统脱节。

候选能力：

- 聊天记录搜索：对 message timeline 建索引，按 tenant / conversation / member ACL 做权限过滤。
- RAG 问答：基于用户可见的会话历史做检索增强回答，必须严格遵守成员可见窗口和消息撤回/删除语义。
- 智能总结：按会话、时间窗口、主题生成摘要，摘要本身也要有权限和版本边界。
- 群聊问答 Agent：只在用户有权访问的 conversation 范围内检索和回答。
- 客服机器人：可作为独立 consumer / agent service 接入，不直接写消息事实源，必要时通过正式消息发送 API 进入会话。
- 推荐 / 风控辅助：用于辅助排序、异常检测或运营，不改变消息事实和权限事实。

推进原则：第四层只在第三层核心产品能力基本稳定后启动。任何 RAG / Agent 功能都必须先定义数据来源、ACL 过滤、撤回/删除后的索引修正、审计和失败降级。

## 3. 硬边界

- 项目统一命名为 `NexusIM`，不再使用旧项目名。
- 每个微服务独立使用六层 DDD，不做全局统一 DDD。
- 微服务内固定六层：`api / app / domain / infrastructure / types / trigger`。
- 根目录 `api/` 只放全局接口契约；`services/<service>/internal/api/` 才是服务内部接口适配实现。
- 开发时优先降低微服务耦合、控制代码复杂度；不要为了“分布式”引入网状依赖、跨服务内部表读取、不必要的同步 RPC 或过度抽象。
- 复杂度控制是硬约束：单个切片如果需要同时改多个服务、多条 Kafka 事件、多张核心表或多种用户语义，必须拆成更小阶段；不要把“完整性”当成一次性大改的理由。
- 新增能力前先判断能否复用已有事实事件、outbox relay、read model、service port 或当前服务内 repository；只有复用会制造更高耦合或明显重复时，才新增独立服务 / 表 / 公共抽象。
- 不把“未来可能用到”作为抽象理由；公共包、共享 helper、跨服务接口和统一框架必须有两个以上真实调用方或明确降低复杂度，否则保持在单服务内。
- 服务间同步调用只用于查询当前请求必须依赖的权限 / 上下文；状态传播优先走 Kafka 事实事件和本服务 projection，避免形成调用链雪崩。
- 单次实现优先形成可运行小闭环和可解释测试，不同时改动多个产品能力；复杂功能拆成契约、事务、投影、relay、smoke、hardening 多个切片。
- `message-service` 第一阶段只实现普通会话 `LOCAL_ROW_LOCK` 的 `SendMessage` 主链路。
- 后续优先补齐真实微服务边界，不继续在单个 `message-service` 上做重型硬件矩阵。
- Kafka 事件只能通过 outbox relay 发布，业务事务不能直接 publish Kafka。

## 4. 暂不实现

- 生产级完整 WebSocket / push / delivery 平台。
- 桌面客户端完整 UI。
- RAG / Agent 正式业务能力。
- 热点会话 sequencer 生产逻辑。
- 用户私有删除 / 合规物理删除；`RevokeMessage`、`EditMessage` 和第一阶段 `DeleteMessage(CONVERSATION_VIEW)` 已完成最小真实进程 smoke，并已收口第一阶段可见性：撤回 tombstone / 编辑 changed item / 删除 tombstone 只投给已收到原消息的用户，乱序变更 fail-closed；push-gateway 已完成 `edit / revoke / delete` 三类消息变更在线通知 smoke；并发变更、管理员权限真实 policy、搜索/回执修正仍未覆盖。

## 5. 当前事实

| 项 | 状态 |
| --- | --- |
| 目标态总架构 | `docs/architecture/target-architecture.md` 已存在 |
| ADD | `docs/architecture/add.md` 已存在 |
| TADD | `docs/architecture/tadd.md` 已存在 |
| message-service SDD | `docs/sdd/message-service.md` 已冻结 |
| Proto 契约 | `api/proto/nexusim/message/v1/` 已存在 |
| conversation-service Proto | `api/proto/nexusim/conversation/v1/conversation_service.proto` 已存在，定义 `GetSendContext` |
| Kafka schema | `schemas/kafka/conversation.timeline.events.proto` 已存在 |
| PostgreSQL migration | `migrations/postgres/message/000001_message_core.sql` 已存在 |
| conversation-service migration | `migrations/postgres/conversation/000001_conversation_core.sql` 已存在，包含 `conversations`、`conversation_members`、`member_change_saga` |
| Docker Compose | `deploy/local/docker-compose.yml` 已存在；压测专用 PostgreSQL override 为 `deploy/local/docker-compose.postgres-loadtest.yml` |
| message-service 六层骨架 | `services/message-service/internal/{api,app,domain,infrastructure,types,trigger}` 已存在 |
| conversation-service 六层骨架 | `services/conversation-service/internal/{api,app,domain,infrastructure,types,trigger}` 已存在；第一条 read path 为 `GetSendContext` |
| Go 工具链 | 项目基线为 Go `1.26.4`；已通过阿里云镜像安装到 `C:\Users\10495\.local\go\go1.26.4\bin\go.exe`；`protoc-gen-go v1.36.11` 和 `protoc-gen-go-grpc 1.6.2` 已安装到 `C:\Users\10495\go\bin`；`protoc` 可用，路径为 `C:\Users\10495\anaconda3\Library\bin\protoc.exe`；本地命令先执行 `. .\tools\go-env.ps1` |
| Proto Go 代码 | 已生成 `api/proto/nexusim/message/v1/*.pb.go` 和 `schemas/kafka/conversation.timeline.events.pb.go` |
| Go 依赖 | `go.mod` 使用 Go `1.26.4`，并已引入 `google.golang.org/grpc v1.81.1`、`google.golang.org/protobuf v1.36.11` |
| SendMessage app/domain | 已补 `SendMessageUseCase` 单元测试、permission version 一致性短重试、稳定 JSON canonical command hash、append record 构造 |
| PostgreSQL repository | 已实现普通会话 `SendMessage` 本地事务：幂等检查、同幂等键 advisory transaction lock、`conversation_seq` row lock、`message_log`、`conversation_timeline_events`、`message_outbox` 同事务写入；outbox payload 对齐 `MessagePersistedV1` 业务 payload；集成测试和并发重复请求测试通过 |
| Outbox relay / Kafka publish path | 已实现 `trigger/outbox` relay、PostgreSQL outbox store、Kafka writer producer；relay 支持 `NEXUSIM_OUTBOX_WORKERS` 多 worker 与 `NEXUSIM_OUTBOX_FAILURE_BACKOFF` 失败退避；真实 PostgreSQL + Kafka 集成测试通过；真实 PostgreSQL 多 worker / `FOR UPDATE SKIP LOCKED` 测试已覆盖同 conversation 顺序和跨 conversation 并发；已将成功 publish 后的 outbox `PUBLISHED` 标记从逐条 update 改为同事务批量 update；relay 支持在 `OutboxStore.ProcessReadyBatch` 路径下使用 Kafka `PublishBatch` 批量写入，并可通过 `NEXUSIM_OUTBOX_PUBLISH_BATCH_ENABLED=false` 在同一 HEAD 下切回 single publish 对照；relay debug metrics 已暴露 `outbox_process_ready_latency_ms`、`outbox_fetch_ready_latency_ms`、`outbox_mark_published_latency_ms`、`outbox_commit_latency_ms` |
| message-service gRPC adapter | 已实现 `SendMessage` gRPC handler、proto request/response 转换、稳定错误码 detail 映射和错误 message 脱敏、`NEXUSIM_MESSAGE_SERVICE_MODE=grpc` 运行入口；支持 `NEXUSIM_DEBUG_ADDR=/debug/metrics` 暴露本进程压测指标；已通过 bufconn client 单测 |
| Backpressure / adaptive admission | 已新增 `MESSAGE_ERROR_CODE_SERVICE_OVERLOADED`，repository 支持默认关闭的 PostgreSQL pool backpressure；启用 `NEXUSIM_PG_BACKPRESSURE_ENABLED=true` 后，连接池可用连接数小于等于 `NEXUSIM_PG_BACKPRESSURE_MIN_AVAILABLE_CONNS` 时快速返回 retryable `service overloaded`；gRPC `SERVICE_OVERLOADED` 默认附带 `RetryInfo=500ms`，adaptive admission 可携带动态 retry delay；app 层已新增默认关闭的 `AdmissionPort`，`infrastructure/admission` 第一版 adaptive controller 可按 PG pool、repository pool acquire p95、outbox pending、relay active process ready、outbox fetched per call、Kafka records per call 触发提前拒绝；已新增 `NEXUSIM_ADAPTIVE_MAX_IN_FLIGHT` app 入口 token / concurrency gate，用于限制进入依赖读取和 DB 事务的并发数 |
| SendMessage loadtest | 已实现 `go run ./loadtest/sendmessage` 参数化 gRPC 压测入口；支持 `target`、`vus`、`duration`、`result-dir`、`pg-dsn`、`stats-wait`、`service-metrics-url`、`relay-metrics-url`；`target` 和 service metrics URL 已支持逗号分隔，用于模拟多 `message-service` 实例；summary 记录 full commit、dirty 状态、outbox total/published/pending/DLQ、SendMessage/repository/commit/seq/Kafka/outbox relay latency、service/relay pgx pool、repository 和 relay 内部分段指标、多进程 metrics、retryable error count、service overloaded count、`message_error_counts[]`、`request_rps`、`accepted_rps`、`error_rps`、attempt-level `overload_rate`、`success_p99_ms`、`error_p99_ms`、logical end-to-end latency；Kafka publish 指标已拆出 `kafka_publish_call_latency_ms`、`kafka_publish_records_per_call`、`kafka_publish_record_latency_estimate_ms`，避免 single path 和 batch path 口径混淆；outbox relay 指标已拆出 `outbox_process_ready_active_latency_ms`、`outbox_process_ready_idle_latency_ms`、`outbox_fetched_per_call`，避免 stats-wait idle 样本稀释 active relay 判断；recent 指标已在 summary 顶层暴露 sample count，便于 adaptive 矩阵判断 warm-up；压测器已支持可选 `--retry-overloaded`，会遵守 gRPC `RetryInfo` 并记录 `logical_request_count`、`logical_success_rate`、`retry_attempt_count`、`retried_request_count`、`retry_delay_count`、`retry_delay_avg_ms`、`retry_delay_p95_ms`、`retry_delay_p99_ms`；多 service metrics 的顶层 latency / pg pool 已改为聚合视图，避免只取第一个实例误导；`run-local-multi-instance.ps1` 已支持 `FixedPerInstance` 和 `FixedTotal` 两种 PG 连接预算模式；`run-local-pgpool-gradient.ps1` 已支持显式 `-BackpressureEnabled` 和 `-RetryOverloaded`；已补 `run-local-gradient.ps1`、`run-local-pgpool-gradient.ps1`、`run-local-multi-instance.ps1`、`run-local-outbox-batch-worker-matrix.ps1`、`collect-postgres-diagnostics.ps1`、`watch-postgres-diagnostics.ps1`；真实 gRPC + PostgreSQL + outbox relay + Kafka smoke、baseline、瓶颈诊断、PG pool / multi-instance 矩阵、PostgreSQL 诊断和 backpressure on/off 矩阵已执行；message-service 第一阶段 27 份压测报告已整合为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-message-service-consolidated.md` |
| 本地双机网络 | Win-Mac 压测网络已改为有线直连：Windows `172.31.50.1/24`，Mac `172.31.50.2/24`，链路 `2.5Gbps`；两端 Wi-Fi 继续负责上网；两端各自的 `127.0.0.1:7890` 只给本机访问外网使用，不作为 Win-Mac 服务间代理。后续双机压测、SSH 和 callback/mock receiver 优先使用 `172.31.50.*`，历史 `192.168.0.*` 地址只作为 Wi-Fi fallback 和旧报告事实 |
| 本地分布式 runbook | `docs/runbook/distributed-local.md` 已新增，定义当前四个真实微服务 / 网关的本地多进程分布式拓扑、`tools/local-distributed-smoke.ps1` 入口、可讲证据和限制；已补 Win/Mac 双机 Docker 节点模拟计划：Windows `172.31.50.1/24`、Mac `172.31.50.2/24`，后续优先用有线直连承载服务间通信；Windows -> Mac 免密 SSH 已恢复，Mac Docker CLI 可用，Docker Desktop 29.5.3，当前 Docker 资源池为 8 CPU / 8192MiB；`tools/check-mac-docker-desktop.ps1` 可复查 CPU / memory / proxy；`tools/sync-mac-distributed-smoke.ps1` 可通过有线 SSH/scp 传 Git bundle 和 darwin/arm64 二进制到 Mac 专用 checkout；clean commit `8c322fc` 已跑通 Win-Mac Docker route smoke；clean commit `b8d8f92` 已跑通 Win-Mac Docker cross-instance resume smoke：首连 Mac Docker WebSocket gateway，ACK 前断开后重连 Windows gateway，命中 Redis-backed resume buffer replay 同一条 `delivery.notify`；原始结果在 `H:\NexusIM\loadtest-results\push-gateway-win-mac-cross-instance-resume-20260609\pushgateway-summary.json` |
| 压测报告归档 | 每个微服务一个目录：`docs/runbook/loadtest/<service>/`；目录内保存小报告、矩阵报告和 consolidated 总报告。`message-service` 当前入口为 `docs/runbook/loadtest/message-service/README.md` |
| conversation-service smoke | 已跑真实进程小规模 smoke：`message-service -> conversation-service -> PostgreSQL`，725/725 成功，p99 13.26ms；报告见 `docs/runbook/loadtest/conversation-service/` |
| conversation-service local runbook | `docs/runbook/conversation-service-local.md` 已存在，记录 migration、seed、双服务启动、smoke 和清理步骤 |
| conversation-service member_change_saga SDD | `docs/sdd/conversation-service-member-change-saga.md` 已冻结 v1.0，选定 timeline append/publish 方案 C |
| conversation-service member change contract | `conversation_service.proto` 已新增 `CreateMemberChange` / `GetMemberChange` 契约；`conversation.timeline.events.proto` 已新增 member boundary oneof payload；`000002_member_change_saga_v2.sql` 已补 saga retry/DLQ/metadata/event id 字段；outbox relay builder 已支持 `conversation.member.*` fail-closed；`000003_member_change_saga_event_unique.sql` 补 saga event id 唯一约束 |
| conversation-service CreateMemberChange | 最小写路径已实现：gRPC adapter -> app usecase -> PostgreSQL repository；同事务写 `member_change_saga`、`conversation_members`、`conversations` version、`conversation_seq`、`conversation_timeline_events`、`message_outbox`；真实 PostgreSQL 集成测试已覆盖首写、幂等 replay、同 key 冲突和 event/timeline/outbox 一致性；权限矩阵已收紧为第一版保守规则，`MERGE/COMPENSATE` 冲突策略暂不接受 |
| conversation-service GetMemberChange / saga progress | `GetMemberChange` 查询接口已实现；`NEXUSIM_CONVERSATION_SERVICE_MODE=member-change-worker` 可启动 saga 推进 worker，观察 `message_outbox.status=PUBLISHED` 后把 `member_change_saga` 从 `OUTBOX_ENQUEUED` 推进到 `DONE`；真实 PostgreSQL 集成测试已覆盖 outbox 未发布不推进、发布后推进、limit 控制 |
| conversation-service member change full smoke | 已跑真实进程小规模 smoke：`CreateMemberChange -> outbox relay -> Kafka member event -> member-change-worker -> GetMemberChange(DONE)`，350/350 成功，p99 40.90ms，saga/outbox/timeline 各 350，outbox `PUBLISHED=350`，saga `DONE=350`，报告见 `docs/runbook/loadtest/conversation-service/loadtest-report-20260609-member-change-full-smoke.md` |
| conversation-service review fixes | 独立评审指出的 `GetMemberChange` 读取授权和 `last_error` 脱敏 P1 已修复：只允许操作者、目标用户、当前 ACTIVE 的 OWNER/ADMIN 查询；对外只返回稳定 `member change processing failed`，不透出 raw DB/Kafka/repair 文本；worker 推进 SQL 已补 conversation/producer/member event 防御性过滤；复核结论无 P0/P1 |
| conversation-service member roster / owner transfer | 已新增最小 `ListConversationMembers` 读接口并跑通 clean commit `99aacc6` 的 `JOIN` roster smoke、clean commit `14ffedc` 的 `LEAVE` roster smoke、clean commit `be2e039` 的 `REMOVE` roster smoke 和 clean commit `7150944` 的 `ROLE_CHANGED` roster smoke：JOIN 后返回新 ACTIVE 成员，LEAVE/REMOVE 后目标成员状态变为 `LEFT` 且普通 roster 不再返回该用户，ROLE_CHANGED 后目标成员仍为 ACTIVE 且 role 更新为 `ADMIN`；只列当前 ACTIVE 成员和当前角色，调用者必须是该会话 ACTIVE 成员；分页 token opaque，当前按 `user_id ASC` keyset；该接口用于降低耦合，避免其它服务跨表读取 `conversation_members`，不承担成员历史 / 审计视图。owner transfer 已在 SDD 中冻结为专用流程，并完成 `TransferConversationOwner` proto、`conversation.member.owner_transferred.v1` Kafka schema、migration 约束、message-service relay builder、delivery-service projection、conversation-service app / repository / gRPC 写路径和 clean commit `490db1a` 的真实进程 smoke：旧 owner 降级为 ACTIVE ADMIN，新 owner 成为唯一 ACTIVE OWNER，`saga_done_count=1`，outbox `PUBLISHED=1/PENDING=0`；报告见 `docs/runbook/loadtest/conversation-service/loadtest-report-20260610-owner-transfer-smoke.md`。 |
| delivery-service SDD | `docs/sdd/delivery-service.md` 已新增 v0.1 Draft，并已按评审 P1 补齐 delivery membership projection、ACK max visible seq 约束、Kafka checkpoint 维度 |
| delivery-service 工程基线 | 已新增 `delivery_service.proto`、delivery migration、六层目录、`PullInbox / AckDelivery` 最小 gRPC + app + PostgreSQL 骨架、`ProjectTimelineEventUseCase` / PostgreSQL projection 方法，以及 timeline consumer worker；真实 PostgreSQL 集成测试已覆盖 projection、ACK 越界、ACK 并发幂等 |
| delivery-service full smoke | 已跑真实进程小规模 smoke：`CreateMemberChange(JOIN) -> Kafka timeline -> delivery projection -> SendMessage -> Kafka timeline -> user_inbox -> PullInbox -> AckDelivery`，SendMessage `64/64` 成功，`delivery-user-1` 拉到 64 条 inbox，ACK 到 seq `66`；`loadtest/delivery` summary 已支持 `--consumer-group`，checkpoint 统计可按本次 consumer group 过滤；报告见 `docs/runbook/loadtest/delivery-service/loadtest-report-20260609-delivery-full-smoke.md` |
| delivery_outbox / push-gateway | 已新增 `schemas/kafka/delivery/v1/im.delivery.events.proto`、delivery-service outbox store、trigger relay、Kafka writer producer 和 `NEXUSIM_DELIVERY_SERVICE_MODE=outbox-relay`；真实 Kafka smoke 已验证 `delivery_outbox PENDING -> PUBLISHED`，并从 `im.delivery.events` 解码出 `DeliveryEvent_AckRecorded`；push-gateway 已完成最小在线通知 smoke，后续生产化依赖 Redis route / resume buffer / slow session active close；push-gateway 仍必须依赖 delivery read model / delivery event，不直接读取 message-service 内部表，也不修改 ACK |
| delivery-service negative visibility | 已新增 `loadtest/deliveryvisibility`，并在 clean commit `a87fc3f` 跑通 `LEAVE / REMOVE` 负向可见性 smoke：目标用户边界前各收到 1 条 inbox，边界后 `membership_status=LEFT`、`leave_seq=boundary_seq=4`，active sender 收到 post-boundary message，目标用户 `target_post_inbox_count=0` 且 `PullInbox(after_seq=boundary_seq)=0`；报告见 `docs/runbook/loadtest/delivery-service/loadtest-report-20260609-delivery-visibility-negative-smoke.md` |
| push-gateway SDD / skeleton / smoke | `docs/sdd/push-gateway.md` 已新增 v0.1 Draft；边界为 WebSocket 在线连接、`im.delivery.events` 轻量通知、客户端回源 `PullInbox` 和 ACK frame 转发到 `delivery-service AckDelivery`；`delivery.notify` 透传 `source_event_type`，用于区分新增 / 编辑 / 撤回 / 删除唤醒，展示事实仍以 `PullInbox` 为准；评审 P1 已补 `server.pong`、`delivery.ack.ok` 和 `PERMISSION_DENIED retryable=false`；`services/push-gateway/internal/{api,app,domain,infrastructure,types,trigger}` 六层骨架、WebSocket adapter、in-memory registry、delivery-service gRPC client、Kafka delivery consumer 和 `NEXUSIM_PUSH_GATEWAY_MODE=all` 已落地；clean commit `984080d` 真实进程 full smoke 已通过；clean commit `99efdc3` 同 user 双 device notify smoke 已通过，两个 device 都收到同一条 `delivery.notify` 并分别 ACK 到 device cursor；queue-full slow session 第一版已支持 registry eviction signal -> WebSocket broad `server.resume_hint` -> active close，且已修复普通断连时 close outbound 与 registry send 的竞态；单实例 in-memory resume buffer 已支持按服务端签发的 `resume_token + last_received` 重放最近 `delivery.notify`，未知或过期客户端 token 会返回 `buffer_miss` 并替换为新服务端 token；clean commit `b362dd7` 已跑通 slow-client 真实进程负向 smoke，证明 queue full / active close 后可通过 durable `PullInbox` 补拉并 ACK；clean commit `80033de` 已跑通单实例 resume replay smoke，证明同一 `resume_token` 重连可重放同一条 buffered `delivery.notify`；Redis route 最小 adapter 已落地，支持 `NEXUSIM_PUSH_ROUTE_BACKEND=redis`、session route TTL、route TTL 周期续期、stale route lookup 清理、后台 stale route cleanup loop、按 gateway Pub/Sub 转发和本机 fanout；Redis-backed cross-instance resume buffer 第一版已落地，支持 Redis token meta / frame list、跨 gateway replay、未知 token buffer_miss 换新 token、跨 device token 拒绝、buffer gap fallback 和 Redis lookup 故障下本地在线 fallback；`/debug/metrics` 已新增 Redis route / Redis resume 调试指标，consumer-only gateway 可用 `NEXUSIM_PUSH_DEBUG_ADDR` 暴露只读 debug metrics；clean commit `903f205` 已跑通真实跨进程 Redis route smoke：WebSocket gateway 和 delivery consumer gateway 分离后仍收到 `delivery.notify`；Redis unavailable / stale route cleanup 已有单元测试覆盖，策略为 connect 写 route 失败 fail-closed、在线 notify lookup/publish 失败 fail-open；clean commit `074902b` 已跑通真实 Redis stop/start fault smoke，证明 Redis route 中断时 online notify 可丢但 `PullInbox + AckDelivery` 可恢复；clean commit `b8d33da` 已跑通本机 cross-instance resume 真实进程 smoke；clean commit `b8d8f92` 已跑通 Win-Mac Docker cross-instance resume smoke：首连 Mac Docker gateway，重连 Windows gateway，consumer gateway 记录 `redis_resume_append_count=1`，重连 gateway 记录 `redis_resume_replay_count=1 / redis_resume_miss_count=0`，并 replay 同一条 `delivery.notify`；push-gateway Redis client 已支持 `single` / `sentinel` 两种模式；clean commit `7bc35a5` 已跑通三 Redis / 三 Sentinel discovery 正常路径下的 route / resume smoke，Sentinel 返回 master `172.31.50.1:6380`；clean commit `819c14a` 已跑通手动 `SENTINEL failover mymaster` 后的 route / resume recovery smoke，master 从 `172.31.50.1:6380` 切到 `172.31.50.1:6381`；clean commit `8ddc2fb` 已跑通停止 Sentinel 当前 master 容器后的自动切主 recovery smoke，master 从 `172.31.50.1:6381` 切回 `172.31.50.1:6380`；当前仍未完成 Redis Cluster、Sentinel quorum / 网络分区和跨实例慢连接组合 smoke；报告见 `docs/runbook/loadtest/push-gateway/` |
| push-gateway message-change notify smoke | `loadtest/pushgateway` 已新增 `message-change-notify` 场景和 `--message-change-action edit|revoke|delete`；标准脚本 `loadtest/pushgateway/run-local-smoke.ps1` 默认按 `RunName` 派生独立 `tenant_id / conversation_id`，避免多组 smoke 复用固定测试数据。2026-06-10 已在 clean commit `81fe92c` 顺序跑通 `edit / revoke / delete` 三类真实进程 smoke：WebSocket `delivery.notify.source_event_type` 分别为 `message.edited.v1 / message.revoked.v1 / message.deleted.v1`，并与 durable `PullInbox` 的 `event_type + message_id + conversation_seq` 一致；三组 `delivery_outbox PUBLISHED=3 / PENDING=0 / DLQ=0`，ACK 均推进到变更 seq `3`。报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260610-push-gateway-message-change-notify-smoke.md`。 |
| receipt-service SDD / implementation | `docs/sdd/receipt-service.md` 已新增 v0.1 Draft；定位为第三层 IM 产品能力，基于 `im.delivery.events` 重建送达 / 已读回执 read model，`delivery.ack.recorded.v1` 只表示 received，不等于 read；read 必须由客户端显式 `MarkRead` 推进；已补 `ReceiptAccessPort` 权限来源、`delivery.inbox_item.created.v1.sender_id`、receipt API proto、`im.receipt.events` schema、receipt migration、六层骨架、PostgreSQL repository、delivery event consumer、receipt outbox relay、`grpc` / `delivery-consumer` / `outbox-relay` 运行模式、`MarkRead` 事务、最小 `ListReceiptStates` 和最小 `ListConversations`；`ListReceiptStates` 是低耦合薄批量查询：app 层一次鉴权后按顺序复用既有 `GetReceiptState`，不新增批量 SQL、跨服务内部表读取或公共抽象，最多 50 项且采用 whole-request failure；repository 集成测试覆盖 inbox projection、ack projection、ack-before-inbox、read cursor 范围校验、outbox 幂等、会话列表未读 `1 -> 0` 和 projection/read 并发更新；clean commit `3c28105` 已跑通真实进程 smoke：`im.delivery.events -> receipt projection -> GetReceiptState(received) -> MarkRead -> GetReceiptState(read)`；clean commit `abfe0ec` 已跑通 receipt outbox relay smoke：`receipt_outbox -> im.receipt.events`；clean commit `503ff25` 已跑通 conversation list smoke：`receipt_inbox_projection -> user_conversation_summaries -> ListConversations`，投递后 `unread_count=1`，`MarkRead` 后 `unread_count=0`；报告见 `docs/runbook/loadtest/receipt-service/` |
| receipt-service conversation list / unread | `docs/sdd/receipt-service-conversation-list.md` v0.1 Draft 和最小实现已落地；会话列表 / 未读数作为 receipt-service 扩展 projection 实现，不新增 `conversation-list-service`，以控制服务数量、跨服务耦合和代码复杂度；最小链路为 `im.delivery.events -> receipt_inbox_projection -> user_conversation_summaries -> ListConversations`，`MarkRead` 同事务更新 summary；summary 更新与 `MarkRead` 使用同一 tenant/user/conversation advisory lock，避免并发 projection 覆盖 read cursor；`delivery.inbox_item.created.v1` 已新增 `source_event_type`，receipt 只把 `message.persisted.v1` 计入 unread，`message.edited.v1` / `message.revoked.v1` / `message.deleted.v1` 只推进 last visible / 列表排序 / `last_source_event_type`，不让已读用户因为消息变更重新出现未读；`ListConversations` 已补显式 `ConversationListSort`，默认 `PINNED_UPDATED_AT_DESC` 使用 `pinned desc + updated_at desc + conversation_id asc` keyset 分页，显式 `UPDATED_AT_DESC` 保留纯更新时间排序，cursor 绑定版本、sort、`include_archived` 和 pinned 维度；`ArchiveConversation` v0.1 已作为当前用户列表过滤偏好落地，默认列表隐藏 archived，会话管理视图可用 `include_archived=true` 查回，取消归档后恢复可见，新消息投影不会自动取消归档；`PinConversation` v0.1 已作为当前用户排序偏好落地，pin/unpin 不进入 Kafka、不影响 unread、delivery、push 或消息事实；真实 PostgreSQL 集成测试覆盖多会话翻页、tie-break、非法 cursor、archive 默认隐藏 / include_archived 可见 / 新投影保留 archive、pinned-first 排序和跨 pinned/unpinned 分页；clean commit `f8ab746` 已跑通 archive 真实进程 smoke，clean commit `bad4dda` 已跑通 pin 真实进程 smoke；报告见 `docs/runbook/loadtest/receipt-service/loadtest-report-20260610-receipt-archive-smoke.md` 和 `docs/runbook/loadtest/receipt-service/loadtest-report-20260610-receipt-pin-smoke.md`；明确 `delivery.ack.recorded.v1` 只是 received，不清 unread；第一阶段不读 delivery/message/conversation 内部表、不返回消息正文、不做 mute/draft、不做 Redis unread counter |
| message-service RevokeMessage | 第三层消息变更能力已推进到最小真实链路；已补 `RevokeMessage` gRPC handler、app use case、PostgreSQL 同事务更新 `message_log/message_change_history/message_command_idempotency/conversation_timeline_events/message_outbox`，message outbox relay 已支持 `message.revoked.v1 -> MessageRevokedV1`，delivery timeline consumer / repository 已把 `message.revoked.v1` 投影为 `user_inbox` tombstone item；2026-06-10 已在 clean commit `8d008de` 跑通真实进程 smoke：`SendMessage -> PullInbox(message.persisted.v1) -> RevokeMessage -> message outbox relay -> delivery projection -> PullInbox(message.revoked.v1) -> AckDelivery`，结果 `message_log.status=REVOKED`、`message_change_history=1`、`message_outbox PUBLISHED=3`、`user_inbox` 同时含 persisted/revoked、`delivery_outbox PUBLISHED=3`、cursor 推进到 `seq=3`；随后补 hardening：repository 锁住原消息后校验 actor 必须是 sender，撤回 tombstone 不再按撤回时成员窗口广播，而是查询 delivery 自己的 `user_inbox`，只投给已收到原 `message.persisted.v1` 的用户；delivery 若先收到 revoke 但本地没有原消息投影，会 fail-closed 且不提交 checkpoint；报告见 `docs/runbook/loadtest/message-service/loadtest-report-20260610-revoke-message-smoke.md` |
| message-service EditMessage | 第三层消息编辑能力已按低耦合原则复用现有 message-service 本地事务、`message_change_history`、message outbox relay、`conversation.timeline.events` 的 `MessageEditedV1` 和 delivery projection。第一阶段限定原发送者编辑自己的 TEXT 消息，采用 last-write-wins + history before/after payload，不新增服务、不读取其它服务内部表；delivery 对 `message.edited.v1` 复用 revoke hardening，只投给已有原始 `message.persisted.v1` inbox 的用户，原消息缺失时 fail-closed 不提交 checkpoint。2026-06-10 已在 clean commit `cb2f07d` 跑通真实进程 smoke：`SendMessage -> PullInbox(message.persisted.v1) -> EditMessage -> message outbox relay -> delivery projection -> PullInbox(message.edited.v1) -> AckDelivery`，结果 `message_log.status=EDITED`、payload 从 original 更新为 updated、`message_change_history=1` 且 `EDIT / NORMAL -> EDITED`、`message_outbox PUBLISHED=3`、`user_inbox` 同时含 persisted/edited、`delivery_outbox PUBLISHED=3`、cursor 推进到 `seq=3`；报告见 `docs/runbook/loadtest/message-service/loadtest-report-20260610-edit-message-smoke.md` |
| message-service DeleteMessage | 第三层消息删除能力已按低耦合原则复用现有 message-service 本地事务、`message_change_history`、message outbox relay、`conversation.timeline.events` 的 `MessageDeletedV1` 和 delivery projection。第一阶段语义是全局 `CONVERSATION_VIEW` tombstone，不是用户私有 delete-for-me，也不是合规物理擦除；delivery 对 `message.deleted.v1` 复用 revoke/edit hardening，只投给已有原始 `message.persisted.v1` inbox 的用户，原消息缺失时 fail-closed 不提交 checkpoint。2026-06-10 已在 clean commit `b001eb1` 跑通真实进程 smoke：`SendMessage -> PullInbox(message.persisted.v1) -> DeleteMessage -> message outbox relay -> delivery projection -> PullInbox(message.deleted.v1) -> AckDelivery`，结果 `message_log.status=DELETED`、`deleted_at` 非空、原始 payload 保留、`message_change_history=1` 且 `DELETE / NORMAL -> DELETED`、`message_outbox PUBLISHED=3`、`user_inbox` 同时含 persisted/deleted、`delivery_outbox PUBLISHED=3`、cursor 推进到 `seq=3`；报告见 `docs/runbook/loadtest/message-service/loadtest-report-20260610-delete-message-smoke.md` |

## 6. 下一步优先级

1. 当前 Codex 进程如果仍找不到 `go`，先执行 `. .\tools\go-env.ps1`。
2. 不再继续做 message-service 重型硬件矩阵；只有公共契约、关键并发语义或新服务链路变化时，才跑 smoke / 小规模验证。
3. `push-gateway` SDD 已完成阶段评审并修复 frame 契约 P1；六层骨架和第一版 WebSocket / delivery consumer 已落地。
4. `delivery_outbox -> im.delivery.events -> push-gateway all mode -> online WebSocket client delivery.notify -> PullInbox -> delivery.ack -> AckDelivery -> delivery.ack.ok` 已通过 clean commit smoke；同 user 双 device notify smoke 也已通过。
5. 当前系统可以表述为“本地多进程 + Win/Mac 双机 Docker 最小分布式 IM 链路”：`conversation-service / message-service / delivery-service / push-gateway` 独立协作，通过 PostgreSQL outbox、Kafka、durable inbox、Redis route、Redis-backed best-effort resume buffer 和 WebSocket notify 串联；已在 clean commit `8c322fc` 跑通 Windows -> Mac Docker WebSocket gateway 的有线直连 smoke，在 clean commit `b8d33da` 跑通带 consumer metrics 的本机多进程 cross-instance resume smoke，在 clean commit `b8d8f92` 跑通 Win-Mac Docker cross-instance resume smoke，在 clean commit `7bc35a5` 跑通三 Redis / 三 Sentinel discovery 正常路径的 route / resume smoke，在 clean commit `819c14a` 跑通手动 Sentinel master failover 后的 route / resume recovery smoke，并在 clean commit `8ddc2fb` 跑通停止 Sentinel 当前 master 容器后的自动切主 recovery smoke。下一步不再继续做重型基础设施矩阵，优先转回第三层 IM 产品能力；receipt-service 已完成 `im.delivery.events -> receipt projection -> MarkRead -> receipt_outbox -> im.receipt.events -> ListConversations` 最小闭环，并已补 `updated_at desc` keyset 分页契约和真实 PostgreSQL 测试；`RevokeMessage` 已在 clean commit `8d008de` 完成最小真实进程 smoke，`EditMessage` 已在 clean commit `cb2f07d` 完成最小真实进程 smoke，第一阶段 `DeleteMessage(CONVERSATION_VIEW)` 已在 clean commit `b001eb1` 完成最小真实进程 smoke；push-gateway 已在 clean commit `81fe92c` 完成 `edit / revoke / delete` 三类 message-change notify smoke。再往后优先考虑真实鉴权、会话列表权限强化 / 置顶 / 静音 / 归档、消息变更对 receipt/search 的修正，或联系人 / 群管理。

## 7. 评审要求

评审采用里程碑触发，不对每个小改动都邀请独立评审线程。

开发阶段应主动使用 sub-agent 分担专项检查，但不要把所有小改动都升级为正式独立评审。推荐分工：

| sub-agent | 使用时机 | 重点 |
| --- | --- | --- |
| Gauss | 设计和契约阶段 | SDD / ADD / TADD、proto、Kafka schema、migration、服务边界 |
| Noether | 编码阶段 | 六层 DDD 依赖、事务、幂等、并发、错误码、数据一致性 |
| Dewey | 验证和报告阶段 | 测试覆盖、smoke/loadtest 方法、runbook、报告口径、面试可讲结论 |

sub-agent 输出默认作为工作中参考；只有出现公共契约、migration、并发/事务/幂等、可运行链路完成等里程碑时，才整理后发送给独立评审线程。

sub-agent 必须按任务生命周期管理：专项任务完成后及时关闭；不要长期保留已经无用的 sub-agent；线程池已满时，先复用或关闭旧 sub-agent，再创建新的。

必须邀请评审的情况：

- 一个可运行链路完成，例如 `SendMessage -> outbox -> Kafka`、gRPC adapter + smoke/loadtest、压测闭环。
- 跨越两层以上 DDD 边界，或修改 ADD / TADD / SDD / migration / proto / Kafka schema。
- 涉及并发、事务、幂等、消息顺序、错误码契约、数据一致性等高风险逻辑。
- 准备把某个阶段标记为完成，或准备做较大的 GitHub 同步。
- 用户明确要求评审。

不需要立即评审的情况：

- 小范围重命名、注释、文档索引、runbook 状态更新。
- 单层内部测试补强，且没有改变对外契约或事务语义。
- 修复明显拼写、格式、路径说明等低风险问题。

低风险变更先本地验证并累计到下一次阶段评审；如果连续开发中出现不确定的架构取舍，再主动请求评审。

当前主工作线程 ID：

```text
019ea0b0-69fd-7be1-9ec4-4b3c6247e36d
```

独立评审线程 ID：

```text
019ea124-dab1-71f2-964b-f5cb8d219aa2
```

评审结果回传规则：

- 优先回传到 delegation 的 `source_thread_id`。
- 如果没有 delegation 元数据，则回传到当前主工作线程 ID。
- 如果主线程迁移或新建，必须同步更新本文。

发送给评审线程的信息必须包含：

- 本轮目标。
- 修改文件列表。
- 核心设计选择。
- 六层 DDD 是否受影响。
- ADD / TADD / SDD 是否同步。
- 已执行检查和结果。
- 已知风险。
- 希望评审重点。

收到评审意见后：

- 阻塞问题必须修复后再继续编码。
- 非阻塞问题记录到本文或对应 runbook。
- 合理建议需要同步到文档或代码。

## 8. 压测要求

压测只服务于验证关键链路和形成可解释证据，不再追求耗尽本机硬件资源的极限矩阵。可以在一台电脑上用多线程模拟多个客户端，也可以按 `docs/runbook/local-loadtest.md` 做双机压测。

硬约束：

- 压测命令必须支持 `target`、`vus`、`duration`、`result-dir` 参数或等价环境变量。
- 第一轮压测只接受真实服务进程，不接受固定字符串 toy endpoint。

每轮压测至少记录：

```text
commit
target
vus
duration
request_count
success_rate
p95
p99
outbox_pending_count
outbox_oldest_pending_age
kafka_publish_call_latency
kafka_publish_records_per_call
kafka_publish_record_latency_estimate
error_topn
```

小规模 smoke 可以输出到仓库内 `loadtest/results/`，大文件和临时日志默认不提交。

中大型压测、趋势图、跨机器测试和长时间运行结果不要再写入 C 盘或 Docker 数据盘；本机默认暂存到机械盘：

```text
H:\NexusIM\loadtest-results
```

每个阶段必须新增一份独立压测报告，不覆盖旧报告。报告文档仍保存在当前仓库，也就是 `E:\development\IM\docs\runbook\loadtest\<service>\`；不要把报告 Markdown 写到 H 盘。推荐命名：

```text
docs/runbook/loadtest/<service>/loadtest-report-YYYYMMDD-<stage>.md
```

每个微服务阶段结束时维护一份 consolidated 总报告，汇总本服务的小报告、压测方法、瓶颈排查过程和面试可讲结论。`message-service` 当前总入口为：

```text
docs/runbook/loadtest/message-service/loadtest-report-20260609-message-service-consolidated.md
```

报告至少说明：

```text
压测目标
压测拓扑
服务端和客户端分别跑在哪里
CPU / 内存 / Docker / 连接池 / worker 配置
具体命令或脚本入口
通过标准
核心结果
中间结果文件路径
瓶颈排查方法
当前结论和下一步
```

`loadtest/results/` 只保存小 smoke 或历史索引；中大型原始数据、中间结果和趋势图保存到 `H:\NexusIM\loadtest-results`。这些数据文件默认不提交，但 E 盘仓库内的报告必须引用关键结果路径，保证以后能追溯。

## 9. GitHub 同步要求

GitHub 同步采用批量策略，不对每个小改动都推送。

优先本地完成并验证一组相关变更；满足以下任一条件再 commit / push：

- 一个可描述的功能切片完成，例如 repository、outbox relay、gRPC adapter、loadtest runner。
- 修改了公共契约、migration、架构文档或会影响他人协作的文件。
- 用户要求同步 GitHub / MacBook / 服务器。
- 工作区累计变更较多，继续开发前需要建立明确恢复点。
- 评审完成且需要把已验收状态固定下来。

小范围文档状态更新、低风险测试补强、探索性修改可以暂不 push，必要时只保留本地工作区或本地 commit。

需要持久化时，必须：

- 执行并记录 `git status --short`。
- 执行可用检查，例如 `git diff --check`、生成配置检查、单元测试或集成测试。
- 提交后记录 `git log -1 --oneline`。
- 达到同步条件时再执行 `git push origin main`，并记录推送结果。
- 推送后再次确认 `git status --short` 干净。
- 如果本轮涉及 MacBook、服务器或压测环境，说明是否需要同步对应环境；不默认静默同步。

## 10. 每轮结束检查

每轮结束前确认：

- 文档是否需要同步更新。
- 是否达到里程碑评审条件；未达到则不邀请评审线程。
- 是否执行了可用检查。
- 是否达到 commit / push 条件；未达到则只记录本地状态，不强行同步 GitHub。
- 本文的状态、风险和下一步是否仍然准确。

## 11. 当前风险

- 当前 Codex 进程可能尚未重新读取用户 PATH；本线程运行 Go 命令前执行 `. .\tools\go-env.ps1`。
- 现阶段已有 app/domain/PostgreSQL repository 测试、outbox relay / Kafka producer 测试、gRPC adapter 测试和真实进程多线程压测入口。
- 当前 Kafka writer 使用 `segmentio/kafka-go`，已配置 `acks=all`、hash key 和禁用自动建 topic，但该 Writer 不暴露 Kafka `enable.idempotence=true` 开关；生产硬化时需要更换支持幂等 producer 的 client 或接入更底层 transactional producer 能力。
- 当前 outbox relay 的 publish callback 在 PostgreSQL 事务内执行，这是第一阶段可接受的至少一次发布取舍；当前已支持 batch publish，但事务仍覆盖 Kafka publish callback、batch mark 和 commit；压测阶段需要重点观察 batch size、Kafka publish latency、DB lock wait 和重复发布窗口。
- 当前 relay 只支持 `message.persisted.v1`；启用 Edit/Revoke/Delete 前必须补齐对应 Kafka oneof payload 构造和测试。
- OutboxStore 已补真实 PostgreSQL 多 worker / `FOR UPDATE SKIP LOCKED` 集成测试，证明同 conversation 顺序不乱、跨 conversation 可并发追平；后续仍可补强 `available_at/next_retry_at` 未到期、低版本 PENDING 阻塞等 ready 条件测试。
- 当前 relay 支持可配置多 worker；只有 `Published > 0` 时才立即继续循环，`Fetched > 0` 但没有成功发布时按 `FailureBackoff` 退避，空转时按 `PollInterval` sleep；同一 conversation 仍按最低 `aggregate_version` 串行发布，因此单会话积压追平能力仍受顺序保护限制。
- 当前 relay 已缓解 Kafka 故障且 backlog 很大时的连续失败放大，但生产化仍需要结合失败比例、Kafka publish latency、DB lock wait 调整 backoff、batch size 和 worker 数。
- 当前 ready 判断使用 DB `now()`，retry 时间写入使用应用时钟；生产硬化时需要统一时间源或明确 DB/relay 节点时钟同步要求。
- 当前尚未实现 DLQ repair/replay；未来实现时必须清理 `dead_lettered_at`、`last_error`、`next_retry_at` 等旧失败字段。
- 已有真实进程级 SendMessage smoke 和 `--vus=100 --duration=60s` baseline 结果；baseline 写入成功率 100%，但 p99 与 outbox backlog 暴露出性能风险。
- 压测 summary 已支持从 gRPC 进程和 relay 进程的 `/debug/metrics` 读取 `conversation_seq_alloc_latency_ms`、兼容旧报告的 `kafka_publish_latency_ms`、`kafka_publish_call_latency_ms`、`kafka_publish_records_per_call`、`kafka_publish_record_latency_estimate_ms`、`outbox_process_ready_latency_ms`、`outbox_process_ready_active_latency_ms`、`outbox_process_ready_idle_latency_ms`、`outbox_fetched_per_call`、`outbox_fetch_ready_latency_ms`、`outbox_mark_published_latency_ms` 和 `outbox_commit_latency_ms`；生产化仍需接入统一 metrics/tracing，而不是依赖本地 debug endpoint。
- 当前 `51772e6` baseline：45212/45212 成功，p95 249.62ms，p99 518.03ms，`stats-wait=30s` 后本轮 tenant outbox `PENDING=27181`、`PUBLISHED=18031`。
- 当前 worker/backoff dirty baseline：`NEXUSIM_OUTBOX_WORKERS=4`、`--vus=100 --duration=60s --stats-wait=30s --conversation-count=1000`，69608/69608 成功，p95 122.10ms，p99 156.24ms，`stats-wait=30s` 后本轮 tenant outbox `PENDING=2123`、`PUBLISHED=67485`；relay 额外 drain 20s 后该 tenant outbox 全部 `PUBLISHED=69608`。该数据用于开发判断，不能作为 clean commit 正式性能归档。
- 当前 metrics clean smoke：commit `ea4eb9a`，`--vus=10 --duration=10s --stats-wait=10s --conversation-count=200`，8699/8699 成功，p95 19.46ms，p99 31.68ms，outbox pending 0；summary 已写入 `conversation_seq_alloc_latency_ms=1.47`、`conversation_seq_alloc_p95_ms=2.59`、`kafka_publish_latency_ms=1.01`、`kafka_publish_p95_ms=1.73`，且 `git_dirty=false`。
- 当前 worker/backoff clean baseline：commit `0ff42d2`，`NEXUSIM_OUTBOX_WORKERS=4`、`--vus=100 --duration=60s --stats-wait=30s --conversation-count=1000`，24714/24714 成功，p95 436.24ms，p99 583.96ms，summary 读取时本轮 tenant outbox `PENDING=392`、`PUBLISHED=24322`、`DLQ=0`，`conversation_seq_alloc_latency_ms=6.98`、`kafka_publish_latency_ms=4.21`，且 `git_dirty=false`；随后查询该 tenant outbox 已全部 `PUBLISHED=24714`。该结果说明 4 worker 能追平，但本机长压测吞吐波动明显，需要重复 clean baseline 或梯度压测后再形成正式性能结论。
- 当前 gradient script dirty smoke：`run-local-gradient.ps1 -Workers 4,8,16 -VUs 20 -Duration 10s -StatsWait 10s -ConversationCount 500` 已跑通三组真实进程压测，全部成功且 outbox pending 0；由于脚本未提交，summary 为 `git_dirty=true`，仅作为脚本可运行验证，不作为正式梯度趋势证据。
- 当前 Windows Docker Desktop 已按用户要求调整为 WSL2 `processors=16`、`memory=24GB`、`swap=8GB`，`docker info` 显示 Docker VM 可用 `16 CPU`、约 `23.47GiB` 内存。
- 当前 win-win Docker 资源矩阵已覆盖 `1/2/4 CPU + 256m/512m/1g`、`8/12/16 CPU + 2g/4g/8g`、以及 `16 CPU + 23g` 档位。按 `success_rate >= 0.99`、`p99 <= 1000ms`、`outbox_pending_count <= 1000` 的门槛，已观察到的最佳通过档为 `16 CPU / 23g / relay workers=8 / 1200 VU`，约 `2493 rps`、p99 `736.28ms`、outbox pending 0；`1600 VU` 时 p99 `1120.48ms` 超线。`16 CPU / 23g / relay workers=16` 在 `1200 VU` 即 p99 `1477.19ms` 超线，说明盲目增加 relay worker 会放大争用。
- 当前 Windows+Mac 分布式双客户端已跑通：Windows 服务端暴露 `10495/10497/10500`，Windows 和 Mac 同时作为 load generator。`600+600 VU` 双客户端全部成功，Windows client p99 `730.11ms`、Mac client p99 `739.50ms`、outbox pending 0；`1000+1000 VU` 双客户端全部成功但 p99 均约 `1331ms`，按当前尾延迟门槛超线。
- 当前压测趋势图已生成到 `loadtest/results/charts/`：`winwin-rps-trend.png`、`winwin-p99-trend.png`、`distributed-clients-trend.png`，摘要为 `loadtest/results/charts/winwin-distributed-summary.md`；这些结果文件默认不提交。
- 当前压测正式报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609.md`，已记录压测拓扑、执行方式、通过标准、结果摘要和瓶颈排查过程。
- 当前 PG pool / multi-instance 诊断报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-pgpool-multi-instance.md`，已记录 repository 细分指标、多 target loadtest、多 service metrics URL、PG pool smoke、multi-instance smoke、正式 PG pool 矩阵和正式 multi-instance 矩阵。
- 当前 p99 瓶颈已初步定位：`16 CPU / 23g / 1600 VU / workers=8` 下，request p99 基本等于 repository append p99；commit、conversation_seq、Kafka publish 都是毫秒级；service pgxpool 默认 `max_conns=16` 时 acquire 平均等待约 `646ms`，`NEXUSIM_PG_MAX_CONNS=64` 后 1600 VU p99 改善到 `779.63ms`，但 2400 VU 仍 p99 `1452.87ms` 超线。下一步优先做 PG 连接池梯度、多 message-service 实例和 repository 细分打点。
- 当前 repository 细分指标已落地：`repository_begin_latency_ms`、`repository_pool_acquire_latency_ms`、`repository_tx_begin_latency_ms`、`repository_idempotency_lock_latency_ms`、`repository_find_existing_latency_ms`、`repository_ensure_seq_latency_ms`、`repository_allocate_seq_latency_ms`、`repository_insert_message_latency_ms`、`repository_insert_timeline_latency_ms`、`repository_insert_outbox_latency_ms`、`repository_commit_latency_ms`。
- 当前 clean commit `e87bb9b` PG pool smoke：`PG_MAX_CONNS=16/64`、`VU=20`、`duration=5s`、`stats-wait=5s`，两组全部成功；p99 分别为 `42.52ms`、`33.46ms`，结果在 `loadtest/results/pgpool-smoke-20260609-013424/`，后续短 relay drain 后对应 outbox 均为 `pending=0`。
- 当前 clean commit `e87bb9b` multi-instance smoke：`Instances=1/2`、`VU=20`、`duration=5s`、`stats-wait=5s`，两组全部成功；p99 分别为 `40.43ms`、`39.35ms`，结果在 `loadtest/results/multi-instance-smoke-20260609-013511/`，多 target 和多 service metrics URL 均已验证；后续短 relay drain 后对应 outbox 均为 `pending=0`。
- 当前 formal PG pool 矩阵：`loadtest/results/pgpool-formal-20260609-014259/`。`PG_MAX_CONNS=16` 时 1200 VU 仍 100% 成功但 p99 `1725.16ms`；1600 VU 成功率降到 `0.6870`。`PG_MAX_CONNS=32` 时 1200/1600 VU 成功率 100%，但 p99 仍为 `1381.33ms` / `1476.37ms`；2000 VU 成功率 `0.9712`。`PG_MAX_CONNS=64` 时 1200/1600/2000 VU 写入成功率高，但 outbox pending 分别升到 `8044` / `19851` / `49948`，说明写入并发放大后 relay 追平成为第二瓶颈。`PG_MAX_CONNS=96/128` 在当前 PostgreSQL `max_connections` 下触发 `FATAL: sorry, too many clients already`，结果无效。
- 当前 formal multi-instance 矩阵：`loadtest/results/multi-instance-formal-20260609-021254/`。在每实例 `PG_MAX_CONNS=16`、1200 VU 下，1/2/4 实例成功率分别为 `0.9660` / `0.5572` / `0.9014`，请求 p99 均约 `2000ms`；用 `service_metrics[]` 重新计算后，4 实例的 per-instance `repository_begin_p99` 范围为 `603.59ms` 到 `2002.33ms`，说明不是每个实例都打满，但整体请求 p99 仍未改善。在当前单 PostgreSQL、当前连接上限和当前写入模型下，多实例不能解决尾延迟。
- 当前 formal 矩阵追加短 relay drain 后确认 `tenant_count=16 total_pending=0`，没有留下未发布 outbox 积压。
- 当前 PostgreSQL 诊断脚本为 `loadtest/sendmessage/collect-postgres-diagnostics.ps1`。正式矩阵后采集结果在 `loadtest/results/postgres-diagnostics-20260609-022602/postgres-diagnostics.json`：`max_connections=100`、`shared_buffers=16384`、`max_wal_size=1024`、`checkpoint_timeout=300`、`synchronous_commit=on`、`deadlocks=0`；`message_outbox n_dead_tup=113312`，说明 outbox 高频 update 已产生明显 dead tuples。
- 当前 multi-instance PG budget 矩阵：`loadtest/results/multi-instance-budget-formal-20260609/`，报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-multi-instance-budget.md`。固定每实例预算 `1x16/2x16/4x16` 下 p99 分别为 `2000.53ms` / `1437.37ms` / `1975.75ms`；固定总预算 `1x64/2x32/4x16` 下 p99 分别为 `1178.97ms` / `1509.55ms` / `1657.18ms`。固定总预算下多实例没有降低尾延迟，且 outbox pending 为 `31889` / `64553` / `65323`，说明当前应优先处理 PostgreSQL acquire/begin 排队与 relay 追平，而不是继续堆 gRPC 实例。
- 当前 PostgreSQL loadtest profile 矩阵：`loadtest/results/pgpool-tuned-formal-20260609/`，报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-postgres-loadtest-profile.md`。启用 `max_connections=200`、`shared_buffers=1GB`、`max_wal_size=4GB` 后，`PG_MAX_CONNS=64/VU1200` p99 `1161.70ms`，`PG_MAX_CONNS=64/VU1600` p99 `1759.11ms`；`PG_MAX_CONNS=128` 未改善，1200 VU 成功率 `0.9760` 且 p99 `2001.08ms`。新指标确认 `repository_begin` 主体是 `repository_pool_acquire`，`repository_tx_begin` p99 仅约 `14-33ms`。watch 采样显示 `LWLock:WALWrite`、`LWLock:WALInsert`、`LWLock:BufferContent` 和 `CheckpointWriteDelay` 已进入瓶颈视野。
- 当前 backpressure on/off 正式矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-backpressure-onoff.md`，结果路径为 `loadtest/results/backpressure-off-formal-20260609/` 与 `loadtest/results/backpressure-on-formal-20260609/`。固定 `PG_MAX_CONNS=64`、relay workers 8 时，off 模式 1200/1600 VU 均 100% 成功，但 success p99 为 `1187.23ms` / `1735.38ms`，且 outbox pending 为 `30689` / `47736`；on 模式 attempt-level overload rate 为 `97.28%` / `98.01%`，error p99 仅 `12.49ms` / `14.26ms`，但 success p99 仍为 `1403.95ms` / `1808.10ms`。结论：backpressure 快速拒绝有效且能降低 backlog，但当前 `MinAvailableConns=0` 策略过于粗糙，不能宣称成功请求 p99 改善。
- 当前 backpressure 阈值梯度：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-backpressure-gradient.md`，结果路径为 `loadtest/results/backpressure-minavail-{0,4,8,16}-formal-20260609/`。`MinAvailable=0` 复跑出现 DeadlineExceeded 和 DB_WRITE_FAILED，说明拒绝太晚；`MinAvailable=4/8` 错误基本稳定为 `SERVICE_OVERLOADED`，outbox pending 均为 0。`MinAvailable=8` 是下一轮优先验证的短期候选之一：1200 VU accepted RPS `818.73`、success p99 `1191.10ms`；1600 VU accepted RPS `933.93`、success p99 `1249.85ms`。该策略仍牺牲大量请求成功率，只能作为保护阈值候选，不是生产默认值或容量提升方案。
- 当前 client retry 正式矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-client-retry.md`，结果路径为 `loadtest/results/backpressure-client-retry-formal-20260609/`。固定 `MinAvailable=8`、`RetryInfo=500ms`、`max_retries=2`、`jitter=100ms` 时，1200 VU logical success rate `68.68%`、accepted RPS `1526.23`、success p99 `421.58ms`、outbox pending `25579`；1600 VU logical success rate `56.04%`、accepted RPS `1288.98`、success p99 `1154.47ms`、outbox pending `32091`。错误全部是 `SERVICE_OVERLOADED`，没有 DeadlineExceeded / DB_WRITE_FAILED；但 accepted 写入回升后 outbox relay 再次成为瓶颈。
- 当前 outbox relay 批量 mark published：`OutboxStore.ProcessReady` 已把成功 publish 后的逐条 `UPDATE message_outbox SET status='PUBLISHED'` 改为同一事务内按 id 数组批量 update；真实 PostgreSQL 集成测试 `TestOutboxStoreProcessReadyBatchMarksPublished` 已验证两条跨 conversation outbox 可同批标记为 published，并已用 client retry 矩阵重跑验证 pending 明显下降。
- 当前 outbox batch mark 对照矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-batchmark.md`，结果路径为 `loadtest/results/backpressure-client-retry-batchmark-formal-20260609/`。对比 `0c542a1` before 与 `b6f0b82` after，1200 VU outbox pending 从 `25579` 降到 `5186`，1600 VU 从 `32091` 降到 `0`；但 accepted RPS 和 success p99 有本机波动，不能宣称整体容量提升。当前结论只限于：批量 mark published 明显降低 relay backlog。
- 当前 relay metrics smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-relay-metrics-smoke.md`，结果路径为 `loadtest/results/relay-metrics-smoke-20260609/bpoff-pgmax-16-vu-10-20260609-052152/sendmessage-summary.json`。commit `148938e`、`git_dirty=false`、5244/5244 成功、p95 `12.334ms`、p99 `15.8853ms`、outbox pending 0；summary 已写入 `outbox_process_ready_latency_ms=43.2098`、`outbox_fetch_ready_latency_ms=18.1205`、`outbox_mark_published_latency_ms=2.1892`、`outbox_commit_latency_ms=2.6068`。该结果只证明观测链路可用，不作为容量结论。
- 当前 outbox batch size 探索矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-batchsize-smoke.md`，结果路径为 `loadtest/results/outbox-batchsize-smoke-20260609/`。commit `aeaf88a`、`git_dirty=false`，固定 `PG_MAX_CONNS=64`、relay workers 8、1200 VU、30s、客户端 retry；batch 100/500/1000 均 outbox pending 0。batch 500 的 logical success rate `0.7160` 和 accepted RPS `1694.93` 较好，但 `outbox_process_ready_latency_ms=415.83ms` 最高；batch 100 success p99 最低为 `584.42ms`。当前只把 batch 500 作为下一轮正式矩阵候选，不作为最终默认值。
- 当前 outbox PublishBatch smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-publishbatch-smoke.md`，结果路径为 `loadtest/results/outbox-publishbatch-smoke-20260609/` 与 `loadtest/results/outbox-publishbatch-smoke-repeat-20260609/`。commit `9f26d0c`、`git_dirty=false`，固定 batch 500、1200 VU、30s。after run 2 logical success rate `0.7125`、accepted RPS `1658.80`、success p99 `576.48ms`、outbox pending 0；`outbox_process_ready_latency_ms` 从 before 的 `415.83ms` 降到 `41.95ms`。但 after run 1 波动明显，不能宣称整体容量已提升；batch path 下 `kafka_publish_latency_ms` 是 batch 调用耗时，不再等价于旧单条 publish 耗时。
- 当前 PublishBatch metrics smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-publishbatch-metrics-smoke.md`，结果路径为 `loadtest/results/publishbatch-metrics-smoke-20260609/bpoff-pgmax-16-vu-10-20260609-055454/sendmessage-summary.json`。commit `8742f84`、`git_dirty=false`、5645/5645 成功、p99 `12.7919ms`、outbox pending 0；summary 已写入 `kafka_publish_call_latency_ms=11.5020`、`kafka_publish_records_per_call=21.0672`、`kafka_publish_record_latency_estimate_ms=0.7350`。该结果只证明指标链路可用，不作为容量结论。
- 当前 PublishBatch formal 矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-publishbatch-formal.md`，有效结果路径为 `loadtest/results/publishbatch-formal-valid-{off,on}-r{1,2}-20260609/`。commit `aec449c`、`git_dirty=false`，固定 `PG_MAX_CONNS=64`、relay workers 8、batch 500、backpressure min 8、客户端 retry。`PublishBatch=false` 时 `kafka_publish_records_per_call=1`，1200/1600 VU 的 stats wait pending 平均为 `33776` / `36980`；`PublishBatch=true` 时 records/call 平均为 `381.08` / `188.60`，1600 VU pending 平均降到 `0`，但 1200 VU 仍有平均 `14904` pending，且 success p99 未稳定改善。第一次矩阵因 `-SkipBuild` 误用旧二进制导致 `pbatchoff` 实际未关闭 batch，已标记为无效中间结果，不作为正式证据。
- 当前 outbox batch/worker 联合矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-batch-worker-matrix.md`，结果路径为 `loadtest/results/outbox-batch-worker-matrix-{1200,1600}-formal-20260609/`。commit `134613c`、`git_dirty=false`，固定 `PG_MAX_CONNS=64`、`PublishBatch=true`、backpressure min 8、客户端 retry；batch 100/500/1000 与 workers 8/12/16 的 18 组全部在 `stats_wait=20s` 后 pending 0。1200 VU 下 batch 500/workers 8 accepted RPS 最高为 `1969.90`；1600 VU 下 batch 100/workers 8 accepted RPS 最高为 `1873.23` 且 success p99 最低为 `667.54ms`。当前只把 `100/8` 与 `500/8` 作为下一轮重复验证候选，不作为最终默认值。
- 当前 outbox active/idle metrics smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-active-idle-metrics-smoke.md`，结果路径为 `loadtest/results/outbox-active-idle-metrics-smoke-20260609/batch-100-workers-2/bpon-pbatchon-pgmax-16-vu-10-20260609-070214/sendmessage-summary.json`。commit `40baec9`、`git_dirty=false`、5487/5487 成功、pending 0；summary 已写入 `outbox_process_ready_active_latency_ms=18.4141`、`outbox_process_ready_idle_latency_ms=2.2710`、`outbox_fetched_per_call=12.2752`。该结果只证明观测链路可用，不作为容量结论。
- 当前 outbox 候选重复矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-candidate-repeat.md`，结果路径为 `loadtest/results/outbox-candidate-repeat-r{1,2}-20260609/`。commit `c41b26a`、`git_dirty=false`，重复验证 `batch_size=100/workers=8` 和 `batch_size=500/workers=8`。两组均 pending 0；`100/8` 的 success p99 更稳，1200/1600 VU 平均为 `524.74ms` / `818.47ms`，优于 `500/8` 的 `696.28ms` / `1150.13ms`。当前本地 relay 基线收敛为 `batch_size=100/workers=8`。
- 当前 adaptive limit smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-limit-smoke.md`，结果路径为 `loadtest/results/adaptive-limit-smoke-final-20260609/bpoff-adapton-pbatchon-pgmax-4-vu-5-20260609-072955/sendmessage-summary.json`。commit `d2af748`、`git_dirty=false`，使用极端阈值 `PG_MAX_CONNS=4`、`AdaptiveMinAvailableConns=4` 验证提前拒绝链路；15136 次请求全部返回 retryable `SERVICE_OVERLOADED`，p99 `3.1742ms`，本轮 tenant outbox `total=0/pending=0/DLQ=0`。该结果只证明 adaptive admission 可运行，不作为容量结论。
- 当前 adaptive limit on/off 矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-limit-onoff.md`，有效结果路径为 `loadtest/results/adaptive-onoff-v3-repo-backpressure-20260609/` 和 `loadtest/results/adaptive-relaxed-v1-admission-20260609/`。commit `7d6e59f`、`git_dirty=false`，固定 `PG_MAX_CONNS=64`、`batch_size=100`、`workers=8`、客户端 retry；relaxed app adaptive 在 1200/1600 VU 下 accepted RPS 为 `1858.43` / `1772.47`，success p99 为 `575.07ms` / `709.94ms`，outbox pending 均为 0。该结果说明 app admission 可作为入口保护，但没有证明容量提升；累计 p95 和 idle relay 样本不能作为独立硬拒绝条件。
- 当前 recent metrics smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-recent-metrics-smoke.md`，结果路径为 `loadtest/results/recent-metrics-smoke-20260609/bpoff-adapton-pbatchon-pgmax-16-vu-10-20260609-075920/sendmessage-summary.json`。commit `6d4910b`、`git_dirty=false`，5392/5392 成功、p99 `14.5047ms`、outbox pending 0；summary 已写入 `repository_pool_acquire_recent_latency_ms`、`outbox_process_ready_active_recent_latency_ms`、`outbox_fetched_per_call_recent`、`kafka_publish_records_per_call_recent`。该结果只证明窗口化观测链路可用，不作为容量结论。
- 当前 adaptive hysteresis smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-hysteresis-smoke.md`，结果路径为 `loadtest/results/adaptive-hysteresis-smoke-20260609/bpoff-adapton-pbatchon-pgmax-16-vu-10-20260609-080738/sendmessage-summary.json`。commit `6f9a438`、`git_dirty=false`，5554/5554 成功、p99 `12.5034ms`、service overloaded 0、outbox pending 0；该结果只证明 hysteresis 配置链路可运行，不作为容量结论。
- 当前 adaptive retry hint smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-retry-hint-smoke.md`，结果路径为 `loadtest/results/adaptive-retry-hint-metrics-smoke-20260609/adaptive-retry-hint-metrics-grpc-only-20260609-082000/sendmessage-summary.json`。commit `c9e6cf1`、`git_dirty=false`，极端过载下 57 次 gRPC attempt 全部返回 `SERVICE_OVERLOADED`，outbox pending 0；summary 已记录 `retry_delay_count=31`、`retry_delay_avg_ms=491.94`、`retry_delay_p95_ms=500`、`retry_delay_p99_ms=500`，证明压测器读取并执行了 gRPC `RetryInfo`。
- 当前评审确认 `retry_delay_count` 表示收到并计划遵守的 RetryInfo 数量，不严格等于完成 sleep 后进入下一次 attempt 的次数；正式 adaptive 矩阵必须同时展示 `retry_delay_count`、`retry_attempt_count` 和 logical success，避免把计划等待次数误读成真实重试次数。
- 当前评审确认 `*_recent` 是最近 4096 个样本窗口，不是时间窗口；低流量或样本数低于 `MinMetricSamples` 时不能基于 recent 指标下调参结论，后续报告必须展示 recent sample count。
- 当前评审确认 relay 相关 adaptive 条件隐含依赖 outbox pending 采样；如果配置了 relay active p95、outbox fetched per call 或 Kafka records per call 条件，必须同时配置 outbox pending 采样阈值，否则这些 relay 条件不会独立触发拒绝。
- 当前 dynamic retry hint 公式仍是启发式：`min(reason_count * base_delay, max_delay)`，recovering 状态额外加一档；不能作为稳定控制律或最优 retry delay，后续应结合 accepted RPS、outbox drain rate 和 pool acquire recent p95 调整。
- 当前 adaptive threshold matrix v1：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-threshold-v1.md`，正式结果路径为 `loadtest/results/adaptive-threshold-v1-clean-20260609/`。commit `9830f34`、`git_dirty=false`，固定 `PG_MAX_CONNS=64`、relay workers `8`、batch size `100`、PublishBatch on、`max_retries=2`、`retry_jitter=100ms`。本轮最佳候选为 `gap8-outbox50-base1000`：1200 VU logical success `80.78%`、accepted RPS `578.30`、success p99 `1955.41ms`；1600 VU logical success `56.28%`、accepted RPS `456.07`、success p99 `1819.91ms`；两组 outbox pending 均为 0。该结果说明更长 retry hint 能缓解重试压力，但 adaptive admission 仍是保护机制，不是容量提升结论。
- 当前 adaptive best candidate 60s repeat：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-best-repeat.md`，正式结果路径为 `loadtest/results/adaptive-threshold-best-repeat-20260609/`。commit `fb872a9`、`git_dirty=false`，重复验证 `gap8-outbox50-base1000`。1200 VU 两轮 logical success 为 `39.01%` / `41.69%`，accepted RPS 为 `261.55` / `273.58`；1600 VU 两轮 logical success 为 `27.96%` / `30.59%`，accepted RPS 为 `226.45` / `246.82`；所有 outbox pending 均为 0。该结果推翻了 30s 短测的乐观判断，说明当前 adaptive admission 过度保护。
- 当前 logical latency smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-logical-latency-smoke.md`，结果路径为 `loadtest/results/logical-latency-smoke-20260609/bpoff-adapton-pbatchon-pgmax-4-vu-5-20260609-092242/sendmessage-summary.json`。commit `91cbb8c`、`git_dirty=false`，attempt p99 `7.3318ms`，logical p99 `507.6808ms`，retry delay p95 `500ms`，证明 summary 已能把 retry sleep 计入用户层等待。
- 当前 adaptive pool acquire p95 matrix v1：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-poolp95-v1.md`，正式结果路径为 `loadtest/results/adaptive-poolp95-v1-20260609/`。commit `b98e3c9`、`git_dirty=false`，对比 `AdaptiveMaxPoolAcquireP95=250ms/500ms/750ms`。所有组合 outbox pending 均为 0；1200 VU 下 accepted RPS 最高为 `277.55`，logical p99 仍为 `5968.57ms`；1600 VU 下 accepted RPS 约 `195-203`，logical p99 约 `5091-5463ms`。该结果说明单纯放宽 pool acquire p95 阈值不能解决当前过度保护和用户层等待过长问题。
- 当前 adaptive in-flight limit v1：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-inflight-v1.md`，正式结果路径为 `loadtest/results/adaptive-inflight-v1-20260609/` 和 `loadtest/results/adaptive-inflight-repeat-20260609/`。commit `7bf37fe`、`git_dirty=false`，`MaxInFlight=64` 的 60s repeat 在 1200/1600 VU 下 accepted RPS 分别为 `1922.23/1926.97`，success p99 分别为 `63.58ms/63.35ms`，outbox pending 均为 0；logical p99 仍约 `2.2s`，说明下一步瓶颈转向 retry/admission 策略而不是 DB 写入尾延迟。
- 当前 debug metrics collector 保存全量样本并在 snapshot 时排序，适合本地短压测，不适合作为生产 metrics；后续应替换为固定窗口、reservoir、HDR histogram 或 Prometheus histogram。
- `CONVERSATION_NOT_FOUND`、`MESSAGE_TOO_LARGE`、`SEQ_BLOCK_EXHAUSTED` 错误 sentinel 和 gRPC 映射暂未补齐；phase-1 普通会话 happy path 不阻塞，但不能声称完整错误契约已完成。
- 当前 raw gRPC server 还没有统一 deadline / trace / metrics interceptor；后续接 Kratos 或统一 gRPC interceptor。
- `delivery-service` 已完成最小 projection / PullInbox / AckDelivery / delivery outbox relay 链路；`push-gateway` SDD v0.1 Draft 已存在但尚未评审冻结；`timeline-service` SDD 未冻结；`conversation-service / member_change_saga` SDD 已冻结，Proto / Kafka schema / migration v2 和 relay builder 已补；成员变更代码必须继续通过 shared timeline/outbox append port 落库，不得绕过统一 outbox。
- `conversation-service` 当前已实现 `CreateMemberChange`、`GetMemberChange` 和最小 saga publish progress worker；DLQ repair 仍未完成。
- `conversation-service` 当前已完成 `CreateMemberChange(JOIN)` 写路径 smoke 和 `CreateMemberChange -> outbox relay -> member-change-worker -> GetMemberChange(DONE)` full smoke；`LEAVE / REMOVE / ROLE_CHANGED` 真实进程 smoke 可后置。
- 统一 outbox relay 当前仍位于 `message-service/internal/trigger/outbox`，但后续会发布 message/member 两类 conversation timeline event；这是阶段性部署折中，生产化前需要在 TADD 中决定是否拆成独立 `timeline-outbox-relay`。

## 12. 最近评审状态

- 2026-06-08：独立评审线程指出文档入口顺序、评审回传规则、GitHub 同步闭环、压测硬约束和目标态总架构入口需要补强；本轮已按建议更新本文和 `docs/README.md`。
- 2026-06-08：独立评审线程复核通过文档闭环；新增 P0 环境结论：`protoc` 可用，但 `go`、`protoc-gen-go`、`protoc-gen-go-grpc` 未检测到，正式实现和验证前必须补齐。
- 2026-06-08：已通过阿里云镜像安装 Go `1.26.4`，并通过 `GOPROXY=https://goproxy.cn,direct` 安装 `protoc-gen-go` 和 `protoc-gen-go-grpc`；按用户要求不刻意压低 Go 版本，项目基线已设为 Go `1.26.4`；`tools/gen-proto.ps1` 与 `go test ./...` 已通过。
- 2026-06-08：已实现 `SendMessageUseCase` 单元测试、领域 command hash / append record、PostgreSQL repository 本地事务；`go test ./...` 通过，`NEXUSIM_PG_DSN=postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable` 的 repository 集成测试通过。
- 2026-06-08：根据评审意见修复同一 `client_msg_id` 并发重复请求可能推进 `conversation_seq` 的问题，repository 已使用同幂等键 advisory transaction lock，并新增真实 PostgreSQL 并发集成测试；同时修复 payload JSON canonical 只压缩不稳定排序的问题。
- 2026-06-08：根据评审意见补齐 `MessagePersistedV1` payload 中的 `command_hash`，明确 `message_outbox.payload_json` 保存业务 payload、envelope/metadata 由 outbox 表字段组装；app 层已增加 `permission_version` 不一致时短重试一次，仍不一致返回可识别 dependency version error；outbox 写入失败已拆为 `ErrOutboxWriteFailed`。
- 2026-06-08：已实现 `trigger/outbox` relay、PostgreSQL outbox store 和真实 Kafka writer producer；本地已启动 `nexusim-kafka`，创建 `conversation.timeline.events` topic，并通过真实 PostgreSQL + Kafka 集成测试验证 outbox 可发布后标记 `PUBLISHED`，Kafka publish 失败时保留 pending/retry/DLQ 状态。
- 2026-06-08：独立评审线程复核 outbox relay + Kafka publish path，无 P0/P1 阻塞；P2/P3 风险已记录到本文，下一步可以提交本轮切片并推进 `message-service` gRPC adapter 与本地多线程压测。
- 2026-06-08：已实现 `message-service` gRPC adapter 和 `NEXUSIM_MESSAGE_SERVICE_MODE=grpc` 运行入口；adapter 已覆盖请求转换、响应转换、稳定错误码 detail、unsupported message type、bufconn client 注册调用。按批量策略，本轮暂不单独评审/推送，和后续 loadtest runner + smoke 结果一起评审。
- 2026-06-08：已实现 `loadtest/sendmessage` gRPC 压测 CLI，并修正 relay 有 ready event 时仍固定 sleep 的追平问题；真实进程 smoke 使用 gRPC server + outbox relay + PostgreSQL + Kafka，`--vus=2 --duration=3s --stats-wait=8s --conversation-count=100` 结果为 1020/1020 成功、p95 7.63ms、p99 9.20ms、本轮 tenant outbox 0 pending。
- 2026-06-08：已在当前 HEAD `51772e6` 跑第一轮 baseline：`--vus=100 --duration=60s --stats-wait=30s --conversation-count=1000`，结果为 45212/45212 成功、p95 249.62ms、p99 518.03ms、outbox pending 27181；已把该结果补充给评审线程作为当前提交验证证据。
- 2026-06-08：独立评审线程复核 gRPC adapter + loadtest runner，指出 P1：gRPC status/detail 暴露底层 DB/PG 错误文本；本轮已改为稳定 public message，并补 `TestSendMessageSanitizesInternalErrorMessages`。同时已补 loadtest summary 的 full commit、dirty 状态、outbox total/published/pending/DLQ 字段。
- 2026-06-08：独立评审线程复核确认 gRPC 错误脱敏 P1 已解除；同时指出当前工作区有未提交 relay worker/backoff 优化雏形。本轮已继续完善该切片，补 `NEXUSIM_OUTBOX_WORKERS` / `NEXUSIM_OUTBOX_FAILURE_BACKOFF` wiring、worker 并发测试、失败退避测试，并跑 4 worker 真实 baseline：69608/69608 成功，p95 122.10ms，p99 156.24ms，`stats-wait=30s` 后 pending 2123，额外 drain 20s 后清零。
- 2026-06-08：独立评审线程复核 `cef25f1 feat: add outbox relay worker controls`，结论为无 P0/P1，可作为 relay 追平能力优化切片保留；P2 跟踪项包括 clean commit baseline 归档、部分成功大量失败场景的退避策略、真实 PostgreSQL 多 worker / `SKIP LOCKED` 集成测试、worker 上限与配置日志。
- 2026-06-08：已补本地 debug metrics collector，gRPC 进程记录 `conversation_seq_alloc_latency`，relay 进程记录 `kafka_publish_latency`；`loadtest/sendmessage` 支持 `--service-metrics-url` 和 `--relay-metrics-url` 并把 avg/p95 写入 summary。commit `ea4eb9a` 的 clean metrics smoke 已验证两个指标非空。
- 2026-06-08：已补真实 PostgreSQL 多 worker / `FOR UPDATE SKIP LOCKED` 集成测试：两个 conversation 各 3 条 outbox，4 个并发 worker `ProcessReady(limit=1)`，断言跨 conversation 可同时进入 publish callback、同 conversation 发布顺序保持 `1,2,3`，最终 outbox 全部 `PUBLISHED`。
- 2026-06-08：已在 clean HEAD `0ff42d2` 跑 4 worker 长 baseline，summary 为 24714/24714 成功、p95 436.24ms、p99 583.96ms、`stats-wait=30s` 后 pending 392；随后 DB 查询显示该 tenant 已全部 published。由于该结果与 dirty baseline 吞吐差异较大，后续需要重复 clean baseline 或按 4/8/16 worker 梯度压测确认趋势。
- 2026-06-08：已补 `loadtest/sendmessage/run-local-gradient.ps1`，用于本机按 worker 数循环启动真实 gRPC / outbox relay / loadtest 进程并采集 metrics；dirty smoke 已验证 4/8/16 worker 短梯度脚本可运行。
- 2026-06-09：已提交 `e87bb9b feat: add multi-instance loadtest diagnostics`，补齐 repository 内部分段指标、多 target loadtest、多 service metrics URL、`run-local-pgpool-gradient.ps1` 和 `run-local-multi-instance.ps1`；`go test ./...`、`go build ./services/message-service/cmd/message-service ./loadtest/sendmessage`、PowerShell 脚本语法检查和 `git diff --check` 均已通过。
- 2026-06-09：已在 clean commit `e87bb9b` 跑 PG pool 短 smoke 和 multi-instance 短 smoke，验证 summary 能记录 repository 分段指标和多实例 service metrics；该结果只作为工具链验证，不作为正式容量结论。下一步必须跑正式长矩阵后再申请阶段评审。
- 2026-06-09：已跑正式 PG pool 矩阵和正式 multi-instance 矩阵，并更新 `docs/runbook/loadtest/message-service/loadtest-report-20260609-pgpool-multi-instance.md`。阶段结论：当前证据显示 p99 主要贴在 `repository_begin`，`PG_MAX_CONNS=96/128` 超出 PostgreSQL 连接上限；在当前单 PostgreSQL、当前连接上限和当前写入模型下，多 `message-service` 实例没有降低尾延迟。下一步应转入 PostgreSQL 观测、写入路径优化、outbox relay 追平和 backpressure 设计。
- 2026-06-09：已新增并执行 `collect-postgres-diagnostics.ps1`，确认 PostgreSQL 当前 `max_connections=100`，解释 PG pool 96/128 档位不可用；已把正式矩阵和诊断结果发送给评审线程做阶段复核，本轮不频繁 push。
- 2026-06-09：评审线程阶段性指出多实例 summary 顶层 `service_latency_metrics` 只取第一个 service metrics URL，报告若直接使用会误导。本轮已修正 loadtest 聚合逻辑，并把报告表改为基于 `service_metrics[]` 的 per-instance min/max。
- 2026-06-09：已扩展 `run-local-multi-instance.ps1`，支持固定每实例 PG 连接预算和固定总 PG 连接预算两类对照实验；下一轮可直接跑 `FixedPerInstance` 与 `FixedTotal` 矩阵，避免实例数和总数据库连接预算混在一起。
- 2026-06-09：已用重建后的 loadtest 二进制跑 `FixedTotal` 短 smoke：`1x8` 与 `2x4` 两组均 100% 成功、outbox pending 0，summary 顶层 `service_pg_pool.max_conns` 分别为 `8` / `8`，证明多实例 PG pool 聚合和固定总预算脚本路径生效。
- 2026-06-09：已在 clean commit `ede5dd7` 跑正式 multi-instance PG budget 矩阵，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-multi-instance-budget.md`。结论：固定总 PG 连接预算时，1 个实例 p99 最低，2/4 实例没有收益；request p99 仍贴近 repository append/begin，outbox pending 暴露 relay 追平为第二瓶颈。矩阵结束后已额外 drain，DB outbox 当前全部为 `PUBLISHED`。
- 2026-06-09：已把 `repository_begin` 拆成 `repository_pool_acquire_latency_ms` 和 `repository_tx_begin_latency_ms`，原 `repository_begin_latency_ms` 保持总耗时用于兼容旧报告。clean smoke commit `c10338e`：`PG_MAX_CONNS=8`、`VU=5`、`duration=3s`，1833/1833 成功，outbox pending 0，两个新指标 count 均为 1833，`git_dirty=false`。
- 2026-06-09：已新增 `watch-postgres-diagnostics.ps1`，用于压测期间按间隔采集 PostgreSQL `pg_stat_activity` wait_event、锁等待、表 dead tuples、bgwriter 和 WAL 统计，输出 `postgres-wait-samples.jsonl`。under-load smoke：`PG_MAX_CONNS=8`、`VU=20`、`duration=5s`，采样 10 次，最大 active backend 为 8，抓到 `LWLock:WALWrite` 等 wait_event。
- 2026-06-09：已新增 `deploy/local/docker-compose.postgres-loadtest.yml` 作为压测专用 PostgreSQL override，不改变默认开发 compose；目标参数包括 `max_connections=200`、`shared_buffers=1GB`、`max_wal_size=4GB`、`checkpoint_timeout=15min` 和更积极的 autovacuum 阈值。
- 2026-06-09：已实际启用 PostgreSQL loadtest override 并跑正式 PG pool 矩阵，同时采集 wait_event。结论：调大 PostgreSQL 与 PG pool 不能单独解决 p99，`repository_pool_acquire` 仍是主等待段；`PG_MAX_CONNS=128` 会放大 commit/WAL/outbox 压力。下一步优先做 backpressure 和 outbox relay 批量优化。
- 2026-06-09：已实现默认关闭的 PostgreSQL pool backpressure，并新增 `SERVICE_OVERLOADED` 错误码。clean smoke commit `78e8375`：`NEXUSIM_PG_BACKPRESSURE_ENABLED=true`、`PG_MAX_CONNS=1`、`VU=20`、`duration=5s`，163055 请求中成功率 `0.0032`，p99 `1.6246ms`，top error 为 `Unavailable: service overloaded`，outbox pending 0；报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-backpressure.md`。
- 2026-06-09：已为 loadtest summary 增加 `retryable_error_count`、`service_overloaded_count` 和 `message_error_counts[]`。clean smoke commit `a9fbdf8`：`PG_MAX_CONNS=1`、`VU=10`、`duration=3s`，`retryable_error_count=62556`、`service_overloaded_count=62556`、`message_error_counts[0]=SERVICE_OVERLOADED`，outbox pending 0。
- 2026-06-09：评审线程指出正式 backpressure on/off 矩阵不能只看混合 p99，否则大量快速拒绝会掩盖成功写入体验。本轮已补 `success_p99_ms`、`error_p99_ms`、`accepted_rps`、`error_rps`、`overload_rate`，并用 clean commit `6f0aa55` 重跑 on/off 矩阵；新报告明确区分整体 p99、成功 p99 和错误 p99。
- 2026-06-09：已在 clean commit `dfb6776` 跑 `MinAvailableConns=0/4/8/16` backpressure 梯度，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-backpressure-gradient.md`。结论：`MinAvailable=0` 拒绝太晚，`MinAvailable=4/8` 的错误语义更稳定；`8` 只是下一轮优先验证的候选之一，同时开始设计 adaptive limit 和客户端退避策略。
- 2026-06-09：已为 gRPC `SERVICE_OVERLOADED` 增加标准 `RetryInfo` detail，当前固定建议延迟为 `500ms`；`MessageError` detail 保持原错误码、retryable 和 correlation id。后续可让 retry delay 跟随 adaptive limit 动态调整。
- 2026-06-09：loadtest 已支持可选 `--retry-overloaded --max-retries=N --retry-jitter=D`，用于模拟客户端遵守服务端 `RetryInfo` 后的有限重试；summary 会同时记录实际 gRPC attempts 和用户层 logical request，避免把重试次数误读成消息吞吐。
- 2026-06-09：已在 clean commit `0c542a1` 跑客户端遵守 `RetryInfo` 的正式矩阵，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-client-retry.md`。结论：客户端 retry 后错误语义稳定、logical success rate 提升，但 accepted RPS 回升使 outbox pending 重新积压；下一阶段优先做 outbox relay 批量优化。
- 2026-06-09：已完成 outbox relay 第一项低风险优化：成功发布的 outbox id 在同一事务内批量 mark `PUBLISHED`，并补真实 PostgreSQL 集成测试。该改动不改变 Kafka publish 顺序，也不改变至少一次投递语义。
- 2026-06-09：已在 clean commit `b6f0b82` 跑 batch mark 后的 client retry 对照矩阵，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-batchmark.md`。结论：pending 明显下降，但容量提升需要重复矩阵或进一步 batch publish 验证。
- 2026-06-09：独立评审线程复核 backpressure 口径修复与正式压测报告，结论为无 P0/P1；`success/error latency` 拆分已解除上一轮 P1。非阻塞建议：`MinAvailableConns=8` 只能写作下一轮候选之一，`RetryInfo=500ms` 是第一版固定 hint，不是验证出的最优值。
- 2026-06-09：独立评审线程复核 client retry + outbox batch mark published，结论为无 P0/P1；本轮已修正 `current-goal.md` 残留旧状态，并把 `overload_rate` 明确为 attempt-level 指标。非阻塞建议中的混合 batch 成功/失败测试已补充。
- 2026-06-09：已新增 relay 分段 metrics，覆盖 `ProcessReady` 总耗时、ready outbox fetch、批量 mark published 和 outbox commit；clean smoke commit `148938e` 已验证真实进程 summary 能读到这些指标。
- 2026-06-09：已跑 outbox batch size 100/500/1000 的 30 秒探索矩阵，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-batchsize-smoke.md`。结论：三组均可追平，batch 500 暂作为下一轮正式矩阵候选，但不能跳过重复验证。
- 2026-06-09：已实现 `OutboxStore.ProcessReadyBatch` 与 Kafka `WriterProducer.PublishBatch`，并在 clean commit `9f26d0c` 跑两轮短 smoke。结论：真实链路可运行且 outbox 可追平，`outbox_process_ready_latency_ms` 在第二轮明显下降；trigger 层已覆盖 Kafka batch error 和单条 payload build error；但单次波动大，需要正式重复矩阵和评审后再宣称阶段完成。
- 2026-06-09：根据评审 P2 已拆分 Kafka publish metrics，新增 `kafka_publish_call_latency_ms`、`kafka_publish_records_per_call`、`kafka_publish_record_latency_estimate_ms`；旧 `kafka_publish_latency_ms` 仅保留兼容。同时修复 `ProcessReadyBatch` 空批次仍调用 publish callback 的问题，并补直接调用 `ProcessReadyBatch` 的真实 PostgreSQL 混合成功/失败测试。
- 2026-06-09：已新增 `NEXUSIM_OUTBOX_PUBLISH_BATCH_ENABLED` 开关和 `run-local-pgpool-gradient.ps1 -PublishBatchEnabled` 参数，支持同一 HEAD 做 PublishBatch on/off 对照；正式矩阵已重建当前 HEAD 二进制后重跑，避免旧 `bin` 污染实验。
- 2026-06-09：已新增 `run-local-outbox-batch-worker-matrix.ps1`，并跑 batch size 100/500/1000 与 workers 8/12/16 在 1200/1600 VU 下的联合矩阵。结论：当前所有组合均可追平 outbox，workers 8 比 12/16 更稳；下一步重复验证 `100/8` 与 `500/8`。
- 2026-06-09：已按评审 P2 补 outbox relay active/idle 指标和 `outbox_fetched_per_call`，并用 clean smoke 验证 summary 能读取这些字段。下一轮候选重复矩阵和 adaptive limit 设计应优先使用 active 指标，而不是混合 `outbox_process_ready_latency_ms`。
- 2026-06-09：已用 active/idle metrics 重复验证 outbox 候选 `100/8` 与 `500/8`。结论：两者均可追平，`100/8` 的 success p99 更稳，作为下一轮 adaptive limit 本地 relay 基线。
- 2026-06-09：已实现默认关闭的 app 层 adaptive admission controller，输入覆盖 PG pool、repository pool acquire p95、outbox pending、relay active process ready、outbox fetched per call、Kafka records per call；clean smoke `d2af748` 证明 `SERVICE_OVERLOADED` 能在依赖读取和写事务之前返回，且不写 outbox。下一步跑 adaptive on/off 或阈值梯度正式矩阵。
- 2026-06-09：已跑 adaptive limit on/off 对照矩阵，并在过程中修复两类误判：idle relay 的 `outbox_fetched_per_call=0` 不能触发拒绝；全量累计 `repository_pool_acquire_p95` 不能作为独立硬门槛。最终 relaxed app adaptive 与 repository backpressure 行为接近，outbox 均可追平，但未证明容量提升。下一步先做窗口化 metrics、hysteresis 和动态 retry hint，再跑下一轮 adaptive 阈值矩阵。
- 2026-06-09：已为 debug metrics collector 增加最近 4096 个样本的 recent 视图，并让 adaptive controller 优先读取 recent 字段；clean smoke `6d4910b` 证明 loadtest summary 能读取 recent 指标。下一步实现 hysteresis 和动态 retry hint。
- 2026-06-09：已为 adaptive admission 增加 hysteresis 配置，支持 `ReleaseAvailableConns` 和 `ReleaseOutboxPending`；clean smoke `6f9a438` 证明低压真实链路正常。下一步把固定 `RetryInfo=500ms` 改成动态 retry hint，并用 recent+hysteresis 跑正式阈值矩阵。
- 2026-06-09：已为 `SERVICE_OVERLOADED` 增加可携带 retry delay 的错误类型，adaptive controller 会根据过载原因数量生成动态 retry hint；clean smoke `31408e7` 验证真实进程链路可运行。随后在 `c9e6cf1` 为 loadtest summary 增加 retry delay histogram，并用真实进程 smoke 验证 `retry_delay_p95_ms=500`。下一步跑正式 adaptive 阈值矩阵。
- 2026-06-09：独立评审线程复核 adaptive admission / recent metrics / hysteresis / dynamic RetryInfo / retry delay histogram，结论为无 P0/P1，不阻塞继续做 adaptive threshold matrix。P2 已记录：relay 条件依赖 outbox pending 采样、recent 是样本窗口不是时间窗口、`retry_delay_count` 是收到并计划遵守的 hint 数量、dynamic retry hint 仍是启发式控制信号。
- 2026-06-09：已在 clean commit `9830f34` 跑 adaptive threshold matrix v1，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-threshold-v1.md`。结论：`RetryBase=1000ms` 的 `gap8-outbox50-base1000` 当前最好，outbox 全部追平；pool release gap 和 outbox release ratio 未显示稳定收益。下一步做 60s 重复验证和 pool acquire p95 阈值矩阵。
- 2026-06-09：已在 clean commit `fb872a9` 对 `gap8-outbox50-base1000` 做 60s 重复验证，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-best-repeat.md`。结论：该配置能保持 outbox 清零，但 accepted RPS 过低，不能作为稳定配置。随后已给 loadtest summary 补 logical end-to-end latency；下一步做 `AdaptiveMaxPoolAcquireP95=250/500/750ms` 阈值矩阵。
- 2026-06-09：已在 clean commit `91cbb8c` 跑 logical latency smoke，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-logical-latency-smoke.md`。结论：attempt p99 与 logical p99 差异明显，后续 adaptive 矩阵必须用 logical p99 判断用户层等待。
- 2026-06-09：已在 clean commit `b98e3c9` 跑 `AdaptiveMaxPoolAcquireP95=250/500/750ms` 矩阵，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-poolp95-v1.md`。结论：outbox 全部清零，但 accepted RPS 偏低、logical p99 仍为 5s 以上；下一步应设计 admission token / concurrency limit，而不是继续只调 p95 阈值。
- 2026-06-09：已实现 app 入口 `NEXUSIM_ADAPTIVE_MAX_IN_FLIGHT` token / concurrency gate，并在 clean commit `7bf37fe` 跑 30s cap 梯度和 60s 候选重复验证；新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-inflight-v1.md`。结论：`MaxInFlight=64` 当前最稳，accepted RPS 回到约 `1.92k` 且 outbox 可清零，但 logical p99 仍约 `2.2s`；下一步调 retry delay / max retries，而不是继续放宽 token 上限。
- 2026-06-09：根据用户反馈，message-service 不再继续做大规模压测矩阵；已新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-message-service-consolidated.md`，整合 27 份原始压测报告、瓶颈排查过程和面试可讲要点。下一步转向第二个真实微服务，优先 `conversation-service`。
- 2026-06-09：已按“每个微服务一个压测报告文件夹”的规则整理 `message-service` 压测报告；所有小报告和总报告均归档到 `docs/runbook/loadtest/message-service/`，目录入口为 `docs/runbook/loadtest/message-service/README.md`。
- 2026-06-09：已将 `docs/runbook/loadtest/message-service/README.md` 从简单索引扩展为 `message-service` 压测结论和面试材料入口，包含真实链路范围、核心压测数字、瓶颈排查路径、outbox/admission 结论和面试讲法。
- 2026-06-09：已开始 `conversation-service` 最小真实 read path：新增 SDD、`conversation_service.proto`、conversation PostgreSQL migration、六层 DDD 骨架、`GetSendContext` app/domain/postgres/api 实现，并为 `message-service` 增加可选 `NEXUSIM_CONVERSATION_SERVICE_ADDR` gRPC client，用于替换 strict conversation mock。
- 2026-06-09：已完成 `conversation-service` 第一轮真实进程 smoke：启动 `conversation-service` 和 `message-service`，由 `message-service` 通过 gRPC 调用真实 `GetSendContext`，`loadtest/sendmessage --vus=2 --duration=3s` 结果为 725/725 成功、p95 10.36ms、p99 13.26ms；本轮未启动 outbox relay，测试结束后已清理 `tenant-conv-smoke` 数据，报告归档到 `docs/runbook/loadtest/conversation-service/`。
- 2026-06-09：已补 `conversation-service` 错误路径测试，覆盖 domain inactive conversation/member、app command validation 和 repository error propagation、gRPC 稳定错误 message、真实 PostgreSQL missing/archived/left/member missing 场景；已新增 `docs/runbook/conversation-service-local.md` 作为本地启动与 smoke 说明。
- 2026-06-09：独立评审线程复核 `conversation-service` 最小 read path，结论为无 P0，但指出 P1：参数缺失会被映射为 Internal。本轮已新增 `ErrInvalidArgument`，`Validate()` 返回稳定 sentinel，gRPC 映射为 `InvalidArgument`，并补 tenant/conversation/user 缺失单测；同时补 `message-service` ConversationClient 响应 tenant/conversation/enum/current_seq_shard 契约校验，以及 conversation dependency unavailable 的一次短重试。
- 2026-06-09：已冻结 `docs/sdd/conversation-service-member-change-saga.md`，明确成员变更 Saga 采用目标架构方案 C：成员边界事件与 message event 共享 `conversation_seq`、`conversation_timeline_events` 和 outbox 流；编码前必须补 `CreateMemberChange` proto、member boundary Kafka oneof payload、saga retry/DLQ migration 字段和本地 smoke runbook。
- 2026-06-09：已落地成员变更第一批公共契约：`conversation_service.proto` 新增 `CreateMemberChange` / `GetMemberChange`、`conversation.timeline.events.proto` 新增 member boundary oneof payload、`000002_member_change_saga_v2.sql` 新增 saga retry/DLQ/metadata/`timeline_event_id`/`outbox_event_id` 字段；outbox relay builder 已支持 `conversation.member.*` 且覆盖 unsupported fail-closed；下一步补 shared append port 和最小 `CreateMemberChange` 写路径。
- 2026-06-09：已实现 `conversation-service` 最小 `CreateMemberChange` 写路径，包含 app/domain/types、gRPC adapter、cmd wiring、PostgreSQL repository 同事务写 saga/member/version/timeline/outbox，以及 v3 saga event id 唯一约束；`go test ./...`、关键二进制 build、真实 PostgreSQL `CreateMemberChange` 集成测试均通过。下一步做真实进程 smoke 和阶段评审。
- 2026-06-09：独立评审线程复核最小 `CreateMemberChange` 写路径，结论无 P0；P1 指出工作区未跟踪 smoke runner 和成员权限矩阵过宽。本轮已提交 `loadtest/memberchange` runner，收紧权限矩阵：`LEAVE` 只允许本人，`OWNER` 不操作另一个 `OWNER`，`ADMIN` 只添加/移除普通 `MEMBER`，`ROLE_CHANGED` 第一版仅 `OWNER` 可在 `ADMIN/MEMBER` 间调整；`MERGE/COMPENSATE` 暂不接受。
- 2026-06-09：已完成 clean HEAD `71c04c9` 的 `CreateMemberChange(JOIN) -> outbox relay -> Kafka member event` 真实进程 smoke：279/279 成功，p99 24.95ms，saga/timeline/outbox 各 279 条，outbox `PUBLISHED=279`、`PENDING=0`、`DLQ=0`；报告归档到 `docs/runbook/loadtest/conversation-service/loadtest-report-20260609-member-change-smoke.md`。
- 2026-06-09：已实现 `conversation-service` 的 `GetMemberChange` 查询接口和 saga publish progress worker：`NEXUSIM_CONVERSATION_SERVICE_MODE=member-change-worker` 会观察 outbox `PUBLISHED` 事件并把 `member_change_saga` 推进到 `DONE`；`go test ./...`、关键二进制 build 和真实 PostgreSQL repository 集成测试均通过。下一步做真实进程 smoke 和阶段评审。
- 2026-06-09：已完成 clean HEAD `ca0a0b6` 的 conversation-service member change full smoke：`CreateMemberChange -> outbox relay -> Kafka member event -> member-change-worker -> GetMemberChange(DONE)`，350/350 成功，p99 40.90ms，`outbox_published_count=350`、`outbox_pending_count=0`、`saga_done_count=350`，样本 `GetMemberChange` 返回 `MEMBER_CHANGE_STATUS_DONE`；报告归档到 `docs/runbook/loadtest/conversation-service/loadtest-report-20260609-member-change-full-smoke.md`。下一步邀请阶段评审。
- 2026-06-09：独立评审指出 `GetMemberChange` 读取授权和 `last_error` 脱敏两个 P1；本轮已修复：repository 校验操作者/目标用户/当前 ACTIVE OWNER 或 ADMIN，未授权返回 `ErrPermissionDenied`；raw `last_error` 映射为稳定 public message；`MarkPublishedMemberChanges` 额外校验 outbox conversation、producer 和 member event type。当前 HEAD `76fff53` 短 full smoke 217/217 成功，`saga_done_count=217`，`sample_get_status=MEMBER_CHANGE_STATUS_DONE`。
- 2026-06-09：独立评审复核 `conversation-service` P1/P2 修复，结论为无 P0/P1，可进入 `delivery-service / push-gateway` SDD 和最小链路；当前已新增 `docs/sdd/delivery-service.md` v0.1 Draft，范围限定为消费 `conversation.timeline.events`、投影 `user_inbox`、提供 `PullInbox / AckDelivery`，不先实现 WebSocket。
- 2026-06-09：独立评审指出 `delivery-service` SDD v0.1 有 3 个 P1：缺成员可见性投影、ACK 可推进到未来 seq、Kafka checkpoint 维度错误。本轮已修 SDD 和 migration：新增 `delivery_membership_projection`，ACK 必须 `received_seq <= user_inbox max_visible_seq`，Kafka checkpoint 改为 `consumer_group + topic + partition`；同步落地 delivery proto / migration / 六层骨架、`PullInbox / AckDelivery` 最小同步路径和 `ProjectTimelineEventUseCase` / PostgreSQL projection 方法。
- 2026-06-09：独立评审复核 delivery baseline 后指出 3 个 P1：首次并发 ACK 可能冲突、membership 单行投影 rejoin 窗口不稳、缺真实 PostgreSQL repository 测试。本轮已修：ACK 前加 transaction advisory lock，JOIN 事件会重置 `join_seq` 并清 `leave_seq`；新增真实 PostgreSQL 集成测试覆盖 join/left/rejoin、message projection、checkpoint、ACK 越界、低 seq 幂等、首次并发 ACK 和 timeline replay 去重。
- 2026-06-09：独立评审复核 delivery P1 修复和 timeline consumer，结论为无 P0/P1，可进入真实小规模 smoke。本轮已新增 `loadtest/delivery` runner，并在 clean commit `ef817f7` 跑通 delivery full smoke：`CreateMemberChange(JOIN user-0 / delivery-user-1) -> message-service outbox relay -> Kafka -> delivery timeline consumer -> SendMessage(user-0) -> Kafka -> user_inbox(delivery-user-1) -> PullInbox -> AckDelivery`。结果：SendMessage `64/64` 成功，`delivery-user-1` 拉到 64 条 inbox，ACK 到 seq `66`，`message_outbox PUBLISHED=66`，`delivery_outbox PENDING=129` 为预期，因为 delivery outbox relay 尚未实现；报告见 `docs/runbook/loadtest/delivery-service/loadtest-report-20260609-delivery-full-smoke.md`。
- 2026-06-09：已实现 delivery-service `delivery_outbox -> Kafka im.delivery.events` 最小发布链路：新增独立 Kafka schema `schemas/kafka/delivery/v1/im.delivery.events.proto`，补 `NEXUSIM_DELIVERY_SERVICE_MODE=outbox-relay`、PostgreSQL outbox store、trigger relay、Kafka writer producer；单元测试覆盖 event builder、malformed fail-closed 和 batch publish error，真实 PostgreSQL 集成测试覆盖 publish/retry/DLQ/低版本阻塞。下一步跑真实进程 smoke 并归档报告。
- 2026-06-09：已完成 delivery-service outbox relay 真实 Kafka smoke：创建 `im.delivery.events` topic，启动 `NEXUSIM_DELIVERY_SERVICE_MODE=outbox-relay`，把 1 条 `delivery.ack.recorded.v1` 从 `PENDING` 发布为 `PUBLISHED`，并从 Kafka partition 1 offset 0 解码出 `DeliveryEvent_AckRecorded`；报告见 `docs/runbook/loadtest/delivery-service/loadtest-report-20260609-delivery-outbox-smoke.md`。
- 2026-06-09：已完成 delivery-service `LEAVE / REMOVE` 负向可见性 smoke：新增 `loadtest/deliveryvisibility` runner；clean commit `a87fc3f` 使用临时 topic `conversation.timeline.visibility.20260609-152208` 跑通两个场景。结论：边界后的 message event 已被 active sender 消费，但离开/移除用户没有任何 `conversation_seq > boundary_seq` 的 `user_inbox`，`PullInbox(after_seq=boundary_seq)` 也返回 0；报告见 `docs/runbook/loadtest/delivery-service/loadtest-report-20260609-delivery-visibility-negative-smoke.md`。
- 2026-06-09：已新增 `docs/sdd/push-gateway.md` v0.1 Draft 和 `docs/runbook/loadtest/push-gateway/README.md`。push-gateway 第一阶段定位为 WebSocket 在线连接、`im.delivery.events` 唤醒、客户端回源 `PullInbox` 和 ACK frame 转发，不拥有 `user_inbox` / cursor，不直接读内部表；下一步应做阶段评审，评审通过后再进入 WebSocket frame 契约和六层骨架。
- 2026-06-09：独立评审复核 `push-gateway` SDD，结论为服务边界正确、无 P0；指出 WebSocket frame 契约缺少 ACK 成功响应和 heartbeat 响应两个 P1。本轮已补 `delivery.ack.ok`、`server.pong`，并明确慢连接队列、resume token、多设备通知和 ACK 成功 / 失败语义；下一步进入 push-gateway 六层骨架和最小在线通知实现。
- 2026-06-09：已实现 `push-gateway` 第一版六层骨架和最小在线通知代码：`cmd/push-gateway` 支持 `noop/ws/delivery-consumer/all`，`all` 模式用于本地 smoke 共享 in-memory session registry；WebSocket adapter 支持 `server.hello`、`server.pong`、`delivery.notify`、`delivery.ack.ok` 和稳定 error frame；delivery event consumer 只消费 `im.delivery.events`，`delivery.inbox_item.created.v1` 转在线 notify，`delivery.ack.recorded.v1` 不广播，unsupported/malformed fail-closed；ACK 通过 delivery-service gRPC `AckDelivery`，不直接写 cursor。`go test ./services/push-gateway/...` 和 `go test ./...` 已通过；下一步跑真实进程 full smoke。
- 2026-06-09：sub-agent 只读复核 `push-gateway` 实现后指出 3 个 P1，本轮已修：WebSocket 改为 `client.hello -> server.hello` 握手并回显 request_id；慢连接队列满时驱逐该 session 并允许 consumer commit，避免单个慢连接卡死 `im.delivery.events` partition；运行入口强制 delivery consumer topic 为 `im.delivery.events`。同时补 envelope/event_type 校验、`retryable=false` 显式 JSON 字段和对应测试。
- 2026-06-09：独立评审复核 `push-gateway` 骨架后指出 P1：delivery-service `PermissionDenied` 被错误映射为 `SERVER_BUSY retryable=true`。本轮已修为 `PERMISSION_DENIED retryable=false` 并补 WebSocket 回归测试；随后新增 `loadtest/pushgateway` 和 `run-local-smoke.ps1`，在 clean commit `984080d` 跑通 full smoke：`server.hello -> delivery.notify -> PullInbox(1 item) -> delivery.ack.ok`，cursor 推到 seq `2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`；报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260609-push-gateway-full-smoke.md`。
- 2026-06-09：独立评审复核 `push-gateway` full smoke，结论无 P0/P1，可以确认第四个真实微服务完成单实例最小在线通知闭环；本轮已清理 `current-goal.md` 中过期的未实现状态，并把 `PERMISSION_DENIED retryable=false` 补入 `docs/sdd/push-gateway.md` 错误码表。后续继续做同 user 多 device notify smoke、slow session active close / resume_hint 和 Redis route。
- 2026-06-09：已扩展 `loadtest/pushgateway`，支持 `--receiver-device-ids` 多设备连接；clean commit `99efdc3` 跑通同 user 双 device notify smoke：`push-device-1` 和 `push-device-2` 都收到同一条 `delivery.notify`，并分别返回 `delivery.ack.ok`、cursor 推进到 seq `2`；`delivery_outbox PUBLISHED=3/PENDING=0/DLQ=0`。报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260609-push-gateway-multidevice-smoke.md`。
- 2026-06-09：已补 push-gateway slow session active close 第一版：session queue full 时 registry 发出 eviction signal，WebSocket writer 尽量发送 broad `server.resume_hint` 后主动以 policy violation close 连接，客户端随后应按本地 durable cursor 重连并 `PullInbox`；同时修复多设备 smoke ACK request_id 可读性，并补 `parseDeviceIDs` 单测。独立评审指出的 `notify_seq - 1` 提示语义和普通断连 `close(outbound)` 竞态已修复。下一步补 Redis route / resume buffer，或增加 slow-client 真实进程负向 smoke 与指标。
- 2026-06-09：已补 push-gateway 单实例 in-memory resume buffer：`client.hello.resume_token + last_received` 会绑定到同 tenant/user/device，并按本地 cursor 过滤重放最近 `delivery.notify`；buffer 只保存轻量通知，不保存完整消息事实。当时仍无 TTL / Redis route / 跨实例 resume，可靠恢复仍以 delivery-service durable inbox 和客户端本地 cursor 为准。
- 2026-06-09：独立评审指出单实例 resume buffer 有 P1：未知客户端 `resume_token` 会被注册成有效 token。本轮已修复为“未知 token -> `buffer_miss` + 服务端签发新 token”，并补 registry / WebSocket 回归测试；同时在 push-gateway WebSocket HTTP server 暴露 `/debug/metrics`，返回单实例 registry 调试指标，供后续 slow-client smoke 排障使用。
- 2026-06-09：已扩展 `loadtest/pushgateway` 和 `run-local-smoke.ps1` 支持 `--scenario slow-client`，并在 clean commit `b362dd7` 跑通 slow-client 真实进程负向 smoke：128 条 SendMessage 触发 `session_queue_full_count=1`、`slow_session_evicted_count=1`，客户端通过 `PullInbox` 拉到 128 条、max seq `129`，随后 `delivery.ack.ok` 推进 cursor 到 `129`，`delivery_outbox PUBLISHED=129/PENDING=0/DLQ=0`；报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260609-push-gateway-slow-client-smoke.md`。
- 2026-06-09：独立评审复核 push-gateway slow-client smoke，结论无 P0/P1，不阻塞继续 Redis route / cross-instance route。P2 已记录：`NEXUSIM_PUSH_TEST_WRITE_DELAY` 仍是生产二进制可见的测试 knob，生产必须 unset/0；本轮 slow-client smoke 证明的是 durable `PullInbox` fallback，不证明 resume buffer replay，后续需单独设计 resume replay smoke。
- 2026-06-09：已实现 push-gateway Redis route 最小 adapter：`NEXUSIM_PUSH_ROUTE_BACKEND=redis` 时 connect 写 `session_id -> gateway_id` route 和 `tenant/user -> session set`，disconnect best-effort 清理，delivery notify 会按 Redis route 发布到远端 gateway Pub/Sub channel，远端再通过本机 registry fanout；同一远端 gateway 多 session 只 publish 一次。当前仅有 miniredis 单元测试，下一步需跑真实双实例 route smoke，并补 TTL 续期 / cleanup。
- 2026-06-09：已跑 clean commit `903f205` 的 push-gateway Redis route 真实进程 smoke：`push-gateway-ws` 持有 WebSocket session，`push-gateway-consumer` 消费 `im.delivery.events`，通知通过 Redis route / PubSub 转发到 WebSocket gateway；客户端收到 seq `2` 的 `delivery.notify`，`PullInbox item_count=1/max_seq=2`，`delivery.ack.ok last_received_seq=2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260609-push-gateway-redis-route-smoke.md`。
- 2026-06-09：已为 push-gateway Redis route 增加 per-session TTL 续期：connect 写 route 后由 gateway 进程按 TTL 比例周期性刷新 session route / user set，disconnect 时取消续期并 best-effort 删除 route；miniredis 测试已覆盖 route 在 TTL 前被续期、unregister 后被删除。当时可把能力表述为“最小分布式在线路由模拟”，但 Redis 故障语义、cleanup ticker、跨实例 resume 还没有完成；后续这些已按日志继续推进。
- 2026-06-09：已在 clean commit `a7b1f7e` 重跑 push-gateway Redis route TTL smoke：`push-gateway-ws` 和 `push-gateway-consumer` 分离后仍通过 Redis route / PubSub 收到 seq `2` 的 `delivery.notify`，`PullInbox item_count=1/max_seq=2`，`delivery.ack.ok last_received_seq=2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`；报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260609-push-gateway-redis-route-ttl-smoke.md`。
- 2026-06-09：根据“把整个系统重构为分布式”的目标，已新增 `docs/runbook/distributed-local.md` 和 `tools/local-distributed-smoke.ps1`，把 `conversation-service / message-service / delivery-service / push-gateway` 的本地多进程分布式拓扑、启动命令、证据链和面试讲法串成统一入口；当前仍限定为本地分布式 smoke，不表述为生产级多实例。
- 2026-06-09：已按 sub-agent 复核建议收敛 push-gateway Redis 故障语义：connect 写 route 失败保持 fail-closed 并回滚本地 session，避免假在线；delivery notify 的 Redis lookup / publish 失败改为 fail-open，不阻塞 Kafka commit，依赖 durable `PullInbox` 兜底；新增单元测试覆盖 Redis unavailable、stale route cleanup 和 register rollback。
- 2026-06-09：已在 clean commit `90e3354` 通过 `tools/local-distributed-smoke.ps1 -SkipBuild` 跑通系统级本地分布式 smoke：四个真实服务 / 网关按独立进程协作，push gateway ws / consumer 分离后通过 Redis route 收到 seq `2` 的 `delivery.notify`，`PullInbox item_count=1/max_seq=2`，`delivery.ack.ok last_received_seq=2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`；原始结果在 `H:\NexusIM\loadtest-results\nexusim-distributed-smoke-20260609-192218\pushgateway-summary.json`。
- 2026-06-09：已为 push-gateway Redis route 增加后台 stale route cleanup loop，`NEXUSIM_PUSH_ROUTE_CLEANUP_INTERVAL` 默认 `30s`，可设置为 `0` 关闭；cleanup 会扫描 `route:user:*` set，移除 session key 缺失、route JSON 损坏或 tenant/user 不匹配的成员，降低异常退出和手工坏数据导致的跨实例路由陈旧状态。
- 2026-06-09：已在 clean commit `29b8cc6` 重跑系统级本地分布式 smoke，验证新增 Redis cleanup loop 不影响正常跨 gateway route：WebSocket gateway 与 delivery consumer gateway 分离后收到 seq `2` 的 `delivery.notify`，`PullInbox item_count=1/max_seq=2`，`delivery.ack.ok last_received_seq=2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`；原始结果在 `H:\NexusIM\loadtest-results\nexusim-distributed-smoke-redis-cleanup-20260609-193331\pushgateway-summary.json`。
- 2026-06-09：已新增 `push-gateway` Redis fault smoke runner，`redisroute` subscriber 支持 Pub/Sub 连接错误后重连并跳过 malformed payload；clean commit `074902b` 真实 stop/start smoke 通过：WebSocket session 注册后停止 `nexusim-redis`，SendMessage 仍投影到 `user_inbox`，客户端通过 `PullInbox item_count=1/max_seq=2` 恢复，Redis 恢复后重连并 `delivery.ack.ok last_received_seq=2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`；原始结果在 `H:\NexusIM\loadtest-results\push-gateway-redis-fault-smoke-20260609-195200\pushgateway-summary.json`，报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260609-push-gateway-redis-fault-smoke.md`。
- 2026-06-09：已恢复 Windows -> Mac 免密 SSH：`172.31.50.2:22` 与 `192.168.0.182:22` 均可达，Windows 当前公钥已加入 Mac `authorized_keys`；Mac Docker CLI 已配置到用户级 PATH，`docker --version` 返回 Docker `29.5.3`，Docker Desktop 当前资源池约 8 CPU / 8GB。两端 Git 代理统一为 `http://127.0.0.1:7890`。Mac `/Users/qsyy0921/Desktop/IM` 存在但有本地 ahead / untracked 文件，后续不要强行 reset；优先 `fetch`，不能 fast-forward 时新建干净 smoke checkout。
- 2026-06-09：已将 Windows `main` 推送到 GitHub，`origin/main` 更新到 `a839e61`，用于后续 Mac fetch / 双机分布式 smoke。
- 2026-06-09：为节省外网流量，已改为通过有线 `172.31.50.2` 同步 Mac：新增 `tools/sync-mac-distributed-smoke.ps1`，Windows 生成 Git bundle 并 scp 到 Mac，再从 Windows 交叉编译 `darwin/arm64` 的 `push-gateway` / `pushgateway-smoke` 传入 Mac 专用 checkout；Mac 专用 checkout 已验证 `NEXUSIM_PUSH_GATEWAY_MODE=noop` 可启动。专用 checkout 已从桌面根目录迁移到 `/Users/qsyy0921/Desktop/IM/_local/distributed-smoke`，Git bundle 归档到 `/Users/qsyy0921/Desktop/IM/_local/artifacts/bundles`，避免散落在 Mac 桌面。
- 2026-06-09：已用 `tools/sync-mac-distributed-smoke.ps1 -SkipBuild` 通过有线重新同步 Mac 专用 checkout 到 Windows HEAD `d514390`，Mac `/Users/qsyy0921/Desktop/IM/_local/distributed-smoke` 显示 `## main`，并再次验证 `bin/darwin-arm64/push-gateway` 的 `noop` 模式可启动。
- 2026-06-09：已新增并执行 `tools/check-mac-docker-desktop.ps1`，通过有线 SSH 复查 Mac Docker Desktop：Docker `29.5.3`，context `desktop-linux`，`Cpus=8`，`MemoryMiB=8192`，`SwapMiB=1024`，Docker proxy `http/https=http://127.0.0.1:7890`，proxy exclude 包含 `172.16.0.0/12`，结果 `mac_docker_desktop_config=OK`。后续 Mac 两节点模拟使用容器级 `--cpus 4 --memory 4g`，不再调大 Docker Desktop 全局池。
- 2026-06-09：已新增并执行 `tools/sync-mac-service-docker-images.ps1`，通过本地交叉编译 + `scratch` Dockerfile 在 Windows / Mac 两端构建四个业务服务镜像：`nexusim/conversation-service:local`、`nexusim/message-service:local`、`nexusim/delivery-service:local`、`nexusim/push-gateway:local`；Mac 镜像为 `linux/arm64`，Windows 镜像为 `linux/amd64`，业务镜像不依赖外网拉取基础层。
- 2026-06-09：已新增 `tools/run-win-mac-push-smoke.ps1` 并跑通 Win-Mac Docker route smoke：Windows 运行基础设施、核心业务服务和 `push-gateway delivery-consumer`，Mac Docker 运行 `nexusim/push-gateway:local` WebSocket gateway；Windows runner 通过 `ws://172.31.50.2:11598` 收到 seq `2` 的 `delivery.notify`，随后 `PullInbox item_count=1/max_seq=2`，Mac gateway 通过 `172.31.50.1:11597` 回调 Windows delivery-service 得到 `delivery.ack.ok last_received_seq=2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`；原始结果在 `H:\NexusIM\loadtest-results\push-gateway-win-mac-redis-smoke-20260609-205219\pushgateway-summary.json`。该 run 是 dirty 工作区脚本验证，不作为 clean 性能基线。
- 2026-06-09：已在 clean commit `8c322fc` 重跑 Win-Mac Docker route smoke：Windows 运行 PostgreSQL / Kafka / Redis / 核心业务进程 / `push-gateway delivery-consumer`，Windows Docker 运行 `nexusim/delivery-service:local` gRPC，Mac Docker 运行 `nexusim/push-gateway:local` WebSocket gateway；Windows runner 通过 `ws://172.31.50.2:11598` 收到 seq `2` 的 `delivery.notify`，随后 `PullInbox item_count=1/max_seq=2`，Mac gateway 通过 `172.31.50.1:11597` 回调 Windows Docker delivery-service 得到 `delivery.ack.ok last_received_seq=2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`；原始结果在 `H:\NexusIM\loadtest-results\push-gateway-win-mac-redis-smoke-20260609-210034\pushgateway-summary.json`，`git_dirty=false`。
- 2026-06-09：已为 push-gateway 单实例 in-memory resume buffer 增加 TTL：`NEXUSIM_PUSH_RESUME_BUFFER_TTL` 默认 `10m`，registry 会在注册、通知入队和 `/debug/metrics` 读取时清理过期 token；过期 token 视为 `buffer_miss` 并签发新 token，不会重放旧通知；`/debug/metrics` 已新增 resume token count / expired count。当时仍未做跨实例 resume buffer，可靠恢复继续依赖 durable `PullInbox`。
- 2026-06-09：已新增 `loadtest/pushgateway --scenario resume-replay`，并在 clean commit `80033de` 跑通 push-gateway 单实例 resume replay smoke：客户端第一次收到 seq `2` 的 `delivery.notify` 后在 ACK 前断开，随后携带同一 `resume_token` 和 `last_received=1` 重连，收到同一 `event_id/message_id/seq` 的 replay；`resume_buffer_replay_count=1`、`resume_buffer_miss_count=0`，随后 `PullInbox item_count=1`、`delivery.ack.ok last_received_seq=2`、`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`；报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260609-push-gateway-resume-replay-smoke.md`。
- 2026-06-09：已为 push-gateway Redis route 补第一版跨实例调试指标：consumer gateway 可记录远端 route 命中 session 数、Pub/Sub publish call、publish error、估算远端 enqueued session、lookup error、stale cleanup；WebSocket gateway subscriber 可记录收到的远端通知、malformed payload、入本地 registry 的 enqueue 数和 subscriber error。`/debug/metrics` 现在返回 `redis_registry_metrics` 和 `redis_subscriber_metrics`，用于解释跨实例 online wakeup 路径；它仍不是 durable delivery 成功率，可靠事实继续看 `PullInbox / AckDelivery / delivery_outbox`。
- 2026-06-09：已为 push-gateway Redis route 增加 Redis-backed cross-instance resume buffer 第一版：route entry 记录 `resume_token`，Redis 保存 token 绑定的 tenant/user/device meta 和轻量 `delivery.notify` frame list；不同 gateway 可用同 token + `last_received` replay，未知 token 返回 `buffer_miss` 并换新 token，跨 device token 返回 `PERMISSION_DENIED`，buffer gap 返回 `buffer_miss`。miniredis 单元测试已覆盖跨 gateway replay、未知 token、跨 device、gap 和 Redis lookup 故障本地 fallback。
- 2026-06-09：已让 `push-gateway delivery-consumer` 模式通过 `NEXUSIM_PUSH_DEBUG_ADDR` 暴露只读 `/debug/metrics`，用于直接观察 consumer 侧 Redis route / Redis resume append 指标，不再只能靠重连 gateway replay 间接判断。
- 2026-06-09：已在 clean commit `b8d33da` 重跑 push-gateway cross-instance resume 真实进程 smoke：客户端首次连接 `push-gateway-ws` 收到 seq `2` 的 `delivery.notify` 后在 ACK 前断开，随后携带同一 `resume_token` 和 `last_received=1` 重连到 `push-gateway-ws-reconnect`，命中 Redis-backed resume buffer 并 replay 同一 `event_id/message_id/conversation_seq` 的通知；consumer gateway 直接记录 `redis_resume_append_count=1`、`redis_route_remote_publish_call_count=1`、`redis_route_remote_publish_error_count=0`，重连 gateway 记录 `redis_resume_replay_count=1`、`redis_resume_miss_count=0`；随后 `PullInbox item_count=1/max_seq=2`、`delivery.ack.ok last_received_seq=2`、`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。原始结果在 `H:\NexusIM\loadtest-results\push-gateway-cross-instance-resume-smoke-20260609-consumer-metrics\pushgateway-summary.json`，报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260609-push-gateway-cross-instance-resume-smoke.md`；它仍不是生产级可靠投递，可靠事实继续由 delivery-service durable inbox / ACK cursor 保证。
- 2026-06-09：已在 clean commit `b8d8f92` 跑通 Win-Mac Docker cross-instance resume smoke：客户端首次连接 Mac Docker `push-gateway` WebSocket gateway `ws://172.31.50.2:11598`，收到 seq `2` 的 `delivery.notify` 后在 ACK 前断开；随后携带同一 `resume_token` 和 `last_received=1` 重连到 Windows `push-gateway` reconnect gateway `ws://127.0.0.1:11599`，命中 Redis-backed resume buffer 并 replay 同一 `event_id/message_id/conversation_seq` 的通知；Windows consumer gateway 记录 `redis_resume_append_count=1`、`redis_route_remote_publish_call_count=1`、`redis_route_remote_publish_error_count=0`，Windows reconnect gateway 记录 `redis_resume_replay_count=1`、`redis_resume_miss_count=0`；随后 `PullInbox item_count=1/max_seq=2`、`delivery.ack.ok last_received_seq=2`、`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。原始结果在 `H:\NexusIM\loadtest-results\push-gateway-win-mac-cross-instance-resume-20260609\pushgateway-summary.json`，报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260609-push-gateway-win-mac-cross-instance-resume-smoke.md`；它证明双机 route/resume 链路可运行，但仍不是 Redis HA 或生产容量结论。
- 2026-06-09：已为 push-gateway Redis route 增加 Sentinel client 配置支持：`NEXUSIM_PUSH_REDIS_MODE=single|sentinel`，Sentinel 模式使用 `NEXUSIM_PUSH_REDIS_SENTINEL_MASTER_NAME` 和 `NEXUSIM_PUSH_REDIS_SENTINEL_ADDRS` 发现 master；本地 Redis/Sentinel compose 已显式关闭 Sentinel protected mode，并通过 `NEXUSIM_REDIS_SENTINEL_ANNOUNCE_IP` 参数化 announce / monitor 地址；`tools/local-up-redis-sentinel.ps1` 会验证 Sentinel 返回的 master 可从宿主机 TCP 连接、可从 Sentinel 容器内 `PING`。clean commit `7bc35a5` 已跑通三 Redis / 三 Sentinel discovery 正常路径 route / resume smoke：Sentinel 返回 master `172.31.50.1:6380`，consumer gateway `redis_resume_append_count=1`，reconnect gateway `redis_resume_replay_count=1 / redis_resume_miss_count=0`，`PullInbox item_count=1/max_seq=2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。这仍不等同于 Redis HA 验收，下一步需补 master failover smoke。
- 2026-06-09：已新增 `redis-sentinel-failover` pushgateway smoke 场景，并在 clean commit `819c14a` 跑通手动 Sentinel failover recovery：WebSocket route 注册后执行 `SENTINEL failover mymaster`，默认脚本等待 master 从 `172.31.50.1:6380` 切到 `172.31.50.1:6381`，并验证新 master 可 `PING` 且 `ROLE=master` 后再继续 `SendMessage`；随后客户端收到 seq `2` 的 `delivery.notify`，断开后重连另一个 gateway 命中 Redis-backed resume replay，同一 `event_id/message_id/conversation_seq` 匹配，`PullInbox item_count=1/max_seq=2`、`delivery.ack.ok last_received_seq=2`、`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260609-push-gateway-redis-sentinel-failover-smoke.md`；这仍不是完整 Redis HA，未覆盖 quorum 异常、网络分区、自动停 master 触发和容量结论。
- 2026-06-09：已新增 `redis-sentinel-master-stop` pushgateway smoke 场景，并在 clean commit `8ddc2fb` 跑通自动 master-stop recovery：脚本先查询 Sentinel 当前 master，再停止对应容器 `nexusim-redis-ha-replica-1`，等待 Sentinel master 从 `172.31.50.1:6381` 切到 `172.31.50.1:6380`，验证新 master 可 `PING` 且 `ROLE=master` 后继续完整链路；随后客户端收到 seq `2` 的 `delivery.notify`，断开后重连另一个 gateway 命中 Redis-backed resume replay，同一 `event_id/message_id/conversation_seq` 匹配，`PullInbox item_count=1/max_seq=2`、`delivery.ack.ok last_received_seq=2`、`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。恢复脚本已重启被停容器并确认 healthy，报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260609-push-gateway-redis-sentinel-master-stop-smoke.md`；这仍不是完整 Redis HA，未覆盖 quorum 异常、网络分区、Redis Cluster、切主窗口零丢失和容量结论。
- 2026-06-10：已为 push-gateway 增加第一版低耦合真实鉴权入口并跑通 clean HMAC auth full smoke：`NEXUSIM_PUSH_AUTH_MODE=hmac` 时 WebSocket 建连必须提供短期 signed gateway token，服务端校验 HMAC 签名、`aud=push-gateway`、`exp` 和 device 绑定；runner 使用 `Authorization: Bearer` 传 token，summary 记录 `push_auth_query_identity_sent=false`，随后完整通过 `server.hello -> delivery.notify -> PullInbox -> delivery.ack.ok`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。原始结果在 `H:\NexusIM\loadtest-results\push-gateway-hmac-auth-smoke-clean-20260610-052736\pushgateway-summary.json`，报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260610-push-gateway-hmac-auth-smoke.md`。该切片不新增 identity-service、不跨服务读取用户/设备表；当前已支持 `NEXUSIM_PUSH_AUTH_HMAC_PREVIOUS_SECRETS` 作为最小密钥轮换验证窗口，`loadtest/pushgateway` 也已支持用 `--push-auth-token-signing-secret` 模拟 old token 兼容 smoke，后续仍需 device revoke、session revoke、refresh token、多 issuer 和 JWK/JWT 标准化。
- 2026-06-10：已按低耦合 / 控制复杂度原则先冻结 conversation-service owner transfer SDD，而非直接把 owner 转移塞进现有 `ROLE_CHANGED`。设计结论：使用专用 `TransferConversationOwner` 和 `conversation.member.owner_transferred.v1`；第一版只允许当前 OWNER 转给 ACTIVE ADMIN/MEMBER，旧 owner 降级为 ADMIN；一个 transfer 只分配一个 `conversation_seq`，同事务更新两条 `conversation_members`、推进一次 conversation version、写一条 timeline/outbox；实现必须分 proto/schema/migration/relay/projection 与 repository/RPC/smoke 多阶段推进。
- 2026-06-10：已完成 owner transfer 第一段代码切片：新增 `TransferConversationOwner` proto、`conversation.member.owner_transferred.v1` Kafka oneof payload、conversation migration 约束、message-service relay builder fail-closed 支持，以及 delivery-service projection 对旧 owner / 新 owner 两行 membership projection 的同事务更新；`go test ./...` 通过。当前仍未实现 conversation-service repository / RPC / smoke，下一步继续保持小切片推进。
- 2026-06-10：已完成 owner transfer 第二段代码切片：新增独立 `TransferConversationOwnerCommand`、薄 use case、gRPC handler 和 PostgreSQL repository 事务；同一事务内校验当前 ACTIVE OWNER 与目标 ACTIVE ADMIN/MEMBER、分配一个 `conversation_seq`、更新两条 `conversation_members`、只递增一次 conversation member/permission version、写一条 `member_change_saga` / timeline / outbox，并补 progress worker 对 `conversation.member.owner_transferred.v1` 的 DONE 推进；conversation-service 单元测试、全包测试和真实 PG 集成测试通过。下一步仍是 owner transfer 真实进程 smoke，不把 smoke/report 混进该代码提交。
- 2026-06-10：已完成 owner transfer 真实进程 smoke：新增 `loadtest/memberchange --change-type owner-transfer` 和 `run-local-smoke.ps1`，在 clean commit `490db1a` 上启动 conversation-service gRPC、message-service outbox relay 和 conversation-service member-change-worker；1 次 `TransferConversationOwner` 成功，旧 owner `owner-1` 降级为 ACTIVE ADMIN，新 owner `owner-transfer-user` 成为唯一 ACTIVE OWNER，`conversation_seq_current=1`，`saga_done_count=1`，outbox `PUBLISHED=1/PENDING=0/DLQ=0`。原始结果在 `H:\NexusIM\loadtest-results\conversation-owner-transfer-smoke-20260610-072627\memberchange-summary.json`，报告见 `docs/runbook/loadtest/conversation-service/loadtest-report-20260610-owner-transfer-smoke.md`。
