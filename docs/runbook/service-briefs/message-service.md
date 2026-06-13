# message-service

## 当前状态

- 已有 `SendMessage`、`EditMessage`、`RevokeMessage`、`DeleteMessage` 主链路。
- 通过 outbox relay 发布 conversation timeline events，不在业务事务里直接 publish Kafka。
- 已接 conversation-service / policy-service，可走 verified metadata、TLS / mTLS。
- 已补 `outbox-audit`、`outbox-repair`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，可直接审计、redrive 和清理 `message_outbox` repair 历史。

## 后续

- 更多消息类型、私有删除、合规删除、容量和生产观测。
