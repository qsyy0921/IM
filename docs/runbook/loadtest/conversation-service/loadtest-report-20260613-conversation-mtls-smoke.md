# conversation-service mTLS Smoke Report

本报告记录 `conversation-service` gRPC server 的第一阶段静态 TLS / mTLS 真实进程 smoke。

这不是全服务 mTLS rollout，也不是证书签发、轮换、撤销、分发或动态服务身份治理验收；它只验证本地真实进程链路：

```text
conversation-service gRPC TLS server
-> client certificate required
-> exact-match client DNS SAN allowlist
-> TransferConversationOwner
-> shared timeline/outbox
-> message-service outbox relay
-> Kafka conversation.timeline.events
-> conversation-service member-change-worker
-> GetMemberChange(DONE)
-> ListConversationMembers
```

## 命令

本轮先生成本地 smoke 专用 CA / server cert / client cert 到 H 盘临时目录：

```text
H:\NexusIM\loadtest-results\conversation-mtls-certs-20260613-194641
```

随后运行：

```powershell
.\loadtest\memberchange\run-local-smoke.ps1 `
  -RunName "conversation-mtls-smoke-20260613-194641" `
  -Scenario owner-transfer `
  -ConversationGrpcTlsCertFile "H:\NexusIM\loadtest-results\conversation-mtls-certs-20260613-194641\conversation-server.crt" `
  -ConversationGrpcTlsKeyFile "H:\NexusIM\loadtest-results\conversation-mtls-certs-20260613-194641\conversation-server.key" `
  -ConversationGrpcTlsClientCaFile "H:\NexusIM\loadtest-results\conversation-mtls-certs-20260613-194641\ca.crt" `
  -ConversationGrpcTlsRequireClientCert "true" `
  -ConversationGrpcTlsClientAllowedDnsNames "api-gateway.nexusim.local" `
  -ConversationTlsCaFile "H:\NexusIM\loadtest-results\conversation-mtls-certs-20260613-194641\ca.crt" `
  -ConversationTlsServerName "conversation-service.nexusim.local" `
  -ConversationTlsClientCertFile "H:\NexusIM\loadtest-results\conversation-mtls-certs-20260613-194641\loadtest-client.crt" `
  -ConversationTlsClientKeyFile "H:\NexusIM\loadtest-results\conversation-mtls-certs-20260613-194641\loadtest-client.key"
```

## 基线

| Item | Value |
| --- | --- |
| Commit | `ab42119` |
| Full commit | `ab421195234aa821ff9f44bae8963ff75358e236` |
| Git dirty | `false` |
| Result dir | `H:\NexusIM\loadtest-results\conversation-mtls-smoke-20260613-194641` |
| Summary | `H:\NexusIM\loadtest-results\conversation-mtls-smoke-20260613-194641\memberchange-summary.json` |
| Timeline topic | `conversation.timeline.ownertransfer.20260613-194655` |
| Server name | `conversation-service.nexusim.local` |
| Client allowed DNS SAN | `api-gateway.nexusim.local` |

## 关键结果

`memberchange-summary.json` 中的关键字段：

```json
{
  "success_count": 1,
  "error_count": 0,
  "success_rate": 1,
  "tls_enabled": true,
  "verified_auth_metadata": false,
  "change_type": "OWNER_TRANSFER",
  "saga_count": 1,
  "saga_done_count": 1,
  "timeline_count": 1,
  "outbox_total_count": 1,
  "outbox_pending_count": 0,
  "outbox_published_count": 1,
  "outbox_dlq_count": 0,
  "conversation_seq_current": 1,
  "sample_get_status": "MEMBER_CHANGE_STATUS_DONE",
  "member_list_count": 2,
  "member_list_target_role": "MEMBER_ROLE_OWNER",
  "member_list_target_status": "MEMBER_STATUS_ACTIVE",
  "owner_transfer_previous_owner_role": "MEMBER_ROLE_ADMIN",
  "owner_transfer_new_owner_role": "MEMBER_ROLE_OWNER",
  "owner_transfer_owner_count": 1,
  "p99_ms": 79.985
}
```

## 结论

本轮证明了 `conversation-service` 静态 gRPC TLS / mTLS 配置在真实进程中可用：

- gRPC server 使用 `NEXUSIM_CONVERSATION_GRPC_TLS_CERT_FILE` / `KEY_FILE` 启动 TLS。
- `NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_CA_FILE` + `REQUIRE_CLIENT_CERT=true` 强制客户端证书。
- `NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES=api-gateway.nexusim.local` 通过 exact-match DNS SAN allowlist 校验客户端身份。
- loadtest client 使用 CA、server name 和 client cert/key 成功完成 owner transfer。
- `message_outbox` 通过 message-service relay 发布 member boundary event，member-change worker 推进 saga 到 `DONE`。
- 最终 `PENDING=0 / PUBLISHED=1 / DLQ=0`，且 roster 中旧 owner 降级为 `ADMIN`，新 owner 成为唯一 `OWNER`。

## 边界

- 这是 conversation-service 单服务静态 mTLS smoke，不代表全服务 mTLS rollout。
- 证书是本地临时生成材料，不代表生产证书签发、轮换、撤销或分发体系。
- client allowlist 是第一阶段 exact-match DNS / URI SAN 校验，不是动态服务身份注册或服务网格。
- 本轮只覆盖 `owner-transfer` 场景；JOIN / LEAVE / REMOVE / ROLE_CHANGED 和 `GetSendContext` 的业务语义已由既有 smoke 覆盖。
