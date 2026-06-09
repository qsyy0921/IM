# push-gateway Multi-device Notify Smoke - 2026-06-09

## 结论

本轮在 clean commit `99efdc3` 跑通了同一用户多设备在线通知 smoke：

```text
delivery_outbox
-> im.delivery.events
-> push-gateway all mode
-> same user / two online devices
-> both WebSocket sessions receive the same delivery.notify
-> both devices send delivery.ack
-> delivery-service records two device cursors
```

这证明当前 `push-gateway` 的 user 级在线唤醒不是只通知单个连接；同一个 `tenant_id + user_id` 下的多个在线 session 会收到同一条 `delivery.inbox_item.created.v1` 对应的 `delivery.notify`。

## 本轮范围

本轮只验证：

- 单实例 `NEXUSIM_PUSH_GATEWAY_MODE=all`。
- 同一 user 的两个 device/session。
- 单条 `TEXT` message。
- 两个 device 都收到 notify，并分别通过 WebSocket ACK。

本轮不验证：

- 多实例 Redis route。
- 真实鉴权。
- resume buffer。
- 慢连接主动关闭。
- WebSocket 容量上限。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `99efdc3e0d5ebde89682dfd0ac8155db01e1e9ba` |
| git dirty | `false` |
| 原始结果 | `H:\NexusIM\loadtest-results\push-gateway-multidevice-smoke-20260609-171614\pushgateway-summary.json` |
| WebSocket | `ws://127.0.0.1:11598` |
| receiver user | `push-user-1` |
| receiver devices | `push-device-1,push-device-2` |

启动链路仍然使用真实本地进程：

```text
conversation-service grpc
message-service grpc
message-service outbox-relay
delivery-service timeline-consumer
delivery-service grpc
delivery-service outbox-relay
push-gateway all mode
```

## 如何执行

执行入口：

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -RunName "push-gateway-multidevice-smoke-$(Get-Date -Format yyyyMMdd-HHmmss)" `
  -ReceiverDeviceIds "push-device-1,push-device-2"
```

runner 做了这些校验：

1. 为同一 receiver user 建立两个 WebSocket 连接，device 分别为 `push-device-1` 和 `push-device-2`。
2. 每个连接都完成 `client.hello -> server.hello`。
3. 调用 `CreateMemberChange(JOIN receiver)`，等待 delivery membership projection。
4. 调用 `SendMessage(owner)`。
5. 等待两个 WebSocket 连接都收到 `delivery.notify`。
6. 校验两个 notify 的 `conversation_seq` 和 `message_id` 都与 `SendMessage` 一致。
7. 调用 `PullInbox` 验证 durable inbox 里有这条消息。
8. 两个 WebSocket 分别发送 `delivery.ack`。
9. 等待两个 device cursor 都推进到同一个 `conversation_seq`。
10. 等待 `delivery_outbox` 清空 PENDING。

## 结果

核心结果：

| 指标 | 值 |
| --- | --- |
| success | `true` |
| member boundary seq | `1` |
| message conversation seq | `2` |
| device count | `2` |
| notify event id | `evt_delivery_inbox_a27d62a8a97630187a283a184c95d658` |
| notify message id | `msg_7e500119-93ae-4f4a-a373-e702b2532b96` |
| PullInbox item count | `1` |
| delivery_outbox_total | `3` |
| delivery_outbox_published | `3` |
| delivery_outbox_pending | `0` |
| delivery_outbox_dlq | `0` |

两个 device 的关键结果：

| device | notify seq | ack ok seq | cursor seq |
| --- | --- | --- | --- |
| `push-device-1` | `2` | `2` | `2` |
| `push-device-2` | `2` | `2` | `2` |

`delivery_outbox_total=3` 的含义：

- 1 条 `delivery.inbox_item.created.v1`。
- 2 条 `delivery.ack.recorded.v1`，分别对应两个 device 的 ACK。

## 排查和验证重点

这次不是重型压测，重点是验证 push-gateway 的 session registry 语义：

- `delivery.inbox_item.created.v1` 是 user 级事件。
- `push-gateway` 的 in-memory registry 按 `tenant_id + user_id` 找到所有在线 session。
- 每个 session 使用 `event_id` 去重，避免同一连接重复收到同一事件。
- 当前 smoke 证明同一 user 的两个 session 都收到 notify，且不会因为第一个 device ACK 影响第二个 device 的 notify/ACK。

如果本轮失败，排查顺序是：

1. 看两个 WebSocket 是否都完成 `server.hello`。
2. 看 `im.delivery.events` 是否有 `delivery.inbox_item.created.v1`。
3. 看 push-gateway consumer group 是否消费到本轮 event。
4. 看 registry 是否按 user 而不是按 device 只命中一个 session。
5. 看 `device_delivery_cursors` 是否为两个 device 分别落 cursor。

## 面试可讲

可以这样描述：

> push-gateway 不按设备单独消费 Kafka，也不直接维护 durable inbox。delivery-service 产出 user 级投递事件后，push-gateway 在自己的在线连接表里查这个 user 当前所有在线 session，并向每个 session 发轻量 `delivery.notify`。每个 device 收到通知后再回源 PullInbox，并通过各自的 WebSocket ACK 推进各自的 device cursor。这样多端在线能同时被唤醒，但可靠投递和 ACK 状态仍然由 delivery-service 负责。

本轮可以支持的结论：

- `push-gateway` 已能覆盖同 user 多 device 的在线唤醒。
- user 级 notify 与 device 级 ACK 没有混在一起。
- 当前仍是单实例 in-memory registry；多实例必须引入 Redis route。

## 剩余风险

- `all` 模式只适合本地单实例 smoke。
- 多实例跨 gateway 的 route 还没有实现。
- WebSocket auth 仍是 mock。
- 慢连接 queue full 仍是 registry 驱逐，不是主动关闭连接。
- resume buffer 尚未实现。

## 下一步

1. 补 slow session active close / `server.resume_hint`。
2. 设计 Redis route，使不同 gateway 实例能找到 user/device session 所在节点。
3. 再做多实例 push smoke，验证 route 后不依赖单进程 in-memory registry。
