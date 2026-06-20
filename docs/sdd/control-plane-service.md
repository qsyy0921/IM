# control-plane-service SDD v0.1 Draft

## 1. 服务定位

`control-plane-service` 是 NexusIM 的多租户运行控制面。它负责功能开关、灰度、
限流 / quota 配置、动态策略发布引用、配置版本、发布审批引用、回滚和
applied-version ACK。

职责：

- 拥有 `control_config_bundle`、`control_config_version`、
  `control_rollout_rule`、`control_applied_ack` 和 `control_outbox`。
- 生成版本化、带 checksum 的配置 snapshot，供 api-gateway、policy-service、
  notification-service、model-gateway 等服务拉取或消费。
- 记录配置发布、回滚、实例 applied ACK 和低敏发布审计。
- 提供配置有效期、灰度规则、environment / ring / tenant scope 和回滚语义。
- 为后续 admin-service / workflow-service 提供受控发布 API。

不负责：

- 不替代 policy-service 的授权 / ReBAC / moderation / tool policy 决策。
- 不替代 api-gateway 的 runtime rate limiter 或请求鉴权。
- 不替代各服务启动安全门禁、mTLS / trusted metadata、secret manager 或 KMS。
- 不保存 provider secret、private key、token、password、raw prompt 或业务 payload。
- 不直接写其它服务私有表；服务通过公开 API、snapshot 或事件应用配置。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游 | admin-service / workflow-service / operator | 创建、审批引用、发布、回滚配置 |
| 下游 pull | api-gateway | tenant quota / rate-limit snapshot |
| 下游 pull | policy-service | tenant policy DSL / quota / moderation rule references |
| 下游 pull | notification-service | tenant channel policy / template allowlist |
| 下游 pull | model-gateway | provider policy、成本上限、fallback 策略 |
| 异步下游 | audit-service | publish / rollback / applied ACK audit |
| 事实源 | PostgreSQL | bundle、version、rollout、ack、outbox |

第一版可以先支持 pull API；watch / Kafka push 是优化，不得绕过服务本地校验。

## 3. 六层 DDD 包结构

```text
services/control-plane-service/
  cmd/control-plane-service/
  internal/{api,app,domain,infrastructure,types,trigger}/
```

| 层 | 本服务内容 |
| --- | --- |
| `api` | gRPC adapter，verified metadata，稳定错误映射 |
| `app` | PublishConfigVersion、GetConfigSnapshot、AckAppliedConfigVersion、RollbackConfigVersion |
| `domain` | config schema、checksum、rollout rule、version transition |
| `infrastructure` | PostgreSQL repository、audit / workflow clients、optional object storage |
| `types` | command、DTO、错误码、枚举、snapshot envelope |
| `trigger` | outbox relay、expiry / cleanup worker、applied-ack monitor |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `ConfigBundle` | 同一配置族，例如 api-gateway quota | tenant / environment / kind 唯一 |
| `ConfigVersion` | append-only 配置版本 | immutable；checksum 稳定；可回滚指向旧版本 |
| `RolloutRule` | 灰度 / ring / percentage / allowlist 规则 | 不包含用户 PII 明文 |
| `AppliedAck` | 服务实例已应用版本回执 | instance_ref 低敏；不能伪造服务名 |
| `ControlOutboxEvent` | 控制面事件 | 只通过 outbox relay 发布 |

Config kind 第一版：

```text
API_GATEWAY_TENANT_QUOTA
FEATURE_FLAG
POLICY_RULESET_REF
POLICY_TENANT_QUOTA
NOTIFICATION_CHANNEL_POLICY
MODEL_PROVIDER_POLICY
MEDIA_POLICY
```

Version 状态：

```text
DRAFT -> PUBLISHED -> ACTIVE
DRAFT -> CANCELED
PUBLISHED/ACTIVE -> ROLLED_BACK
PUBLISHED/ACTIVE -> EXPIRED
```

`DRAFT` 只能用于本服务内部或 workflow 审批等待；下游服务只能拉取
`PUBLISHED` / `ACTIVE` 且在 effective window 内的版本。

## 5. 同步 API 契约

```text
rpc PublishConfigVersion(PublishConfigVersionRequest) returns (PublishConfigVersionResponse)
rpc GetConfigSnapshot(GetConfigSnapshotRequest) returns (GetConfigSnapshotResponse)
rpc AckAppliedConfigVersion(AckAppliedConfigVersionRequest) returns (AckAppliedConfigVersionResponse)
rpc RollbackConfigVersion(RollbackConfigVersionRequest) returns (RollbackConfigVersionResponse)
rpc QueryAppliedConfigVersions(QueryAppliedConfigVersionsRequest) returns (QueryAppliedConfigVersionsResponse)
```

`PublishConfigVersion` 请求字段：

