# api-gateway SDD v0.1

## 定位

`api-gateway` 是 NexusIM 第一版统一 user-facing gRPC 入口。它负责把客户端提交的 gateway token 验证为可信身份，再把该身份转换为下游服务已经支持的 gateway verified gRPC metadata。

它不拥有业务事实，不写 PostgreSQL，不消费或发布 Kafka，也不直接访问任何微服务内部表。

## 第一阶段范围

第一阶段新增 `nexusim.gateway.v1.GatewayService` 作为 public facade proto，复用已有服务的 request / response message，先暴露客户端需要的公开 RPC：

- `ConversationService`
- `MessageService`
- `DeliveryService`
- `ReceiptService`
- `ContactsService`

覆盖 demo 主链路：

```text
CreateMemberChange
-> SendMessage
-> PullInbox
-> MarkRead
-> ListConversations
```

为了兼容已有 smoke 和过渡期客户端，gateway 默认仍同时注册 contacts / conversation / message / delivery / receipt 的 legacy service descriptor；`NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS=false` 时只注册 `GatewayService` public facade。新客户端应优先切到 `GatewayService`，再逐步关闭 legacy descriptor 暴露面。gateway 内部仍调用对应下游 service client。

## 鉴权与身份传播

入口请求必须携带 gateway token：

- `authorization: Bearer <token>`
- 或 first-stage 本地兼容 metadata `x-nexusim-gateway-token`

第一阶段复用共享 `internal/gatewayauth`：

- HMAC legacy token / HS256 JWT。
- RS256 JWT + JWKS / issuer allowlist。
- 默认 audience 为 `api-gateway`；如需兼容历史 `push-gateway` token，必须显式配置 `NEXUSIM_API_GATEWAY_AUTH_AUDIENCE=push-gateway`，不能作为默认生产口径。

`NEXUSIM_API_GATEWAY_AUTH_MODE=mock` 只允许本地 smoke 使用。若 gateway gRPC 监听地址不是 loopback 或 RFC1918 私网地址，进程应在启动前直接失败，避免把裸 query 身份模式暴露到公网。`hmac` / `jwt` 模式若用于非私网监听地址，也必须同时启用入口 gRPC TLS；否则进程应在启动前直接失败，避免把签名 token 暴露到 plaintext 公网入口。

gateway 验证 token 后向下游注入：

```text
x-nexusim-tenant-id
x-nexusim-user-id
x-nexusim-device-id
x-nexusim-session-id
x-nexusim-trace-id
x-nexusim-request-id
```

gateway 会重写 request body 里的 `AuthContext`，以已验证 token 身份为准。客户端传入的 body 身份和 `x-nexusim-*` metadata 都不能作为可信身份来源。

## 不暴露的接口

`ConversationService/GetSendContext` 是 message-service 的服务间 read path，不包含在 `GatewayService` public facade 中。兼容期 legacy `ConversationService` descriptor 默认仍会注册该 method，但 api-gateway 对它返回 `Unimplemented`；关闭 legacy descriptor 后该 method 不再出现在 api-gateway 暴露面。

`PolicyService/CheckMessageAction` 也是内部策略判定面，不通过 api-gateway 暴露给客户端。

## 运行模式

第一版 runtime：

```text
NEXUSIM_API_GATEWAY_MODE=grpc
NEXUSIM_API_GATEWAY_GRPC_ADDR=0.0.0.0:12000
NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS=true
```

`NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS` 默认 `true`，用于兼容已有客户端和历史 smoke。secure demo wrapper 已走 `GatewayService` facade，可设置为 `false` 验证用户入口不再依赖旧 service descriptor；这只改变 api-gateway 对外注册的 gRPC service descriptor，不改变下游后端连接和转发逻辑。

Auth audience：

```text
NEXUSIM_API_GATEWAY_AUTH_AUDIENCE=api-gateway
```

该配置只接受单个 audience。第一阶段 identity-service / demo runner 均可在签发 gateway token 时指定 `aud=api-gateway`；push-gateway 仍继续使用自身的 `push-gateway` audience。这样 online WebSocket token 和 api-gateway user-facing RPC token 不再默认复用同一个 audience。

