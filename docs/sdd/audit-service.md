# audit-service SDD v0.1 Draft

## 1. 服务定位

`audit-service` 是 NexusIM 的统一审计归档和证明服务。它接收身份、安全、
策略、管理、Agent、工具执行、通知和 operator 相关低敏审计事件，提供统一查询、
导出、retention、legal hold 和 hash-chain tamper evidence。

职责：

- 拥有 `audit_record`、`audit_hash_segment`、`audit_export_job`、
  `audit_ingestion_checkpoint` 和 `audit_outbox`。
- 通过公开 API 或 Kafka consumer 归档各服务已经产出的低敏审计事实。
- 为 admin / security / compliance 场景提供查询、导出和完整性校验。
- 为 Agent 写动作串联 proposal、approval、policy decision、tool prepare、
  executor result 和 operator action。
- 为审计导出提供 manifest、hash proof 和低敏 redaction。

不负责：

- 不拥有业务事实，不决定登录、发消息、成员、投递、权限、通知或 Agent 执行结果。
- 不替代各服务本地事务内 audit、repair audit 或 outbox audit。
- 不直接读取其它服务私有表。
- 不直接修复业务状态，不作为 repair workflow 的执行者。
- 不保存 token、password、TOTP、recovery code、raw prompt、EvidencePack 原文、
  message body、provider body、SQL error、secret 或完整 PII 明文。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游事件 | identity / policy / agent / action / notification / admin / control-plane | 低敏 audit events |
| 上游 API | trusted internal services / operators | `AppendAuditRecord` 追加低敏审计 |
| 同步依赖 | policy-service | audit query / export 权限检查 |
| 同步依赖 | object storage adapter | export manifest / proof 文件写入 |
| 下游 | admin-service / security tooling | 查询、导出、proof verify |
| 异步下游 | SIEM / external sink（后续） | audit export / forward events |
| 事实源 | PostgreSQL | records、hash segments、export jobs、checkpoints |

审计输入必须来自公开 API、Kafka 事件或明确 port。`audit-service` 不能通过 SQL 连接
读取 identity、policy、message、agent 或 action-executor 的私有表来补数据。

## 3. 六层 DDD 包结构

```text
services/audit-service/
  cmd/audit-service/
  internal/{api,app,domain,infrastructure,types,trigger}/
```

| 层 | 本服务内容 |
| --- | --- |
| `api` | gRPC adapter，verified metadata，稳定错误映射 |
| `app` | AppendAuditRecord、QueryAuditRecords、CreateAuditExport、VerifyAuditProof |
| `domain` | audit record canonicalization、hash-chain、redaction、retention policy |
| `infrastructure` | PostgreSQL repository、Kafka consumers、object storage adapter、policy client |
| `types` | command、DTO、错误码、枚举、low-sensitive envelope |
| `trigger` | audit consumer、export worker、outbox relay、retention cleanup |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `AuditRecord` | 一条低敏审计记录 | append-only；canonical hash 稳定；tenant scoped |
| `AuditHashSegment` | 一段 record hash 链摘要 | per tenant / stream / time bucket 单调推进 |
| `AuditExportJob` | 导出任务和 manifest | 导出内容必须经过 redaction policy |
| `AuditIngestionCheckpoint` | Kafka consumer checkpoint | 按 group/topic/partition 记录 next offset |
| `AuditOutboxEvent` | 审计服务事件 | 只发布低敏 export / proof / ingestion 状态 |

Record 类型第一版：

```text
IDENTITY_AUTH
IDENTITY_CHALLENGE_DELIVERY
POLICY_DECISION
POLICY_TOOL_DECISION
AGENT_PROPOSAL
AGENT_APPROVAL
MCP_TOOL_PREPARE
ACTION_EXECUTION
NOTIFICATION_DELIVERY
ADMIN_OPERATION
OPERATOR_REPAIR
SYSTEM_SECURITY
```

Record 生命周期：

