# identity-service

## 当前状态

- 已有 `RegisterUser`、`Login`、`RefreshGatewayToken`、JWKS / RS256 keyring、one-shot `gateway-token-keyring-rotate` 本地 operator、device/session revoke；keyring rotate 可通过 `NEXUSIM_IDENTITY_GATEWAY_TOKEN_KEYRING_ROTATE_OUTPUT` 写不含 JWK 材料的低敏 JSON summary。
- 已有 verification / password reset challenge、challenge delivery outbox、webhook / SMTP email challenge sender、first-stage SMTP subject/body templates、MFA TOTP、recovery codes、Refresh step-up、mTLS。
- 已补 `/healthz`、`/readyz`、`/debug/metrics`、Prometheus text `/metrics` 和 `/.well-known/jwks.json` / `/jwks.json`，可观察低敏 identity、MFA、challenge delivery outbox、worker retry、gRPC 和 challenge delivery debug 聚合状态；debug HTTP 监听默认只允许 loopback / 私网，公网或未指定地址必须显式 `NEXUSIM_IDENTITY_DEBUG_ALLOW_PUBLIC=true`。
- 本地 Prometheus scrape / alert rules 与 Grafana dashboard 原型已覆盖 identity gRPC error、login / MFA lock、challenge delivery failure / outbox DLQ、worker / relay error 和 OTLP endpoint missing；这仍是本地开发 / 面试演示级观测，不是生产 Alertmanager / SLO。
- 已补 first-stage OpenTelemetry gRPC server span，默认关闭；启用后可输出 stdout 或 OTLP gRPC trace，并从入口 metadata 提取 W3C traceparent。
- gRPC 结构化访问日志已限制 `trace_id` / `request_id` 为低敏安全字符集，外部 metadata 中夹带的邮箱、token 或任意文本不会进入日志 correlation 字段。
- `outbox-relay` 对非取消运行时错误已改为退避重试，并在 relay 模式暴露低敏 retry 快照；malformed payload / unsupported event 仍保持 fail-closed。
- Kafka writer 已显式固定 `acks=all`、禁自动建 topic、bounded attempts/backoff，并由本地门禁和 package 单测防漂移；真正 idempotent / transactional producer 仍属后续客户端选型。
- 普通 `identity_outbox` relay 写入 retry / DLQ 的 `last_error` 时只保存稳定低敏公开文案，不落 Kafka / 网络 / provider 原始错误正文。
- `challenge-delivery-worker` 对非取消运行时错误已改为退避重试，并在 worker 模式暴露低敏 retry 快照；decrypt failure / incomplete message / notifier error 仍保持 store 驱动的 retry / expire / DLQ 语义，持久化 last_error 只保存稳定公开文案和 failure_class，并有同步 sender 与 worker retry / DLQ 回归测试防止 provider body / destination / raw token 泄漏或低敏分类丢失。
- 当 `NEXUSIM_IDENTITY_ADMIN_AUTH_MODE=metadata|verified-metadata` 时，公网监听地址 + 无 gRPC mTLS client cert 的危险组合会在启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。
- TOTP / recovery-code proof 已在最终 Login / Refresh 事务内重新检查 lock；锁定期间不消费 proof、不写 session、不轮换 refresh token。
- 已有只读 `session-mfa-proof-audit`、`challenge-delivery-repair`、只读 `challenge-delivery-repair-audit`、`challenge-delivery-repair-cleanup` 和 `challenge-request-limit-cleanup` operator，用于发现历史 session MFA proof 脏数据、处理 challenge delivery repair、直接审计 repair 历史，以及按 retention / scope 清理 repair audit 或 challenge request limit 历史；`challenge-delivery-repair-cleanup` / `challenge-request-limit-cleanup` 分别支持 `NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_DRY_RUN=true` / `NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_DRY_RUN=true` 只统计候选行不删除；`challenge-delivery-repair-audit` 支持按 delivery / tenant / user / challenge / mode / outcome / failure class / `repaired_at` RFC3339 时间窗口缩小范围并输出 compacted filters；`session-mfa-proof-audit` / `challenge-delivery-repair` / audit / cleanup 可通过 `NEXUSIM_IDENTITY_SESSION_MFA_PROOF_AUDIT_OUTPUT` / `NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_OUTPUT` / `NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_OUTPUT` / `NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_OUTPUT` / `NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_OUTPUT` 写低敏 JSON summary、结果或 cleanup summary，便于 operator 留存证据。
- PostgreSQL repository 已拆出 challenge helper、challenge command methods、session/device/MFA proof、refresh token、MFA/recovery-code 和 identity outbox helpers，核心文件降到约 800 行；后续继续按主题拆测试和存储 helpers。
- app 层登录相关测试已按基础 login/register、MFA、Refresh token step-up、Challenge / Password reset 拆到同 package 测试文件，降低单文件复杂度并保留原覆盖面。
- cmd 层 challenge delivery、MFA、gateway token / JWKS 和 env config helpers 已拆到同 package 文件，`main.go` 继续保留进程模式和启动编排。
- `loadtest/identity` summary 已输出 `capacity_summary`，包含运行时长、identity 操作数、token 签发次数、预期 step-up 错误数、challenge delivery outbox 聚合、challenge delivery attempt 数、ops/s、latency p95/p99 和 MFA recovery code 数；后续容量验证可直接复用该结构。
- `capacity-baseline-identity-stack-20260616` 本地 stack 短基线已跑通：临时 identity gRPC + webhook fixture + `challenge-delivery-worker` 覆盖 Register/Login/Refresh/Password Reset/MFA/recovery-code 管理，`challenge_delivery_outbox DELIVERED=2/PENDING=0/DLQ=0`，`operations_per_second=14.09`；报告见 `loadtest/distributed/loadtest-report-20260616-identity-stack-capacity-baseline.md`。

## 后续

- WebAuthn/passkeys、OIDC federation、KMS/HSM、完整风控、SMS provider、bounce handling、租户级通知模板治理；统一 OTel collector、生产告警路由、retention、SLO dashboard、长时间容量曲线仍属于后续统一观测治理。