First-stage rate limit：

```text
NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED=false
NEXUSIM_API_GATEWAY_RATE_LIMIT_BACKEND=local
NEXUSIM_API_GATEWAY_RATE_LIMIT_SCOPE=token
NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS=100
NEXUSIM_API_GATEWAY_RATE_LIMIT_BURST=200
NEXUSIM_API_GATEWAY_RATE_LIMIT_MAX_KEYS=10000
```

该限流器默认关闭；启用后默认按 `gRPC method + token fingerprint` 限流，缺少 token 时退化到 `gRPC method + peer address`。`NEXUSIM_API_GATEWAY_RATE_LIMIT_SCOPE=tenant` 时，api-gateway 会先用已有 gateway authenticator 验证 token，再按 `gRPC method + tenant_id` 做 first-stage 租户级 quota；无效 token 不会计入某个租户，而是回退到 token / peer key，并由后续鉴权返回稳定错误。它不记录 token 原文、tenant_id 或 user_id，也不向业务服务透出限流 key。被限流请求返回 `ResourceExhausted / rate limit exceeded`，并携带 gRPC `RetryInfo`：local backend 使用 token bucket 补齐下一枚 token的估算等待时间，Redis backend 使用 fixed-window 下一窗口剩余时间。该请求也会进入 api-gateway gRPC metrics。

`local` backend 是本进程 token bucket。需要跨实例共享入口预算时启用 Redis backend：

```text
NEXUSIM_API_GATEWAY_RATE_LIMIT_BACKEND=redis
NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_MODE=single
NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_ADDR=127.0.0.1:6379
NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_KEY_PREFIX=nexusim:api-gateway
NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_WINDOW=1s
NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_FAIL_OPEN=true
```

Redis backend 使用 fixed-window counter，把同一 token/method 或 tenant/method 在多个 api-gateway 实例上的请求合并计数。`FAIL_OPEN=true` 时 Redis 短故障只记录 `redis_error_count` 并放行请求；启动阶段 Redis `PING` 失败也不会阻止 api-gateway 启动，后续请求仍会按运行时 Redis 错误计数并放行。`FAIL_OPEN=false` 时启动探测失败会阻止进程启动，运行时 Redis 错误返回 `Unavailable / rate limiter unavailable`。

这是第一阶段分布式入口保护，不是完整产品级配额系统：tenant plan、动态 per-tenant 配额配置、IP reputation、WAF、自适应风控、跨区域一致性、Redis 限流故障演练和统一告警仍是后续 production hardening。

入口 gRPC 默认 plaintext；本地 secure smoke 和后续部署可以启用静态 TLS / mTLS：

```text
NEXUSIM_API_GATEWAY_GRPC_TLS_CERT_FILE
NEXUSIM_API_GATEWAY_GRPC_TLS_KEY_FILE
NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_CA_FILE
NEXUSIM_API_GATEWAY_GRPC_TLS_REQUIRE_CLIENT_CERT
NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES
NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_ALLOWED_URIS
```

如果启用 client cert 校验，allowlist 使用 exact-match DNS / URI SAN；这仍是第一阶段静态配置，不是证书签发、轮换、撤销或动态服务身份治理。

Debug endpoint：

```text
NEXUSIM_API_GATEWAY_DEBUG_ADDR=127.0.0.1:12001
```

启用后暴露：

```text
/healthz
/readyz
/debug/metrics
```

`/debug/metrics` 只输出进程内聚合指标：gRPC method/code/count/error_count/latency 和 JWT/JWKS 缓存刷新状态。gRPC access log 只记录 service/event/method/code/latency_ms/request_id/trace_id，不记录 gateway token、tenant_id、user_id、device_id、session_id 或 request body。该 endpoint 是 first-stage local/debug observability，不是 Prometheus 指标规范、统一 trace 或生产审计日志。

下游地址：