```text
APPENDED -> SEALED
APPENDED -> REDACTED_VIEW_ONLY
APPENDED/SEALED -> RETENTION_EXPIRED
```

物理行保持 append-only。`REDACTED_VIEW_ONLY` 表示查询 / 导出视图被脱敏，不表示原始
canonical low-sensitive record 被改写。

## 5. 同步 API 契约

```text
rpc AppendAuditRecord(AppendAuditRecordRequest) returns (AppendAuditRecordResponse)
rpc QueryAuditRecords(QueryAuditRecordsRequest) returns (QueryAuditRecordsResponse)
rpc CreateAuditExport(CreateAuditExportRequest) returns (CreateAuditExportResponse)
rpc GetAuditExport(GetAuditExportRequest) returns (GetAuditExportResponse)
rpc VerifyAuditProof(VerifyAuditProofRequest) returns (VerifyAuditProofResponse)
```

`AppendAuditRecord` 请求字段：

```text
tenant_id, source_service, source_event_id, record_type
actor_ref, subject_ref, resource_ref
action, outcome, reason_code, risk_level
correlation_id, causation_id, trace_id, request_id
occurred_at
attributes_json
idempotency_key
```

`attributes_json` 只能包含低敏 key-value：

```text
message_key, conversation_key, session_key, device_key, proposal_id,
approval_id, prepared_audit_id, execution_id, policy_decision_id,
failure_class, provider_class, operator_mode, repair_outcome
```

禁止字段：

```text
raw token, password, TOTP/recovery code, raw message body, raw prompt,
raw EvidencePack, provider body, SQL error, object storage key, destination,
email/phone full value, IP full value unless explicitly hashed/masked
```

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | record type、字段、时间窗口或 attributes 非法 | 否 |
| `PERMISSION_DENIED` | 调用方无 append / query / export 权限 | 否 |
| `ALREADY_EXISTS` | idempotency key replay 命令冲突 | 否 |
| `NOT_FOUND` | record / export / proof 不存在 | 否 |
| `FAILED_PRECONDITION` | segment 未封存、export 未完成、retention 禁止访问 | 否 |
| `UNAVAILABLE` | storage / policy / object storage 暂不可用 | 是 |

## 6. 异步输入和输出

输入来源：

| 来源 | Topic / API | 说明 |
| --- | --- | --- |
| identity-service | `im.identity.events` / internal API | login、session revoke、challenge delivery |
| policy-service | `im.policy.events` | policy decision / tool decision |
| agent-service | `im.agent.events` | proposal / approval |
| action-executor | `im.action.events` 或 internal API | execution audit / provider failure |
| notification-service | `im.notification.events` | delivery state / bounce |
| admin / control-plane | internal API | operator / config / quota change |

输出事件：

| 事件 | Topic | 分区键 | 说明 |
| --- | --- | --- | --- |
| `audit.record.appended.v1` | `im.audit.events` | `tenant_id:audit_stream` | 记录已归档 |
| `audit.segment.sealed.v1` | `im.audit.events` | `tenant_id:audit_stream` | hash segment 已封存 |
| `audit.export.completed.v1` | `im.audit.events` | `tenant_id:export_id` | 导出完成 |
| `audit.export.failed.v1` | `im.audit.events` | `tenant_id:export_id` | 导出失败 |

输出事件禁止包含完整 audit record body，只能包含 record count、stream、time range、
manifest hash、object reference hash、failure_class 等低敏字段。

## 7. 数据库设计

第一版表：

```text
audit_records
audit_hash_segments
audit_export_jobs
audit_ingestion_checkpoints
audit_outbox
```

关键字段：

