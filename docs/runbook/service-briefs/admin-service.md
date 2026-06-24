# admin-service

状态：product-active / operation、approval、outbox relay、operation-worker、
operator CLI 和 first-stage downstream adapters 已落。

定位：管理后台 API 和 operator workflow 入口；负责租户 / 配置 / repair /
高风险操作的申请、审批、执行视图和补偿入口。

边界：

- 不直接写其他服务私有表；下游 mutation 只能走公开 API、事件或 operator command。
- 高风险操作必须走 policy precheck、approval、audit 和幂等键。
- 不承载普通用户 IM 流量，不替代 api-gateway。

已覆盖：

- `CreateAdminOperation` / `ApproveAdminOperation` / `GetAdminOperation` /
  `ListAdminOperations`、低敏 `admin_outbox -> im.admin.events`。
- `operation-worker`：APPROVED operation -> risk / type route -> result projection。
- workflow route：`REPAIR_REQUEST -> REPAIR_APPROVAL`、`CRITICAL -> ADMIN_OPERATION`。
- provider replay route：`PROVIDER_REPLAY_REQUEST -> workflow-service REPAIR_APPROVAL`，
  target service 为 `action-executor`，approval policy 为
  `admin.workflow.provider_replay.v1`；admin-service 只创建 / 审批 / 路由请求，不执行
  `RedriveProviderFailure`。
- provider replay operator bridge：`loadtest/admin provider-replay-submit` 可读取
  action-executor handoff artifact 并创建低敏 `PROVIDER_REPLAY_REQUEST`；`provider-replay-list`
  / `provider-replay-approve` / `provider-replay-reject` 提供第一版 operator UX，不执行 redrive。
- control-plane adapters：`CONFIG_PUBLISH`、`CONFIG_ROLLBACK`、
  `TENANT_QUOTA_CHANGE`、`POLICY_RULE_CHANGE`。
- audit adapter：`AUDIT_EXPORT_REQUEST -> audit-service.CreateAuditExport`；
  只传低敏 filter hash / redaction profile / requester refs。
- first-stage `compensation-request` operator 和 workflow handoff。

证据入口：`docs/runbook/loadtest/admin-service/`。

后续：admin-event ingestion、admin UI、provider-grade provider replay request UI、更多明确下游公开
API adapter、更多补偿 adapter、provider-grade compensation instruction 审批 / UI 和运维。