```text
NEXUSIM_API_GATEWAY_CONVERSATION_ADDR=127.0.0.1:10496
NEXUSIM_API_GATEWAY_MESSAGE_ADDR=127.0.0.1:10495
NEXUSIM_API_GATEWAY_DELIVERY_ADDR=127.0.0.1:10497
NEXUSIM_API_GATEWAY_RECEIPT_ADDR=127.0.0.1:10499
NEXUSIM_API_GATEWAY_CONTACTS_ADDR=127.0.0.1:10500
```

下游 client 默认 plaintext，但已支持与现有 smoke client 相同的静态 TLS / mTLS 配置：

```text
NEXUSIM_API_GATEWAY_CONVERSATION_TLS_CA_FILE
NEXUSIM_API_GATEWAY_CONVERSATION_TLS_SERVER_NAME
NEXUSIM_API_GATEWAY_CONVERSATION_TLS_CLIENT_CERT_FILE
NEXUSIM_API_GATEWAY_CONVERSATION_TLS_CLIENT_KEY_FILE

NEXUSIM_API_GATEWAY_MESSAGE_TLS_CA_FILE
NEXUSIM_API_GATEWAY_MESSAGE_TLS_SERVER_NAME
NEXUSIM_API_GATEWAY_MESSAGE_TLS_CLIENT_CERT_FILE
NEXUSIM_API_GATEWAY_MESSAGE_TLS_CLIENT_KEY_FILE

NEXUSIM_API_GATEWAY_DELIVERY_TLS_CA_FILE
NEXUSIM_API_GATEWAY_DELIVERY_TLS_SERVER_NAME
NEXUSIM_API_GATEWAY_DELIVERY_TLS_CLIENT_CERT_FILE
NEXUSIM_API_GATEWAY_DELIVERY_TLS_CLIENT_KEY_FILE

NEXUSIM_API_GATEWAY_RECEIPT_TLS_CA_FILE
NEXUSIM_API_GATEWAY_RECEIPT_TLS_SERVER_NAME
NEXUSIM_API_GATEWAY_RECEIPT_TLS_CLIENT_CERT_FILE
NEXUSIM_API_GATEWAY_RECEIPT_TLS_CLIENT_KEY_FILE

NEXUSIM_API_GATEWAY_CONTACTS_TLS_CA_FILE
NEXUSIM_API_GATEWAY_CONTACTS_TLS_SERVER_NAME
NEXUSIM_API_GATEWAY_CONTACTS_TLS_CLIENT_CERT_FILE
NEXUSIM_API_GATEWAY_CONTACTS_TLS_CLIENT_KEY_FILE
```

启用下游服务 `metadata` / `verified-metadata` auth 时，不能只依赖 metadata 字段本身。后端必须只暴露在可信内网 / loopback，或者启用 gRPC mTLS 并把 client DNS / URI SAN allowlist 收敛到 `api-gateway.nexusim.local` / `spiffe://nexusim/api-gateway` 一类明确服务身份；否则客户端直连后端仍可伪造 metadata。

2026-06-14 补充：api-gateway 当前已在启动阶段执行第一版 trusted-metadata guard。若下游 `*_AUTH_MODE` 配为 `metadata` / `verified-metadata`，且下游地址不是 loopback / RFC1918 私网地址，同时 gateway 侧没有配置下游 gRPC client certificate，则进程会直接启动失败，避免把 trusted metadata 注入到明显不受保护的公网直连链路上。当前 guard 已覆盖 conversation / message / delivery / receipt / contacts / identity 下游。这仍不是完整零信任网络治理；真正的生产边界仍应依赖 mTLS + 服务身份 allowlist。

## 边界与后续

第一阶段不是完整 API gateway / BFF：

- 不做 HTTP / REST / GraphQL 转换。
- 不做 OIDC federation。
- 不做全服务 mTLS rollout 或证书生命周期治理。
- 不做限流、配额、WAF、OpenTelemetry 全链路 trace。
- 不替代 push-gateway 的 WebSocket online notify / ACK 转发职责。

后续优先级：

