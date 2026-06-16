# policy-service

## 当前状态

- 已有 `CheckMessageAction`、PG exact rules、user message action restrictions、tenant rules、conversation role gate、contacts projection。
- 已有 ownership gate / override、decision audit outbox relay 和 repair。
- 已补 `/healthz`、`/readyz`、`/debug/metrics` 和 first-stage Prometheus text `/metrics`，可观察低敏 gRPC、decision、PG pool、policy rule store、projection、worker / relay retry 和 `policy_decision_audit_outbox` 聚合状态；本地 Prometheus scrape target 为 `host.docker.internal:11916`，并已有本地 alert rules / Grafana dashboard 原型；debug HTTP 监听默认只允许 loopback / RFC1918 私网，公网或 unspecified 地址必须显式 `NEXUSIM_POLICY_DEBUG_ALLOW_PUBLIC=true`。
- 已补 first-stage OpenTelemetry gRPC server span；gRPC access log 只记录低敏 `trace_id/request_id`，并对白名单外入口 metadata 直接丢弃。
- 当 `NEXUSIM_POLICY_GRPC_ADDR` 是公网地址时，若未启用入口 gRPC TLS，进程会在启动前直接失败，避免把内部 policy decision API 暴露到 plaintext 公网入口；私网 / loopback 仍保留第一阶段本地直连。
- `outbox-relay` 对非取消运行时错误已改为退避重试，并在 relay 模式通过 `/debug/metrics` 暴露 low-sensitive outbox relay retry 快照；malformed payload / unsupported event 仍保持 store 驱动的 retry / DLQ 语义。
- Kafka writer 已显式固定 `acks=all`、禁自动建 topic、bounded attempts/backoff，并由本地门禁和 package 单测防漂移；真正 idempotent / transactional producer 仍属后续客户端选型。
- `policy_decision_audit_outbox` publish / audit / repair audit 已补错误脱敏，`last_error` / `previous_last_error` 只保留稳定低敏分类，不暴露 broker body、账号、token 或 provider 原文。
- `contact-consumer` 和 `timeline-consumer` 对非取消错误已改为退避重试，并在 worker 模式通过 `/debug/metrics` 暴露 projection worker retry 快照。
- 已补只读 `outbox-audit`、`outbox-repair`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，可直接审计 / redrive 当前 `policy_decision_audit_outbox` 行，以及按 retention / scope 清理 policy decision audit outbox repair 历史；`outbox-audit` / `outbox-repair` / `outbox-repair-audit` / `outbox-repair-cleanup` 可通过 `NEXUSIM_POLICY_OUTBOX_AUDIT_OUTPUT` / `NEXUSIM_POLICY_OUTBOX_REPAIR_OUTPUT` / `NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_OUTPUT` / `NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OUTPUT` 写低敏 JSON 结果或 cleanup summary，便于 operator 留存证据。
- 已补 first-stage `decision-audit-export` 运维模式，可按 tenant / event / action / allowed / classification / reason_code / outbox status / created_at RFC3339 时间窗口导出 `policy_decision_audit_outbox` 中的低敏决策审计 JSON，只输出 stable key、决策元数据、trace/request id 和发布状态，不输出消息正文、provider body、payload_json 或用户原始标识。
- 已补 first-stage tenant action quota：`policy_tenant_message_action_quotas` 可按 tenant / action 配置窗口内已允许决策上限，达到阈值时在 exact / tenant allow rule 前 fail-closed deny；`tenant-quota-audit` / `tenant-quota-set` 提供本地 JSON operator，输出只暴露配置元数据和 reason-present，不输出 operator reason 原文。
- 已补 first-stage content moderation adapter：message-service 只把 `SEND` / `EDIT` 的 `payload.text` 传给 policy-service；policy-service 通过 `NEXUSIM_POLICY_MODERATION_MODE=keyword` 做本地分类 deny，或通过 `NEXUSIM_POLICY_MODERATION_MODE=http` 调用外部 provider 返回稳定 allow/deny 决策；不持久化正文，不把 provider 原文、provider body 或消息正文写入 audit/outbox，HTTP endpoint 默认要求 HTTPS。
- message-service 通过 policy-service 做权限决策，不复制策略实现。
- `loadtest/policyintegration` smoke runner 已按 config / model / auth / util 同 package 拆分，避免策略集成验证继续堆进单个 `main.go`。
- `loadtest/policy` summary 已输出 `capacity_summary`，包含运行时长、action/allow/deny 计数、decision/s、latency p95/p99、permission version 和 classification 口径；已通过 `capacity-baseline-direct-20260616-v3` 跑过本地 direct 短基线，原始结果在 `H:\NexusIM\loadtest-results\capacity-baseline-direct-20260616-v3`，报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260616-policy-direct-capacity-baseline.md`。

## 后续

- 完整 ReBAC、provider-grade moderation / risk scoring、provider-grade tenant DSL / quota、provider-grade 外部 audit sink；当前 keyword / HTTP moderation、decision-audit-export、tenant action quota、Prometheus / Grafana 和 direct capacity baseline 只用于本地开发和面试展示，生产 OTel collector / alerting / SLO dashboard、长时间容量曲线和生产 sizing 仍属于后续统一观测治理。
