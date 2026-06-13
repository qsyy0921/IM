# api-gateway downstream correlation smoke - 2026-06-14

## 结论

本轮 smoke 证明 api-gateway 生成 / 保留的 `trace_id` 与 `request_id` 已经进入第一个下游服务的结构化 access log：

- 客户端通过 `GatewayService` contacts facade 调 api-gateway。
- api-gateway 使用 HMAC gateway token 验证身份，并向 contacts-service 注入 trusted metadata。
- contacts-service 的 gRPC JSON access log 读取 `x-nexusim-trace-id` / `x-nexusim-request-id`，并输出低敏 `trace_id` / `request_id`。
- contacts accept 主链路仍成功，contacts outbox relay 发布 2 条事件，无 PENDING / DLQ。

这仍是 first-stage correlation，不是完整 OpenTelemetry trace、span model、collector 或告警。

## 原始结果

- Result dir: `H:\NexusIM\loadtest-results\contacts-api-gateway-downstream-correlation-smoke-20260614-clean`
- Summary: `H:\NexusIM\loadtest-results\contacts-api-gateway-downstream-correlation-smoke-20260614-clean\contacts-summary.json`
- Code commit in summary: `b022a09`
- `git_dirty`: `false`
- Scenario: contacts `accept`
- Gateway target: `127.0.0.1:59296`
- Contacts target: `127.0.0.1:59295`
- Contact topic: `im.contact.events.contacts-accept-smoke.20260614-005254`

## 关键结果

Summary:

```text
success=true
gateway_facade=true
gateway_auth_mode=hmac
gateway_auth_audience=api-gateway
verified_auth_metadata=false
send_contact_request.status=CONTACT_REQUEST_STATUS_PENDING
respond_contact_request.status=CONTACT_REQUEST_STATUS_ACCEPTED
sender_list.contact_count=1
receiver_list.contact_count=1
contacts_outbox.total=2
contacts_outbox.published=2
contacts_outbox.pending=0
contacts_outbox.dlq=0
```

api-gateway facade access log:

```text
/nexusim.gateway.v1.GatewayService/SendContactRequest trace_id=trace-contact-20260613165303 request_id=contact-send-20260613165303 code=OK
/nexusim.gateway.v1.GatewayService/ListContactRequests trace_id=trace-contact-request-list request_id=contact-request-list-contacts-receiver code=OK
/nexusim.gateway.v1.GatewayService/RespondContactRequest trace_id=trace-contact-20260613165303 request_id=contact-respond-20260613165303 code=OK
/nexusim.gateway.v1.GatewayService/ListContacts trace_id=trace-contact-list request_id=contact-list-contacts-sender code=OK
/nexusim.gateway.v1.GatewayService/GetContactState trace_id=trace-contact-state request_id=contact-state-contacts-sender code=OK
```

contacts-service downstream access log now carries the same correlation values:

```text
/nexusim.contacts.v1.ContactsService/SendContactRequest trace_id=trace-contact-20260613165303 request_id=contact-send-20260613165303 code=OK
/nexusim.contacts.v1.ContactsService/ListContactRequests trace_id=trace-contact-request-list request_id=contact-request-list-contacts-receiver code=OK
/nexusim.contacts.v1.ContactsService/RespondContactRequest trace_id=trace-contact-20260613165303 request_id=contact-respond-20260613165303 code=OK
/nexusim.contacts.v1.ContactsService/ListContacts trace_id=trace-contact-list request_id=contact-list-contacts-sender code=OK
/nexusim.contacts.v1.ContactsService/GetContactState trace_id=trace-contact-state request_id=contact-state-contacts-sender code=OK
```

## 边界

- 本轮只把下游结构化日志的第一站落在 contacts-service，尚未横向改完所有服务。
- contacts-service access log 只记录 service/event/method/code/latency_ms/trace_id/request_id，不记录 gateway token、authorization header、tenant/user/device/session、request body 或业务 payload。
- 本轮没有引入 OpenTelemetry SDK / collector。
- Kafka envelope、debug metrics、跨服务 span 和告警仍是后续项。
