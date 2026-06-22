# message-service mTLS Smoke Report

本报告记录 `message-service` gRPC server 的第一阶段静态 TLS / mTLS 真实进程 smoke。

这不是全服务 mTLS rollout，也不是证书签发、轮换、撤销、分发或动态服务身份治理验收；它只验证本地真实进程链路：

```text
message-service gRPC TLS server
-> client certificate required
-> exact-match client DNS SAN allowlist
-> SendMessage
-> PostgreSQL local transaction
-> message_log / conversation_timeline_events / message_outbox
-> message-service outbox relay
-> Kafka conversation.timeline.events
```

## 命令

本轮先生成本地 smoke 专用 CA / server cert / client cert 到 H 盘临时目录：

```text
H:\NexusIM\loadtest-results\message-mtls-certs-20260613-195657
```

随后启动：

- `message-service` gRPC mode，配置 `NEXUSIM_MESSAGE_GRPC_TLS_*`。
- `message-service` outbox-relay mode，发布到本轮独立 Kafka topic。
- `loadtest/sendmessage`，使用 `--message-tls-*` client 参数连接 message-service。

关键 client 参数：

```powershell
.\bin\sendmessage-loadtest.exe `
  --target 127.0.0.1:59980 `
  --vus 1 `
  --duration 1s `
  --request-timeout 5s `
  --tenant-id tenant-message-mtls-smoke-20260613-195916 `
  --conversation-prefix conv-message-mtls-smoke `
  --conversation-count 1 `
  --pg-dsn "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  --stats-wait 10s `
  --message-tls-ca-file "H:\NexusIM\loadtest-results\message-mtls-certs-20260613-195657\ca.crt" `
  --message-tls-server-name "message-service.nexusim.local" `
  --message-tls-client-cert-file "H:\NexusIM\loadtest-results\message-mtls-certs-20260613-195657\loadtest-client.crt" `
  --message-tls-client-key-file "H:\NexusIM\loadtest-results\message-mtls-certs-20260613-195657\loadtest-client.key"
```

Server 侧关键环境：

```text
NEXUSIM_MESSAGE_GRPC_TLS_CERT_FILE=...\message-server.crt
NEXUSIM_MESSAGE_GRPC_TLS_KEY_FILE=...\message-server.key
NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_CA_FILE=...\ca.crt
NEXUSIM_MESSAGE_GRPC_TLS_REQUIRE_CLIENT_CERT=true
NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES=api-gateway.nexusim.local
```

## 基线

| Item | Value |
| --- | --- |
| Commit | `e43a403` |
| Full commit | `e43a4036cecc9910f7ffd782eb6c3325f0ddd3e3` |
| Git dirty | `false` |
| Result dir | `H:\NexusIM\loadtest-results\message-mtls-smoke-20260613-195916` |
| Summary | `H:\NexusIM\loadtest-results\message-mtls-smoke-20260613-195916\sendmessage-summary.json` |
| Kafka topic | `conversation.timeline.message-mtls-smoke.20260613-195916` |
| Server name | `message-service.nexusim.local` |
| Client allowed DNS SAN | `api-gateway.nexusim.local` |

## 关键结果

`sendmessage-summary.json` 中的关键字段：

```json
{
  "success_count": 143,
  "error_count": 0,
  "success_rate": 1,
  "message_tls_enabled": true,
  "verified_auth_metadata": false,
  "p95_ms": 9.3582,
  "p99_ms": 10.3563,
  "outbox_total_count": 143,
  "outbox_published_count": 143,
  "outbox_pending_count": 0,
  "outbox_dlq_count": 0,
  "kafka_publish_records_per_call_recent_sample_count": 2
}
```

## 结论

本轮证明了 `message-service` 静态 gRPC TLS / mTLS 配置在真实进程中可用：

- gRPC server 使用 `NEXUSIM_MESSAGE_GRPC_TLS_CERT_FILE` / `KEY_FILE` 启动 TLS。
- `NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_CA_FILE` + `REQUIRE_CLIENT_CERT=true` 强制客户端证书。
- `NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES=api-gateway.nexusim.local` 通过 exact-match DNS SAN allowlist 校验客户端身份。
- `loadtest/sendmessage` 使用 CA、server name 和 client cert/key 成功完成 143 次 `SendMessage`。
- PostgreSQL 本地事务写入 `message_log / conversation_timeline_events / message_outbox`，outbox relay 发布到 Kafka。
- 最终 `PENDING=0 / PUBLISHED=143 / DLQ=0`。

## 边界

- 这是 message-service 单服务静态 mTLS smoke，不代表全服务 mTLS rollout。
- 证书是本地临时生成材料，不代表生产证书签发、轮换、撤销或分发体系。
- client allowlist 是第一阶段 exact-match DNS / URI SAN 校验，不是动态服务身份注册或服务网格。
- 本轮使用 message-service 的 static conversation / policy recovery，不覆盖真实 conversation-service 或 policy-service RPC mTLS；那些服务间 TLS 能力已有独立配置和 smoke 证据继续补齐。
- 本轮不是容量压测；1 VU / 1s 只用于验证 mTLS transport 和主写链路闭环。
