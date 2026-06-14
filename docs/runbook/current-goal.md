# NexusIM Current Goal

## 短 Goal Prompt

可复制到 Codex 目标框：

```text
持续推进 E:\development\IM 的 NexusIM 项目。每轮先读取仓库根目录 prompt.md，并按其中的当前主线、下一步优先级、工作原则执行。不全文读取长历史文档，不回滚用户已有修改。
```

维护真实 prompt 内容时只改仓库根目录 `prompt.md`，不要把完整 prompt 复制回本文件。

## 当前主线

当前面试主线是后端、分布式可靠性和 AI 应用后端。先把 9 个已有后端服务收干净，再进入 `search-service`，然后在搜索和权限边界之上推进 `rag-service` / `summary-service` / `agent-service`。Web / App / 桌面端属于后续产品化展示层，暂不作为当前主线。

当前优先级：
1. 先治理已有 9 个微服务，新增服务后置；api-gateway 已补 first-stage tenant-scoped rate limit、静态 tenant plan override、tenant plan 文件热更新、legacy descriptor 显式 opt-in 默认和 first-stage OpenTelemetry gRPC server span，下一步继续 OTel collector / alerting / 跨服务 rollout、legacy opt-in 使用面迁移审计和配置中心 / DB-backed quota hardening。`search-service` 只保留 SDD draft，不进入 proto / migration / skeleton。
2. 当前已开始治理代码复杂度：identity PostgreSQL repository、repository test、`loadtest/pushgateway/main.go` 与目标架构长文档已完成拆分；identity-service 已补 `/healthz`、`/readyz`、`/debug/metrics`、JWKS debug 入口、只读 `session-mfa-proof-audit`、只读 `challenge-delivery-repair-audit` 和按 retention / scope 清理 repair audit 历史的 `challenge-delivery-repair-cleanup` operator，以及低敏 identity / MFA / challenge-delivery 聚合观测，`outbox-relay` 和 `challenge-delivery-worker` 对非取消运行时错误已改为退避重试，并分别在 relay / worker 模式暴露 low-sensitive retry 快照；当 `NEXUSIM_IDENTITY_ADMIN_AUTH_MODE=metadata|verified-metadata` 时，公网监听地址 + 无 gRPC mTLS client cert 的危险组合会在启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连；malformed payload / unsupported event 仍保持 fail-closed，decrypt failure / incomplete message / notifier error 仍保持 store 驱动的 retry / expire / DLQ 语义。message-service 已补 `/healthz`、`/readyz`、`/debug/metrics`、`outbox-audit`、`outbox-repair`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，`outbox-relay` 对非取消运行时错误已改为退避重试，并在 relay 模式暴露 retry 快照；当 `NEXUSIM_MESSAGE_AUTH_MODE=metadata|verified-metadata` 时，公网监听地址 + 无 gRPC mTLS client cert 的危险组合会在启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连；malformed event / payload 仍保持 fail-closed。contacts-service 已补 `/healthz`、`/readyz`、`/debug/metrics`、`outbox-audit`、`outbox-repair`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，`outbox-relay` 对非取消运行时错误已改为退避重试，并在 relay 模式暴露 low-sensitive outbox relay retry 快照；当 `NEXUSIM_CONTACTS_AUTH_MODE=metadata|verified-metadata` 时，公网监听地址 + 无 gRPC mTLS client cert 的危险组合会在启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连；policy-service 已补 `/healthz`、`/readyz`、`/debug/metrics`、`outbox-audit`、`outbox-repair`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，以及低敏 gRPC / decision / rule-store / projection / audit-outbox 聚合观测，`contact-consumer`、`timeline-consumer` 和 `outbox-relay` 对非取消运行时错误已改为退避重试，并分别在 worker / relay 模式暴露 retry 快照；当 `NEXUSIM_POLICY_GRPC_ADDR` 是公网地址时，若未启用入口 gRPC TLS，进程也会在启动前直接失败，避免把内部 policy decision API 暴露到 plaintext 公网入口；push-gateway 已补 `/healthz`、`/readyz`、`/debug/metrics` 统一入口和低敏 session / resume / Redis route / auth JWK 聚合观测，并区分 Redis publish error 与 0-subscriber stale-route 场景，`delivery-consumer`、`identity-consumer` 和 Redis route subscriber 仅对运行时错误做退避重试；当 `NEXUSIM_PUSH_AUTH_MODE=mock` 时，公网 WebSocket 监听地址会在启动前直接失败，避免把本地 smoke 身份模式暴露到公网；当 `NEXUSIM_PUSH_AUTH_MODE=hmac|jwt` 时，公网 WebSocket 监听地址若未启用入口 TLS / WSS 也会在启动前直接失败，避免把签名 token 暴露到 plaintext 公网入口；`invalid frame` / `unsupported event` 和 malformed subscriber payload 仍保持 fail-closed 或仅记聚合计数，并分别在 worker / subscriber 模式暴露 retry 快照；Redis route 续约连续失败达到阈值后会主动踢掉本地 session，避免 route TTL 失效后仍长期假装在线；receipt-service 已补 `/healthz`、`/readyz`、`/debug/metrics`、只读 `outbox-audit`、`outbox-repair`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，以及 low-sensitive receipt projection worker / outbox relay retry 快照；当 `NEXUSIM_RECEIPT_AUTH_MODE=metadata|verified-metadata` 时，公网监听地址 + 无 gRPC mTLS client cert 的危险组合会在启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。delivery-service 已补 `/debug/metrics`、只读 `outbox-audit`、`outbox-repair`、只读 `outbox-repair-audit`、按 retention/scope 清理 outbox repair audit 历史、`projection-checkpoint-repair`、只读 `projection-checkpoint-repair-audit`、按 retention/class/scope 清理 repair audit 历史、projection failure audit、resolved 标记、按 unresolved failure 定点 replay、按最早 unresolved failure 自动 rewind、只读 `projection-failure-audit` 及其按 offset/event/class 的定点过滤、以及按 class/scope 过滤的 resolved failure cleanup operator，`timeline-consumer` 与 `outbox-relay` 对非取消运行时错误已改为退避重试，并分别在 worker / relay 模式暴露 retry 快照；当 `NEXUSIM_DELIVERY_AUTH_MODE=metadata|verified-metadata` 时，公网监听地址 + 无 gRPC mTLS client cert 的危险组合会在启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连；malformed event、projection failure、failure recorder 异常和 malformed payload / unsupported event 仍保持持久审计或 outbox fail-closed；conversation-service 已补 `/healthz`、`/readyz`、`/debug/metrics` 和低敏 gRPC / PostgreSQL / member-change 聚合观测，`member-change-worker` 对非取消错误已改为退避重试，并在 worker 模式暴露 retry 快照；当 `NEXUSIM_CONVERSATION_AUTH_MODE=metadata|verified-metadata` 时，公网监听地址 + 无 gRPC mTLS client cert 的危险组合会在启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连；api-gateway 已补 gateway auth 与 trusted metadata 双启动门禁：当 `NEXUSIM_API_GATEWAY_AUTH_MODE=mock` 时，公网 gRPC 监听地址会在启动前直接失败，避免把本地 smoke 身份模式暴露到公网；当 `NEXUSIM_API_GATEWAY_AUTH_MODE=hmac|jwt` 时，公网 gRPC 监听地址若未启用入口 TLS 也会在启动前直接失败，避免把签名 token 暴露到 plaintext 公网入口；当下游服务启用 `metadata` / `verified-metadata` auth 时，公网地址 + 无 mTLS client cert 的危险组合也会在启动前直接失败；当前已覆盖 conversation / message / delivery / receipt / contacts / identity 下游，私网 / loopback 仍保留第一阶段 trusted metadata 直连，后续继续清理更完整的 projection repair、故障语义和测试缺口。
3. 保持 api-gateway、identity、message、conversation、delivery、push、receipt、contacts、policy 已有链路稳定。
4. 当前已补本地 PostgreSQL `repmgr + pgpool` failover smoke、本地 Kafka KRaft 三 broker failover smoke，以及本地 Redis Sentinel quorum-loss fallback smoke；完整 Redis 网络分区仍属于生产化后续项，不阻塞当前功能推进。

