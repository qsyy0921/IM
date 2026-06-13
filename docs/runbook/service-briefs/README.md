# NexusIM Service Briefs

本文件是短状态索引。只记录“现在是什么、下一步是什么”。历史证据去对应 `docs/runbook/loadtest/<service>/` 或 `docs/runbook/archive/` 按关键词查。

## 已有服务

### message-service
- 已有 SendMessage、EditMessage、RevokeMessage、DeleteMessage 主链路，outbox relay 发布 conversation timeline events。
- 已接 conversation-service / policy-service，可走 verified metadata、TLS / mTLS。
- 后续：更多消息类型、私有删除、合规删除、容量和生产观测。

### conversation-service
- 已有 GetSendContext、CreateMemberChange、GetMemberChange、ListConversationMembers、owner transfer。
- 成员变更走 shared timeline/outbox，保持 conversation_seq 顺序。
- 后续：更完整群管理、owner transfer 策略、成员窗口历史 repair。

### delivery-service
- 已有 timeline projection、durable `user_inbox`、PullInbox、AckDelivery、delivery_outbox relay。
- 是 push-gateway 的可靠事实源。
- 后续：projection DLQ / repair、更多 delivery event 消费方。

### push-gateway
- 已有 WebSocket notify、ACK 转发、slow session close、resume buffer、Redis route、cross-instance smoke。
- 只做在线唤醒，不拥有 durable inbox。
- 后续：Redis route TTL / 故障指标、跨实例 resume 强化、容量测试。

### receipt-service
- 已有 MarkRead、GetReceiptState、ListReceiptStates、ListConversations、unread、archive / pin / mute。
- 复用 delivery events 和 receipt projection。
- 后续：送达回执扩展、批量接口优化、会话列表产品化。

### contacts-service
- 已有好友申请、列表、接受、拒绝、取消、删除、拉黑、解除拉黑、备注。
- contacts 是独立事实源；policy-service 通过 contacts event projection 做 direct block 决策。
- 后续：联系人分组、搜索、更多隐私策略。

### identity-service
- 已有 RegisterUser、Login、RefreshGatewayToken、JWKS / RS256 keyring、device/session revoke、verification/password reset challenge、challenge delivery outbox、MFA TOTP、recovery codes、Refresh step-up、mTLS。
- 已完成 TOTP / recovery-code proof 在最终 Login / Refresh 事务内重新检查 lock，锁定期间不消费 proof、不写 session、不轮换 refresh token。
- 后续：WebAuthn/passkeys、OIDC federation、KMS/HSM、完整风控、生产级 email/SMS provider。

### policy-service
- 已有 CheckMessageAction、PG exact rules、tenant rules、conversation role gate、contacts projection、ownership gate / override、decision audit outbox relay 和 repair。
- 后续：完整 ReBAC、moderation policy、tenant DSL / quota、外部 audit sink。

### api-gateway
- 已有 GatewayService facade、gateway token 验证、verified metadata 注入、下游代理、health/ready/metrics、correlation propagation、基础 rate limiter。
- 后续：统一 OpenTelemetry、配额治理、逐步收敛 legacy descriptors。

## 文档拆分规则

- 当前任务入口只放在 `docs/runbook/current-brief.md`。
- 长期目标只放短版 `docs/runbook/current-goal.md`。
- 历史长文档归档到 `docs/runbook/archive/`。
- 单服务细节不要塞回入口文档；放到本文件对应服务段落或对应 SDD / loadtest README。
