# identity-service

## 当前状态

- 已有 `RegisterUser`、`Login`、`RefreshGatewayToken`、JWKS / RS256 keyring、device/session revoke。
- 已有 verification / password reset challenge、challenge delivery outbox、MFA TOTP、recovery codes、Refresh step-up、mTLS。
- TOTP / recovery-code proof 已在最终 Login / Refresh 事务内重新检查 lock；锁定期间不消费 proof、不写 session、不轮换 refresh token。
- 已有只读 `session-mfa-proof-audit` 运维模式，用于发现历史 session MFA proof 脏数据。
- PostgreSQL repository 已偏大；后续 identity 存储改动优先拆同 package 文件，不继续堆到单文件。

## 后续

- WebAuthn/passkeys、OIDC federation、KMS/HSM、完整风控、生产级 email/SMS provider。
