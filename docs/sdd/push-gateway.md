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
| 同步依赖 | auth / identity provider | 当前已支持本地 mock、legacy HMAC gateway token、JWT HS256 gateway token 和异步 revoke deny-list；生产仍需完整 Login / refresh token / 非对称 JWK |
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

当前代码支持两种 auth 模式：

- `mock`：本地 smoke 使用，允许 query string 中的 `tenant_id/user_id/device_id` 或 `token=tenant:user[:device]` 生成 `AuthContext`。
- `hmac`：第一版 signed gateway token verifier。当前兼容两种格式：legacy `base64url(json claims).base64url(hmac_sha256(payload, secret))`，以及标准三段 JWT HS256 `base64url(header).base64url(claims).base64url(signature)`。claims 至少包含 `tenant_id/user_id/aud/exp`，JWT 还应包含 `iss/sub/iat/kid`；`aud` 默认必须为 `push-gateway`，可包含 `device_id/session_id/trace_id`；如果 token 中带 `device_id`，必须与 query / `client.hello.device_id` 一致。真实客户端优先用 `Authorization: Bearer <token>`，query token 只作为本地兼容入口。
- `jwt`：第一版 RS256 gateway JWT verifier。push-gateway 从 `NEXUSIM_PUSH_AUTH_JWKS_JSON` / `NEXUSIM_PUSH_AUTH_JWKS_FILE` 读取静态公钥 JWKS，或从 `NEXUSIM_PUSH_AUTH_JWKS_URL` 启动拉取并按 `NEXUSIM_PUSH_AUTH_JWKS_REFRESH_INTERVAL` 定期刷新。它只接受 `alg=RS256`、`use=sig` 或空 use、2048-bit 以上 RSA modulus 和匹配 `kid` 的公钥签名，可用 `NEXUSIM_PUSH_AUTH_TRUSTED_ISSUERS` 限定受信 issuer。

当 `NEXUSIM_PUSH_AUTH_MODE=mock` 时，WebSocket 监听地址必须是 loopback 或 RFC1918 私网；非私网监听地址应在启动前直接失败，避免把本地 smoke 身份模式暴露到公网。`hmac` / `jwt` 模式若用于非私网 WebSocket 监听地址，也必须同时启用入口 TLS / WSS；否则进程应在启动前直接失败，避免把签名 token 暴露到 plaintext 公网入口。

