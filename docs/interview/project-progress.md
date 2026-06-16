# NexusIM 面试版项目进度

本文用于面试时介绍项目进度，重点说明：

- 已经开发了哪些后端能力；
- 当前系统能证明什么；
- 还差哪些生产化和产品化能力；
- 后续如何从后端主链路推进到分布式可靠性和 AI 应用后端。

它不是每轮 Codex 工作入口，也不是工程待办来源；每轮工作仍先看 `docs/runbook/current-brief.md`，当前未完成工作以 `docs/runbook/remaining-goals.md` 为准。

## 项目定位

NexusIM 是一个以 Go 微服务为主的分布式 IM 后端项目。当前目标不是做一个简单聊天 demo，而是逐步实现：

```text
身份认证
-> 会话和成员
-> 消息写入
-> outbox / Kafka timeline
-> durable inbox
-> WebSocket 在线通知
-> ACK / 回执 / 联系人 / 策略权限
-> 后续搜索、RAG、Agent
```

当前可以准确表述为：

```text
本地 / 双机可运行的最小分布式 IM 后端。
```

不能过度表述为：

```text
生产级完整分布式 IM 平台。
```

## 开发过程主线

面试时建议按阶段讲，而不是按提交流水账讲：

```text
第一阶段：先做 message-service，验证 SendMessage + outbox + Kafka 的最小写入链路。
第二阶段：补 conversation-service，把发送上下文、成员事实和成员事件边界拆出来。
第三阶段：补 delivery-service 和 push-gateway，把 durable inbox、PullInbox、AckDelivery、在线通知和跨实例 route 串起来。
第四阶段：补 receipt-service、contacts-service、policy-service 和 api-gateway，把已读/未读、联系人、权限决策和统一入口补齐。
第五阶段：集中治理分布式可靠性、安全启动门禁、trusted metadata / TLS 边界、repair / audit / cleanup、debug metrics 和代码复杂度。
第六阶段：收干净现有 9 个服务后，再进入 search-service，并在搜索和权限边界上继续做 RAG / summary / agent 后端。
```

当前项目处在第五阶段到第六阶段之间：

```text
9 个后端服务已经能跑通主链路；
现在先继续做 9 服务 hardening，不急着新增 search / RAG 服务；
api-gateway 已补 first-stage tenant-scoped rate limit、静态 tenant plan override、tenant plan 文件热更新、版本化 quota URL source、URL bearer token / HTTPS guard、URL source CA / client cert TLS 边界、可选 checksum-required gate、applied quota snapshot stale 观测和 quota snapshot gate；
api-gateway 已补 legacy/facade traffic metrics，用于旧 descriptor 迁移观察；
legacy descriptor 已收敛为显式 opt-in 默认；
当前 9 个服务已补 first-stage Prometheus text /metrics、本地 Prometheus alert rules 和本地 Grafana dashboard 原型；
api-gateway 已补 first-stage OpenTelemetry 入口 server span 和下游 gRPC client span；当前 9 个服务均已纳入 first-stage trace runtime wiring，其中 8 个后端 gRPC 服务使用 server span，push-gateway 使用 WebSocket connection span，并由采样策略和本地 check-local 门禁约束；
本地 OTel collector debug 入口和 policy OTLP smoke 脚本已补，可用于面试演示 OTLP trace 链路，但还不是生产告警平台；
search-service 和 AI 应用后端后置；
客户端暂不纳入当前面试主线。
```

## 已完成的后端服务

当前已有 9 个真实后端微服务：

