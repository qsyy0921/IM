# api-gateway SDD v0.1

## 定位

`api-gateway` 是 NexusIM 第一版统一 user-facing gRPC 入口。它负责把客户端提交的 gateway token 验证为可信身份，再把该身份转换为下游服务已经支持的 gateway verified gRPC metadata。

它不拥有业务事实，不写 PostgreSQL，不消费或发布 Kafka，也不直接访问任何微服务内部表。

## 第一阶段范围

第一阶段只代理已经存在的公开 gRPC proto service：

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

为了复用现有 proto，gateway 直接注册这些 service 的 server interface，并调用对应下游 service client。

## 鉴权与身份传播

入口请求必须携带 gateway token：

- `authorization: Bearer <token>`
- 或 first-stage 本地兼容 metadata `x-nexusim-gateway-token`

第一阶段复用共享 `internal/gatewayauth`：

- HMAC legacy token / HS256 JWT。
- RS256 JWT + JWKS / issuer allowlist。
- 默认 audience 仍是 `push-gateway`，用于兼容当前 identity-service 已签发的 gateway token；后续 identity-service 支持明确 `api-gateway` audience 后再收紧。

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

`ConversationService/GetSendContext` 是 message-service 的服务间 read path，不作为 user-facing API 暴露。api-gateway 对该方法返回 `Unimplemented`。

`PolicyService/CheckMessageAction` 也是内部策略判定面，不通过 api-gateway 暴露给客户端。

## 运行模式

第一版 runtime：

```text
NEXUSIM_API_GATEWAY_MODE=grpc
NEXUSIM_API_GATEWAY_GRPC_ADDR=0.0.0.0:12000
```

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

1. 让 secure demo 的 conversation / message / delivery / receipt gRPC target 指向 api-gateway，证明真实 token -> trusted metadata -> 下游 mTLS metadata auth 链路。
2. 增加低敏 gRPC metrics 和结构化 access log。
3. 在 identity-service 支持 `api-gateway` audience 后收紧默认 audience。
4. 后续拆 public facade proto，把 `GetSendContext` 从 user-facing service descriptor 中彻底移出。