`hmac` 模式只证明 gateway 能拒绝伪造 / 过期 / device mismatch 的客户端身份，不等同完整 identity-service。当前已支持最小密钥轮换：`NEXUSIM_PUSH_AUTH_HMAC_SECRET` 是当前签发密钥，`NEXUSIM_PUSH_AUTH_HMAC_PREVIOUS_SECRETS` 是逗号分隔的旧密钥，只用于验证旧 token。当前也支持 `im.identity.events` 异步 revoke projection：`identity.device.revoked.v1` / `identity.session.revoked.v1` 会进入 in-memory 或 Redis deny-list，WebSocket 建连时命中 deny-list 返回 `PERMISSION_DENIED`；已在线 session 会收到 broad `server.resume_hint(reason=identity_revoked)` 并被 active close。RS256 模式把 hot path 仍保持为本地验签，并已有第一版远程 JWKS cache、refresh 失败保留旧 key set、`/debug/metrics.auth_jwks` 观测和 identity-service 旧公钥 overlap；自动 key rotation、KMS/HSM、多 issuer 治理和 token exchange 仍属于后续生产化。

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
  "source_event_type": "message.persisted.v1",
  "message_id": "msg_01",
  "correlation_id": "corr_01",
  "pull_required": true
}
```

`delivery.hide`：

```json
{
  "op": "delivery.hide",
  "event_id": "evt_hide_01",
  "tenant_id": "tenant_1",
  "conversation_id": "conv_1",
  "conversation_seq": 1025,
  "source_event_type": "delivery.inbox_item.hidden.v1",
  "message_id": "msg_01",
  "correlation_id": "corr_01",
  "pull_required": true
}
```

`delivery.hide` 是同一用户其它在线设备的轻量本地视图更新提示，不代表 message-service 的撤回 / 会话级删除 / 合规删除事实。客户端可先按 `conversation_id + conversation_seq` 本地移除，再用 `PullInbox` 校准 durable delivery 视图。

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
| `DELIVERY_UNAVAILABLE` | delivery-service 暂不可用 | 稍后重试 / pull recovery | 是 |
| `ACK_OUT_OF_VISIBLE_RANGE` | ACK 超出可见范围 | 触发 `PullInbox` 同步 | 否 |
| `SEQ_GAP` | 客户端报告或本地 buffer 发现 gap | 调用 `PullInbox` | 是 |

对外 error message 必须稳定，不暴露 Redis、Kafka、gRPC 内部错误文本。

## 6. 异步事件契约

### 6.1 消费事件

| 事件 | Topic | 分区键 | 处理 |
| --- | --- | --- | --- |
| `delivery.inbox_item.created.v1` | `im.delivery.events` | `tenant_id + conversation_id` | 找到在线 user/device session，发送 `delivery.notify` |
| `delivery.inbox_item.hidden.v1` | `im.delivery.events` | `tenant_id + conversation_id` | 找到在线 user/device session，发送 `delivery.hide` |
| `delivery.ack.recorded.v1` | `im.delivery.events` | `tenant_id + conversation_id` | 第一阶段只记录指标，不向客户端广播 |

消费规则：

- Kafka consumer 只能在 notification 已成功交给本地 session queue 或确认目标用户不在线后提交 offset。
- 对没有历史提交位点的新 consumer group，push-gateway 从 latest delivery event 开始消费；它不负责重放历史在线通知，历史缺口由客户端 `PullInbox` 补拉。
- `delivery.inbox_item.created.v1` 和 `delivery.inbox_item.hidden.v1` 都是 user 级 delivery 事件；push-gateway 应向该 user 当前所有在线 device/session 发送对应 `delivery.notify` / `delivery.hide`，同一 device/session 通过 `event_id` 去重。
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

Redis route 第一版最小实现已经落地，用于把 user 级 `delivery.notify` 从消费到事件的 gateway 转发到持有目标 WebSocket session 的 gateway。当前实现保存 session route，并通过 Redis Pub/Sub 按 `gateway_id` 转发通知：

```text
push:route:session:{session_id} -> {
  tenant_id,
  user_id,
  device_id,
  resume_token,
  gateway_id,
}

