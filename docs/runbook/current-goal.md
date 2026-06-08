# NexusIM Current Goal

本文是 Codex Goal 的持续入口。每轮工作开始时先执行 `git status --short`，然后读取本文，再决定本轮行动。

## 1. 当前目标

持续推进 `E:\development\IM` 的 NexusIM 项目落地，当前阶段聚焦：

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

## 2. 硬边界

- 项目统一命名为 `NexusIM`，不再使用旧项目名。
- 每个微服务独立使用六层 DDD，不做全局统一 DDD。
- 微服务内固定六层：`api / app / domain / infrastructure / types / trigger`。
- 根目录 `api/` 只放全局接口契约；`services/<service>/internal/api/` 才是服务内部接口适配实现。
- 第一阶段只实现普通会话 `LOCAL_ROW_LOCK` 的 `SendMessage` 主链路。
- Kafka 事件只能通过 outbox relay 发布，业务事务不能直接 publish Kafka。

## 3. 暂不实现

- 完整 WebSocket / push / delivery 闭环。
- 桌面客户端完整 UI。
- RAG / Agent 正式业务能力。
- 热点会话 sequencer 生产逻辑。
- `EditMessage`、`RevokeMessage`、`DeleteMessage` 业务实现。

## 4. 当前事实

| 项 | 状态 |
| --- | --- |
| 目标态总架构 | `docs/architecture/target-architecture.md` 已存在 |
| ADD | `docs/architecture/add.md` 已存在 |
| TADD | `docs/architecture/tadd.md` 已存在 |
| message-service SDD | `docs/sdd/message-service.md` 已冻结 |
| Proto 契约 | `api/proto/nexusim/message/v1/` 已存在 |
| Kafka schema | `schemas/kafka/conversation.timeline.events.proto` 已存在 |
| PostgreSQL migration | `migrations/postgres/message/000001_message_core.sql` 已存在 |
| Docker Compose | `deploy/local/docker-compose.yml` 已存在 |
| message-service 六层骨架 | `services/message-service/internal/{api,app,domain,infrastructure,types,trigger}` 已存在 |
| Go 工具链 | 项目基线为 Go `1.26.4`；已通过阿里云镜像安装到 `C:\Users\10495\.local\go\go1.26.4\bin\go.exe`；`protoc-gen-go v1.36.11` 和 `protoc-gen-go-grpc 1.6.2` 已安装到 `C:\Users\10495\go\bin`；`protoc` 可用，路径为 `C:\Users\10495\anaconda3\Library\bin\protoc.exe`；本地命令先执行 `. .\tools\go-env.ps1` |
| Proto Go 代码 | 已生成 `api/proto/nexusim/message/v1/*.pb.go` 和 `schemas/kafka/conversation.timeline.events.pb.go` |
| Go 依赖 | `go.mod` 使用 Go `1.26.4`，并已引入 `google.golang.org/grpc v1.81.1`、`google.golang.org/protobuf v1.36.11` |
| SendMessage app/domain | 已补 `SendMessageUseCase` 单元测试、permission version 一致性短重试、稳定 JSON canonical command hash、append record 构造 |
| PostgreSQL repository | 已实现普通会话 `SendMessage` 本地事务：幂等检查、同幂等键 advisory transaction lock、`conversation_seq` row lock、`message_log`、`conversation_timeline_events`、`message_outbox` 同事务写入；outbox payload 对齐 `MessagePersistedV1` 业务 payload；集成测试和并发重复请求测试通过 |
| Outbox relay / Kafka publish path | 已实现 `trigger/outbox` 最小 relay、PostgreSQL outbox store、Kafka writer producer；真实 PostgreSQL + Kafka 集成测试通过 |

## 5. 下一步优先级

1. 当前 Codex 进程如果仍找不到 `go`，先执行 `. .\tools\go-env.ps1`。
2. 补 `message-service` gRPC adapter，把 proto `SendMessage` 转换到 app command。
3. 补本地多线程 SendMessage 压测入口。
4. 用真实 message-service 进程压 `SendMessage -> outbox -> Kafka` 链路。
5. 视评审结果补强 outbox relay 工程化细节，例如多 worker 锁、显式 idempotency reservation 或 observability。

## 6. 评审要求

重要变更完成后，邀请独立评审线程评审。

当前主工作线程 ID：

```text
019ea0b0-69fd-7be1-9ec4-4b3c6247e36d
```

独立评审线程 ID：

```text
019ea124-dab1-71f2-964b-f5cb8d219aa2
```

评审结果回传规则：

- 优先回传到 delegation 的 `source_thread_id`。
- 如果没有 delegation 元数据，则回传到当前主工作线程 ID。
- 如果主线程迁移或新建，必须同步更新本文。

发送给评审线程的信息必须包含：

- 本轮目标。
- 修改文件列表。
- 核心设计选择。
- 六层 DDD 是否受影响。
- ADD / TADD / SDD 是否同步。
- 已执行检查和结果。
- 已知风险。
- 希望评审重点。

收到评审意见后：

- 阻塞问题必须修复后再继续编码。
- 非阻塞问题记录到本文或对应 runbook。
- 合理建议需要同步到文档或代码。

## 7. 压测要求

第一阶段压测只验证真实落地的 `message-service SendMessage` 链路。可以在一台电脑上用多线程模拟多个客户端，也可以按 `docs/runbook/local-loadtest.md` 做双机压测。

硬约束：