1. 继续让新客户端优先使用 `nexusim.gateway.v1.GatewayService`，并用 `NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS=false` 做 facade-only smoke。
2. 继续把限流、配额、审计采样和 tracing 作为独立 production hardening，不塞进当前 proxy skeleton。

2026-06-13 补充：clean commit `cff1668` 已跑通 `loadtest/demo/run-local-secure-demo.ps1` 经真实 api-gateway 的 secure E2E smoke。demo runner 对 conversation / message / delivery / receipt 的 gRPC target 均指向 api-gateway，使用 HMAC gateway token 和 desktop-client mTLS；api-gateway 再通过 mTLS 调下游，并向下游注入 trusted metadata。报告见 `docs/runbook/loadtest/demo/loadtest-report-20260613-e2e-demo-api-gateway-secure-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-api-gateway-secure-smoke-20260613-clean`。

2026-06-13 补充：api-gateway 已补 first-stage `/healthz`、`/readyz`、`/debug/metrics` 和低敏 gRPC JSON access log；`run-local-secure-demo.ps1` 会启动 `NEXUSIM_API_GATEWAY_DEBUG_ADDR=127.0.0.1:11904` 并把 metrics 保存为 `api-gateway-debug-metrics.json`。

2026-06-13 补充：clean commit `9335bd1` 已把 api-gateway 默认 `NEXUSIM_API_GATEWAY_AUTH_AUDIENCE` 收紧为 `api-gateway`；secure demo runner 生成 `aud=api-gateway` 的 HMAC gateway token，并通过 `e2e-demo-api-gateway-audience-smoke-20260613-clean` 验证真实 api-gateway + 下游 mTLS 链路仍可完成 E2E 主流程。历史 `push-gateway` audience 仍可通过显式 env 配置兼容，但不再是 api-gateway 默认值。

2026-06-13 补充：api-gateway 已补第一阶段 gRPC rate limiter，默认关闭；`local` backend 为进程内 token bucket，`redis` backend 为跨实例 fixed-window counter，并在 `/debug/metrics` 输出低敏 `rate_limit` 聚合状态。该能力不是完整产品级 quota / WAF / 风控系统。

2026-06-14 补充：clean commit `ee4461e` 已给 api-gateway rate-limit 拒绝响应补 gRPC `RetryInfo`，让客户端在 `ResourceExhausted / rate limit exceeded` 时获得保守重试等待时间；单元测试覆盖 local token bucket 与 Redis fixed-window 两种后端。这仍不是完整客户端退避策略或 tenant quota。

2026-06-14 补充：api-gateway rate limiter 已新增 `NEXUSIM_API_GATEWAY_RATE_LIMIT_SCOPE=tenant`，在启用时使用已验证 gateway token 的 `tenant_id` 作为 low-sensitive per-method quota key；`/debug/metrics` 输出 `key_scope` 和 `identity_error_count`，不输出 token、tenant_id 或 user_id。无效 token 不会污染租户预算，会回退到 token / peer key 并继续由鉴权层返回稳定错误。这仍不是动态 tenant plan 或完整 WAF / 风控系统。

2026-06-14 补充：clean commit `9b16b8c` 已修正 Redis rate-limit fail-open 启动语义：`NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_FAIL_OPEN=true` 时 Redis `PING` 失败只记录启动日志并继续启动，首个请求上的 Redis 错误仍进入 `redis_error_count` 并放行；`FAIL_OPEN=false` 仍 fail-closed 拒绝启动。

2026-06-14 补充：clean commit `b4b3714` 已补 api-gateway 第一阶段 W3C `traceparent` 输入桥接：当 gateway token 和 `x-nexusim-trace-id` 都没有提供 trace id 时，api-gateway 会从合法 `traceparent` 提取 32 hex trace id，规范化为小写后写入下游 `x-nexusim-trace-id`、gRPC response header 和低敏 access log；非法 `traceparent` 会被忽略并回退到本地 `trace_*` 生成。这只是外部 trace 入口兼容，不是完整 OpenTelemetry span、collector 或 exporter。

