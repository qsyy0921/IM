# delivery-service LEAVE / REMOVE Negative Visibility Smoke

## 目标

验证 `delivery-service` 的成员可见窗口在真实 timeline consumer 中生效：

```text
CreateMemberChange(JOIN)
-> message_outbox
-> message-service outbox relay
-> Kafka conversation timeline topic
-> delivery-service timeline-consumer
-> delivery_membership_projection ACTIVE
-> pre-boundary message event
-> user_inbox visible
-> CreateMemberChange(LEAVE / REMOVE)
-> message-service outbox relay
-> delivery_membership_projection non-ACTIVE
-> post-boundary message event
-> active sender receives inbox
-> left/removed target does not receive inbox
```

这是小规模 smoke，不是容量压测。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `a87fc3f` |
| git_dirty | `false` |
| PostgreSQL | Docker `nexusim-postgres`，`localhost:5432` |
| Kafka | Docker `nexusim-kafka` |
| timeline topic | `conversation.timeline.visibility.20260609-152208` |
| delivery consumer group | `nexusim-delivery-visibility-20260609-152208` |
| conversation-service gRPC | `127.0.0.1:14496` |
| message-service outbox relay | 本机进程，workers `2`，batch size `100` |
| delivery-service timeline-consumer | 本机进程 |
| delivery-service gRPC | `127.0.0.1:14497` |
| tenant | `tenant-delivery-visibility-20260609-152208` |

原始结果目录：

```text
loadtest/results/delivery-visibility-20260609-152208/
```

## 方法说明

本 smoke 使用新增 runner：

```text
loadtest/deliveryvisibility
```

为了只验证 delivery read model 的成员窗口，不受 `message-service` 第一阶段 static policy version mock 影响，本轮做了一个明确取舍：

- 成员 JOIN / LEAVE / REMOVE 仍走真实 `conversation-service CreateMemberChange`。
- 成员边界事件仍通过 `message_outbox -> message-service outbox relay -> Kafka` 发布。
- pre/post message event 使用 runner 按 `conversation.timeline.events` protobuf 合约写入临时 Kafka topic。
- delivery-service timeline consumer 消费同一个临时 topic，并写入 `delivery_membership_projection` / `user_inbox`。

因此本轮证明的是：

```text
delivery-service 在消费到边界后的 message event 时，不会继续给 LEFT/REMOVED 用户写 user_inbox。
```

不把它解释成 `message-service SendMessage` 的容量或尾延迟结果。

## 启动拓扑

```powershell
docker exec nexusim-kafka kafka-topics `
  --bootstrap-server localhost:9092 `
  --create --if-not-exists `
  --topic conversation.timeline.visibility.20260609-152208 `
  --partitions 3 `
  --replication-factor 1
```

```powershell
$env:NEXUSIM_CONVERSATION_SERVICE_MODE='grpc'
$env:NEXUSIM_CONVERSATION_GRPC_ADDR='127.0.0.1:14496'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
bin\conversation-service.exe
```

```powershell
$env:NEXUSIM_MESSAGE_SERVICE_MODE='outbox-relay'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
$env:NEXUSIM_KAFKA_BROKERS='localhost:9092'
$env:NEXUSIM_KAFKA_TOPIC='conversation.timeline.visibility.20260609-152208'
$env:NEXUSIM_OUTBOX_BATCH_SIZE='100'
$env:NEXUSIM_OUTBOX_WORKERS='2'
$env:NEXUSIM_OUTBOX_POLL_INTERVAL='100ms'
bin\message-service.exe
```

```powershell
$env:NEXUSIM_DELIVERY_SERVICE_MODE='timeline-consumer'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
$env:NEXUSIM_KAFKA_BROKERS='localhost:9092'
$env:NEXUSIM_TIMELINE_TOPIC='conversation.timeline.visibility.20260609-152208'
$env:NEXUSIM_DELIVERY_CONSUMER_GROUP='nexusim-delivery-visibility-20260609-152208'
bin\delivery-service.exe
```

```powershell
$env:NEXUSIM_DELIVERY_SERVICE_MODE='grpc'
$env:NEXUSIM_DELIVERY_GRPC_ADDR='127.0.0.1:14497'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
bin\delivery-service.exe
```

## 执行命令

```powershell
bin\deliveryvisibility-loadtest.exe `
  --conversation-target 127.0.0.1:14496 `
  --delivery-target 127.0.0.1:14497 `
  --kafka-brokers localhost:9092 `
  --timeline-topic conversation.timeline.visibility.20260609-152208 `
  --consumer-group nexusim-delivery-visibility-20260609-152208 `
  --pg-dsn 'postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable' `
  --tenant-id tenant-delivery-visibility-20260609-152208 `
  --conversation-prefix conv-delivery-visibility-20260609-152208 `
  --result-dir loadtest\results\delivery-visibility-20260609-152208 `
  --wait-timeout 20s `
  --request-timeout 5s `
  --poll-interval 200ms
