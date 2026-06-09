# push-gateway Win-Mac cross-instance resume smoke

## 结论

本轮验证了 `push-gateway` Redis-backed cross-instance resume 在 Windows / Mac 双机有线直连拓扑下的真实进程链路。

结果通过：客户端第一次连接 Mac Docker 中的 WebSocket gateway，收到 `delivery.notify` 后在 ACK 前断开；随后携带同一个服务端签发的 `resume_token` 和本地 `last_received=1` 重连到 Windows 上的另一个 WebSocket gateway，Windows gateway 从 Redis-backed resume buffer 重放了同一条 `delivery.notify`。客户端随后仍用 `PullInbox` 校准 durable inbox，并通过 `delivery.ack` 推进 `delivery-service` 的 device cursor。

这条链路证明：

```text
Mac push-gateway WS receives first client session
-> Windows push-gateway delivery-consumer consumes im.delivery.events
-> Redis route / Redis resume buffer stores lightweight delivery.notify
-> client reconnects to Windows push-gateway WS with same resume_token
-> Windows reconnect gateway replays the same delivery.notify
-> PullInbox verifies durable user_inbox
-> delivery.ack -> AckDelivery -> delivery.ack.ok
```

这不是生产级可靠投递或容量结论。Redis route / Redis resume 只提升在线体验；可靠投递事实仍由 `delivery-service` 的 `user_inbox`、`device_delivery_cursors` 和 outbox 保证。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `b8d8f92` |
| full commit | `b8d8f92287675c48491a7995b1a7724687b6ea5f` |
| dirty | `false` |
| 原始结果 | `H:\NexusIM\loadtest-results\push-gateway-win-mac-cross-instance-resume-20260609\pushgateway-summary.json` |
| scenario | `cross-instance-resume` |
| route backend | `redis` |
| Redis key prefix | `nexusim:push:push-gateway-win-mac-cross-instance-resume-20260609` |
| first push URL | `ws://172.31.50.2:11598` |
| reconnect push URL | `ws://127.0.0.1:11599` |
| first push metrics URL | `http://172.31.50.2:11598/debug/metrics` |
| reconnect push metrics URL | `http://127.0.0.1:11599/debug/metrics` |
| consumer metrics URL | `http://127.0.0.1:11600/debug/metrics` |
| first gateway id | `push-mac-ws-push-gateway-win-mac-cross-instance-resume-20260609` |
| reconnect gateway id | `push-win-reconnect-push-gateway-win-mac-cross-instance-resume-20260609` |
| consumer gateway id | `push-win-consumer-push-gateway-win-mac-cross-instance-resume-20260609` |
| Windows wired IP | `172.31.50.1` |
| Mac wired IP | `172.31.50.2` |

## 方法

使用脚本：

```powershell
. .\tools\go-env.ps1
.\tools\run-win-mac-push-smoke.ps1 `
  -Scenario cross-instance-resume `
  -RunName push-gateway-win-mac-cross-instance-resume-20260609 `
  -MacRunMode docker `
  -WindowsDeliveryRunMode docker
```

脚本启动真实双机链路：

```text
Windows:
  PostgreSQL / Kafka / Redis
  conversation-service grpc
  message-service grpc
  message-service outbox relay
  delivery-service timeline consumer
  delivery-service grpc in Docker, exposed on 172.31.50.1:11597
  delivery-service outbox relay
  push-gateway delivery-consumer, metrics on 127.0.0.1:11600
  push-gateway ws-reconnect on 127.0.0.1:11599
  pushgateway-smoke runner

Mac:
  Docker push-gateway ws on 172.31.50.2:11598
  Redis route points to 172.31.50.1:6379
  AckDelivery points to 172.31.50.1:11597
```

关键步骤：

1. Windows 通过有线 `172.31.50.2` 同步 Mac 专用 smoke checkout 和 Docker image。
2. Mac Docker 启动首连 `push-gateway ws`，使用 Windows Redis route 和 Windows delivery-service gRPC。
3. Windows 启动 `push-gateway delivery-consumer`，消费 `im.delivery.events`，并通过 Redis route / Redis resume 写入首连 session 的 online state。
4. 客户端先连接 Mac `ws://172.31.50.2:11598`，完成 `client.hello -> server.hello`，记录服务端签发的 `resume_token`。
5. 通过真实 `CreateMemberChange(JOIN)` 加入 receiver，boundary seq 为 `1`。
6. 发送 1 条真实 `SendMessage`，等待 Mac WebSocket gateway 收到首次 `delivery.notify`。
7. 在 ACK 前关闭第一次 WebSocket。
8. 客户端带同一个 `resume_token` 和 `last_received=1` 重连到 Windows `ws://127.0.0.1:11599`。
9. 等待 replay 的 `delivery.notify`，断言它和原始 notify 的 `event_id`、`message_id`、`conversation_seq` 完全一致。
10. 调用 `PullInbox` 校准 durable inbox，再通过 WebSocket 发送 `delivery.ack` 并验证 cursor。