push:route:user:{tenant_id}:{user_id} -> set(session_id)
push:route:gateway:{gateway_id}:notify -> Pub/Sub delivery notification
```

Redis 约束：

- 所有 route key 必须有 TTL。
- 第一版 `NEXUSIM_PUSH_ROUTE_BACKEND=redis` 会在 connect 时写入 session route，并由 gateway 进程按 TTL 比例周期性刷新 route；disconnect 时 best-effort 删除 route。
- 进程崩溃、机器断电或 Redis 短故障时，session route 最终依赖 TTL 过期；gateway 已接入后台 stale route cleanup，会周期扫描 `route:user:*` set 并移除 session key 缺失、JSON 损坏或 tenant/user 不匹配的成员。
- disconnect / timeout 必须 best-effort 删除 route。
- Redis route 是在线状态，不是投递事实源；Redis 丢失后客户端重连恢复。
- 同一远端 gateway 上有多个 session 时，只向该 gateway Pub/Sub channel 发布一次，远端本地 registry 再 fanout 到本机 session。
- identity revoke 也复用同一套 Redis route 控制面：identity-consumer 写入 deny-list 后，可按 `tenant/user/device/session` 查 route，并向持有 WebSocket 的 gateway 发布 eviction 控制消息；远端 gateway 本地 registry 再发送 `server.resume_hint(reason=identity_revoked)` 并关闭对应 session。
- lookup 时如果发现 session key 已过期、route JSON 损坏或 route tenant/user 与当前通知不匹配，必须把该 session 从 user route set 中移除，避免 stale route 长期放大。
- 当前最小策略采用在线通知 fail-open：Redis lookup / publish 返回错误时，不阻塞 delivery consumer 提交当前 Kafka event；该 notify 视为在线唤醒失败，客户端通过 durable `PullInbox` 恢复。对于 Redis `Publish` 成功但 subscriber 数为 0 的情况，也必须按在线唤醒失败处理，不能把 stale route 或已掉线 gateway 误记为远端已入队。connect 写 route 失败仍采用 fail-closed，避免把只有本地可见、跨实例不可路由的 session 伪装为在线。
- Redis route eviction 是 online control，不是安全事实源。Pub/Sub publish 成功不代表远端 session 一定关闭；deny-list 才是 revoke 后拒绝新建连的安全线。

Resume buffer：

```text
session_id -> ring buffer of delivery.notify frames
push:resume:token:{resume_token}:meta   -> tenant_id / user_id / device_id
push:resume:token:{resume_token}:frames -> list(delivery.notify frame)
```

约束：

- buffer 按服务端签发的 `resume_token` 保留最近 N 条通知；重连会创建新的 `session_id`，但可以复用同一 token 读取单实例 buffer。
- buffer 中只放轻量 notification，不放完整 message fact。
- buffer miss 必须回退到 delivery-service `PullInbox`。
- `resume_token` 第一阶段为 in-memory opaque token，绑定 `tenant_id / user_id / device_id`；重连会创建新的 `session_id`，但可以复用同一 token 读取单实例 buffer。TTL 与 resume buffer TTL 一致。
- `resume_token` 必须由服务端签发。客户端携带未知 token 时，服务端返回 `server.resume_hint(reason=buffer_miss)`，并签发新的 opaque token；不能把客户端自带 token 注册成有效 token。
- 服务重启、token 过期或 buffer miss 时，服务端返回 `server.resume_hint`，客户端 recovery `PullInbox`；已知 token 绑定身份不匹配时返回 `PERMISSION_DENIED`。
- 当前单实例第一版已实现 in-memory、按条数和 TTL 裁剪的 best-effort resume buffer；活跃 session 会续住 token，断线后的非活跃 token 到期后返回 `server.resume_hint(reason=buffer_miss)` 并签发新 token。
- 启用 `NEXUSIM_PUSH_ROUTE_BACKEND=redis` 时，Redis route 会把 session route 中的 `resume_token` 作为跨实例 resume 索引。任意 gateway 在处理 `delivery.notify` 时，会为 Redis route 中命中的在线 session 写入 `push:resume:token:{resume_token}:frames`；客户端带同一 token 重连到其他 gateway 时，只要 Redis buffer 覆盖 `last_received` 之后的通知，就可以 replay。
- Redis-backed resume buffer 仍是 best-effort 体验优化，不是可靠投递层。Redis meta / frames miss、TTL 到期、JSON 损坏、buffer gap、Redis 读写错误或 token 身份不匹配时，不得伪造送达；应返回 `server.resume_hint(reason=buffer_miss)` 或 `PERMISSION_DENIED`，最终回退到 durable `PullInbox`。
- Redis route 与 Redis resume 是两个不同职责：route 找在线 session，resume buffer 支持短断线 replay。Pub/Sub publish 成功不代表 WebSocket 写出成功，Redis resume replay 成功也不代表客户端已 ACK。

当前已接入 runtime 的配置：

```text
NEXUSIM_PUSH_SESSION_QUEUE_SIZE=256
NEXUSIM_PUSH_WRITE_TIMEOUT=2s
NEXUSIM_PUSH_HEARTBEAT_INTERVAL=30s
NEXUSIM_PUSH_TEST_WRITE_DELAY=0
NEXUSIM_PUSH_RESUME_BUFFER_TTL=10m
```

规划配置，尚未接入 runtime：

```text
NEXUSIM_PUSH_SLOW_EVICT_AFTER=3
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

第一阶段可以只对当前 gateway 进程内在线 session 通知。当前已接入 Redis route 最小 adapter，并已用真实进程 smoke 验证 WebSocket gateway 与 delivery consumer gateway 分离时的跨进程在线路由；它证明的是最小分布式在线唤醒链路，不等同于完整生产多实例能力。当前已完成本地三 Redis / 三 Sentinel 手动 master failover 后的 route / resume recovery smoke，也已完成停止 Sentinel 当前 master 容器后由 Sentinel 自主选主的 recovery smoke；生产化前仍需补 Sentinel quorum / 网络分区、Redis Cluster、跨实例慢连接组合和正式指标。
当前 Redis-backed resume buffer 已有最小代码、单元测试、本机跨进程 smoke 和 Win-Mac Docker smoke 覆盖，证明不同 gateway 可以通过同一 Redis token buffer replay `delivery.notify`；但这仍是短时体验优化，不是可靠投递。Redis miss、token mismatch、buffer gap、Redis error 或 gateway 重启后仍必须回退 `PullInbox`。

