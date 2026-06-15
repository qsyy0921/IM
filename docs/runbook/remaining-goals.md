# NexusIM Remaining Goals

这份文档只记录当前还没有完成的工作。当前进度总览见 `development-progress.md`，单服务事实见 `service-briefs/<service>.md`。

维护规则：

- 新发现的待完成工作追加到本文件。
- 已完成的工作从本文件移除，并同步到对应 service brief / progress / smoke report。
- 不记录已完成证据，不写长历史，不替代 SDD / ADR。

## 全局未完成工作

1. 安全启动门禁继续收口：
   继续复核 public listener、mock auth、metadata auth、gateway verified metadata、TLS / mTLS allowlist、弱鉴权公网暴露保护，并保证新增服务或新增 listener 同步纳入 `tools/check-local.ps1` 和服务级测试。

2. Trusted metadata / TLS 边界继续补强：
   对 trusted metadata listener / backend guard 继续补私网无 mTLS 放行、公网无 mTLS 拒绝、公网 mTLS 放行、body auth 跳过等覆盖；服务端 gRPC / WSS TLS 配置继续覆盖 cert/key 成对、invalid require-client-cert bool、缺 client CA 等启动失败路径。

3. 观测闭环生产化：
   当前 first-stage `/metrics`、Prometheus rules、Grafana dashboard、OTel trace wiring 只用于本地开发和面试展示；仍需补统一 collector、alert、dashboard smoke、采样治理、结构化日志和更接近生产的告警口径。

4. 分布式故障 smoke 深化：
   继续补更长时间 Kafka ISR flapping、consumer rebalance、producer retry budget；继续完善 Redis / PostgreSQL / Kafka 更接近生产 HA 的故障演练。PostgreSQL 生产 quorum / split-brain fencing 边界以 ADR-034 为准。

5. Repair / DLQ / audit 深化：
   outbox、projection、challenge delivery、policy、contacts、receipt 等 operator 继续补可复跑 repair、cleanup、audit、批量处理和审批边界。

6. 代码复杂度治理：
   生产手写文件接近 2500 行、测试或 runner 接近 3000 行时继续同 package 拆分，避免继续堆大文件。

## 逐服务未完成工作

| 服务 | 未完成工作 |
| --- | --- |
| `api-gateway` | 目标环境 legacy quiet-window observation；legacy descriptor 移除计划；完整配置中心 / DB-backed quota hardening；统一 collector / alerting / dashboard。 |
| `identity-service` | WebAuthn/passkeys；OIDC；多 issuer；KMS/HSM；完整风控；生产级 email/SMS provider；租户级通知模板；bounce handling。 |
| `message-service` | 更多消息类型；会话级删除策略深化；合规删除；容量观测深化；发送链路生产观测。 |
| `conversation-service` | 更完整群管理；owner transfer 策略；成员窗口历史 repair / repair action。 |
| `delivery-service` | Projection DLQ / repair 深化；更多 delivery event 消费方；隐藏项跨设备提示。 |
| `push-gateway` | 跨实例 resume 强化；容量测试；Redis Cluster / 生产级 HA 设计；Redis 网络分区组合 smoke。 |
| `receipt-service` | 送达回执扩展；批量接口优化；会话列表产品化。 |
| `contacts-service` | 更细 profile / 来源；联系人分组深化；联系人搜索深化；租户级隐私策略。 |
| `policy-service` | 完整 ReBAC；moderation policy；tenant DSL / quota；外部 audit sink。 |

## 后续未启动工作

这些工作暂不抢在前 9 个服务收口之前做：

- `search-service`
- `memory-service` / memory projection
- `media-service`
- `notification-service`
- `audit-service`
- `admin-service`
- `retrieval-gateway`
- `rag-service`
- `summary-service`
- `agent-service`
- `ai-eval-service` / AI eval harness
- Web / App / 桌面端产品化展示层
