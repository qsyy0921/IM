# admin-service Brief

状态：product-active / operation、approval、outbox relay、operator CLI 和 first-stage downstream adapters 已落。

定位：管理后台 API 和 operator workflow 入口；负责租户 / 配置 / repair / 高风险操作的申请、
审批、执行视图和补偿入口。

## 已落

- `CreateAdminOperation` / `ApproveAdminOperation` / `GetAdminOperation` /
  `ListAdminOperations`、低敏 `admin_outbox -> im.admin.events`。
- `operation-worker`：APPROVED operation -> risk / type route -> result projection。
- workflow route：`REPAIR_REQUEST -> REPAIR_APPROVAL`、`CRITICAL -> ADMIN_OPERATION`、
  `PROVIDER_REPLAY_REQUEST -> workflow-service REPAIR_APPROVAL`。
- provider replay operator bridge：submit / list / approve / reject 只创建、查询和审批
  low-sensitive admin operation，不调用 `RedriveProviderFailure`。
- provider replay handoff review、readiness、redrive invocation manifest 已落；admin-service
  只提供 approved operation 证据，不持有 raw resource id / input / reason。
- control-plane adapters：`CONFIG_PUBLISH`、`CONFIG_ROLLBACK`、
  `TENANT_QUOTA_CHANGE`、`POLICY_RULE_CHANGE`。
- audit adapter：`AUDIT_EXPORT_REQUEST -> audit-service.CreateAuditExport`。
- first-stage `compensation-request` operator 和 workflow handoff。

## 边界 / 后续

- 不直接写其他服务私有表；下游 mutation 只能走公开 API、事件或 operator command。
- 高风险操作必须走 policy precheck、approval、audit 和幂等键；不承载普通用户 IM 流量，不替代 api-gateway。
- admin-event ingestion、provider-grade replay / compensation UI、更多明确下游公开 API adapter。