Redis route debug metrics 已提供第一版跨实例在线路由计数：

| 指标 | 含义 |
| --- | --- |
| `redis_route_remote_matched_sessions` | 当前 gateway 在 Redis route 中命中的远端 session 数，按 session 计 |
| `redis_route_remote_publish_call_count` | 当前 gateway 向远端 gateway Pub/Sub channel 发布通知的次数，按 gateway 去重 |
| `redis_route_remote_enqueued_sessions` | 当前 gateway 估算已转交远端 gateway 的 session 数；仅在 publish 成功且存在 subscriber 时递增，不代表远端 WebSocket 已成功写出 |
| `redis_route_remote_publish_error_count` | Redis Publish 调用失败次数 |
| `redis_route_remote_no_subscriber_count` | Redis Publish 成功但 subscriber 数为 0 的远端 session 数；通常表示 stale route 或远端 subscriber 已不在线 |
| `redis_route_lookup_error_count` | Redis route lookup 失败次数；当前策略 fail-open，delivery consumer 不因在线通知失败而阻塞 |
| `redis_route_stale_removed_count` | lookup 或 cleanup 移除的 stale session route 成员数 |
| `redis_route_subscriber_message_count` | 当前 gateway 从自身 Pub/Sub channel 收到的远端通知数 |
| `redis_route_subscriber_enqueued_count` | 远端通知进入当前 gateway 本机 session registry 的数量 |
| `redis_route_subscriber_evicted_count` | 远端 revoke eviction 控制消息导致当前 gateway 本机 session 被关闭的数量 |
| `redis_route_subscriber_malformed_count` | Pub/Sub 收到 malformed / incomplete payload 并跳过的次数 |
| `redis_resume_append_count` | 当前 gateway 写入 Redis-backed resume buffer 的 delivery.notify frame 数 |
| `redis_resume_append_error_count` | 写入 Redis-backed resume buffer 失败次数；失败只降级 resume，不改变 durable inbox |
| `redis_resume_replay_count` | 从 Redis-backed resume buffer replay 的 delivery.notify frame 数 |
| `redis_resume_miss_count` | 未知 token、buffer gap 或 replay queue 满导致的 buffer miss 次数 |
| `redis_resume_permission_denied_count` | 已知 token 与当前 tenant/user/device 不匹配的拒绝次数 |

这些指标只解释 online wakeup 路径，不是 durable delivery 成功率。可靠事实仍以 `delivery-service` 的 `user_inbox`、`device_delivery_cursors` 和 `delivery_outbox` 为准。

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
- 本地 smoke 可使用 `NEXUSIM_PUSH_AUTH_MODE=mock`；进入真实客户端或跨机器演示时优先使用 `NEXUSIM_PUSH_AUTH_MODE=jwt` + RS256 JWKS，或短期内继续使用 `hmac` 兼容模式。
- WebSocket 建连必须校验 token。当前 HMAC 模式兼容 legacy token 和 JWT HS256；JWT 模式只接受 RS256 公钥验签。两种模式都校验签名、`aud`、过期时间、device 绑定和本地 / Redis deny-list；device / session revoke 由 `im.identity.events` 异步投影，热路径不同步 RPC 查询 identity-service。revoke event 也会 best-effort 主动关闭当前在线 session；安全语义仍以 deny-list 拒绝后续建连为准。
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
push_delivery_pull_recovery_count
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

`all` 是第一阶段本地 smoke 推荐模式：WebSocket handler 和 delivery consumer 在同一个进程里共享 session registry。默认 route backend 为 in-memory；需要跨实例在线路由时启用 `NEXUSIM_PUSH_ROUTE_BACKEND=redis`。

本地分布式模拟使用 `NEXUSIM_PUSH_GATEWAY_MODE=ws` 和 `NEXUSIM_PUSH_GATEWAY_MODE=delivery-consumer` 启动两个独立 `push-gateway` 进程：WebSocket 连接只落在 ws 进程，Kafka `im.delivery.events` 只由 consumer 进程消费，在线通知必须经过 Redis route / PubSub 才能到达客户端。该模式用于验证分布式路由边界，不作为生产容量结论。

