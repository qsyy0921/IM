# message-service

## 当前状态

- 已有 `SendMessage`、`EditMessage`、`RevokeMessage`、`DeleteMessage` 主链路；`SendMessage` 支持 `TEXT`、第一阶段 `IMAGE` / `FILE` / `VOICE` 附件引用消息，以及 `LOCATION` / `CARD` 结构化 payload 消息。
- 通过 outbox relay 发布 conversation timeline events，不在业务事务里直接 publish Kafka。
- 已接 conversation-service / policy-service，可走 verified metadata、TLS / mTLS。
- 已补 `/healthz`、`/readyz`、`/debug/metrics` 和 Prometheus text `/metrics`，可观察低敏 PG pool、send / repository / Kafka / outbox relay 聚合指标和固定 operation latency；debug HTTP 监听默认只允许 loopback / 私网，公网或未指定地址必须显式 `NEXUSIM_MESSAGE_DEBUG_ALLOW_PUBLIC=true`。
- 本地 Prometheus scrape / alert rules 与 Grafana dashboard 原型已覆盖 SendMessage / PG pool / Kafka / outbox relay latency、relay runtime error 和 OTLP endpoint missing；这仍是本地开发 / 面试演示级观测，不是生产 Alertmanager / SLO。
- 已补 first-stage OpenTelemetry gRPC server span，默认关闭；启用后可输出 stdout 或 OTLP gRPC trace，并从入口 metadata 提取 W3C `traceparent`。
- gRPC access log 只记录低敏 `trace_id/request_id`，并对入口 metadata 做 trim、长度上限和字符白名单过滤，避免把 token / 邮箱 / 原始认证头写入结构化日志。
- `outbox-relay` 对非取消运行时错误已改为退避重试，并在 relay 模式通过 `/debug/metrics` 暴露 low-sensitive outbox relay retry 快照；malformed event / payload 仍保持 fail-closed，交给 outbox retry / DLQ 语义处理；`message_outbox.last_error` 和 repair audit `previous_last_error` 只暴露稳定公开文案，不落 Kafka / publisher 原始错误正文。
- Kafka writer 已显式固定 `acks=all`、禁自动建 topic、bounded attempts/backoff，并由本地门禁防漂移；真正 idempotent / transactional producer 仍属后续客户端选型。
- 已补 `outbox-audit`、`outbox-repair`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，可直接审计、redrive 和清理 `message_outbox` repair 历史。
- 已补 trusted metadata 启动门禁：当 `NEXUSIM_MESSAGE_AUTH_MODE=metadata|verified-metadata` 时，如果 gRPC 监听地址不是 loopback / RFC1918 私网，且服务端未启用 mTLS client cert 校验，则启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。

## 后续

- 会话级删除策略深化、合规删除、容量和生产观测；用户私有隐藏已由 delivery-service `HideInboxItem` 承担，图片 / 文件 / 语音二进制上传和处理属于后续 media 能力。
- 生产级 OTel collector、告警路由、retention 和 SLO dashboard 仍属于后续统一观测治理。