```text
audit_records:
tenant_id, audit_id, audit_stream, source_service, source_event_id,
record_type, actor_ref, subject_ref, resource_ref, action, outcome,
reason_code, risk_level, occurred_at, ingested_at,
attributes_json, canonical_json_hash, previous_record_hash, record_hash,
segment_id, retention_class, legal_hold, idempotency_key

audit_hash_segments:
tenant_id, segment_id, audit_stream, sequence, starts_at, ends_at,
record_count, first_record_hash, last_record_hash, segment_root_hash,
previous_segment_hash, sealed_at, seal_status

audit_export_jobs:
tenant_id, export_id, requested_by, filter_hash, redaction_profile,
status, object_ref_hash, manifest_hash, record_count,
created_at, completed_at, expires_at, failure_class, public_error

audit_ingestion_checkpoints:
consumer_group, topic, partition_id, offset_value, updated_at

audit_outbox:
event_id, tenant_id, aggregate_type, aggregate_id, event_type,
event_version, partition_key, payload_json, status, retry_count,
next_retry_at, published_at
```

`canonical_json_hash` 是 canonicalized low-sensitive audit payload 的 hash。
`record_hash = hash(previous_record_hash + canonical_json_hash + metadata)`。

## 8. Hash-chain 和 proof

第一版 hash-chain 语义：

```text
per tenant + audit_stream + day bucket
-> append records in ingested order
-> compute previous_record_hash / record_hash
-> seal segment with segment_root_hash
-> chain segment_root_hash to previous_segment_hash
```

Hash-chain 只证明：

- 记录顺序和内容在 `audit-service` 归档后没有被无痕改写；
- 某段导出 manifest 与封存 segment 可对应；
- operator 可以发现 segment 缺口或 hash 不一致。

Hash-chain 不证明：

- 上游业务事实一定真实；
- 上游没有漏发事件；
- 记录内容没有在上游被脱敏或裁剪；
- 数据本身保密。保密由 redaction、访问控制、存储加密和 retention 负责。

## 9. 核心流程

同步追加：

```text
AppendAuditRecord
-> verify trusted metadata / policy
-> validate low-sensitive schema
-> idempotency replay check
-> canonicalize payload
-> append audit_records with hash-chain pointer
-> write audit.record.appended.v1 outbox
```

Kafka ingestion：

```text
audit-consumer
-> fetch upstream audit event
-> map to low-sensitive AuditRecord
-> append record + checkpoint in one transaction
-> commit Kafka offset after DB commit
```

导出：

```text
CreateAuditExport
-> policy check
-> persist export job
-> export-worker reads allowed records
-> apply redaction profile
-> write manifest / proof to object storage
-> mark completed + write audit.export.completed.v1
```

Agent 动作审计链：

```text
agent proposal
-> policy tool decision
-> mcp prepare audit
-> approval
-> action-executor result
-> audit-service query can join by proposal_id / approval_id / prepared_audit_id / execution_id refs
```

## 10. 一致性和事务

强一致边界：

- `audit_records` append、hash pointer 更新和 `audit_outbox` 写入同事务。
- ingestion checkpoint 与 record append 同事务；Kafka commit 只在 DB commit 后执行。
- export job 状态和 export outbox 同事务。

最终一致边界：

- 上游服务本地 audit 与 audit-service 归档之间通过 Kafka / API 最终同步。
- export 文件写 object storage 成功但 DB commit 失败时可能遗留 orphan object；cleanup
  worker 负责按 manifest 状态清理。
- external SIEM sink 是后续功能，不进入第一版强一致边界。

## 11. 幂等、重试和补偿

| 场景 | 幂等键 | 重试策略 | 补偿 |
| --- | --- | --- | --- |
| AppendAuditRecord | tenant + source_service + source_event_id | replay 返回原 audit_id | command hash 冲突 fail closed |
| Kafka ingestion | topic + partition + offset | DB commit 后 offset commit | checkpoint rewind operator |
| Segment sealing | tenant + stream + sequence | bounded retry | seal audit / repair operator |
| Export job | export_id | worker retry + DLQ | cancel / redrive / orphan cleanup |
| Outbox relay | event_id | bounded retry + DLQ | outbox repair operator |

## 12. 权限、隐私和安全

