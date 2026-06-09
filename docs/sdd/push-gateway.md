# NexusIM push-gateway SDD v0.1

状态：Draft

本文定义 `push-gateway` 的第一条可编码切片：维护 WebSocket 在线连接，消费 `im.delivery.events`，把 delivery-service 已经落库的投递事件通知给在线客户端，并在重连时协调客户端回源 `PullInbox`。它不是消息事实源，也不是 durable inbox。

## 1. 服务定位

`push-gateway` 拥有在线连接和短时会话状态：

- WebSocket session；
- device connection route；
- heartbeat / idle timeout；
- per-session short resume buffer；
- online push backpressure state。

职责：

- 接收 Web / Desktop 客户端 WebSocket 连接。
- 校验 auth context，绑定 `tenant_id / user_id / device_id / session_id`。
- 消费 Kafka `im.delivery.events`。
- 对在线设备发送轻量通知 frame，提示客户端回源 `delivery-service PullInbox`。
- 处理客户端 ACK frame 的协议适配，调用 `delivery-service AckDelivery`。
- 在短断线窗口内提供 best-effort resume buffer。

不负责：

- 不写 `user_inbox`。
- 不修改 `device_delivery_cursors`，只能调用 `delivery-service AckDelivery`。
- 不直接读取 `message-service`、`conversation-service`、`delivery-service` 的内部表。
- 不分配 conversation seq。
- 不决定成员可见性。
- 不承载完整 message payload 作为事实源。
- 不直接 publish 业务 Kafka 事件。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游客户端 | Web / Desktop Client | WebSocket connect、heartbeat、resume、ACK frame |
| 上游事件 | Kafka `im.delivery.events` | 消费 delivery notification / ack 事件 |
| 同步依赖 | delivery-service gRPC | `PullInbox`、`AckDelivery` |
| 同步依赖 | auth / identity provider | 第一阶段可用本地 mock，生产需校验 token 和 device |
| 临时状态 | Redis route / in-memory registry | 连接路由、在线 session、短时 resume buffer |
| 下游客户端 | Web / Desktop Client | `delivery.notify`、`server.resume_hint`、错误 frame |

第一阶段本地实现可以只支持单进程 in-memory connection registry。多实例部署前必须接入 Redis route，避免 Kafka consumer 在某个实例收到事件但目标连接在另一个实例。

## 3. 六层 DDD 包结构

```text
services/push-gateway/
  cmd/push-gateway
  internal/
    api/
    app/
    domain/
    infrastructure/
    types/
    trigger/
```

| 层 | 本服务内容 |
| --- | --- |
| `api` | WebSocket handler、frame decode/encode、HTTP health/debug |
| `app` | `ConnectSessionUseCase`、`HandleClientFrameUseCase`、`NotifyDeliveryUseCase`、`ResumeSessionUseCase` |
| `domain` | session 状态机、heartbeat、resume buffer、slow connection eviction、frame ordering |
| `infrastructure` | Kafka delivery event consumer、delivery-service gRPC client、Redis route adapter、clock/logger |
| `types` | AuthContext、Frame、Command、DTO、错误码 |
| `trigger` | Kafka consumer worker、session cleanup ticker、heartbeat timeout scanner |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `PushSession` | 一个 WebSocket 连接 | 同一连接只能绑定一个 tenant/user/device/session |
| `DeviceRoute` | device 到当前 gateway instance / session 的路由 | Redis route 必须带 TTL，断线或 timeout 后删除 |
| `DeliveryNotification` | 从 `im.delivery.events` 转成客户端通知 | 必须包含 delivery event id 和 conversation seq，不包含完整 message fact |
| `ResumeBuffer` | 短断线 best-effort buffer | 只提升体验，丢失后必须回退到 `PullInbox` |
| `ClientAckFrame` | 客户端已持久化后的 ACK | 必须转发到 `delivery-service AckDelivery`，push-gateway 不直接写 cursor |
| `ConnectionBackpressure` | 慢连接或发送队列保护 | 队列超限时降级 / 断开，不阻塞 Kafka consumer 无限堆积 |

## 5. 同步 API / WebSocket 协议

第一阶段入口：

```text
GET /ws?token=...&device_id=...
```

