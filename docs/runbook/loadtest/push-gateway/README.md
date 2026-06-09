# push-gateway Loadtest / Smoke Index

本文是 `push-gateway` 验证报告入口。当前已完成六层骨架、WebSocket frame codec、in-memory session registry、delivery event consumer、`server.pong`、`delivery.notify` 和 `delivery.ack.ok` 的单元 / 集成测试；真实进程 full smoke 仍待执行。

## 当前验证目标

第一阶段只验证在线通知链路，不做 WebSocket 容量极限：

```text
delivery_outbox
-> delivery-service outbox-relay
-> Kafka im.delivery.events
-> push-gateway delivery event consumer
-> online WebSocket client receives delivery.notify
-> client PullInbox reads durable user_inbox
-> client sends delivery.ack frame
-> push-gateway calls delivery-service AckDelivery
-> client receives delivery.ack.ok
```

必须证明：

- push-gateway 消费的是 `im.delivery.events`，不是 `conversation.timeline.events`。
- `delivery.notify` 是轻量唤醒信号，不是 message 事实源。
- 客户端展示和本地持久化以 `PullInbox` 返回为准。
- ACK 仍由 `delivery-service AckDelivery` 推进 cursor。
- `delivery.ack` 成功必须有 `delivery.ack.ok`，失败必须返回稳定 error frame。
- push-gateway 不直接读写 `message_log`、`conversation_members`、`user_inbox`、`device_delivery_cursors`。

当前最小运行模式：

```text
NEXUSIM_PUSH_GATEWAY_MODE=all
NEXUSIM_PUSH_WS_ADDR=0.0.0.0:10496
NEXUSIM_DELIVERY_GRPC_ADDR=127.0.0.1:10497
NEXUSIM_KAFKA_BROKERS=localhost:9092
NEXUSIM_DELIVERY_EVENTS_TOPIC=im.delivery.events
NEXUSIM_PUSH_CONSUMER_GROUP=nexusim-push-gateway-smoke
```

`all` 模式只用于第一阶段本地 smoke：WebSocket handler 和 `im.delivery.events` consumer 共享同一个 in-memory session registry。多实例前必须改用 Redis route。

## 报告位置

报告 Markdown 保存在仓库内：

```text
E:\development\IM\docs\runbook\loadtest\push-gateway\
```

推荐命名：

```text
loadtest-report-YYYYMMDD-push-gateway-smoke.md
```

中大型原始数据、长日志和趋势图保存到机械盘：

```text
H:\NexusIM\loadtest-results
```

小规模 smoke 的轻量 summary 可以临时放在：

```text
E:\development\IM\loadtest\results
```

## 第一阶段不做

- 不做十万级 WebSocket 长连接压测。
- 不打满 Win-Mac 2.5Gbps 链路。
- 不重新做 message-service 硬件矩阵。
- 不把短时 resume buffer 当作 durable inbox。
- 不把 push smoke 表述为生产容量结论。

## 面试可讲点

`push-gateway` 的价值不是“直接把消息正文推给客户端”，而是把在线通道放在 durable delivery 之后：

```text
message-service 写消息事实
-> delivery-service 写 user_inbox / delivery_outbox
-> push-gateway 只做在线唤醒
-> 客户端 PullInbox + AckDelivery 完成可靠投递
```

这样断线、重连、成员边界、ACK 丢失和服务重启都可以由 `delivery-service` 的 durable inbox / cursor 兜底。