当 WebSocket HTTP server 启动时，`GET /healthz` 返回 `{"service":"push-gateway","status":"ok"}`，`GET /readyz` 返回 `{"service":"push-gateway","status":"ready"}`。`GET /debug/metrics` 返回当前单实例 registry 调试指标，包括 connected sessions、queue-full eviction、resume replay / buffer miss、resume buffer stored frames、resume token count 和 expired token count。启用 Redis route 时还会返回 `redis_registry_metrics` 和 `redis_subscriber_metrics`，用于区分远端 route 命中、Pub/Sub publish、subscriber 入站 fanout 和 stale cleanup。启用 JWT remote JWKS 时还会返回 `auth_jwks`，包含是否配置远程 URL、当前缓存 key 数、最近 refresh 成功 / 失败时间和失败计数。启用 first-stage OpenTelemetry 时还会返回 `trace` 配置快照。

`GET /metrics` 复用同一低敏 snapshot 并输出 Prometheus text。第一阶段本地 scrape target 为 `host.docker.internal:11913/metrics`，对应进程可用 `NEXUSIM_PUSH_DEBUG_ADDR=127.0.0.1:11913` 启动独立 debug listener。当前指标覆盖 WebSocket session、slow eviction、in-memory / Redis resume、Redis route / subscriber、delivery / identity consumer worker、auth JWKS、OTel trace config 聚合，以及 WebSocket writer 的 `frame_write` / `delivery_notify` 写耗时 histogram / sum / count / max。writer duration 指标只用固定低基数 `operation` label，用于热点群压测区分 `conn.Write` 长尾、flush / scheduling、网络吞吐和客户端读取背压。labels 只允许 `state`、`event`、`role`、`consumer`、`exporter`、`operation` 等低基数字段，不输出 token、tenant_id、user_id、device_id、session_id、request_id、trace_id、conversation_id、message_id 或 event_id。该端点、本地 Prometheus alert rules 和 Grafana dashboard 只用于本地开发 / 面试演示，不是生产 SLO、retention 或 Alertmanager route。debug metrics 默认只挂在 loopback / 私网 WebSocket listener 或 `NEXUSIM_PUSH_DEBUG_ADDR` 独立 debug listener 上；公网 WebSocket listener 默认不挂 `/debug/metrics` / `/metrics`，独立 debug listener 绑定公网也会启动失败，显式公网暴露必须设置 `NEXUSIM_PUSH_DEBUG_ALLOW_PUBLIC=true`。

first-stage OpenTelemetry 通过 `NEXUSIM_PUSH_OTEL_*` 显式启用，默认关闭。当前只覆盖 WebSocket connection span，用于观察在线入口连接生命周期；span 只允许记录 auth mode、route backend、TLS 是否启用、gateway id 是否配置等低敏连接形态字段，不记录 token、tenant_id、user_id、device_id、session_id、conversation_id、message_id、payload 或 Redis / Kafka / gRPC 内部错误文本。生产化前仍需要统一采样、trace retention、告警和高基数属性审计。

最小本地启动参数：

```text
NEXUSIM_PUSH_GATEWAY_MODE=all
NEXUSIM_PUSH_WS_ADDR=0.0.0.0:10496
NEXUSIM_DELIVERY_GRPC_ADDR=127.0.0.1:10497
NEXUSIM_KAFKA_BROKERS=localhost:9092
NEXUSIM_DELIVERY_EVENTS_TOPIC=im.delivery.events
NEXUSIM_PUSH_CONSUMER_GROUP=nexusim-push-gateway
NEXUSIM_PUSH_AUTH_MODE=mock
```

默认情况下，push-gateway 调 `delivery-service AckDelivery` 使用 plaintext gRPC，兼容本地 smoke。若 delivery-service gRPC server 开启 TLS / mTLS，可在 push-gateway WebSocket 进程配置第一阶段静态出站 TLS：

```text
NEXUSIM_DELIVERY_SERVICE_TLS_CA_FILE=...
NEXUSIM_DELIVERY_SERVICE_TLS_SERVER_NAME=delivery-service.nexusim.local
NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_CERT_FILE=...
NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_KEY_FILE=...
```

