# NexusIM SDD Index

本文记录服务级 SDD 的完成状态和编码门禁。

## 文档关系

| 文档 | 路径 | 作用 |
| --- | --- | --- |
| ADD | `docs/architecture/add.md` | 系统级业务架构、服务边界、核心流程和阶段路线 |
| TADD | `docs/architecture/tadd.md` | 技术栈、六层 DDD、工程目录、中间件、部署、观测和编码门禁 |
| SDD Template | `docs/sdd/TEMPLATE.md` | 后续服务级 SDD 的统一模板 |
| message-service SDD | `docs/sdd/message-service.md` | 第一条可编码切片的服务设计 |

## 六层 DDD 约定

所有服务统一使用六层 DDD 架构：

| 层 | 职责 | 示例 |
| --- | --- | --- |
| `api` | 对外接口适配层 | gRPC handler、HTTP handler、request/response 转换 |
| `app` | 应用用例层 | UseCase、事务编排、调用 domain 和 infrastructure |
| `domain` | 领域规则层 | 聚合、实体、领域事件、幂等规则、状态流转 |
| `infrastructure` | 基础设施实现层 | PostgreSQL、Kafka、Redis、外部 RPC client、sqlc repo |
| `types` | 类型定义层 | Command、DTO、枚举、错误码、常量、跨层轻量类型 |
| `trigger` | 触发器 / 后台任务层 | Outbox Relay、Kafka consumer、定时巡检、补偿任务 |

## 当前状态

| 服务 / 设计 | 状态 | 对编码的影响 |
| --- | --- | --- |
| `message-service` | SDD 已冻结 v1.0 | 可以开始 `SendMessage` 普通会话写入链路 |
| `timeline-service / sequencer` | SDD 未完成 | 不阻塞 `LOCAL_ROW_LOCK`；阻塞热点会话生产实现 |
| `conversation-service / member_change_saga` | SDD 未完成 | 不阻塞 strict mock；阻塞真实成员变更和 ACL 投影 |
| `push-gateway` | SDD 未完成 | 不阻塞 `message-service`；阻塞 WebSocket 完整闭环 |
| `delivery-service` | SDD 未完成 | 不阻塞 `message-service`；阻塞 fanout、offline pull、ACK 闭环 |
| `retrieval-gateway` | SDD 未完成 | 不进入第一条代码切片 |

## 第一条可编码切片

当前可以开始的代码范围：

```text
message-service SendMessage
-> PostgreSQL local transaction
-> conversation_seq
-> message_log
-> conversation_timeline_events
-> message_outbox
-> outbox relay
-> Kafka publish path
```

边界：

- 只实现普通会话 `LOCAL_ROW_LOCK`。
- `policy-service`、`conversation-service`、`timeline-service` 使用 strict mock port。
- 不实现热点 sequencer。
- 不实现 delivery、push、RAG、Agent。
- 不绕过 outbox 直接发布 Kafka。

## 编码前必须补齐

| 文件 | 状态 | 说明 |
| --- | --- | --- |
| `api/proto/nexusim/message/v1/message_service.proto` | 已存在 | 需要生成 Go 代码 |
| `api/proto/nexusim/message/v1/message_error.proto` | 已存在 | 需要生成 Go 代码 |
| `schemas/kafka/conversation.timeline.events.proto` | 已存在 | 需要生成 Go 代码或接入发布侧序列化 |
| `migrations/postgres/message/000001_message_core.sql` | 已存在 | 可进入本地事务实现 |
| `services/message-service` 六层目录 | 已存在 | 可进入服务实现 |
| `deploy/local` 基础设施 | 已存在 | 可启动本地 PostgreSQL / Kafka / Schema Registry / Redis |

## 下一批 SDD

优先级：

1. `timeline-service-sequencer.md`
2. `conversation-service-member-change-saga.md`
3. `delivery-service.md`
4. `push-gateway.md`
5. `retrieval-gateway.md`

其中 `timeline-service` 与 `conversation-service` 的 SDD 不阻塞第一条 `SendMessage` 普通会话链路，但必须在热点会话、成员变更、成员边界投递和 ACL 投影生产化前冻结。
