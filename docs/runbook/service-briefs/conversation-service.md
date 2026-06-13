# conversation-service

## 当前状态

- 已有 `GetSendContext`、`CreateMemberChange`、`GetMemberChange`、`ListConversationMembers`、owner transfer。
- 成员变更走 shared timeline/outbox，保持 `conversation_seq` 顺序。
- 是会话成员事实源，其它服务不要跨表读取 `conversation_members`。
- 已补第一阶段本地观测：`/healthz`、`/readyz`、`/debug/metrics`，包含低敏 gRPC、PG pool、`conversations` / `conversation_members` / `member_change_saga` 聚合快照。

## 后续

- 更完整群管理、owner transfer 策略、成员窗口历史 repair。
