# identity-service mTLS Smoke Report

本报告记录 `identity-service` gRPC server 的第一阶段静态 TLS / mTLS 真实进程 smoke。

这不是全服务 mTLS rollout，也不是证书签发 / 轮换 / 分发或动态服务身份治理验收；它只验证本地真实进程链路：

```text
identity-service gRPC TLS server
-> client certificate required
-> exact-match client DNS SAN allowlist
-> RegisterUser
-> RequestVerificationChallenge(outbox)
-> challenge-delivery-worker
-> local webhook receives token
-> ConfirmVerificationChallenge
```

## 命令

本轮先生成本地 smoke 专用 CA / server cert / client cert 到 H 盘临时目录：

```text
H:\NexusIM\loadtest-results\identity-mtls-certs-20260613-192130
```

随后运行：

```powershell
.\loadtest\identity\run-local-smoke.ps1 `
  -RunName "identity-mtls-smoke-20260613-192500" `
  -IdentityGrpcTlsCertFile "H:\NexusIM\loadtest-results\identity-mtls-certs-20260613-192130\identity-server.crt" `
  -IdentityGrpcTlsKeyFile "H:\NexusIM\loadtest-results\identity-mtls-certs-20260613-192130\identity-server.key" `
  -IdentityGrpcTlsClientCaFile "H:\NexusIM\loadtest-results\identity-mtls-certs-20260613-192130\ca.crt" `
  -IdentityGrpcTlsRequireClientCert "true" `
  -IdentityGrpcTlsClientAllowedDnsNames "push-gateway.nexusim.local" `
  -IdentityTlsCaFile "H:\NexusIM\loadtest-results\identity-mtls-certs-20260613-192130\ca.crt" `
  -IdentityTlsServerName "identity-service.nexusim.local" `
  -IdentityTlsClientCertFile "H:\NexusIM\loadtest-results\identity-mtls-certs-20260613-192130\loadtest-client.crt" `
  -IdentityTlsClientKeyFile "H:\NexusIM\loadtest-results\identity-mtls-certs-20260613-192130\loadtest-client.key" `
  -SkipBuild
```

## 基线

| Item | Value |
| --- | --- |
| Commit | `2f0a4ab` |
| Full commit | `2f0a4ab903f1828a8ed8d216209fe4cec5944629` |
| Git dirty | `false` |
| Result dir | `H:\NexusIM\loadtest-results\identity-mtls-smoke-20260613-192500` |
| Summary | `H:\NexusIM\loadtest-results\identity-mtls-smoke-20260613-192500\identity-summary.json` |
| gRPC target | `127.0.0.1:64510` |
| TLS enabled | `true` |
| Server name | `identity-service.nexusim.local` |
| Client allowed DNS SAN | `push-gateway.nexusim.local` |

## 关键结果

`identity-summary.json` 中的关键字段：

```json
{
  "success": true,
  "tls_enabled": true,
  "git_dirty": false,
  "register_user": {
    "status": "USER_STATUS_ACTIVE"
  },
  "request_verification_challenge": {
    "channel": "VERIFICATION_CHANNEL_EMAIL",
    "dev_challenge_token_set": false
  },
  "webhook": {
    "received": true,
    "token_set": true,
    "authorization_ok": true
  },
  "challenge_delivery_outbox": {
    "total": 1,
    "pending": 0,
    "delivered": 1,
    "dlq": 0,
    "canceled": 0
  },
  "challenge_delivery_outbox_row": {
    "status": "DELIVERED",
    "retry_count": 0,
    "delivered": true,
    "dlq": false
  },
  "challenge_row": {
    "status": "CONSUMED",
    "delivery_status": "DELIVERED",
    "delivery_attempt_count": 1
  }
}
```

## 结论

本轮证明了 `identity-service` 静态 gRPC TLS / mTLS 配置在真实进程中可用：

- gRPC server 使用 `NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE` / `KEY_FILE` 启动 TLS。
- `NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_CA_FILE` + `REQUIRE_CLIENT_CERT=true` 强制客户端证书。
- `NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES=push-gateway.nexusim.local` 通过 exact-match DNS SAN allowlist 校验客户端身份。
- loadtest client 使用 CA、server name 和 client cert/key 成功完成完整 challenge delivery outbox 链路。
- `dev_challenge_token_set=false`，Confirm 使用的是 webhook 收到的真实 token，不是 dev token 旁路。

## 边界

- 这是单服务静态 mTLS smoke，不代表全服务 mTLS rollout。
- 证书是本地临时生成材料，不代表生产证书签发、轮换、撤销或分发体系。
- client allowlist 是第一阶段 exact-match DNS / URI SAN 校验，不是动态服务身份注册或服务网格。
- `challenge-delivery-worker` 到 webhook 仍使用本地 HTTP；本轮验证的是 identity gRPC client/server 之间的 TLS / mTLS。
