# Hot Group Sequencer Member Boundary Diagnostic

日期：2026-06-30

本报告记录一次 pre-commit diagnostic。它用于解释中等规模热点群压测失败原因和修复方向，
不是正式容量结论。正式压测报告必须绑定 clean commit 后重跑。

## 背景

在三机 Docker 链路上扩大到 200 人群、16 sender、500 messages 时，runner 初始失败：

```text
join member hot-user-000045: rpc error: code = Unavailable desc = sequencer unavailable
```

原因不是 loadtest 造数错误，而是 conversation 被提升到：

```text
conversation_mode = SEQUENCER_BLOCK
fanout_mode = BROADCAST_SIGNAL
```

之后，message-service 的 `SendMessage` 已经接 timeline-service seq block，但
conversation-service 的成员 JOIN / LEAVE / REMOVE / owner transfer 仍只支持本地
`LOCAL_ROW_LOCK` 成员边界。扩容造群需要继续写成员边界 timeline event，因此在热点会话模式下
fail-closed。

## 修复方向

conversation-service 在 `SEQUENCER_BLOCK` 会话中：

- 通过 timeline-service `AllocateSeqBlock` 为成员边界获取单 seq lease；
- 以当前 `conversation_timeline_events` 最大 seq + 1 作为 minimum floor；
- 校验 `epoch / lease_id / expires_at`；
- 未配置 sequencer、lease 无效或 lease 过期时 fail-closed；
- 不回退到本地 row lock。

这保持了热点会话“消息 seq”和“成员边界 seq”共享同一个 timeline sequencer 边界。

## 诊断复验

原始目录：

```text
H:\NexusIM\loadtest-results\hotgroup-medium-seq-member-20260630-001157
```

运行状态：

```text
commit = b2b6cbe
git_dirty = true
```

由于该运行发生在未提交工作区，因此只作为诊断证据。

关键结果：

```text
group_size = 200
sender_count = 16
message_count = 500
actual_fanout_mode = BROADCAST_SIGNAL
conversation_mode = SEQUENCER_BLOCK
send_success_count = 500
send_error_count = 0
send_p95_ms = 10.736
send_p99_ms = 13.07
conversation_subscribers = 20
conversation_signal_count = 10000
receiver_pull_success = 20
receiver_ack_success = 16
delivery_timeline_rows = 500
user_inbox_rows = 0
delivery_outbox_rows = 516
delivery_outbox_pending = 0
delivery_outbox_dlq = 0
Kafka conversation.timeline.events lag = 0
Kafka im.delivery.events lag = 0
```

## 结论

本次诊断证明：成员边界 sequencer 修复后，中等规模热点群可以完成造群、发送、
conversation signal、PullInbox / ACK 和 delivery outbox drain。

它不证明容量上限。下一步必须：

1. 提交当前修复，得到 clean commit；
2. 以该 commit 重建 Docker 镜像并 redeploy；
3. 重跑小 / 中规模热点群复验；
4. 再扩大压测并记录 Grafana / Prometheus 趋势、Kafka lag、projection lag、push signal、
   PullInbox / ACK 和 PostgreSQL 指标。
