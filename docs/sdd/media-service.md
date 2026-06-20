# media-service SDD v0.1 Draft

## 1. 服务定位

`media-service` 是 NexusIM 的媒体资产事实服务。它负责图片、语音、视频和文件的上传会话、对象存储 metadata、下载授权、缩略图、病毒扫描、语音转码、时长 / hash 探测和媒体删除证明。

职责：

- 拥有 `media_asset`、`media_upload_session`、`media_processing_job` 和 `media_outbox`。
- 通过 S3-compatible object storage port 管理二进制对象；PostgreSQL 只保存 metadata 和 object key。
- 给 message-service 提供稳定 `asset_id`，message-service 只保存附件引用和低敏 metadata。
- 下载前做 tenant / conversation / user / message visibility 校验，不能只凭 object key。
- 对 scan / thumbnail / transcode / delete 产生可审计状态和事件。

不负责：

- 不保存消息正文，不决定消息是否可发送。
- 不替代 message-service 的 message fact、timeline、edit/revoke/delete 语义。
- 不替代 conversation-service / policy-service 的成员和权限事实。
- 不直接暴露对象存储 key，不让客户端绕过授权下载。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游 | api-gateway / 客户端 | 创建上传会话、完成上传、获取下载 URL |
| 同步依赖 | identity / policy / conversation | verified metadata、下载 / 绑定消息前的可见性校验 |
| 同步依赖 | S3-compatible object storage | PUT/HEAD/GET/presign/delete object |
| 同步依赖 | scan / thumbnail / transcode adapters | 第一版可用 mock adapter，后续替换为真实 provider |
| 异步下游 | message-service / search / audit | media ready / deleted / processing failed 事件 |
| 事实源 | PostgreSQL | 媒体 metadata、上传会话、处理任务、outbox |

## 3. 六层 DDD 包结构

```text
services/media-service/
  cmd/media-service/
  internal/{api,app,domain,infrastructure,types,trigger}/
```

| 层 | 本服务内容 |
| --- | --- |
| `api` | gRPC adapter，稳定错误映射，verified metadata 解析 |
| `app` | CreateUploadSession、CompleteUpload、GetMediaDownloadURL、DeleteMediaAsset |
| `domain` | asset/session/job 状态机、hash/size/content-type 校验、visibility gate |
| `infrastructure` | PostgreSQL repository、object storage adapter、scan/thumbnail/transcode/mock adapters、RPC clients |
| `types` | command、DTO、错误码、枚举 |
| `trigger` | processing worker、outbox relay、cleanup worker |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `MediaAsset` | 媒体资产 metadata 和状态 | tenant 内唯一；object_key 不出现在 public response |
| `UploadSession` | 有过期时间的一次上传授权 | 只能绑定一个 asset；过期后不能 complete |
| `ProcessingJob` | scan / thumbnail / transcode 任务 | 只处理已上传对象；失败进入 retry / DLQ |
| `MediaAccessGrant` | 临时下载授权 | 必须由可见性校验派生；短 TTL |
| `MediaOutboxEvent` | 媒体事件 | 只通过 outbox relay 发布 |

Asset 状态：

```text
UPLOAD_PENDING -> UPLOADED -> PROCESSING -> READY
UPLOAD_PENDING -> EXPIRED
UPLOADED/PROCESSING -> QUARANTINED
UPLOADED/PROCESSING -> FAILED
READY -> DELETED
```

## 5. 同步 API 契约

```text
rpc CreateUploadSession(CreateUploadSessionRequest) returns (CreateUploadSessionResponse)
rpc CompleteUpload(CompleteUploadRequest) returns (CompleteUploadResponse)
rpc GetMediaAsset(GetMediaAssetRequest) returns (GetMediaAssetResponse)
rpc GetMediaDownloadURL(GetMediaDownloadURLRequest) returns (GetMediaDownloadURLResponse)
rpc DeleteMediaAsset(DeleteMediaAssetRequest) returns (DeleteMediaAssetResponse)
```

`CreateUploadSession` 请求字段：

```text
tenant_id, user_id, device_id
conversation_id
media_kind: IMAGE | FILE | VOICE | VIDEO
file_name, content_type, size_bytes, sha256
idempotency_key
```

响应字段：

```text
asset_id, upload_session_id
upload_url, required_headers, expires_at
max_size_bytes, accepted_content_types
```

`CompleteUpload` 请求字段：

```text
tenant_id, user_id, device_id
asset_id, upload_session_id
sha256, size_bytes
```

响应字段：

```text
asset_id, status, scan_status, thumbnail_status, transcode_status
message_attachment_ref
```