后续可以由 api-gateway 做 token 交换，push-gateway 只接收已签名的 gateway token。

### 5.1 Client -> Server frames

`hello`：

```json
{
  "op": "client.hello",
  "request_id": "req_01",
  "device_id": "dev_01",
  "resume_token": "optional",
  "last_received": [
    {
      "conversation_id": "conv_1",
      "seq": 1024
    }
  ]
}
```

`ping`：

```json
{
  "op": "client.ping",
  "request_id": "req_02"
}
```

`client.ping` 是应用层 heartbeat frame。第一阶段不依赖 WebSocket protocol-level ping/pong 作为唯一心跳依据；服务端收到合法 `client.ping` 后必须返回 `server.pong`。

`delivery.ack`：

```json
{
  "op": "delivery.ack",
  "request_id": "req_03",
  "conversation_id": "conv_1",
  "received_seq": 1025
}
```

第一阶段不通过 WebSocket 实现 `message.send`。发送消息仍走已有 gRPC / HTTP adapter，后续再决定是否在 push-gateway 里做协议适配。

### 5.2 Server -> Client frames

`server.hello`：

```json
{
  "op": "server.hello",
  "request_id": "req_01",
  "session_id": "sess_01",
  "resume_token": "resume_01",
  "heartbeat_interval_ms": 30000
}
```

`server.pong`：

```json
{
  "op": "server.pong",
  "request_id": "req_02",
  "server_time_ms": 1781000000000
}
```

`delivery.notify`：

```json
{
  "op": "delivery.notify",
  "event_id": "evt_01",
  "tenant_id": "tenant_1",
  "conversation_id": "conv_1",
  "conversation_seq": 1025,
  "source_event_id": "timeline_evt_01",
  "message_id": "msg_01",
  "correlation_id": "corr_01",
  "pull_required": true
}
```

`delivery.ack.ok`：

```json
{
  "op": "delivery.ack.ok",
  "request_id": "req_03",
  "conversation_id": "conv_1",
  "last_received_seq": 1025
}
```

`delivery.ack` 成功调用 `delivery-service AckDelivery` 后必须返回 `delivery.ack.ok`。失败时返回 `error` frame，并使用稳定错误码，例如 `PERMISSION_DENIED`、`DELIVERY_UNAVAILABLE` 或 `ACK_OUT_OF_VISIBLE_RANGE`。客户端不得把“没有返回 error”解释成 ACK 成功。

`server.resume_hint`：

```json
{
  "op": "server.resume_hint",
  "reason": "buffer_miss",
  "pull_required": true,
  "conversations": [
    {
      "conversation_id": "conv_1",
      "after_seq": 1024
    }
  ]
}
```

`conversations` 是可选提示，不是服务端对客户端已持久化进度的权威判断。queue-full / slow-session 场景下，gateway 可以不返回具体 `conversation_id / seq`；客户端必须以本地 durable cursor / `last_received` 为准调用 `PullInbox`，不能把 gateway 提示的 seq 当作可靠 ACK 或已送达水位。

`error`：

```json
{
  "op": "error",
  "request_id": "req_03",
  "code": "ACK_OUT_OF_VISIBLE_RANGE",
  "message": "ack out of visible range",
  "retryable": false
}
```

### 5.3 错误码

| 错误码 | 语义 | 客户端动作 | 是否可重试 |
| --- | --- | --- | --- |
| `INVALID_FRAME` | frame JSON 或 op 不合法 | 修正客户端逻辑 | 否 |
| `AUTH_EXPIRED` | token 过期 | 刷新 token 后重连 | 是 |
| `DEVICE_REVOKED` | device/session 被吊销 | 退出登录或重新认证 | 否 |
| `PERMISSION_DENIED` | 当前 session 无权执行 ACK 或访问资源 | 停止重试，重新同步权限或重新登录 | 否 |
| `RATE_LIMITED` | 连接或 frame 超限 | 按 `retry_after_ms` 退避 | 是 |
| `SERVER_BUSY` | gateway 过载 | 指数退避重连 | 是 |
| `DELIVERY_UNAVAILABLE` | delivery-service 暂不可用 | 稍后重试 / pull fallback | 是 |
| `ACK_OUT_OF_VISIBLE_RANGE` | ACK 超出可见范围 | 触发 `PullInbox` 同步 | 否 |
| `SEQ_GAP` | 客户端报告或本地 buffer 发现 gap | 调用 `PullInbox` | 是 |