```text
tenant_id, environment, config_kind, bundle_key
version, effective_at, expires_at
schema_version, payload_json, payload_checksum
rollout_rules
approval_ref, operator_ref, reason_ref
idempotency_key
correlation_id, causation_id, trace_id
```

`GetConfigSnapshot` 请求字段：

```text
tenant_id, environment, service_name, config_kind, bundle_key
current_version, instance_ref, ring, service_version
```

响应字段：

```text
version, schema_version, generated_at_unix_ms, effective_at, expires_at,
checksum, payload_json, rollout_decision, min_client_version, previous_version
```

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | config kind、schema、checksum、rollout rule 非法 | 否 |
| `PERMISSION_DENIED` | operator / service 无发布或读取权限 | 否 |
| `ALREADY_EXISTS` | version / idempotency 冲突 | 否 |
| `NOT_FOUND` | bundle / version 不存在 | 否 |
| `FAILED_PRECONDITION` | approval 缺失、版本过期、rollback target 不合法 | 否 |
| `UNAVAILABLE` | storage / audit / policy 暂不可用 | 是 |

## 6. Snapshot 兼容契约

api-gateway tenant quota 第一版必须兼容已有 versioned quota snapshot：

```json
{
  "version": "quota-v1.20260614",
  "generated_at_unix_ms": 1800000000000,
  "checksum": "sha256:<plans-json-sha256>",
  "plans": {
    "tenant-free": {"requests_per_second": 20, "burst": 40}
  }
}
```

`control-plane-service` 可以成为该 snapshot 的 authoring / publishing source，
但 api-gateway 仍负责：

- 验证 snapshot version / checksum / max age；
- 执行 local / Redis rate limiter；
- 暴露低敏 runtime metrics；
- 在 reload 失败时保留上一版有效配置。

`control-plane-service` 不参与请求热路径，不做每次请求的 quota 判定。

## 7. 异步事件契约

| 事件 | Topic | 分区键 | 说明 |
| --- | --- | --- | --- |
| `control.config.published.v1` | `im.control.events` | `tenant_id:bundle_key` | 新版本已发布 |
| `control.config.rolled_back.v1` | `im.control.events` | `tenant_id:bundle_key` | 已回滚到旧版本 |
| `control.config.expired.v1` | `im.control.events` | `tenant_id:bundle_key` | 版本过期 |
| `control.config.applied.v1` | `im.control.events` | `tenant_id:service_name` | 服务实例 ACK |

事件 payload 只包含 low-sensitive fields：config kind、bundle key、version、
environment、schema_version、checksum-present、rollout summary、operator ref、
approval ref、trace/correlation refs。禁止输出 payload_json 原文、tenant plan 明细、
secret ref 实值、operator reason 原文或 DSN / endpoint secret。

## 8. 数据库设计

第一版表：

```text
control_config_bundles
control_config_versions
control_rollout_rules
control_applied_acks
control_outbox
```

关键字段：

```text
control_config_bundles:
tenant_id, environment, config_kind, bundle_key, status,
created_at, updated_at

control_config_versions:
tenant_id, environment, config_kind, bundle_key, version,
schema_version, payload_json, payload_checksum, status,
effective_at, expires_at, published_at, rolled_back_at,
approval_ref, operator_ref, reason_ref, idempotency_key

control_rollout_rules:
tenant_id, environment, config_kind, bundle_key, version,
rule_id, ring, percentage, tenant_allowlist_hash,
service_version_constraint, starts_at, ends_at

control_applied_acks:
tenant_id, environment, service_name, instance_ref,
config_kind, bundle_key, version, applied_at,
service_version, status, last_error_class

control_outbox:
event_id, tenant_id, aggregate_type, aggregate_id,
event_type, event_version, partition_key, payload_json,
status, retry_count, next_retry_at, published_at
```

`payload_json` 不允许保存 secret value。需要 secret 时只保存 `secret_ref`，由对应
服务在自身 secret manager 中解析。

## 9. 核心流程

发布配置：

```text
PublishConfigVersion
-> verify operator / approval ref
-> validate schema and low-sensitive payload
-> compute checksum
-> insert immutable config version
-> write control.config.published.v1 outbox
```

服务拉取：

```text
GetConfigSnapshot
-> verify service identity
-> select current version by tenant/environment/kind/bundle/ring
-> return versioned snapshot + checksum
-> service validates and applies locally
```

应用 ACK：

```text
AckAppliedConfigVersion
-> verify service identity
-> upsert control_applied_acks
-> write control.config.applied.v1 outbox
```

回滚：

```text
RollbackConfigVersion
-> verify operator / approval ref
-> ensure target version is immutable and compatible
-> mark current version ROLLED_BACK
-> publish target version as ACTIVE pointer
-> write control.config.rolled_back.v1 outbox
```

## 10. 一致性和事务

强一致边界：

