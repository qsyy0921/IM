# NexusIM Current Goal

本文是 Codex Goal 的持续入口。每轮工作开始时先执行 `git status --short`，然后读取本文，再决定本轮行动。

## 1. 当前目标

持续推进 `E:\development\IM` 的 NexusIM 项目落地。`message-service` 第一阶段主链路和压测证据已经形成，`conversation-service` 已完成第二个真实微服务的最小闭环；当前阶段转向第三个真实微服务：

```text
delivery-service
-> consume Kafka conversation.timeline.events
-> project user_inbox / device_delivery_cursors
-> PullInbox / AckDelivery
-> prepare push-gateway online delivery
```

## 2. 硬边界

- 项目统一命名为 `NexusIM`，不再使用旧项目名。
- 每个微服务独立使用六层 DDD，不做全局统一 DDD。
- 微服务内固定六层：`api / app / domain / infrastructure / types / trigger`。
- 根目录 `api/` 只放全局接口契约；`services/<service>/internal/api/` 才是服务内部接口适配实现。
- `message-service` 第一阶段只实现普通会话 `LOCAL_ROW_LOCK` 的 `SendMessage` 主链路。
- 后续优先补齐真实微服务边界，不继续在单个 `message-service` 上做重型硬件矩阵。
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
| conversation-service Proto | `api/proto/nexusim/conversation/v1/conversation_service.proto` 已存在，定义 `GetSendContext` |
| Kafka schema | `schemas/kafka/conversation.timeline.events.proto` 已存在 |
| PostgreSQL migration | `migrations/postgres/message/000001_message_core.sql` 已存在 |
| conversation-service migration | `migrations/postgres/conversation/000001_conversation_core.sql` 已存在，包含 `conversations`、`conversation_members`、`member_change_saga` |
| Docker Compose | `deploy/local/docker-compose.yml` 已存在；压测专用 PostgreSQL override 为 `deploy/local/docker-compose.postgres-loadtest.yml` |
| message-service 六层骨架 | `services/message-service/internal/{api,app,domain,infrastructure,types,trigger}` 已存在 |
| conversation-service 六层骨架 | `services/conversation-service/internal/{api,app,domain,infrastructure,types,trigger}` 已存在；第一条 read path 为 `GetSendContext` |
| Go 工具链 | 项目基线为 Go `1.26.4`；已通过阿里云镜像安装到 `C:\Users\10495\.local\go\go1.26.4\bin\go.exe`；`protoc-gen-go v1.36.11` 和 `protoc-gen-go-grpc 1.6.2` 已安装到 `C:\Users\10495\go\bin`；`protoc` 可用，路径为 `C:\Users\10495\anaconda3\Library\bin\protoc.exe`；本地命令先执行 `. .\tools\go-env.ps1` |
| Proto Go 代码 | 已生成 `api/proto/nexusim/message/v1/*.pb.go` 和 `schemas/kafka/conversation.timeline.events.pb.go` |
| Go 依赖 | `go.mod` 使用 Go `1.26.4`，并已引入 `google.golang.org/grpc v1.81.1`、`google.golang.org/protobuf v1.36.11` |
| SendMessage app/domain | 已补 `SendMessageUseCase` 单元测试、permission version 一致性短重试、稳定 JSON canonical command hash、append record 构造 |
| PostgreSQL repository | 已实现普通会话 `SendMessage` 本地事务：幂等检查、同幂等键 advisory transaction lock、`conversation_seq` row lock、`message_log`、`conversation_timeline_events`、`message_outbox` 同事务写入；outbox payload 对齐 `MessagePersistedV1` 业务 payload；集成测试和并发重复请求测试通过 |
| Outbox relay / Kafka publish path | 已实现 `trigger/outbox` relay、PostgreSQL outbox store、Kafka writer producer；relay 支持 `NEXUSIM_OUTBOX_WORKERS` 多 worker 与 `NEXUSIM_OUTBOX_FAILURE_BACKOFF` 失败退避；真实 PostgreSQL + Kafka 集成测试通过；真实 PostgreSQL 多 worker / `FOR UPDATE SKIP LOCKED` 测试已覆盖同 conversation 顺序和跨 conversation 并发；已将成功 publish 后的 outbox `PUBLISHED` 标记从逐条 update 改为同事务批量 update；relay 支持在 `OutboxStore.ProcessReadyBatch` 路径下使用 Kafka `PublishBatch` 批量写入，并可通过 `NEXUSIM_OUTBOX_PUBLISH_BATCH_ENABLED=false` 在同一 HEAD 下切回 single publish 对照；relay debug metrics 已暴露 `outbox_process_ready_latency_ms`、`outbox_fetch_ready_latency_ms`、`outbox_mark_published_latency_ms`、`outbox_commit_latency_ms` |
| message-service gRPC adapter | 已实现 `SendMessage` gRPC handler、proto request/response 转换、稳定错误码 detail 映射和错误 message 脱敏、`NEXUSIM_MESSAGE_SERVICE_MODE=grpc` 运行入口；支持 `NEXUSIM_DEBUG_ADDR=/debug/metrics` 暴露本进程压测指标；已通过 bufconn client 单测 |
| Backpressure / adaptive admission | 已新增 `MESSAGE_ERROR_CODE_SERVICE_OVERLOADED`，repository 支持默认关闭的 PostgreSQL pool backpressure；启用 `NEXUSIM_PG_BACKPRESSURE_ENABLED=true` 后，连接池可用连接数小于等于 `NEXUSIM_PG_BACKPRESSURE_MIN_AVAILABLE_CONNS` 时快速返回 retryable `service overloaded`；gRPC `SERVICE_OVERLOADED` 默认附带 `RetryInfo=500ms`，adaptive admission 可携带动态 retry delay；app 层已新增默认关闭的 `AdmissionPort`，`infrastructure/admission` 第一版 adaptive controller 可按 PG pool、repository pool acquire p95、outbox pending、relay active process ready、outbox fetched per call、Kafka records per call 触发提前拒绝；已新增 `NEXUSIM_ADAPTIVE_MAX_IN_FLIGHT` app 入口 token / concurrency gate，用于限制进入依赖读取和 DB 事务的并发数 |
| SendMessage loadtest | 已实现 `go run ./loadtest/sendmessage` 参数化 gRPC 压测入口；支持 `target`、`vus`、`duration`、`result-dir`、`pg-dsn`、`stats-wait`、`service-metrics-url`、`relay-metrics-url`；`target` 和 service metrics URL 已支持逗号分隔，用于模拟多 `message-service` 实例；summary 记录 full commit、dirty 状态、outbox total/published/pending/DLQ、SendMessage/repository/commit/seq/Kafka/outbox relay latency、service/relay pgx pool、repository 和 relay 内部分段指标、多进程 metrics、retryable error count、service overloaded count、`message_error_counts[]`、`request_rps`、`accepted_rps`、`error_rps`、attempt-level `overload_rate`、`success_p99_ms`、`error_p99_ms`、logical end-to-end latency；Kafka publish 指标已拆出 `kafka_publish_call_latency_ms`、`kafka_publish_records_per_call`、`kafka_publish_record_latency_estimate_ms`，避免 single path 和 batch path 口径混淆；outbox relay 指标已拆出 `outbox_process_ready_active_latency_ms`、`outbox_process_ready_idle_latency_ms`、`outbox_fetched_per_call`，避免 stats-wait idle 样本稀释 active relay 判断；recent 指标已在 summary 顶层暴露 sample count，便于 adaptive 矩阵判断 warm-up；压测器已支持可选 `--retry-overloaded`，会遵守 gRPC `RetryInfo` 并记录 `logical_request_count`、`logical_success_rate`、`retry_attempt_count`、`retried_request_count`、`retry_delay_count`、`retry_delay_avg_ms`、`retry_delay_p95_ms`、`retry_delay_p99_ms`；多 service metrics 的顶层 latency / pg pool 已改为聚合视图，避免只取第一个实例误导；`run-local-multi-instance.ps1` 已支持 `FixedPerInstance` 和 `FixedTotal` 两种 PG 连接预算模式；`run-local-pgpool-gradient.ps1` 已支持显式 `-BackpressureEnabled` 和 `-RetryOverloaded`；已补 `run-local-gradient.ps1`、`run-local-pgpool-gradient.ps1`、`run-local-multi-instance.ps1`、`run-local-outbox-batch-worker-matrix.ps1`、`collect-postgres-diagnostics.ps1`、`watch-postgres-diagnostics.ps1`；真实 gRPC + PostgreSQL + outbox relay + Kafka smoke、baseline、瓶颈诊断、PG pool / multi-instance 矩阵、PostgreSQL 诊断和 backpressure on/off 矩阵已执行；message-service 第一阶段 27 份压测报告已整合为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-message-service-consolidated.md` |
| 压测报告归档 | 每个微服务一个目录：`docs/runbook/loadtest/<service>/`；目录内保存小报告、矩阵报告和 consolidated 总报告。`message-service` 当前入口为 `docs/runbook/loadtest/message-service/README.md` |
| conversation-service smoke | 已跑真实进程小规模 smoke：`message-service -> conversation-service -> PostgreSQL`，725/725 成功，p99 13.26ms；报告见 `docs/runbook/loadtest/conversation-service/` |
| conversation-service local runbook | `docs/runbook/conversation-service-local.md` 已存在，记录 migration、seed、双服务启动、smoke 和清理步骤 |
| conversation-service member_change_saga SDD | `docs/sdd/conversation-service-member-change-saga.md` 已冻结 v1.0，选定 timeline append/publish 方案 C |
| conversation-service member change contract | `conversation_service.proto` 已新增 `CreateMemberChange` / `GetMemberChange` 契约；`conversation.timeline.events.proto` 已新增 member boundary oneof payload；`000002_member_change_saga_v2.sql` 已补 saga retry/DLQ/metadata/event id 字段；outbox relay builder 已支持 `conversation.member.*` fail-closed；`000003_member_change_saga_event_unique.sql` 补 saga event id 唯一约束 |
| conversation-service CreateMemberChange | 最小写路径已实现：gRPC adapter -> app usecase -> PostgreSQL repository；同事务写 `member_change_saga`、`conversation_members`、`conversations` version、`conversation_seq`、`conversation_timeline_events`、`message_outbox`；真实 PostgreSQL 集成测试已覆盖首写、幂等 replay、同 key 冲突和 event/timeline/outbox 一致性；权限矩阵已收紧为第一版保守规则，`MERGE/COMPENSATE` 冲突策略暂不接受 |
| conversation-service GetMemberChange / saga progress | `GetMemberChange` 查询接口已实现；`NEXUSIM_CONVERSATION_SERVICE_MODE=member-change-worker` 可启动 saga 推进 worker，观察 `message_outbox.status=PUBLISHED` 后把 `member_change_saga` 从 `OUTBOX_ENQUEUED` 推进到 `DONE`；真实 PostgreSQL 集成测试已覆盖 outbox 未发布不推进、发布后推进、limit 控制 |
| conversation-service member change full smoke | 已跑真实进程小规模 smoke：`CreateMemberChange -> outbox relay -> Kafka member event -> member-change-worker -> GetMemberChange(DONE)`，350/350 成功，p99 40.90ms，saga/outbox/timeline 各 350，outbox `PUBLISHED=350`，saga `DONE=350`，报告见 `docs/runbook/loadtest/conversation-service/loadtest-report-20260609-member-change-full-smoke.md` |
| conversation-service review fixes | 独立评审指出的 `GetMemberChange` 读取授权和 `last_error` 脱敏 P1 已修复：只允许操作者、目标用户、当前 ACTIVE 的 OWNER/ADMIN 查询；对外只返回稳定 `member change processing failed`，不透出 raw DB/Kafka/repair 文本；worker 推进 SQL 已补 conversation/producer/member event 防御性过滤；复核结论无 P0/P1 |
| delivery-service SDD | `docs/sdd/delivery-service.md` 已新增 v0.1 Draft，并已按评审 P1 补齐 delivery membership projection、ACK max visible seq 约束、Kafka checkpoint 维度 |
| delivery-service 工程基线 | 已新增 `delivery_service.proto`、delivery migration、六层目录、`PullInbox / AckDelivery` 最小 gRPC + app + PostgreSQL 骨架，以及 `ProjectTimelineEventUseCase` / PostgreSQL projection 方法；Kafka consumer worker 尚未完成 |

## 5. 下一步优先级

1. 当前 Codex 进程如果仍找不到 `go`，先执行 `. .\tools\go-env.ps1`。
2. adaptive limit 第一版已接入并跑完 on/off 对照；debug metrics collector 已新增 recent 窗口字段，adaptive controller 已新增 hysteresis 和动态 retry hint；loadtest summary 已记录 retry delay histogram。
3. adaptive best candidate 60s 重复验证已完成，`gap8-outbox50-base1000` 能保护 outbox 清零，但 accepted RPS 过低，不能作为稳定配置。
4. pool acquire p95 阈值矩阵已完成，单纯放宽 `250ms / 500ms / 750ms` 不能解决 accepted RPS 偏低和 logical p99 过高的问题。
5. admission token / concurrency limit 已实现并跑完 v1 矩阵；`MaxInFlight=64` 是当前 Windows 本机候选，60s 下 1200/1600 VU accepted RPS 约 `1.92k`、success p99 约 `63ms`、outbox pending 为 0。
6. message-service 第一阶段压测报告已归档到 `docs/runbook/loadtest/message-service/`，总入口为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-message-service-consolidated.md`；后续不再围绕 message-service 做大规模硬件矩阵，只在关键机制变更后跑 smoke / 小规模验证。
7. `conversation-service` 最小 RPC read path 已落地并通过真实进程 smoke：SDD、proto、migration、六层骨架、PostgreSQL repository、gRPC handler、`message-service` 可选 gRPC client 和 `message-service -> conversation-service -> PostgreSQL` 小规模验证均已完成。
8. `conversation-service` 本地运行 runbook 和更多错误路径测试已补齐；独立评审指出的 P1 参数缺失错误映射已修复；P2 中的 `message-service -> conversation-service` 短重试和 response contract 防御也已补。
9. `conversation-service / member_change_saga` 最小 `CreateMemberChange` 写路径已落地，并已完成真实进程 smoke：`CreateMemberChange -> outbox relay -> Kafka member event -> outbox PUBLISHED`，报告见 `docs/runbook/loadtest/conversation-service/loadtest-report-20260609-member-change-smoke.md`。
10. 独立评审已确认 `conversation-service` member change full smoke 阶段无 P0/P1；第二个真实微服务最小闭环可以收口。
11. 当前进入 `delivery-service`：SDD 评审 P1 已应用；proto、migration、六层骨架、`PullInbox / AckDelivery` 最小同步路径和 `ProjectTimelineEventUseCase` 已开始落地；下一步补 Kafka timeline consumer worker，再做真实小规模 smoke。`push-gateway` 暂只做后续 SDD，不先抢实现。

