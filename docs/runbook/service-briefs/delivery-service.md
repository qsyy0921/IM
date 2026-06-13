# delivery-service

## 当前状态

- 已有 timeline projection、durable `user_inbox`、`PullInbox`、`AckDelivery`、`delivery_outbox` relay。
- 是 push-gateway 的可靠事实源。
- 不要求 push-gateway 持久化消息或 ACK cursor。

## 后续

- Projection DLQ / repair、更多 delivery event 消费方。
