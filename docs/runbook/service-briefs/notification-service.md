# notification-service

状态：product-active / SDD v0.1 draft 已存在 / stage-switch review passed /
first implementation slice completed。第一版 proto、migration、六层 skeleton、
`grpc` runtime、Docker、Prometheus 和 Grafana 覆盖已落，并已通过 focused checks /
完整 `check-local`。`notification_outbox -> im.notification.events` 最小 relay、
Kafka protobuf schema、runtime mode、service-registry / compose wiring、trigger
builder 单测和真实 PostgreSQL relay 集成测试已补。当前不宣称 provider-grade
email / SMS / APNs / FCM。
`notification_outbox -> im.notification.events` 真实 Kafka smoke 已通过，报告见
`docs/runbook/loadtest/notification-service/loadtest-report-20260620-notification-outbox-relay-smoke.md`。
第一版 `delivery-worker` 和 noop provider adapter 已落地，真实本地 delivery smoke
已通过，报告见
`docs/runbook/loadtest/notification-service/loadtest-report-20260620-notification-delivery-worker-smoke.md`。
第一版 webhook provider adapter 已落地，支持低敏 HTTP provider envelope、
provider idempotency header、provider message id hash 和稳定失败分类；不发送
destination hash、secret payload 或 provider response body。

Stage-switch 记录：`docs/runbook/stage-switch/notification-service.md`。

定位：统一通知服务，负责 email、SMS、APNs / FCM、系统通知、模板、
bounce handling、provider retry 和通知审计。

边界：

- identity-service 的 webhook / SMTP challenge sender 只是局部能力，不是完整通知平台。
- notification-service 拥有 template、delivery attempt、provider response class 和 bounce 状态。
- 业务服务只能写 notification request / outbox，不直接耦合 provider SDK。
- 不持久化验证码明文、token 明文或 provider response body。

第一切片建议：

- 具体边界见 `docs/sdd/notification-service.md`。
- `CreateNotificationRequest` / `GetNotificationStatus` / `CancelNotificationRequest`。
- PostgreSQL request + delivery attempt + outbox + suppression state。
- provider adapter 先做 webhook / SMTP fake/real boundary，后续再接 SMS / APNs / FCM。

下一步：

- 继续做 SMTP / SMS / APNs / FCM provider adapter 边界，或先按 promotion plan
  转入 `audit-service`。
- 后续再做 bounce / suppression worker、provider redrive / audit 产品化。
