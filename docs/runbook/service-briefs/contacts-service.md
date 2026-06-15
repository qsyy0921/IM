# contacts-service

## 当前状态

- 已有好友申请、申请来源 metadata、列表 / 搜索 / 分组过滤、接受、拒绝、取消、删除、拉黑、解除拉黑、备注、联系人分组、联系人申请隐私开关；新建申请会被双向 `BLOCKED` 关系、租户级来源策略、目标用户 / 租户默认隐私 gate；目标用户可在总体允许好友申请的前提下单独拒绝 `SEARCH` 来源的陌生申请；申请 `source_ref` 只允许低敏业务引用，不放原始 invite token、手机号 / 邮箱或认证 secret；联系人搜索覆盖 `contact_user_id / remark / group_name`；联系人申请隐私支持 `USER -> TENANT_DEFAULT -> SYSTEM_DEFAULT` 三层读取，并提供租户默认值 / 来源策略本地 operator mode。
- contacts-service 是联系人事实源。
- policy-service 通过 contacts event projection 做 direct block 决策。
- 已补 `/healthz`、`/readyz`、`/debug/metrics` 和 first-stage Prometheus text `/metrics`；可观察低敏 gRPC、PG pool、联系人申请 / 联系人边状态聚合、`contacts_outbox`、outbox relay retry 和 OTel trace config 聚合状态；本地 scrape 目标为 `host.docker.internal:11915`，并已补 Prometheus alert rules 和 Grafana dashboard 原型；这些只用于本地开发 / 面试展示，不代表生产 SLO。
- debug HTTP 监听默认只允许 loopback / RFC1918 私网，公网或 unspecified 地址必须显式 `NEXUSIM_CONTACTS_DEBUG_ALLOW_PUBLIC=true`。
- 已补 first-stage OpenTelemetry gRPC server span；gRPC access log 只记录低敏 `trace_id/request_id`，并对白名单外入口 metadata 直接丢弃。
- `outbox-relay` 现已对非取消运行时错误做退避重试，并在 relay 模式通过 `/debug/metrics` 暴露低敏 retry 快照；现有 outbox / retry / DLQ 业务语义不变。
- Kafka writer 已显式固定 `acks=all`、禁自动建 topic、bounded attempts/backoff，并由本地门禁防漂移；真正 idempotent / transactional producer 仍属后续客户端选型。
- 已补只读 `outbox-audit`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，可直接审计当前 `contacts_outbox`，并按 retention 清理 contacts outbox repair 历史。
- 已补 contacts outbox publish / audit / repair audit 错误脱敏：`last_error`、`previous_last_error` 只保留稳定低敏分类，不暴露 broker body、账号、token 或 provider 原文。
- 当 `NEXUSIM_CONTACTS_AUTH_MODE=metadata|verified-metadata` 时，公网监听地址 + 无 gRPC mTLS client cert 的危险组合会在启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。
- PostgreSQL repository 已按主流程 / helper / privacy / source policy 同 package 拆分，主文件降到约 1000 行，避免继续把联系人逻辑堆进单个大文件。

## 后续

- 更细隐私策略：profile 可见性、陌生人申请的组织 / 风险 / 审批策略；租户默认值和来源策略后续可接入 admin/config service 的正式权限面。
- 生产级 OTel collector、Alertmanager、SLO dashboard 和容量验证仍属于后续统一观测治理。
