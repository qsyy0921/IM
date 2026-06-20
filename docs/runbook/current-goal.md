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
media-service
notification-service
audit-service
admin-service
control-plane-service
presence-service
model-gateway
workflow-service
knowledge-ingestion-service
vector-index-service
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

- `docs/sdd/future-platform-services.md` 已冻结组合 promotion 边界。
- `media-service`、`notification-service`、`audit-service`、`control-plane-service`、
  `admin-service`、`presence-service`、`model-gateway`、
  `knowledge-ingestion-service`、`workflow-service` 和 `vector-index-service`
  SDD v0.1 draft 已存在。
- `media-service` stage-switch review 已通过，记录见
  `docs/runbook/stage-switch/media-service.md`。
- `media-service` 已进入 `product-active`，第一版 proto / migration / 六层
  skeleton / cmd runtime / Docker / Prometheus / Grafana 覆盖已落。
- `media-service` focused hardening 已补真实 PostgreSQL repository 集成测试和
  object_key 不出 public response / fake presign URL / outbox payload 的回归门禁。
- `media-service` 最小 gRPC smoke 已通过，报告见
  `docs/runbook/loadtest/media-service/loadtest-report-20260620-media-grpc-smoke.md`。
- `media-service` 已补 `media_outbox -> im.media.events` 最小 outbox relay、
  Kafka protobuf schema、真实 PostgreSQL outbox relay 集成测试和
  `NEXUSIM_MEDIA_SERVICE_MODE=outbox-relay` runtime mode。
- `media-service` outbox relay 真实 Kafka smoke 已通过，报告见
  `docs/runbook/loadtest/media-service/loadtest-report-20260620-media-outbox-relay-smoke.md`。
- `media-service` 第一版 processing worker 已落地，使用本地 mock scanner /
  thumbnail / transcode adapter；真实进程 smoke 已通过，报告见
  `docs/runbook/loadtest/media-service/loadtest-report-20260620-media-processing-worker-smoke.md`。
- `notification-service` stage-switch review 已通过，记录见
  `docs/runbook/stage-switch/notification-service.md`。
- `notification-service` 已进入 `product-active` implementation slice，第一版
  proto / migration / 六层 skeleton / `grpc` runtime / Docker / observability 覆盖已落，
  并已通过 focused checks / 完整 `check-local`；当前能力只覆盖 request 事实源、
  status 查询、cancel 和 accepted outbox。
- `notification-service` 已补 `notification_outbox -> im.notification.events` 最小
  outbox relay、Kafka protobuf schema、runtime mode、service-registry / compose wiring、
  trigger builder 单测和真实 PostgreSQL relay 集成测试。
- `notification_outbox -> im.notification.events` 真实 Kafka smoke 已通过，报告见
  `docs/runbook/loadtest/notification-service/loadtest-report-20260620-notification-outbox-relay-smoke.md`。
- `notification-service` 第一版 delivery worker 和 noop provider adapter 已落地，
  `CreateNotificationRequest -> delivery-worker -> noop provider -> notification_outbox
  -> im.notification.events` 真实本地 smoke 已通过，报告见
  `docs/runbook/loadtest/notification-service/loadtest-report-20260620-notification-delivery-worker-smoke.md`。
- 下一步默认继续 notification provider-grade adapter / bounce-suppression 边界，
  或按 promotion plan 转入 `audit-service` stage-switch。

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
- 未来服务总表：`docs/runbook/service-registry.json`
- 新发现待办写入 `docs/runbook/remaining-goals.md`
