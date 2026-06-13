# NexusIM E2E Demo Runner

`loadtest/demo` 是本地端到端演示入口，不是新的微服务，也不是容量压测工具。

它通过公开 gRPC / WebSocket 接口串起一条最小 IM 用户路径：

```text
conversation-service CreateMemberChange(JOIN)
-> message-service SendMessage
-> push-gateway WebSocket delivery.notify
-> delivery-service PullInbox
-> push-gateway delivery.ack -> delivery-service AckDelivery
-> receipt-service MarkRead
-> receipt-service ListConversations
```

数据库只用于本地演示的 tenant 清理、seed conversation 和结果证据采集；业务动作不跨服务读取内部表。

默认原始结果写入：

```text
H:\NexusIM\loadtest-results\<run-name>\e2e-demo-summary.json
```

运行前需要 PostgreSQL / Kafka / Redis 以及相关微服务和 relay / consumer 已经启动。

```powershell
.\loadtest\demo\run-local-demo.ps1
```

HMAC push auth 示例：

```powershell
.\loadtest\demo\run-local-demo.ps1 `
  -PushAuthMode hmac `
  -PushAuthHmacSecret "local-demo-secret"
```

gateway verified metadata auth 示例：

```powershell
.\loadtest\demo\run-local-demo.ps1 -VerifiedAuthMetadata
```

该模式会把 demo 请求身份同时写入 user-facing gRPC metadata，用于验证 conversation / message / delivery / receipt 的 `metadata` / `verified-metadata` auth mode；request body 仍保留兼容字段。

api-gateway token auth 示例：

```powershell
.\loadtest\demo\run-local-demo.ps1 `
  -GatewayAuthMode hmac `
  -GatewayAuthHmacSecret "local-gateway-secret" `
  -GatewayAuthAudience api-gateway
```

该模式用于 demo runner 指向 api-gateway 时提交 gateway token，由 api-gateway 验证 token、重写下游 request `AuthContext` 并注入 trusted metadata。默认 audience 是 `api-gateway`；如需兼容历史 token，可显式覆盖，但不要把 `push-gateway` audience 作为新的 api-gateway 默认口径。不要和 `-VerifiedAuthMetadata` 同时使用；前者模拟客户端经 gateway，后者只用于直接打后端服务的 smoke。

api-gateway public facade 示例：

```powershell
.\loadtest\demo\run-local-demo.ps1 `
  -GatewayAuthMode hmac `
  -GatewayAuthHmacSecret "local-gateway-secret" `
  -GatewayAuthAudience api-gateway `
  -GatewayFacade
```

该模式使用 `nexusim.gateway.v1.GatewayService` 调 conversation / message / delivery / receipt 的 user-facing RPC，不再依赖 api-gateway 兼容保留的 legacy service descriptor。facade 不包含服务间 `GetSendContext`。

gRPC mTLS + WebSocket WSS/mTLS 示例：

```powershell
.\loadtest\demo\run-local-demo.ps1 `
  -VerifiedAuthMetadata `
  -ConversationTlsCaFile .\certs\ca.crt `
  -ConversationTlsServerName conversation-service.nexusim.local `
  -ConversationTlsClientCertFile .\certs\api-gateway.crt `
  -ConversationTlsClientKeyFile .\certs\api-gateway.key `
  -MessageTlsCaFile .\certs\ca.crt `
  -MessageTlsServerName message-service.nexusim.local `
  -MessageTlsClientCertFile .\certs\api-gateway.crt `
  -MessageTlsClientKeyFile .\certs\api-gateway.key `
  -DeliveryTlsCaFile .\certs\ca.crt `
  -DeliveryTlsServerName delivery-service.nexusim.local `
  -DeliveryTlsClientCertFile .\certs\api-gateway.crt `
  -DeliveryTlsClientKeyFile .\certs\api-gateway.key `
  -ReceiptTlsCaFile .\certs\ca.crt `
  -ReceiptTlsServerName receipt-service.nexusim.local `
  -ReceiptTlsClientCertFile .\certs\api-gateway.crt `
  -ReceiptTlsClientKeyFile .\certs\api-gateway.key `
  -PushUrl wss://127.0.0.1:10498 `
  -PushTlsCaFile .\certs\ca.crt `
  -PushTlsServerName push-gateway.nexusim.local `
  -PushTlsClientCertFile .\certs\desktop-client.crt `
  -PushTlsClientKeyFile .\certs\desktop-client.key
```

这些参数只验证本地静态证书下的 gRPC TLS/mTLS 和 push-gateway WSS/mTLS 连接；证书签发、轮换、分发、撤销和动态服务身份治理不在 demo runner 范围内。

`run-local-secure-demo.ps1` 会启动真实 api-gateway，把 conversation / message / delivery / receipt 四个 gRPC target 指向 api-gateway，并默认启用 `--gateway-facade`，用 `GatewayAuthMode=hmac` 验证 token -> `GatewayService` facade -> trusted metadata -> 下游 mTLS metadata auth 链路。
该脚本同时启用 api-gateway `/debug/metrics`，并把低敏进程内指标保存为结果目录下的 `api-gateway-debug-metrics.json`。

边界：

- 不自动创建会话产品流程，只 seed 本地 demo conversation。
- 不让 message-service 同步依赖 contacts-service。
- 不把 push-gateway 当作 durable inbox；展示事实仍以 `PullInbox` 为准。
- 不把本 runner 的成功当作生产级容量或 HA 结论。
