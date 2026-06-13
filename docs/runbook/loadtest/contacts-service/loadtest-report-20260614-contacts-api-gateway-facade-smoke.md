# contacts-service api-gateway facade smoke - 2026-06-14

## 结论

本轮 smoke 证明 `contacts-service` 的 accept-flow 已能通过 `api-gateway` 的 `nexusim.gateway.v1.GatewayService` public facade 完成，而不是客户端直连 `contacts-service`。

链路：

```text
loadtest/contacts
-> api-gateway GatewayService + HMAC gateway token
-> contacts-service metadata auth
-> contact_requests / contact_edges / contacts_outbox
-> contacts-service outbox-relay
-> Kafka im.contact.events.*
```

这仍是本地小规模 smoke，不是容量测试，也不是完整生产 API gateway 部署。传输层本轮使用 plaintext；身份链路验证重点是 gateway token、`AuthContext` 重写和下游 trusted metadata 注入。

## 原始结果

- Result dir: `H:\NexusIM\loadtest-results\contacts-api-gateway-facade-smoke-20260614-clean`
- Summary: `H:\NexusIM\loadtest-results\contacts-api-gateway-facade-smoke-20260614-clean\contacts-summary.json`
- Code commit in summary: `f3b290f`
- `git_dirty`: `false`
- Scenario: `accept`
- Target: `127.0.0.1:64486` (api-gateway)
- Contact topic: `im.contact.events.contacts-accept-smoke.20260614-003531`

## 关键证据

Summary:

```text
success=true
gateway_facade=true
gateway_auth_mode=hmac
gateway_auth_audience=api-gateway
verified_auth_metadata=false
send_contact_request.status=CONTACT_REQUEST_STATUS_PENDING
respond_contact_request.status=CONTACT_REQUEST_STATUS_ACCEPTED
receiver_incoming_pending_before_respond.request_count=1
receiver_incoming_pending_after_respond.request_count=0
receiver_incoming_terminal_after_respond.request_count=1
sender_list.contact_count=1
receiver_list.contact_count=1
sender_state.status=CONTACT_EDGE_STATUS_ACTIVE
receiver_state.status=CONTACT_EDGE_STATUS_ACTIVE
contacts_outbox.total=2
contacts_outbox.published=2
contacts_outbox.pending=0
contacts_outbox.dlq=0
```

Kafka read-back:

```text
contact.request.created.v1 aggregate_version=1 status=PENDING
contact.request.accepted.v1 aggregate_version=2 status=ACCEPTED
partition_key=tenant-contacts-accept-smoke-20260614-003531:contacts-receiver:contacts-sender
```

api-gateway access log shows all client-facing contacts calls used the facade descriptor:

```text
/nexusim.gateway.v1.GatewayService/SendContactRequest OK
/nexusim.gateway.v1.GatewayService/ListContactRequests OK
/nexusim.gateway.v1.GatewayService/RespondContactRequest OK
/nexusim.gateway.v1.GatewayService/ListContacts OK
/nexusim.gateway.v1.GatewayService/GetContactState OK
```

contacts-service logs show the downstream service received normal contacts RPCs after gateway forwarding:

```text
/nexusim.contacts.v1.ContactsService/SendContactRequest OK
/nexusim.contacts.v1.ContactsService/ListContactRequests OK
/nexusim.contacts.v1.ContactsService/RespondContactRequest OK
/nexusim.contacts.v1.ContactsService/ListContacts OK
/nexusim.contacts.v1.ContactsService/GetContactState OK
```

## 边界

- `contacts-service` 仍是联系人事实源；api-gateway 不写联系人事实表、不发布 contacts Kafka event。
- api-gateway 只验证 gateway token、覆盖 request body `AuthContext`、向 contacts-service 注入 trusted metadata。
- `message-service` 仍不同步依赖 contacts-service。
- 本轮没有覆盖 contacts facade 的 TLS / mTLS；下游 TLS 配置能力仍保留在 api-gateway 和 contacts-service。
- 本轮只覆盖 accept-flow；decline / cancel / delete / block / unblock / remark / readd 已在 contacts-service direct smoke 中覆盖，后续可按需要补 facade 参数化回归。