对外 error message 必须稳定，不暴露 Redis、Kafka、gRPC 内部错误文本。

## 6. 异步事件契约

### 6.1 消费事件

| 事件 | Topic | 分区键 | 处理 |
| --- | --- | --- | --- |
| `delivery.inbox_item.created.v1` | `im.delivery.events` | `tenant_id + conversation_id` | 找到在线 user/device session，发送 `delivery.notify` |
| `delivery.ack.recorded.v1` | `im.delivery.events` | `tenant_id + conversation_id` | 第一阶段只记录指标，不向客户端广播 |

消费规则：

- Kafka consumer 只能在 notification 已成功交给本地 session queue 或确认目标用户不在线后提交 offset。
- 对没有历史提交位点的新 consumer group，push-gateway 从 latest delivery event 开始消费；它不负责重放历史在线通知，历史缺口由客户端 `PullInbox` 兜底。
- `delivery.inbox_item.created.v1` 是 user 级投递事件；push-gateway 应向该 user 当前所有在线 device/session 发送 `delivery.notify`，同一 device/session 通过 `event_id` 去重。
- 如果目标用户不在线，事件可直接视为已处理；离线补拉由 `user_inbox` 保证。
- 如果发送队列满，不能无限阻塞 Kafka consumer；应标记 session slow，发送 `server.resume_hint` 或断开连接，让客户端回源 `PullInbox`。
- unsupported / malformed delivery event 必须 fail-closed：不向客户端发送错误通知，不提交业务完成状态；第一阶段可以停止 worker 并报警，后续进入 projection DLQ / repair。

### 6.2 发布事件

第一阶段 push-gateway 不发布业务 Kafka 事件。

后续如需要审计连接事件，必须使用独立 `push.audit.events` 或 audit-service，不允许把连接事件写入 `conversation.timeline.events` 或 `im.delivery.events`。

## 7. 数据库 / 状态设计

第一阶段不新增 PostgreSQL migration。

单进程 smoke 使用 in-memory registry：

```text
tenant_id:user_id:device_id -> session_id
session_id -> websocket connection
session_id -> resume buffer
```

多实例前必须接入 Redis route：

```text
push:route:{tenant_id}:{user_id}:{device_id} -> {
  gateway_id,
  session_id,
  connected_at,
  expires_at
}

push:session:{session_id} -> {
  tenant_id,
  user_id,
  device_id,
  gateway_id,
  state,
  last_heartbeat_at
}
```

Redis 约束：

- 所有 route key 必须有 TTL。
- heartbeat 成功后续期。
- disconnect / timeout 必须 best-effort 删除 route。
- Redis route 是在线状态，不是投递事实源；Redis 丢失后客户端重连恢复。

Resume buffer：

```text
session_id -> ring buffer of delivery.notify frames
```

约束：

- buffer 按 session 保留最近 N 条或 N 秒。
- buffer 中只放轻量 notification，不放完整 message fact。
- buffer miss 必须回退到 delivery-service `PullInbox`。
- `resume_token` 第一阶段为 in-memory opaque token，绑定 `tenant_id / user_id / device_id`；重连会创建新的 `session_id`，但可以复用同一 token 读取单实例 buffer。TTL 与 resume buffer TTL 一致。
- 服务重启、token 过期或 token 与 device/session 不匹配时，resume 失败，服务端返回 `server.resume_hint`，客户端 fallback `PullInbox`。
- 当前单实例第一版只实现 in-memory、按条数裁剪的 best-effort resume buffer；TTL、Redis route 和跨实例 resume 仍是后续切片。

第一版可编码配置：

```text
NEXUSIM_PUSH_SESSION_QUEUE_SIZE=256
NEXUSIM_PUSH_WRITE_TIMEOUT=2s
NEXUSIM_PUSH_SLOW_EVICT_AFTER=3
NEXUSIM_PUSH_RESUME_BUFFER_SIZE=256
NEXUSIM_PUSH_RESUME_BUFFER_TTL=5m
NEXUSIM_PUSH_HEARTBEAT_INTERVAL=30s
NEXUSIM_PUSH_IDLE_TIMEOUT=75s
```