- 压测命令必须支持 `target`、`vus`、`duration`、`result-dir` 参数或等价环境变量。
- 第一轮压测只接受真实 `message-service` 进程，不接受固定字符串 toy endpoint。

每轮压测至少记录：

```text
commit
target
vus
duration
request_count
success_rate
p95
p99
outbox_pending_count
outbox_oldest_pending_age
kafka_publish_latency
error_topn
```

压测结果输出到 `loadtest/results/`，大文件和临时日志默认不提交。

## 8. GitHub 同步要求

有意义的变更需要持久化时，必须：

- 执行并记录 `git status --short`。
- 执行可用检查，例如 `git diff --check`、生成配置检查、单元测试或集成测试。
- 提交后记录 `git log -1 --oneline`。
- 能推送时执行 `git push origin main`，并记录推送结果。
- 推送后再次确认 `git status --short` 干净。
- 如果本轮涉及 MacBook、服务器或压测环境，说明是否需要同步对应环境；不默认静默同步。

## 9. 每轮结束检查

每轮结束前确认：

- 文档是否需要同步更新。
- 是否需要邀请评审线程。
- 是否执行了可用检查。
- 是否需要 commit / push，并是否已经完成 GitHub 同步。
- 本文的状态、风险和下一步是否仍然准确。

## 10. 当前风险

- 当前 Codex 进程可能尚未重新读取用户 PATH；本线程运行 Go 命令前执行 `. .\tools\go-env.ps1`。
- 现阶段已有 app/domain/PostgreSQL repository 测试，也已有 outbox relay / Kafka producer 测试；服务入口和本地多线程压测入口尚未完成。
- 当前 Kafka writer 使用 `segmentio/kafka-go`，已配置 `acks=all`、hash key 和禁用自动建 topic，但该 Writer 不暴露 Kafka `enable.idempotence=true` 开关；生产硬化时需要更换支持幂等 producer 的 client 或接入更底层 transactional producer 能力。
- 当前 outbox relay 的 publish callback 在 PostgreSQL 事务内执行，这是第一阶段可接受的至少一次发布取舍；压测阶段需要重点观察 batch size、Kafka publish latency、DB lock wait 和重复发布窗口。
- 当前 relay 只支持 `message.persisted.v1`；启用 Edit/Revoke/Delete 前必须补齐对应 Kafka oneof payload 构造和测试。
- OutboxStore 后续进入多 worker 或压测前，应补强 `available_at/next_retry_at` 未到期、低版本 PENDING 阻塞、并发 `FOR UPDATE SKIP LOCKED` 等 ready/concurrency 测试。
- 当前 ready 判断使用 DB `now()`，retry 时间写入使用应用时钟；生产硬化时需要统一时间源或明确 DB/relay 节点时钟同步要求。
- 当前尚未实现 DLQ repair/replay；未来实现时必须清理 `dead_lettered_at`、`last_error`、`next_retry_at` 等旧失败字段。
- 还没有完整 SendMessage 端到端压测结果。
- `timeline-service`、`conversation-service`、`delivery-service`、`push-gateway` SDD 未冻结，不能扩展到对应生产逻辑。

## 11. 最近评审状态

- 2026-06-08：独立评审线程指出文档入口顺序、评审回传规则、GitHub 同步闭环、压测硬约束和目标态总架构入口需要补强；本轮已按建议更新本文和 `docs/README.md`。
- 2026-06-08：独立评审线程复核通过文档闭环；新增 P0 环境结论：`protoc` 可用，但 `go`、`protoc-gen-go`、`protoc-gen-go-grpc` 未检测到，正式实现和验证前必须补齐。
- 2026-06-08：已通过阿里云镜像安装 Go `1.26.4`，并通过 `GOPROXY=https://goproxy.cn,direct` 安装 `protoc-gen-go` 和 `protoc-gen-go-grpc`；按用户要求不刻意压低 Go 版本，项目基线已设为 Go `1.26.4`；`tools/gen-proto.ps1` 与 `go test ./...` 已通过。
- 2026-06-08：已实现 `SendMessageUseCase` 单元测试、领域 command hash / append record、PostgreSQL repository 本地事务；`go test ./...` 通过，`NEXUSIM_PG_DSN=postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable` 的 repository 集成测试通过。
- 2026-06-08：根据评审意见修复同一 `client_msg_id` 并发重复请求可能推进 `conversation_seq` 的问题，repository 已使用同幂等键 advisory transaction lock，并新增真实 PostgreSQL 并发集成测试；同时修复 payload JSON canonical 只压缩不稳定排序的问题。
- 2026-06-08：根据评审意见补齐 `MessagePersistedV1` payload 中的 `command_hash`，明确 `message_outbox.payload_json` 保存业务 payload、envelope/metadata 由 outbox 表字段组装；app 层已增加 `permission_version` 不一致时短重试一次，仍不一致返回可识别 dependency version error；outbox 写入失败已拆为 `ErrOutboxWriteFailed`。
- 2026-06-08：已实现 `trigger/outbox` relay、PostgreSQL outbox store 和真实 Kafka writer producer；本地已启动 `nexusim-kafka`，创建 `conversation.timeline.events` topic，并通过真实 PostgreSQL + Kafka 集成测试验证 outbox 可发布后标记 `PUBLISHED`，Kafka publish 失败时保留 pending/retry/DLQ 状态。
- 2026-06-08：独立评审线程复核 outbox relay + Kafka publish path，无 P0/P1 阻塞；P2/P3 风险已记录到本文，下一步可以提交本轮切片并推进 `message-service` gRPC adapter 与本地多线程压测。
