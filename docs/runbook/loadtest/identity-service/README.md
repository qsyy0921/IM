# identity-service Loadtest Reports

本目录保存 `identity-service` 的小规模验证报告和阶段入口。当前重点是真实鉴权、challenge、MFA、refresh token 和 gateway token 安全链路；不是容量压测目录。

## 当前阶段结论

`identity-service` 已完成真实鉴权第一阶段主线，并补了多个安全 hardening 切片：

- `RegisterUser / Login / RefreshGatewayToken`。
- gateway token 本地验签链路：HS256 兼容、RS256 static key ring、JWKS public-only、old public-key overlap。
- refresh token rotation、reuse detection、device/session revoke event。
- email/phone verification 与 password reset challenge，数据库只保存 token hash。
- challenge webhook sender、delivery failure 过期补偿、debug metrics、持久状态审计。
- durable challenge delivery outbox：challenge row 与 encrypted delivery row 同事务提交，worker 解密后调用 webhook，支持 retry / DLQ / canceled。
- challenge delivery repair / audit：一次性 operator mode 支持 `audit`、`redrive-active-pending`、`cancel-inactive`；DLQ row 不会复活旧 challenge token，必须走正常 API 重新申请 challenge。
- TOTP MFA 生命周期、Login MFA enforcement、MFA lockout、recovery codes、Refresh step-up 和 Refresh 期间直接提交 MFA proof。

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

## 面试可讲重点

- challenge token 不落库，PostgreSQL 只保存 `token_hash`；delivery outbox 里保存 AES-GCM 加密后的 token。
- RPC 成功表示“durable enqueue”，不是第三方 provider 已送达；真实投递由 worker 异步完成。
- worker 使用 ready query + row lock 拉取 delivery outbox，成功后标记 `DELIVERED`；失败按 retry / DLQ 处理。
- repair 工具是 audit-first，不解密 token、不直接标记 `DELIVERED`、不把 `EXPIRED` challenge 复活；这比把 challenge delivery DLQ 当普通 Kafka outbox replay 更安全。
- 本轮 smoke 没开 `NEXUSIM_IDENTITY_DEV_RETURN_CHALLENGE_TOKEN`，Confirm 使用的是 webhook 收到的真实 token，所以能证明 worker 链路生效。
- 该能力仍不是完整 email/SMS provider 或密钥管理平台；provider 模板、bounce handling、自动 key rotation、KMS/HSM keyring、统一告警仍是后续项。
