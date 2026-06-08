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
| Docker Compose | `deploy/local/docker-compose.yml` 已存在；压测专用 PostgreSQL override 为 `deploy/local/docker-compose.postgres-loadtest.yml` |
| message-service 六层骨架 | `services/message-service/internal/{api,app,domain,infrastructure,types,trigger}` 已存在 |
| Go 工具链 | 项目基线为 Go `1.26.4`；已通过阿里云镜像安装到 `C:\Users\10495\.local\go\go1.26.4\bin\go.exe`；`protoc-gen-go v1.36.11` 和 `protoc-gen-go-grpc 1.6.2` 已安装到 `C:\Users\10495\go\bin`；`protoc` 可用，路径为 `C:\Users\10495\anaconda3\Library\bin\protoc.exe`；本地命令先执行 `. .\tools\go-env.ps1` |
| Proto Go 代码 | 已生成 `api/proto/nexusim/message/v1/*.pb.go` 和 `schemas/kafka/conversation.timeline.events.pb.go` |
| Go 依赖 | `go.mod` 使用 Go `1.26.4`，并已引入 `google.golang.org/grpc v1.81.1`、`google.golang.org/protobuf v1.36.11` |
| SendMessage app/domain | 已补 `SendMessageUseCase` 单元测试、permission version 一致性短重试、稳定 JSON canonical command hash、append record 构造 |
| PostgreSQL repository | 已实现普通会话 `SendMessage` 本地事务：幂等检查、同幂等键 advisory transaction lock、`conversation_seq` row lock、`message_log`、`conversation_timeline_events`、`message_outbox` 同事务写入；outbox payload 对齐 `MessagePersistedV1` 业务 payload；集成测试和并发重复请求测试通过 |
| Outbox relay / Kafka publish path | 已实现 `trigger/outbox` relay、PostgreSQL outbox store、Kafka writer producer；relay 支持 `NEXUSIM_OUTBOX_WORKERS` 多 worker 与 `NEXUSIM_OUTBOX_FAILURE_BACKOFF` 失败退避；真实 PostgreSQL + Kafka 集成测试通过；真实 PostgreSQL 多 worker / `FOR UPDATE SKIP LOCKED` 测试已覆盖同 conversation 顺序和跨 conversation 并发 |
| message-service gRPC adapter | 已实现 `SendMessage` gRPC handler、proto request/response 转换、稳定错误码 detail 映射和错误 message 脱敏、`NEXUSIM_MESSAGE_SERVICE_MODE=grpc` 运行入口；支持 `NEXUSIM_DEBUG_ADDR=/debug/metrics` 暴露本进程压测指标；已通过 bufconn client 单测 |
| Backpressure | 已新增 `MESSAGE_ERROR_CODE_SERVICE_OVERLOADED`，repository 支持默认关闭的 PostgreSQL pool backpressure；启用 `NEXUSIM_PG_BACKPRESSURE_ENABLED=true` 后，连接池可用连接数小于等于 `NEXUSIM_PG_BACKPRESSURE_MIN_AVAILABLE_CONNS` 时快速返回 retryable `service overloaded` |
| SendMessage loadtest | 已实现 `go run ./loadtest/sendmessage` 参数化 gRPC 压测入口；支持 `target`、`vus`、`duration`、`result-dir`、`pg-dsn`、`stats-wait`、`service-metrics-url`、`relay-metrics-url`；`target` 和 service metrics URL 已支持逗号分隔，用于模拟多 `message-service` 实例；summary 记录 full commit、dirty 状态、outbox total/published/pending/DLQ、SendMessage/repository/commit/seq/Kafka latency、service/relay pgx pool、repository 内部分段指标、多进程 metrics、retryable error count、service overloaded count、`message_error_counts[]`、`request_rps`、`accepted_rps`、`error_rps`、`overload_rate`、`success_p99_ms` 和 `error_p99_ms`；多 service metrics 的顶层 latency / pg pool 已改为聚合视图，避免只取第一个实例误导；`run-local-multi-instance.ps1` 已支持 `FixedPerInstance` 和 `FixedTotal` 两种 PG 连接预算模式；`run-local-pgpool-gradient.ps1` 已支持显式 `-BackpressureEnabled`；已补 `run-local-gradient.ps1`、`run-local-pgpool-gradient.ps1`、`run-local-multi-instance.ps1`、`collect-postgres-diagnostics.ps1`、`watch-postgres-diagnostics.ps1`；真实 gRPC + PostgreSQL + outbox relay + Kafka smoke、baseline、瓶颈诊断、PG pool / multi-instance 矩阵、PostgreSQL 诊断和 backpressure on/off 矩阵已执行 |

