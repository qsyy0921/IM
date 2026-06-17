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
   已有 Kafka producer first-stage `acks=all` / bounded retry-backoff 门禁、6 个 producer package 配置单测、config summary、ISR observation raw summary 的 JSON / Markdown validator、Kafka producer hardening evaluation（明确当前 `kafka-go` 不声明 idempotent / transactional producer 语义）、本地 `kafka-go` producer in-flight broker-fault observation（120 条 ack / consume unique，0 missing ack，0 observed duplicate）、push-gateway delivery-consumer 本地 consumer group rebalance smoke、本地 consumer churn smoke（2 轮 leave / rejoin、8 个 transition 均回到 Stable 且 3 个 partition 已分配）、本地 consumer churn probe smoke（8 个 transition 后共写入 24 条合法 `delivery.inbox_item.created.v1` probe，全部 ack 且 post-probe lag 回到 0），以及本地 Kafka KRaft repeated ISR flapping smoke（2 轮 broker stop/start，验证 ISR 2/3 收缩恢复和 `acks=all` 探针写入）。仍需补更长时间 / 更高频 consumer rebalance storm、长时间 ISR flapping / 容量曲线、更恶劣 fault 下的 ambiguous write / duplicate campaign，以及如果后续要声明 exactly-once producer 语义时的客户端替换和实测验证。Redis Cluster 本地真实拓扑、node-stop fallback、六节点自动 failover smoke 和六节点短容量基线已通过，后续仍需生产级 Redis HA 设计、长时间 Redis Cluster 容量曲线 / 生产 sizing、PostgreSQL quorum / split-brain fencing 和服务发现 / 部署编排。PostgreSQL 生产 quorum 边界以 ADR-034 为准。

3. Repair / DLQ / audit 产品化：
   多数服务已有本地 operator / audit / cleanup，且已有统一 operator 索引、机器可读 `repair-operators.catalog.json`、只生成低敏 JSON 的 `write-repair-operator-plan.ps1` 计划入口、`write-repair-approval-request.ps1` first-stage 审批请求入口、`write-repair-approval-decision.ps1` first-stage 审批决定入口、`validate-repair-approval-chain.ps1` 执行前链路校验入口和默认不执行的 `invoke-approved-repair-operator.ps1` 本地执行预检入口；后续要补真正跨服务批量编排、正式审批系统、外部审计 sink 和运维 UI，不把手写 SQL 当作长期方案。

4. 安全启动门禁维护：
   现有 public listener、mock auth、metadata auth、verified metadata、TLS / mTLS allowlist 已纳入 `tools/check-local.ps1`；repair / cleanup operator 的 dry-run env 文档和 cmd wiring 也已纳入 `check-repair-operator-index.ps1`；后续新增 listener、服务或会 mutate / cleanup 的 operator 时必须同步门禁和服务级测试。

5. 容量和复杂度治理：
   已有 9 服务健康态 Docker resource snapshot 入口、摘要工具、文件大小 hotspot summary；9 个服务均已有可复用 `capacity_summary` 口径，其中 api-gateway 通过 `loadtest/demo --gateway-facade` 统计 GatewayService facade 端到端容量，其余服务通过对应 loadtest runner 统计；`tools/summarize-loadtest-capacity-baselines.ps1` 可从 H 盘原始结果聚合容量基线索引，`tools/run-loadtest-capacity-baseline-suite.ps1` 可 dry-run / 顺序执行 direct 短基线，并会把需要额外 relay/consumer 角色的 runner 标记为 `skipped_stack_required`、把需要预置业务数据的 runner 标记为 `skipped_seed_required`；`deploy/local/docker-compose.service-workers.yml` 已提供本地后台 relay / consumer overlay；`loadtest/capacityseed` 已提供 message / conversation / delivery seeded runner 的本地 fixture 准备入口，且三条 seeded 短基线已跑通；contacts stack 短基线已通过 contacts outbox relay 和 `im.contact.events` Kafka readback；identity stack 短基线已通过临时 webhook fixture 和 challenge-delivery-worker；receipt stack 短基线已通过 message / delivery / receipt relay-consumer 链路和 receipt Kafka readback；api-gateway stack 短基线已通过 secure mTLS + HMAC GatewayService facade、push WebSocket、delivery / receipt / policy Kafka readback 链路；push-gateway stack 短基线已通过 full 场景在线 notify / PullInbox / ACK / delivery_outbox 链路；policy-service 已有本地 direct 短基线。9 个服务的短基线证据已覆盖，后续仍需长时间瓶颈、资源曲线和生产 sizing。生产手写文件接近 2500 行、测试或 runner 接近 3000 行时继续同 package 拆分。

## 逐服务未完成工作

| 服务 | 未完成工作 |
| --- | --- |
| `api-gateway` | 在目标环境持续运行 legacy observation window gate 并归档结果；legacy descriptor 移除计划；完整配置中心 / DB-backed quota hardening；生产级 collector / alerting / dashboard；长时间容量曲线和生产 sizing。 |
| `identity-service` | WebAuthn/passkeys；OIDC；多 issuer；KMS/HSM；完整风控；生产级 email/SMS provider；租户级通知模板；bounce handling；长时间容量曲线和生产 sizing。 |
| `message-service` | 会话级删除策略深化；provider-grade 外部 proof 工作流 / 审批系统集成；发送链路生产观测；长时间容量曲线和生产 sizing；图片 / 文件 / 语音二进制上传处理后续由 media 能力承担。 |
| `conversation-service` | 更完整群管理；owner transfer 策略继续打磨；完整历史窗口 / targeted replay repair；长时间容量曲线和生产 sizing。 |
| `delivery-service` | 更多 delivery event 消费方；已完成 projection failure audit / rewind / resolve / cleanup 第一阶段 operator 闭环，仍需长时间容量曲线和生产 sizing。 |
| `push-gateway` | 生产级 Redis HA 设计；长时间容量曲线和生产 sizing。 |
| `receipt-service` | 会话列表更多产品化能力（更多摘要策略等）；长时间容量曲线和生产 sizing。 |
| `contacts-service` | 组织级策略、租户默认值 / 来源策略 / 隐私例外接入 admin/config service 正式权限面；已完成 first-stage 来源风险标注、`REVIEW_REQUIRED` 持久化和本地 operator 审批状态机，仍需长时间容量曲线和生产 sizing。 |
| `policy-service` | 完整 ReBAC；provider-grade moderation / risk scoring；provider-grade tenant DSL / quota；provider-grade 外部 audit sink；已完成 first-stage keyword / HTTP content moderation、低敏 `decision-audit-export` 本地审计交接、first-stage tenant action quota 和本地 direct 短基线，仍需长时间容量曲线和生产 sizing。 |

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