配置任一 `NEXUSIM_DELIVERY_SERVICE_TLS_*` 后必须提供 CA file，client cert/key 必须成对配置。该能力只覆盖 push-gateway 到 delivery-service 的 ACK RPC client，不代表证书签发 / 轮换 / 分发、动态服务身份治理或全服务 mTLS rollout 已完成。

默认情况下，push-gateway WebSocket listener 使用 plaintext `ws://`。若需要第一阶段静态 WSS / mTLS，可配置：

```text
NEXUSIM_PUSH_WS_TLS_CERT_FILE=...
NEXUSIM_PUSH_WS_TLS_KEY_FILE=...
NEXUSIM_PUSH_WS_TLS_CLIENT_CA_FILE=...
NEXUSIM_PUSH_WS_TLS_REQUIRE_CLIENT_CERT=true
NEXUSIM_PUSH_WS_TLS_CLIENT_ALLOWED_DNS_NAMES=desktop-client.nexusim.local
NEXUSIM_PUSH_WS_TLS_CLIENT_ALLOWED_URIS=spiffe://nexusim/desktop-client
```

配置 cert/key 后 WebSocket server 使用 `ListenAndServeTLS`；配置 client CA、显式 require client cert 或客户端身份 allowlist 后启用 mTLS。allowlist 只做 exact-match DNS SAN / URI SAN，不做动态服务身份发现、证书签发、轮换、分发或浏览器证书 UX。

HMAC gateway token 可选参数：

```text
NEXUSIM_PUSH_AUTH_MODE=hmac
NEXUSIM_PUSH_AUTH_HMAC_SECRET=local-dev-secret
NEXUSIM_PUSH_AUTH_HMAC_PREVIOUS_SECRETS=old-secret-1,old-secret-2
```

`hmac` 模式下，query string 裸 `tenant_id/user_id` 不再被信任；缺失 token、签名错误、JWT `alg` 不是 `HS256`、audience 不匹配、token 与 device 不匹配、deny-list 命中或 revoke checker 不可用会返回 `PERMISSION_DENIED`，过期 token 返回 `AUTH_EXPIRED`。

本地 smoke 可用：

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -PushAuthMode hmac `
  -PushAuthHmacSecret local-push-smoke-secret
```

密钥轮换窗口可用 current + previous secrets 验证：服务端 current secret 使用新密钥，previous secrets 放旧密钥，runner 用旧密钥签发 token。

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -PushAuthMode hmac `
  -PushAuthHmacSecret new-local-push-secret `
  -PushAuthHmacPreviousSecrets old-local-push-secret `
  -PushAuthTokenSigningSecret old-local-push-secret
```

runner 会使用 `Authorization: Bearer` 传 token，并在 summary 中记录 `push_auth_query_identity_sent=false`、`push_auth_hmac_previous_secrets_configured`、`push_auth_token_signing_secret_explicit` 和 `push_auth_token_signed_with_non_current_secret`，用于证明没有依赖裸 query 身份，也能区分服务端是否配置 previous secrets、客户端 token 是否显式用非 current secret 签名。

identity-service 签发 JWT HS256 gateway token 时，使用：

```text
NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT=jwt
NEXUSIM_IDENTITY_GATEWAY_TOKEN_KEY_ID=<kid>
NEXUSIM_IDENTITY_GATEWAY_TOKEN_ISSUER=nexusim-identity
```

identity-service 签发 JWT RS256 gateway token 时，使用：

```text
NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT=jwt-rs256
NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_PEM=<pem>
# or
NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_FILE=<path>
NEXUSIM_IDENTITY_GATEWAY_TOKEN_KEY_ID=<kid>
NEXUSIM_IDENTITY_GATEWAY_TOKEN_ISSUER=nexusim-identity
```

push-gateway 对应配置：

```text
NEXUSIM_PUSH_AUTH_MODE=jwt
NEXUSIM_PUSH_AUTH_JWKS_JSON=<jwks-json>
# or
NEXUSIM_PUSH_AUTH_JWKS_FILE=<path>
# or
NEXUSIM_PUSH_AUTH_JWKS_URL=http://identity-service:10601/.well-known/jwks.json
NEXUSIM_PUSH_AUTH_JWKS_REFRESH_INTERVAL=5m
NEXUSIM_PUSH_AUTH_TRUSTED_ISSUERS=nexusim-identity
```

