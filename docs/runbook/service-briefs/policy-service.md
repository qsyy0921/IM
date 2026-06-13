# policy-service

## 当前状态

- 已有 `CheckMessageAction`、PG exact rules、tenant rules、conversation role gate、contacts projection。
- 已有 ownership gate / override、decision audit outbox relay 和 repair。
- 已补只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，可直接审计并按 retention / scope 清理 policy decision audit outbox repair 历史，不改当前 outbox 状态。
- message-service 通过 policy-service 做权限决策，不复制策略实现。

## 后续

- 完整 ReBAC、moderation policy、tenant DSL / quota、外部 audit sink。
