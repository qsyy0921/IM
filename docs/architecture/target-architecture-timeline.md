## 5. 消息写入与 Timeline

### 5.1 普通会话写入

普通会话保持最短本地事务链路：

```text
api-gateway
-> message-service validate auth and permission projection
-> PostgreSQL tx:
     allocate conversation_seq by row lock
     insert message_log
     insert conversation_timeline_events
     insert message_outbox
-> return message_id + conversation_seq
-> outbox relay publishes message.persisted to Kafka
```

该模式保住核心不变量：

```text
message_log、timeline、outbox 不出现跨服务半成功。
```

### 5.2 热点会话写入

热点会话由 timeline-service 提供 sequencer control-plane：

```text
hot conversation detected
-> timeline-service leader owns conversation
-> leader pre-allocates seq block
-> message-service gets seq from local block cache
-> message-service tx writes message_log + timeline + outbox
```

Seq 模式状态机：

```text
LOCAL_ROW_LOCK
-> PROMOTING_TO_SEQUENCER
-> SEQUENCER_BLOCK
-> DEMOTING_TO_LOCAL
-> LOCAL_ROW_LOCK
```

升级协议：

```text
control-plane marks conversation PROMOTING_TO_SEQUENCER
-> message-service queues or rate-limits this conversation
-> timeline-service reads current next_seq
-> timeline-service creates sequencer_epoch
-> timeline-service allocates first seq block
-> control-plane switches mode to SEQUENCER_BLOCK
-> message-service refreshes block cache
```

降级协议：

```text
control-plane marks DEMOTING_TO_LOCAL
-> stop issuing new seq block
-> drain message-service block cache
-> mark unused seq as gap marker
-> persist next_seq to conversation_seq
-> switch mode to LOCAL_ROW_LOCK
```

timeline-service 不接管普通消息事务，只负责：

- 热点识别；
- sequencer leader election；
- seq block 分配；
- epoch fencing；
- gap marker；
- owner 切换审计。

Sequencer 当前目标实现：

| 能力 | 技术 |
| --- | --- |
| 选主 | Kubernetes Lease |
| fencing | PostgreSQL `sequencer_epoch` |
| seq block 状态 | PostgreSQL |
| 热点状态缓存 | Redis counter/cache |
| leader 审计 | audit-service |

Seq 规则：

- 允许有系统解释的 gap，不允许乱序。
- leader 崩溃后未使用 seq 作废，并写 `TimelineGapMarker`。
- 客户端遇到 gap marker 不阻塞后续展示。
- 补拉接口返回 gap marker，客户端不重试不存在的 seq。

### 5.3 Timeline Append / Publish 顺序

所有进入 `conversation.timeline.events` 的事件必须共享同一 conversation 顺序轴：

```text
message.persisted / edited / revoked / deleted
conversation.member.joined / left / role_changed
boundary event
gap marker
repair event
```

目标态原则：

```text
所有 conversation timeline event 必须经过同一个 append / publish ordering mechanism。
```

允许的实现路线：

| 方案 | 说明 | 适用性 |
| --- | --- | --- |
| A | conversation-service 只提交 boundary command，最终由 timeline authority 统一写 timeline + outbox | 推荐用于热点和成员边界复杂场景 |
| B | 多服务各写 outbox，但共享 `conversation_timeline_publish_cursor` 按 `aggregate_version` 全局发布 | 可用但治理复杂 |
| C | 所有 timeline event 进入同一张 `conversation_timeline_events` 和同一条 outbox 流 | 推荐用于第一批生产化 |

当前普通消息 timeline event 仍由 message-service 写入并通过 outbox 保护顺序。成员边界、gap marker、repair event 和后续更多 producer 生产化前，必须显式选择统一 timeline append / publish 机制，并证明同 conversation 顺序不被破坏。

热点 seq 分配流水：

```text
seq_allocation_journal:
  tenant_id
  conversation_id
  sequencer_epoch
  seq
  allocation_id
  allocated_to
  status: ALLOCATED / COMMITTED / GAP_MARKED
  allocated_at
  committed_at
  gap_marked_at
  reason
```

约束：

- message-service 从 block cache 取 seq 时写 `ALLOCATED`。
- 本地事务提交成功后标记 `COMMITTED`。
- 事务失败、实例崩溃、block 作废时标记 `GAP_MARKED` 并写 gap marker。
- 巡检任务告警长时间停留在 `ALLOCATED` 的 seq。
- journal 用于证明无重复 seq、无未解释 gap。

### 5.4 消息变更

