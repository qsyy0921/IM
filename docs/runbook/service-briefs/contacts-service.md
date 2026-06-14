# contacts-service

## 当前状态

- 已有好友申请、列表、接受、拒绝、取消、删除、拉黑、解除拉黑、备注。
- contacts-service 是联系人事实源。
- policy-service 通过 contacts event projection 做 direct block 决策。
- 已补 `/healthz`、`/readyz`、`/debug/metrics`，可观察低敏 PG pool、联系人申请 / 联系人边状态聚合和 `contacts_outbox` 聚合状态。
- `outbox-relay` 现已对非取消运行时错误做退避重试，并在 relay 模式通过 `/debug/metrics` 暴露低敏 retry 快照；现有 outbox / retry / DLQ 业务语义不变。
- 已补只读 `outbox-audit`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，可直接审计当前 `contacts_outbox`，并按 retention 清理 contacts outbox repair 历史。
- 当 `NEXUSIM_CONTACTS_AUTH_MODE=metadata|verified-metadata` 时，公网监听地址 + 无 gRPC mTLS client cert 的危险组合会在启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。

## 后续

- 联系人分组、搜索、更多隐私策略。
