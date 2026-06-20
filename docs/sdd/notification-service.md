# notification-service SDD v0.1 Draft

## 1. 服务定位

`notification-service` 是 NexusIM 的统一通知投递服务。它负责 email、SMS、
APNs / FCM、系统通知、模板版本、provider route、delivery attempt、retry /
DLQ、bounce / suppression 和低敏投递审计。

职责：

- 拥有 `notification_request`、`notification_template`、
  `notification_delivery_attempt`、`notification_suppression` 和
  `notification_outbox`。
- 给 identity、admin、control-plane、system jobs 和后续业务服务提供统一
  notification request API。
- 通过 provider adapter 投递 email / SMS / push notification；第一版可先接
  webhook / SMTP adapter。
- 记录 provider delivery 状态、稳定失败分类、retry / DLQ 和 bounce / suppress
  状态。
- 发布低敏 `im.notification.events`，供 audit、admin 和运营视图消费。

不负责：

- 不拥有 identity challenge、password reset token、MFA 或用户凭证事实。
- 不拥有 IM message、delivery inbox、ACK、push-gateway online session 或
  presence facts。
- 不直接决定用户隐私、租户 quota 或发送策略；这些来自 policy /
  control-plane 的公开端口。
- 不保存验证码、reset token、provider response body、provider secret 或完整用户
  PII 明文到事件、debug metrics 或 audit export。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游 | identity-service | verification / password reset challenge delivery request |
| 上游 | admin / control-plane / system jobs | 管理通知、系统通知、运营通知 |
| 同步依赖 | policy-service / control-plane | tenant channel policy、quota、template allowlist |
| 同步依赖 | provider adapters | webhook、SMTP、SMS、APNs、FCM |
| 异步下游 | audit-service / admin-service | delivery status、DLQ、bounce / suppression 事件 |
| 事实源 | PostgreSQL | request、template、attempt、suppression、outbox |

`notification-service` 不反向读取 identity-service、message-service 或
push-gateway 私有表。需要关联业务事实时，上游必须在 request 中携带稳定
`correlation_id`、`causation_id` 和低敏业务引用。

## 3. 六层 DDD 包结构

```text
services/notification-service/
  cmd/notification-service/
  internal/{api,app,domain,infrastructure,types,trigger}/
```

| 层 | 本服务内容 |
| --- | --- |
| `api` | gRPC adapter，verified metadata，稳定错误映射 |
| `app` | CreateNotificationRequest、GetNotificationStatus、CancelNotificationRequest |
| `domain` | request / attempt 状态机、template 选择、suppression 决策 |
| `infrastructure` | PostgreSQL repository、provider adapters、policy/control-plane clients |
| `types` | command、DTO、错误码、枚举、低敏 envelope |
| `trigger` | delivery worker、outbox relay、bounce worker、cleanup worker |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `NotificationRequest` | 一次业务通知请求 | tenant 内 idempotency；不含 provider raw body |
| `NotificationTemplate` | 模板 key / version / locale / channel | 发布后 append-only，废弃走 status |
| `DeliveryAttempt` | 单 provider 投递尝试 | bounded retry；失败分类稳定 |
| `Suppression` | bounce / unsubscribe / policy suppress | 只由低敏 reason 和 scope 驱动 |
| `NotificationOutboxEvent` | 通知事件 | 只通过 outbox relay 发布 |

Request 状态：

```text
ACCEPTED -> SCHEDULED -> SENDING -> DELIVERED
ACCEPTED/SCHEDULED/SENDING -> RETRY_WAIT
RETRY_WAIT -> SENDING
ACCEPTED/SCHEDULED/SENDING/RETRY_WAIT -> DLQ
ACCEPTED/SCHEDULED/RETRY_WAIT -> CANCELED
```

## 5. 同步 API 契约

```text
rpc CreateNotificationRequest(CreateNotificationRequestRequest) returns (CreateNotificationRequestResponse)
rpc GetNotificationStatus(GetNotificationStatusRequest) returns (GetNotificationStatusResponse)
rpc CancelNotificationRequest(CancelNotificationRequestRequest) returns (CancelNotificationRequestResponse)
```

`CreateNotificationRequest` 请求字段：

```text
tenant_id, requester_service, requester_user_id
channel: EMAIL | SMS | APNS | FCM | SYSTEM
recipient_ref, destination_ref
template_key, template_version, locale
priority, scheduled_at, expires_at
idempotency_key
template_variables_json
secret_payload_ciphertext, secret_payload_key_version, secret_payload_expires_at
correlation_id, causation_id, trace_id
```

安全约束：

- `template_variables_json` 只能保存低敏变量。
- challenge code、password reset token 等一次性 secret 只能进入
  `secret_payload_ciphertext`，并必须带 key version 和短 TTL。
