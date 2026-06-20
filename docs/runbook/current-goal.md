# NexusIM Current Goal

本文件只维护当前可执行目标。Codex 目标框短 prompt 见根目录 `prompt.md`，
不要把长 prompt 复制到这里。

## 当前 Active Slice

```text
future platform / product services promotion
```

用户已点名把 goal 指向全部待开发 future 微服务。当前目标不是只做
`media-service`，也不是一次性把全部服务目录铺开，而是把这批服务按统一边界推进到
可逐个实现的 promotion plan。

## 当前目标服务

```text
media / notification / audit / admin / control-plane / presence
model-gateway / workflow / knowledge-ingestion / vector-index
```

## 默认推进方式

1. 每轮先读 `prompt.md`、`agent.md`、本文件和相关 service brief。
2. 先冻结组合边界：哪些服务先做、哪些只保留 port / adapter、哪些需要 ADR。
   组合边界文档见 `docs/sdd/future-platform-services.md`。
3. 对每个服务按顺序推进：SDD v0.1 -> proto / migration -> 六层 skeleton
   -> cmd runtime -> Docker / Prometheus / Grafana -> focused smoke。
4. 第一组优先级建议：
   `media-service` -> `notification-service` -> `audit-service`
   -> `control-plane-service` -> `presence-service`
   -> `model-gateway` / `knowledge-ingestion-service` / `workflow-service`
   -> `vector-index-service`。
5. 只有完成对应服务 SDD v0.1 和门禁影响确认后，才把该服务从 `future`
   stage switch 到 active，并创建 `services/<service>`。

## 当前进展

- 组合 promotion 边界见 `docs/sdd/future-platform-services.md`。
- 10 个目标服务的 SDD v0.1 draft 已存在，单服务状态见 service brief。
- `media-service`、`notification-service`、`audit-service`、
  `control-plane-service`、`presence-service`、`model-gateway`、
  `knowledge-ingestion-service` 已 product-active 并通过各自第一版 focused
  checks / smoke。
- `workflow-service` 已从 stage-switch 进入 product-active 第一版 implementation
  slice，`CreateWorkflow`、`RecordWorkflowDecision`、`GetWorkflow` 最小路径已落地并
  支持 `ACTION_APPROVAL`、`REPAIR_APPROVAL` 和 `ADMIN_OPERATION`，通过 focused
  checks / 完整 `check-local`。
- `vector-index-service` 已从 stage-switch 进入 product-active 第一版 implementation
  slice，覆盖 `UpsertVectorItem`、`TombstoneVectorItem`、`SearchVectors`、
  `GetVectorIndexJob`、PostgreSQL metadata 和 local / PostgreSQL-backed test vector adapter。
- `admin-service` 已从 stage-switch 进入 product-active 第一版 implementation
  slice，覆盖 `CreateAdminOperation`、`ApproveAdminOperation`、
  `GetAdminOperation`、`ListAdminOperations`、PostgreSQL operation / approval
  状态、低敏 admin outbox、`admin_outbox -> im.admin.events` outbox relay 和
  `operation-worker` risk routing 第一版执行闭环。`REPAIR_REQUEST` 已接入
  `workflow-service` 创建 `REPAIR_APPROVAL`，其它 `CRITICAL` operation 已接入
  `ADMIN_OPERATION`，并为 config / quota / policy / audit / notification 类操作写入
  第一版专用 approval policy 和 target service；未配置 workflow 时
  `REPAIR_REQUEST` / `CRITICAL` 操作 fail-closed，不再被本地 no-op executor 标记成功。
- `admin-service` 已新增 `loadtest/admin` operator CLI，用公开 gRPC 完成 approve /
  reject / get / list，输出低敏 JSON，不读取私表。
- `admin-service` 已新增第一条真实下游公开 API adapter：非 `CRITICAL` 的
  `CONFIG_PUBLISH` 可在配置 `NEXUSIM_CONTROL_PLANE_GRPC_ADDR` 后由
  operation-worker 调 `control-plane-service.PublishConfigVersion`；critical 操作
  仍走 workflow。

## 下一步

- 默认继续 `admin-service` 的 `Create -> operator approve -> operation-worker ->
  control-plane` 真实进程 smoke，或继续补更多下游公开 admin API adapter。
- 也可以继续 `vector-index-service` embedding worker / rebuild worker / outbox relay。
- 也可以继续 notification SMTP / SMS / APNs / FCM adapter 或 bounce-suppression。

## 硬边界

- 不一次性 promotion 全部 future 服务。
- 不把媒体二进制塞回 message-service。
- 不把 identity 局部 webhook / SMTP 扩成完整 notification-service 前的生产承诺。
- 不让 admin / control-plane / workflow 直接改其它服务私有表。
- 不让 model-gateway / vector-index / knowledge-ingestion 绕过 retrieval /
  policy / EvidencePack 边界。
- 不回滚用户已有修改。
- 小改跑 focused checks；涉及 service-registry / Docker / compose / proto /
  migration / 安全边界时再扩大门禁。

## 文档路由

- 当前阶段背景：`docs/runbook/current-brief.md`
- 剩余待办：`docs/runbook/remaining-goals.md`
- 服务入口：`docs/runbook/service-briefs/<service>.md`
- 新发现待办写入 `docs/runbook/remaining-goals.md`
