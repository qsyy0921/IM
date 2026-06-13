# api-gateway correlation smoke - 2026-06-14

## 结论

本轮 smoke 证明 api-gateway 第一阶段 correlation propagation 已可用：

- gateway token 或 incoming metadata 已带 `trace_id / request_id` 时，api-gateway 保留最终值。
- 缺失时 api-gateway 会生成 `trace_* / request_*`。
- 最终 correlation 会写入下游 trusted metadata。
- 最终 correlation 会通过 gRPC response header 回给客户端。
- api-gateway 低敏 JSON access log 使用最终 correlation，而不是只读原始 incoming metadata。

这不是完整 OpenTelemetry，也不代表全服务 trace exporter、span model、采样、collector、告警已经完成。

## 原始结果

- Result dir: `H:\NexusIM\loadtest-results\contacts-api-gateway-correlation-smoke-20260614-clean`
- Summary: `H:\NexusIM\loadtest-results\contacts-api-gateway-correlation-smoke-20260614-clean\contacts-summary.json`
- Code commit in summary: `ccae224`
- `git_dirty`: `false`
- Scenario: contacts `accept`
- Gateway target: `127.0.0.1:57327`
- Contacts target: `127.0.0.1:57326`
- Contact topic: `im.contact.events.contacts-accept-smoke.20260614-004430`

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

api-gateway access log now includes final correlation for facade methods:

```text
/nexusim.gateway.v1.GatewayService/SendContactRequest trace_id=trace-contact-20260613164439 request_id=contact-send-20260613164439 code=OK
/nexusim.gateway.v1.GatewayService/ListContactRequests trace_id=trace-contact-request-list request_id=contact-request-list-contacts-receiver code=OK
/nexusim.gateway.v1.GatewayService/RespondContactRequest trace_id=trace-contact-20260613164439 request_id=contact-respond-20260613164439 code=OK
/nexusim.gateway.v1.GatewayService/ListContacts trace_id=trace-contact-list request_id=contact-list-contacts-sender code=OK
/nexusim.gateway.v1.GatewayService/GetContactState trace_id=trace-contact-state request_id=contact-state-contacts-sender code=OK
```

Downstream contacts-service still sees normal contacts RPCs:

```text
/nexusim.contacts.v1.ContactsService/SendContactRequest OK
/nexusim.contacts.v1.ContactsService/ListContactRequests OK
/nexusim.contacts.v1.ContactsService/RespondContactRequest OK
/nexusim.contacts.v1.ContactsService/ListContacts OK
/nexusim.contacts.v1.ContactsService/GetContactState OK
```

## 边界

- 本轮复用 contacts facade smoke 作为真实进程证据；未重新覆盖 demo 主链路。
- 本轮没有引入 OpenTelemetry SDK / collector。
- access log 仍不记录 token、authorization header、user body 或 request body。
- 这是 api-gateway 入口侧的 first-stage correlation；后续仍需逐步把其它服务日志、Kafka envelope、debug metrics 和分布式 trace exporter 对齐。