- 第一版本地可用 `NEXUSIM_NOTIFICATION_SECRET_PAYLOAD_KEY` / keyring；生产级
  identity challenge 承接前必须切到 KMS/HSM 或等价密钥托管。
- provider adapter 只能在内存中解密 secret payload，不能写入日志、事件或 audit。

响应字段：

```text
request_id, status, next_attempt_at, accepted_at
```

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | channel、template、locale、payload 或 TTL 非法 | 否 |
| `PERMISSION_DENIED` | tenant policy / channel policy 拒绝 | 否 |
| `FAILED_PRECONDITION` | recipient suppressed、template 未发布或 provider 未配置 | 否 |
| `ALREADY_EXISTS` | idempotency key replay 命令冲突 | 否 |
| `NOT_FOUND` | request / template 不存在 | 否 |
| `UNAVAILABLE` | provider / policy / storage 暂不可用 | 是 |

## 6. 异步事件契约

| 事件 | Topic | 分区键 | 说明 |
| --- | --- | --- | --- |
| `notification.request.accepted.v1` | `im.notification.events` | `tenant_id:request_id` | request 已持久化 |
| `notification.delivery.succeeded.v1` | `im.notification.events` | `tenant_id:request_id` | provider 接受或确认成功 |
| `notification.delivery.failed.v1` | `im.notification.events` | `tenant_id:request_id` | 单次失败，可能重试 |
| `notification.delivery.dead_lettered.v1` | `im.notification.events` | `tenant_id:request_id` | 达到 retry 上限或不可恢复 |
| `notification.recipient.suppressed.v1` | `im.notification.events` | `tenant_id:recipient_ref` | bounce / unsubscribe / policy suppress |

Envelope 必须包含 `event_id`、`event_type`、`event_version`、`tenant_id`、
`request_id`、`partition_key`、`producer=notification-service`、`occurred_at`、
`trace_id`、`correlation_id`、`causation_id`。

事件 payload 禁止包含：

- raw challenge code、reset token、TOTP / recovery code；
- provider authorization、provider response body、SMTP transcript；
- destination 明文；需要展示时只使用 masked destination 或 recipient_ref；
- encrypted secret payload blob。

## 7. 数据库设计

第一版表：

```text
notification_requests
notification_templates
notification_delivery_attempts
notification_provider_routes
notification_suppressions
notification_outbox
```

关键字段：

```text
notification_requests:
tenant_id, request_id, idempotency_key, requester_service,
channel, recipient_ref, destination_hash, destination_masked,
template_key, template_version, locale, priority,
template_variables_json, secret_payload_ciphertext,
secret_payload_key_version, secret_payload_expires_at,
status, attempt_count, next_attempt_at, expires_at,
last_failure_class, last_public_error, created_at, delivered_at, dead_lettered_at

notification_delivery_attempts:
tenant_id, attempt_id, request_id, provider_id, provider_message_id_hash,
status, failure_class, public_error, started_at, finished_at, retry_after

notification_templates:
tenant_id, template_key, template_version, channel, locale,
status, subject_template, body_template_ref, checksum, created_at, deprecated_at

notification_suppressions:
tenant_id, suppression_id, channel, recipient_ref, destination_hash,
reason, source, starts_at, expires_at, created_at

notification_outbox:
event_id, tenant_id, request_id, event_type, event_version,
partition_key, payload_json, status, retry_count, next_retry_at, published_at
```

`destination_hash` 使用服务专用 HMAC secret；`destination_masked` 只保存可展示的
低敏掩码，例如 `a***@example.com` 或 `+86******1234`。

## 8. 核心流程

创建通知：

```text
CreateNotificationRequest
-> verify trusted metadata
-> validate channel/template/TTL/idempotency
-> check tenant channel policy and suppression
-> insert notification_requests(ACCEPTED)
-> write notification.request.accepted.v1 outbox
```

投递 worker：

```text
delivery-worker
-> lock ready requests FOR UPDATE SKIP LOCKED
-> recheck expiry / suppression / provider route
-> decrypt optional secret payload in memory
-> render template
-> call provider adapter
-> write delivery attempt
-> mark DELIVERED / RETRY_WAIT / DLQ
-> write notification delivery outbox
```

identity challenge 承接：

```text
identity-service remains challenge/token fact owner
-> notification-service handles provider delivery attempts only
-> raw token may only be passed as encrypted short-lived secret payload
-> delivery result does not confirm identity challenge consumption
```

## 9. 一致性和事务

强一致边界：

- request、delivery attempt、status transition 和 notification outbox 在同一
  PostgreSQL 事务内更新。
- idempotency replay 不创建重复 request 或重复 accepted event。
- DLQ transition 写 outbox 同事务，不能只更新 attempt。

最终一致边界：

- provider 成功但 DB commit 失败可能造成 at-least-once provider send；provider
  adapter 必须携带幂等 provider key。