| 服务 | 已完成能力 | 面试可讲重点 |
| --- | --- | --- |
| `api-gateway` | 统一 user-facing gRPC 入口，gateway token 验证，verified metadata 注入，下游代理，token / tenant scope rate limit，静态 tenant plan override，tenant plan 文件热更新，版本化 quota URL source，URL bearer token / HTTPS guard，URL source CA / client cert TLS 边界，可选 checksum-required gate，applied quota snapshot stale 观测，quota snapshot gate，legacy descriptor 显式 opt-in 默认，legacy/facade traffic metrics，legacy quiet-window gate 和 observation 归档脚本，first-stage OTel 入口 server span 和下游 gRPC client span，debug metrics | 统一入口、安全边界、correlation / trace 传播、facade-only 默认暴露面 |
| `identity-service` | 注册、登录、Refresh Token、MFA TOTP、recovery codes、JWKS、session/device revoke、verification/password reset challenge、webhook / SMTP email challenge sender，first-stage OTel gRPC server span | 身份认证、MFA、token 轮换、JWKS、公私钥边界、通知投递可靠性，身份服务已进入 trace rollout |
| `message-service` | `SendMessage`、编辑、撤回、删除，合规删除 external proof manifest verifier，`TEXT` + `IMAGE` / `FILE` / `VOICE` 附件引用消息，`LOCATION` / `CARD` 结构化 payload 消息，message log，outbox，Kafka timeline event，first-stage OTel gRPC server span | 业务事务不直接 publish Kafka，使用 outbox 保证事件传播；合规 proof 只登记低敏 ref/provider/hash，不保存正文；核心写服务已进入 trace rollout；图片 / 文件 / 语音二进制处理后续交给 media 能力 |
| `conversation-service` | 会话成员事实源，`GetSendContext`，成员变更 saga，owner transfer，成员窗口 audit / repair / repair audit（含当前窗口 `join_seq` / `leave_seq` / 版本 floor 保守修复），first-stage Prometheus text `/metrics`、本地 alert rules / Grafana dashboard、first-stage OTel gRPC server span | 会话成员事实边界、成员事件和消息事件共享 timeline seq，成员事实服务已进入观测 rollout |
| `delivery-service` | timeline projection，durable `user_inbox`，`PullInbox`，`AckDelivery`，delivery outbox，projection failure audit / checkpoint rewind / failure resolve / cleanup operator，first-stage Prometheus text `/metrics`、本地 alert rules / Grafana dashboard、first-stage OTel gRPC server span | 断线可恢复，push-gateway 不拥有 durable inbox，投递服务已进入观测和 projection repair rollout |
| `push-gateway` | WebSocket 在线通知，ACK 转发，resume buffer，Redis route，跨实例在线路由，Redis resume negative fallback，Redis Cluster topology、node-stop fallback、六节点 failover smoke 和六节点短容量基线，first-stage Prometheus text `/metrics`、本地 Prometheus scrape / alert rules 原型、本地 Grafana dashboard 原型、first-stage OTel WebSocket connection span | 在线唤醒层和可靠投递层解耦，Redis / resume / Cluster node 故障时 PullInbox 兜底，在线层已进入观测 rollout |
| `receipt-service` | 已读 / 未读，会话列表，archive / pin / mute / tags / 多标签 all-match 过滤 / draft，unread-first 会话排序，receipt projection，receipt outbox，`ListReceiptStates` repository 级批量查询，低敏 `received_device_count` 聚合和 opt-in capped device details，first-stage Prometheus text `/metrics`、本地 Prometheus scrape / alert rules 原型、本地 Grafana dashboard 原型、first-stage OTel gRPC server span | 会话列表和回执从投递事件投影，不跨服务读内部表，设备明细默认隐藏、显式开启且限量返回，回执服务已进入观测 rollout |
| `contacts-service` | 好友申请、申请来源 metadata、租户级来源策略、来源风险标注和 `REVIEW_REQUIRED` operator 审批状态机、接受、拒绝、取消、删除、拉黑、解除拉黑、备注、分组、联系人搜索、用户 / 租户 / 系统三级申请隐私、first-stage ALLOW-DENY 隐私例外写入 / 查询 / 清理、搜索来源申请 gate、profile visibility 总开关和字段级白名单、租户默认隐私 operator、contacts outbox，first-stage Prometheus text `/metrics`、本地 Prometheus scrape / alert rules 原型、本地 Grafana dashboard 原型、first-stage OTel gRPC server span | 联系人事实源，策略服务通过事件投影使用联系人关系；隐私、来源策略、审批状态和拉黑只影响本服务关系事实，消息权限通过 policy projection 表达 |
| `policy-service` | 权限决策、规则存储、用户级消息动作限制、first-stage keyword / HTTP content moderation、first-stage tenant action quota、conversation role gate、contacts projection、decision audit outbox、低敏 decision audit export、first-stage Prometheus text `/metrics`、本地 Prometheus scrape / alert rules 原型、本地 Grafana dashboard 原型、first-stage OTel gRPC server span | 策略权限独立服务化，不在 message-service 复制权限逻辑；内容分类通过 policy provider port 接入，keyword / HTTP adapter 都不持久化正文；决策审计和 tenant quota 先以本地低敏 operator 形式闭环，provider-grade 外部 audit sink、tenant DSL 和 risk scoring 后续深化 |

