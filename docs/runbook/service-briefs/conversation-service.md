# conversation-service

## 当前状态

- 已有 `GetSendContext`、`CreateMemberChange`、`GetMemberChange`、`ListConversationMembers`、owner transfer。
- 成员变更走 shared timeline/outbox，保持 `conversation_seq` 顺序。
- 是会话成员事实源，其它服务不要跨表读取 `conversation_members`。

## 后续

- 更完整群管理、owner transfer 策略、成员窗口历史 repair。
