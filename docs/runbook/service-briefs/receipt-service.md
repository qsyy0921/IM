# receipt-service

## 当前状态

- 已有 `MarkRead`、`GetReceiptState`、`ListReceiptStates`、`ListConversations`。
- 已支持 unread、archive / pin / mute 的最小会话列表能力。
- 复用 delivery events 和 receipt projection，不跨服务读 delivery 内部表。
- 已补 `/healthz`、`/readyz`、`/debug/metrics`，可观察低敏 gRPC、PG pool、receipt projection、conversation summary 和 `receipt_outbox` 聚合状态；debug HTTP 监听默认只允许 loopback / RFC1918 私网，公网或 unspecified 地址必须显式 `NEXUSIM_RECEIPT_DEBUG_ALLOW_PUBLIC=true`。
- `delivery-consumer` 对非取消错误已改为退避重试，并在 worker 模式通过 `/debug/metrics` 暴露 projection worker retry 快照。
- `outbox-relay` 对非取消运行时错误已改为退避重试，并在 relay 模式通过 `/debug/metrics` 暴露 low-sensitive outbox relay retry 快照；publisher 错误写入稳定低敏 `last_error`。
- 已补只读 `outbox-audit`，以及 `outbox-repair` / 只读 `outbox-repair-audit` / `outbox-repair-cleanup` 运维模式，可直接审计、redrive 和清理 `receipt_outbox` repair 历史。
- 已补 receipt outbox audit / repair audit 错误脱敏：`last_error`、`previous_last_error` 对外和新写 repair 历史只保留稳定低敏分类，不暴露 broker body、账号、token 或 provider 原文。
- 已补 trusted metadata 启动门禁：当 `NEXUSIM_RECEIPT_AUTH_MODE=metadata|verified-metadata` 时，如果 gRPC 监听地址不是 loopback / RFC1918 私网，且服务端未启用 mTLS client cert 校验，则启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。
- 已补 first-stage OpenTelemetry gRPC server span；gRPC access log 只记录低敏 `trace_id/request_id`，并对白名单外入口 metadata 直接丢弃。

## 后续

- 送达回执扩展、批量接口优化、会话列表产品化；OTel collector / alerting / dashboard 仍属于后续统一观测治理。
