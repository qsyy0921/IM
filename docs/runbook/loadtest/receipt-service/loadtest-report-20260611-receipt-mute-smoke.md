# receipt-service MuteConversation Smoke

日期：2026-06-11

## 目标

验证 `receipt-service` 的 `MuteConversation` 当前用户列表偏好已经接入真实进程 smoke，并确认它只影响 `ListConversations.muted` 标志，不改变 unread、read cursor、delivery、push 或消息事实。

## 方法

使用标准 receipt smoke runner，在已有链路基础上增加：

```text
SendMessage
-> receipt projection
-> MarkRead
-> SendMessage while archived
-> ArchiveConversation(false)
-> PinConversation(true/false)
-> MuteConversation(true)
-> ListConversations
-> MuteConversation(false)
-> ListConversations
```

执行命令：

```powershell
. .\tools\go-env.ps1
.\loadtest\receipt\run-local-smoke.ps1 -RunName receipt-mute-smoke-clean-20260611-171611
```

原始结果：

```text
H:\NexusIM\loadtest-results\receipt-mute-smoke-clean-20260611-171611\receipt-summary.json
```

## 结果

| 指标 | 结果 |
| --- | --- |
| commit | `cd429d9` |
| git_dirty | `false` |
| success | `true` |
| after mute | `muted=true` |
| after unmute | `muted=false` |
| last_visible_seq | `3` |
| unread_count | `1` |
| last_read_seq | `2` |
| archived | `false` |
| pinned | `false` |
| receipt_outbox | `total=3 / published=3 / pending=0 / dlq=0` |
| delivery_outbox | `total=4 / published=4 / pending=0 / dlq=0` |

关键解释：

- `last_read_seq=2` 来自显式 `MarkRead`。
- 归档期间新消息推进 `last_visible_seq=3`，并使 `unread_count=1`。
- `MuteConversation(true/false)` 只改变 `muted`，没有清未读、没有推进 read cursor、没有插入新的 receipt outbox event。

## 结论

`MuteConversation` 已作为 receipt-service 内的当前用户列表偏好落地。它适合在客户端列表 UI 或后续通知策略中作为输入，但当前阶段不是推送静音闭环，也不是 delivery 事实变更。

部署注意：`ListConversations` 已读取 `user_conversation_summaries.muted`，因此升级服务前需要先执行 `migrations/postgres/receipt/000007_conversation_mute.sql`。

## 限制

- 未验证真正 push suppression。
- 未接入真实 policy / AuthContext。
- 未做多设备产品语义、客户端 UI 或容量压测。
- 本报告只证明最小列表偏好链路，不改变 receipt-service 的可靠回执边界。
