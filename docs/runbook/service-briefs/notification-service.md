# notification-service

状态：future / SDD v0.1 draft pending。当前不得创建
`services/notification-service` 目录，直到完成 ADR 或 stage switch。

定位：统一通知服务，负责 email、SMS、APNs / FCM、系统通知、模板、
bounce handling、provider retry 和通知审计。

边界：

- identity-service 的 webhook / SMTP challenge sender 只是局部能力，不是完整通知平台。
- notification-service 拥有 template、delivery attempt、provider response class 和 bounce 状态。
- 业务服务只能写 notification request / outbox，不直接耦合 provider SDK。
- 不持久化验证码明文、token 明文或 provider response body。

第一切片建议：

- 先接 identity challenge delivery request，保留现有稳定 public error。
- PostgreSQL delivery audit + retry / DLQ skeleton。
- provider adapter 先做 webhook / SMTP，后续再接 SMS / APNs / FCM。
