# api-gateway message-service correlation smoke - 2026-06-14

## 结论

本轮 smoke 证明 api-gateway 注入的 `trace_id` / `request_id` 已经贯通到 message-service 的 gRPC access log：

- 客户端使用 HMAC gateway token 和 mTLS 调 `GatewayService` facade。
- api-gateway 通过 mTLS 调 message-service，并注入 trusted metadata。
- message-service `SendMessage` gRPC access log 输出同一组 `trace_id=e2e-demo-send` / `request_id=e2e-demo-send`。
- E2E 主链路仍成功：`CreateMemberChange -> SendMessage -> delivery.notify -> PullInbox -> AckDelivery -> MarkRead -> ListConversations`。

这仍是 first-stage correlation，不是完整 OpenTelemetry trace、span model、collector、采样或告警。

## 原始结果

- Result dir: `H:\NexusIM\loadtest-results\e2e-demo-api-gateway-message-correlation-smoke-20260614-clean`
- Summary: `H:\NexusIM\loadtest-results\e2e-demo-api-gateway-message-correlation-smoke-20260614-clean\e2e-demo-summary.json`
- Code commit in summary: `ce69c88`
- `git_dirty`: `false`
- Gateway facade: `true`
- Gateway auth mode: `hmac`
- Gateway auth audience: `api-gateway`
- gRPC TLS enabled: conversation / message / delivery / receipt all `true`
- Push WSS enabled: `true`

## 关键结果

Summary:

```text
success=true
gateway_facade=true
gateway_auth_mode=hmac
gateway_auth_audience=api-gateway
conversation_tls_enabled=true
message_tls_enabled=true
delivery_tls_enabled=true
receipt_tls_enabled=true
push_tls_enabled=true
send_message.conversation_seq=2
delivery_notify.source_event_type=message.persisted.v1
pull_inbox.item_count=1
websocket_ack.last_received_seq=2
mark_read.last_read_seq=2
list_conversations_before_read.items[0].unread_count=1
list_conversations_after_read.items[0].unread_count=0
policy_audit_kafka.event_count=1
```

api-gateway facade access log:

```text
/nexusim.gateway.v1.GatewayService/SendMessage trace_id=e2e-demo-send request_id=e2e-demo-send code=OK
```

message-service downstream access log:

```text
/nexusim.message.v1.MessageService/SendMessage trace_id=e2e-demo-send request_id=e2e-demo-send code=OK
```

## 边界

- 本轮只把下游结构化日志扩到 message-service 的 gRPC access log；delivery / receipt / conversation 仍未在本切片中处理。
- message-service access log 只记录 service/event/method/code/latency_ms/trace_id/request_id，不记录 gateway token、authorization header、tenant/user/device/session、request body 或消息 payload。
- 本轮没有引入 OpenTelemetry SDK / collector。
- Kafka envelope、debug metrics、跨服务 span 和告警仍是后续项。
