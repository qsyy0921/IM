# presence-service SDD v0.1 Draft

## 1. 服务定位

`presence-service` 是 NexusIM 的在线状态和实时活动投影服务。它负责用户在线、
设备在线、输入中、最后在线时间、presence 隐私过滤、订阅和广播。

职责：

- 拥有 `presence_session`、`presence_user_state`、`presence_subscription`、
  `typing_indicator` 和 `presence_outbox`。
- 消费 push-gateway 连接 / 断开 / heartbeat 事件，或接收显式 heartbeat。
- 聚合用户 / 设备在线状态和 last_seen_at。
- 提供 `GetPresence`、`UpdatePresence`、`SubscribePresence` 和 typing 状态接口。
- 根据 contacts / privacy / policy 过滤 presence 可见性。

不负责：

- 不拥有 push-gateway WebSocket route，不替代 Redis route / resume buffer。
- 不拥有 durable inbox、delivery ACK、message delivery 或 conversation timeline。
- 不决定成员权限、消息可见性、是否允许发消息或是否应该推送。
- 不作为强一致在线事实源；presence 是 near-real-time UX signal。
- 不保存 message body、token、device secret、IP 明文或完整 user-agent 明文。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游 | push-gateway | session connected/disconnected/heartbeat 事件 |
| 上游 | clients / api-gateway | typing、manual status、presence subscribe |
| 同步依赖 | contacts-service / policy-service | presence visibility / block / privacy gate |
| 同步依赖 | Redis | 热状态、TTL、fanout cache（可选） |
| 异步下游 | push-gateway / notification / audit | presence changed / typing changed 事件 |
| 事实源 | PostgreSQL + Redis | durable last_seen / subscriptions + hot TTL state |

push-gateway 仍负责在线连接和 delivery notification；presence-service 只保存和广播产品
层在线状态，不参与 `delivery.notify` 路由决策。

## 3. 六层 DDD 包结构

```text
services/presence-service/
  cmd/presence-service/
  internal/{api,app,domain,infrastructure,types,trigger}/
```

| 层 | 本服务内容 |
| --- | --- |
| `api` | gRPC / streaming adapter，verified metadata，稳定错误映射 |
| `app` | UpdatePresence、GetPresence、SubscribePresence、UpdateTyping |
| `domain` | presence 状态机、TTL、visibility、typing debounce |
| `infrastructure` | PostgreSQL repository、Redis state adapter、contacts/policy clients |
| `types` | command、DTO、错误码、枚举 |
| `trigger` | push session consumer、subscription broadcaster、cleanup worker、outbox relay |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `PresenceSession` | 一个在线设备 / gateway session 投影 | 带 TTL；过期自动离线 |
| `PresenceUserState` | 用户聚合在线状态 | 从 device/session 推导；last_seen 单调 |
| `PresenceSubscription` | 谁订阅谁的 presence | 必须经过 visibility gate |
| `TypingIndicator` | conversation-scoped 输入中状态 | 短 TTL；不持久保存输入内容 |
| `PresenceOutboxEvent` | presence 事件 | 只通过 outbox relay 发布 |

Presence 状态：

```text
OFFLINE
ONLINE
AWAY
DO_NOT_DISTURB
INVISIBLE
```

Device 状态：

```text
CONNECTED -> HEARTBEAT_ACTIVE -> STALE -> DISCONNECTED
CONNECTED/HEARTBEAT_ACTIVE -> REVOKED
```

`INVISIBLE` 是用户隐私设置，不代表真实设备一定离线；对无权观察者返回
`OFFLINE` 或 `UNKNOWN`。

## 5. 同步 API 契约

```text
rpc UpdatePresence(UpdatePresenceRequest) returns (UpdatePresenceResponse)
rpc GetPresence(GetPresenceRequest) returns (GetPresenceResponse)
rpc SubscribePresence(SubscribePresenceRequest) returns (stream PresenceEvent)
rpc UpdateTyping(UpdateTypingRequest) returns (UpdateTypingResponse)
```

