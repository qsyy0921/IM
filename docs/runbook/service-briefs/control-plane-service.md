# control-plane-service

状态：future / SDD v0.1 draft 已存在 / stage-switch 已通过。下一实现切片
必须同步切换 service-registry，并创建 `services/control-plane-service` 目录。

Stage-switch 记录：`docs/runbook/stage-switch/control-plane-service.md`。

定位：多租户运行控制面，负责功能开关、灰度、限流策略、配额、动态策略发布、
配置版本和 applied-version ACK。

边界：

- 不替代 policy-service 的授权决策，也不替代 api-gateway 的请求限流执行。
- 配置发布必须版本化、可回滚、可审计，并能证明实例已应用。
- 动态配置不能绕过启动安全门禁和 mTLS / trusted metadata 边界。
- tenant/config-service 需求统一收敛到该服务，除非后续 ADR 另拆。

第一切片建议：

- 具体边界见 `docs/sdd/control-plane-service.md`。
- Tenant quota / feature flag snapshot 的 DB-backed source。
- `PublishConfigVersion` / `AckAppliedConfigVersion`。
- 与 api-gateway quota snapshot gate 对齐。