编辑、撤回、删除流程：

```text
operator request
-> message-service validate permission
-> PostgreSQL tx: update message_log + message_change_history + timeline + outbox
-> Kafka: message.edited / revoked / deleted
-> delivery updates inbox visibility
-> search updates/deletes OpenSearch document
-> rag deletes/rebuilds chunks and vectors
-> audit appends immutable record
```

删除和撤回对用户侧只返回 tombstone，不返回旧正文。

## 6. 成员边界与 Fanout

### 6.1 成员变更 Saga

成员事实由 conversation-service 拥有。成员边界必须进入 timeline，并通过显式 Saga 管理失败窗口。

`member_change_saga`：

```text
change_id
tenant_id
conversation_id
user_id
change_type
boundary_seq
status
idempotency_key
expected_member_version
command_hash
operator_id
conflict_policy
retry_count
last_error
created_at
updated_at
```

状态机：

```text
PENDING_BOUNDARY
-> BOUNDARY_ALLOCATED
-> MEMBER_UPDATED
-> EVENT_PUBLISHED
-> DONE

any state -> FAILED_COMPENSATED
```

协议：

```text
conversation-service receives member command
-> create member_change_saga(PENDING_BOUNDARY)
-> allocate boundary_seq
-> update conversation_members(join_seq / leave_seq / permission_version)
-> publish conversation.member.*
-> update search/rag ACL projection
-> audit
```

失败补偿：

| 失败点 | 补偿 |
| --- | --- |
| boundary 分配失败 | saga 失败，成员表不变 |
| boundary 已分配但成员更新失败 | 写 boundary cancelled，审计失败原因 |
| 成员已更新但事件发布失败 | outbox 重试，超限进 DLQ |
| ACL 投影失败 | retrieval-gateway 进入 `strict_acl_mode` 回源校验 |

并发规则：

- 同一 `idempotency_key` 重试返回同一 `change_id`。
- 同一 `conversation_id + user_id` 的成员变更串行化。
- `conversation_members.member_version` 做乐观并发控制。
- 加入中又退出、退群中又改角色时，按 `conflict_policy` 拒绝、合并或补偿。
- 所有冲突结果写入 saga 和 audit。

### 6.2 Fanout 状态机

每条 timeline event 固化 `fanout_mode` 和 `fanout_policy_version`，保证重放、审计和投递异常排查可解释。

```text
WRITE_FANOUT -> HYBRID_FANOUT -> READ_FANOUT -> BROADCAST_SIGNAL
```

| 模式 | 行为 |
| --- | --- |
| WRITE_FANOUT | 为所有目标成员写 user_inbox |
| HYBRID_FANOUT | 活跃成员写 inbox，非活跃成员按 timeline 补拉 |
| READ_FANOUT | 不做全量 inbox 写扩散，客户端按 timeline 拉取 |
| BROADCAST_SIGNAL | 只推送新消息信号，内容按需拉取 |

切换条件：

```text
member_count
active_member_count
msg_qps_1m
fanout_lag_seconds
inbox_write_amplification
push_success_rate
redis_hot_key_score
delivery_consumer_lag
```

新 mode 只影响新 timeline event，旧 event 按固化 mode 继续处理。

## 7. 数据模型

核心表：

| 表 | 主键/唯一键 | 职责 |
| --- | --- | --- |
| conversation_seq | `tenant_id + conversation_id` | 普通会话 seq |
| conversation_sequencer_state | `tenant_id + conversation_id` | 热点会话 owner、epoch、seq block |
| seq_allocation_journal | `tenant_id + conversation_id + seq` | 热点 seq 分配、提交、gap 标记流水 |
| timeline_seq_gap_markers | `tenant_id + conversation_id + gap_start` | timeline-service owned seq gap 解释 |
| message_log | `tenant_id + conversation_id + conversation_seq` | 消息事实源 |
| conversation_timeline_events | `tenant_id + conversation_id + seq` | 会话顺序轴 |
| message_outbox | `event_id` | 待发布事件 |
| conversation_members | `tenant_id + conversation_id + user_id` | 成员、join_seq、leave_seq、permission_version |
| member_change_saga | `change_id` | 成员边界 Saga |
| conversation_fanout_state | `tenant_id + conversation_id` | fanout mode |
| user_inbox | `tenant_id + user_id + conversation_id + seq` | durable delivery index |
| device_delivery_cursors | `tenant_id + user_id + device_id + conversation_id` | 设备 ACK |
| conversation_read_cursors | `tenant_id + user_id + conversation_id` | 用户已读 |
| control_plane_configs | `config_key + version` | 控制面配置版本 |
| control_plane_rollouts | `rollout_id` | 策略灰度和回滚状态 |
| control_plane_applied_versions | `service_name + instance_id + config_key` | 服务实例已应用配置版本 ACK |
| kafka_partition_mappings | `topic + mapping_version + virtual_partition` | virtual partition 到 physical partition 映射 |
| acl_relation_tuples | `tenant_id + subject + relation + resource` | ReBAC 事实 |
| acl_projection_versions | `tenant_id + resource_id` | 索引/向量权限投影版本 |
| rag_chunks | `tenant_id + chunk_id` | chunk 元数据和向量归因 |
| delete_proofs | `tenant_id + delete_proof_id` | 删除证明 |
| audit_logs | `tenant_id + audit_id` | 审计记录 |
| audit_manifests | `tenant_id + manifest_date` | 审计 hash manifest |
| tenant_budgets | `tenant_id + budget_type` | 租户预算 |
| ai_eval_datasets | `dataset_id + version` | AI safety 评测集版本 |
| ai_eval_runs | `run_id` | 评测执行记录 |

