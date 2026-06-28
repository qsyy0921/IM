# conversation-service Brief

状态：core-active / conversation and member boundary fact source。

## 已落

- `GetSendContext`、`CreateMemberChange`、`GetMemberChange`、`ListConversationMembers`、
  owner transfer。
- Conversation profile：群标题 / 头像 URI / 公告事实源；ACTIVE OWNER / ADMIN 更新，
  ACTIVE 成员可读，`expected_profile_version` fail-closed。
- Conversation note：Agent / workflow 可审批写入的低风险会话注记事实源；公开 gRPC
  API 写入，幂等键保护。
- 成员变更 timeline / outbox、member-change worker、MarkPublished 防御过滤。
- member-change-audit、member-window-audit、保守 repair operator。
- mTLS / trusted metadata 启动门禁、Prometheus / OTel / access log、容量 summary。
- `loadtest/memberchange` 已输出 `capacity_summary`，用于成员变更容量基线。
- Conversation scale policy 已进入 domain 层：direct / small group 继续走当前
  `LOCAL_ROW_LOCK + WRITE_FANOUT`；medium / large / hot group 的目标策略分别建模为
  `HYBRID_FANOUT`、`READ_FANOUT`、`BROADCAST_SIGNAL + SEQUENCER_BLOCK`，但在
  timeline-service 和 delivery fanout planner 完成前保持 contract-only / fail-closed。

## 边界

- 其它服务不得跨表读取 `conversation_members` 或 `conversation_notes`。
- 成员窗口、群资料、会话注记必须通过公开 API / event 传播。
- Agent 写入 note / profile 必须经 action-executor 和公开 gRPC API。
- 群聊规模策略只由 conversation-service domain 计算；message / delivery 不得自行猜测
  群规模或把未知 fanout mode 降级成 write-fanout。

## 下一步

- 冻结 timeline-service SDD 后，再把 medium / large / hot group 策略从 contract-only
  分阶段推进到真实 sequencer / hybrid fanout / read fanout runtime。
- 群管理深化、owner transfer 策略、历史窗口 / targeted replay repair。