`UpdatePresence` 请求字段：

```text
tenant_id, user_id, device_id, session_id
presence_state, manual_status, ttl_ms
source: PUSH_GATEWAY | CLIENT | OPERATOR
idempotency_key
correlation_id, causation_id, trace_id
```

`GetPresence` 请求字段：

```text
tenant_id, requester_user_id
target_user_ids[]
conversation_id
include_devices
```

响应字段：

```text
target_user_id, visible_state, last_seen_at, device_count,
device_states[], visibility_decision
```

`UpdateTyping` 请求字段：

```text
tenant_id, conversation_id, user_id, device_id
typing_state: STARTED | STOPPED
ttl_ms
```

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | state、TTL、target list、conversation 或字段非法 | 否 |
| `PERMISSION_DENIED` | requester 不可见 target presence | 否 |
| `FAILED_PRECONDITION` | conversation / device / session 状态不允许 | 否 |
| `NOT_FOUND` | target / subscription 不存在或不可见 | 否 |
| `UNAVAILABLE` | Redis / policy / contacts 暂不可用 | 是 |

## 6. 异步事件契约

| 事件 | Topic | 分区键 | 说明 |
| --- | --- | --- | --- |
| `presence.user.changed.v1` | `im.presence.events` | `tenant_id:user_id` | 用户聚合状态变化 |
| `presence.device.changed.v1` | `im.presence.events` | `tenant_id:user_id` | 设备状态变化 |
| `presence.typing.changed.v1` | `im.presence.events` | `tenant_id:conversation_id` | typing 状态变化 |
| `presence.subscription.changed.v1` | `im.presence.events` | `tenant_id:subscriber_user_id` | 订阅变化 |

事件 payload 只包含低敏状态、TTL、masked device class、conversation ref、trace /
correlation refs。禁止包含 IP、user-agent 原文、token、device secret、message draft 或 typing 内容。

## 7. 数据库 / 热状态设计

PostgreSQL 第一版表：

```text
presence_user_states
presence_sessions
presence_subscriptions
presence_outbox
```

Redis 热状态第一版 key：

```text
presence:session:{tenant_id}:{session_id} -> session state, TTL
presence:user:{tenant_id}:{user_id}:sessions -> set(session_id), TTL
presence:typing:{tenant_id}:{conversation_id}:{user_id}:{device_id} -> typing state, TTL
presence:subscribers:{tenant_id}:{target_user_id} -> set(subscriber_user_id), TTL
```

PostgreSQL 保存 durable last_seen、manual presence、subscription 和 outbox。Redis 保存
短 TTL 热状态；Redis 丢失时服务可退化为 `UNKNOWN/OFFLINE + last_seen`，不得影响消息投递。

## 8. 核心流程

Push session connected：

```text
push-gateway session event
-> presence consumer validates event
-> upsert presence_sessions CONNECTED with TTL
-> recompute presence_user_states ONLINE
-> write presence.user.changed.v1 if aggregate changed
```

Heartbeat / stale：

```text
heartbeat
-> extend Redis TTL
-> update last_seen_at with monotonic guard
-> stale scanner marks expired sessions STALE/DISCONNECTED
```

GetPresence：

```text
GetPresence
-> load target states
-> call contacts/policy visibility gate
-> apply privacy / invisible / block filters
-> return visible state only
```

Typing：

```text
UpdateTyping STARTED
-> verify conversation visibility
-> write Redis typing key with short TTL
-> publish presence.typing.changed.v1
```

## 9. 可见性和隐私

Visibility gate 第一版：

```text
same tenant
AND not blocked
AND (direct contact active OR same active conversation member OR admin/operator scope)
AND target privacy allows requester
```

输出策略：

| 目标设置 | 有权观察者 | 无权观察者 |
| --- | --- | --- |
| ONLINE / AWAY / DND | 返回真实可见状态 | `UNKNOWN` 或 `OFFLINE` |
| INVISIBLE | `OFFLINE` + last_seen policy | `OFFLINE` |
| BLOCKED | `PERMISSION_DENIED` 或 `OFFLINE` | `OFFLINE` |