2026-06-13 补充：api-gateway 已新增第一阶段 `nexusim.gateway.v1.GatewayService` public facade proto，覆盖 conversation / message / delivery / receipt 的 user-facing RPC，并明确不包含服务间 `GetSendContext`。legacy service descriptor 暂时保留用于兼容；下一步是让 demo runner / 客户端切到 facade 后再收敛旧 descriptor。

2026-06-14 补充：clean commit `3473ffc` 已把 identity-service 的 public / self-service RPC 接入 `GatewayService` facade：`RegisterUser / Login / RefreshGatewayToken / RequestVerificationChallenge / ConfirmVerificationChallenge / RequestPasswordReset / ConfirmPasswordReset / BeginMFAEnrollment / ConfirmMFAEnrollment / DisableMFAFactor / RegenerateMFARecoveryCodes / RevokeMFARecoveryCodes`。这些入口不要求已有 gateway token，只负责补齐/透传 `trace_id / request_id` 并转发到 identity-service；`IssueGatewayToken / RevokeDevice / RevokeSession / GetDeviceState` 仍不进入 public facade，也不注册 legacy `IdentityService` descriptor。

2026-06-14 补充：clean commit `443029a` 已让 `loadtest/identity` 支持 `--gateway-facade`，并跑通 identity challenge delivery 经 `GatewayService` facade 的真实进程 smoke：`RegisterUser -> RequestVerificationChallenge(outbox) -> challenge-delivery-worker -> webhook token -> ConfirmVerificationChallenge`。summary `git_dirty=false/success=true/gateway_facade=true`，api-gateway access log 只出现 `/nexusim.gateway.v1.GatewayService/RegisterUser|RequestVerificationChallenge|ConfirmVerificationChallenge`；报告见 `docs/runbook/loadtest/identity-service/loadtest-report-20260614-identity-api-gateway-facade-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\identity-api-gateway-facade-smoke-20260614-clean`。

2026-06-14 补充：clean commit `d07373a` 已把同一 identity facade smoke 扩展到 `Login -> RefreshGatewayToken`，summary `git_dirty=false/success=true/gateway_facade=true`，`login.gateway_token_set=true/refresh_token_set=true`，`refresh_gateway_token.refresh_token_rotated=true`，且 summary 不写 token 明文；api-gateway access log 显示 `/nexusim.gateway.v1.GatewayService/Login|RefreshGatewayToken` 均为 `OK`。原始结果在 `H:\NexusIM\loadtest-results\identity-api-gateway-login-refresh-smoke-20260614-clean`。

2026-06-14 补充：clean commit `852a992` 已把 identity facade smoke 继续扩展到 `RequestPasswordReset -> ConfirmPasswordReset -> post-reset Login`。summary `git_dirty=false/success=true/gateway_facade=true`，`request_password_reset.dev_challenge_token_set=false`，`password_reset_webhook.received=true/token_set=true/authorization_ok=true`，`confirm_password_reset.reset_at_unix_ms` 非零，`post_reset_login.gateway_token_set=true/refresh_token_set=true`，`challenge_delivery_outbox.total=2/delivered=2/pending=0/dlq=0`；summary 不写 verification token、password reset token、gateway token 或 refresh token 明文。原始结果在 `H:\NexusIM\loadtest-results\identity-api-gateway-password-reset-smoke-20260614-clean`。

2026-06-14 补充：clean commit `5fbc622` 已把 identity facade smoke 继续扩展到 MFA lifecycle：`BeginMFAEnrollment -> ConfirmMFAEnrollment -> Login without MFA rejected -> Login with TOTP -> RegenerateMFARecoveryCodes -> RevokeMFARecoveryCodes -> DisableMFAFactor`。summary `git_dirty=false/success=true/gateway_facade=true`，MFA factor 从 `PENDING` 到 `ACTIVE` 再到 `DISABLED`，无 MFA 的 Login 返回 `FailedPrecondition`，TOTP Login 成功，recovery codes 再生成和吊销各记录 `10` 条；summary 不写 TOTP secret、otpauth URI、recovery code、gateway token 或 refresh token 明文。原始结果在 `H:\NexusIM\loadtest-results\identity-api-gateway-mfa-facade-smoke-20260614-clean`。

