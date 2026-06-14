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

当前未进入真实实现主线、仍属于后续能力：

- `search-service`
- `media-service`
- `notification-service`
- `audit-service`
- `admin-service`
- `rag / summary / agent` 等智能化扩展

当前面试主线只覆盖：

```text
后端微服务主链路
-> 分布式可靠性
-> 安全 / 观测 / repair / 运维 hardening
-> search / RAG / Agent 应用后端
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
- PostgreSQL `repmgr + pgpool` local failover smoke
- Kafka KRaft 3 broker local failover smoke

当前已经证明：

- 在线通知层可以跨实例工作
- Redis 故障时 durable `PullInbox + AckDelivery` 可以兜底
- PostgreSQL / Kafka 单点切换后最小链路仍可恢复

当前还没有证明：

- 真实 Redis 网络分区
- 生产级 Redis HA / Redis Cluster
- 生产级 PostgreSQL HA / split-brain / quorum
- 生产级 Kafka multi-failure / controller failover / ISR 抖动治理
- 完整部署编排、服务发现、统一观测、灰度发布

### 3. 安全与运维

当前已经落地的共性 hardening：

- 各核心服务已补 `/healthz`、`/readyz`、`/debug/metrics`
- 公网地址 + 弱鉴权 / 明文入口的启动门禁
- trusted metadata / mTLS 边界的第一阶段收口
- outbox / projection / challenge delivery 等 repair / audit / cleanup operator
- worker / relay 非取消错误退避重试

当前仍属于后续 hardening：

- 更完整的 trace / alert / structured logging
- 更细粒度的故障演练
- 更系统化的运维 UI / repair workflow

## 服务进度矩阵

| 服务 | 当前状态 | 已有证据 | 主要剩余工作 |
| --- | --- | --- | --- |
| `api-gateway` | 已落地、已接主链路 | gateway auth / downstream trusted metadata smoke、token / tenant scope rate limit、静态 tenant plan override、legacy descriptor 显式 opt-in 默认、first-stage OTel gRPC server span | 运行时动态 quota 配置、OTel collector / alerting / 跨服务 rollout、legacy opt-in 使用面迁移审计、生产部署治理 |
| `identity-service` | 已落地、已接登录主链路 | login / refresh / MFA / recovery code / JWKS / challenge delivery | WebAuthn/passkeys、OIDC、多 issuer、KMS/HSM、完整风控、生产级 email/SMS provider |
| `message-service` | 已落地、已接主链路 | `SendMessage` / outbox / Kafka timeline | 更多消息类型、私有删除、合规删除、容量和生产观测 |
| `conversation-service` | 已落地、已接主链路 | `GetSendContext` / member change / saga / worker | 更完整群管理、owner transfer 策略、成员窗口历史 repair |
| `delivery-service` | 已落地、已接主链路 | projection / `PullInbox` / `AckDelivery` / delivery outbox | Projection DLQ / repair 深化、更多 delivery event 消费方 |
| `push-gateway` | 已落地、已接主链路 | notify / ACK / resume / Redis route / Win-Mac / Sentinel / TLS smoke | Redis 网络分区、跨实例 resume 强化、容量测试 |
| `receipt-service` | 已落地、已接主链路 | receipt projection / outbox / audit / repair | 送达回执扩展、批量接口优化、会话列表产品化 |
| `contacts-service` | 已落地、已接主链路 | contacts grpc / outbox / audit / repair | 联系人分组、联系人搜索、更多隐私策略 |
| `policy-service` | 已落地、已接主链路 | decision / projection / outbox / audit / repair | 完整 ReBAC、moderation policy、tenant DSL / quota、外部 audit sink |
| `search-service` | 仅保留占位和 brief | 无真实实现主线 | 等前 9 个服务收干净后再进入 |

## 当前问题处理队列

当前 9 个服务没有已知 P0/P1 阻塞。后续按小切片逐个清 P2，默认顺序如下：

1. `api-gateway`：first-stage tenant-scoped rate limit、静态 tenant plan override、legacy descriptor 显式 opt-in 默认和 first-stage OTel gRPC server span 已补；下一步继续运行时动态 quota 配置、OTel collector / alerting / 跨服务 rollout 和 legacy opt-in 使用面迁移审计，不先扩新 facade。
2. `identity-service`：继续身份安全 hardening，优先真实通知 / issuer / key 管理边界。
3. `message-service`：补消息类型和删除语义前，先守住 outbox / policy /容量观测。
4. `conversation-service`：补群管理前，先收 owner transfer 和成员窗口 repair。
5. `delivery-service`：补 projection DLQ / repair 深化，再扩 delivery event 消费方。
6. `push-gateway`：补 Redis 网络分区和容量测试，不把在线通知当 durable delivery。
7. `receipt-service`：补送达回执和批量接口，再产品化会话列表。
8. `contacts-service`：补分组 / 搜索 / 隐私策略。
9. `policy-service`：补 ReBAC / tenant DSL / quota / 外部审计。

每个切片必须满足：

- 只改一个服务或一个清晰跨服务边界。
- 有单测 / 集成测试 / smoke 中至少一种可复核证据。
- 更新对应 `service-briefs/<service>.md`。
- 不把入口文档重新写长。

## 当前阶段判断

当前最准确的阶段判断是：

```text
前 9 个微服务已经能跑通 IM 主链路，
现在处于“继续清现有服务的生产化 hardening”，
而不是“继续快速新增新服务”。
```

所以接下来的优先级是：

1. 继续做 `api-gateway` 入口治理：运行时动态 quota 配置、OTel collector / alerting / 跨服务 rollout 和 legacy opt-in 使用面迁移审计。
2. 继续把现有 9 个服务做干净。
3. 继续补分布式故障恢复 smoke。
4. 清各服务剩余 P2 hardening。
5. 等这批收口后，再进入 `search-service`。
6. `search-service` 稳定后，再做 `rag-service` / `summary-service` / `agent-service`。

## 维护规则

- 这里只写阶段结论，不堆长历史。
- 新增真实里程碑时才更新本页。
- 具体 smoke 证据写入 `docs/runbook/loadtest/<service>/`。
- 服务细节变化写入 `docs/runbook/service-briefs/<service>.md`。
