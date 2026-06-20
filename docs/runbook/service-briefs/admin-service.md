# admin-service

状态：product-active / 第一版 implementation + outbox relay landed。已同步 registry、
proto、migration、runtime、Docker、worker compose 和 observability。

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
- 覆盖 proto、core migration、六层 skeleton、`grpc` runtime、Docker / Prometheus / Grafana。
- PostgreSQL first path 覆盖 create conflict / replay、approval replay、separation-of-duty、get/list 和低敏 outbox。
- Outbox relay 覆盖 typed protobuf schema、Kafka writer producer、builder fail-closed、
  PostgreSQL retry / DLQ / same-operation order blocker 和 worker compose。

后续：

- operation worker、workflow-service 长审批、audit-service ingestion / export、
  admin UI、operator approval CLI、下游公开 admin API adapter。
