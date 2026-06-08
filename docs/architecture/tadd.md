# NexusIM TADD v1.0

TADD（Technical Architecture Design Document）描述技术栈、工程结构、中间件、部署、观测、压测和编码门禁。ADD 负责系统业务架构，SDD 负责单个服务设计。

## 1. 技术栈

| 模块 | 方案 | 约束 |
| --- | --- | --- |
| 语言 | Go 1.26.4 | 业务服务、网关、outbox relay |
| AI worker | Python | RAG、Embedding、rerank、Agent eval |
| 微服务框架 | Kratos | Go 服务统一框架 |
| 内部通信 | gRPC + Protobuf | 统一 deadline、错误码、幂等语义 |
| 外部 API | HTTP + OpenAPI | 由 api-gateway 适配 |
| 数据访问 | pgx + sqlc | 消息热路径不使用 ORM |
| 事实源 | PostgreSQL | PITR、分区、索引、事务 |
| 事件平台 | Kafka KRaft | 核心事件流 |
| 事件契约 | Schema Registry + Protobuf | 字段兼容治理 |
| 缓存 | Redis | route/counter/cache，第一阶段可单实例 |
| 对象存储 | S3-compatible / MinIO | 文件内容存储 |
| 搜索 | OpenSearch | 搜索投影 |
| 向量 | Milvus / pgvector 起步可选 | RAG embedding |
| 长事务 | Temporal | 审批、补偿、Agent 写动作 |
| 可观测性 | OpenTelemetry + Prometheus + Grafana | trace/metric/log |
| 本地环境 | Docker Compose | PostgreSQL、Kafka、Redis 等基础设施 |

## 2. 六层 DDD 工程结构

所有服务统一使用：

```text
api / app / domain / infrastructure / types / trigger
```

职责：

| 层 | 职责 | 示例 |
| --- | --- | --- |
| `api` | 对外接口适配层 | gRPC handler、HTTP handler、request/response 转换 |
| `app` | 应用用例层 | UseCase、事务编排、调用 domain 和 infrastructure |
| `domain` | 领域规则层 | Message、TimelineEvent、OutboxEvent、幂等规则、状态流转 |
| `infrastructure` | 基础设施实现层 | PostgreSQL、Kafka、Redis、外部 RPC client、sqlc repo |
| `types` | 类型定义层 | Command、DTO、枚举、错误码、常量、跨层轻量类型 |
| `trigger` | 触发器 / 后台任务层 | Outbox Relay、Kafka consumer、定时巡检、补偿任务 |

依赖方向：

```text
api -> app
api -> types
trigger -> app
trigger -> types
app -> domain
app -> infrastructure
app -> types
domain -> types
infrastructure -> domain
infrastructure -> types
```

禁止方向：

```text
domain -> infrastructure
domain -> api
domain -> trigger
infrastructure -> api
infrastructure -> trigger
types -> app/domain/infrastructure/api/trigger
```

说明：

- `infrastructure -> domain` 仅允许基础设施实现 repository / publisher adapter 时使用领域输入、结果或领域对象。
- `domain -> infrastructure` 仍然禁止，领域层不能依赖 SQL、Kafka、Redis 或外部 SDK。
- `app -> infrastructure` 仅用于当前轻量骨架和组合根过渡；正式实现时优先由 `app` 定义 port，由 `cmd`/composition root 注入 infrastructure 实现。

## 3. 仓库目录

目标目录：

```text
api/
  proto/
  openapi/
  asyncapi/
schemas/
  kafka/
migrations/
  postgres/
services/
  message-service/
    cmd/
    internal/
      api/
      app/
      domain/
      infrastructure/
      types/
      trigger/
deploy/
  local/
loadtest/
docs/
  architecture/
    README.md
    target-architecture.md
    add.md
    tadd.md
  sdd/
  runbook/
```

约束：

- 服务之间不能 import 对方的 `internal`。
- 跨服务只通过 Protobuf、Kafka schema 或明确 port interface 协作。
- `pkg` 只能放日志、配置、trace、错误基础设施和测试工具，禁止放业务 domain。
- Go 包名使用 `types`，不使用 `type`，因为 `type` 是 Go 关键字。

## 4. 本地 Docker Compose

第一阶段使用 Docker Compose 启动基础设施：

```text
PostgreSQL
Kafka
Schema Registry
Redis
Kafka UI optional
```

