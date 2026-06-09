# push-gateway cross-instance resume smoke

## 结论

本轮验证了 `push-gateway` Redis-backed cross-instance resume 的真实进程链路。

结果通过：客户端第一次连接 `push-gateway-ws` 收到 `delivery.notify` 后，在 ACK 前断开；随后携带同一个服务端签发的 `resume_token` 和本地 `last_received=1` 重连到另一个 WebSocket gateway，`push-gateway-ws-reconnect` 从 Redis-backed resume buffer 重放了同一条 `delivery.notify`。客户端随后仍用 `PullInbox` 校准 durable inbox，并通过 `delivery.ack` 推进 `delivery-service` 的 device cursor。

这条链路证明：

```text
WebSocket gateway A receives first client session
-> delivery consumer gateway consumes im.delivery.events
-> Redis route / Redis resume buffer stores lightweight delivery.notify
-> client reconnects to WebSocket gateway B with same resume_token
-> gateway B replays the same delivery.notify
-> PullInbox verifies durable user_inbox
-> delivery.ack -> AckDelivery -> delivery.ack.ok
```

这不是生产级可靠投递结论。Redis resume 是短时体验优化；可靠投递事实仍由 `delivery-service` 的 `user_inbox`、`device_delivery_cursors` 和 outbox 保障。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `fa089e9` |
| full commit | `fa089e9eae78af5dbd05bf79d61c407ba82e7138` |
| dirty | `false` |
| 原始结果 | `H:\NexusIM\loadtest-results\push-gateway-cross-instance-resume-smoke-20260609-clean\pushgateway-summary.json` |
| scenario | `cross-instance-resume` |
| route backend | `redis` |
| Redis key prefix | `nexusim:push:push-gateway-cross-instance-resume-smoke-20260609-clean` |
| first push URL | `ws://127.0.0.1:11598` |
| reconnect push URL | `ws://127.0.0.1:11599` |
| first gateway id | `push-ws-push-gateway-cross-instance-resume-smoke-20260609-clean` |
| reconnect gateway id | `push-ws-reconnect-push-gateway-cross-instance-resume-smoke-20260609-clean` |
| consumer gateway id | `push-consumer-push-gateway-cross-instance-resume-smoke-20260609-clean` |

## 方法

使用脚本：

```powershell
. .\tools\go-env.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario cross-instance-resume `
  -RouteBackend redis `
  -RunName push-gateway-cross-instance-resume-smoke-20260609-clean
```

脚本启动真实本地链路：

```text
conversation-service grpc
message-service grpc
message-service outbox relay
delivery-service timeline consumer
delivery-service grpc
delivery-service outbox relay
push-gateway-ws on 127.0.0.1:11598
push-gateway-ws-reconnect on 127.0.0.1:11599
push-gateway-consumer
Redis / Kafka / PostgreSQL
```

关键步骤：

1. 客户端先连接 `push-gateway-ws`，完成 `client.hello -> server.hello`，记录服务端签发的 `resume_token`。
2. 通过真实 `CreateMemberChange(JOIN)` 加入 receiver，boundary seq 为 `1`。
3. 发送 1 条真实 `SendMessage`，等待首次 WebSocket 收到 `delivery.notify`。
4. 在 ACK 前关闭第一次 WebSocket。
5. 客户端带同一个 `resume_token` 和 `last_received=1` 重连到 `push-gateway-ws-reconnect`。
6. 等待 replay 的 `delivery.notify`，断言它和原始 notify 的 `event_id`、`message_id`、`conversation_seq` 完全一致。
7. 调用 `PullInbox` 校准 durable inbox，再通过 WebSocket 发送 `delivery.ack` 并验证 cursor。

## 关键数据

| 指标 | 结果 |
| --- | --- |
| success | `true` |
| original session_id | `sess_7b8c28973eb2444c4de8f33e158677ed` |
| reconnect session_id | `sess_d5e9fe75fdf4b7a81e258001041a59f4` |
| resume_token | `resume_a2ad141ed6c67b5bea88efa37ad8a783` |
| last_received_seq | `1` |
| notify conversation_seq | `2` |
| original / replay event_id | `evt_delivery_inbox_fa5018677bcb99679be262bda8f5019b` |
| message_id | `msg_4ec3e453-42e3-475f-9d83-d87798d9eae5` |
| source_event_id | `d7c04a78-674c-464d-99dc-1aa5eaaf2357` |
| reconnect gateway `redis_resume_replay_count` | `1` |
| reconnect gateway `redis_resume_miss_count` | `0` |
| reconnect gateway `redis_resume_permission_denied_count` | `0` |
| reconnect gateway local `resume_buffer_replay_count` | `0` |
| PullInbox item count | `1` |
| PullInbox max seq | `2` |
| delivery.ack.ok | `last_received_seq=2` |
| device cursor last_received_seq | `2` |
| delivery_outbox published | `2` |
| delivery_outbox pending | `0` |
| delivery_outbox DLQ | `0` |

`delivery_outbox published=2` 包含 1 条 inbox created 事件和 1 条 ACK recorded 事件。

## 如何排查

本轮要排除两个误判。

第一，不能把 `PullInbox` 成功误认为 resume replay 成功。因此先看 replay 本身：

```text
original_notify.event_id == replayed_notify.event_id
original_notify.message_id == replayed_notify.message_id
original_notify.conversation_seq == replayed_notify.conversation_seq
reconnect gateway redis_resume_replay_count: 0 -> 1
reconnect gateway redis_resume_miss_count: 0
reconnect gateway local resume_buffer_replay_count: 0
```

这些证据说明重连 gateway 不是靠本机 in-memory buffer replay，而是命中了 Redis-backed resume buffer。

第二，不能把 Redis resume 当成可靠投递事实。因此还要看 delivery durable path：

```text
PullInbox item_count=1
PullInbox max_seq=2
delivery.ack.ok last_received_seq=2
device cursor last_received_seq=2
delivery_outbox pending=0
delivery_outbox DLQ=0
```

如果 Redis resume miss，但 PullInbox 和 ACK 成功，结论应是“在线体验退化，可靠投递未丢”。当前 clean run 中 replay 和 durable fallback 两层都成立。

## 面试讲法

可以这样讲：

> 我把 WebSocket gateway 和 Kafka delivery consumer 拆成不同进程，并用 Redis route 做在线连接定位；随后又把 resume token 的轻量 notify buffer 放到 Redis。这样客户端第一次连在 gateway A，断线后重连到 gateway B，也能用同一个服务端签发的 resume token 重放短时 notify。这个能力只提升在线体验，可靠性仍然由 delivery-service 的 durable inbox 和 ACK cursor 保证。

这体现了两个边界：

- 在线层：Redis route / Redis resume / WebSocket notify，允许 best-effort。
- 可靠层：`user_inbox` / `PullInbox` / `AckDelivery` / cursor，必须 durable。

## 限制

- 本轮只覆盖单用户、单设备、单条消息。
- 本轮不覆盖 Redis HA / Sentinel / Cluster。
- 本轮不覆盖 Redis resume TTL 过期后的真实进程路径。
- 本轮不覆盖多条 buffer gap、部分 replay + buffer_miss、高并发重连。
- delivery-consumer gateway 当前不单独暴露 HTTP debug metrics，本轮没有直接记录 consumer 侧 `redis_resume_append_count`；reconnect gateway 的 `redis_resume_replay_count=1`、原始/replay notify 完全一致和 durable path 成立，足以证明最小 cross-instance replay 链路。
- `/debug/metrics` 仍是本地 smoke 调试端点，不是生产 Prometheus 指标。
