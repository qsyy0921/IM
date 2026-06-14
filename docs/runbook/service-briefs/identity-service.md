# identity-service

## 当前状态

- 已有 `RegisterUser`、`Login`、`RefreshGatewayToken`、JWKS / RS256 keyring、device/session revoke。
- 已有 verification / password reset challenge、challenge delivery outbox、MFA TOTP、recovery codes、Refresh step-up、mTLS。
- 已补 `/healthz`、`/readyz`、`/debug/metrics` 和 `/.well-known/jwks.json` / `/jwks.json`，可观察低敏 identity、MFA、challenge delivery outbox 和 challenge delivery debug 聚合状态。
- `outbox-relay` 对非取消运行时错误已改为退避重试，并在 relay 模式暴露低敏 retry 快照；malformed payload / unsupported event 仍保持 fail-closed。
- `challenge-delivery-worker` 对非取消运行时错误已改为退避重试，并在 worker 模式暴露低敏 retry 快照；decrypt failure / incomplete message / notifier error 仍保持 store 驱动的 retry / expire / DLQ 语义。
- TOTP / recovery-code proof 已在最终 Login / Refresh 事务内重新检查 lock；锁定期间不消费 proof、不写 session、不轮换 refresh token。
- 已有只读 `session-mfa-proof-audit`、只读 `challenge-delivery-repair-audit` 和 `challenge-delivery-repair-cleanup` operator，用于发现历史 session MFA proof 脏数据、直接审计 challenge delivery repair 历史，以及按 retention / scope 清理 repair audit 历史。
- PostgreSQL repository 已拆出 challenge helpers，核心文件降到 2500 行以下；后续继续按主题拆测试和存储 helpers。

## 后续

- WebAuthn/passkeys、OIDC federation、KMS/HSM、完整风控、生产级 email/SMS provider。
