# policy-service

## 当前状态

- 已有 `CheckMessageAction`、PG exact rules、tenant rules、conversation role gate、contacts projection。
- 已有 ownership gate / override、decision audit outbox relay 和 repair。
- 已补 `/healthz`、`/readyz`、`/debug/metrics`，可观察低敏 gRPC、decision、PG pool、policy rule store、projection 和 `policy_decision_audit_outbox` 聚合状态。
- `contact-consumer` 和 `timeline-consumer` 对非取消错误已改为退避重试，并在 worker 模式通过 `/debug/metrics` 暴露 projection worker retry 快照。
- 已补只读 `outbox-audit`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，可直接审计当前 `policy_decision_audit_outbox` 行，以及按 retention / scope 清理 policy decision audit outbox repair 历史，不改当前 live outbox 状态。
- message-service 通过 policy-service 做权限决策，不复制策略实现。

## 后续

- 完整 ReBAC、moderation policy、tenant DSL / quota、外部 audit sink。
