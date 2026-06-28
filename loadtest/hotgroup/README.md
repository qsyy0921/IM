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
-> wait user_inbox fanout
-> sampled PullInbox
-> sampled AckDelivery
-> summary/report/users.jsonl
```

暂不覆盖：

```text
WebSocket notify storm
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
message outbox relay、delivery timeline consumer、PostgreSQL 和 Kafka。

结果默认写入：

```text
H:\NexusIM\loadtest-results\<run-name>\
```