`GetMediaDownloadURL` 请求字段：

```text
tenant_id, user_id, device_id
asset_id
conversation_id
message_id
requested_variant: ORIGINAL | THUMBNAIL | TRANSCODED
```

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | content type、size、hash、variant 或字段非法 | 否 |
| `PERMISSION_DENIED` | 不可见、非成员、策略拒绝或 tenant mismatch | 否 |
| `FAILED_PRECONDITION` | asset 未上传、未扫描通过、已隔离或已删除 | 否 |
| `NOT_FOUND` | asset / upload session 不存在 | 否 |
| `ALREADY_EXISTS` | idempotency key replay 命令冲突 | 否 |
| `UNAVAILABLE` | object storage / scanner / transcode provider 暂不可用 | 是 |

## 6. 异步事件契约

| 事件 | Topic | 分区键 | 下游 |
| --- | --- | --- | --- |
| `media.asset.uploaded.v1` | `im.media.events` | `tenant_id:asset_id` | audit / processing |
| `media.asset.ready.v1` | `im.media.events` | `tenant_id:asset_id` | message / search / audit |
| `media.asset.quarantined.v1` | `im.media.events` | `tenant_id:asset_id` | message / audit |
| `media.asset.deleted.v1` | `im.media.events` | `tenant_id:asset_id` | search / memory / audit |

Envelope 必须包含 `event_id`、`event_type`、`event_version`、`tenant_id`、`asset_id`、`partition_key`、`producer=media-service`、`occurred_at`、`trace_id`、`correlation_id`、`causation_id`。payload 不包含 object_key、download URL、raw file name 以外的敏感正文，也不包含 scanner 原始错误 body。

## 7. 数据库设计

第一版表：

```text
media_assets
media_upload_sessions
media_processing_jobs
media_outbox
media_access_audit
```

关键字段：

```text
media_assets:
tenant_id, asset_id, owner_user_id, conversation_id, media_kind,
content_type, file_name, size_bytes, sha256, object_key,
status, scan_status, thumbnail_status, transcode_status,
created_at, uploaded_at, ready_at, deleted_at

media_upload_sessions:
tenant_id, upload_session_id, asset_id, idempotency_key,
status, expires_at, completed_at

media_processing_jobs:
tenant_id, job_id, asset_id, job_type, status,
attempt_count, next_retry_at, last_error, dead_lettered_at

media_outbox:
event_id, tenant_id, asset_id, event_type, event_version,
partition_key, payload_json, status, retry_count, next_retry_at, published_at,
last_error
```

`object_key` 是服务内部字段，禁止在 public API response、Kafka payload、debug metrics 或 audit export 中直接输出。

## 8. 核心流程

上传：

```text
CreateUploadSession
-> validate metadata / quota / content type
-> allocate asset_id + object_key
-> insert media_assets(UPLOAD_PENDING) + media_upload_sessions(PENDING)
-> return short-lived upload_url
```

完成上传：

```text
CompleteUpload
-> lock upload_session + asset
-> HEAD object storage, verify size / sha256
-> mark asset UPLOADED
-> enqueue scan / thumbnail / transcode jobs
-> write media.asset.uploaded.v1 outbox
```

下载：

```text
GetMediaDownloadURL
-> load asset
-> reject deleted/quarantined/not-ready
-> verify conversation/message visibility through public port
-> create short-lived presigned GET URL
-> write media_access_audit
```

绑定消息：

```text
message-service SendMessage(IMAGE/FILE/VOICE/VIDEO)
-> stores asset_id as attachment reference
-> media-service remains asset metadata owner
-> message-service does not read object_key
```

## 9. 一致性和事务

强一致边界：

- upload session、asset metadata、processing job 和 media outbox 在同一 PostgreSQL 事务内更新。
- CompleteUpload 验证 object metadata 后再推进状态；失败不写 ready event。
- DeleteMediaAsset 写 asset tombstone 和 outbox 同事务。

最终一致边界：

- object storage 写入先于 CompleteUpload；过期 session cleanup 删除孤儿对象。
- scan / thumbnail / transcode 通过 worker 重试；失败不让 asset 进入 READY。
- 下游 search / memory / audit 通过 `im.media.events` 最终同步。

## 10. 幂等、重试和补偿

