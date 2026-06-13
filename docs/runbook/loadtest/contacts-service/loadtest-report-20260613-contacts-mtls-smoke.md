# contacts-service mTLS Smoke Report

本报告记录 `contacts-service` gRPC server 的第一阶段静态 TLS / mTLS 真实进程 smoke。

这不是全服务 mTLS rollout，也不是证书签发 / 轮换 / 分发或动态服务身份治理验收；它只验证本地真实进程链路：

```text
contacts-service gRPC TLS server
-> client certificate required
-> exact-match client DNS SAN allowlist
-> SendContactRequest
-> ListContactRequests
-> RespondContactRequest(ACCEPT)
-> contacts_outbox
-> contacts-service outbox relay
-> Kafka im.contact.events
-> ListContacts / GetContactState
```

## 命令

本轮先生成本地 smoke 专用 CA / server cert / client cert 到 H 盘临时目录：

```text
H:\NexusIM\loadtest-results\contacts-mtls-certs-20260613-193706
```

随后运行：

```powershell
.\loadtest\contacts\run-local-smoke.ps1 `
  -RunName "contacts-mtls-smoke-20260613-193706" `
  -Scenario accept `
  -ContactsGrpcTlsCertFile "H:\NexusIM\loadtest-results\contacts-mtls-certs-20260613-193706\contacts-server.crt" `
  -ContactsGrpcTlsKeyFile "H:\NexusIM\loadtest-results\contacts-mtls-certs-20260613-193706\contacts-server.key" `
  -ContactsGrpcTlsClientCaFile "H:\NexusIM\loadtest-results\contacts-mtls-certs-20260613-193706\ca.crt" `
  -ContactsGrpcTlsRequireClientCert "true" `
  -ContactsGrpcTlsClientAllowedDnsNames "api-gateway.nexusim.local" `
  -ContactsTlsCaFile "H:\NexusIM\loadtest-results\contacts-mtls-certs-20260613-193706\ca.crt" `
  -ContactsTlsServerName "contacts-service.nexusim.local" `
  -ContactsTlsClientCertFile "H:\NexusIM\loadtest-results\contacts-mtls-certs-20260613-193706\loadtest-client.crt" `
  -ContactsTlsClientKeyFile "H:\NexusIM\loadtest-results\contacts-mtls-certs-20260613-193706\loadtest-client.key"
```

## 基线

| Item | Value |
| --- | --- |
| Commit | `0490ded` |
| Full commit | `0490deda5a7e43461404b7f14d574aeb101886bc` |
| Git dirty | `false` |
| Result dir | `H:\NexusIM\loadtest-results\contacts-mtls-smoke-20260613-193706` |
| Summary | `H:\NexusIM\loadtest-results\contacts-mtls-smoke-20260613-193706\contacts-summary.json` |
| Contact topic | `im.contact.events.contacts-accept-smoke.20260613-193744` |
| Server name | `contacts-service.nexusim.local` |
| Client allowed DNS SAN | `api-gateway.nexusim.local` |

## 关键结果

`contacts-summary.json` 中的关键字段：

```json
{
  "success": true,
  "git_dirty": false,
  "tls_enabled": true,
  "verified_auth_metadata": false,
  "send_contact_request": {
    "status": "CONTACT_REQUEST_STATUS_PENDING",
    "idempotent_replay": false
  },
  "respond_contact_request": {
    "status": "CONTACT_REQUEST_STATUS_ACCEPTED",
    "idempotent_replay": false
  },
  "sender_list": {
    "contact_count": 1,
    "contact_user_ids": ["contacts-receiver"]
  },
  "receiver_list": {
    "contact_count": 1,
    "contact_user_ids": ["contacts-sender"]
  },
  "sender_state": {
    "status": "CONTACT_EDGE_STATUS_ACTIVE",
    "version": 1
  },
  "receiver_state": {
    "status": "CONTACT_EDGE_STATUS_ACTIVE",
    "version": 1
  },
  "contacts_outbox": {
    "total": 2,
    "pending": 0,
    "published": 2,
    "dlq": 0
  },
  "contact_kafka_events": [
    {"event_type": "contact.request.created.v1", "status": "PENDING"},
    {"event_type": "contact.request.accepted.v1", "status": "ACCEPTED"}
  ]
}
```

## 结论

本轮证明了 `contacts-service` 静态 gRPC TLS / mTLS 配置在真实进程中可用：

- gRPC server 使用 `NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE` / `KEY_FILE` 启动 TLS。
- `NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE` + `REQUIRE_CLIENT_CERT=true` 强制客户端证书。
- `NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES=api-gateway.nexusim.local` 通过 exact-match DNS SAN allowlist 校验客户端身份。
- loadtest client 使用 CA、server name 和 client cert/key 成功完成 contacts accept-flow。
- outbox relay 发布 `contact.request.created.v1` 和 `contact.request.accepted.v1`，最终 `PENDING=0 / PUBLISHED=2 / DLQ=0`。

## 边界

- 这是 contacts-service 单服务静态 mTLS smoke，不代表全服务 mTLS rollout。
- 证书是本地临时生成材料，不代表生产证书签发、轮换、撤销或分发体系。
- client allowlist 是第一阶段 exact-match DNS / URI SAN 校验，不是动态服务身份注册或服务网格。
- 本轮只覆盖 `accept` 场景；其它 contacts 场景的业务语义已由既有 smoke 覆盖。
