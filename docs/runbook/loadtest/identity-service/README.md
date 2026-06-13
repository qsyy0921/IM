# identity-service Loadtest Reports

本目录保存 `identity-service` 的小规模验证报告和阶段入口。当前重点是真实鉴权、challenge、MFA、refresh token 和 gateway token 安全链路；不是容量压测目录。

## 当前阶段结论

`identity-service` 已完成真实鉴权第一阶段主线，并补了多个安全 hardening 切片：

- `RegisterUser / Login / RefreshGatewayToken`。
- gateway token 本地验签链路：HS256 兼容、RS256 static key ring、one-shot keyring rotate operator、JWKS public-only、old public-key overlap。
- refresh token rotation、reuse detection、device/session revoke event。
- gRPC 结构化请求日志包含 method / code / latency，并透传 bounded `trace_id` / `request_id`，但不记录 token、用户目标地址或 provider error body。
- email/phone verification 与 password reset challenge，数据库只保存 token hash，并有 target-level active cap + request window throttle；配置 `NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_SECRET` 后，password reset 的无效 / 不存在目标也会进入 HMAC target limiter，并可用 one-shot cleanup operator 清理过期 limiter row，`/debug/metrics` 只暴露 limiter 聚合计数。
- challenge webhook sender、delivery failure 过期补偿、debug metrics、持久状态审计；`/debug/metrics` 同时暴露 challenge delivery 的低敏 `failure_classes` 和 delivery outbox 的 PENDING / ready / scheduled / expired / DELIVERED / DLQ / CANCELED 聚合状态，PostgreSQL challenge / delivery outbox / repair audit 也持久化低敏 failure class 便于排障。
- durable challenge delivery outbox：challenge row 与 encrypted delivery row 同事务提交，worker 解密后调用 webhook，支持 retry / DLQ / canceled。
- challenge delivery repair / audit：一次性 operator mode 支持 `audit`、`redrive-active-pending`、`cancel-inactive`；DLQ row 不会复活旧 challenge token，必须走正常 API 重新申请 challenge。
- TOTP MFA 生命周期、Login MFA enforcement、MFA lockout、recovery codes、Refresh step-up 和 Refresh 期间直接提交 MFA proof。
- MFA TOTP secret 和 challenge delivery token 已支持本地版本化 keyring：新写入使用 current key version，旧 `key_version` 在轮换窗口内仍可解密；这不是 KMS/HSM，也不自动分发密钥。
- identity-service gRPC server 支持可选 TLS / mTLS 配置：`CERT_FILE + KEY_FILE` 必须成对配置，`CLIENT_CA_FILE` 或 `REQUIRE_CLIENT_CERT=true` 启用客户端证书校验；`CLIENT_ALLOWED_DNS_NAMES` / `CLIENT_ALLOWED_URIS` 可用 exact-match DNS SAN / URI SAN allowlist 限制可接入客户端身份。配置错误会 fail-fast，不静默降级 plaintext。`loadtest/identity` smoke runner 已支持可选 CA、server name 和 client cert/key。当前只覆盖静态配置 smoke，不代表所有客户端、所有服务、证书分发或动态服务身份策略都已完成。

可选 TLS / mTLS smoke 参数示例：

```powershell
.\loadtest\identity\run-local-smoke.ps1 `
  -IdentityGrpcTlsCertFile .\certs\identity-server.crt `
  -IdentityGrpcTlsKeyFile .\certs\identity-server.key `
  -IdentityGrpcTlsClientCaFile .\certs\ca.pem `
  -IdentityGrpcTlsRequireClientCert true `
  -IdentityGrpcTlsClientAllowedDnsNames push-gateway.nexusim.local `
  -IdentityTlsCaFile .\certs\ca.pem `
  -IdentityTlsServerName identity-service.nexusim.local `
  -IdentityTlsClientCertFile .\certs\loadtest-client.crt `
  -IdentityTlsClientKeyFile .\certs\loadtest-client.key
