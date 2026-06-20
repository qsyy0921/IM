# notification-service

状态：product-active。第一版 proto、migration、六层 skeleton、`grpc` runtime、
Docker、Prometheus / Grafana、outbox relay、delivery worker、noop provider 和
webhook provider 已落。

当前能力：request / status / cancel、accepted outbox、`im.notification.events`、
delivery succeeded event、noop / webhook provider、idempotency header 和 provider
message id hash；不宣称 provider-grade email / SMS / APNs / FCM。

Stage-switch 记录：`docs/runbook/stage-switch/notification-service.md`。

定位：统一通知服务，负责 email、SMS、APNs / FCM、系统通知、模板、
bounce handling、provider retry 和通知审计。

边界：

- identity-service 的 webhook / SMTP challenge sender 只是局部能力，不是完整通知平台。
- notification-service 拥有 template、delivery attempt、provider response class 和 bounce 状态。
- 业务服务只能写 notification request / outbox，不直接耦合 provider SDK。
- 不持久化验证码明文、token 明文或 provider response body。

- `CreateNotificationRequest` / `GetNotificationStatus` / `CancelNotificationRequest`。
- PostgreSQL request + delivery attempt + outbox。
- provider adapter 先做 noop / webhook boundary。

下一步：
- 继续做 SMTP / SMS / APNs / FCM provider adapter，或进入 `audit-service`。
- 后续做 bounce / suppression worker、provider redrive / audit。