- provider bounce / delivery receipt 通过 bounce worker 最终写 suppression。
- audit-service 通过 `im.notification.events` 最终归档低敏状态。

## 10. 幂等、重试和补偿

| 场景 | 幂等键 | 重试策略 | 补偿 |
| --- | --- | --- | --- |
| CreateNotificationRequest | tenant + requester + idempotency_key | replay 返回原 request | command hash 冲突 fail closed |
| DeliveryAttempt | request_id + attempt number | bounded retry + retry-after | DLQ / operator redrive |
| Provider send | provider idempotency key | provider-specific | duplicate delivery audit |
| Bounce handling | provider_message_id_hash | idempotent upsert suppression | suppression cleanup |
| OutboxRelay | event_id | bounded retry + DLQ | outbox repair operator |

## 11. 权限和安全

- API 必须使用 gateway verified metadata；request body 不能覆盖 trusted tenant /
  requester。
- 业务服务只能通过公开 API 或 outbox adapter 创建 request，不能直接写 provider 表。
- provider credentials 只来自 secret manager / env，不进入 debug endpoint。
- provider body、SMTP transcript、SMS body、raw push token 和 raw destination 不写入
  public response、Kafka payload、metrics、audit export 或 repair summary。
- template 渲染必须限制变量集合；subject 禁止 CR/LF 注入。
- `secret_payload_ciphertext` 有短 TTL；过期后 request 只能 DLQ / cancel，不能继续投递。
- tenant policy / suppression 命中返回稳定 public error，不泄漏具体黑名单原因。

## 12. SLO 和指标

第一阶段不写生产 SLO，只保留本地 / 面试展示指标：

```text
notification_request_total{channel,status}
notification_delivery_attempt_total{channel,provider,status,failure_class}
notification_outbox_total{status}
notification_suppression_total{channel,reason}
notification_secret_payload_expired_total{channel}
```

debug / metrics 禁止输出 tenant_id、user_id、destination、request_id、provider URL、
provider body、template variables 或 secret payload。

## 13. 测试方案

| 测试 | 目标 |
| --- | --- |
| domain unit | request / attempt 状态机、template validation、suppression decision |
| app unit | idempotency、policy deny、secret payload TTL、provider unavailable |
| PostgreSQL integration | request + attempt + outbox 同事务、replay、DLQ |
| provider fake | webhook / SMTP 成功、超时、非 2xx、retry-after |
| event builder | malformed payload fail closed，不发布敏感字段 |
| worker test | retry / DLQ / expired secret payload / suppression recheck |
| smoke | create request -> fake provider delivered -> status -> Kafka event |

## 14. Runbook

运行模式：

```text
NEXUSIM_NOTIFICATION_SERVICE_MODE=grpc
NEXUSIM_NOTIFICATION_SERVICE_MODE=delivery-worker
NEXUSIM_NOTIFICATION_SERVICE_MODE=outbox-relay
NEXUSIM_NOTIFICATION_SERVICE_MODE=bounce-worker
NEXUSIM_NOTIFICATION_SERVICE_MODE=cleanup
```

provider 配置第一版：

```text
NEXUSIM_NOTIFICATION_PROVIDER_MODE=noop|fake|webhook
NEXUSIM_NOTIFICATION_WEBHOOK_URL=...
NEXUSIM_NOTIFICATION_WEBHOOK_BEARER_TOKEN=...
NEXUSIM_NOTIFICATION_WEBHOOK_TIMEOUT=5s
NEXUSIM_NOTIFICATION_SECRET_PAYLOAD_KEY=...
NEXUSIM_NOTIFICATION_SECRET_PAYLOAD_KEYRING_JSON=...
```

`noop` / `fake` 只用于本地 smoke 和 provider boundary 验证。`webhook` 是第一版
真实 HTTP provider 边界，只发送低敏 notification envelope / template variables，
不发送 `destination_hash`、secret payload、provider credential 或 provider response
body。真实 SMTP / SMS / APNs / FCM adapter 需要单独切片接入。

operator：

```text
notification-request-audit
notification-delivery-redrive
notification-delivery-cancel
notification-template-audit
notification-suppression-cleanup
notification-outbox-repair
```

## 15. 验收标准

进入编码前：

- 本 SDD v0.1 draft 被复核，无 P0/P1。
- `notification-service` brief 指向本 SDD。
- 明确 identity challenge delivery 当前仍由 identity-service 局部能力承载，切换到
  notification-service 需要单独 adapter / migration plan。

进入 first smoke 前：

- proto / migration / 六层 skeleton / cmd runtime 已落。
- PostgreSQL repository、fake provider、event builder 和 worker retry 测试通过。
- `CreateNotificationRequest -> delivery-worker -> fake provider -> GetNotificationStatus`
  本地 smoke 通过。
- secret payload、provider body、destination 明文不会出现在事件、metrics、audit 或
  repair summary。
