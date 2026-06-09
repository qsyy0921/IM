# delivery-service delivery outbox smoke

日期：2026-06-09

Commit：`2dd5f5d feat: add delivery outbox relay`

## 目标

验证 `delivery-service` 的本地事务 outbox 已经能通过真实后台进程发布到 Kafka：

```text
delivery_outbox PENDING
-> delivery-service outbox-relay
-> Kafka im.delivery.events
-> delivery_outbox PUBLISHED
```

本轮不是容量压测，只验证最小真实发布链路。

## 拓扑

```text
Windows 本机
-> PostgreSQL Docker: localhost:5432
-> Kafka Docker: localhost:9092
-> delivery-service outbox-relay
-> Kafka topic im.delivery.events
```

使用的输入数据来自上一轮 delivery full smoke 留下的一条有效 `delivery_outbox` PENDING 行：

```text
event_type = delivery.ack.recorded.v1
tenant_id = tenant-delivery
conversation_id = conv-delivery
aggregate_version = 5
```

## 执行过程

确认 Docker 依赖：

```powershell
docker compose -f deploy\local\docker-compose.yml ps
```

创建 Kafka topic：

```powershell
docker exec nexusim-kafka kafka-topics `
  --bootstrap-server localhost:9092 `
  --create --if-not-exists `
  --topic im.delivery.events `
  --partitions 3 `
  --replication-factor 1
```

启动真实 relay：

```powershell
$env:NEXUSIM_DELIVERY_SERVICE_MODE='outbox-relay'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
$env:NEXUSIM_KAFKA_BROKERS='localhost:9092'
$env:NEXUSIM_DELIVERY_EVENTS_TOPIC='im.delivery.events'
$env:NEXUSIM_DELIVERY_OUTBOX_POLL_INTERVAL='200ms'
go run ./services/delivery-service/cmd/delivery-service
```

读取 Kafka 后，用 `schemas/kafka/delivery/v1` 生成的 protobuf Go 类型反序列化 `DeliveryEvent`。

## 结果

结果文件：

```text
loadtest/results/delivery-outbox-smoke-20260609-144110/delivery-outbox-smoke-summary.json
```

PostgreSQL 状态变化：

| 阶段 | PENDING | PUBLISHED | DLQ |
| --- | ---: | ---: | ---: |
| relay 前 | 1 | 0 | 0 |
| relay 后 | 0 | 1 | 0 |

Kafka 解码结果：

```text
partition=1
offset=0
event_id=evt_delivery_ack_9d831183b7b52f75e240f3c3e09bca1c
event_type=delivery.ack.recorded.v1
tenant_id=tenant-delivery
aggregate_id=conv-delivery
aggregate_version=5
payload=*deliveryeventsv1.DeliveryEvent_AckRecorded
ack_user=user-1
ack_device=device-1
ack_seq=5
```

## 判断

通过标准已满足：

- `delivery_outbox` 从 `PENDING` 变为 `PUBLISHED`。
- Kafka `im.delivery.events` 中读到了 protobuf `DeliveryEvent`。
- oneof payload 正确解码为 `AckRecorded`。
- 本链路由 outbox relay 发布，业务事务没有直接 publish Kafka。

## 剩余风险

- 本轮只验证了 `delivery.ack.recorded.v1`；`delivery.inbox_item.created.v1` 仍需在后续 smoke 覆盖。
- malformed / unsupported 的 fail-closed 已有单元测试和 PostgreSQL store 测试，本轮没有再用真实 Kafka 故障场景验证。
- `delivery_outbox` relay 暂无 debug metrics endpoint；后续 push-gateway smoke 至少应记录 outbox total/pending/published/DLQ 和 Kafka read count。
- 本轮报告生成时 push-gateway 尚未实现，因此不能把本轮单独表述为完整在线推送链路；后续报告已验证单实例最小在线通知闭环。

## 面试可讲

这一阶段把 delivery-service 从“只写本地投递事实”推进到“可通过 outbox relay 发布投递事件”。核心点是：

- `AckDelivery` / `ProjectTimelineEvent` 仍只写 PostgreSQL outbox，不直接写 Kafka。
- relay 负责至少一次发布，失败 retry，超限 DLQ。
- Kafka event 只作为在线通知和下游订阅信号，客户端最终仍以 durable inbox 的 `PullInbox` 为准。