```

服务端 TLS 可通过 `NEXUSIM_IDENTITY_GRPC_TLS_*` 环境变量配置，也可在本地 smoke 中用上面的 `IdentityGrpcTls*` 参数注入到脚本启动的 identity-service 进程；`IdentityTls*` 参数只控制 loadtest client 如何连接 identity-service。

当前 `challenge delivery outbox` 真实进程 smoke 已证明：

```text
RegisterUser
-> RequestVerificationChallenge(outbox mode)
-> identity_challenge_delivery_outbox(PENDING)
-> challenge-delivery-worker
-> local webhook receives raw token
-> identity_challenge_delivery_outbox(DELIVERED)
-> ConfirmVerificationChallenge
-> identity_challenges(CONSUMED, delivery_status=DELIVERED)
```

## 报告列表

| 报告 | 内容 |
| --- | --- |
| `loadtest-report-20260613-identity-challenge-delivery-outbox-smoke.md` | `RequestVerificationChallenge(outbox)` -> `challenge-delivery-worker` -> webhook token -> `ConfirmVerificationChallenge` 真实进程 smoke |
| `loadtest-report-20260613-identity-mtls-smoke.md` | identity-service gRPC TLS / mTLS + client DNS SAN allowlist 下的 challenge delivery outbox 真实进程 smoke |
| `loadtest-report-20260614-identity-api-gateway-facade-smoke.md` | `RegisterUser -> RequestVerificationChallenge(outbox) -> ConfirmVerificationChallenge -> Login -> RefreshGatewayToken -> RequestPasswordReset -> ConfirmPasswordReset -> post-reset Login` 经 api-gateway `GatewayService` facade 的真实进程 smoke |

## 面试可讲重点

- challenge token 不落库，PostgreSQL 只保存 `token_hash`；delivery outbox 里保存 AES-GCM 加密后的 token。
- challenge 创建有本地防滥用：同一目标最多 3 个 ACTIVE challenge，默认 15 分钟内最多 5 次创建；password reset 的限流仍返回 neutral accepted，避免暴露账号存在性。无效目标 limiter 只保存 HMAC target key，不保存 raw email/phone；`challenge-request-limit-cleanup` 只删除窗口和锁都过期的 limiter row，debug metrics 只输出总数和锁定数。
- RPC 成功表示“durable enqueue”，不是第三方 provider 已送达；真实投递由 worker 异步完成。
- worker 使用 ready query + row lock 拉取 delivery outbox，成功后标记 `DELIVERED`；失败按 retry / DLQ 处理，并在 challenge / delivery outbox / repair audit 中持久化低敏 failure class；debug metrics 只暴露失败分类、队列状态聚合和最大 pending retry，不暴露 token 密文、目标地址、provider URL 或 provider error body。
- 结构化 gRPC 日志足以把本地 smoke / provider webhook / outbox repair 串到同一个 `trace_id` 或 `request_id`，但它仍不是完整 OpenTelemetry tracing 或生产告警平台。
- repair 工具是 audit-first，不解密 token、不直接标记 `DELIVERED`、不把 `EXPIRED` challenge 复活；这比把 challenge delivery DLQ 当普通 Kafka outbox replay 更安全。
- 本轮 smoke 没开 `NEXUSIM_IDENTITY_DEV_RETURN_CHALLENGE_TOKEN`，Confirm 使用的是 webhook 收到的真实 token，所以能证明 worker 链路生效。
- `NEXUSIM_IDENTITY_SERVICE_MODE=gateway-token-keyring-rotate` 可以轮换本地 RS256 keyring 文件：生成新当前私钥，把旧当前 key 降级为 public-only overlap，并按 `NEXUSIM_IDENTITY_GATEWAY_TOKEN_ROTATE_OLD_KEY_LIMIT` 保留旧公钥。它不分发密钥、不接 KMS/HSM，也不是完整自动轮换平台。
- gRPC TLS / mTLS 配置可以作为“服务端和 smoke 客户端的第一阶段传输安全已接通”的面试点；SAN allowlist 已有第一版 exact-match 配置，但仍要说明没有完成全服务 mTLS、证书轮换、动态服务身份注册或服务网格治理。
- 该能力仍不是完整 email/SMS provider、服务网格或密钥管理平台；provider 模板、bounce handling、KMS/HSM-backed keyring、统一告警仍是后续项。
