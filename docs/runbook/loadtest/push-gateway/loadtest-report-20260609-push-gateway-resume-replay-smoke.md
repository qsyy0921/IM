# push-gateway resume replay smoke

## 结论

本轮验证的是 `push-gateway` 单实例 in-memory resume buffer 的真实进程 replay 链路，不是 WebSocket 容量压测，也不是跨实例 / Redis-backed resume。

结果通过：客户端第一次在线收到 `delivery.notify` 后，在 ACK 前断开连接；随后携带同一个服务端签发的 `resume_token` 和本地 `last_received=1` 重连，`push-gateway` 从短时 buffer 重放了同一条 `delivery.notify`。客户端随后仍用 `PullInbox` 校准 durable inbox，并通过 `delivery.ack` 推进 `delivery-service` 的 device cursor。

这条链路证明：

```text
original WebSocket delivery.notify
-> disconnect before ACK
-> reconnect with server-issued resume_token + local last_received
-> push-gateway replays buffered delivery.notify
-> PullInbox verifies durable inbox
-> delivery.ack -> AckDelivery -> delivery.ack.ok
```

## 环境

| 项 | 值 |
| --- | --- |
| commit | `80033de` |
| full commit | `80033ded2e8a7c987e790db80bf4e9b58a70b835` |
| dirty | `false` |
| 原始结果 | `H:\NexusIM\loadtest-results\push-gateway-resume-replay-smoke-20260609\pushgateway-summary.json` |
| scenario | `resume-replay` |
| route backend | `memory` |
| push metrics | `http://127.0.0.1:11598/debug/metrics` |

## 方法

1. 启动真实本地链路：`conversation-service`、`message-service`、`delivery-service` timeline consumer、`delivery-service` outbox relay、`push-gateway all mode`。
2. WebSocket 客户端完成 `client.hello -> server.hello`，记录服务端签发的 `resume_token`。
3. 通过真实 `CreateMemberChange(JOIN)` 加入 receiver，boundary seq 为 `1`。
4. 发送 1 条真实 `SendMessage`，等待 WebSocket 收到原始 `delivery.notify`。
5. 在 ACK 前主动关闭 WebSocket，制造“客户端可能未持久化当前 notify”的短断线窗口。
6. 使用同一 `resume_token` 重连，并携带本地 `last_received=1`。
7. 等待重放的 `delivery.notify`，并断言它与原始 notify 的 `event_id`、`message_id`、`conversation_seq` 完全一致。
8. 调用 `PullInbox` 校准 durable inbox，再通过 WebSocket 发送 `delivery.ack` 并验证 cursor。

关键点是没有直接调用 in-memory registry，也没有手写 `im.delivery.events`。数据仍通过真实业务链路产生。

## 关键数据

| 指标 | 结果 |
| --- | --- |
| success | `true` |
| original session_id | `sess_3615dd172ff2a1280d932680391d93ac` |
| reconnect session_id | `sess_763080efab9a56cbb840364110d3d040` |
| resume_token | `resume_9d2dd5017f7511d397a28782a4bb94c3` |
| last_received_seq | `1` |
| notify conversation_seq | `2` |
| original / replay event_id | `evt_delivery_inbox_87869fbcf12b480b78a600ff2181c998` |
| message_id | `msg_22c8949e-650f-497e-9228-8c523527df31` |
| resume_buffer_stored_frames before replay | `1` |
| resume_buffer_replay_count after replay | `1` |
| resume_buffer_miss_count after replay | `0` |
| PullInbox item count | `1` |
| cursor last_received_seq | `2` |
| delivery_outbox published | `2` |
| delivery_outbox pending | `0` |
| delivery_outbox DLQ | `0` |

`delivery_outbox published=2` 包含 1 条 inbox created 事件和 1 条 ACK recorded 事件。

## 如何排查

本轮要证明的不是“客户端能重新拉到消息”，而是“短断线窗口内 push-gateway 确实做了 buffer replay”。因此排查分成两层。

第一层验证 replay 本身：

```text
original_notify.event_id == replayed_notify.event_id
original_notify.message_id == replayed_notify.message_id
original_notify.conversation_seq == replayed_notify.conversation_seq
resume_buffer_replay_count: 0 -> 1
resume_buffer_miss_count: 0
```

第二层验证可靠兜底仍在 delivery-service：

```text
PullInbox item_count=1
PullInbox max_seq=2
delivery.ack.ok last_received_seq=2
device cursor last_received_seq=2
delivery_outbox pending=0
```

如果第一层失败但第二层成功，说明 online resume 体验退化，但 durable delivery 没丢。当前 clean run 两层都成立。

## 面试讲法

可以这样讲：

> push-gateway 的 resume buffer 是短时体验优化，不是可靠存储。客户端短断线后，可以用服务端签发的 resume token 和本地 last_received 重连；如果 buffer 覆盖缺口，gateway 会重放轻量 notify。无论 replay 是否命中，客户端最终都以 delivery-service 的 PullInbox 校准事实，并通过 AckDelivery 推进设备 cursor。

这个设计有两个好处：

- 在线体验上，短断线不一定需要完整回源扫描。
- 可靠性上，resume buffer 丢失、过期或跨实例不命中也不会丢消息，因为 `user_inbox` 才是 durable 投递事实。

## 限制

- 本轮只验证单实例 `memory` route backend，不验证 Redis route / 跨实例 resume。
- 本轮不验证 resume buffer TTL 过期后的 `buffer_miss` 真实进程路径。
- 本轮只发送 1 条消息，不覆盖多条 buffer gap、部分 replay + buffer_miss 或高并发重连。
- `/debug/metrics` 仍是本地 smoke 调试端点，不是生产 Prometheus 指标。