- Append API 只允许服务身份或 operator 身份；客户端不能直接写审计记录。
- Query / export 必须通过 policy-service 或 admin verified role check。
- 默认查询返回 redacted view；原始 low-sensitive canonical JSON 只给受控 operator。
- tenant 间严格隔离；跨 tenant export 禁止。
- `attributes_json` 需要 schema allowlist；未知字段默认拒绝或进入 `ignored_keys_hash`。
- raw prompt、raw model output、EvidencePack 原文和 message body 不能作为 audit
  record attributes；只能存 hash、count、source ref 和 safety classification。
- retention cleanup 不能删除 legal hold 记录；legal hold 只能通过 control-plane /
  admin 审批流设置。

## 13. Retention、legal hold 和 export

Retention class：

```text
SECURITY_SHORT
SECURITY_STANDARD
COMPLIANCE_LONG
OPERATOR_REPAIR
AI_ACTION
```

规则：

- cleanup worker 只删除超过 retention 且未 legal hold 的记录或导出文件。
- 删除前写 `audit.retention.expired.v1` outbox。
- export manifest 必须包含 filter hash、record count、time range、segment refs、
  manifest hash 和 redaction profile。
- 导出文件默认短 TTL；长期合规归档需要外部 object-lock 或 WORM storage，第一版只保留
  adapter port。

## 14. SLO 和指标

第一阶段不写生产 SLO，只保留本地 / 面试展示指标：

```text
audit_record_total{record_type,outcome,source_service}
audit_ingestion_lag{topic,partition}
audit_hash_segment_total{stream,status}
audit_export_job_total{status,redaction_profile}
audit_outbox_total{status}
audit_redaction_reject_total{reason}
```

debug / metrics 禁止输出 tenant_id、user_id、audit_id、request_id、trace_id、
attributes_json、record hash 原文或 object URL。

## 15. 测试方案

| 测试 | 目标 |
| --- | --- |
| domain unit | canonicalization、hash-chain、redaction allowlist、retention |
| app unit | idempotency、permission deny、unknown field reject、export lifecycle |
| PostgreSQL integration | append + hash pointer + outbox 同事务、segment seal |
| consumer test | malformed upstream event fail closed、checkpoint only after append |
| export worker test | redaction、manifest hash、orphan cleanup |
| event builder | 不输出 attributes_json / secrets / raw prompt |
| smoke | AppendAuditRecord -> Query -> SealSegment -> CreateExport -> VerifyProof |

## 16. Runbook

运行模式：

```text
NEXUSIM_AUDIT_SERVICE_MODE=grpc
NEXUSIM_AUDIT_SERVICE_MODE=audit-consumer
NEXUSIM_AUDIT_SERVICE_MODE=segment-sealer
NEXUSIM_AUDIT_SERVICE_MODE=export-worker
NEXUSIM_AUDIT_SERVICE_MODE=outbox-relay
NEXUSIM_AUDIT_SERVICE_MODE=retention-cleanup
```

operator：

```text
audit-record-audit
audit-segment-verify
audit-export-audit
audit-export-redrive
audit-checkpoint-rewind
audit-retention-cleanup
audit-outbox-repair
```

## 17. 验收标准

进入编码前：

- 本 SDD v0.1 draft 被复核，无 P0/P1。
- `audit-service` brief 指向本 SDD。
- 明确第一版只做低敏审计归档、查询、导出和 proof，不替代本地业务 audit。

进入 first smoke 前：

- proto / migration / 六层 skeleton / cmd runtime 已落。
- PostgreSQL append、hash-chain、redaction、consumer checkpoint 和 export manifest 测试通过。
- `AppendAuditRecord -> QueryAuditRecords -> VerifyAuditProof` 本地 smoke 通过。
- malformed audit event fail closed，不推进 checkpoint，不写不完整 hash-chain。
- raw token、raw prompt、message body、provider body 和 SQL error 不会出现在事件、
  metrics、export manifest 或 repair summary。
