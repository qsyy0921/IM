# api-gateway conversation correlation smoke

日期：2026-06-14

目标：验证 api-gateway 注入的 `x-nexusim-trace-id` / `x-nexusim-request-id` 能被 conversation-service 的 user-facing gRPC access log 读取并输出。该 smoke 复用 secure E2E facade 链路，不是完整 OpenTelemetry trace、collector 或告警验证。

## 运行信息

- 代码 commit：`c6c3bc5`
- 工作区：`git_dirty=false`
- 结果目录：`H:\NexusIM\loadtest-results\e2e-demo-api-gateway-conversation-correlation-smoke-20260614-clean`
- runner：`.\loadtest\demo\run-local-secure-demo.ps1 -RunName e2e-demo-api-gateway-conversation-correlation-smoke-20260614-clean`
- 入口：`GatewayService` facade，`gateway_auth_mode=hmac`
- TLS：conversation / message / delivery / receipt / push 均启用本地短期证书

## 结果

summary 显示：

```text
success=true
gateway_facade=true
gateway_auth_mode=hmac
member_join.boundary_seq=1
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
/nexusim.gateway.v1.GatewayService/CreateMemberChange code=OK trace_id=e2e-demo-join request_id=e2e-demo-join
```

conversation-service access log：

```text
/nexusim.conversation.v1.ConversationService/CreateMemberChange code=OK trace_id=e2e-demo-join request_id=e2e-demo-join
```

## 边界

- 已覆盖：api-gateway -> conversation-service user-facing `CreateMemberChange` 的 gRPC metadata correlation 与低敏 access log 输出。
- 未覆盖：message-service -> conversation-service `GetSendContext` 服务间 RPC 的 correlation 透传；本轮 smoke 中该调用仍没有 trace/request 字段。
- 未覆盖：Kafka envelope correlation、统一 trace/span、OpenTelemetry collector、生产日志采集、告警和跨进程 trace 采样策略。
- access log 只记录 `service/event/method/code/latency_ms/trace_id/request_id`，不记录 token、请求体、tenant/user/device/session 字段或 authorization metadata。
