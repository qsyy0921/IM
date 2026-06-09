# push-gateway slow-client smoke

## 结论

本轮验证的是 `push-gateway` 单实例慢客户端负向链路，不是 WebSocket 容量压测。

结果通过：当在线 WebSocket 客户端不及时读取、触发 session queue full 后，`push-gateway` 会驱逐该 session，真实数据仍保留在 `delivery-service` 的 durable inbox 中。客户端重连后用本地 cursor / `PullInbox` 能补齐消息，再通过 `delivery.ack` 调用 `delivery-service AckDelivery` 推进 device cursor。

这条链路证明：

```text
slow WebSocket client
-> push-gateway queue full / active close
-> delivery-service user_inbox still complete
-> reconnect + PullInbox recovers messages
-> delivery.ack -> AckDelivery -> delivery.ack.ok
```

## 环境

| 项 | 值 |
| --- | --- |
| commit | `b362dd7` |
| dirty | `false` |
| 原始结果 | `H:\NexusIM\loadtest-results\push-gateway-slow-client-smoke-clean-20260609-182226\pushgateway-summary.json` |
| scenario | `slow-client` |
| slow message count | `128` |
| push session queue size | `1` |
| push write timeout | `1ms` |
| push test write delay | `50ms` |
| push metrics | `http://127.0.0.1:11598/debug/metrics` |

`NEXUSIM_PUSH_TEST_WRITE_DELAY` 只用于本地 smoke 稳定模拟慢网络 / 慢写出，不是生产配置。

## 方法

1. 启动真实本地链路：`conversation-service`、`message-service`、`delivery-service` timeline consumer、`delivery-service` outbox relay、`push-gateway all mode`。
2. WebSocket 客户端完成 `client.hello -> server.hello` 后不主动消费通知，模拟慢客户端。
3. 连续发送 128 条真实 `SendMessage`，每条使用不同 `client_msg_id`，避免幂等 replay 造成假流量。
4. 通过 `push-gateway /debug/metrics` 等待 `session_queue_full_count` 和 `slow_session_evicted_count` 增加。
5. 使用 `delivery-service PullInbox` 验证 receiver 的 durable inbox 能拉到全部 128 条消息。
6. 等待 `delivery_outbox` 追平后，使用同一 `resume_token` 和本地 `last_received` 重连。
7. 通过 WebSocket 发送 `delivery.ack`，确认收到 `delivery.ack.ok`，并检查 `device_delivery_cursors`。

关键点是没有直接调用 in-memory registry，也没有手写 `im.delivery.events`。数据仍通过真实业务链路产生。

## 关键数据

| 指标 | 结果 |
| --- | --- |
| success | `true` |
| first message seq | `2` |
| last message seq | `129` |
| PullInbox item count | `128` |
| PullInbox max seq | `129` |
| cursor last_received_seq | `129` |
| session_queue_full_count | `1` |
| slow_session_evicted_count | `1` |
| connected_sessions after eviction | `0` |
| resume_buffer_stored_frames | `3` |
| delivery_outbox total | `129` |
| delivery_outbox published | `129` |
| delivery_outbox pending | `0` |
| delivery_outbox DLQ | `0` |

`delivery_outbox total=129` 包含 128 条 inbox created 事件和 1 条 ACK recorded 事件。

## 如何排查

最开始只让客户端“不读 WebSocket”并不足以稳定触发 queue full，因为本机 loopback 和 WebSocket 写缓冲太快，服务端能把通知写进系统缓冲，registry 队列不一定满。

为避免把 smoke 变成偶然结果，本轮加入了本地测试用写延迟：

```text
NEXUSIM_PUSH_SESSION_QUEUE_SIZE=1
NEXUSIM_PUSH_WRITE_TIMEOUT=1ms
NEXUSIM_PUSH_TEST_WRITE_DELAY=50ms
```

这样可以稳定制造“delivery event 到达速度快于 WebSocket writer 消费速度”的场景。判断是否真的触发慢连接，不靠日志肉眼判断，而是读取 `/debug/metrics`：

```text
session_queue_full_count=1
slow_session_evicted_count=1
connected_sessions=0
```

随后再用 `PullInbox` 和 cursor 表验证可靠性：

```text
PullInbox item_count=128
PullInbox max_seq=129
cursor_last_received_seq=129
delivery_outbox pending=0
```

所以瓶颈/异常不是 message 写入失败，也不是 delivery 投影丢失，而是在线 WebSocket 连接消费过慢。系统采取的策略是丢弃在线唤醒连接，并把恢复责任交给 durable inbox。

## 面试讲法

可以这样讲：

> push-gateway 不承诺 WebSocket notify 可靠送达，它只做在线唤醒。慢连接被关闭不会丢消息，因为消息投递事实已经落在 delivery-service 的 user_inbox。客户端重连后按本地 cursor 调 PullInbox 补拉，再通过 AckDelivery 推进设备级 cursor。

这个设计把“在线体验”和“可靠投递事实”分开：

- `push-gateway` 负责低延迟在线通知和连接治理。
- `delivery-service` 负责 durable inbox、ACK cursor 和断线补拉。
- 慢客户端不会卡住 Kafka consumer，也不会影响同 user 后续通过 PullInbox 恢复。

## 限制

- 本轮是单实例 `all` mode，不证明 Redis route / 多实例在线路由。
- 本轮不证明 resume buffer TTL，也不证明跨实例 resume。
- 本轮通过测试写延迟稳定制造慢连接，不代表生产默认写路径有该延迟。
- `/debug/metrics` 是当前单实例调试指标，不是最终 Prometheus 体系。