## 已完成的主链路

当前主链路已经覆盖：

- 注册 / 登录 / Refresh Token / MFA；
- 会话成员创建和发送上下文查询；
- 普通消息发送、编辑、撤回、删除；
- PostgreSQL 事务事实源；
- outbox + Kafka event 传播；
- durable inbox 投递模型；
- `PullInbox` 和 `AckDelivery`；
- WebSocket 在线通知；
- 已读 / 未读 / 会话列表基础能力；
- 联系人关系和拉黑策略；
- policy-service 权限决策。

可以用这句话概括：

```text
消息从客户端入口进入后，可以经过身份、权限、会话、消息、投递、在线通知、ACK 和回执链路闭环。
```

## 已完成的分布式与可靠性能力

当前已经做过的关键验证：

- 本地多进程 smoke；
- Win / Mac 双机 Docker smoke；
- Redis route / Redis-backed resume；
- Redis stop / start fallback；
- Redis Sentinel discovery / failover / master-stop / quorum-loss fallback；
- Redis Cluster 本地三节点 topology smoke；
- Redis Cluster node-stop fallback smoke；
- Redis Cluster 六节点自动 failover smoke；
- Redis Cluster 六节点短容量基线；
- PostgreSQL `repmgr + pgpool` local failover smoke；
- Kafka KRaft 3 broker local leader failover / controller-switch / ISR observation smoke；
- Kafka KRaft repeated ISR flapping smoke：2 轮 broker stop/start 均观察到 ISR 收缩 / 恢复和 `acks=all` probe 写入成功；
- Kafka producer hardening evaluation：6 个 producer package 固定 `acks=all`、禁自动建 topic、bounded retry/backoff，并明确当前 `kafka-go` 不声明 idempotent / transactional producer 语义，业务可靠性边界仍是 outbox / event_id 幂等。
- Kafka producer fault observation：本地 `kafka-go` producer 在 broker stop/restore 窗口内写入 120 条 records，消费侧观察到 unique 120、missing ack 0、duplicate 0；这只是本地观察，不是 exactly-once 证明。
- Kafka consumer group rebalance smoke：两个 push-gateway delivery-consumer 进入同一 group，停止一个后，`im.delivery.events` 的 3 个 partition 被重新分配给剩余 consumer。
- Kafka consumer churn smoke：2 个 push-gateway delivery-consumer 在同一 group 中反复 leave / rejoin，2 轮 8 个 transition 均回到 Stable，且 3 个 partition 都已分配。
- Kafka consumer churn probe smoke：8 个 transition 后共写入 24 条合法 `delivery.inbox_item.created.v1` probe，producer ack 24，consumer group 每次 post-probe lag 回到 0。

这些验证证明：

- 在线通知可以跨实例工作；
- 在线通知失败时，durable `PullInbox + AckDelivery` 能兜底；
- Redis、Kafka、PostgreSQL 单点切换后，最小链路可以恢复；
- Kafka 在本地 RF=3 / min.insync.replicas=2 下，一 broker down 仍可写，两 broker down 会按 `NOT_ENOUGH_REPLICAS` fail-closed；
- Kafka repeated ISR flapping 下，本地 broker stop/start 后 ISR 能在 2 / 3 之间按预期收缩和恢复；
- Kafka producer fault observation 下，本地已 ack records 可以在消费侧全部找到，同时继续保留 outbox / event_id 幂等作为业务可靠性边界；
- Kafka consumer group 可以完成第一阶段本地 rebalance；
- Kafka consumer group 可以完成第一阶段 repeated leave / rejoin churn；
- Kafka consumer group 在本地 churn 后仍可继续消费合法 delivery event 并提交到 zero lag；
- 多个 worker / relay 已具备退避重试和 fail-closed 行为；
- outbox / projection / challenge delivery 具备第一阶段 audit / repair / cleanup。

## 已完成的安全与运维基础

当前已经落地：