2026-06-14 补充：clean commit `e464209` 已把 identity facade smoke 继续扩展到 Refresh step-up：启用 ACTIVE TOTP 后，用旧 password-only session refresh token 调 `RefreshGatewayToken` 且不提交 MFA proof 会返回 `FailedPrecondition`；同一 refresh token 携带 TOTP proof 时成功签发 gateway token 并轮换 refresh token。summary `git_dirty=false/success=true/gateway_facade=true`，`refresh_without_mfa.code=FailedPrecondition`，`refresh_with_mfa.gateway_token_set=true/refresh_token_set=true/refresh_token_rotated=true`，且不写任何 token / TOTP secret / recovery code 明文。原始结果在 `H:\NexusIM\loadtest-results\identity-api-gateway-refresh-stepup-smoke-20260614-clean`。

2026-06-13 补充：clean commit `bb13300` 已跑通 `run-local-secure-demo.ps1` 的 `--gateway-facade` 真实进程 smoke。summary `git_dirty=false/success=true/gateway_facade=true/gateway_auth_mode=hmac/gateway_auth_audience=api-gateway`，api-gateway debug metrics 显示本轮 user-facing gRPC calls 均走 `/nexusim.gateway.v1.GatewayService/...`，报告见 `docs/runbook/loadtest/demo/loadtest-report-20260613-e2e-demo-api-gateway-facade-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-api-gateway-facade-smoke-20260613-clean`。

2026-06-14 补充：api-gateway 注册层已支持 `NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS=false`，默认仍为 `true` 保持兼容。clean commit `c1328ca` 已跑通 facade-only 真实进程 smoke：`run-local-secure-demo.ps1` 启动 api-gateway 时关闭 legacy descriptor，summary `git_dirty=false/success=true/gateway_facade=true`，api-gateway metrics 只出现 `/nexusim.gateway.v1.GatewayService/...` user-facing method；报告见 `docs/runbook/loadtest/demo/loadtest-report-20260614-e2e-demo-api-gateway-facade-only-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-api-gateway-facade-only-smoke-20260614-clean`。

2026-06-14 补充：`GatewayService` public facade 已扩展 contacts-service 的 user-facing RPC：`SendContactRequest / RespondContactRequest / CancelContactRequest / ListContactRequests / ListContacts / GetContactState / DeleteContact / BlockContact / UnblockContact / UpdateContactRemark`。api-gateway 仍只做身份验证、`AuthContext` 重写和转发，不拥有联系人事实源，也不让 message-service 同步依赖 contacts-service。

2026-06-14 补充：clean commit `f3b290f` 已跑通 contacts 经 `GatewayService` facade 的真实进程 smoke。runner 使用 HMAC gateway token 调 api-gateway，api-gateway 关闭 legacy descriptor 并转发到 `NEXUSIM_CONTACTS_AUTH_MODE=metadata` 的 contacts-service；summary `git_dirty=false/success=true/gateway_facade=true/gateway_auth_mode=hmac`，api-gateway access log 显示 contacts 调用均为 `/nexusim.gateway.v1.GatewayService/...`，contacts outbox `PUBLISHED=2/PENDING=0/DLQ=0`；报告见 `docs/runbook/loadtest/contacts-service/loadtest-report-20260614-contacts-api-gateway-facade-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\contacts-api-gateway-facade-smoke-20260614-clean`。

2026-06-14 补充：clean commit `ccae224` 已补 api-gateway first-stage correlation propagation。api-gateway 会优先保留 gateway token / incoming metadata 中已有的 `trace_id / request_id`，缺失时生成 `trace_* / request_*`，并把最终值写入下游 trusted metadata、gRPC response header 和低敏 JSON access log。contacts facade correlation smoke 已通过，api-gateway access log 显示 `/nexusim.gateway.v1.GatewayService/SendContactRequest` 等 method 带最终 `trace_id` 和 `request_id`；报告见 `docs/runbook/loadtest/api-gateway/loadtest-report-20260614-api-gateway-correlation-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\contacts-api-gateway-correlation-smoke-20260614-clean`。这仍不是完整 OpenTelemetry trace / collector / alerting。

