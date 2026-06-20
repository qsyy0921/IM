# notification-service smoke

本目录记录 `notification-service` 本地小规模 smoke。原始 summary / log 写入
`H:\NexusIM\loadtest-results`，仓库只保存说明和报告。

## Outbox relay / Kafka smoke

脚本：

```powershell
.\loadtest\notification\run-local-smoke.ps1
```

默认行为：

1. 构建 `services/notification-service/cmd/notification-service` 和
   `loadtest/notification`。
2. 确认 / 创建本轮临时 `im.notification.events.*` Kafka topic。
3. 启动 `NEXUSIM_NOTIFICATION_SERVICE_MODE=grpc` 和
   `NEXUSIM_NOTIFICATION_SERVICE_MODE=outbox-relay` 两个真实进程。
4. runner 通过真实 gRPC 调用 `CreateNotificationRequest`。
5. runner 等待 `notification_outbox` 从 `PENDING` 变成 `PUBLISHED`。
6. runner 从 Kafka topic 读回 typed protobuf `NotificationEvent`，确认：
   - `notification.request.accepted.v1` 已发布；
   - payload oneof 为 `request_accepted`；
   - outbox / Kafka event 不暴露 raw destination、destination hash、secret payload
     或 provider body；
   - `notification_outbox PENDING=0 / PUBLISHED=1 / DLQ=0`。

报告：

- `docs/runbook/loadtest/notification-service/loadtest-report-20260620-notification-outbox-relay-smoke.md`

## Delivery worker / noop provider smoke

脚本：

```powershell
.\loadtest\notification\run-local-smoke.ps1 -WithDeliveryWorker
```

新增行为：

1. 在 outbox relay smoke 基础上额外启动
   `NEXUSIM_NOTIFICATION_SERVICE_MODE=delivery-worker`。
2. delivery worker 使用 `NEXUSIM_NOTIFICATION_PROVIDER_MODE=noop`。
3. runner 等待 request 进入 `DELIVERED`，并确认：
   - delivery attempt 写入一次；
   - `notification.delivery.succeeded.v1` outbox 写入；
   - `notification_outbox PENDING=0 / PUBLISHED=2 / DLQ=0`；
   - Kafka readback 包含 `request_accepted` 和 `delivery_succeeded` 两种 payload。

报告：

- `docs/runbook/loadtest/notification-service/loadtest-report-20260620-notification-delivery-worker-smoke.md`

可选 provider 参数：

```powershell
.\loadtest\notification\run-local-smoke.ps1 -WithDeliveryWorker `
  -ProviderMode webhook `
  -WebhookUrl http://127.0.0.1:18080/notification `
  -WebhookBearerToken local-token
```

`webhook` provider 只发送低敏 envelope / template variables，并使用 provider
idempotency header；provider response body 会被丢弃，`provider_message_id` 只以 hash
形式进入 delivery result。

边界：

- 这是单节点本地 Kafka smoke，不证明 Kafka HA / ISR / 网络分区语义。
- 当前只验证 noop provider smoke 和 webhook HTTP adapter 边界，不验证真实 email /
  SMS / APNs / FCM、bounce 或 suppression。
- raw summary / logs 写入 `H:\NexusIM\loadtest-results`。