- 各核心服务的 `/healthz`、`/readyz`、`/debug/metrics`，以及当前 9 个服务第一阶段 Prometheus text `/metrics`、本地 scrape / alert rules 原型、本地 Grafana dashboard 原型和 first-stage trace sampling policy / check；
- gRPC / WebSocket 公网监听下的弱鉴权 / 明文入口启动门禁；
- trusted metadata 和 mTLS 边界的第一阶段收口；
- gateway token、JWKS、RS256 key overlap；
- identity MFA / recovery code / refresh step-up；
- challenge delivery outbox、retry、DLQ、repair audit；
- outbox / projection repair 和 cleanup operator；
- 低敏 debug metrics，不暴露 token、secret、TOTP、recovery code、用户敏感标识。

## 待开发功能清单

这里按面试表达分层。当前没有已知 P0 / P1 阻塞；下面主要是还没完成的产品能力、生产化能力和大模型应用能力。

### 短期：继续把 9 个核心服务做干净

短期不急着新开服务，先把已有 9 个服务收口：

统一收口顺序：

1. 安全启动门禁和 trusted metadata / TLS 边界。
2. 本地观测、故障恢复 smoke、repair / DLQ / audit。
3. 各服务 P2 hardening 和容量验证。
4. 代码复杂度治理。
5. 完成这些后再进入 `search-service`。

| 服务 | 待开发 / 待完善功能 |
| --- | --- |
| `api-gateway` | 在目标环境持续运行 legacy quiet-window observation 并形成最终删除计划、采样治理 hardening、配置中心 / DB-backed quota hardening、生产部署治理；当前已有第一阶段 Prometheus text `/metrics`、本地 alert rules、本地 Grafana dashboard、9 服务 first-stage trace runtime wiring 和 trace sampling / wiring check，但还不是生产观测平台 |
| `identity-service` | WebAuthn / passkeys、OIDC federation、多 issuer、KMS / HSM key management、完整登录风控、SMS provider、bounce handling、多租户通知模板 |
| `message-service` | 会话级删除策略深化、provider-grade 外部 proof 工作流 / 审批系统集成、容量压测、生产级发送链路观测；图片 / 文件 / 语音二进制上传处理后续交给 media 能力 |
| `conversation-service` | 更完整群管理、owner transfer 策略细化、完整历史窗口 / targeted replay repair；当前已有第一阶段 Prometheus text `/metrics`、本地 alert rules 和本地 Grafana dashboard，但还不是生产观测平台 |
| `delivery-service` | 更多 delivery event 消费方、投递容量压测；当前已有 projection failure audit / checkpoint rewind / failure resolve / cleanup 第一阶段 operator 闭环、第一阶段 Prometheus text `/metrics`、本地 alert rules 和本地 Grafana dashboard，但还不是生产观测平台 |
| `push-gateway` | 生产级 Redis HA 设计、跨实例 resume 生产化策略、在线连接容量测试、慢连接组合故障验证、长时间容量曲线和生产 sizing；当前已有 Redis route / Sentinel / network-partition / Redis Cluster topology / Redis Cluster node-stop fallback / Redis Cluster failover / Redis Cluster 短容量基线 / Redis resume negative fallback smoke、第一阶段 Prometheus text `/metrics`、本地 alert rules 和本地 Grafana dashboard，但还不是生产观测平台 |
| `receipt-service` | 会话列表更多摘要策略等产品化能力；当前已有 tags / draft、opt-in capped device details、第一阶段 Prometheus text `/metrics`、本地 alert rules 和本地 Grafana dashboard，但还不是生产观测平台 |
| `contacts-service` | 组织级联系人策略、后续接入 admin/config service 正式权限面；当前已有 first-stage ALLOW-DENY 隐私例外写入 / 查询 / 清理、字段级 profile 可见性、`REVIEW_REQUIRED` 本地 operator 审批状态机、第一阶段 Prometheus text `/metrics`、本地 alert rules 和本地 Grafana dashboard，但还不是生产观测平台 |
| `policy-service` | 完整 ReBAC、provider-grade moderation / risk scoring、tenant DSL / quota、外部 audit sink；当前已有 first-stage keyword / HTTP content moderation、第一阶段 Prometheus text `/metrics`、本地 alert rules 和本地 Grafana dashboard，但还不是生产观测平台 |

### 中期：完整 IM 产品后端

等 9 个服务稳定后，再补产品级后端服务。服务数量不写死，只有满足独立数据模型、独立伸缩需求、独立故障边界或能明显降低现有服务复杂度时才拆。