演进原则：当前 9 个服务够支撑 IM 后端主链路；后续服务和中间件都不写死。只有当能力有独立数据模型、独立伸缩需求、独立故障边界，或会明显降低复杂度时才新增服务；替换中间件必须说明兼容、迁移、回滚和压测证据，并通过 ADR。

## 分层路线

第一层：最小 IM 主链路
发消息、会话、投递、在线通知、ACK。已完成最小闭环。

第二层：分布式与可靠性
outbox、Kafka、durable inbox、Redis route、多实例、Win/Mac Docker smoke、基础观测，以及本地 PostgreSQL `repmgr + pgpool` failover smoke 和本地 Kafka KRaft 三 broker failover smoke。已有小规模 smoke 证据，不是生产 HA。

第三层：完整 IM 后端产品能力
已读/送达回执、撤回/编辑/删除、会话列表、未读数、联系人、群管理、真实鉴权、api-gateway。当前主线是把这些已有后端服务收干净，不急着补客户端。

第四层：搜索与智能化后端
先做 `search-service`，再做 RAG、Agent、聊天记录搜索、智能总结、群聊问答、客服机器人、推荐、风控辅助。AI 只能通过权限过滤后的检索层访问消息事实。

## 文档路由

- 当前入口：`docs/runbook/current-brief.md`
- 文档总入口：`docs/README.md`
- runbook 路由：`docs/runbook/README.md`
- 服务状态索引：`docs/runbook/service-briefs/README.md`
- 单服务短状态：`docs/runbook/service-briefs/<service>.md`
- 历史长目标：`docs/runbook/archive/current-goal-20260614-long.md`
- 历史长 brief：`docs/runbook/archive/current-brief-20260614-long.md`
- 服务设计：`docs/sdd/<service>.md`
- 压测 / smoke 证据：`docs/runbook/loadtest/<service>/`

默认不要读 archive 全文。只在查历史证据时按关键词读取相关段落。

## 硬边界

- 项目名统一为 `NexusIM`。
- 每个微服务独立六层 DDD：`api / app / domain / infrastructure / types / trigger`。
- Kafka 事件只能通过 outbox relay 发布，业务事务不能直接 publish Kafka。
- push-gateway 只做在线唤醒和 ACK 转发；可靠事实在 delivery-service。
- 服务间同步调用只用于当前请求必须依赖的权限 / 上下文；状态传播优先走 Kafka 事实事件和本服务 projection。
- 新能力优先复用已有事实流、outbox、projection、read model 和端口。
