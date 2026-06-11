# NexusIM E2E Demo Smoke Index

本文归档跨服务端到端演示，不属于单个微服务容量压测。

当前 demo 链路：

```text
CreateMemberChange(JOIN)
-> SendMessage
-> delivery.notify
-> PullInbox
-> delivery.ack
-> MarkRead
-> ListConversations
```

报告：

| 报告 | 说明 |
| --- | --- |
| `loadtest-report-20260612-e2e-demo-smoke.md` | 本地多进程 E2E demo smoke，验证投递后未读数为 1，ACK + MarkRead 后未读数为 0 |
