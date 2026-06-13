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
| `loadtest-report-20260613-e2e-demo-verified-metadata-smoke.md` | 本地多进程 E2E demo smoke，验证 metadata auth 下的 notify、PullInbox、ACK、MarkRead 和未读数归零 |

## TLS / mTLS 参数

`loadtest/demo` 和 `loadtest/demo/run-local-demo.ps1` 默认仍使用 plaintext gRPC。若外部启动的 conversation-service / message-service / delivery-service / receipt-service 已开启 gRPC TLS 或 mTLS，可给 demo runner 传入对应 client 参数：

```powershell
.\loadtest\demo\run-local-demo.ps1 `
  -ConversationTlsCaFile .\certs\ca.pem `
  -ConversationTlsServerName conversation-service.nexusim.local `
  -ConversationTlsClientCertFile .\certs\loadtest-client.crt `
  -ConversationTlsClientKeyFile .\certs\loadtest-client.key `
  -MessageTlsCaFile .\certs\ca.pem `
  -MessageTlsServerName message-service.nexusim.local `
  -MessageTlsClientCertFile .\certs\loadtest-client.crt `
  -MessageTlsClientKeyFile .\certs\loadtest-client.key `
  -DeliveryTlsCaFile .\certs\ca.pem `
  -DeliveryTlsServerName delivery-service.nexusim.local `
  -DeliveryTlsClientCertFile .\certs\loadtest-client.crt `
  -DeliveryTlsClientKeyFile .\certs\loadtest-client.key `
  -ReceiptTlsCaFile .\certs\ca.pem `
  -ReceiptTlsServerName receipt-service.nexusim.local `
  -ReceiptTlsClientCertFile .\certs\loadtest-client.crt `
  -ReceiptTlsClientKeyFile .\certs\loadtest-client.key
```

这些参数只覆盖 demo runner 到四个 gRPC server 的静态 TLS / mTLS 连接。未配置时保持 plaintext，兼容现有本地演示；证书生命周期和全服务 mTLS rollout 不在 demo runner 范围内。

## Gateway verified metadata auth

如果 conversation / message / delivery / receipt 四个 user-facing gRPC server 以 metadata auth 模式启动，demo runner 可用以下开关发送 gateway verified identity metadata：

```powershell
.\loadtest\demo\run-local-demo.ps1 -VerifiedAuthMetadata
```

该模式会把 demo 请求身份同时写入 user-facing gRPC metadata，用于验证 conversation / message / delivery / receipt 的 `metadata` / `verified-metadata` auth mode；request body 仍保留兼容字段。

2026-06-13 补充：`-VerifiedAuthMetadata` 真实进程 demo smoke 已通过，验证 receiver JOIN、SendMessage、`delivery.notify`、`PullInbox`、WebSocket ACK、`MarkRead` 和 `ListConversations` unread `1 -> 0` 全链路。
