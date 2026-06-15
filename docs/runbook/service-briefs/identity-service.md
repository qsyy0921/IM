# identity-service

## 当前状态

- 已有 `RegisterUser`、`Login`、`RefreshGatewayToken`、JWKS / RS256 keyring、device/session revoke。
- 已有 verification / password reset challenge、challenge delivery outbox、webhook / SMTP email challenge sender、first-stage SMTP subject/body templates、MFA TOTP、recovery codes、Refresh step-up、mTLS。
- 已补 `/healthz`、`/readyz`、`/debug/metrics`、Prometheus text `/metrics` 和 `/.well-known/jwks.json` / `/jwks.json`，可观察低敏 identity、MFA、challenge delivery outbox、worker retry、gRPC 和 challenge delivery debug 聚合状态；debug HTTP 监听默认只允许 loopback / 私网，公网或未指定地址必须显式 `NEXUSIM_IDENTITY_DEBUG_ALLOW_PUBLIC=true`。
- 本地 Prometheus scrape / alert rules 与 Grafana dashboard 原型已覆盖 identity gRPC error、login / MFA lock、challenge delivery failure / outbox DLQ、worker / relay error 和 OTLP endpoint missing；这仍是本地开发 / 面试演示级观测，不是生产 Alertmanager / SLO。
- 已补 first-stage OpenTelemetry gRPC server span，默认关闭；启用后可输出 stdout 或 OTLP gRPC trace，并从入口 metadata 提取 W3C traceparent。
- gRPC 结构化访问日志已限制 `trace_id` / `request_id` 为低敏安全字符集，外部 metadata 中夹带的邮箱、token 或任意文本不会进入日志 correlation 字段。
- `outbox-relay` 对非取消运行时错误已改为退避重试，并在 relay 模式暴露低敏 retry 快照；malformed payload / unsupported event 仍保持 fail-closed。
- Kafka writer 已显式固定 `acks=all`、禁自动建 topic、bounded attempts/backoff，并由本地门禁防漂移；真正 idempotent / transactional producer 仍属后续客户端选型。
- 普通 `identity_outbox` relay 写入 retry / DLQ 的 `last_error` 时只保存稳定低敏公开文案，不落 Kafka / 网络 / provider 原始错误正文。
- `challenge-delivery-worker` 对非取消运行时错误已改为退避重试，并在 worker 模式暴露低敏 retry 快照；decrypt failure / incomplete message / notifier error 仍保持 store 驱动的 retry / expire / DLQ 语义，持久化 last_error 只保存稳定公开文案和 failure_class，并有回归测试防止 provider body / destination / raw token 泄漏或低敏分类丢失。
- 当 `NEXUSIM_IDENTITY_ADMIN_AUTH_MODE=metadata|verified-metadata` 时，公网监听地址 + 无 gRPC mTLS client cert 的危险组合会在启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。
- TOTP / recovery-code proof 已在最终 Login / Refresh 事务内重新检查 lock；锁定期间不消费 proof、不写 session、不轮换 refresh token。
- 已有只读 `session-mfa-proof-audit`、只读 `challenge-delivery-repair-audit` 和 `challenge-delivery-repair-cleanup` operator，用于发现历史 session MFA proof 脏数据、直接审计 challenge delivery repair 历史，以及按 retention / scope 清理 repair audit 历史。
- PostgreSQL repository 已拆出 challenge、session/device/MFA proof、refresh token 和 identity outbox helpers，核心文件降到 1800 行以下；后续继续按主题拆测试和存储 helpers。

## 后续

- WebAuthn/passkeys、OIDC federation、KMS/HSM、完整风控、SMS provider、bounce handling、租户级通知模板治理；统一 OTel collector、生产告警路由、retention、SLO dashboard 仍属于后续统一观测治理。