| 待开发服务 / 能力 | 目标 |
| --- | --- |
| `search-service` | 聊天记录搜索、索引、权限过滤、撤回 / 删除 tombstone |
| `media-service` | 图片、语音、视频、文件上传下载、对象存储、缩略图、病毒扫描、语音转码 / 时长探测 |
| `notification-service` | 邮件、短信、APNs / FCM、系统通知、模板、bounce handling |
| `audit-service` | 登录审计、安全审计、管理操作审计、策略决策归档 |
| `admin-service` | 租户管理、封禁、配置、运维操作、repair 工作台 |
| `tenant/config-service` | 多租户配置、功能开关、限流策略、灰度配置；是否独立成服务后续用 ADR 决定 |
| `presence-service` | 在线状态、输入中、最后在线时间；当前 push-gateway session registry 还不是完整 presence 服务 |

### 中期：生产级分布式平台能力

当前已经做了本地 / 双机 smoke，但还没完整证明生产级 HA。后续待开发 / 待验证：

- Redis Cluster 容量验证；
- 生产级 Redis HA；
- PostgreSQL split-brain / quorum / 跨机存储故障；
- Kafka multi-failure / controller failover / ISR 抖动；
- 完整服务发现；
- 统一 OpenTelemetry trace / alert / dashboard；
- 结构化日志和统一告警；
- 灰度发布、部署编排、配置治理；
- 运维 UI / repair approval workflow。

### 后期：大模型应用后端

大模型能力必须建立在搜索、权限和审计边界之上，不能让模型直接读业务库。

| 待开发服务 / 能力 | 目标 |
| --- | --- |
| `rag-service` | 基于聊天记录的权限安全问答 |
| `summary-service` | 会话总结、未读摘要、日报 |
| `agent-service` | 客服机器人、群助手、任务 Agent |
| retrieval gateway | 统一检索入口，强制 policy check 和成员可见窗口过滤 |
| evidence pack | AI 输出必须携带 source message id、conversation seq、conversation id |
| Agent 写动作链路 | Proposal -> Approval -> Executor -> Audit，避免 Agent 直接改业务事实 |

## 当前不纳入面试主线

Web / App / 桌面端属于后续产品化展示层，不作为当前面试文档重点。

面试时只讲下面四类能力：

```text
后端微服务主链路；
分布式可靠性；
安全、观测、repair 和运维 hardening；
search / RAG / Agent 后端能力。
```

## 当前开发阶段

当前阶段是：

```text
继续把已有 9 个核心服务做干净。
```

短期优先级：

1. 先收敛安全启动门禁、trusted metadata、TLS / mTLS 边界；
2. 清已有 9 个服务的 P2 hardening；
3. 继续做 `api-gateway` OTel collector / alerting / dashboard、legacy opt-in 实际迁移观察和移除计划，以及配置中心 / DB-backed quota hardening；
4. 补更完整的故障恢复 smoke；
5. 收敛观测、repair、audit 和 DLQ 处理；
5. 控制代码复杂度，避免核心文件继续变大；
6. 等 9 个服务稳定后，再进入 `search-service`。

## 面试讲述线

可以这样介绍：

```text
我实现了一个事件驱动的分布式 IM 后端。系统用 PostgreSQL 作为交易事实源，用 outbox 保证业务事务和事件发布之间的一致性，用 Kafka 传播 timeline 和投递事件，用 delivery-service 构建 durable inbox，push-gateway 只负责在线唤醒，不承担可靠投递。这样即使 WebSocket、Redis route 或 push-gateway 出问题，客户端仍可以通过 PullInbox 和 AckDelivery 恢复状态。

在身份侧，我实现了登录、Refresh Token、MFA、recovery code、JWKS、challenge delivery outbox、SMTP / webhook challenge sender 和启动安全门禁。系统也补了 health、ready、debug metrics、repair、audit、cleanup、worker retry 和多种本地故障 smoke。

后续我会在当前 9 个服务稳定后继续做 search-service、rag-service 和 agent-service，让大模型只能通过权限过滤后的检索层访问聊天记录，并且输出必须带证据消息和审计链路。
```

## 维护规则

- 这个文档只在阶段变化时更新。
- 不记录每个提交的流水账。
- 新服务完成真实链路后，更新“已完成的后端服务”。
- 新的 smoke 证据仍写入 `docs/runbook/loadtest/<service>/`。
- 新的详细设计仍写入 `docs/sdd/`。