## 8. 核心流程

### 8.1 建连

```text
Client WebSocket connect
-> api WebSocket handler
-> app ConnectSessionUseCase
-> auth token validate
-> domain create PushSession
-> register route in memory / Redis
-> server.hello
```

### 8.2 在线通知

```text
delivery-service local transaction
-> delivery_outbox
-> delivery outbox relay
-> Kafka im.delivery.events
-> push-gateway delivery event consumer
-> NotifyDeliveryUseCase
-> route lookup by tenant_id + user_id + device_id / user sessions
-> enqueue delivery.notify
-> client receives notification
-> client calls PullInbox
-> client persists locally
-> client sends delivery.ack frame
-> push-gateway calls delivery-service AckDelivery
-> server returns delivery.ack.ok
```

第一阶段可以只对当前 gateway 进程内在线 session 通知；多实例前必须接入 Redis route 或统一 consumer ownership。

### 8.3 重连恢复

```text
Client reconnects with resume_token and last_received seq
-> push-gateway validates session
-> if resume buffer covers gap:
      replay buffered delivery.notify
   else:
      server.resume_hint pull_required=true
-> client calls PullInbox(after_seq)
-> client de-duplicates by conversation_id + seq / message_id
-> client sends AckDelivery after local durable write
```

### 8.4 慢连接处理

```text
session send queue over threshold
-> mark DEGRADED
-> send broad server.resume_hint if possible
-> first implementation closes the WebSocket on queue-full eviction
-> future implementation may add NEXUSIM_PUSH_SLOW_EVICT_AFTER before active close
-> route cleanup
-> client reconnects and PullInbox
```

## 9. 一致性和事务

强一致边界：

```text
push-gateway 无业务数据库事务。
delivery-service 的 user_inbox / cursor / delivery_outbox 事务仍是投递事实边界。
```

最终一致边界：

```text
delivery-service delivery_outbox
-> Kafka im.delivery.events
-> push-gateway online notification
-> client PullInbox
-> client local durable write
-> AckDelivery
```

关键约束：

- `delivery.notify` 到达客户端不代表 ACK 完成。
- 客户端必须以 `PullInbox` 返回的 `user_inbox` 结果为准。
- push-gateway 断线、重启、buffer 丢失不能造成消息丢失；最多造成在线通知丢失，客户端重连补拉恢复。

## 10. 幂等、重试和补偿

| 场景 | 幂等键 | 重试策略 | 补偿 |
| --- | --- | --- | --- |
| WebSocket connect | `session_id` / `resume_token` | 客户端指数退避重连 | 重新认证 |
| delivery notify | `event_id` | Kafka 可重放，客户端按 `event_id` / `conversation_id + seq` 去重 | `PullInbox` 补拉 |
| client ACK frame | `tenant_id + user_id + device_id + conversation_id + received_seq` | push-gateway 可重试调用 `AckDelivery` | delivery-service cursor 单调幂等 |
| slow connection eviction | `session_id` | 客户端重连 | `PullInbox` 补拉 |

## 11. 权限和安全

- `tenant_id / user_id / device_id / session_id` 必须从认证上下文派生，不信任客户端 frame 裸字段。
- WebSocket 建连必须校验 token、device 状态和 session 状态。
- `delivery.notify` 只能发给同 tenant 的目标 user/device session。
- ACK frame 的 user/device 以 session auth context 为准。
- error frame 不返回内部 Redis / Kafka / gRPC 错误文本。
- 连接日志必须包含 `tenant_id / user_id / device_id / session_id / trace_id`，但不记录 message payload。

## 12. SLO 和指标

第一阶段本地目标：

| 指标 | 目标 |
| --- | --- |
| WebSocket connect p95 | `< 100ms` |
| delivery event -> notify enqueue p95 | `< 100ms` |
| heartbeat timeout cleanup | `< 2 * heartbeat_interval` |
| push event loss causing missed durable inbox | `0`，用 PullInbox 恢复 |