```

## 通过标准

每个场景都必须满足：

```text
target_pre_inbox_count > 0
membership_status != ACTIVE
membership_leave_seq == boundary_seq
sender_post_inbox_count > 0
target_post_inbox_count == 0
PullInbox(after_seq=boundary_seq).item_count == 0
```

这里的关键不是“当前 PullInbox 为空”，而是：

```text
边界后的 message event 已被 active sender 收到，但 LEFT/REMOVED 目标用户没有任何 conversation_seq > boundary_seq 的 user_inbox。
```

## 核心结果

| 场景 | boundary_seq | membership_status | target_pre_inbox | sender_post_inbox | target_post_inbox | pull_after_boundary |
| --- | ---: | --- | ---: | ---: | ---: | ---: |
| `LEAVE` | `4` | `LEFT` | `1` | `1` | `0` | `0` |
| `REMOVE` | `4` | `LEFT` | `1` | `1` | `0` | `0` |

`REMOVE` 当前显示为 `LEFT` 是符合当前契约的：第一版 `REMOVE` 表示可再加入的移除，不是永久封禁。

## 排查方法

本轮按以下顺序排查断点：

1. 先用 `CreateMemberChange(JOIN)` 建立 sender 和 target 的成员边界。
2. 等待 delivery consumer 把两人都投影为 `ACTIVE`。
3. 写入 pre-boundary message event，等待 target 用户出现对应 `user_inbox`，证明正向可见。
4. 执行 `LEAVE` 或 `REMOVE`。
5. 等待 `delivery_membership_projection.status` 变为非 `ACTIVE`，并确认 `leave_seq=boundary_seq`。
6. 写入 post-boundary message event。
7. 等待 active sender 收到 post-boundary inbox，证明该 message event 已被 delivery consumer 消费。
8. 查询 target 用户 `conversation_seq > boundary_seq` 的 `user_inbox`，并调用 `PullInbox(after_seq=boundary_seq)`，均为 0。

## 当前结论

- `delivery-service` 的成员窗口不只在 repository 集成测试中成立，也已经通过真实 Kafka consumer smoke 验证。
- LEAVE / REMOVE 后不会删除历史 inbox；历史消息仍可见，边界后的新消息不再投递给该用户。
- 这为 push-gateway 提供了前置保证：push-gateway 后续只应消费 delivery event / 查询 delivery read model，不能直接绕过 `user_inbox` 和成员窗口。

## 剩余风险

- 本轮 post message event 是按 timeline protobuf 合约直接写入临时 Kafka topic，用于隔离 delivery 投影行为；不是 `SendMessage` 容量验证。
- projection DLQ / repair 仍未实现，坏事件仍会 fail-closed 阻塞 consumer。
- 更完整的历史 repair 仍建议后续引入 membership window/history 表；当前 latest projection 已覆盖顺序消费 smoke。

## 下一步

1. 更新 `current-goal.md`，把 delivery negative visibility 标为已完成。
2. 进入 `push-gateway` SDD，明确在线推送只依赖 delivery read model / delivery event。
3. SDD 冻结后再进入 push-gateway proto / 六层骨架 / 最小在线推送链路。
