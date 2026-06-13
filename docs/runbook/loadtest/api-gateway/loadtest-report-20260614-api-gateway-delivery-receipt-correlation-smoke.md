# api-gateway delivery / receipt correlation smoke

日期：2026-06-14

目标：验证 api-gateway 注入的 `x-nexusim-trace-id` / `x-nexusim-request-id` 能被 delivery-service 与 receipt-service 的 gRPC access log 读取并输出。该 smoke 复用 secure E2E facade 链路，不是完整 OpenTelemetry trace、collector 或告警验证。

## 运行信息

- 代码 commit：`50685b4`
- 工作区：`git_dirty=false`
- 结果目录：`H:\NexusIM\loadtest-results\e2e-demo-api-gateway-delivery-receipt-correlation-smoke-20260614-clean`
- runner：`.\loadtest\demo\run-local-secure-demo.ps1 -RunName e2e-demo-api-gateway-delivery-receipt-correlation-smoke-20260614-clean`
- 入口：`GatewayService` facade，`gateway_auth_mode=hmac`
- TLS：conversation / message / delivery / receipt / push 均启用本地短期证书

## 结果

summary 显示：

```text
success=true
gateway_facade=true
gateway_auth_mode=hmac
send_message.conversation_seq=2
pull_inbox.item_count=1
websocket_ack.last_received_seq=2
mark_read.last_read_seq=2
list_conversations_before_read.unread_count=1
list_conversations_after_read.unread_count=0
```

## Correlation 证据

api-gateway access log：

```text
/nexusim.gateway.v1.GatewayService/PullInbox trace_id=e2e-demo-pull request_id=e2e-demo-pull
/nexusim.gateway.v1.GatewayService/ListConversations trace_id=e2e-demo-list request_id=e2e-demo-list
/nexusim.gateway.v1.GatewayService/MarkRead trace_id=e2e-demo-mark-read request_id=e2e-demo-mark-read
```

delivery-service access log：

```text
/nexusim.delivery.v1.DeliveryService/PullInbox code=OK trace_id=e2e-demo-pull request_id=e2e-demo-pull
```

receipt-service access log：

```text
/nexusim.receipt.v1.ReceiptService/ListConversations code=OK trace_id=e2e-demo-list request_id=e2e-demo-list
/nexusim.receipt.v1.ReceiptService/MarkRead code=OK trace_id=e2e-demo-mark-read request_id=e2e-demo-mark-read
```

说明：`AckDelivery` 在该 demo 中由 push-gateway 直接调用 delivery-service，不经过 api-gateway，因此本轮 api-gateway correlation 证据聚焦 `PullInbox`、`ListConversations` 和 `MarkRead`。

## 边界

- 已覆盖：api-gateway -> delivery-service / receipt-service 的 gRPC metadata correlation 与低敏 access log 输出。
- 未覆盖：Kafka envelope correlation、统一 trace/span、OpenTelemetry collector、生产日志采集、告警和跨进程 trace 采样策略。
- access log 只记录 `service/event/method/code/latency_ms/trace_id/request_id`，不记录 token、请求体、tenant/user/device/session 字段或 authorization metadata。