2026-06-14 补充：clean commit `b022a09` 已补 contacts-service 下游 gRPC access log 的 `trace_id / request_id` 输出，并跑通 contacts 经 `GatewayService` facade 的 downstream correlation smoke。summary `git_dirty=false/success=true/gateway_facade=true/gateway_auth_mode=hmac`，contacts-service 日志显示 `/nexusim.contacts.v1.ContactsService/SendContactRequest`、`ListContactRequests`、`RespondContactRequest`、`ListContacts`、`GetContactState` 均带 api-gateway 注入的 correlation；报告见 `docs/runbook/loadtest/api-gateway/loadtest-report-20260614-api-gateway-downstream-correlation-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\contacts-api-gateway-downstream-correlation-smoke-20260614-clean`。本轮只完成 contacts-service 第一站，不代表所有服务 access log / Kafka envelope / OpenTelemetry exporter 已完成。

2026-06-14 补充：clean commit `ce69c88` 已补 message-service 下游 gRPC access log 的 `trace_id / request_id` 输出，并跑通 secure E2E demo 经 `GatewayService` facade 的 message correlation smoke。summary `git_dirty=false/success=true/gateway_facade=true/gateway_auth_mode=hmac`，api-gateway 和 message-service 的 `SendMessage` access log 均显示 `trace_id=e2e-demo-send/request_id=e2e-demo-send`；报告见 `docs/runbook/loadtest/api-gateway/loadtest-report-20260614-api-gateway-message-correlation-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-api-gateway-message-correlation-smoke-20260614-clean`。本轮仍不是全服务 OpenTelemetry / collector / alerting。

2026-06-14 补充：clean commit `50685b4` 已补 delivery-service 与 receipt-service 下游 gRPC access log 的 `trace_id / request_id` 输出，并跑通 secure E2E demo 经 `GatewayService` facade 的 delivery / receipt correlation smoke。summary `git_dirty=false/success=true/gateway_facade=true/gateway_auth_mode=hmac`，api-gateway `PullInbox / ListConversations / MarkRead` 与下游 delivery-service `PullInbox`、receipt-service `ListConversations / MarkRead` access log 均显示同一组 correlation；报告见 `docs/runbook/loadtest/api-gateway/loadtest-report-20260614-api-gateway-delivery-receipt-correlation-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-api-gateway-delivery-receipt-correlation-smoke-20260614-clean`。这仍不是全服务 OpenTelemetry / collector / alerting。

2026-06-14 补充：clean commit `c6c3bc5` 已补 conversation-service 下游 gRPC access log 的 `trace_id / request_id` 输出，并跑通 secure E2E demo 经 `GatewayService` facade 的 conversation correlation smoke。summary `git_dirty=false/success=true/gateway_facade=true/gateway_auth_mode=hmac`，api-gateway `CreateMemberChange` 与 conversation-service `CreateMemberChange` access log 均显示 `trace_id=e2e-demo-join/request_id=e2e-demo-join`；报告见 `docs/runbook/loadtest/api-gateway/loadtest-report-20260614-api-gateway-conversation-correlation-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-api-gateway-conversation-correlation-smoke-20260614-clean`。本轮覆盖 user-facing conversation RPC；message-service -> conversation-service `GetSendContext` 服务间 RPC 的 correlation 透传仍是后续项。

2026-06-14 补充：clean commit `4ecb05b` 已补 message-service conversation RPC client 的服务间 correlation metadata 透传，并跑通 secure E2E demo。api-gateway `SendMessage`、message-service `SendMessage` 和 conversation-service `GetSendContext` access log 均显示 `trace_id=e2e-demo-send/request_id=e2e-demo-send`；报告见 `docs/runbook/loadtest/api-gateway/loadtest-report-20260614-message-conversation-correlation-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-message-conversation-correlation-smoke-20260614-clean`。这仍不是全服务 OpenTelemetry；其它服务间 RPC 与 Kafka envelope correlation 仍是后续项。
