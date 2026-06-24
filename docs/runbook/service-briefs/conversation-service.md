# conversation-service

- 会话和成员事实源；其它服务不得跨表读取 `conversation_members`。
- 已有 `GetSendContext`、`CreateMemberChange`、`GetMemberChange`、
  `ListConversationMembers`、owner transfer。
- conversation profile 是群标题 / 头像 URI / 群公告的事实源；`GetConversationProfile`
  要求当前 ACTIVE 成员可读，`UpdateConversationProfile` 只允许 ACTIVE OWNER /
  ADMIN 更新 GROUP conversation，并用 `expected_profile_version` fail-closed。
- conversation note 是 Agent / workflow 可审批写入的低风险会话注记事实源；
  `CreateConversationNote` 要求当前 ACTIVE 成员，通过公开 gRPC API 写入
  `conversation_notes`，按 `tenant + conversation + author + idempotency_key` 幂等。
- 成员变更走 shared timeline/outbox，保持 `conversation_seq` 顺序。
- 成员列表支持当前 ACTIVE roster 分页、legacy 单 role、多 role OR、`USER_ID_ASC` /
  `ROLE_USER_ID_ASC`、`user_id_prefix`。
- 本地观测已有 `/healthz`、`/readyz`、`/debug/metrics`、Prometheus `/metrics`、
  first-stage OTel gRPC span、低敏 access log 和 worker retry metrics。
- `member-change-worker` 非取消错误按 backoff 重试，batch size 会归一化到安全上限。
- `MarkPublishedMemberChanges` 只接受同 tenant / conversation、正确 producer 和
  `conversation.member.*` event type 的已发布 outbox 行推进 saga。
- owner transfer、成员列表、群公告 profile 读写、成员变更 / 发布推进已有真实 PostgreSQL 回归。
- conversation note 写入、幂等 replay、非 ACTIVE 成员拒绝和 DELETED conversation
  拒绝已有真实 PostgreSQL 回归。
- JOIN / rejoin 会刷新 `join_seq` 并清旧 `leave_seq`。
- 已有只读 `member-change-audit`、`member-window-audit` 和保守
  `member-window-repair` / repair audit operator。
- metadata / verified-metadata auth 模式在公网监听 + 无 gRPC mTLS client cert 时启动失败。
- PostgreSQL repository / tests 已按当前窗口、成员变更、发布推进、owner transfer 等拆同 package 文件。
- `loadtest/memberchange` 已输出 `capacity_summary`；seeded 短基线和 30m longrun 报告已归档。

- 支撑 search visibility、group memory 和 EvidencePack 前，继续关注 owner transfer、
  成员历史可见窗口和 member boundary event。
- 更完整群管理产品化、完整历史窗口 / targeted replay repair 后续设计。
- OTel collector、生产 alerting / SLO dashboard 和更完整容量曲线后置统一观测治理。