## 6. 评审要求

评审采用里程碑触发，不对每个小改动都邀请独立评审线程。

必须邀请评审的情况：

- 一个可运行链路完成，例如 `SendMessage -> outbox -> Kafka`、gRPC adapter + smoke/loadtest、压测闭环。
- 跨越两层以上 DDD 边界，或修改 ADD / TADD / SDD / migration / proto / Kafka schema。
- 涉及并发、事务、幂等、消息顺序、错误码契约、数据一致性等高风险逻辑。
- 准备把某个阶段标记为完成，或准备做较大的 GitHub 同步。
- 用户明确要求评审。

不需要立即评审的情况：

- 小范围重命名、注释、文档索引、runbook 状态更新。
- 单层内部测试补强，且没有改变对外契约或事务语义。
- 修复明显拼写、格式、路径说明等低风险问题。

低风险变更先本地验证并累计到下一次阶段评审；如果连续开发中出现不确定的架构取舍，再主动请求评审。

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

压测只服务于验证关键链路和形成可解释证据，不再追求耗尽本机硬件资源的极限矩阵。可以在一台电脑上用多线程模拟多个客户端，也可以按 `docs/runbook/local-loadtest.md` 做双机压测。

硬约束：

- 压测命令必须支持 `target`、`vus`、`duration`、`result-dir` 参数或等价环境变量。
- 第一轮压测只接受真实服务进程，不接受固定字符串 toy endpoint。

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
kafka_publish_call_latency
kafka_publish_records_per_call
kafka_publish_record_latency_estimate
error_topn
```

压测结果输出到 `loadtest/results/`，大文件和临时日志默认不提交。

每个阶段必须新增一份独立压测报告，不覆盖旧报告。报告按微服务放在 `docs/runbook/loadtest/<service>/`，推荐命名：

```text
docs/runbook/loadtest/<service>/loadtest-report-YYYYMMDD-<stage>.md
```

每个微服务阶段结束时维护一份 consolidated 总报告，汇总本服务的小报告、压测方法、瓶颈排查过程和面试可讲结论。`message-service` 当前总入口为：

```text
docs/runbook/loadtest/message-service/loadtest-report-20260609-message-service-consolidated.md
```

报告至少说明：

```text
压测目标
压测拓扑
服务端和客户端分别跑在哪里
CPU / 内存 / Docker / 连接池 / worker 配置
具体命令或脚本入口
通过标准
核心结果
中间结果文件路径
瓶颈排查方法
当前结论和下一步
```

`loadtest/results/` 保存所有中间结果和趋势图；这些文件默认不提交，但报告必须引用关键结果路径，保证以后能追溯。

## 8. GitHub 同步要求

GitHub 同步采用批量策略，不对每个小改动都推送。

优先本地完成并验证一组相关变更；满足以下任一条件再 commit / push：

- 一个可描述的功能切片完成，例如 repository、outbox relay、gRPC adapter、loadtest runner。
- 修改了公共契约、migration、架构文档或会影响他人协作的文件。
- 用户要求同步 GitHub / MacBook / 服务器。
- 工作区累计变更较多，继续开发前需要建立明确恢复点。
- 评审完成且需要把已验收状态固定下来。

小范围文档状态更新、低风险测试补强、探索性修改可以暂不 push，必要时只保留本地工作区或本地 commit。

需要持久化时，必须：

- 执行并记录 `git status --short`。
- 执行可用检查，例如 `git diff --check`、生成配置检查、单元测试或集成测试。
- 提交后记录 `git log -1 --oneline`。
- 达到同步条件时再执行 `git push origin main`，并记录推送结果。
- 推送后再次确认 `git status --short` 干净。
- 如果本轮涉及 MacBook、服务器或压测环境，说明是否需要同步对应环境；不默认静默同步。

## 9. 每轮结束检查

每轮结束前确认：

- 文档是否需要同步更新。
- 是否达到里程碑评审条件；未达到则不邀请评审线程。
- 是否执行了可用检查。
- 是否达到 commit / push 条件；未达到则只记录本地状态，不强行同步 GitHub。
- 本文的状态、风险和下一步是否仍然准确。

## 10. 当前风险

- 当前 Codex 进程可能尚未重新读取用户 PATH；本线程运行 Go 命令前执行 `. .\tools\go-env.ps1`。
- 现阶段已有 app/domain/PostgreSQL repository 测试、outbox relay / Kafka producer 测试、gRPC adapter 测试和真实进程多线程压测入口。
- 当前 Kafka writer 使用 `segmentio/kafka-go`，已配置 `acks=all`、hash key 和禁用自动建 topic，但该 Writer 不暴露 Kafka `enable.idempotence=true` 开关；生产硬化时需要更换支持幂等 producer 的 client 或接入更底层 transactional producer 能力。
- 当前 outbox relay 的 publish callback 在 PostgreSQL 事务内执行，这是第一阶段可接受的至少一次发布取舍；当前已支持 batch publish，但事务仍覆盖 Kafka publish callback、batch mark 和 commit；压测阶段需要重点观察 batch size、Kafka publish latency、DB lock wait 和重复发布窗口。
- 当前 relay 只支持 `message.persisted.v1`；启用 Edit/Revoke/Delete 前必须补齐对应 Kafka oneof payload 构造和测试。
- OutboxStore 已补真实 PostgreSQL 多 worker / `FOR UPDATE SKIP LOCKED` 集成测试，证明同 conversation 顺序不乱、跨 conversation 可并发追平；后续仍可补强 `available_at/next_retry_at` 未到期、低版本 PENDING 阻塞等 ready 条件测试。
- 当前 relay 支持可配置多 worker；只有 `Published > 0` 时才立即继续循环，`Fetched > 0` 但没有成功发布时按 `FailureBackoff` 退避，空转时按 `PollInterval` sleep；同一 conversation 仍按最低 `aggregate_version` 串行发布，因此单会话积压追平能力仍受顺序保护限制。
- 当前 relay 已缓解 Kafka 故障且 backlog 很大时的连续失败放大，但生产化仍需要结合失败比例、Kafka publish latency、DB lock wait 调整 backoff、batch size 和 worker 数。
- 当前 ready 判断使用 DB `now()`，retry 时间写入使用应用时钟；生产硬化时需要统一时间源或明确 DB/relay 节点时钟同步要求。
- 当前尚未实现 DLQ repair/replay；未来实现时必须清理 `dead_lettered_at`、`last_error`、`next_retry_at` 等旧失败字段。
- 已有真实进程级 SendMessage smoke 和 `--vus=100 --duration=60s` baseline 结果；baseline 写入成功率 100%，但 p99 与 outbox backlog 暴露出性能风险。
- 压测 summary 已支持从 gRPC 进程和 relay 进程的 `/debug/metrics` 读取 `conversation_seq_alloc_latency_ms`、兼容旧报告的 `kafka_publish_latency_ms`、`kafka_publish_call_latency_ms`、`kafka_publish_records_per_call`、`kafka_publish_record_latency_estimate_ms`、`outbox_process_ready_latency_ms`、`outbox_process_ready_active_latency_ms`、`outbox_process_ready_idle_latency_ms`、`outbox_fetched_per_call`、`outbox_fetch_ready_latency_ms`、`outbox_mark_published_latency_ms` 和 `outbox_commit_latency_ms`；生产化仍需接入统一 metrics/tracing，而不是依赖本地 debug endpoint。
- 当前 `51772e6` baseline：45212/45212 成功，p95 249.62ms，p99 518.03ms，`stats-wait=30s` 后本轮 tenant outbox `PENDING=27181`、`PUBLISHED=18031`。
- 当前 worker/backoff dirty baseline：`NEXUSIM_OUTBOX_WORKERS=4`、`--vus=100 --duration=60s --stats-wait=30s --conversation-count=1000`，69608/69608 成功，p95 122.10ms，p99 156.24ms，`stats-wait=30s` 后本轮 tenant outbox `PENDING=2123`、`PUBLISHED=67485`；relay 额外 drain 20s 后该 tenant outbox 全部 `PUBLISHED=69608`。该数据用于开发判断，不能作为 clean commit 正式性能归档。
- 当前 metrics clean smoke：commit `ea4eb9a`，`--vus=10 --duration=10s --stats-wait=10s --conversation-count=200`，8699/8699 成功，p95 19.46ms，p99 31.68ms，outbox pending 0；summary 已写入 `conversation_seq_alloc_latency_ms=1.47`、`conversation_seq_alloc_p95_ms=2.59`、`kafka_publish_latency_ms=1.01`、`kafka_publish_p95_ms=1.73`，且 `git_dirty=false`。
- 当前 worker/backoff clean baseline：commit `0ff42d2`，`NEXUSIM_OUTBOX_WORKERS=4`、`--vus=100 --duration=60s --stats-wait=30s --conversation-count=1000`，24714/24714 成功，p95 436.24ms，p99 583.96ms，summary 读取时本轮 tenant outbox `PENDING=392`、`PUBLISHED=24322`、`DLQ=0`，`conversation_seq_alloc_latency_ms=6.98`、`kafka_publish_latency_ms=4.21`，且 `git_dirty=false`；随后查询该 tenant outbox 已全部 `PUBLISHED=24714`。该结果说明 4 worker 能追平，但本机长压测吞吐波动明显，需要重复 clean baseline 或梯度压测后再形成正式性能结论。
- 当前 gradient script dirty smoke：`run-local-gradient.ps1 -Workers 4,8,16 -VUs 20 -Duration 10s -StatsWait 10s -ConversationCount 500` 已跑通三组真实进程压测，全部成功且 outbox pending 0；由于脚本未提交，summary 为 `git_dirty=true`，仅作为脚本可运行验证，不作为正式梯度趋势证据。
- 当前 Windows Docker Desktop 已按用户要求调整为 WSL2 `processors=16`、`memory=24GB`、`swap=8GB`，`docker info` 显示 Docker VM 可用 `16 CPU`、约 `23.47GiB` 内存。
- 当前 win-win Docker 资源矩阵已覆盖 `1/2/4 CPU + 256m/512m/1g`、`8/12/16 CPU + 2g/4g/8g`、以及 `16 CPU + 23g` 档位。按 `success_rate >= 0.99`、`p99 <= 1000ms`、`outbox_pending_count <= 1000` 的门槛，已观察到的最佳通过档为 `16 CPU / 23g / relay workers=8 / 1200 VU`，约 `2493 rps`、p99 `736.28ms`、outbox pending 0；`1600 VU` 时 p99 `1120.48ms` 超线。`16 CPU / 23g / relay workers=16` 在 `1200 VU` 即 p99 `1477.19ms` 超线，说明盲目增加 relay worker 会放大争用。
- 当前 Windows+Mac 分布式双客户端已跑通：Windows 服务端暴露 `10495/10497/10500`，Windows 和 Mac 同时作为 load generator。`600+600 VU` 双客户端全部成功，Windows client p99 `730.11ms`、Mac client p99 `739.50ms`、outbox pending 0；`1000+1000 VU` 双客户端全部成功但 p99 均约 `1331ms`，按当前尾延迟门槛超线。
- 当前压测趋势图已生成到 `loadtest/results/charts/`：`winwin-rps-trend.png`、`winwin-p99-trend.png`、`distributed-clients-trend.png`，摘要为 `loadtest/results/charts/winwin-distributed-summary.md`；这些结果文件默认不提交。
- 当前压测正式报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609.md`，已记录压测拓扑、执行方式、通过标准、结果摘要和瓶颈排查过程。
- 当前 PG pool / multi-instance 诊断报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-pgpool-multi-instance.md`，已记录 repository 细分指标、多 target loadtest、多 service metrics URL、PG pool smoke、multi-instance smoke、正式 PG pool 矩阵和正式 multi-instance 矩阵。
- 当前 p99 瓶颈已初步定位：`16 CPU / 23g / 1600 VU / workers=8` 下，request p99 基本等于 repository append p99；commit、conversation_seq、Kafka publish 都是毫秒级；service pgxpool 默认 `max_conns=16` 时 acquire 平均等待约 `646ms`，`NEXUSIM_PG_MAX_CONNS=64` 后 1600 VU p99 改善到 `779.63ms`，但 2400 VU 仍 p99 `1452.87ms` 超线。下一步优先做 PG 连接池梯度、多 message-service 实例和 repository 细分打点。
- 当前 repository 细分指标已落地：`repository_begin_latency_ms`、`repository_pool_acquire_latency_ms`、`repository_tx_begin_latency_ms`、`repository_idempotency_lock_latency_ms`、`repository_find_existing_latency_ms`、`repository_ensure_seq_latency_ms`、`repository_allocate_seq_latency_ms`、`repository_insert_message_latency_ms`、`repository_insert_timeline_latency_ms`、`repository_insert_outbox_latency_ms`、`repository_commit_latency_ms`。
- 当前 clean commit `e87bb9b` PG pool smoke：`PG_MAX_CONNS=16/64`、`VU=20`、`duration=5s`、`stats-wait=5s`，两组全部成功；p99 分别为 `42.52ms`、`33.46ms`，结果在 `loadtest/results/pgpool-smoke-20260609-013424/`，后续短 relay drain 后对应 outbox 均为 `pending=0`。
- 当前 clean commit `e87bb9b` multi-instance smoke：`Instances=1/2`、`VU=20`、`duration=5s`、`stats-wait=5s`，两组全部成功；p99 分别为 `40.43ms`、`39.35ms`，结果在 `loadtest/results/multi-instance-smoke-20260609-013511/`，多 target 和多 service metrics URL 均已验证；后续短 relay drain 后对应 outbox 均为 `pending=0`。
- 当前 formal PG pool 矩阵：`loadtest/results/pgpool-formal-20260609-014259/`。`PG_MAX_CONNS=16` 时 1200 VU 仍 100% 成功但 p99 `1725.16ms`；1600 VU 成功率降到 `0.6870`。`PG_MAX_CONNS=32` 时 1200/1600 VU 成功率 100%，但 p99 仍为 `1381.33ms` / `1476.37ms`；2000 VU 成功率 `0.9712`。`PG_MAX_CONNS=64` 时 1200/1600/2000 VU 写入成功率高，但 outbox pending 分别升到 `8044` / `19851` / `49948`，说明写入并发放大后 relay 追平成为第二瓶颈。`PG_MAX_CONNS=96/128` 在当前 PostgreSQL `max_connections` 下触发 `FATAL: sorry, too many clients already`，结果无效。
- 当前 formal multi-instance 矩阵：`loadtest/results/multi-instance-formal-20260609-021254/`。在每实例 `PG_MAX_CONNS=16`、1200 VU 下，1/2/4 实例成功率分别为 `0.9660` / `0.5572` / `0.9014`，请求 p99 均约 `2000ms`；用 `service_metrics[]` 重新计算后，4 实例的 per-instance `repository_begin_p99` 范围为 `603.59ms` 到 `2002.33ms`，说明不是每个实例都打满，但整体请求 p99 仍未改善。在当前单 PostgreSQL、当前连接上限和当前写入模型下，多实例不能解决尾延迟。
- 当前 formal 矩阵追加短 relay drain 后确认 `tenant_count=16 total_pending=0`，没有留下未发布 outbox 积压。
- 当前 PostgreSQL 诊断脚本为 `loadtest/sendmessage/collect-postgres-diagnostics.ps1`。正式矩阵后采集结果在 `loadtest/results/postgres-diagnostics-20260609-022602/postgres-diagnostics.json`：`max_connections=100`、`shared_buffers=16384`、`max_wal_size=1024`、`checkpoint_timeout=300`、`synchronous_commit=on`、`deadlocks=0`；`message_outbox n_dead_tup=113312`，说明 outbox 高频 update 已产生明显 dead tuples。
- 当前 multi-instance PG budget 矩阵：`loadtest/results/multi-instance-budget-formal-20260609/`，报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-multi-instance-budget.md`。固定每实例预算 `1x16/2x16/4x16` 下 p99 分别为 `2000.53ms` / `1437.37ms` / `1975.75ms`；固定总预算 `1x64/2x32/4x16` 下 p99 分别为 `1178.97ms` / `1509.55ms` / `1657.18ms`。固定总预算下多实例没有降低尾延迟，且 outbox pending 为 `31889` / `64553` / `65323`，说明当前应优先处理 PostgreSQL acquire/begin 排队与 relay 追平，而不是继续堆 gRPC 实例。
- 当前 PostgreSQL loadtest profile 矩阵：`loadtest/results/pgpool-tuned-formal-20260609/`，报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-postgres-loadtest-profile.md`。启用 `max_connections=200`、`shared_buffers=1GB`、`max_wal_size=4GB` 后，`PG_MAX_CONNS=64/VU1200` p99 `1161.70ms`，`PG_MAX_CONNS=64/VU1600` p99 `1759.11ms`；`PG_MAX_CONNS=128` 未改善，1200 VU 成功率 `0.9760` 且 p99 `2001.08ms`。新指标确认 `repository_begin` 主体是 `repository_pool_acquire`，`repository_tx_begin` p99 仅约 `14-33ms`。watch 采样显示 `LWLock:WALWrite`、`LWLock:WALInsert`、`LWLock:BufferContent` 和 `CheckpointWriteDelay` 已进入瓶颈视野。
- 当前 backpressure on/off 正式矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-backpressure-onoff.md`，结果路径为 `loadtest/results/backpressure-off-formal-20260609/` 与 `loadtest/results/backpressure-on-formal-20260609/`。固定 `PG_MAX_CONNS=64`、relay workers 8 时，off 模式 1200/1600 VU 均 100% 成功，但 success p99 为 `1187.23ms` / `1735.38ms`，且 outbox pending 为 `30689` / `47736`；on 模式 attempt-level overload rate 为 `97.28%` / `98.01%`，error p99 仅 `12.49ms` / `14.26ms`，但 success p99 仍为 `1403.95ms` / `1808.10ms`。结论：backpressure 快速拒绝有效且能降低 backlog，但当前 `MinAvailableConns=0` 策略过于粗糙，不能宣称成功请求 p99 改善。
- 当前 backpressure 阈值梯度：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-backpressure-gradient.md`，结果路径为 `loadtest/results/backpressure-minavail-{0,4,8,16}-formal-20260609/`。`MinAvailable=0` 复跑出现 DeadlineExceeded 和 DB_WRITE_FAILED，说明拒绝太晚；`MinAvailable=4/8` 错误基本稳定为 `SERVICE_OVERLOADED`，outbox pending 均为 0。`MinAvailable=8` 是下一轮优先验证的短期候选之一：1200 VU accepted RPS `818.73`、success p99 `1191.10ms`；1600 VU accepted RPS `933.93`、success p99 `1249.85ms`。该策略仍牺牲大量请求成功率，只能作为保护阈值候选，不是生产默认值或容量提升方案。
- 当前 client retry 正式矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-client-retry.md`，结果路径为 `loadtest/results/backpressure-client-retry-formal-20260609/`。固定 `MinAvailable=8`、`RetryInfo=500ms`、`max_retries=2`、`jitter=100ms` 时，1200 VU logical success rate `68.68%`、accepted RPS `1526.23`、success p99 `421.58ms`、outbox pending `25579`；1600 VU logical success rate `56.04%`、accepted RPS `1288.98`、success p99 `1154.47ms`、outbox pending `32091`。错误全部是 `SERVICE_OVERLOADED`，没有 DeadlineExceeded / DB_WRITE_FAILED；但 accepted 写入回升后 outbox relay 再次成为瓶颈。
- 当前 outbox relay 批量 mark published：`OutboxStore.ProcessReady` 已把成功 publish 后的逐条 `UPDATE message_outbox SET status='PUBLISHED'` 改为同一事务内按 id 数组批量 update；真实 PostgreSQL 集成测试 `TestOutboxStoreProcessReadyBatchMarksPublished` 已验证两条跨 conversation outbox 可同批标记为 published，并已用 client retry 矩阵重跑验证 pending 明显下降。
- 当前 outbox batch mark 对照矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-batchmark.md`，结果路径为 `loadtest/results/backpressure-client-retry-batchmark-formal-20260609/`。对比 `0c542a1` before 与 `b6f0b82` after，1200 VU outbox pending 从 `25579` 降到 `5186`，1600 VU 从 `32091` 降到 `0`；但 accepted RPS 和 success p99 有本机波动，不能宣称整体容量提升。当前结论只限于：批量 mark published 明显降低 relay backlog。
- 当前 relay metrics smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-relay-metrics-smoke.md`，结果路径为 `loadtest/results/relay-metrics-smoke-20260609/bpoff-pgmax-16-vu-10-20260609-052152/sendmessage-summary.json`。commit `148938e`、`git_dirty=false`、5244/5244 成功、p95 `12.334ms`、p99 `15.8853ms`、outbox pending 0；summary 已写入 `outbox_process_ready_latency_ms=43.2098`、`outbox_fetch_ready_latency_ms=18.1205`、`outbox_mark_published_latency_ms=2.1892`、`outbox_commit_latency_ms=2.6068`。该结果只证明观测链路可用，不作为容量结论。
- 当前 outbox batch size 探索矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-batchsize-smoke.md`，结果路径为 `loadtest/results/outbox-batchsize-smoke-20260609/`。commit `aeaf88a`、`git_dirty=false`，固定 `PG_MAX_CONNS=64`、relay workers 8、1200 VU、30s、客户端 retry；batch 100/500/1000 均 outbox pending 0。batch 500 的 logical success rate `0.7160` 和 accepted RPS `1694.93` 较好，但 `outbox_process_ready_latency_ms=415.83ms` 最高；batch 100 success p99 最低为 `584.42ms`。当前只把 batch 500 作为下一轮正式矩阵候选，不作为最终默认值。
- 当前 outbox PublishBatch smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-publishbatch-smoke.md`，结果路径为 `loadtest/results/outbox-publishbatch-smoke-20260609/` 与 `loadtest/results/outbox-publishbatch-smoke-repeat-20260609/`。commit `9f26d0c`、`git_dirty=false`，固定 batch 500、1200 VU、30s。after run 2 logical success rate `0.7125`、accepted RPS `1658.80`、success p99 `576.48ms`、outbox pending 0；`outbox_process_ready_latency_ms` 从 before 的 `415.83ms` 降到 `41.95ms`。但 after run 1 波动明显，不能宣称整体容量已提升；batch path 下 `kafka_publish_latency_ms` 是 batch 调用耗时，不再等价于旧单条 publish 耗时。
- 当前 PublishBatch metrics smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-publishbatch-metrics-smoke.md`，结果路径为 `loadtest/results/publishbatch-metrics-smoke-20260609/bpoff-pgmax-16-vu-10-20260609-055454/sendmessage-summary.json`。commit `8742f84`、`git_dirty=false`、5645/5645 成功、p99 `12.7919ms`、outbox pending 0；summary 已写入 `kafka_publish_call_latency_ms=11.5020`、`kafka_publish_records_per_call=21.0672`、`kafka_publish_record_latency_estimate_ms=0.7350`。该结果只证明指标链路可用，不作为容量结论。
- 当前 PublishBatch formal 矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-publishbatch-formal.md`，有效结果路径为 `loadtest/results/publishbatch-formal-valid-{off,on}-r{1,2}-20260609/`。commit `aec449c`、`git_dirty=false`，固定 `PG_MAX_CONNS=64`、relay workers 8、batch 500、backpressure min 8、客户端 retry。`PublishBatch=false` 时 `kafka_publish_records_per_call=1`，1200/1600 VU 的 stats wait pending 平均为 `33776` / `36980`；`PublishBatch=true` 时 records/call 平均为 `381.08` / `188.60`，1600 VU pending 平均降到 `0`，但 1200 VU 仍有平均 `14904` pending，且 success p99 未稳定改善。第一次矩阵因 `-SkipBuild` 误用旧二进制导致 `pbatchoff` 实际未关闭 batch，已标记为无效中间结果，不作为正式证据。
- 当前 outbox batch/worker 联合矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-batch-worker-matrix.md`，结果路径为 `loadtest/results/outbox-batch-worker-matrix-{1200,1600}-formal-20260609/`。commit `134613c`、`git_dirty=false`，固定 `PG_MAX_CONNS=64`、`PublishBatch=true`、backpressure min 8、客户端 retry；batch 100/500/1000 与 workers 8/12/16 的 18 组全部在 `stats_wait=20s` 后 pending 0。1200 VU 下 batch 500/workers 8 accepted RPS 最高为 `1969.90`；1600 VU 下 batch 100/workers 8 accepted RPS 最高为 `1873.23` 且 success p99 最低为 `667.54ms`。当前只把 `100/8` 与 `500/8` 作为下一轮重复验证候选，不作为最终默认值。
- 当前 outbox active/idle metrics smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-active-idle-metrics-smoke.md`，结果路径为 `loadtest/results/outbox-active-idle-metrics-smoke-20260609/batch-100-workers-2/bpon-pbatchon-pgmax-16-vu-10-20260609-070214/sendmessage-summary.json`。commit `40baec9`、`git_dirty=false`、5487/5487 成功、pending 0；summary 已写入 `outbox_process_ready_active_latency_ms=18.4141`、`outbox_process_ready_idle_latency_ms=2.2710`、`outbox_fetched_per_call=12.2752`。该结果只证明观测链路可用，不作为容量结论。
- 当前 outbox 候选重复矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-candidate-repeat.md`，结果路径为 `loadtest/results/outbox-candidate-repeat-r{1,2}-20260609/`。commit `c41b26a`、`git_dirty=false`，重复验证 `batch_size=100/workers=8` 和 `batch_size=500/workers=8`。两组均 pending 0；`100/8` 的 success p99 更稳，1200/1600 VU 平均为 `524.74ms` / `818.47ms`，优于 `500/8` 的 `696.28ms` / `1150.13ms`。当前本地 relay 基线收敛为 `batch_size=100/workers=8`。
- 当前 adaptive limit smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-limit-smoke.md`，结果路径为 `loadtest/results/adaptive-limit-smoke-final-20260609/bpoff-adapton-pbatchon-pgmax-4-vu-5-20260609-072955/sendmessage-summary.json`。commit `d2af748`、`git_dirty=false`，使用极端阈值 `PG_MAX_CONNS=4`、`AdaptiveMinAvailableConns=4` 验证提前拒绝链路；15136 次请求全部返回 retryable `SERVICE_OVERLOADED`，p99 `3.1742ms`，本轮 tenant outbox `total=0/pending=0/DLQ=0`。该结果只证明 adaptive admission 可运行，不作为容量结论。
- 当前 adaptive limit on/off 矩阵：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-limit-onoff.md`，有效结果路径为 `loadtest/results/adaptive-onoff-v3-repo-backpressure-20260609/` 和 `loadtest/results/adaptive-relaxed-v1-admission-20260609/`。commit `7d6e59f`、`git_dirty=false`，固定 `PG_MAX_CONNS=64`、`batch_size=100`、`workers=8`、客户端 retry；relaxed app adaptive 在 1200/1600 VU 下 accepted RPS 为 `1858.43` / `1772.47`，success p99 为 `575.07ms` / `709.94ms`，outbox pending 均为 0。该结果说明 app admission 可作为入口保护，但没有证明容量提升；累计 p95 和 idle relay 样本不能作为独立硬拒绝条件。
- 当前 recent metrics smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-recent-metrics-smoke.md`，结果路径为 `loadtest/results/recent-metrics-smoke-20260609/bpoff-adapton-pbatchon-pgmax-16-vu-10-20260609-075920/sendmessage-summary.json`。commit `6d4910b`、`git_dirty=false`，5392/5392 成功、p99 `14.5047ms`、outbox pending 0；summary 已写入 `repository_pool_acquire_recent_latency_ms`、`outbox_process_ready_active_recent_latency_ms`、`outbox_fetched_per_call_recent`、`kafka_publish_records_per_call_recent`。该结果只证明窗口化观测链路可用，不作为容量结论。
- 当前 adaptive hysteresis smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-hysteresis-smoke.md`，结果路径为 `loadtest/results/adaptive-hysteresis-smoke-20260609/bpoff-adapton-pbatchon-pgmax-16-vu-10-20260609-080738/sendmessage-summary.json`。commit `6f9a438`、`git_dirty=false`，5554/5554 成功、p99 `12.5034ms`、service overloaded 0、outbox pending 0；该结果只证明 hysteresis 配置链路可运行，不作为容量结论。
- 当前 adaptive retry hint smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-retry-hint-smoke.md`，结果路径为 `loadtest/results/adaptive-retry-hint-metrics-smoke-20260609/adaptive-retry-hint-metrics-grpc-only-20260609-082000/sendmessage-summary.json`。commit `c9e6cf1`、`git_dirty=false`，极端过载下 57 次 gRPC attempt 全部返回 `SERVICE_OVERLOADED`，outbox pending 0；summary 已记录 `retry_delay_count=31`、`retry_delay_avg_ms=491.94`、`retry_delay_p95_ms=500`、`retry_delay_p99_ms=500`，证明压测器读取并执行了 gRPC `RetryInfo`。
- 当前评审确认 `retry_delay_count` 表示收到并计划遵守的 RetryInfo 数量，不严格等于完成 sleep 后进入下一次 attempt 的次数；正式 adaptive 矩阵必须同时展示 `retry_delay_count`、`retry_attempt_count` 和 logical success，避免把计划等待次数误读成真实重试次数。
- 当前评审确认 `*_recent` 是最近 4096 个样本窗口，不是时间窗口；低流量或样本数低于 `MinMetricSamples` 时不能基于 recent 指标下调参结论，后续报告必须展示 recent sample count。
- 当前评审确认 relay 相关 adaptive 条件隐含依赖 outbox pending 采样；如果配置了 relay active p95、outbox fetched per call 或 Kafka records per call 条件，必须同时配置 outbox pending 采样阈值，否则这些 relay 条件不会独立触发拒绝。
- 当前 dynamic retry hint 公式仍是启发式：`min(reason_count * base_delay, max_delay)`，recovering 状态额外加一档；不能作为稳定控制律或最优 retry delay，后续应结合 accepted RPS、outbox drain rate 和 pool acquire recent p95 调整。
- 当前 adaptive threshold matrix v1：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-threshold-v1.md`，正式结果路径为 `loadtest/results/adaptive-threshold-v1-clean-20260609/`。commit `9830f34`、`git_dirty=false`，固定 `PG_MAX_CONNS=64`、relay workers `8`、batch size `100`、PublishBatch on、`max_retries=2`、`retry_jitter=100ms`。本轮最佳候选为 `gap8-outbox50-base1000`：1200 VU logical success `80.78%`、accepted RPS `578.30`、success p99 `1955.41ms`；1600 VU logical success `56.28%`、accepted RPS `456.07`、success p99 `1819.91ms`；两组 outbox pending 均为 0。该结果说明更长 retry hint 能缓解重试压力，但 adaptive admission 仍是保护机制，不是容量提升结论。
- 当前 adaptive best candidate 60s repeat：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-best-repeat.md`，正式结果路径为 `loadtest/results/adaptive-threshold-best-repeat-20260609/`。commit `fb872a9`、`git_dirty=false`，重复验证 `gap8-outbox50-base1000`。1200 VU 两轮 logical success 为 `39.01%` / `41.69%`，accepted RPS 为 `261.55` / `273.58`；1600 VU 两轮 logical success 为 `27.96%` / `30.59%`，accepted RPS 为 `226.45` / `246.82`；所有 outbox pending 均为 0。该结果推翻了 30s 短测的乐观判断，说明当前 adaptive admission 过度保护。
- 当前 logical latency smoke：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-logical-latency-smoke.md`，结果路径为 `loadtest/results/logical-latency-smoke-20260609/bpoff-adapton-pbatchon-pgmax-4-vu-5-20260609-092242/sendmessage-summary.json`。commit `91cbb8c`、`git_dirty=false`，attempt p99 `7.3318ms`，logical p99 `507.6808ms`，retry delay p95 `500ms`，证明 summary 已能把 retry sleep 计入用户层等待。
- 当前 adaptive pool acquire p95 matrix v1：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-poolp95-v1.md`，正式结果路径为 `loadtest/results/adaptive-poolp95-v1-20260609/`。commit `b98e3c9`、`git_dirty=false`，对比 `AdaptiveMaxPoolAcquireP95=250ms/500ms/750ms`。所有组合 outbox pending 均为 0；1200 VU 下 accepted RPS 最高为 `277.55`，logical p99 仍为 `5968.57ms`；1600 VU 下 accepted RPS 约 `195-203`，logical p99 约 `5091-5463ms`。该结果说明单纯放宽 pool acquire p95 阈值不能解决当前过度保护和用户层等待过长问题。
- 当前 adaptive in-flight limit v1：报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-inflight-v1.md`，正式结果路径为 `loadtest/results/adaptive-inflight-v1-20260609/` 和 `loadtest/results/adaptive-inflight-repeat-20260609/`。commit `7bf37fe`、`git_dirty=false`，`MaxInFlight=64` 的 60s repeat 在 1200/1600 VU 下 accepted RPS 分别为 `1922.23/1926.97`，success p99 分别为 `63.58ms/63.35ms`，outbox pending 均为 0；logical p99 仍约 `2.2s`，说明下一步瓶颈转向 retry/admission 策略而不是 DB 写入尾延迟。
- 当前 debug metrics collector 保存全量样本并在 snapshot 时排序，适合本地短压测，不适合作为生产 metrics；后续应替换为固定窗口、reservoir、HDR histogram 或 Prometheus histogram。
- `CONVERSATION_NOT_FOUND`、`MESSAGE_TOO_LARGE`、`SEQ_BLOCK_EXHAUSTED` 错误 sentinel 和 gRPC 映射暂未补齐；phase-1 普通会话 happy path 不阻塞，但不能声称完整错误契约已完成。
- 当前 raw gRPC server 还没有统一 deadline / trace / metrics interceptor；后续接 Kratos 或统一 gRPC interceptor。
- `delivery-service` SDD v0.1 已存在并已应用评审 P1，但尚未冻结；`timeline-service`、`push-gateway` SDD 未冻结；`conversation-service / member_change_saga` SDD 已冻结，Proto / Kafka schema / migration v2 和 relay builder 已补；成员变更代码必须继续通过 shared timeline/outbox append port 落库，不得绕过统一 outbox。
- `conversation-service` 当前已实现 `CreateMemberChange`、`GetMemberChange` 和最小 saga publish progress worker；DLQ repair 仍未完成。
- `conversation-service` 当前已完成 `CreateMemberChange(JOIN)` 写路径 smoke 和 `CreateMemberChange -> outbox relay -> member-change-worker -> GetMemberChange(DONE)` full smoke；`LEAVE / REMOVE / ROLE_CHANGED` 真实进程 smoke 可后置。
- 统一 outbox relay 当前仍位于 `message-service/internal/trigger/outbox`，但后续会发布 message/member 两类 conversation timeline event；这是阶段性部署折中，生产化前需要在 TADD 中决定是否拆成独立 `timeline-outbox-relay`。

## 11. 最近评审状态

- 2026-06-08：独立评审线程指出文档入口顺序、评审回传规则、GitHub 同步闭环、压测硬约束和目标态总架构入口需要补强；本轮已按建议更新本文和 `docs/README.md`。
- 2026-06-08：独立评审线程复核通过文档闭环；新增 P0 环境结论：`protoc` 可用，但 `go`、`protoc-gen-go`、`protoc-gen-go-grpc` 未检测到，正式实现和验证前必须补齐。
- 2026-06-08：已通过阿里云镜像安装 Go `1.26.4`，并通过 `GOPROXY=https://goproxy.cn,direct` 安装 `protoc-gen-go` 和 `protoc-gen-go-grpc`；按用户要求不刻意压低 Go 版本，项目基线已设为 Go `1.26.4`；`tools/gen-proto.ps1` 与 `go test ./...` 已通过。
- 2026-06-08：已实现 `SendMessageUseCase` 单元测试、领域 command hash / append record、PostgreSQL repository 本地事务；`go test ./...` 通过，`NEXUSIM_PG_DSN=postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable` 的 repository 集成测试通过。
- 2026-06-08：根据评审意见修复同一 `client_msg_id` 并发重复请求可能推进 `conversation_seq` 的问题，repository 已使用同幂等键 advisory transaction lock，并新增真实 PostgreSQL 并发集成测试；同时修复 payload JSON canonical 只压缩不稳定排序的问题。
- 2026-06-08：根据评审意见补齐 `MessagePersistedV1` payload 中的 `command_hash`，明确 `message_outbox.payload_json` 保存业务 payload、envelope/metadata 由 outbox 表字段组装；app 层已增加 `permission_version` 不一致时短重试一次，仍不一致返回可识别 dependency version error；outbox 写入失败已拆为 `ErrOutboxWriteFailed`。
- 2026-06-08：已实现 `trigger/outbox` relay、PostgreSQL outbox store 和真实 Kafka writer producer；本地已启动 `nexusim-kafka`，创建 `conversation.timeline.events` topic，并通过真实 PostgreSQL + Kafka 集成测试验证 outbox 可发布后标记 `PUBLISHED`，Kafka publish 失败时保留 pending/retry/DLQ 状态。
- 2026-06-08：独立评审线程复核 outbox relay + Kafka publish path，无 P0/P1 阻塞；P2/P3 风险已记录到本文，下一步可以提交本轮切片并推进 `message-service` gRPC adapter 与本地多线程压测。
- 2026-06-08：已实现 `message-service` gRPC adapter 和 `NEXUSIM_MESSAGE_SERVICE_MODE=grpc` 运行入口；adapter 已覆盖请求转换、响应转换、稳定错误码 detail、unsupported message type、bufconn client 注册调用。按批量策略，本轮暂不单独评审/推送，和后续 loadtest runner + smoke 结果一起评审。
- 2026-06-08：已实现 `loadtest/sendmessage` gRPC 压测 CLI，并修正 relay 有 ready event 时仍固定 sleep 的追平问题；真实进程 smoke 使用 gRPC server + outbox relay + PostgreSQL + Kafka，`--vus=2 --duration=3s --stats-wait=8s --conversation-count=100` 结果为 1020/1020 成功、p95 7.63ms、p99 9.20ms、本轮 tenant outbox 0 pending。
- 2026-06-08：已在当前 HEAD `51772e6` 跑第一轮 baseline：`--vus=100 --duration=60s --stats-wait=30s --conversation-count=1000`，结果为 45212/45212 成功、p95 249.62ms、p99 518.03ms、outbox pending 27181；已把该结果补充给评审线程作为当前提交验证证据。
- 2026-06-08：独立评审线程复核 gRPC adapter + loadtest runner，指出 P1：gRPC status/detail 暴露底层 DB/PG 错误文本；本轮已改为稳定 public message，并补 `TestSendMessageSanitizesInternalErrorMessages`。同时已补 loadtest summary 的 full commit、dirty 状态、outbox total/published/pending/DLQ 字段。
- 2026-06-08：独立评审线程复核确认 gRPC 错误脱敏 P1 已解除；同时指出当前工作区有未提交 relay worker/backoff 优化雏形。本轮已继续完善该切片，补 `NEXUSIM_OUTBOX_WORKERS` / `NEXUSIM_OUTBOX_FAILURE_BACKOFF` wiring、worker 并发测试、失败退避测试，并跑 4 worker 真实 baseline：69608/69608 成功，p95 122.10ms，p99 156.24ms，`stats-wait=30s` 后 pending 2123，额外 drain 20s 后清零。
- 2026-06-08：独立评审线程复核 `cef25f1 feat: add outbox relay worker controls`，结论为无 P0/P1，可作为 relay 追平能力优化切片保留；P2 跟踪项包括 clean commit baseline 归档、部分成功大量失败场景的退避策略、真实 PostgreSQL 多 worker / `SKIP LOCKED` 集成测试、worker 上限与配置日志。
- 2026-06-08：已补本地 debug metrics collector，gRPC 进程记录 `conversation_seq_alloc_latency`，relay 进程记录 `kafka_publish_latency`；`loadtest/sendmessage` 支持 `--service-metrics-url` 和 `--relay-metrics-url` 并把 avg/p95 写入 summary。commit `ea4eb9a` 的 clean metrics smoke 已验证两个指标非空。
- 2026-06-08：已补真实 PostgreSQL 多 worker / `FOR UPDATE SKIP LOCKED` 集成测试：两个 conversation 各 3 条 outbox，4 个并发 worker `ProcessReady(limit=1)`，断言跨 conversation 可同时进入 publish callback、同 conversation 发布顺序保持 `1,2,3`，最终 outbox 全部 `PUBLISHED`。
- 2026-06-08：已在 clean HEAD `0ff42d2` 跑 4 worker 长 baseline，summary 为 24714/24714 成功、p95 436.24ms、p99 583.96ms、`stats-wait=30s` 后 pending 392；随后 DB 查询显示该 tenant 已全部 published。由于该结果与 dirty baseline 吞吐差异较大，后续需要重复 clean baseline 或按 4/8/16 worker 梯度压测确认趋势。
- 2026-06-08：已补 `loadtest/sendmessage/run-local-gradient.ps1`，用于本机按 worker 数循环启动真实 gRPC / outbox relay / loadtest 进程并采集 metrics；dirty smoke 已验证 4/8/16 worker 短梯度脚本可运行。
- 2026-06-09：已提交 `e87bb9b feat: add multi-instance loadtest diagnostics`，补齐 repository 内部分段指标、多 target loadtest、多 service metrics URL、`run-local-pgpool-gradient.ps1` 和 `run-local-multi-instance.ps1`；`go test ./...`、`go build ./services/message-service/cmd/message-service ./loadtest/sendmessage`、PowerShell 脚本语法检查和 `git diff --check` 均已通过。
- 2026-06-09：已在 clean commit `e87bb9b` 跑 PG pool 短 smoke 和 multi-instance 短 smoke，验证 summary 能记录 repository 分段指标和多实例 service metrics；该结果只作为工具链验证，不作为正式容量结论。下一步必须跑正式长矩阵后再申请阶段评审。
- 2026-06-09：已跑正式 PG pool 矩阵和正式 multi-instance 矩阵，并更新 `docs/runbook/loadtest/message-service/loadtest-report-20260609-pgpool-multi-instance.md`。阶段结论：当前证据显示 p99 主要贴在 `repository_begin`，`PG_MAX_CONNS=96/128` 超出 PostgreSQL 连接上限；在当前单 PostgreSQL、当前连接上限和当前写入模型下，多 `message-service` 实例没有降低尾延迟。下一步应转入 PostgreSQL 观测、写入路径优化、outbox relay 追平和 backpressure 设计。
- 2026-06-09：已新增并执行 `collect-postgres-diagnostics.ps1`，确认 PostgreSQL 当前 `max_connections=100`，解释 PG pool 96/128 档位不可用；已把正式矩阵和诊断结果发送给评审线程做阶段复核，本轮不频繁 push。
- 2026-06-09：评审线程阶段性指出多实例 summary 顶层 `service_latency_metrics` 只取第一个 service metrics URL，报告若直接使用会误导。本轮已修正 loadtest 聚合逻辑，并把报告表改为基于 `service_metrics[]` 的 per-instance min/max。
- 2026-06-09：已扩展 `run-local-multi-instance.ps1`，支持固定每实例 PG 连接预算和固定总 PG 连接预算两类对照实验；下一轮可直接跑 `FixedPerInstance` 与 `FixedTotal` 矩阵，避免实例数和总数据库连接预算混在一起。
- 2026-06-09：已用重建后的 loadtest 二进制跑 `FixedTotal` 短 smoke：`1x8` 与 `2x4` 两组均 100% 成功、outbox pending 0，summary 顶层 `service_pg_pool.max_conns` 分别为 `8` / `8`，证明多实例 PG pool 聚合和固定总预算脚本路径生效。
- 2026-06-09：已在 clean commit `ede5dd7` 跑正式 multi-instance PG budget 矩阵，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-multi-instance-budget.md`。结论：固定总 PG 连接预算时，1 个实例 p99 最低，2/4 实例没有收益；request p99 仍贴近 repository append/begin，outbox pending 暴露 relay 追平为第二瓶颈。矩阵结束后已额外 drain，DB outbox 当前全部为 `PUBLISHED`。
- 2026-06-09：已把 `repository_begin` 拆成 `repository_pool_acquire_latency_ms` 和 `repository_tx_begin_latency_ms`，原 `repository_begin_latency_ms` 保持总耗时用于兼容旧报告。clean smoke commit `c10338e`：`PG_MAX_CONNS=8`、`VU=5`、`duration=3s`，1833/1833 成功，outbox pending 0，两个新指标 count 均为 1833，`git_dirty=false`。
- 2026-06-09：已新增 `watch-postgres-diagnostics.ps1`，用于压测期间按间隔采集 PostgreSQL `pg_stat_activity` wait_event、锁等待、表 dead tuples、bgwriter 和 WAL 统计，输出 `postgres-wait-samples.jsonl`。under-load smoke：`PG_MAX_CONNS=8`、`VU=20`、`duration=5s`，采样 10 次，最大 active backend 为 8，抓到 `LWLock:WALWrite` 等 wait_event。
- 2026-06-09：已新增 `deploy/local/docker-compose.postgres-loadtest.yml` 作为压测专用 PostgreSQL override，不改变默认开发 compose；目标参数包括 `max_connections=200`、`shared_buffers=1GB`、`max_wal_size=4GB`、`checkpoint_timeout=15min` 和更积极的 autovacuum 阈值。
- 2026-06-09：已实际启用 PostgreSQL loadtest override 并跑正式 PG pool 矩阵，同时采集 wait_event。结论：调大 PostgreSQL 与 PG pool 不能单独解决 p99，`repository_pool_acquire` 仍是主等待段；`PG_MAX_CONNS=128` 会放大 commit/WAL/outbox 压力。下一步优先做 backpressure 和 outbox relay 批量优化。
- 2026-06-09：已实现默认关闭的 PostgreSQL pool backpressure，并新增 `SERVICE_OVERLOADED` 错误码。clean smoke commit `78e8375`：`NEXUSIM_PG_BACKPRESSURE_ENABLED=true`、`PG_MAX_CONNS=1`、`VU=20`、`duration=5s`，163055 请求中成功率 `0.0032`，p99 `1.6246ms`，top error 为 `Unavailable: service overloaded`，outbox pending 0；报告为 `docs/runbook/loadtest/message-service/loadtest-report-20260609-backpressure.md`。
- 2026-06-09：已为 loadtest summary 增加 `retryable_error_count`、`service_overloaded_count` 和 `message_error_counts[]`。clean smoke commit `a9fbdf8`：`PG_MAX_CONNS=1`、`VU=10`、`duration=3s`，`retryable_error_count=62556`、`service_overloaded_count=62556`、`message_error_counts[0]=SERVICE_OVERLOADED`，outbox pending 0。
- 2026-06-09：评审线程指出正式 backpressure on/off 矩阵不能只看混合 p99，否则大量快速拒绝会掩盖成功写入体验。本轮已补 `success_p99_ms`、`error_p99_ms`、`accepted_rps`、`error_rps`、`overload_rate`，并用 clean commit `6f0aa55` 重跑 on/off 矩阵；新报告明确区分整体 p99、成功 p99 和错误 p99。
- 2026-06-09：已在 clean commit `dfb6776` 跑 `MinAvailableConns=0/4/8/16` backpressure 梯度，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-backpressure-gradient.md`。结论：`MinAvailable=0` 拒绝太晚，`MinAvailable=4/8` 的错误语义更稳定；`8` 只是下一轮优先验证的候选之一，同时开始设计 adaptive limit 和客户端退避策略。
- 2026-06-09：已为 gRPC `SERVICE_OVERLOADED` 增加标准 `RetryInfo` detail，当前固定建议延迟为 `500ms`；`MessageError` detail 保持原错误码、retryable 和 correlation id。后续可让 retry delay 跟随 adaptive limit 动态调整。
- 2026-06-09：loadtest 已支持可选 `--retry-overloaded --max-retries=N --retry-jitter=D`，用于模拟客户端遵守服务端 `RetryInfo` 后的有限重试；summary 会同时记录实际 gRPC attempts 和用户层 logical request，避免把重试次数误读成消息吞吐。
- 2026-06-09：已在 clean commit `0c542a1` 跑客户端遵守 `RetryInfo` 的正式矩阵，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-client-retry.md`。结论：客户端 retry 后错误语义稳定、logical success rate 提升，但 accepted RPS 回升使 outbox pending 重新积压；下一阶段优先做 outbox relay 批量优化。
- 2026-06-09：已完成 outbox relay 第一项低风险优化：成功发布的 outbox id 在同一事务内批量 mark `PUBLISHED`，并补真实 PostgreSQL 集成测试。该改动不改变 Kafka publish 顺序，也不改变至少一次投递语义。
- 2026-06-09：已在 clean commit `b6f0b82` 跑 batch mark 后的 client retry 对照矩阵，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-batchmark.md`。结论：pending 明显下降，但容量提升需要重复矩阵或进一步 batch publish 验证。
- 2026-06-09：独立评审线程复核 backpressure 口径修复与正式压测报告，结论为无 P0/P1；`success/error latency` 拆分已解除上一轮 P1。非阻塞建议：`MinAvailableConns=8` 只能写作下一轮候选之一，`RetryInfo=500ms` 是第一版固定 hint，不是验证出的最优值。
- 2026-06-09：独立评审线程复核 client retry + outbox batch mark published，结论为无 P0/P1；本轮已修正 `current-goal.md` 残留旧状态，并把 `overload_rate` 明确为 attempt-level 指标。非阻塞建议中的混合 batch 成功/失败测试已补充。
- 2026-06-09：已新增 relay 分段 metrics，覆盖 `ProcessReady` 总耗时、ready outbox fetch、批量 mark published 和 outbox commit；clean smoke commit `148938e` 已验证真实进程 summary 能读到这些指标。
- 2026-06-09：已跑 outbox batch size 100/500/1000 的 30 秒探索矩阵，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-outbox-batchsize-smoke.md`。结论：三组均可追平，batch 500 暂作为下一轮正式矩阵候选，但不能跳过重复验证。
- 2026-06-09：已实现 `OutboxStore.ProcessReadyBatch` 与 Kafka `WriterProducer.PublishBatch`，并在 clean commit `9f26d0c` 跑两轮短 smoke。结论：真实链路可运行且 outbox 可追平，`outbox_process_ready_latency_ms` 在第二轮明显下降；trigger 层已覆盖 Kafka batch error 和单条 payload build error；但单次波动大，需要正式重复矩阵和评审后再宣称阶段完成。
- 2026-06-09：根据评审 P2 已拆分 Kafka publish metrics，新增 `kafka_publish_call_latency_ms`、`kafka_publish_records_per_call`、`kafka_publish_record_latency_estimate_ms`；旧 `kafka_publish_latency_ms` 仅保留兼容。同时修复 `ProcessReadyBatch` 空批次仍调用 publish callback 的问题，并补直接调用 `ProcessReadyBatch` 的真实 PostgreSQL 混合成功/失败测试。
- 2026-06-09：已新增 `NEXUSIM_OUTBOX_PUBLISH_BATCH_ENABLED` 开关和 `run-local-pgpool-gradient.ps1 -PublishBatchEnabled` 参数，支持同一 HEAD 做 PublishBatch on/off 对照；正式矩阵已重建当前 HEAD 二进制后重跑，避免旧 `bin` 污染实验。
- 2026-06-09：已新增 `run-local-outbox-batch-worker-matrix.ps1`，并跑 batch size 100/500/1000 与 workers 8/12/16 在 1200/1600 VU 下的联合矩阵。结论：当前所有组合均可追平 outbox，workers 8 比 12/16 更稳；下一步重复验证 `100/8` 与 `500/8`。
- 2026-06-09：已按评审 P2 补 outbox relay active/idle 指标和 `outbox_fetched_per_call`，并用 clean smoke 验证 summary 能读取这些字段。下一轮候选重复矩阵和 adaptive limit 设计应优先使用 active 指标，而不是混合 `outbox_process_ready_latency_ms`。
- 2026-06-09：已用 active/idle metrics 重复验证 outbox 候选 `100/8` 与 `500/8`。结论：两者均可追平，`100/8` 的 success p99 更稳，作为下一轮 adaptive limit 本地 relay 基线。
- 2026-06-09：已实现默认关闭的 app 层 adaptive admission controller，输入覆盖 PG pool、repository pool acquire p95、outbox pending、relay active process ready、outbox fetched per call、Kafka records per call；clean smoke `d2af748` 证明 `SERVICE_OVERLOADED` 能在依赖读取和写事务之前返回，且不写 outbox。下一步跑 adaptive on/off 或阈值梯度正式矩阵。
- 2026-06-09：已跑 adaptive limit on/off 对照矩阵，并在过程中修复两类误判：idle relay 的 `outbox_fetched_per_call=0` 不能触发拒绝；全量累计 `repository_pool_acquire_p95` 不能作为独立硬门槛。最终 relaxed app adaptive 与 repository backpressure 行为接近，outbox 均可追平，但未证明容量提升。下一步先做窗口化 metrics、hysteresis 和动态 retry hint，再跑下一轮 adaptive 阈值矩阵。
- 2026-06-09：已为 debug metrics collector 增加最近 4096 个样本的 recent 视图，并让 adaptive controller 优先读取 recent 字段；clean smoke `6d4910b` 证明 loadtest summary 能读取 recent 指标。下一步实现 hysteresis 和动态 retry hint。
- 2026-06-09：已为 adaptive admission 增加 hysteresis 配置，支持 `ReleaseAvailableConns` 和 `ReleaseOutboxPending`；clean smoke `6f9a438` 证明低压真实链路正常。下一步把固定 `RetryInfo=500ms` 改成动态 retry hint，并用 recent+hysteresis 跑正式阈值矩阵。
- 2026-06-09：已为 `SERVICE_OVERLOADED` 增加可携带 retry delay 的错误类型，adaptive controller 会根据过载原因数量生成动态 retry hint；clean smoke `31408e7` 验证真实进程链路可运行。随后在 `c9e6cf1` 为 loadtest summary 增加 retry delay histogram，并用真实进程 smoke 验证 `retry_delay_p95_ms=500`。下一步跑正式 adaptive 阈值矩阵。
- 2026-06-09：独立评审线程复核 adaptive admission / recent metrics / hysteresis / dynamic RetryInfo / retry delay histogram，结论为无 P0/P1，不阻塞继续做 adaptive threshold matrix。P2 已记录：relay 条件依赖 outbox pending 采样、recent 是样本窗口不是时间窗口、`retry_delay_count` 是收到并计划遵守的 hint 数量、dynamic retry hint 仍是启发式控制信号。
- 2026-06-09：已在 clean commit `9830f34` 跑 adaptive threshold matrix v1，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-threshold-v1.md`。结论：`RetryBase=1000ms` 的 `gap8-outbox50-base1000` 当前最好，outbox 全部追平；pool release gap 和 outbox release ratio 未显示稳定收益。下一步做 60s 重复验证和 pool acquire p95 阈值矩阵。
- 2026-06-09：已在 clean commit `fb872a9` 对 `gap8-outbox50-base1000` 做 60s 重复验证，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-best-repeat.md`。结论：该配置能保持 outbox 清零，但 accepted RPS 过低，不能作为稳定配置。随后已给 loadtest summary 补 logical end-to-end latency；下一步做 `AdaptiveMaxPoolAcquireP95=250/500/750ms` 阈值矩阵。
- 2026-06-09：已在 clean commit `91cbb8c` 跑 logical latency smoke，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-logical-latency-smoke.md`。结论：attempt p99 与 logical p99 差异明显，后续 adaptive 矩阵必须用 logical p99 判断用户层等待。
- 2026-06-09：已在 clean commit `b98e3c9` 跑 `AdaptiveMaxPoolAcquireP95=250/500/750ms` 矩阵，并新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-poolp95-v1.md`。结论：outbox 全部清零，但 accepted RPS 偏低、logical p99 仍为 5s 以上；下一步应设计 admission token / concurrency limit，而不是继续只调 p95 阈值。
- 2026-06-09：已实现 app 入口 `NEXUSIM_ADAPTIVE_MAX_IN_FLIGHT` token / concurrency gate，并在 clean commit `7bf37fe` 跑 30s cap 梯度和 60s 候选重复验证；新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-adaptive-inflight-v1.md`。结论：`MaxInFlight=64` 当前最稳，accepted RPS 回到约 `1.92k` 且 outbox 可清零，但 logical p99 仍约 `2.2s`；下一步调 retry delay / max retries，而不是继续放宽 token 上限。
- 2026-06-09：根据用户反馈，message-service 不再继续做大规模压测矩阵；已新增 `docs/runbook/loadtest/message-service/loadtest-report-20260609-message-service-consolidated.md`，整合 27 份原始压测报告、瓶颈排查过程和面试可讲要点。下一步转向第二个真实微服务，优先 `conversation-service`。
- 2026-06-09：已按“每个微服务一个压测报告文件夹”的规则整理 `message-service` 压测报告；所有小报告和总报告均归档到 `docs/runbook/loadtest/message-service/`，目录入口为 `docs/runbook/loadtest/message-service/README.md`。
- 2026-06-09：已将 `docs/runbook/loadtest/message-service/README.md` 从简单索引扩展为 `message-service` 压测结论和面试材料入口，包含真实链路范围、核心压测数字、瓶颈排查路径、outbox/admission 结论和面试讲法。
- 2026-06-09：已开始 `conversation-service` 最小真实 read path：新增 SDD、`conversation_service.proto`、conversation PostgreSQL migration、六层 DDD 骨架、`GetSendContext` app/domain/postgres/api 实现，并为 `message-service` 增加可选 `NEXUSIM_CONVERSATION_SERVICE_ADDR` gRPC client，用于替换 strict conversation mock。
- 2026-06-09：已完成 `conversation-service` 第一轮真实进程 smoke：启动 `conversation-service` 和 `message-service`，由 `message-service` 通过 gRPC 调用真实 `GetSendContext`，`loadtest/sendmessage --vus=2 --duration=3s` 结果为 725/725 成功、p95 10.36ms、p99 13.26ms；本轮未启动 outbox relay，测试结束后已清理 `tenant-conv-smoke` 数据，报告归档到 `docs/runbook/loadtest/conversation-service/`。
- 2026-06-09：已补 `conversation-service` 错误路径测试，覆盖 domain inactive conversation/member、app command validation 和 repository error propagation、gRPC 稳定错误 message、真实 PostgreSQL missing/archived/left/member missing 场景；已新增 `docs/runbook/conversation-service-local.md` 作为本地启动与 smoke 说明。
- 2026-06-09：独立评审线程复核 `conversation-service` 最小 read path，结论为无 P0，但指出 P1：参数缺失会被映射为 Internal。本轮已新增 `ErrInvalidArgument`，`Validate()` 返回稳定 sentinel，gRPC 映射为 `InvalidArgument`，并补 tenant/conversation/user 缺失单测；同时补 `message-service` ConversationClient 响应 tenant/conversation/enum/current_seq_shard 契约校验，以及 conversation dependency unavailable 的一次短重试。
- 2026-06-09：已冻结 `docs/sdd/conversation-service-member-change-saga.md`，明确成员变更 Saga 采用目标架构方案 C：成员边界事件与 message event 共享 `conversation_seq`、`conversation_timeline_events` 和 outbox 流；编码前必须补 `CreateMemberChange` proto、member boundary Kafka oneof payload、saga retry/DLQ migration 字段和本地 smoke runbook。
- 2026-06-09：已落地成员变更第一批公共契约：`conversation_service.proto` 新增 `CreateMemberChange` / `GetMemberChange`、`conversation.timeline.events.proto` 新增 member boundary oneof payload、`000002_member_change_saga_v2.sql` 新增 saga retry/DLQ/metadata/`timeline_event_id`/`outbox_event_id` 字段；outbox relay builder 已支持 `conversation.member.*` 且覆盖 unsupported fail-closed；下一步补 shared append port 和最小 `CreateMemberChange` 写路径。
- 2026-06-09：已实现 `conversation-service` 最小 `CreateMemberChange` 写路径，包含 app/domain/types、gRPC adapter、cmd wiring、PostgreSQL repository 同事务写 saga/member/version/timeline/outbox，以及 v3 saga event id 唯一约束；`go test ./...`、关键二进制 build、真实 PostgreSQL `CreateMemberChange` 集成测试均通过。下一步做真实进程 smoke 和阶段评审。
- 2026-06-09：独立评审线程复核最小 `CreateMemberChange` 写路径，结论无 P0；P1 指出工作区未跟踪 smoke runner 和成员权限矩阵过宽。本轮已提交 `loadtest/memberchange` runner，收紧权限矩阵：`LEAVE` 只允许本人，`OWNER` 不操作另一个 `OWNER`，`ADMIN` 只添加/移除普通 `MEMBER`，`ROLE_CHANGED` 第一版仅 `OWNER` 可在 `ADMIN/MEMBER` 间调整；`MERGE/COMPENSATE` 暂不接受。
- 2026-06-09：已完成 clean HEAD `71c04c9` 的 `CreateMemberChange(JOIN) -> outbox relay -> Kafka member event` 真实进程 smoke：279/279 成功，p99 24.95ms，saga/timeline/outbox 各 279 条，outbox `PUBLISHED=279`、`PENDING=0`、`DLQ=0`；报告归档到 `docs/runbook/loadtest/conversation-service/loadtest-report-20260609-member-change-smoke.md`。
- 2026-06-09：已实现 `conversation-service` 的 `GetMemberChange` 查询接口和 saga publish progress worker：`NEXUSIM_CONVERSATION_SERVICE_MODE=member-change-worker` 会观察 outbox `PUBLISHED` 事件并把 `member_change_saga` 推进到 `DONE`；`go test ./...`、关键二进制 build 和真实 PostgreSQL repository 集成测试均通过。下一步做真实进程 smoke 和阶段评审。
- 2026-06-09：已完成 clean HEAD `ca0a0b6` 的 conversation-service member change full smoke：`CreateMemberChange -> outbox relay -> Kafka member event -> member-change-worker -> GetMemberChange(DONE)`，350/350 成功，p99 40.90ms，`outbox_published_count=350`、`outbox_pending_count=0`、`saga_done_count=350`，样本 `GetMemberChange` 返回 `MEMBER_CHANGE_STATUS_DONE`；报告归档到 `docs/runbook/loadtest/conversation-service/loadtest-report-20260609-member-change-full-smoke.md`。下一步邀请阶段评审。
- 2026-06-09：独立评审指出 `GetMemberChange` 读取授权和 `last_error` 脱敏两个 P1；本轮已修复：repository 校验操作者/目标用户/当前 ACTIVE OWNER 或 ADMIN，未授权返回 `ErrPermissionDenied`；raw `last_error` 映射为稳定 public message；`MarkPublishedMemberChanges` 额外校验 outbox conversation、producer 和 member event type。当前 HEAD `76fff53` 短 full smoke 217/217 成功，`saga_done_count=217`，`sample_get_status=MEMBER_CHANGE_STATUS_DONE`。
- 2026-06-09：独立评审复核 `conversation-service` P1/P2 修复，结论为无 P0/P1，可进入 `delivery-service / push-gateway` SDD 和最小链路；当前已新增 `docs/sdd/delivery-service.md` v0.1 Draft，范围限定为消费 `conversation.timeline.events`、投影 `user_inbox`、提供 `PullInbox / AckDelivery`，不先实现 WebSocket。
- 2026-06-09：独立评审指出 `delivery-service` SDD v0.1 有 3 个 P1：缺成员可见性投影、ACK 可推进到未来 seq、Kafka checkpoint 维度错误。本轮已修 SDD 和 migration：新增 `delivery_membership_projection`，ACK 必须 `received_seq <= user_inbox max_visible_seq`，Kafka checkpoint 改为 `consumer_group + topic + partition`；同步落地 delivery proto / migration / 六层骨架、`PullInbox / AckDelivery` 最小同步路径和 `ProjectTimelineEventUseCase` / PostgreSQL projection 方法。