关键约束：

- 所有业务表必须包含 `tenant_id`。
- `message_log` 唯一键：`tenant_id + sender_id + device_id + client_msg_id`，并保存 `command_hash` 用于判断重复请求是否语义一致。
- `client_msg_id` 是 device scoped globally unique UUID，同一 `tenant_id + sender_id + device_id` 下不能跨会话复用。
- `conversation_seq` 由 conversation-service 创建会话时初始化；message-service 只允许幂等兜底补建并记录 metric / repair log。
- `conversation_timeline_events` 唯一键：`tenant_id + conversation_id + seq`。
- outbox relay 使用 `FOR UPDATE SKIP LOCKED`。
- outbox relay 对同一 `tenant_id + conversation_id` 必须按 `aggregate_version` 严格发布；存在更小版本的 `PENDING` 或 `DLQ` 事件时，不允许发布后续事件。
- 消费者先完成持久化副作用，再提交 Kafka offset。
- Redis 中的状态必须能从 PostgreSQL/Kafka 重建。

## 8. Kafka 事件平台

Kafka 使用 KRaft 模式，核心配置：

```text
replication.factor = 3
min.insync.replicas = 2
producer.acks = all
producer.enable.idempotence = true
schema compatibility = BACKWARD_TRANSITIVE
shared long-lived events = FULL_TRANSITIVE
```

Topic 规划：

| Topic | 分区键 | 保留 | 顺序 |
| --- | --- | --- | --- |
| conversation.timeline.events | `tenant_id + conversation_id` | 14 到 30 天 | 同会话严格有序 |
| im.delivery.events | `tenant_id + conversation_id` | 7 天 | 同会话有序 |
| im.receipt.events | `tenant_id + conversation_id` | 3 到 7 天 | 可按游标压缩 |
| media.asset.events | `tenant_id + asset_id` | 14 天 | 允许局部乱序 |
| agent.workflow.events | `tenant_id + agent_job_id` | 30 天 | 允许局部乱序 |
| audit.repair.events | `tenant_id + repair_id` | 长期 | 可重放 |

分区数不写死，按公式确定：

```text
partition_count = max(
  peak_topic_throughput / safe_throughput_per_partition,
  required_consumer_parallelism
)
```

建议容量档：

```text
conversation.timeline.events: 512 / 1024 / 2048
im.delivery.events: 512 / 1024
im.receipt.events: 256 / 512
```

核心 timeline topic 使用自定义分区器：

```text
virtual_partition = hash(tenant_id + conversation_id) % virtual_partition_count
physical_partition = virtual_partition_mapping[virtual_partition]
```

映射由 control-plane 管理：

```text
mapping_version
virtual_partition
physical_partition
status: ACTIVE / MIGRATING / DRAINING / ROLLED_BACK
rollout_scope
created_by
approved_by
```

约束：

- 不依赖 Kafka 默认分区策略。
- `conversation.timeline.events` 上线后不随意扩分区。
- 扩容通过 virtual partition 映射迁移，避免同一 conversation 新旧事件落点不稳定。
- producer 热加载 `mapping_version`，每条 timeline event 带 `mapping_version`。
- mapping 只追加新版本，不原地覆盖旧版本。
- 迁移期间一个 virtual partition 只能有一个 active physical partition。
- consumer lag 清零且 checksum 通过后，才能完成映射切换。
- 迁移失败回滚到上一 `mapping_version`。
- DLQ replay 必须带 `replay_id`、限速、审计，并遵守同会话 `aggregate_version` 顺序保护。

