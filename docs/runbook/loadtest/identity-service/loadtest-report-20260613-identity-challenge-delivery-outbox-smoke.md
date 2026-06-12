# identity-service Challenge Delivery Outbox Smoke

本报告记录 `identity-service` challenge delivery outbox 的最小真实进程 smoke。

这不是邮件 / 短信 provider 生产验收，也不是容量压测；它只验证本地真实进程链路：gRPC 进程以 `outbox` 模式创建 challenge 和 encrypted delivery row，独立 `challenge-delivery-worker` 解密 token 并调用 webhook，客户端再用 webhook token 完成验证。

## Command

```powershell
.\loadtest\identity\run-local-smoke.ps1
```

脚本会：

- 应用 `migrations/postgres/identity/*.sql`。
- 启动本地 webhook receiver。
- 启动 `identity-service` gRPC：`NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_MODE=outbox`。
- 启动 `identity-service` worker：`NEXUSIM_IDENTITY_SERVICE_MODE=challenge-delivery-worker`。
- 运行 `loadtest/identity` client：`RegisterUser -> RequestVerificationChallenge -> wait webhook -> ConfirmVerificationChallenge`。

## Result

| Item | Value |
| --- | --- |
| Code commit | `9e48451` |
| `git_dirty` | `false` |
| Result dir | `H:\NexusIM\loadtest-results\identity-challenge-delivery-outbox-smoke-20260613-043646` |
| Summary | `H:\NexusIM\loadtest-results\identity-challenge-delivery-outbox-smoke-20260613-043646\identity-summary.json` |
| gRPC target | `127.0.0.1:51605` |
| Webhook URL | `http://127.0.0.1:51606/challenge` |

关键 summary：

```json
{
  "success": true,
  "request_verification_challenge": {
    "channel": "VERIFICATION_CHANNEL_EMAIL",
    "dev_challenge_token_set": false
  },
  "webhook": {
    "received": true,
    "challenge_type": "EMAIL_VERIFICATION",
    "channel": "EMAIL",
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

## Conclusion

本轮证明了 challenge delivery outbox 的最小闭环：

```text
RegisterUser
-> RequestVerificationChallenge(outbox)
-> identity_challenge_delivery_outbox(PENDING)
-> challenge-delivery-worker
-> webhook receives token
-> ConfirmVerificationChallenge
-> identity_challenge_delivery_outbox(DELIVERED)
-> identity_challenges(CONSUMED, delivery_status=DELIVERED)
```

关键点：

- `dev_challenge_token_set=false`，说明客户端没有走开发模式直返 token。
- webhook 收到的 token 能完成 `ConfirmVerificationChallenge`。
- outbox 无积压：`PENDING=0 / DELIVERED=1 / DLQ=0`。
- challenge 状态最终为 `CONSUMED`，delivery 状态为 `DELIVERED`。

## Limits

- 本轮 webhook 是本地 receiver，不代表真实 SMTP / SMS provider。
- 未覆盖 provider 失败后的 retry / DLQ repair / 人工审批。
- 未覆盖 KMS/HSM keyring、token key rotation、provider template、bounce handling、统一 trace / alert。
- 这是小规模功能 smoke，不是容量或 HA 结论。
