# NexusIM Remaining Goals

这份文档只记录当前还没有完成的工作。当前进度总览见 `development-progress.md`，单服务事实见 `service-briefs/<service>.md`。

维护规则：

- 新发现的待完成工作追加到本文件。
- 已完成的工作从本文件移除，并同步到对应 service brief / progress / smoke report。
- 不记录已完成证据，不写长历史，不替代 SDD / ADR。

## 全局未完成工作

1. 生产级观测闭环：
   当前 `/metrics`、Prometheus rules、Grafana dashboard、OTel trace wiring、本地 Alertmanager null route 和本地观测栈 smoke runner 是本地开发 / 面试展示级；仍需目标环境 dashboard smoke、统一 collector、生产 Alertmanager 路由、retention、结构化日志汇聚、容量基线和 SLO 口径。

2. 分布式 HA / 故障演练深化：
   已有 Kafka producer first-stage `acks=all` / bounded retry-backoff 门禁、6 个 producer package 配置单测和 config summary；仍需补更长时间 Kafka ISR flapping、consumer rebalance、Kafka producer 真实故障重试行为 smoke / idempotent-producer 客户端评估。继续完善 Redis Cluster / 生产级 Redis HA 设计、PostgreSQL quorum / split-brain fencing 和服务发现 / 部署编排。PostgreSQL 生产 quorum 边界以 ADR-034 为准。

3. Repair / DLQ / audit 产品化：
   多数服务已有本地 operator / audit / cleanup，且已有统一 operator 索引；后续要补跨服务执行编排、批量 repair、审批边界、外部审计 sink 和运维 UI，不把手写 SQL 当作长期方案。

4. 安全启动门禁维护：
   现有 public listener、mock auth、metadata auth、verified metadata、TLS / mTLS allowlist 已纳入 `tools/check-local.ps1`；后续新增 listener / 服务时必须同步门禁和服务级测试。

5. 容量和复杂度治理：
   已有 9 服务健康态 Docker resource snapshot 入口、摘要工具和文件大小 hotspot summary；仍需补各服务容量基线、瓶颈和资源曲线。生产手写文件接近 2500 行、测试或 runner 接近 3000 行时继续同 package 拆分。

## 逐服务未完成工作

| 服务 | 未完成工作 |
| --- | --- |
| `api-gateway` | 目标环境 legacy quiet-window observation；legacy descriptor 移除计划；完整配置中心 / DB-backed quota hardening；生产级 collector / alerting / dashboard。 |
| `identity-service` | WebAuthn/passkeys；OIDC；多 issuer；KMS/HSM；完整风控；生产级 email/SMS provider；租户级通知模板；bounce handling。 |
| `message-service` | 会话级删除策略深化；合规删除；容量观测深化；发送链路生产观测；图片 / 文件 / 语音二进制上传处理后续由 media 能力承担。 |
| `conversation-service` | 更完整群管理；owner transfer 策略继续打磨；成员窗口历史 repair / repair action。 |
| `delivery-service` | Projection DLQ / repair 深化；更多 delivery event 消费方；隐藏项跨设备提示。 |
| `push-gateway` | 跨实例 resume 强化；容量测试；Redis Cluster / 生产级 HA 设计。 |
| `receipt-service` | 送达回执扩展；会话列表更多产品化能力（草稿、标签、更多摘要策略等）。 |
| `contacts-service` | 更细 profile；陌生人申请的组织 / 风险 / 审批策略；租户默认值和来源策略后续接入 admin/config service 正式权限面。 |
| `policy-service` | 完整 ReBAC；内容分类 / provider-backed moderation；tenant DSL / quota；外部 audit sink。 |

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
