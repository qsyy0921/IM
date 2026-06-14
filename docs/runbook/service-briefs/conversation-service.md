# conversation-service

## 当前状态

- 已有 `GetSendContext`、`CreateMemberChange`、`GetMemberChange`、`ListConversationMembers`、owner transfer。
- 成员变更走 shared timeline/outbox，保持 `conversation_seq` 顺序。
- 是会话成员事实源，其它服务不要跨表读取 `conversation_members`。
- 已补第一阶段本地观测：`/healthz`、`/readyz`、`/debug/metrics`，包含低敏 gRPC、PG pool、`conversations` / `conversation_members` / `member_change_saga` 聚合快照。
- 已补 first-stage OpenTelemetry gRPC server span，默认关闭；启用后可输出 stdout 或 OTLP gRPC trace，并从入口 metadata 提取 W3C `traceparent`。
- `member-change-worker` 遇到非取消错误不再直接退出；当前会按 `error_backoff` 退避重试，且进度 batch size 会归一化到安全上限，避免 PostgreSQL 瞬时失败或误配置超大批次把 worker 打死。
- worker 模式的 `/debug/metrics` 现已额外暴露 `member_change_worker` retry 快照，便于区分持续重试、最近成功和最近推进批次。
- `MarkPublishedMemberChanges` 只接受同 tenant / conversation、`producer='conversation-service'` 且 event_type 属于 `conversation.member.*` 的已发布 outbox 行推进 saga，异常 outbox 行 fail-closed。
- 当 `NEXUSIM_CONVERSATION_AUTH_MODE=metadata|verified-metadata` 时，公网监听地址 + 无 gRPC mTLS client cert 的危险组合会在启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。

## 后续

- 更完整群管理、owner transfer 策略、成员窗口历史 repair。
- OTel collector / alerting / dashboard 仍属于后续统一观测治理。
