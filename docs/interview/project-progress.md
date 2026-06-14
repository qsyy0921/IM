# NexusIM 面试版项目进度

本文用于面试时介绍项目进度，重点说明：

- 已经开发了哪些后端能力；
- 当前系统能证明什么；
- 还差哪些生产化和产品化能力；
- 后续为什么先做后端，再做客户端和 AI 扩展。

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

## 已完成的后端服务

当前已有 9 个真实后端微服务：

| 服务 | 已完成能力 | 面试可讲重点 |
| --- | --- | --- |
| `api-gateway` | 统一 user-facing gRPC 入口，gateway token 验证，verified metadata 注入，下游代理，rate limit，debug metrics | 统一入口、安全边界、correlation 传播、逐步收敛 legacy descriptor |
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

## 还没有完成的后端能力

### 生产级分布式平台能力

仍未完整证明：

- 真实 Redis 网络分区；
- 生产级 Redis HA / Redis Cluster；
- PostgreSQL split-brain / quorum / 跨机存储故障；
- Kafka multi-failure / controller failover / ISR 抖动；
- 完整服务发现；
- 统一 OpenTelemetry trace / alert / dashboard；
- 灰度发布和部署编排。

### 完整 IM 产品后端

后续还需要：

- `search-service`：聊天记录搜索、索引、权限过滤；
- `media-service`：图片、语音、视频、文件、对象存储、缩略图；
- `notification-service`：邮件、短信、APNs / FCM、系统通知；
- `audit-service`：安全审计、管理操作审计、策略决策归档；
- `admin-service`：租户管理、封禁、配置、运维操作；
- 更完整的群管理、联系人分组、隐私策略、会话列表产品化。

### 大模型应用后端

后续计划：

- `rag-service`：基于聊天记录的权限安全问答；
- `summary-service`：会话总结、未读摘要、日报；
- `agent-service`：客服机器人、群助手、任务 Agent；
- retrieval gateway / evidence pack / source message id / seq；
- Agent 写操作必须走 proposal / approval / executor / audit。

## 客户端状态

当前重点仍是后端。客户端后续再做：

1. Web demo：用于展示登录、聊天、搜索、RAG 问答；
2. 移动 App：验证弱网、通知、前后台和本地缓存；
3. 桌面端：复用 Web 能力做产品化延展。

面试时应明确：

```text
前端不是当前项目的主战场。
当前核心价值是后端微服务、事件驱动、分布式可靠性和大模型应用后端边界。
```

## 当前开发阶段

当前阶段是：

```text
继续把已有 9 个核心服务做干净。
```

短期优先级：

1. 清已有 9 个服务的 P2 hardening；
2. 补更完整的故障恢复 smoke；
3. 收敛观测、repair、audit、TLS / mTLS 和 trusted metadata 边界；
4. 控制代码复杂度，避免核心文件继续变大；
5. 等 9 个服务稳定后，再进入 `search-service`。

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
