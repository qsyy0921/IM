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
- 已补只读 `outbox-audit`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，可直接审计当前 `policy_decision_audit_outbox` 行，以及按 retention / scope 清理 policy decision audit outbox repair 历史，不改当前 live outbox 状态；`outbox-audit` / `outbox-repair-audit` / `outbox-repair-cleanup` 可通过 `NEXUSIM_POLICY_OUTBOX_AUDIT_OUTPUT` / `NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_OUTPUT` / `NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OUTPUT` 写低敏 JSON 结果或 cleanup summary，便于 operator 留存证据。
- message-service 通过 policy-service 做权限决策，不复制策略实现。
- `loadtest/policyintegration` smoke runner 已按 config / model / auth / util 同 package 拆分，避免策略集成验证继续堆进单个 `main.go`。
- `loadtest/policy` summary 已输出 `capacity_summary`，包含运行时长、action/allow/deny 计数、decision/s、latency p95/p99、permission version 和 classification 口径；已通过 `capacity-baseline-direct-20260616-v3` 跑过本地 direct 短基线，原始结果在 `H:\NexusIM\loadtest-results\capacity-baseline-direct-20260616-v3`，报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260616-policy-direct-capacity-baseline.md`。

## 后续

- 完整 ReBAC、内容分类 / provider-backed moderation、tenant DSL / quota、外部 audit sink；当前 Prometheus / Grafana 和 direct capacity baseline 只用于本地开发和面试展示，生产 OTel collector / alerting / SLO dashboard、长时间容量曲线和生产 sizing 仍属于后续统一观测治理。
