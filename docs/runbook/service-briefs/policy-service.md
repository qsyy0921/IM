# policy-service

## 当前状态

- 已有 `CheckMessageAction`、PG exact rules、tenant rules、conversation role gate、contacts projection。
- 已有 ownership gate / override、decision audit outbox relay 和 repair。
- 已补 `/healthz`、`/readyz`、`/debug/metrics`，可观察低敏 gRPC、decision、PG pool、policy rule store、projection 和 `policy_decision_audit_outbox` 聚合状态。
- 已补 first-stage OpenTelemetry gRPC server span，默认关闭；启用后可输出 stdout 或 OTLP gRPC trace，并从入口 metadata 提取 W3C traceparent。
- 当 `NEXUSIM_POLICY_GRPC_ADDR` 是公网地址时，若未启用入口 gRPC TLS，进程会在启动前直接失败，避免把内部 policy decision API 暴露到 plaintext 公网入口；私网 / loopback 仍保留第一阶段本地直连。
- `outbox-relay` 对非取消运行时错误已改为退避重试，并在 relay 模式通过 `/debug/metrics` 暴露 low-sensitive outbox relay retry 快照；malformed payload / unsupported event 仍保持 store 驱动的 retry / DLQ 语义。
- `contact-consumer` 和 `timeline-consumer` 对非取消错误已改为退避重试，并在 worker 模式通过 `/debug/metrics` 暴露 projection worker retry 快照。
- 已补只读 `outbox-audit`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，可直接审计当前 `policy_decision_audit_outbox` 行，以及按 retention / scope 清理 policy decision audit outbox repair 历史，不改当前 live outbox 状态。
- message-service 通过 policy-service 做权限决策，不复制策略实现。

## 后续

- 完整 ReBAC、moderation policy、tenant DSL / quota、外部 audit sink；OTel collector / alerting / dashboard 仍属于后续统一观测治理。
