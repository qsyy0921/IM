# identity-service Loadtest Reports

本目录保存 `identity-service` 的小规模验证报告和阶段入口。当前重点是真实鉴权、challenge、MFA、refresh token 和 gateway token 安全链路；不是容量压测目录。

## 当前阶段结论

`identity-service` 已完成真实鉴权第一阶段主线，并补了多个安全 hardening 切片：

- `RegisterUser / Login / RefreshGatewayToken`。
- gateway token 本地验签链路：HS256 兼容、RS256 static key ring、one-shot keyring rotate operator、JWKS public-only、old public-key overlap。
- refresh token rotation、reuse detection、device/session revoke event。
- email/phone verification 与 password reset challenge，数据库只保存 token hash，并有 target-level active cap + request window throttle；配置 `NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_SECRET` 后，password reset 的无效 / 不存在目标也会进入 HMAC target limiter，并可用 one-shot cleanup operator 清理过期 limiter row，`/debug/metrics` 只暴露 limiter 聚合计数。
- challenge webhook sender、delivery failure 过期补偿、debug metrics、持久状态审计；`/debug/metrics` 同时暴露 challenge delivery outbox 的 PENDING / ready / scheduled / expired / DELIVERED / DLQ / CANCELED 聚合状态。
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
- challenge 创建有本地防滥用：同一目标最多 3 个 ACTIVE challenge，默认 15 分钟内最多 5 次创建；password reset 的限流仍返回 neutral accepted，避免暴露账号存在性。无效目标 limiter 只保存 HMAC target key，不保存 raw email/phone；`challenge-request-limit-cleanup` 只删除窗口和锁都过期的 limiter row，debug metrics 只输出总数和锁定数。
- RPC 成功表示“durable enqueue”，不是第三方 provider 已送达；真实投递由 worker 异步完成。
- worker 使用 ready query + row lock 拉取 delivery outbox，成功后标记 `DELIVERED`；失败按 retry / DLQ 处理；debug metrics 只暴露队列状态聚合和最大 pending retry，不暴露 token 密文、目标地址或 provider error body。
- repair 工具是 audit-first，不解密 token、不直接标记 `DELIVERED`、不把 `EXPIRED` challenge 复活；这比把 challenge delivery DLQ 当普通 Kafka outbox replay 更安全。
- 本轮 smoke 没开 `NEXUSIM_IDENTITY_DEV_RETURN_CHALLENGE_TOKEN`，Confirm 使用的是 webhook 收到的真实 token，所以能证明 worker 链路生效。
- `NEXUSIM_IDENTITY_SERVICE_MODE=gateway-token-keyring-rotate` 可以轮换本地 RS256 keyring 文件：生成新当前私钥，把旧当前 key 降级为 public-only overlap，并按 `NEXUSIM_IDENTITY_GATEWAY_TOKEN_ROTATE_OLD_KEY_LIMIT` 保留旧公钥。它不分发密钥、不接 KMS/HSM，也不是完整自动轮换平台。
- 该能力仍不是完整 email/SMS provider 或密钥管理平台；provider 模板、bounce handling、KMS/HSM keyring、统一告警仍是后续项。
