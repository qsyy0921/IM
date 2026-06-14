# NexusIM conversation-service SDD v0.1

本文冻结 `conversation-service` 第一条可编码切片：为 `message-service SendMessage` 提供真实会话发送上下文读取，替换 strict conversation mock。

## 1. 第一阶段目标

```text
message-service
-> conversation-service GetSendContext
-> PostgreSQL read model
-> return member_version / permission_version / mode / fanout policy
```

第一阶段只做 read path，不做完整成员变更 Saga 执行。

## 2. 服务职责

`conversation-service` 拥有：

- 会话事实：`conversations`。
- 成员事实：`conversation_members`。
- 成员版本：`member_version`。
- 权限版本：`permission_version`。
- 会话模式：`LOCAL_ROW_LOCK / SEQUENCER_BLOCK`。
- fanout 策略：`fanout_mode / fanout_policy_version`。
- 成员变更 Saga 表结构。

禁止事项：

- 不写消息正文。
- 不分配普通消息 seq。
- 不直接发布 message timeline Kafka 事件。
- 不绕过 `message-service` 修改 `message_log`。

## 3. 六层 DDD

```text
services/conversation-service/
  cmd/conversation-service
  internal/api
  internal/app
  internal/domain
  internal/infrastructure
  internal/types
  internal/trigger
```

| 层 | 职责 |
| --- | --- |
| `api` | gRPC handler，request/response 转换，错误码映射 |
| `app` | `GetSendContextUseCase`，编排 repository port |
| `domain` | 会话状态、成员状态、发送上下文不变量 |
| `infrastructure` | PostgreSQL repository |
| `types` | Command、DTO、枚举、错误 sentinel |
| `trigger` | 后续 member_change_saga worker，占位 |

## 4. gRPC 契约

契约文件：

```text
api/proto/nexusim/conversation/v1/conversation_service.proto
```

第一阶段 RPC：

```text
GetSendContext(tenant_id, conversation_id, user_id, trace_id)
-> member_version
-> permission_version
-> conversation_mode
-> fanout_mode
-> fanout_policy_version
-> current_seq_shard
```

错误语义：

| 条件 | gRPC code | 说明 |
| --- | --- | --- |
| 参数缺失 | `InvalidArgument` | 不进入 repository |
| 会话不存在 / 非 ACTIVE | `NotFound` | message-service 映射为 `CONVERSATION_NOT_FOUND` |
| 成员不存在 / 非 ACTIVE | `PermissionDenied` | message-service 映射为 `PERMISSION_DENIED` |
| PostgreSQL 读取失败 | `Unavailable` | 可重试依赖错误 |

## 5. PostgreSQL 表

Migration：

```text
migrations/postgres/conversation/000001_conversation_core.sql
```

核心表：

- `conversations`
- `conversation_members`
- `member_change_saga`

第一阶段只读取 `conversations + conversation_members`。

## 6. 与 message-service 的集成方式

`message-service` 保持 `ConversationQueryPort` 不变。

运行时策略：

- 未配置 `NEXUSIM_CONVERSATION_SERVICE_ADDR`：继续使用 strict mock，便于本地开发和历史压测复现。
- 配置 `NEXUSIM_CONVERSATION_SERVICE_ADDR`：通过 gRPC 调用真实 `conversation-service`。

这样可以逐步替换 mock，同时不破坏已有 message-service smoke 和压测入口。

第一阶段 transport hardening 保持可选静态配置，默认 plaintext：

```text
NEXUSIM_CONVERSATION_GRPC_TLS_CERT_FILE=
NEXUSIM_CONVERSATION_GRPC_TLS_KEY_FILE=
NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_CA_FILE=
NEXUSIM_CONVERSATION_GRPC_TLS_REQUIRE_CLIENT_CERT=false
NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES=
NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_ALLOWED_URIS=

NEXUSIM_CONVERSATION_SERVICE_TLS_CA_FILE=
NEXUSIM_CONVERSATION_SERVICE_TLS_SERVER_NAME=
NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_CERT_FILE=
NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_KEY_FILE=
```

`conversation-service` server TLS requires cert/key together and uses TLS 1.2 or newer. Supplying a client CA, requiring client certs or configuring client DNS / URI SAN allowlists enables mTLS. message-service only enables TLS for the conversation RPC when `NEXUSIM_CONVERSATION_SERVICE_TLS_CA_FILE` is configured, and client cert/key must be configured together. Partial TLS configuration fails startup rather than silently falling back to plaintext.

This is not a full service-mesh identity layer. Certificate issuance, rotation, dynamic service identity registry and all-service mTLS rollout remain future hardening.

Gateway verified identity hardening is opt-in for user-facing conversation RPCs:

```text
NEXUSIM_CONVERSATION_AUTH_MODE=body              # default, legacy request AuthContext
NEXUSIM_CONVERSATION_AUTH_MODE=metadata          # read tenant/user/device/session from verified gRPC metadata
NEXUSIM_CONVERSATION_AUTH_MODE=verified-metadata # alias of metadata
```

In `metadata` / `verified-metadata` mode, `CreateMemberChange`, `GetMemberChange`, `TransferConversationOwner`, and `ListConversationMembers` ignore caller-supplied `AuthContext.tenant_id/user_id/device_id/session_id` and use gateway-injected metadata keys instead. `trace_id/request_id` may still fall back to the request body for observability. `GetSendContext` remains the message-service service-to-service read path and keeps its request contract. When `NEXUSIM_CONVERSATION_AUTH_MODE=metadata|verified-metadata`, a non-loopback / non-RFC1918 gRPC listen address without mTLS client-certificate verification must fail startup; first-stage trusted metadata is only allowed on private listeners unless transport auth is enabled. This is not a full API gateway or centralized identity-governance implementation.

第一阶段本地运维观测保持低敏：

```text
NEXUSIM_CONVERSATION_DEBUG_ADDR=
```

配置后暴露：

- `/healthz`
- `/readyz`
- `/debug/metrics`

`/debug/metrics` 只返回低敏聚合快照：gRPC 请求统计、PostgreSQL pool、`conversations` / `conversation_members` / `member_change_saga` 的总量和状态分布，不返回成员标识、会话标题、target user 明细或 raw error 文本。

## 7. 本阶段验收

- `conversation_service.proto` 已生成 Go 代码。
- `conversation-service` 具备六层目录和 `cmd/conversation-service`。
- `GetSendContext` gRPC handler 有单元测试。
- PostgreSQL repository 有可选集成测试。
- `conversation-service` 已有 `/healthz`、`/readyz`、`/debug/metrics` 和 gRPC metrics。
- `message-service` 可以通过 gRPC client 替换 strict conversation mock。
- `go test ./...` 通过。

## 8. 后续范围

下一阶段再实现：

- 创建会话。
- 添加 / 移除成员。
- 角色变更。
- `member_change_saga` 状态机。
- 成员边界 timeline event。
- ACL 投影与 strict ACL fallback。