## 5. 下一步优先级

1. 当前 Codex 进程如果仍找不到 `go`，先执行 `. .\tools\go-env.ps1`。
2. 基于已完成的 backpressure on/off 矩阵，继续跑 `MinAvailableConns=0/4/8/16` 梯度，比较成功率、accepted RPS、success p99、error p99、outbox pending 和 overload rate。
3. 根据 backpressure 梯度结果设计更细的 adaptive limit，而不是长期使用瞬时 acquired conns 判断。
4. 评估 outbox relay 追平优化：批量 publish、批量 mark published、batch size、worker 数和故障退避；避免高写入吞吐下 pending 快速增加。
5. 继续采集 PostgreSQL wait_event，重点看 `LWLock:WALWrite`、`LWLock:WALInsert`、`LWLock:BufferContent` 和 `CheckpointWriteDelay`。
6. 视评审复核结果决定是否推送 GitHub。

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

每个阶段必须新增一份独立压测报告，不覆盖旧报告。报告放在 `docs/runbook/`，推荐命名：

```text
loadtest-report-YYYYMMDD-<stage>.md
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
- 当前 outbox relay 的 publish callback 在 PostgreSQL 事务内执行，这是第一阶段可接受的至少一次发布取舍；压测阶段需要重点观察 batch size、Kafka publish latency、DB lock wait 和重复发布窗口。
- 当前 relay 只支持 `message.persisted.v1`；启用 Edit/Revoke/Delete 前必须补齐对应 Kafka oneof payload 构造和测试。
- OutboxStore 已补真实 PostgreSQL 多 worker / `FOR UPDATE SKIP LOCKED` 集成测试，证明同 conversation 顺序不乱、跨 conversation 可并发追平；后续仍可补强 `available_at/next_retry_at` 未到期、低版本 PENDING 阻塞等 ready 条件测试。
- 当前 relay 支持可配置多 worker；只有 `Published > 0` 时才立即继续循环，`Fetched > 0` 但没有成功发布时按 `FailureBackoff` 退避，空转时按 `PollInterval` sleep；同一 conversation 仍按最低 `aggregate_version` 串行发布，因此单会话积压追平能力仍受顺序保护限制。
- 当前 relay 已缓解 Kafka 故障且 backlog 很大时的连续失败放大，但生产化仍需要结合失败比例、Kafka publish latency、DB lock wait 调整 backoff、batch size 和 worker 数。
- 当前 ready 判断使用 DB `now()`，retry 时间写入使用应用时钟；生产硬化时需要统一时间源或明确 DB/relay 节点时钟同步要求。
- 当前尚未实现 DLQ repair/replay；未来实现时必须清理 `dead_lettered_at`、`last_error`、`next_retry_at` 等旧失败字段。
- 已有真实进程级 SendMessage smoke 和 `--vus=100 --duration=60s` baseline 结果；baseline 写入成功率 100%，但 p99 与 outbox backlog 暴露出性能风险。
- 压测 summary 已支持从 gRPC 进程和 relay 进程的 `/debug/metrics` 读取 `conversation_seq_alloc_latency_ms` 与 `kafka_publish_latency_ms`；生产化仍需接入统一 metrics/tracing，而不是依赖本地 debug endpoint。
- 当前 `51772e6` baseline：45212/45212 成功，p95 249.62ms，p99 518.03ms，`stats-wait=30s` 后本轮 tenant outbox `PENDING=27181`、`PUBLISHED=18031`。
- 当前 worker/backoff dirty baseline：`NEXUSIM_OUTBOX_WORKERS=4`、`--vus=100 --duration=60s --stats-wait=30s --conversation-count=1000`，69608/69608 成功，p95 122.10ms，p99 156.24ms，`stats-wait=30s` 后本轮 tenant outbox `PENDING=2123`、`PUBLISHED=67485`；relay 额外 drain 20s 后该 tenant outbox 全部 `PUBLISHED=69608`。该数据用于开发判断，不能作为 clean commit 正式性能归档。
- 当前 metrics clean smoke：commit `ea4eb9a`，`--vus=10 --duration=10s --stats-wait=10s --conversation-count=200`，8699/8699 成功，p95 19.46ms，p99 31.68ms，outbox pending 0；summary 已写入 `conversation_seq_alloc_latency_ms=1.47`、`conversation_seq_alloc_p95_ms=2.59`、`kafka_publish_latency_ms=1.01`、`kafka_publish_p95_ms=1.73`，且 `git_dirty=false`。
- 当前 worker/backoff clean baseline：commit `0ff42d2`，`NEXUSIM_OUTBOX_WORKERS=4`、`--vus=100 --duration=60s --stats-wait=30s --conversation-count=1000`，24714/24714 成功，p95 436.24ms，p99 583.96ms，summary 读取时本轮 tenant outbox `PENDING=392`、`PUBLISHED=24322`、`DLQ=0`，`conversation_seq_alloc_latency_ms=6.98`、`kafka_publish_latency_ms=4.21`，且 `git_dirty=false`；随后查询该 tenant outbox 已全部 `PUBLISHED=24714`。该结果说明 4 worker 能追平，但本机长压测吞吐波动明显，需要重复 clean baseline 或梯度压测后再形成正式性能结论。
- 当前 gradient script dirty smoke：`run-local-gradient.ps1 -Workers 4,8,16 -VUs 20 -Duration 10s -StatsWait 10s -ConversationCount 500` 已跑通三组真实进程压测，全部成功且 outbox pending 0；由于脚本未提交，summary 为 `git_dirty=true`，仅作为脚本可运行验证，不作为正式梯度趋势证据。
- 当前 Windows Docker Desktop 已按用户要求调整为 WSL2 `processors=16`、`memory=24GB`、`swap=8GB`，`docker info` 显示 Docker VM 可用 `16 CPU`、约 `23.47GiB` 内存。
- 当前 win-win Docker 资源矩阵已覆盖 `1/2/4 CPU + 256m/512m/1g`、`8/12/16 CPU + 2g/4g/8g`、以及 `16 CPU + 23g` 档位。按 `success_rate >= 0.99`、`p99 <= 1000ms`、`outbox_pending_count <= 1000` 的门槛，已观察到的最佳通过档为 `16 CPU / 23g / relay workers=8 / 1200 VU`，约 `2493 rps`、p99 `736.28ms`、outbox pending 0；`1600 VU` 时 p99 `1120.48ms` 超线。`16 CPU / 23g / relay workers=16` 在 `1200 VU` 即 p99 `1477.19ms` 超线，说明盲目增加 relay worker 会放大争用。
- 当前 Windows+Mac 分布式双客户端已跑通：Windows 服务端暴露 `10495/10497/10500`，Windows 和 Mac 同时作为 load generator。`600+600 VU` 双客户端全部成功，Windows client p99 `730.11ms`、Mac client p99 `739.50ms`、outbox pending 0；`1000+1000 VU` 双客户端全部成功但 p99 均约 `1331ms`，按当前尾延迟门槛超线。
- 当前压测趋势图已生成到 `loadtest/results/charts/`：`winwin-rps-trend.png`、`winwin-p99-trend.png`、`distributed-clients-trend.png`，摘要为 `loadtest/results/charts/winwin-distributed-summary.md`；这些结果文件默认不提交。
- 当前压测正式报告为 `docs/runbook/loadtest-report-20260609.md`，已记录压测拓扑、执行方式、通过标准、结果摘要和瓶颈排查过程。
- 当前 PG pool / multi-instance 诊断报告为 `docs/runbook/loadtest-report-20260609-pgpool-multi-instance.md`，已记录 repository 细分指标、多 target loadtest、多 service metrics URL、PG pool smoke、multi-instance smoke、正式 PG pool 矩阵和正式 multi-instance 矩阵。
- 当前 p99 瓶颈已初步定位：`16 CPU / 23g / 1600 VU / workers=8` 下，request p99 基本等于 repository append p99；commit、conversation_seq、Kafka publish 都是毫秒级；service pgxpool 默认 `max_conns=16` 时 acquire 平均等待约 `646ms`，`NEXUSIM_PG_MAX_CONNS=64` 后 1600 VU p99 改善到 `779.63ms`，但 2400 VU 仍 p99 `1452.87ms` 超线。下一步优先做 PG 连接池梯度、多 message-service 实例和 repository 细分打点。
- 当前 repository 细分指标已落地：`repository_begin_latency_ms`、`repository_pool_acquire_latency_ms`、`repository_tx_begin_latency_ms`、`repository_idempotency_lock_latency_ms`、`repository_find_existing_latency_ms`、`repository_ensure_seq_latency_ms`、`repository_allocate_seq_latency_ms`、`repository_insert_message_latency_ms`、`repository_insert_timeline_latency_ms`、`repository_insert_outbox_latency_ms`、`repository_commit_latency_ms`。
- 当前 clean commit `e87bb9b` PG pool smoke：`PG_MAX_CONNS=16/64`、`VU=20`、`duration=5s`、`stats-wait=5s`，两组全部成功；p99 分别为 `42.52ms`、`33.46ms`，结果在 `loadtest/results/pgpool-smoke-20260609-013424/`，后续短 relay drain 后对应 outbox 均为 `pending=0`。
- 当前 clean commit `e87bb9b` multi-instance smoke：`Instances=1/2`、`VU=20`、`duration=5s`、`stats-wait=5s`，两组全部成功；p99 分别为 `40.43ms`、`39.35ms`，结果在 `loadtest/results/multi-instance-smoke-20260609-013511/`，多 target 和多 service metrics URL 均已验证；后续短 relay drain 后对应 outbox 均为 `pending=0`。
- 当前 formal PG pool 矩阵：`loadtest/results/pgpool-formal-20260609-014259/`。`PG_MAX_CONNS=16` 时 1200 VU 仍 100% 成功但 p99 `1725.16ms`；1600 VU 成功率降到 `0.6870`。`PG_MAX_CONNS=32` 时 1200/1600 VU 成功率 100%，但 p99 仍为 `1381.33ms` / `1476.37ms`；2000 VU 成功率 `0.9712`。`PG_MAX_CONNS=64` 时 1200/1600/2000 VU 写入成功率高，但 outbox pending 分别升到 `8044` / `19851` / `49948`，说明写入并发放大后 relay 追平成为第二瓶颈。`PG_MAX_CONNS=96/128` 在当前 PostgreSQL `max_connections` 下触发 `FATAL: sorry, too many clients already`，结果无效。
- 当前 formal multi-instance 矩阵：`loadtest/results/multi-instance-formal-20260609-021254/`。在每实例 `PG_MAX_CONNS=16`、1200 VU 下，1/2/4 实例成功率分别为 `0.9660` / `0.5572` / `0.9014`，请求 p99 均约 `2000ms`；用 `service_metrics[]` 重新计算后，4 实例的 per-instance `repository_begin_p99` 范围为 `603.59ms` 到 `2002.33ms`，说明不是每个实例都打满，但整体请求 p99 仍未改善。在当前单 PostgreSQL、当前连接上限和当前写入模型下，多实例不能解决尾延迟。
- 当前 formal 矩阵追加短 relay drain 后确认 `tenant_count=16 total_pending=0`，没有留下未发布 outbox 积压。
- 当前 PostgreSQL 诊断脚本为 `loadtest/sendmessage/collect-postgres-diagnostics.ps1`。正式矩阵后采集结果在 `loadtest/results/postgres-diagnostics-20260609-022602/postgres-diagnostics.json`：`max_connections=100`、`shared_buffers=16384`、`max_wal_size=1024`、`checkpoint_timeout=300`、`synchronous_commit=on`、`deadlocks=0`；`message_outbox n_dead_tup=113312`，说明 outbox 高频 update 已产生明显 dead tuples。
- 当前 multi-instance PG budget 矩阵：`loadtest/results/multi-instance-budget-formal-20260609/`，报告为 `docs/runbook/loadtest-report-20260609-multi-instance-budget.md`。固定每实例预算 `1x16/2x16/4x16` 下 p99 分别为 `2000.53ms` / `1437.37ms` / `1975.75ms`；固定总预算 `1x64/2x32/4x16` 下 p99 分别为 `1178.97ms` / `1509.55ms` / `1657.18ms`。固定总预算下多实例没有降低尾延迟，且 outbox pending 为 `31889` / `64553` / `65323`，说明当前应优先处理 PostgreSQL acquire/begin 排队与 relay 追平，而不是继续堆 gRPC 实例。
- 当前 PostgreSQL loadtest profile 矩阵：`loadtest/results/pgpool-tuned-formal-20260609/`，报告为 `docs/runbook/loadtest-report-20260609-postgres-loadtest-profile.md`。启用 `max_connections=200`、`shared_buffers=1GB`、`max_wal_size=4GB` 后，`PG_MAX_CONNS=64/VU1200` p99 `1161.70ms`，`PG_MAX_CONNS=64/VU1600` p99 `1759.11ms`；`PG_MAX_CONNS=128` 未改善，1200 VU 成功率 `0.9760` 且 p99 `2001.08ms`。新指标确认 `repository_begin` 主体是 `repository_pool_acquire`，`repository_tx_begin` p99 仅约 `14-33ms`。watch 采样显示 `LWLock:WALWrite`、`LWLock:WALInsert`、`LWLock:BufferContent` 和 `CheckpointWriteDelay` 已进入瓶颈视野。
- 当前 backpressure on/off 正式矩阵：报告为 `docs/runbook/loadtest-report-20260609-backpressure-onoff.md`，结果路径为 `loadtest/results/backpressure-off-formal-20260609/` 与 `loadtest/results/backpressure-on-formal-20260609/`。固定 `PG_MAX_CONNS=64`、relay workers 8 时，off 模式 1200/1600 VU 均 100% 成功，但 success p99 为 `1187.23ms` / `1735.38ms`，且 outbox pending 为 `30689` / `47736`；on 模式 overload rate 为 `97.28%` / `98.01%`，error p99 仅 `12.49ms` / `14.26ms`，但 success p99 仍为 `1403.95ms` / `1808.10ms`。结论：backpressure 快速拒绝有效且能降低 backlog，但当前 `MinAvailableConns=0` 策略过于粗糙，不能宣称成功请求 p99 改善。
- 当前 debug metrics collector 保存全量样本并在 snapshot 时排序，适合本地短压测，不适合作为生产 metrics；后续应替换为固定窗口、reservoir、HDR histogram 或 Prometheus histogram。
- `CONVERSATION_NOT_FOUND`、`MESSAGE_TOO_LARGE`、`SEQ_BLOCK_EXHAUSTED` 错误 sentinel 和 gRPC 映射暂未补齐；phase-1 普通会话 happy path 不阻塞，但不能声称完整错误契约已完成。
- 当前 raw gRPC server 还没有统一 deadline / trace / metrics interceptor；后续接 Kratos 或统一 gRPC interceptor。
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
- 2026-06-09：已跑正式 PG pool 矩阵和正式 multi-instance 矩阵，并更新 `docs/runbook/loadtest-report-20260609-pgpool-multi-instance.md`。阶段结论：当前证据显示 p99 主要贴在 `repository_begin`，`PG_MAX_CONNS=96/128` 超出 PostgreSQL 连接上限；在当前单 PostgreSQL、当前连接上限和当前写入模型下，多 `message-service` 实例没有降低尾延迟。下一步应转入 PostgreSQL 观测、写入路径优化、outbox relay 追平和 backpressure 设计。
- 2026-06-09：已新增并执行 `collect-postgres-diagnostics.ps1`，确认 PostgreSQL 当前 `max_connections=100`，解释 PG pool 96/128 档位不可用；已把正式矩阵和诊断结果发送给评审线程做阶段复核，本轮不频繁 push。
- 2026-06-09：评审线程阶段性指出多实例 summary 顶层 `service_latency_metrics` 只取第一个 service metrics URL，报告若直接使用会误导。本轮已修正 loadtest 聚合逻辑，并把报告表改为基于 `service_metrics[]` 的 per-instance min/max。
- 2026-06-09：已扩展 `run-local-multi-instance.ps1`，支持固定每实例 PG 连接预算和固定总 PG 连接预算两类对照实验；下一轮可直接跑 `FixedPerInstance` 与 `FixedTotal` 矩阵，避免实例数和总数据库连接预算混在一起。
- 2026-06-09：已用重建后的 loadtest 二进制跑 `FixedTotal` 短 smoke：`1x8` 与 `2x4` 两组均 100% 成功、outbox pending 0，summary 顶层 `service_pg_pool.max_conns` 分别为 `8` / `8`，证明多实例 PG pool 聚合和固定总预算脚本路径生效。
- 2026-06-09：已在 clean commit `ede5dd7` 跑正式 multi-instance PG budget 矩阵，并新增 `docs/runbook/loadtest-report-20260609-multi-instance-budget.md`。结论：固定总 PG 连接预算时，1 个实例 p99 最低，2/4 实例没有收益；request p99 仍贴近 repository append/begin，outbox pending 暴露 relay 追平为第二瓶颈。矩阵结束后已额外 drain，DB outbox 当前全部为 `PUBLISHED`。
- 2026-06-09：已把 `repository_begin` 拆成 `repository_pool_acquire_latency_ms` 和 `repository_tx_begin_latency_ms`，原 `repository_begin_latency_ms` 保持总耗时用于兼容旧报告。clean smoke commit `c10338e`：`PG_MAX_CONNS=8`、`VU=5`、`duration=3s`，1833/1833 成功，outbox pending 0，两个新指标 count 均为 1833，`git_dirty=false`。
- 2026-06-09：已新增 `watch-postgres-diagnostics.ps1`，用于压测期间按间隔采集 PostgreSQL `pg_stat_activity` wait_event、锁等待、表 dead tuples、bgwriter 和 WAL 统计，输出 `postgres-wait-samples.jsonl`。under-load smoke：`PG_MAX_CONNS=8`、`VU=20`、`duration=5s`，采样 10 次，最大 active backend 为 8，抓到 `LWLock:WALWrite` 等 wait_event。
- 2026-06-09：已新增 `deploy/local/docker-compose.postgres-loadtest.yml` 作为压测专用 PostgreSQL override，不改变默认开发 compose；目标参数包括 `max_connections=200`、`shared_buffers=1GB`、`max_wal_size=4GB`、`checkpoint_timeout=15min` 和更积极的 autovacuum 阈值。
- 2026-06-09：已实际启用 PostgreSQL loadtest override 并跑正式 PG pool 矩阵，同时采集 wait_event。结论：调大 PostgreSQL 与 PG pool 不能单独解决 p99，`repository_pool_acquire` 仍是主等待段；`PG_MAX_CONNS=128` 会放大 commit/WAL/outbox 压力。下一步优先做 backpressure 和 outbox relay 批量优化。
- 2026-06-09：已实现默认关闭的 PostgreSQL pool backpressure，并新增 `SERVICE_OVERLOADED` 错误码。clean smoke commit `78e8375`：`NEXUSIM_PG_BACKPRESSURE_ENABLED=true`、`PG_MAX_CONNS=1`、`VU=20`、`duration=5s`，163055 请求中成功率 `0.0032`，p99 `1.6246ms`，top error 为 `Unavailable: service overloaded`，outbox pending 0；报告为 `docs/runbook/loadtest-report-20260609-backpressure.md`。
- 2026-06-09：已为 loadtest summary 增加 `retryable_error_count`、`service_overloaded_count` 和 `message_error_counts[]`。clean smoke commit `a9fbdf8`：`PG_MAX_CONNS=1`、`VU=10`、`duration=3s`，`retryable_error_count=62556`、`service_overloaded_count=62556`、`message_error_counts[0]=SERVICE_OVERLOADED`，outbox pending 0。
- 2026-06-09：评审线程指出正式 backpressure on/off 矩阵不能只看混合 p99，否则大量快速拒绝会掩盖成功写入体验。本轮已补 `success_p99_ms`、`error_p99_ms`、`accepted_rps`、`error_rps`、`overload_rate`，并用 clean commit `6f0aa55` 重跑 on/off 矩阵；新报告明确区分整体 p99、成功 p99 和错误 p99。