事件 envelope：

```json
{
  "event_id": "evt_01J...",
  "event_type": "message.persisted",
  "event_version": "1.0.0",
  "tenant_id": "t_001",
  "aggregate_type": "conversation",
  "aggregate_id": "conv_123",
  "aggregate_version": 1024,
  "partition_key": "t_001:conv_123",
  "trace_id": "otel-trace-id",
  "correlation_id": "req_01J...",
  "causation_id": "cmd_01J...",
  "payload": {}
}
```

timeline 事件 payload / metadata 必须携带：

```text
fanout_mode
fanout_policy_version
permission_version
classification
mapping_version
```

Replay Source Policy：

```text
事实以 PostgreSQL 为准，传播回放优先 Kafka。
```

| 场景 | 回放源 |
| --- | --- |
| Kafka 保留期内，下游投影损坏 | Kafka replay |
| Kafka 超出保留期 | PostgreSQL fact source |
| Kafka 事件疑似污染 | PostgreSQL fact source + audit.repair.events |
| message_log 被人工修复 | PostgreSQL fact source + audit.repair.events |
| search / rag 重建 | Kafka 优先，不足时回源 PostgreSQL |
| 审计复核 | audit_log + fact source |

## 9. Redis 与长连接

Redis 按逻辑角色拆成三类；物理上可以先共用本地 Redis，也可以按生产 ADR 拆成独立集群：

| 集群 | 用途 | 降级策略 |
| --- | --- | --- |
| redis-route | WebSocket 连接路由、在线状态、session 映射 | 客户端重连恢复 |
| redis-counter | 限流、receipt 聚合、未读热点、fanout 热点 | 降低刷新频率，保留主链路 |
| redis-cache | 权限缓存、预算缓存、检索缓存 | 缓存 miss 回源 |

生产形态不建议一个 Redis 集群承载所有热状态，避免未读和限流热点拖垮连接路由；本地开发 / smoke 可用单 Redis namespace 简化。

push-gateway 只负责：

```text
connect
authenticate
heartbeat
register route
online push
slow connection eviction
disconnect cleanup
```

WebSocket 基础帧：

```json
{
  "op": "message.send",
  "request_id": "req_01",
  "client_msg_id": "cm_01",
  "conversation_id": "conv_1",
  "payload": {}
}
```

客户端约束：

```text
client_msg_id 必须是同一 device_id 下全局唯一 UUID，不能只按 conversation 维度递增或复用。
```

服务端接受：

```json
{
  "op": "message.accepted",
  "request_id": "req_01",
  "message_id": "msg_01",
  "conversation_seq": 1024
}
```

断线恢复：

```text
client reconnects with resume_token
-> push-gateway verifies session
-> client reports last_received_seq
-> delivery-service returns missing range
-> client de-duplicates by message_id and seq
-> client sends delivery.ack after local durable write
```

短断线优先走 push resume buffer：

```text
push-gateway keeps last N seconds / N messages unacked push buffer per session
client reconnects within server_push_buffer_window
-> resume from push buffer
else
-> recovery to delivery-service pull
```

约束：

- push buffer 只提升体验，不是事实源。
- buffer 丢失不影响补拉正确性。
- `server_push_buffer_window` 由 control-plane 按租户和客户端类型配置。

客户端连接状态机：

```text
DISCONNECTED
-> CONNECTING
-> AUTHENTICATING
-> CONNECTED
-> RESUMING
-> SYNCING
-> READY
-> DEGRADED
```

本地消息状态：

```text
LOCAL_PENDING
-> ACCEPTED
-> DELIVERED
-> READ

LOCAL_PENDING -> FAILED_RETRYABLE -> LOCAL_PENDING
LOCAL_PENDING -> FAILED_FINAL
```

标准错误码：

| 错误码 | 客户端动作 |
| --- | --- |
| AUTH_EXPIRED | 刷新 token 后重连 |
| DEVICE_REVOKED | 清理 session 并退出登录 |
| RATE_LIMITED | 按 `retry_after_ms` 退避 |
| CONVERSATION_NOT_FOUND | 停止重试并刷新会话 |
| PERMISSION_DENIED | 停止重试 |
| SEQ_GAP | 触发补拉 |
| SERVER_BUSY | 指数退避 |
| RETRY_AFTER | 使用服务端退避时间 |