必须打点：

```text
push_ws_connected_sessions
push_ws_connect_latency_ms
push_ws_frame_decode_error_count
push_delivery_event_lag
push_notify_enqueue_latency_ms
push_notify_sent_count
push_notify_dropped_count
push_slow_session_evicted_count
push_ack_forward_latency_ms
push_delivery_pull_fallback_count
```

## 13. 测试方案

| 测试 | 目标 |
| --- | --- |
| unit | session 状态机、resume buffer、frame decode/encode、slow queue policy |
| integration | fake delivery-service + WebSocket client，验证 notify / ack forwarding |
| Kafka smoke | 构造 `delivery.inbox_item.created.v1`，在线 client 收到 `delivery.notify` |
| full smoke | `SendMessage -> delivery projection -> delivery_outbox -> im.delivery.events -> push-gateway -> client PullInbox -> AckDelivery` |
| negative smoke | push-gateway 离线时事件不丢；客户端重连后 PullInbox 能补到 |
| loadtest | 小规模 WS connect / notify / heartbeat，不做硬件极限矩阵 |

原始压测数据保存到：

```text
H:\NexusIM\loadtest-results
```

报告文档保存到：

```text
docs/runbook/loadtest/push-gateway/
```

## 14. Runbook

运行模式规划：

```text
NEXUSIM_PUSH_GATEWAY_MODE=ws
NEXUSIM_PUSH_GATEWAY_MODE=delivery-consumer
NEXUSIM_PUSH_GATEWAY_MODE=all
```

`all` 是第一阶段本地 smoke 推荐模式：WebSocket handler 和 delivery consumer 在同一个进程里共享 in-memory session registry。后续多实例或压测隔离时再拆成 `ws` 与 `delivery-consumer`，并接入 Redis route。

最小本地启动参数：

```text
NEXUSIM_PUSH_GATEWAY_MODE=all
NEXUSIM_PUSH_WS_ADDR=0.0.0.0:10496
NEXUSIM_DELIVERY_GRPC_ADDR=127.0.0.1:10497
NEXUSIM_KAFKA_BROKERS=localhost:9092
NEXUSIM_DELIVERY_EVENTS_TOPIC=im.delivery.events
NEXUSIM_PUSH_CONSUMER_GROUP=nexusim-push-gateway
```

本地依赖：

```text
PostgreSQL
Kafka
Redis
delivery-service grpc
delivery-service outbox-relay
```

本地默认端口：

```text
10496  push-gateway WebSocket
10497  debug / metrics
```

常见故障：

| 故障 | 排查 |
| --- | --- |
| 连接成功但没有推送 | 查 `im.delivery.events`、consumer group lag、route 是否存在 |
| 客户端收到通知但拉不到 | 查 delivery-service `user_inbox` 和 conversation seq |
| ACK 失败 | 查 `AckDelivery` 错误码和 session auth context，尤其 `PERMISSION_DENIED` / `ACK_OUT_OF_VISIBLE_RANGE` |
| 多实例漏推 | 查 Redis route / gateway_id / session TTL |
| 慢连接堆积 | 查 send queue、eviction count、client reconnect |

## 15. 验收标准

进入编码前：

- 本 SDD 经过阶段评审，无 P0/P1。
- WebSocket frame 契约冻结。
- 明确第一阶段不新增 PostgreSQL migration。
- 明确 push-gateway 只依赖 `im.delivery.events` 和 delivery-service API。

第一阶段完成：

- `services/push-gateway/internal/{api,app,domain,infrastructure,types,trigger}` 存在。
- WebSocket client 可以 connect / hello / ping。
- push-gateway 能消费 `delivery.inbox_item.created.v1` 并向在线 client 发送 `delivery.notify`。
- 客户端收到 notify 后通过 `PullInbox` 拉到 durable inbox item。
- 客户端 ACK frame 通过 push-gateway 转发到 `delivery-service AckDelivery`。
- push-gateway 离线或重启不造成 durable inbox 丢失；客户端重连后可补拉。
- 小规模 smoke 报告归档到 `docs/runbook/loadtest/push-gateway/`。
- 不把 smoke 表述为完整生产 WebSocket 平台或容量结论。
