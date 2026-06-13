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

覆盖 demo 主链路：

```text
CreateMemberChange
-> SendMessage
-> PullInbox
-> MarkRead
-> ListConversations
```

为了兼容已有 smoke 和过渡期客户端，gateway 当前仍同时注册 conversation / message / delivery / receipt 的 legacy service descriptor；后续客户端应优先切到 `GatewayService`，再逐步收敛 legacy descriptor 暴露面。gateway 内部仍调用对应下游 service client。

## 鉴权与身份传播

入口请求必须携带 gateway token：

- `authorization: Bearer <token>`
- 或 first-stage 本地兼容 metadata `x-nexusim-gateway-token`

第一阶段复用共享 `internal/gatewayauth`：

- HMAC legacy token / HS256 JWT。
- RS256 JWT + JWKS / issuer allowlist。
- 默认 audience 为 `api-gateway`；如需兼容历史 `push-gateway` token，必须显式配置 `NEXUSIM_API_GATEWAY_AUTH_AUDIENCE=push-gateway`，不能作为默认生产口径。

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

`ConversationService/GetSendContext` 是 message-service 的服务间 read path，不包含在 `GatewayService` public facade 中。兼容期 legacy `ConversationService` descriptor 仍会注册该 method，但 api-gateway 对它返回 `Unimplemented`。

`PolicyService/CheckMessageAction` 也是内部策略判定面，不通过 api-gateway 暴露给客户端。

## 运行模式

第一版 runtime：

```text
NEXUSIM_API_GATEWAY_MODE=grpc
NEXUSIM_API_GATEWAY_GRPC_ADDR=0.0.0.0:12000
```

Auth audience：

```text
NEXUSIM_API_GATEWAY_AUTH_AUDIENCE=api-gateway
```

该配置只接受单个 audience。第一阶段 identity-service / demo runner 均可在签发 gateway token 时指定 `aud=api-gateway`；push-gateway 仍继续使用自身的 `push-gateway` audience。这样 online WebSocket token 和 api-gateway user-facing RPC token 不再默认复用同一个 audience。

First-stage rate limit：

```text
NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED=false
NEXUSIM_API_GATEWAY_RATE_LIMIT_BACKEND=local
NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS=100
NEXUSIM_API_GATEWAY_RATE_LIMIT_BURST=200
NEXUSIM_API_GATEWAY_RATE_LIMIT_MAX_KEYS=10000
```

该限流器默认关闭；启用后按 `gRPC method + token fingerprint` 限流，缺少 token 时退化到 `gRPC method + peer address`。它不记录 token 原文，也不向业务服务透出限流 key。被限流请求返回 `ResourceExhausted / rate limit exceeded`，并进入 api-gateway gRPC metrics。

`local` backend 是本进程 token bucket。需要跨实例共享入口预算时启用 Redis backend：

```text
NEXUSIM_API_GATEWAY_RATE_LIMIT_BACKEND=redis
NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_MODE=single
NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_ADDR=127.0.0.1:6379
NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_KEY_PREFIX=nexusim:api-gateway
NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_WINDOW=1s
NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_FAIL_OPEN=true
```

Redis backend 使用 fixed-window counter，把同一 token/method 在多个 api-gateway 实例上的请求合并计数。`FAIL_OPEN=true` 时 Redis 短故障只记录 `redis_error_count` 并放行请求；`FAIL_OPEN=false` 时返回 `Unavailable / rate limiter unavailable`。

这是第一阶段分布式入口保护，不是完整产品级配额系统：tenant quota、IP reputation、WAF、自适应风控、跨区域一致性、Redis 限流故障演练和统一告警仍是后续 production hardening。

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
```

启用下游服务 `metadata` / `verified-metadata` auth 时，不能只依赖 metadata 字段本身。后端必须只暴露在可信内网 / loopback，或者启用 gRPC mTLS 并把 client DNS / URI SAN allowlist 收敛到 `api-gateway.nexusim.local` / `spiffe://nexusim/api-gateway` 一类明确服务身份；否则客户端直连后端仍可伪造 metadata。

## 边界与后续

第一阶段不是完整 API gateway / BFF：

- 不做 HTTP / REST / GraphQL 转换。
- 不做 OIDC federation。
- 不做全服务 mTLS rollout 或证书生命周期治理。
- 不做限流、配额、WAF、OpenTelemetry 全链路 trace。
- 不替代 push-gateway 的 WebSocket online notify / ACK 转发职责。

后续优先级：

1. 继续让新客户端优先使用 `nexusim.gateway.v1.GatewayService`，再评估是否禁用 legacy service descriptor。
2. 继续把限流、配额、审计采样和 tracing 作为独立 production hardening，不塞进当前 proxy skeleton。

2026-06-13 补充：clean commit `cff1668` 已跑通 `loadtest/demo/run-local-secure-demo.ps1` 经真实 api-gateway 的 secure E2E smoke。demo runner 对 conversation / message / delivery / receipt 的 gRPC target 均指向 api-gateway，使用 HMAC gateway token 和 desktop-client mTLS；api-gateway 再通过 mTLS 调下游，并向下游注入 trusted metadata。报告见 `docs/runbook/loadtest/demo/loadtest-report-20260613-e2e-demo-api-gateway-secure-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-api-gateway-secure-smoke-20260613-clean`。

2026-06-13 补充：api-gateway 已补 first-stage `/healthz`、`/readyz`、`/debug/metrics` 和低敏 gRPC JSON access log；`run-local-secure-demo.ps1` 会启动 `NEXUSIM_API_GATEWAY_DEBUG_ADDR=127.0.0.1:11904` 并把 metrics 保存为 `api-gateway-debug-metrics.json`。

2026-06-13 补充：clean commit `9335bd1` 已把 api-gateway 默认 `NEXUSIM_API_GATEWAY_AUTH_AUDIENCE` 收紧为 `api-gateway`；secure demo runner 生成 `aud=api-gateway` 的 HMAC gateway token，并通过 `e2e-demo-api-gateway-audience-smoke-20260613-clean` 验证真实 api-gateway + 下游 mTLS 链路仍可完成 E2E 主流程。历史 `push-gateway` audience 仍可通过显式 env 配置兼容，但不再是 api-gateway 默认值。

2026-06-13 补充：api-gateway 已补第一阶段 gRPC rate limiter，默认关闭；`local` backend 为进程内 token bucket，`redis` backend 为跨实例 fixed-window counter，并在 `/debug/metrics` 输出低敏 `rate_limit` 聚合状态。该能力不是完整产品级 quota / WAF / 风控系统。

2026-06-13 补充：api-gateway 已新增第一阶段 `nexusim.gateway.v1.GatewayService` public facade proto，覆盖 conversation / message / delivery / receipt 的 user-facing RPC，并明确不包含服务间 `GetSendContext`。legacy service descriptor 暂时保留用于兼容；下一步是让 demo runner / 客户端切到 facade 后再收敛旧 descriptor。

2026-06-13 补充：clean commit `bb13300` 已跑通 `run-local-secure-demo.ps1` 的 `--gateway-facade` 真实进程 smoke。summary `git_dirty=false/success=true/gateway_facade=true/gateway_auth_mode=hmac/gateway_auth_audience=api-gateway`，api-gateway debug metrics 显示本轮 user-facing gRPC calls 均走 `/nexusim.gateway.v1.GatewayService/...`，报告见 `docs/runbook/loadtest/demo/loadtest-report-20260613-e2e-demo-api-gateway-facade-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-api-gateway-facade-smoke-20260613-clean`。
