# push-gateway Full Smoke Report - 2026-06-09

## 结论

本轮在 clean commit `984080d` 跑通了 `push-gateway` 第一条真实在线通知链路：

```text
CreateMemberChange(JOIN receiver)
-> message-service outbox relay
-> Kafka conversation timeline topic
-> delivery-service timeline consumer
-> SendMessage(owner)
-> delivery-service writes user_inbox + delivery_outbox
-> delivery-service outbox relay
-> Kafka im.delivery.events
-> push-gateway all mode
-> online WebSocket client receives delivery.notify
-> client PullInbox reads durable inbox
-> client sends delivery.ack
-> push-gateway calls delivery-service AckDelivery
-> client receives delivery.ack.ok
```

这证明 `push-gateway` 已经不是孤立 WebSocket demo，而是接在 `delivery-service` durable read model 之后的在线唤醒层。

## 本轮范围

本轮只验证单实例、单用户、单设备、单条消息的 happy path smoke。

不验证：

- 大规模 WebSocket 长连接。
- 多实例 Redis route。
- resume buffer。
- 慢连接主动关闭。
- 真实鉴权。
- push-gateway 容量上限。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `984080d5678a9a3f1ed5f3a5654020aabb056438` |
| git dirty | `false` |
| PostgreSQL | Docker `nexusim-postgres` |
| Kafka | Docker `nexusim-kafka` |
| Schema Registry | `localhost:18081` |
| Kafka UI | `localhost:9090` |
| 原始结果 | `H:\NexusIM\loadtest-results\push-gateway-smoke-20260609-165929\pushgateway-summary.json` |

本轮启动的真实进程：

| 进程 | 地址 / 说明 |
| --- | --- |
| conversation-service gRPC | `127.0.0.1:11596` |
| message-service gRPC | `127.0.0.1:11595` |
| message-service outbox relay | 发布到临时 timeline topic |
| delivery-service timeline consumer | 消费临时 timeline topic |
| delivery-service gRPC | `127.0.0.1:11597` |
| delivery-service outbox relay | 发布到 `im.delivery.events` |
| push-gateway all mode | WebSocket `ws://127.0.0.1:11598`，同时消费 `im.delivery.events` |

## 如何执行

