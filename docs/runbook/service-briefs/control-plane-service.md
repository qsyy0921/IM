# control-plane-service

状态：product-active / first-stage config version、snapshot、rollback 和 ACK 已落。

定位：多租户运行控制面，负责功能开关、灰度、限流策略、配额、配置版本和
applied-version ACK。

边界：
- 不替代 policy-service 授权决策，也不替代 api-gateway 的请求限流执行。
- 配置发布必须版本化、可回滚、可审计，并能证明实例已应用。
- 动态配置不能绕过启动安全门禁、mTLS 或 trusted metadata 边界。

已覆盖：
- `PublishConfigVersion` / `RollbackConfigVersion` / `GetConfigSnapshot` /
  `AckAppliedConfigVersion`。
- Tenant quota / feature flag snapshot 的 DB-backed source。
- 回滚使用 idempotency key，replay 不重复推进，并写低敏 rollback outbox。
- 最小 gRPC smoke 和 admin-driven config publish / rollback / tenant quota /
  policy ruleset smoke 已归档在 `docs/runbook/loadtest/`。
- 与 api-gateway quota snapshot gate 对齐。

后续：outbox relay、drift monitor、expiry / cleanup worker、api-gateway quota
consumer、provider-grade rollout。