## 关键数据

| 指标 | 结果 |
| --- | --- |
| success | `true` |
| original session_id | `sess_5384b14d28051af356534c8fb4f393b4` |
| reconnect session_id | `sess_366b70e0758c6b72360ac14b49f5f8a4` |
| resume_token | `resume_725c872c9c86e905ef097284e068f086` |
| last_received_seq | `1` |
| notify conversation_seq | `2` |
| original / replay event_id | `evt_delivery_inbox_2429cbc442a1a3891ef292f6e6415516` |
| message_id | `msg_13080715-312d-4def-8920-3c9a734d4576` |
| source_event_id | `7bd19303-198a-40fe-a35b-45333c5c7736` |
| consumer gateway `redis_resume_append_count` | `1` |
| consumer gateway `redis_route_remote_matched_sessions` | `1` |
| consumer gateway `redis_route_remote_publish_call_count` | `1` |
| consumer gateway `redis_route_remote_publish_error_count` | `0` |
| reconnect gateway `redis_resume_replay_count` | `1` |
| reconnect gateway `redis_resume_miss_count` | `0` |
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

本轮要排除三个误判。

第一，不能把普通 Win-Mac online route 成功误认为 cross-instance resume 成功。因此要看首连和重连是否真的落在不同 gateway：

```text
push_url = ws://172.31.50.2:11598
reconnect_push_url = ws://127.0.0.1:11599
push_ws_gateway_id = push-mac-ws-...
push_reconnect_gateway_id = push-win-reconnect-...
original session_id != reconnect session_id
resume_token 保持相同
```

第二，不能把 `PullInbox` 成功误认为 resume replay 成功。因此先看 replay 本身：

```text
original_notify.event_id == replayed_notify.event_id
original_notify.message_id == replayed_notify.message_id
original_notify.conversation_seq == replayed_notify.conversation_seq
consumer gateway redis_resume_append_count = 1
consumer gateway redis_route_remote_publish_call_count = 1
consumer gateway redis_route_remote_publish_error_count = 0
reconnect gateway redis_resume_replay_count = 1
reconnect gateway redis_resume_miss_count = 0
reconnect gateway local resume_buffer_replay_count = 0
```

这些证据说明 delivery consumer gateway 已把轻量 notify 写入 Redis-backed resume buffer，Windows 重连 gateway 不是靠本机 in-memory buffer replay，而是命中了 Redis-backed resume buffer。

第三，不能把 Redis resume 当成可靠投递事实。因此还要看 delivery durable path：

```text
PullInbox item_count=1
PullInbox max_seq=2
delivery.ack.ok last_received_seq=2
device cursor last_received_seq=2
delivery_outbox pending=0
delivery_outbox DLQ=0
```

如果 Redis resume miss，但 PullInbox 和 ACK 成功，结论应是“在线体验退化，可靠投递未丢”。当前 clean run 中 cross-instance replay 和 durable fallback 两层都成立。

## 面试讲法

可以这样讲：

> 我把 push-gateway 的在线连接和 Kafka delivery consumer 拆成不同实例，并且放到了两台机器上。客户端第一次连到 Mac 上的 WebSocket gateway，Kafka delivery event 在 Windows 的 consumer gateway 被消费；consumer 通过 Redis route 找到 Mac 的 session，并把轻量 notify 写入 Redis-backed resume buffer。客户端断开后重连到 Windows 的另一个 gateway，仍能用同一个服务端签发的 resume token 重放这条 notify。这个能力只提升在线体验，可靠性仍然由 delivery-service 的 durable inbox 和 ACK cursor 保证。

这体现了两个边界：

- 在线层：Redis route / Redis resume / WebSocket notify，允许 best-effort。
- 可靠层：`user_inbox` / `PullInbox` / `AckDelivery` / cursor，必须 durable。

## 限制

- 本轮只覆盖单用户、单设备、单条消息。
- 本轮只把首连 WebSocket gateway 放在 Mac Docker，把重连 gateway 和 consumer gateway 放在 Windows；不是完整多节点调度平台。
- 本轮不覆盖 Redis HA / Sentinel / Cluster。
- 本轮不覆盖 Redis resume TTL 过期后的双机路径。
- 本轮不覆盖跨实例慢连接组合、buffer gap、部分 replay + buffer_miss、高并发重连。
- `/debug/metrics` 仍是本地 smoke 调试端点，不是生产 Prometheus 指标。
- 可靠投递仍以 `delivery-service PullInbox / AckDelivery` 为准，不以 WebSocket notify 是否送达为准。