- bundle/version/rollout 写入和 outbox 同 PostgreSQL 事务。
- applied ACK upsert 和 outbox 同事务。
- rollback 状态更新和 rollback outbox 同事务。

最终一致边界：

- 服务实例拉取配置后本地应用；ACK 可能延迟或丢失。
- 配置事件只通知“有新版本”，服务仍必须通过 GetConfigSnapshot 验证 checksum。
- audit-service 通过 `im.control.events` 最终归档，不阻塞发布事务。

## 11. 幂等、回滚和补偿

| 场景 | 幂等键 | 重试策略 | 补偿 |
| --- | --- | --- | --- |
| PublishConfigVersion | tenant + bundle + idempotency_key | replay 返回原 version | command hash 冲突 fail closed |
| GetConfigSnapshot | 无状态查询 | 服务侧 bounded retry | 保留上一版有效配置 |
| AckAppliedConfigVersion | service + instance + bundle + version | idempotent upsert | monitor stale ACK |
| RollbackConfigVersion | rollback request id | replay 返回 rollback target | 再次发布新版本 |
| OutboxRelay | event_id | bounded retry + DLQ | outbox repair operator |

## 12. 权限和安全

- Publish / rollback 只能由 admin-service、workflow-service 或受控 operator 调用。
- Get snapshot 只能由服务身份调用；客户端不能直接读取 control-plane。
- 动态配置不能降低服务启动安全门禁，例如 debug public allow、mTLS 关闭、mock auth。
- 配置 payload 必须通过 per-kind schema allowlist；未知字段 fail closed。
- rollout allowlist 保存 hash，不保存用户 / 设备 / IP 明文列表。
- approval reason 原文放在审批系统或文件证据中；control-plane 只保存 reason_ref。
- snapshot 输出不包含 secrets、DSN、provider token、private key 或 webhook URL query。

## 13. Applied ACK 和漂移检测

`control-plane-service` 维护低敏 applied ACK：

```text
tenant, environment, service_name, instance_ref, config_kind, bundle_key,
expected_version, applied_version, applied_at, service_version, status
```

漂移状态：

```text
IN_SYNC
STALE_VERSION
MISSING_ACK
APPLY_FAILED
UNKNOWN_INSTANCE
```

漂移检测只用于观测和发布门禁，不强制下游服务热更新。需要强制停止旧版本实例时，
后续通过 admin / workflow / deployment system 实现。

## 14. SLO 和指标

第一阶段不写生产 SLO，只保留本地 / 面试展示指标：

```text
control_config_version_total{kind,status,environment}
control_applied_ack_total{service,kind,status}
control_config_drift_total{service,kind,drift_status}
control_outbox_total{status}
control_snapshot_request_total{service,kind,code}
```

debug / metrics 禁止输出 tenant_id、instance_ref、operator_ref、approval_ref、
payload_json、checksum 原文、reason_ref 原文、secret_ref 原文或 URL。

## 15. 测试方案

| 测试 | 目标 |
| --- | --- |
| domain unit | schema validation、checksum、rollout rule、version transition |
| app unit | publish idempotency、permission deny、rollback、ACK drift |
| PostgreSQL integration | immutable version、outbox 同事务、ACK upsert |
| snapshot contract | api-gateway quota snapshot 兼容、checksum fail closed |
| event builder | 不输出 payload_json / secrets / tenant plan 明细 |
| smoke | Publish quota -> Get snapshot -> Ack applied -> Query drift |

## 16. Runbook

运行模式：

```text
NEXUSIM_CONTROL_PLANE_SERVICE_MODE=grpc
NEXUSIM_CONTROL_PLANE_SERVICE_MODE=outbox-relay
NEXUSIM_CONTROL_PLANE_SERVICE_MODE=ack-monitor
NEXUSIM_CONTROL_PLANE_SERVICE_MODE=expiry-worker
NEXUSIM_CONTROL_PLANE_SERVICE_MODE=cleanup
```

operator：

```text
control-config-audit
control-config-publish
control-config-rollback
control-applied-audit
control-outbox-repair
```

## 17. 验收标准

进入编码前：

- 本 SDD v0.1 draft 被复核，无 P0/P1。
- `control-plane-service` brief 指向本 SDD。
- 明确第一版只是版本化配置 / quota snapshot / ACK 控制面，不替代执行服务逻辑。

进入 first smoke 前：

- proto / migration / 六层 skeleton / cmd runtime 已落。
- PostgreSQL repository、snapshot checksum、outbox 和 ACK tests 通过。
- `PublishConfigVersion -> GetConfigSnapshot -> AckAppliedConfigVersion` 本地 smoke 通过。
- api-gateway tenant quota snapshot contract 仍通过已有 quota snapshot gate。
- secret、DSN、provider token、payload_json 原文不会出现在事件、metrics、audit 或
  repair summary。
