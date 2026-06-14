# NexusIM 面试版项目进度

本文用于面试时介绍项目进度，重点说明：

- 已经开发了哪些后端能力；
- 当前系统能证明什么；
- 还差哪些生产化和产品化能力；
- 后续如何从后端主链路推进到分布式可靠性和 AI 应用后端。

它不是每轮 Codex 工作入口；每轮工作仍先看 `docs/runbook/current-brief.md`。

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
现在先继续做 9 服务 hardening；
api-gateway 已补 first-stage tenant-scoped rate limit、静态 tenant plan override 和 tenant plan 文件热更新；
legacy descriptor 已收敛为显式 opt-in 默认；
api-gateway 已补 first-stage OpenTelemetry gRPC server span；
search-service 和 AI 应用后端后置；
客户端暂不纳入当前面试主线。
```

## 已完成的后端服务

当前已有 9 个真实后端微服务：

| 服务 | 已完成能力 | 面试可讲重点 |
| --- | --- | --- |
| `api-gateway` | 统一 user-facing gRPC 入口，gateway token 验证，verified metadata 注入，下游代理，token / tenant scope rate limit，静态 tenant plan override，tenant plan 文件热更新，legacy descriptor 显式 opt-in 默认，first-stage OTel gRPC server span，debug metrics | 统一入口、安全边界、correlation / trace 传播、facade-only 默认暴露面 |
| `identity-service` | 注册、登录、Refresh Token、MFA TOTP、recovery codes、JWKS、session/device revoke、verification/password reset challenge、webhook / SMTP email challenge sender | 身份认证、MFA、token 轮换、JWKS、公私钥边界、通知投递可靠性 |
| `message-service` | `SendMessage`、编辑、撤回、删除，message log，outbox，Kafka timeline event | 业务事务不直接 publish Kafka，使用 outbox 保证事件传播 |
| `conversation-service` | 会话成员事实源，`GetSendContext`，成员变更 saga，owner transfer | 会话成员事实边界、成员事件和消息事件共享 timeline seq |
| `delivery-service` | timeline projection，durable `user_inbox`，`PullInbox`，`AckDelivery`，delivery outbox | 断线可恢复，push-gateway 不拥有 durable inbox |
| `push-gateway` | WebSocket 在线通知，ACK 转发，resume buffer，Redis route，跨实例在线路由 | 在线唤醒层和可靠投递层解耦，Redis 故障时 PullInbox 兜底 |
| `receipt-service` | 已读 / 未读，会话列表，archive / pin / mute，receipt projection，receipt outbox | 会话列表和回执从投递事件投影，不跨服务读内部表 |
| `contacts-service` | 好友申请、接受、拒绝、取消、删除、拉黑、备注，contacts outbox | 联系人事实源，策略服务通过事件投影使用联系人关系 |
| `policy-service` | 权限决策、规则存储、conversation role gate、contacts projection、decision audit outbox | 策略权限独立服务化，不在 message-service 复制权限逻辑 |

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
- PostgreSQL `repmgr + pgpool` local failover smoke；
- Kafka KRaft 3 broker local leader failover smoke。

这些验证证明：

- 在线通知可以跨实例工作；
- 在线通知失败时，durable `PullInbox + AckDelivery` 能兜底；
- Redis、Kafka、PostgreSQL 单点切换后，最小链路可以恢复；
- 多个 worker / relay 已具备退避重试和 fail-closed 行为；
- outbox / projection / challenge delivery 具备第一阶段 audit / repair / cleanup。

## 已完成的安全与运维基础

当前已经落地：

- 各核心服务的 `/healthz`、`/readyz`、`/debug/metrics`；
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

| 服务 | 待开发 / 待完善功能 |
| --- | --- |
| `api-gateway` | OTel collector / alerting / 跨服务 rollout、legacy opt-in 使用面迁移审计、配置中心 / DB-backed quota hardening、生产部署治理 |
| `identity-service` | WebAuthn / passkeys、OIDC federation、多 issuer、KMS / HSM key management、完整登录风控、SMS provider、bounce handling、多租户通知模板 |
| `message-service` | 更多消息类型、私有删除、合规删除、容量压测、生产级发送链路观测 |
| `conversation-service` | 更完整群管理、owner transfer 策略细化、成员可见窗口历史 repair |
| `delivery-service` | Projection DLQ / repair 深化、更多 delivery event 消费方、投递容量压测 |
| `push-gateway` | Redis 网络分区 smoke、跨实例 resume 强化、在线连接容量测试、慢连接组合故障验证 |
| `receipt-service` | 送达回执扩展、批量接口优化、会话列表产品化 |
| `contacts-service` | 联系人分组、联系人搜索、更多隐私策略 |
| `policy-service` | 完整 ReBAC、moderation policy、tenant DSL / quota、外部 audit sink |

### 中期：完整 IM 产品后端

等 9 个服务稳定后，再补产品级后端服务。服务数量不写死，只有满足独立数据模型、独立伸缩需求、独立故障边界或能明显降低现有服务复杂度时才拆。

| 待开发服务 / 能力 | 目标 |
| --- | --- |
| `search-service` | 聊天记录搜索、索引、权限过滤、撤回 / 删除 tombstone |
| `media-service` | 图片、语音、视频、文件上传下载、对象存储、缩略图、病毒扫描 |
| `notification-service` | 邮件、短信、APNs / FCM、系统通知、模板、bounce handling |
| `audit-service` | 登录审计、安全审计、管理操作审计、策略决策归档 |
| `admin-service` | 租户管理、封禁、配置、运维操作、repair 工作台 |
| `tenant/config-service` | 多租户配置、功能开关、限流策略、灰度配置；是否独立成服务后续用 ADR 决定 |
| `presence-service` | 在线状态、输入中、最后在线时间；当前 push-gateway session registry 还不是完整 presence 服务 |

### 中期：生产级分布式平台能力

当前已经做了本地 / 双机 smoke，但还没完整证明生产级 HA。后续待开发 / 待验证：

- 真实 Redis 网络分区；
- 生产级 Redis HA / Redis Cluster；
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

1. 清已有 9 个服务的 P2 hardening；
2. 继续做 `api-gateway` OTel collector / 跨服务 rollout、legacy opt-in 使用面迁移审计和配置中心 / DB-backed quota hardening；
3. 补更完整的故障恢复 smoke；
4. 收敛观测、repair、audit、TLS / mTLS 和 trusted metadata 边界；
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
