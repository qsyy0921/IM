# contacts-service

## 当前状态

- 已有好友申请、列表、接受、拒绝、取消、删除、拉黑、解除拉黑、备注。
- contacts-service 是联系人事实源。
- policy-service 通过 contacts event projection 做 direct block 决策。
- 已补只读 `outbox-repair-audit` 运维模式，可直接审计 contacts outbox repair 历史，不改当前 outbox 状态。

## 后续

- 联系人分组、搜索、更多隐私策略。
