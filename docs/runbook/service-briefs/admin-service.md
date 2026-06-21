# admin-service

状态：product-active / 第一版 implementation + outbox relay + operation worker + repair workflow routing landed。
已同步 registry、proto、migration、runtime、Docker、worker compose 和 observability。

定位：管理后台 API，负责租户管理、封禁、配置操作、repair 审批和 operator workflow 入口。

边界：

- admin-service 不直接写其他服务私有表，必须通过公开 API / operator command。
- 高风险操作必须走 policy precheck、approval、audit 和幂等键。
- 不承载普通用户 IM 流量，不替代 api-gateway 的客户端入口。
- 管理操作默认最小权限、可追溯、可撤销或可补偿。

第一切片：
- 边界见 `docs/sdd/admin-service.md`；stage-switch 见
  `docs/runbook/stage-switch/admin-service.md`。
- 已落 `CreateAdminOperation`、`ApproveAdminOperation`、`GetAdminOperation`、
  `ListAdminOperations`，不直接执行真实下游 mutation。
- 已输出低敏 admin outbox event，并提供 `outbox-relay` runtime 发布
  `admin_outbox -> im.admin.events`，后续可由 audit-service 归档。
- 已提供 `operation-worker` runtime：claim APPROVED operation -> risk router ->
  local no-op executor 或 workflow-service -> 写 `admin_operation_results` ->
  operation 终态 `SUCCEEDED/FAILED` -> 写
  `admin.operation.executed/failed.v1` outbox。
- 已提供 first-stage `compensation-request` 本地 operator：默认 dry-run，显式关闭
  dry-run 后只允许把 `FAILED` operation 标记为 `COMPENSATION_REQUESTED`，并写低敏
  `admin.operation.compensation_requested.v1` outbox；reason file 只落 hash / ref，
  不落 reason 原文。
- `compensation-request` 已支持可选 workflow handoff：设置
  `NEXUSIM_WORKFLOW_GRPC_ADDR` 后，会为同一 failed operation 创建 / replay
  workflow-service 的 `COMPENSATION_REQUEST` workflow；不在 admin-service 内联执行
  高风险补偿 mutation。
- 已接入第一版 workflow 路由：`REPAIR_REQUEST` 创建
  `workflow-service` 的 `REPAIR_APPROVAL`，其它 `CRITICAL` operation 创建
  `ADMIN_OPERATION`；config / quota / policy / audit / notification 类 operation
  会写入专用 approval policy 和 target service，result 记录 `workflow:<workflow_id>`；
  未配置 workflow 地址时，`REPAIR_REQUEST` / `CRITICAL` 操作 fail-closed，
  不会被本地 no-op executor 标记成功。
- 覆盖 proto、core migration、六层 skeleton、`grpc` runtime、Docker / Prometheus / Grafana。
- PostgreSQL first path 覆盖 create conflict / replay、approval replay、separation-of-duty、get/list 和低敏 outbox。
- Outbox relay 覆盖 typed protobuf schema、Kafka writer producer、builder fail-closed、
  PostgreSQL retry / DLQ / same-operation order blocker 和 worker compose。
- Operation worker 覆盖 worker unit、executor risk routing、workflow RPC executor
  unit、真实 PostgreSQL result / outbox integration、`operation-worker` cmd mode、
  worker registry 和本地 compose wiring。
- 已新增 `loadtest/admin` operator CLI，支持通过公开 gRPC 执行 create / approve /
  reject / get / list，本地输出低敏 JSON，不读数据库私表。
- 已新增第一条真实下游公开 API adapter：非 `CRITICAL` 的 `CONFIG_PUBLISH`
  在配置 `NEXUSIM_CONTROL_PLANE_GRPC_ADDR` 后由 operation-worker 调
  `control-plane-service.PublishConfigVersion`；未配置时保留本地 first-stage
  no-op fallback，`CRITICAL` 操作仍走 workflow。
- 已新增第二条 control-plane 下游公开 API adapter：非 `CRITICAL` 的
  `CONFIG_ROLLBACK` 在配置 `NEXUSIM_CONTROL_PLANE_GRPC_ADDR` 后由
  operation-worker 调 `control-plane-service.RollbackConfigVersion`。
- 已新增第三条 control-plane 下游公开 API adapter：非 `CRITICAL` 的
  `TENANT_QUOTA_CHANGE` 在配置 `NEXUSIM_CONTROL_PLANE_GRPC_ADDR` 后由
  operation-worker 调 `control-plane-service.PublishConfigVersion` 发布
  `API_GATEWAY_TENANT_QUOTA` 配置。
- 已新增第四条 control-plane 下游公开 API adapter：非 `CRITICAL` 的
  `POLICY_RULE_CHANGE` 在配置 `NEXUSIM_CONTROL_PLANE_GRPC_ADDR` 后由
  operation-worker 调 `control-plane-service.PublishConfigVersion` 发布低敏
  `POLICY_RULESET_REF` 配置引用；admin-service 不承载 policy 规则正文，也不写
  policy-service 私有表。
- 已新增并跑通本地多进程 config publish smoke：通过公开 gRPC 执行
  `CreateAdminOperation -> operator approve -> operation-worker ->
  control-plane PublishConfigVersion -> GetConfigSnapshot`，报告见
  `docs/runbook/loadtest/admin-service/loadtest-report-20260621-admin-config-publish-smoke.md`。
- 已新增并跑通本地多进程 config rollback smoke：通过公开 gRPC 连续发布 v1 / v2，
  再执行 `CONFIG_ROLLBACK -> control-plane RollbackConfigVersion` 回到 v1，报告见
  `docs/runbook/loadtest/admin-service/loadtest-report-20260621-admin-config-rollback-smoke.md`。
- 已新增并跑通本地多进程 tenant quota smoke：通过公开 gRPC 执行
  `TENANT_QUOTA_CHANGE -> control-plane PublishConfigVersion(API_GATEWAY_TENANT_QUOTA)`
  并用 `GetConfigSnapshot` 验证生效版本，报告见
  `docs/runbook/loadtest/admin-service/loadtest-report-20260621-admin-tenant-quota-smoke.md`。
- 已新增并跑通本地多进程 policy ruleset smoke：通过公开 gRPC 执行
  `POLICY_RULE_CHANGE -> control-plane PublishConfigVersion(POLICY_RULESET_REF)`
  并用 `GetConfigSnapshot` 验证生效版本，报告见
  `docs/runbook/loadtest/admin-service/loadtest-report-20260621-admin-policy-ruleset-smoke.md`。

后续：

- audit-service ingestion / export、admin UI、更多下游公开 admin API adapter、
  provider-grade compensation execution / 运维。