| 场景 | 幂等键 | 重试策略 | 补偿 |
| --- | --- | --- | --- |
| CreateUploadSession | tenant + user + idempotency_key | 同 command replay 返回原 session | 过期 cleanup |
| CompleteUpload | tenant + asset_id + upload_session_id | replay 返回当前 asset 状态 | hash/size mismatch fail closed |
| ProcessingJob | job_id | bounded retry + DLQ | operator redrive / quarantine |
| OutboxRelay | event_id | bounded retry + DLQ | outbox repair operator |
| DeleteMediaAsset | tenant + asset_id + delete_request_id | replay 返回 tombstone | object delete retry |

## 11. 权限和安全

- API 必须使用 verified metadata；request body 中 tenant/user 不能覆盖 trusted context。
- CreateUploadSession 要校验用户可向 conversation 发送对应媒体类型。
- GetMediaDownloadURL 必须校验 conversation membership、message visibility、tombstone 和 policy deny。
- presigned URL TTL 默认短，且不能包含业务敏感 metadata。
- virus scan 未通过的 asset 不能下载，也不能被 message-service 视为可展示附件。
- audit 只保存低敏字段：asset_id、media_kind、size、hash、status、request_id、decision_source。
- scanner / storage / transcode 原始错误不进入 public response、Kafka payload 或 audit export。

## 12. SLO 和指标

第一阶段不写生产 SLO，只保留本地 / 面试展示指标：

| 指标 | 目标 |
| --- | --- |
| upload session create p95 | 本地 smoke 可观测 |
| complete upload p95 | 本地 smoke 可观测 |
| processing backlog | 可通过 `/debug/metrics` 查看 |
| outbox pending / DLQ | 可通过 `/metrics` 和 repair operator 查看 |

必须打点：

```text
media_asset_total{status,kind}
media_upload_session_total{status}
media_processing_job_total{type,status}
media_outbox_total{status}
media_access_denied_total{reason}
```

## 13. 测试方案

| 测试 | 目标 |
| --- | --- |
| domain unit | 状态机、content type、size/hash、variant 校验 |
| app unit | idempotency、permission deny、provider unavailable |
| PostgreSQL integration | upload session / complete / delete / outbox 同事务 |
| object storage fake | HEAD mismatch、missing object、presign failure |
| worker test | scan / thumbnail / transcode retry 和 DLQ |
| contract test | gRPC error mapping、event builder fail-closed |
| smoke | create session -> fake upload -> complete -> ready -> download URL |

## 14. Runbook

运行模式：

```text
NEXUSIM_MEDIA_SERVICE_MODE=grpc
NEXUSIM_MEDIA_SERVICE_MODE=processing-worker
NEXUSIM_MEDIA_SERVICE_MODE=outbox-relay
NEXUSIM_MEDIA_SERVICE_MODE=cleanup
```

本地第一版可使用 fake object storage / scanner / thumbnail / transcode adapter。接真实 S3-compatible storage 前，必须补 endpoint、TLS、credential、bucket allowlist 和 object key prefix guard。

operator：

```text
media-outbox-audit
media-outbox-repair
media-processing-audit
media-processing-redrive
media-orphan-cleanup
```

## 15. 验收标准

进入编码前：

- 本 SDD v0.1 draft 被复核，无 P0/P1。
- `media-service` brief 指向本 SDD。
- promotion 对 service-registry、proto、migration、Docker、Prometheus、Grafana 的影响明确。

进入 first smoke 前：

- proto / migration /六层 skeleton / cmd runtime 已落。
- PostgreSQL repository 和 fake object storage integration 测试通过。
- `CreateUploadSession -> CompleteUpload -> GetMediaDownloadURL` 本地 smoke 通过。
- malformed outbox / processing payload fail closed，不发布错误事件。

当前实现进展：

- `CreateUploadSession -> CompleteUpload -> GetMediaDownloadURL` 最小 gRPC smoke 已通过。
- `media_outbox -> im.media.events` 最小 relay 代码切片已落，包含 Kafka schema、
  outbox store、Kafka producer、trigger relay 和 `outbox-relay` runtime mode。
- 真实 PostgreSQL 测试已覆盖同一 asset 事件按递增 `id` 顺序发布、低版本 DLQ
  阻塞后续事件、retry 写稳定低敏 `last_error`。
- 真实 Kafka outbox relay smoke 已通过，验证 `media_outbox PUBLISHED=2`、
  `PENDING=0`、`DLQ=0`，并从 Kafka 读回两个 typed `MediaEvent`。
- 第一版 processing worker 已落地：`CompleteUpload` 进入 `PROCESSING` 并写
  `SCAN` / `THUMBNAIL` / `TRANSCODE` jobs；worker 使用本地 mock adapter 完成
  jobs 后推进 asset `READY`，再写 `media.asset.ready.v1`。
- 真实 S3、scanner、thumbnail/transcode provider 继续后置。
