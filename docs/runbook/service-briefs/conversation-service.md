# conversation-service

## 当前状态

- 已有 `GetSendContext`、`CreateMemberChange`、`GetMemberChange`、`ListConversationMembers` 当前 ACTIVE roster 分页 / role 过滤、owner transfer。
- 成员变更走 shared timeline/outbox，保持 `conversation_seq` 顺序。
- 是会话成员事实源，其它服务不要跨表读取 `conversation_members`。
- 已补第一阶段本地观测：`/healthz`、`/readyz`、`/debug/metrics` 和 Prometheus text `/metrics`，包含低敏 gRPC、PG pool、`conversations` / `conversation_members` / `member_change_saga` 聚合快照；debug HTTP 监听默认只允许 loopback / 私网，公网或未指定地址必须显式 `NEXUSIM_CONVERSATION_DEBUG_ALLOW_PUBLIC=true`。
- 本地 Prometheus alert rules / Grafana dashboard 原型已接入，默认 scrape target 为 `host.docker.internal:11911`，用于开发和面试演示，不等同于生产告警体系。
- 已补 first-stage OpenTelemetry gRPC server span，默认关闭；启用后可输出 stdout 或 OTLP gRPC trace，并从入口 metadata 提取 W3C `traceparent`。
- gRPC access log 只记录低敏 `trace_id/request_id`，并对入口 metadata 做 trim、长度上限和字符白名单过滤，避免把 token / 邮箱 / 原始认证头写入结构化日志。
- `member-change-worker` 遇到非取消错误不再直接退出；当前会按 `error_backoff` 退避重试，且进度 batch size 会归一化到安全上限，避免 PostgreSQL 瞬时失败或误配置超大批次把 worker 打死。
- worker 模式的 `/debug/metrics` 现已额外暴露 `member_change_worker` retry 快照，便于区分持续重试、最近成功和最近推进批次。
- `MarkPublishedMemberChanges` 只接受同 tenant / conversation、`producer='conversation-service'` 且 event_type 属于 `conversation.member.*` 的已发布 outbox 行推进 saga，异常 outbox 行 fail-closed。
- owner transfer 已补真实 PostgreSQL 负向回归：目标成员 inactive 或目标已是 owner 时必须拒绝，且不能提交 `conversation_seq`、`member_change_saga`、timeline、outbox 或 roster mutation。
- 已补 `member-change-audit` 只读 operator，可按 `change_id`、tenant、conversation、status、outbox event 过滤 `member_change_saga`，并对 `last_error` 使用稳定公开文案，避免泄露 SQL / Kafka / repair 内部错误文本。
- 当 `NEXUSIM_CONVERSATION_AUTH_MODE=metadata|verified-metadata` 时，公网监听地址 + 无 gRPC mTLS client cert 的危险组合会在启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。
- `loadtest/memberchange` summary 已输出 `capacity_summary`，包含运行时长、VUs、请求 / 成功 / 错误计数、成功率、RPS、latency avg/p95/p99、成员变更类型、saga / timeline / outbox / roster / conversation_seq 聚合；后续容量验证可直接复用该结构。
- `loadtest/capacityseed` 已能准备 `tenant-capacity-conversation` 下的 ACTIVE owner fixture；`capacity-baseline-seeded-20260616` 本地 seeded 短基线中 `memberchange` 成功 214、`requests_per_second=42.8`，报告见 `loadtest/distributed/loadtest-report-20260616-seeded-capacity-baseline.md`。

## 后续

- 更完整群管理、owner transfer 策略继续打磨、成员窗口历史 repair / repair action。
- OTel collector / 生产级 alerting / SLO dashboard、长时间容量曲线仍属于后续统一观测治理。
