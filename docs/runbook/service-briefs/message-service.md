# message-service

## 当前状态

- 已有 `SendMessage`、`EditMessage`、`RevokeMessage`、`DeleteMessage` 主链路。
- 通过 outbox relay 发布 conversation timeline events，不在业务事务里直接 publish Kafka。
- 已接 conversation-service / policy-service，可走 verified metadata、TLS / mTLS。
- 已补 `/healthz`、`/readyz` 和 `/debug/metrics`，可观察低敏 PG pool 和 send / repository / outbox relay 聚合指标。
- 已补 first-stage OpenTelemetry gRPC server span，默认关闭；启用后可输出 stdout 或 OTLP gRPC trace，并从入口 metadata 提取 W3C `traceparent`。
- `outbox-relay` 对非取消运行时错误已改为退避重试，并在 relay 模式通过 `/debug/metrics` 暴露 low-sensitive outbox relay retry 快照；malformed event / payload 仍保持 fail-closed，交给 outbox retry / DLQ 语义处理。
- 已补 `outbox-audit`、`outbox-repair`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，可直接审计、redrive 和清理 `message_outbox` repair 历史。
- 已补 trusted metadata 启动门禁：当 `NEXUSIM_MESSAGE_AUTH_MODE=metadata|verified-metadata` 时，如果 gRPC 监听地址不是 loopback / RFC1918 私网，且服务端未启用 mTLS client cert 校验，则启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。

## 后续

- 更多消息类型、私有删除、合规删除、容量和生产观测。
- OTel collector / alerting / dashboard 仍属于后续统一观测治理。
