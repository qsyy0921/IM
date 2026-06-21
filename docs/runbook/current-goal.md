# NexusIM Current Goal

本文件只维护当前可执行目标。Codex 目标框短 prompt 见根目录 `prompt.md`；
阶段背景看 `current-brief.md`，详细进度看 `development-progress.md` 和 service brief。

## Active Slice

```text
future platform / product services promotion
```

目标服务：

```text
media / notification / audit / admin / control-plane / presence
model-gateway / workflow / knowledge-ingestion / vector-index
```

## 当前状态

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
  它只输出低敏 refs / version / status，不输出 payload / reason 原文。

## 下一步优先级

1. 继续 workflow compensation adapter / instruction approval UI / ops 管理；
   当前已有本地 workflow get / decision / instruction list CLI，后续可接审批 UI /
   external approval binding。
2. 继续明确其它下游公开 admin API adapter。
3. 在镜像可用后补 vector-index focused pgvector smoke；后续再接 Milvus /
   OpenSearch backend、provider repair 和真 provider backfill smoke。
4. 可继续 notification SMTP / SMS / APNs / FCM adapter 或 bounce-suppression。
5. 新发现待办写入 `docs/runbook/remaining-goals.md`。

## 工作方式

- 按服务小切片闭环：代码、必要测试、文档一起收。
- 当前任务涉及哪个服务，只读对应 service brief 和必要 SDD 章节。
- 不一次性 promotion 全部 future 服务，不铺空目录。
- 小改跑 focused checks；proto、migration、跨服务 adapter、安全边界或提交推送前再扩大门禁。

## 硬边界

- 不把媒体二进制塞回 message-service。
- 不把 identity 局部 webhook / SMTP 扩成完整 notification-service 的生产承诺。
- admin / control-plane / workflow / audit 之间只能走公开 API、事件或明确 port。
- model / vector / ingestion 不得绕过 retrieval、policy、EvidencePack、approval 和 audit。
- 不回滚用户已有修改。

## 文档路由

- 当前阶段背景：`docs/runbook/current-brief.md`
- 剩余待办：`docs/runbook/remaining-goals.md`
- 服务入口：`docs/runbook/service-briefs/<service>.md`
- 总览：`docs/runbook/development-progress.md`
