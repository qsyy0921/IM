# receipt-service Unread Conversation Filter Smoke

日期：2026-06-11

## 目标

验证 `ListConversations(unread_only=true)` 已接入 receipt-service 真实进程链路，并确认它是基于 `user_conversation_summaries.unread_count > 0` 的列表过滤，不改变 read cursor、unread 计算、delivery 或 receipt outbox 语义。

## 方法

使用标准 receipt smoke runner，在既有链路中增加两次未读列表查询：

```text
SendMessage
-> delivery AckDelivery
-> receipt projection
-> ListConversations(unread_only=true)
-> MarkRead
-> ListConversations(unread_only=true)
```

执行命令：

```powershell
. .\tools\go-env.ps1
.\loadtest\receipt\run-local-smoke.ps1 -RunName receipt-unread-filter-smoke-clean-20260611-183000
```

原始结果：

```text
H:\NexusIM\loadtest-results\receipt-unread-filter-smoke-clean-20260611-183000\receipt-summary.json
```

## 结果

| 指标 | 结果 |
| --- | --- |
| commit | `22ed67f` |
| commit_full | `22ed67f446f0b47708a43b1d0100736e6adb1a96` |
| git_dirty | `false` |
| success | `true` |
| unread before read | `item_count=1 / unread_count=1 / last_read_seq=0 / last_visible_seq=2` |
| after MarkRead | `last_read_seq=2 / unread_count=0` |
| unread after read | `item_count=0` |
| archived included after new message | `archived=true / unread_count=1 / last_read_seq=2 / last_visible_seq=3` |
| receipt_outbox | `total=3 / published=3 / pending=0 / dlq=0` |
| delivery_outbox | `total=4 / published=4 / pending=0 / dlq=0` |

## 结论

`unread_only` 已作为 `ListConversations` 的低耦合过滤条件落地。它复用 receipt-service 自己的 `user_conversation_summaries.unread_count`，没有新增服务、没有读取 delivery-service 内部表，也没有改变 `MarkRead` 或 receipt outbox 语义。

实现上，`unread_only` 进入 SQL `WHERE unread_count > 0`，并写入分页 cursor。这样普通列表 cursor 不能用于未读列表，未读列表 cursor 也不能用于普通列表，避免分页条件切换导致漏数据。

## 限制

- 本轮验证的是功能 smoke，不是未读列表容量压测。
- 当前权限仍是第一阶段本地访问控制，真实 policy / AuthContext 仍需后续接入。
- `unread_only` 是过滤，不是 unread-first 排序；默认排序仍是 pinned-first + updated_at。