identity debug server 会暴露 `/.well-known/jwks.json` 和 `/jwks.json`。该 JWKS 只用于 RS256 公钥发现：HS256 对称密钥只能通过双方本地配置共享，不会作为 `oct` JWK 暴露。RS256 JWK 只包含公钥 `n/e`，可供 push-gateway 本地验签。identity-service 可用 `NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_JSON` / `NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_FILE` 在轮换窗口额外暴露旧公钥；额外 JWKS 只接受 RS256 RSA 公钥，当前签名 key 的 `kid` 优先。`NEXUSIM_PUSH_AUTH_JWKS_URL` 会在启动时拉取一次；如果没有静态 recovery 且拉取失败则 fail-closed，如果已有静态 key set 则记录失败并继续启动。后台定期刷新失败时保留上一份可用 key set，并在 `/debug/metrics.auth_jwks` 记录失败计数。当前仍不等于生产级自动轮换、KMS/HSM 私钥托管或完整 issuer federation。

Redis route 可选参数：

```text
NEXUSIM_PUSH_ROUTE_BACKEND=redis
NEXUSIM_PUSH_GATEWAY_ID=push-gateway-a
NEXUSIM_PUSH_REDIS_MODE=single
NEXUSIM_PUSH_REDIS_ADDR=127.0.0.1:6379
NEXUSIM_PUSH_REDIS_USERNAME=
NEXUSIM_PUSH_REDIS_PASSWORD=
NEXUSIM_PUSH_REDIS_DB=0
NEXUSIM_PUSH_REDIS_KEY_PREFIX=nexusim:push
NEXUSIM_PUSH_ROUTE_TTL=90s
NEXUSIM_PUSH_ROUTE_CLEANUP_INTERVAL=30s
```

Redis Sentinel 可选参数：

```text
NEXUSIM_PUSH_REDIS_MODE=sentinel
NEXUSIM_PUSH_REDIS_SENTINEL_MASTER_NAME=mymaster
NEXUSIM_PUSH_REDIS_SENTINEL_ADDRS=127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381
NEXUSIM_PUSH_REDIS_USERNAME=
NEXUSIM_PUSH_REDIS_PASSWORD=
NEXUSIM_PUSH_REDIS_SENTINEL_USERNAME=
NEXUSIM_PUSH_REDIS_SENTINEL_PASSWORD=
NEXUSIM_PUSH_REDIS_DB=0
```

当前代码已支持 `single` 和 `sentinel` 两种 Redis client 模式。Sentinel 模式只表示 push-gateway 通过 Sentinel 发现当前 Redis master；它不改变 route / resume 的业务语义：Redis route 仍是 best-effort online wakeup，Redis resume 仍是短时体验优化，可靠投递仍必须回到 `PullInbox / AckDelivery`。本地三 Redis / 三 Sentinel discovery 正常路径 smoke 已通过；手动 `SENTINEL failover mymaster` 后的 route / resume recovery smoke 已通过；停止 Sentinel 当前 master 容器后由 Sentinel 自主选主的 route / resume recovery smoke 也已通过。但这仍不应表述为 Redis HA 已验收，因为 quorum 异常、网络分区、Redis Cluster、切主窗口内零丢失和容量结论尚未覆盖。

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
10496  push-gateway WebSocket and /debug/metrics
11913  push-gateway local Prometheus /metrics when using NEXUSIM_PUSH_DEBUG_ADDR
10497  delivery-service gRPC dependency
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
- Gateway token 可由 identity-service 签发；legacy HMAC token 和 JWT HS256 token 均可被 push-gateway 本地验证；device / session revoke 事件可异步投影到 gateway deny-list，旧 token 建连返回稳定 `PERMISSION_DENIED`；device revoke 和 session revoke 的在线旧连接 active close 已有真实进程 smoke。
- 小规模 smoke 报告归档到 `docs/runbook/loadtest/push-gateway/`。
- 不把 smoke 表述为完整生产 WebSocket 平台或容量结论。