Typing 状态只对同一 conversation 当前可见成员广播；成员离开后不再收到 typing。

## 10. 一致性和事务

强一致边界：

- PostgreSQL session/user state/outbox 同事务更新。
- subscription create/delete 和 outbox 同事务。
- manual presence setting 和 user state 同事务。

最终一致边界：

- Redis TTL 过期驱动离线；stale scanner 负责补偿。
- push-gateway 连接事件到 presence-service 是最终一致；短窗口内状态可能过期或延迟。
- contacts / policy projection 延迟时，GetPresence 必须 fail closed 或返回 `UNKNOWN`，不能泄露在线状态。

## 11. 幂等、重试和补偿

| 场景 | 幂等键 | 重试策略 | 补偿 |
| --- | --- | --- | --- |
| UpdatePresence | session_id + state_version | replay 返回当前 state | stale scanner |
| Session event consumer | source_event_id | DB append idempotent | checkpoint rewind |
| UpdateTyping | conversation + user + device | last-write-wins within TTL | TTL auto-expire |
| SubscribePresence | subscriber + target | idempotent upsert | subscription cleanup |
| OutboxRelay | event_id | bounded retry + DLQ | outbox repair operator |

## 12. 安全边界

- API 必须使用 gateway verified metadata；request body 不能覆盖 trusted tenant/user。
- push-gateway session events 必须带 service identity 或 signed envelope。
- device_id、session_id 在 metrics / events 中必须 hash 或省略。
- typing event 不保存输入内容、光标位置或 draft。
- IP / user-agent 只能在 push-gateway 本地低敏统计中使用，presence-service 不存原文。
- Redis key 必须有 tenant prefix 和 TTL；不能把 Redis 当长期事实源。

## 13. SLO 和指标

第一阶段不写生产 SLO，只保留本地 / 面试展示指标：

```text
presence_user_state_total{visible_state}
presence_session_total{state,source}
presence_typing_total{state}
presence_subscription_total{status}
presence_visibility_denied_total{reason}
presence_outbox_total{status}
```

metrics 禁止输出 tenant_id、user_id、device_id、session_id、conversation_id、IP、
user-agent、trace_id 或 request_id。

## 14. 测试方案

| 测试 | 目标 |
| --- | --- |
| domain unit | session TTL、aggregate state、invisible / privacy filter |
| app unit | GetPresence visibility、typing debounce、idempotency |
| PostgreSQL integration | user state / session / subscription / outbox 同事务 |
| Redis adapter test | TTL expiry、set cleanup、cluster hash tag |
| consumer test | push event malformed fail closed、checkpoint only after DB commit |
| smoke | session connected -> GetPresence ONLINE -> disconnect/stale -> OFFLINE |

## 15. Runbook

运行模式：

```text
NEXUSIM_PRESENCE_SERVICE_MODE=grpc
NEXUSIM_PRESENCE_SERVICE_MODE=session-consumer
NEXUSIM_PRESENCE_SERVICE_MODE=stale-scanner
NEXUSIM_PRESENCE_SERVICE_MODE=outbox-relay
NEXUSIM_PRESENCE_SERVICE_MODE=cleanup
```

operator：

```text
presence-session-audit
presence-session-expire
presence-subscription-audit
presence-outbox-repair
```

## 16. 验收标准

进入编码前：

- 本 SDD v0.1 draft 被复核，无 P0/P1。
- `presence-service` brief 指向本 SDD。
- 明确 presence 不是 push route、delivery、ACK 或权限事实源。

进入 first smoke 前：

- proto / migration / 六层 skeleton / cmd runtime 已落。
- PostgreSQL + Redis adapter + visibility gate tests 通过。
- `UpdatePresence -> GetPresence -> UpdateTyping -> SubscribePresence` 本地 smoke 通过。
- Redis 丢失或 projection 不可用时不泄露在线状态，不影响 `PullInbox / AckDelivery`。
