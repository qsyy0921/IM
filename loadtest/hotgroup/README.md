# loadtest/hotgroup

`loadtest/hotgroup` 是热点群聊端到端业务压测 runner。它不替代
`loadtest/sendmessage` 的单服务基线，而是把 conversation / message / delivery
链路串起来验证群聊 fanout。

当前 v0.1 覆盖：

```text
CreateConversation(GROUP)
-> batch CreateMemberChange(JOIN)
-> SendMessage
-> wait delivery membership projection
-> wait user_inbox fanout or delivery_timeline_items
-> optional WebSocket conversation.subscribe / delivery.notify signal verification
-> sampled PullInbox
-> sampled AckDelivery
-> summary/report/users.jsonl
```

热点群准备状态：

```text
conversation-service fanout policy
-> timeline-service seq block allocator / lease status / gap marker repair operator
-> message-service SEQUENCER_BLOCK local seq block cache
-> delivery-service READ_FANOUT / BROADCAST_SIGNAL projection
-> push-gateway conversation signal notify
```

注意：本 runner 仍只记录压测事实，不负责自动修复 gap 或自动调整群规模策略。
正式三机压测前需要先重建最新 Docker 镜像并重新部署服务容器。

暂不覆盖：

```text
slow client active close
Redis route fault
member churn during send
delivery outage recovery
```

Dry-run：

```powershell
. .\tools\go-env.ps1
go run .\loadtest\hotgroup `
  --dry-run `
  --run-name hotgroup-review-dryrun `
  --group-size 100 `
  --sender-count 5 `
  --message-count 20
```

真实执行前必须启动 conversation-service、message-service、delivery-service、
push-gateway、message outbox relay、delivery timeline consumer、delivery outbox relay、
PostgreSQL 和 Kafka。

热点群 `BROADCAST_SIGNAL` 路径示例：

```powershell
. .\tools\go-env.ps1
go run .\loadtest\hotgroup `
  --run-name hotgroup-broadcast-push-smoke `
  --conversation-target 172.31.50.2:10496 `
  --message-target 172.31.50.2:10495 `
  --delivery-target 172.31.50.2:10497 `
  --push-url ws://172.31.50.2:10498/ws `
  --pg-dsn "postgres://nexusim:nexusim@172.31.50.2:5432/nexusim?sslmode=disable" `
  --group-size 61 `
  --sender-count 4 `
  --message-count 20 `
  --expect-fanout-mode BROADCAST_SIGNAL `
  --conversation-subscriber-count 3 `
  --require-conversation-notify `
  --require-delivery-outbox-drain
```

结果默认写入：

```text
H:\NexusIM\loadtest-results\<run-name>\
```
