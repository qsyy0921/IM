# timeline-service SDD v0.1

## 目标

`timeline-service` 是热点会话 sequencer 的 owner。第一阶段只实现 seq block
allocator：按 `tenant_id + conversation_id` 原子分配一段连续 `conversation_seq`，
把 lease、幂等键和分配边界写入 timeline-service 自有 PostgreSQL 表。

## 边界

- 拥有：`timeline_sequence_state`、`timeline_seq_block_leases`。
- 提供：`nexusim.timeline.v1.TimelineService/AllocateSeqBlock`。
- 不拥有：消息正文、message facts、conversation members、durable inbox、delivery
  cursor、push session。
- 不发布 Kafka；后续 gap marker / repair event 另行设计。
- 不允许隐藏 fallback。message-service 在没有 valid seq block lease 时必须
  fail-closed，不能悄悄回退成 `LOCAL_ROW_LOCK`。

## API

`AllocateSeqBlock` request:

```text
tenant_id
conversation_id
requester_id
block_size
idempotency_key
```

response:

```text
start_seq
end_seq
block_size
sequencer_epoch
lease_id
expires_at_unix_ms
idempotent_replay
```

同一 `tenant_id + conversation_id + requester_id + idempotency_key` 重试必须返回同一
lease；同一幂等键携带不同 request hash 时返回 idempotency conflict。

## PostgreSQL

`timeline_sequence_state` 保存每个会话的 `next_seq` 和 `sequencer_epoch`。
`timeline_seq_block_leases` 保存已分配 block、requester、幂等键、command hash 和
lease 过期时间。

分配事务顺序：

1. `FOR UPDATE` 锁定已有幂等 lease；存在且 hash 相同则 replay。
2. 不存在时插入或读取 `timeline_sequence_state`。
3. `FOR UPDATE` 锁定该 conversation state。
4. 按 `block_size` 推进 `next_seq`。
5. 插入 `timeline_seq_block_leases`。
6. commit。

## 当前实现状态

- `NEXUSIM_TIMELINE_SERVICE_MODE=seq-block-allocator` 已可启动。
- 本地 Docker 运行链路默认容器为 `timeline-service-seq-block-allocator`。
- message-service 已通过 `NEXUSIM_TIMELINE_SERVICE_ADDR` 接入第一阶段 active
  `SEQUENCER_BLOCK` 写路径：每条消息申请 `block_size=1`，拿到 valid lease 后写
  message facts；未配置或 lease 无效时 fail-closed。
- 这仍不是完整热点 sequencer：本地 block cache、gap marker、epoch fencing、
  virtual partition mapping 和 repair operator 仍在后续。

## 后续

- message-service local block cache / journal。
- sequencer epoch fencing 和 leader ownership audit。
- gap marker 和 gap repair operator。
- virtual partition mapping 与 control-plane rollout。
- seq block / gap marker Prometheus metrics。
