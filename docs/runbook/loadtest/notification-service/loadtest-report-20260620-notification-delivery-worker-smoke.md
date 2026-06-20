# notification-service delivery worker smoke

日期：2026-06-20

目标：验证 `CreateNotificationRequest -> delivery-worker -> noop provider -> notification_outbox -> im.notification.events` 的本地最小闭环。

命令：

```powershell
. .\tools\go-env.ps1
.\loadtest\notification\run-local-smoke.ps1 -WithDeliveryWorker
```

原始 summary：

```text
H:\NexusIM\loadtest-results\notification-service-delivery-worker-smoke-20260620-204035\notification-outbox-relay-summary.json
```

结果：

```text
success=true
request_status=DELIVERED
delivery_attempt_count=1
notification_outbox total=2
notification_outbox accepted=1
notification_outbox succeeded=1
notification_outbox pending=0
notification_outbox published=2
notification_outbox dlq=0
notification_event_count=2
```

Kafka readback：

```text
notification.request.accepted.v1
notification.delivery.succeeded.v1
```

结论：

`notification-service` 第一版 delivery worker 已能从 PostgreSQL claim ready request，经 noop provider 标记 `DELIVERED`，写入 delivery attempt 和 `notification.delivery.succeeded.v1` outbox，并由 outbox relay 发布到 Kafka typed protobuf topic。

边界：

- 本轮使用 `NEXUSIM_NOTIFICATION_PROVIDER_MODE=noop`，不代表真实 email / SMS / APNs / FCM provider 已完成。
- 本轮是本地单 broker / 单 worker smoke，不证明 Kafka HA、provider SLA、bounce handling 或 provider-grade retry / redrive。
- raw destination、destination hash、secret payload、provider response body 不应出现在 outbox / Kafka event / summary。