执行入口：

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1
```

脚本做了这些事：

1. 编译 `conversation-service`、`message-service`、`delivery-service`、`push-gateway` 和 `loadtest/pushgateway`。
2. 创建本轮临时 conversation timeline topic，避免 delivery timeline consumer 重放历史事件。
3. 将本轮 push consumer group 的 `im.delivery.events` offset 初始化到 latest，避免在线网关重放旧 push 事件。
4. 启动真实服务进程。
5. runner seed 一个 conversation owner。
6. WebSocket client 先连接 push-gateway，并完成 `client.hello -> server.hello`。
7. 调用 `CreateMemberChange(JOIN receiver)`，等待 delivery membership projection。
8. 调用 `SendMessage(owner)`。
9. 等待 WebSocket 收到 `delivery.notify`。
10. 调用 `PullInbox` 验证 durable inbox。
11. 通过 WebSocket 发送 `delivery.ack`，等待 `delivery.ack.ok`。
12. 查询 PostgreSQL 验证 cursor 和 delivery outbox 状态。

## 结果

核心结果：

| 指标 | 值 |
| --- | --- |
| success | `true` |
| member boundary seq | `1` |
| message conversation seq | `2` |
| WebSocket notify op | `delivery.notify` |
| notify conversation seq | `2` |
| PullInbox item count | `1` |
| Ack response op | `delivery.ack.ok` |
| cursor last_received_seq | `2` |
| user_inbox_count | `1` |
| delivery_outbox_total | `2` |
| delivery_outbox_published | `2` |
| delivery_outbox_pending | `0` |
| delivery_outbox_dlq | `0` |

关键帧：

```json
{
  "op": "delivery.notify",
  "event_id": "evt_delivery_inbox_b4bcf54bf01bd496288a09eb1f435ec4",
  "conversation_id": "conv-push-smoke",
  "conversation_seq": 2,
  "source_event_id": "368c5887-ce55-49a1-9ae6-3c857301e251",
  "message_id": "msg_f5e87559-7881-4a9f-928c-00c85c9fb900",
  "pull_required": true
}
```

ACK 成功帧：

```json
{
  "op": "delivery.ack.ok",
  "request_id": "push-smoke-ack",
  "conversation_id": "conv-push-smoke",
  "last_received_seq": 2
}
```

## 排查过程

本轮先修复了评审指出的 P1：

- delivery-service 返回 `PermissionDenied` 时，push-gateway 原先会对外映射成 `SERVER_BUSY retryable=true`。
- 这会让客户端对无权限 ACK 进行无意义重试。
- 修复后对外返回 `PERMISSION_DENIED retryable=false`，并补了 WebSocket 级测试。

第一次真实 smoke 失败在 `wait notify`：

- PostgreSQL 显示 `message_outbox` 已经 `PUBLISHED=2`。
- `delivery_outbox` 已经存在 `delivery.inbox_item.created.v1` 并且为 `PUBLISHED`。
- `user_inbox` 已经写入 receiver 的消息。
- push consumer group offset 已推进，说明 Kafka 消费发生过。

继续排查后确认两个问题：

1. `im.delivery.events` 是固定 topic，历史数据较多。新的 push consumer group 如果从 topic 头部开始，会先追历史事件，短 smoke 可能等不到本轮 notify。
2. smoke runner 用 200ms 超时循环读取 WebSocket，读取消会干扰 nhooyr 的连接读状态。

修复策略：

- push-gateway Kafka reader 对没有提交 offset 的 group 使用 latest 起始语义；这符合在线网关定位，历史缺口由 `PullInbox` 兜底。
- smoke 脚本在启动 push-gateway 前显式把本轮 consumer group offset 初始化到 latest。
- runner 改为一次性用 `wait-timeout` 等待 WebSocket notify，不再反复取消 read。

修复后 clean commit `984080d` 的 full smoke 通过。

## 面试可讲

可以这样描述：

> 我没有让 WebSocket 网关直接推完整消息正文，也没有让它写 ACK 游标。消息事实先进入 message-service 的本地事务和 outbox，再进入 delivery-service 的 user_inbox。push-gateway 只消费 delivery-service 发布的轻量 delivery event，把在线客户端唤醒。客户端收到 `delivery.notify` 后仍然回源 `PullInbox` 获取权威数据，再通过 `delivery.ack` 让 push-gateway 调用 delivery-service 的 `AckDelivery`。这样即使 WebSocket 断线、网关重启或在线通知丢失，客户端也能靠 durable inbox 补拉，不会丢消息。

这轮能证明：

- 第四个真实微服务 `push-gateway` 已接入前面三个服务的事件链。
- `push-gateway` 不拥有 durable inbox，不直接写 cursor。
- 在线通知和可靠投递被分层：WebSocket 负责实时性，delivery-service 负责可靠性。
- Kafka consumer offset 策略按在线网关特点处理：不重放历史 push 事件，历史缺口由客户端补拉。

## 剩余风险

- WebSocket auth 仍是 query/mock token，不是生产身份校验。
- `all` 模式只适合本地单进程 smoke，多实例需要 Redis route。
- 慢连接队列满时当前是从 registry 驱逐，不是主动关闭 WebSocket。
- resume buffer 尚未实现。
- 本轮只验证单用户单设备，不是容量压测。

## 下一步

1. 邀请独立评审复核 `push-gateway` full smoke。
2. 后续可进入 `push-gateway` 多设备 smoke，验证同一 user 多 device 都收到 notify。
3. 再进入 Redis route / resume buffer / slow session close 的生产化切片。
