# contacts-service

## 当前状态

- 已有好友申请、列表、接受、拒绝、取消、删除、拉黑、解除拉黑、备注。
- contacts-service 是联系人事实源。
- policy-service 通过 contacts event projection 做 direct block 决策。
- 已补只读 `outbox-audit`、只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，可直接审计当前 `contacts_outbox`，并按 retention 清理 contacts outbox repair 历史。

## 后续

- 联系人分组、搜索、更多隐私策略。