Go 服务前期建议本机直接运行，便于调试；服务骨架稳定后再容器化。

第一阶段端口建议：

| 组件 | 端口 |
| --- | ---: |
| message-service gRPC | 10495 |
| message-service metrics/debug | 10497 |
| PostgreSQL | 5432 |
| Kafka | 9092 |
| Schema Registry | 8081 |
| Redis | 6379 |

本地双机压测端口以 `docs/runbook/local-loadtest.md` 为准。

## 5. PostgreSQL 落地

第一条代码切片需要 migration：

```text
migrations/postgres/message/000001_message_core.sql
```

必须包含：

```text
conversation_seq
message_log
conversation_timeline_events
message_outbox
message_change_history
message_command_idempotency
```

事务边界：

```text
conversation_seq + message_log + conversation_timeline_events + message_outbox
```

数据库变更规则：

```text
expand -> migrate -> contract
```

## 6. Kafka 落地

第一阶段 topic：

```text
conversation.timeline.events
```

Schema：

```text
schemas/kafka/conversation.timeline.events.proto
```

发布规则：

- 业务事务只写 `message_outbox`；
- 不允许业务代码直接 publish Kafka；
- `trigger/outbox` 从 outbox 拉取 pending 事件；
- 发布成功后标记 published；
- 发布失败保留 pending/retry/DLQ 状态；
- Kafka 停机不能影响 SendMessage 落库。

## 7. Proto / OpenAPI / AsyncAPI

已有：

```text
api/proto/nexusim/message/v1/message_service.proto
api/proto/nexusim/message/v1/message_error.proto
schemas/kafka/conversation.timeline.events.proto
```

当前 Go 代码生成基线：

```text
Go 1.26.4
protoc 29.3
protoc-gen-go v1.36.11
protoc-gen-go-grpc 1.6.2
google.golang.org/grpc v1.81.1
google.golang.org/protobuf v1.36.11
```

当前唯一支持的生成入口：

```text
make proto
tools/gen-proto.ps1
```

`api/proto/buf.gen.yaml` 和 `schemas/kafka/buf.gen.yaml` 暂时只作为 Buf 工作流草稿；在统一 Buf 执行目录和输出根之前，不作为当前生成入口。

待补：

```text
api/openapi/
api/asyncapi/
```

第一阶段可以先生成 Go Protobuf 代码，不急于生成完整 OpenAPI 页面。

## 8. Go + Python 融合

Go 负责：

```text
message-service
push-gateway
delivery-service
receipt-service
outbox relay
```

Python 负责：

```text
rag-ingest-service
retrieval-rerank-worker
agent eval
embedding worker
```

融合方式：

```text
Go writes fact source
-> Go outbox relay publishes Kafka
-> Python workers consume Kafka
-> Python writes vector/search/agent projections
-> Python publishes result events
```

Python 不直接写 Go 服务拥有的业务事实表。

## 9. 可观测性

第一阶段必须记录：

```text
trace_id
request_id
message_append_latency
conversation_seq_alloc_latency
db_tx_latency
outbox_pending_count
outbox_oldest_pending_age
kafka_publish_latency
idempotency_hit_count
idempotency_conflict_count
```

日志必须包含：

```text
tenant_id
conversation_id
message_id
client_msg_id
trace_id
request_id
```

## 10. 测试和压测

第一阶段测试门禁：

- 重复 `client_msg_id` 不重复写 `message_log`；
- 同一幂等键 command hash 不同返回 `IDEMPOTENCY_CONFLICT`；
- DB 事务失败不能留下半条消息；
- Kafka 停机时 SendMessage 仍可落库；
- Kafka 恢复后 outbox 能追平；
- 本地压测记录 p95/p99、错误率、outbox pending age。

压测 runbook：

```text
docs/runbook/local-loadtest.md
```

## 11. 当前编码门禁

可以开始编码前必须补：

| 项 | 状态 |
| --- | --- |
| message-service SDD | 已有 |
| proto | 已有 |
| Kafka schema | 已有 |
| PostgreSQL migration | 已有 |
| services/message-service 六层目录 | 已有 |
| deploy/local/docker-compose.yml | 已有 |

因此下一步可以进入：

```text
生成 Protobuf Go 代码
实现 message-service SendMessageUseCase
实现 PostgreSQL repository
实现 trigger/outbox relay
编写集成测试
```
