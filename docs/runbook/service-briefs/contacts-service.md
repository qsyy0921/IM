# contacts-service

## 当前状态

- 已有好友申请、申请来源 metadata、列表 / 搜索 / 分组过滤、接受、拒绝、取消、删除、拉黑、解除拉黑、备注、联系人分组、联系人申请隐私开关；新建申请会被双向 `BLOCKED` 关系、租户级来源策略、目标用户 / 租户默认隐私 gate；已补 first-stage `ALLOW / DENY` 联系人隐私例外和查询 / 清理 API：目标用户可以对指定请求人放行、拒绝、查询或删除例外，`DENY` 会在普通隐私 gate 前拒绝，`ALLOW` 可绕过目标用户 / 租户默认隐私 gate 但不绕过双向 `BLOCKED` 关系或租户来源策略；目标用户可在总体允许好友申请的前提下单独拒绝 `SEARCH` 来源的陌生申请，并可通过 `allow_profile_visibility` 总开关和字段级 `profile_visibility_fields` 表达陌生人 profile 可见性偏好，当前字段白名单为 `DISPLAY_NAME / AVATAR / ORGANIZATION / TITLE / STATUS_MESSAGE`；申请 `source_ref` 只允许低敏业务引用，types 层已 fail-fast 拒绝邮箱、手机号样式、token / password / secret / bearer 等认证敏感字段和控制字符，不放原始 invite token、手机号 / 邮箱或认证 secret；租户来源策略已支持 `LOW / MEDIUM / HIGH` 风险标注和 `review_required` 审批门禁，新建申请会持久化并通过 `contact.request.created.v1` 发布这些低敏审计字段；当来源策略要求 review 时，新建申请进入 `REVIEW_REQUIRED`，普通接收方不能直接接受 / 拒绝，需由 `contact-request-review` operator mode 审批，审批通过后回到 `PENDING` 等待接收方处理，审批拒绝则进入 `DECLINED` 并写低敏 review audit / declined outbox；联系人搜索覆盖 `contact_user_id / remark / group_name`；联系人申请隐私支持 `USER -> TENANT_DEFAULT -> SYSTEM_DEFAULT` 三层读取，并提供租户默认值 / 来源策略本地 operator mode。
- contacts-service 是联系人事实源。
- policy-service 通过 contacts event projection 做 direct block 决策。
- 已补 `/healthz`、`/readyz`、`/debug/metrics` 和 first-stage Prometheus text `/metrics`；可观察低敏 gRPC、PG pool、联系人申请 / 联系人边状态聚合、`contacts_outbox`、outbox relay retry 和 OTel trace config 聚合状态；本地 scrape 目标为 `host.docker.internal:11915`，并已补 Prometheus alert rules 和 Grafana dashboard 原型；这些只用于本地开发 / 面试展示，不代表生产 SLO。
- debug HTTP 监听默认只允许 loopback / RFC1918 私网，公网或 unspecified 地址必须显式 `NEXUSIM_CONTACTS_DEBUG_ALLOW_PUBLIC=true`。
- 已补 first-stage OpenTelemetry gRPC server span；gRPC access log 只记录低敏 `trace_id/request_id`，并对白名单外入口 metadata 直接丢弃。
- `outbox-relay` 现已对非取消运行时错误做退避重试，并在 relay 模式通过 `/debug/metrics` 暴露低敏 retry 快照；现有 outbox / retry / DLQ 业务语义不变。
- Kafka writer 已显式固定 `acks=all`、禁自动建 topic、bounded attempts/backoff，并由本地门禁和 package 单测防漂移；真正 idempotent / transactional producer 仍属后续客户端选型。
- 已补只读 `outbox-audit`、`outbox-repair`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，可直接审计、redrive 当前 `contacts_outbox`，并按 retention 清理 contacts outbox repair 历史；`outbox-audit` / `outbox-repair` / `outbox-repair-audit` / `outbox-repair-cleanup` 可通过 `NEXUSIM_CONTACTS_OUTBOX_AUDIT_OUTPUT` / `NEXUSIM_CONTACTS_OUTBOX_REPAIR_OUTPUT` / `NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_OUTPUT` / `NEXUSIM_CONTACTS_OUTBOX_REPAIR_CLEANUP_OUTPUT` 写低敏 JSON 结果或 cleanup summary；租户隐私默认值 / 来源策略 audit 和 set 可通过 `NEXUSIM_CONTACTS_TENANT_PRIVACY_AUDIT_OUTPUT` / `NEXUSIM_CONTACTS_TENANT_PRIVACY_SET_OUTPUT` / `NEXUSIM_CONTACTS_SOURCE_POLICY_AUDIT_OUTPUT` / `NEXUSIM_CONTACTS_SOURCE_POLICY_SET_OUTPUT` 写低敏策略 JSON；`contact-request-review-audit` 可按 tenant / request / operator / decision / next_status / risk_level 导出低敏审核审计 JSON，只输出 `reason_present` 不输出审核 reason 原文，便于 operator 留存证据。
- 已补 contacts outbox publish / audit / repair audit 错误脱敏：`last_error`、`previous_last_error` 只保留稳定低敏分类，不暴露 broker body、账号、token 或 provider 原文。
- 当 `NEXUSIM_CONTACTS_AUTH_MODE=metadata|verified-metadata` 时，公网监听地址 + 无 gRPC mTLS client cert 的危险组合会在启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。
- PostgreSQL repository 已按主流程 / helper / privacy / source policy 同 package 拆分，主文件降到约 1000 行；PostgreSQL privacy / source-policy 集成测试也已拆到独立同 package 测试文件，避免联系人逻辑和测试继续堆进单个大文件。
- `loadtest/contacts` summary 已输出 `capacity_summary`，包含运行时长、场景、操作数、Kafka contact event 数、contacts outbox 聚合、ops/s、events/s 和 latency p95/p99；后续容量验证可直接复用该结构。
- `capacity-baseline-contacts-stack-20260616-r2` 本地 stack 短基线已跑通：`contacts_outbox PUBLISHED=2/PENDING=0/DLQ=0`，Kafka 读回 `contact.request.created.v1` 和 `contact.request.accepted.v1`，`operations_per_second=0.89`；报告见 `loadtest/distributed/loadtest-report-20260616-contacts-stack-capacity-baseline.md`。

## 后续

- 更细隐私策略：组织级策略、租户默认值 / 来源策略 / 隐私例外接入 admin/config service 正式权限面；当前 `REVIEW_REQUIRED` 和 `contact-request-review` 是 first-stage 本地 operator 审批状态机，还不是完整后台审批产品。
- 生产级 OTel collector、Alertmanager、SLO dashboard、长时间容量曲线仍属于后续统一观测治理。
