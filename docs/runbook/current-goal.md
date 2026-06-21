# NexusIM Current Goal

本文件只维护当前可执行目标。Codex 目标框短 prompt 见根目录 `prompt.md`；
阶段背景看 `current-brief.md`，详细进度看 `development-progress.md` 和 service brief。

## Active Slice

```text
client platform MVP foundation
```

目标：

```text
Browser + PC + Android client architecture + client BFF contract + reusable client packages
```

## 当前状态

- 用户明确切入客户端平台；本轮从 product services promotion 临时转为
  `client platform MVP foundation`。
- 目标是先按最高标准冻结三端客户端架构，不写临时 demo：浏览器先可运行，PC
  desktop 和 Android 从第一天保留 package / runtime / packaging 边界，三端复用
  `protocol` / `client-core`。
- `clients/` workspace skeleton 已创建并通过 focused validation：`protocol`、
  `client-core`、`web`、`desktop`、`android` 均有明确 package / runtime contract；
  Web 端已有 Vite shell，PC / Android 仍是 packaging/runtime contract，不产出安装包。
- 真实业务语言选择：后端和 client BFF 继续使用 Go；浏览器、PC desktop 和
  Android 的共享协议 / 同步核心 / UI 使用 TypeScript；Tauri 的 Rust、Android
  Kotlin 只作为薄平台桥；Python 只用于 AI worker / eval / 离线工具，不进入
  客户端主链路和后端事实源。
- 客户端只面向 `api-gateway` 和 `push-gateway`；PullInbox 是消息事实源，WebSocket
  是在线唤醒。
- v0.1 SDD 见 `docs/sdd/client-platform.md`，短 brief 见
  `docs/runbook/client-platform.md`。
- 10 个目标服务的 SDD v0.1 draft 已存在，组合 promotion 边界见
  `docs/sdd/future-platform-services.md`。
- 10 个目标服务均已进入 product-active first-stage implementation。
- 最新焦点在 admin / audit / workflow / vector-index 之间补公开 API handoff、
  compensation boundary、provider backend 和 focused smoke。
- `audit-service` 已新增 first-stage `CreateAuditExport` / `GetAuditExport`
  job metadata API；只保存低敏 filter hash / redaction profile / requester refs。
- `admin-service` 已新增 `AUDIT_EXPORT_REQUEST -> audit-service.CreateAuditExport`
  公开 API adapter；不读 audit-service 私有表。
- `audit-service` 已新增 first-stage `admin-consumer`，消费公开
  `im.admin.events` 并映射为低敏 `AppendAuditRecord`；Kafka offset 只在 append
  成功后提交，持久 ingestion checkpoint / rewind 仍是后续项。
- `workflow-service` 已新增 first-stage
  `ListWorkflowCompensationInstructions` 公开查询 API，按 workflow 返回低敏
  compensation instruction refs / version / status；不读 admin-service 私表。
- `loadtest/workflow` 已新增 first-stage workflow operator CLI，通过 workflow-service
  公开 gRPC get workflow、record decision、查询 compensation instruction metadata；
  它只输出低敏 refs / version / status，不输出 payload / reason 原文，并在
  `record-decision` 本地拒绝明显敏感的 decider / policy / reason / evidence ref；
  `-decision-manifest` 可作为 first-stage external approval binding；本地 writer /
  validator 可生成和校验仓库外低敏 decision manifest。
- 已新增本地 workflow compensation instruction manifest writer / validator /
  self-test，用于生成和校验仓库外低敏 control-plane rollback instruction JSON；
  manifest 只保存 workflow / payload hash / config target / operator / reason ref，
  不保存 rollback payload、operator reason 原文或本机文件路径。
- `workflow-service compensation-instruction-import` 已纳入机器可读
  `repair-operators.catalog.json`，可进入本地 approval request / decision /
  invocation 链路；导入 instruction metadata，不直接执行 rollback mutation。
- approved repair invocation 会在 `workflow-service compensation-instruction-import`
  执行前校验 instruction manifest，只在 summary 中记录 manifest hash / count，
  不输出 manifest 路径、payload ref 文件正文或 operator reason 原文。
- 已新增本地静态 repair approval review page writer，用于把 plan / request /
  decision / invocation / audit bundle 渲染为低敏 HTML 审批页；页面只展示 hash、
  path hash、env key 和 preflight 摘要，不复制 reason、payload、manifest path
  或 evidence 原文。

## 下一步优先级

1. 实现 `api-gateway` client BFF v0.1 endpoints：auth、conversation list、
   messages、PullInbox、ACK、contacts、receipts。
2. 接 Web fetch / WebSocket adapter、local store adapter 和局域网 smoke。
3. 后续按同一 core 接 PC desktop Tauri runner 和 Android runtime shell。
4. 再回到 workflow compensation adapter / instruction approval UI / ops 管理；
   当前已有本地 workflow get / decision / decision manifest / instruction list CLI，
   低敏 compensation instruction manifest 生成 / 校验，以及 catalog-backed import
   approval 链路、invocation preflight 和静态 review page；后续可接正式审批 UI。
5. 继续明确其它下游公开 admin API adapter。
6. 在镜像可用后补 vector-index focused pgvector smoke；后续再接 Milvus /
   OpenSearch backend、provider repair 和真 provider backfill smoke。
7. 可继续 notification SMTP / SMS / APNs / FCM adapter 或 bounce-suppression。
8. 新发现待办写入 `docs/runbook/remaining-goals.md`。

## 工作方式

- 按服务小切片闭环：代码、必要测试、文档一起收。
- 当前任务涉及哪个服务，只读对应 service brief 和必要 SDD 章节。
- 不一次性 promotion 全部 future 服务，不铺空目录。
- 小改跑 focused checks；proto、migration、跨服务 adapter、安全边界或提交推送前再扩大门禁。

## 硬边界

- 不把媒体二进制塞回 message-service。
- 客户端不直接调用内部微服务，不读取任何服务私表。
- 客户端 local store 只做缓存 / 离线队列，不成为服务端事实源。
- 不把 identity 局部 webhook / SMTP 扩成完整 notification-service 的生产承诺。
- admin / control-plane / workflow / audit 之间只能走公开 API、事件或明确 port。
- model / vector / ingestion 不得绕过 retrieval、policy、EvidencePack、approval 和 audit。
- 不回滚用户已有修改。

## 文档路由

- 当前阶段背景：`docs/runbook/current-brief.md`
- 剩余待办：`docs/runbook/remaining-goals.md`
- 服务入口：`docs/runbook/service-briefs/<service>.md`
- 总览：`docs/runbook/development-progress.md`
