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

边界：

- 这是单节点本地 Kafka smoke，不证明 Kafka HA / ISR / 网络分区语义。
- 当前只验证 request accepted event publish，不验证 provider worker、真实 email /
  SMS / APNs / FCM、bounce 或 suppression。
- raw summary / logs 写入 `H:\NexusIM\loadtest-results`。
