# message-service

## 当前状态

- 已有 `SendMessage`、`EditMessage`、`RevokeMessage`、`DeleteMessage` 主链路；`SendMessage` 支持 `TEXT`、第一阶段 `IMAGE` / `FILE` / `VOICE` 附件引用消息，以及 `LOCATION` / `CARD` 结构化 payload 消息。
- `DeleteMessage` 已收紧 delete scope 契约：内部未知 scope 直接拒绝；`CONVERSATION_VIEW` 保持作者 / policy override 删除语义；`COMPLIANCE_RETENTION` 第一阶段只允许 policy 返回 `OwnershipOverride=true`、提交 `compliance_approval_id` / `external_proof_ref`，且本地 external proof ref 处于 `VERIFIED`、approval row 处于 `APPROVED` 后执行；成功删除会在同一事务内消费 approval、redaction 当前 `message_log.payload_json` 与本次 change history payload，timeline / outbox 只写低敏 reason-present / proof-ref-present 证明。
- 已补 first-stage legal hold：`message_legal_holds` 可记录 / release 消息级 hold，`DeleteMessage` 在同一事务内检查 ACTIVE hold，命中时 fail-closed，不推进 `conversation_seq`，不写 change history、timeline 或 outbox；`legal-hold-audit` / `legal-hold-set` / `legal-hold-release` operator 可写低敏 JSON 结果，输出只暴露 reason-present，不输出 hold reason 原文。
- 已补 first-stage compliance external proof / delete approval：`message_compliance_external_proofs` 只保存低敏 proof ref、provider 和 proof hash，不保存 proof 正文；`message_compliance_delete_approvals` 引用已 VERIFIED 的 external proof ref；`compliance-proof-audit/register/revoke` 和 `compliance-approval-audit/create/cancel` operator 可写低敏 JSON 结果。
- 通过 outbox relay 发布 conversation timeline events，不在业务事务里直接 publish Kafka。
- 已接 conversation-service / policy-service，可走 verified metadata、TLS / mTLS。
- 已补 `/healthz`、`/readyz`、`/debug/metrics` 和 Prometheus text `/metrics`，可观察低敏 PG pool、send / repository / Kafka / outbox relay 聚合指标和固定 operation latency；debug HTTP 监听默认只允许 loopback / 私网，公网或未指定地址必须显式 `NEXUSIM_MESSAGE_DEBUG_ALLOW_PUBLIC=true`。
- 本地 Prometheus scrape / alert rules 与 Grafana dashboard 原型已覆盖 SendMessage / PG pool / Kafka / outbox relay latency、relay runtime error 和 OTLP endpoint missing；这仍是本地开发 / 面试演示级观测，不是生产 Alertmanager / SLO。
- 已补 first-stage OpenTelemetry gRPC server span，默认关闭；启用后可输出 stdout 或 OTLP gRPC trace，并从入口 metadata 提取 W3C `traceparent`。
- gRPC access log 只记录低敏 `trace_id/request_id`，并对入口 metadata 做 trim、长度上限和字符白名单过滤，避免把 token / 邮箱 / 原始认证头写入结构化日志。
- `outbox-relay` 对非取消运行时错误已改为退避重试，并在 relay 模式通过 `/debug/metrics` 暴露 low-sensitive outbox relay retry 快照；malformed event / payload 仍保持 fail-closed，交给 outbox retry / DLQ 语义处理；`message_outbox.last_error` 和 repair audit `previous_last_error` 只暴露稳定公开文案，不落 Kafka / publisher 原始错误正文。
- Kafka writer 已显式固定 `acks=all`、禁自动建 topic、bounded attempts/backoff，并由本地门禁和 package 单测防漂移；真正 idempotent / transactional producer 仍属后续客户端选型。
- 已补 `outbox-audit`、`outbox-repair`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，可直接审计、redrive 和清理 `message_outbox` repair 历史；`outbox-audit` / `outbox-repair` / `outbox-repair-audit` / `outbox-repair-cleanup` 可通过 `NEXUSIM_MESSAGE_OUTBOX_AUDIT_OUTPUT` / `NEXUSIM_MESSAGE_OUTBOX_REPAIR_OUTPUT` / `NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_OUTPUT` / `NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_OUTPUT` 写低敏 JSON 结果或 cleanup summary，便于 operator 留存证据。
- 已补只读 `change-history-audit`，可按 tenant / conversation / message / change type / changed_by 审计 `message_change_history` 中 `EDIT / REVOKE / DELETE` 变更证明；可通过 `NEXUSIM_MESSAGE_CHANGE_HISTORY_AUDIT_OUTPUT` 写低敏 JSON 结果，输出只包含状态转换和 payload / reason 存在性，不输出消息 payload 或 reason 原文。
- 已补只读 `retention-proof-audit`，可按 tenant / conversation / message / status 审计当前 `message_log` 与最新 `DELETE` change history、`message.deleted.v1` timeline / outbox 的删除证明；默认只看 `DELETED` 消息，可通过 `NEXUSIM_MESSAGE_RETENTION_PROOF_AUDIT_OUTPUT` 写低敏 JSON 结果，不输出消息 payload 或删除 reason 原文。
- 已补 trusted metadata 启动门禁：当 `NEXUSIM_MESSAGE_AUTH_MODE=metadata|verified-metadata` 时，如果 gRPC 监听地址不是 loopback / RFC1918 私网，且服务端未启用 mTLS client cert 校验，则启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。
- `loadtest/sendmessage` 容量 runner 已按 config / model / auth / util 同 package 拆分，避免后续容量观测继续堆进单个 `main.go`。
- `loadtest/sendmessage` summary 已新增 `capacity_summary`，统一输出 actual duration、targets、VU、conversation、request/success/error、logical request、RPS、p95/p99、outbox 和 PG pool 关键计数；这是容量基线口径，不等于已完成生产容量压测。
- `loadtest/capacityseed` 已能准备 `tenant-capacity-message` 下的 ACTIVE conversation / member fixture；`capacity-baseline-seeded-20260616` 本地 seeded 短基线中 `sendmessage` 成功 408、`accepted_rps=81.34`，报告见 `loadtest/distributed/loadtest-report-20260616-seeded-capacity-baseline.md`。
- PostgreSQL repository 的 revoke / edit / delete mutation 集成测试已拆到同 package `repository_mutation_test.go`，保留原覆盖面并降低单个测试文件复杂度。

## 后续

- 会话级删除策略深化、外部 proof provider 集成 / proof 正文存证留在外部系统、长时间容量曲线和生产观测；用户私有隐藏已由 delivery-service `HideInboxItem` 承担，图片 / 文件 / 语音二进制上传和处理属于后续 media 能力。
- 生产级 OTel collector、告警路由、retention 和 SLO dashboard 仍属于后续统一观测治理。
