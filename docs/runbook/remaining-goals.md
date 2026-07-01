# NexusIM Remaining Goals

只记录未完成工作。当前进度见 `development-progress.md`，单服务事实见
`service-briefs/<service>.md`，完整架构见
`docs/architecture/target-architecture-complete.md`。

## 维护规则

- 新发现待办追加到本文件；完成后删除或改写为下一阶段 hardening。
- 不写长历史、完成证据、SDD / ADR 正文或 loadtest report。
- 新功能先做架构分析；新增服务 / 中间件 / provider 时同步 README、目标架构、
  service brief、相关 SDD / ADR、runtime profile 和本文件。
- 隐藏 fallback 按 `docs/architecture/fail-closed-policy.md` 治理；触达的
  fallback-like 代码必须清理或记录 owner、范围和风险。

## 当前优先顺序

1. Hot group fanout / sequencer / projection hardening：小规模 Docker 热点群 smoke 已通过；
   message-service seq block cache、timeline-service lease status / gap marker / repair
   operator first path 已进入收口；conversation-service 热点成员边界 seq 分配已接
   timeline-service；clean commit `d13bff6c` 已完成 61 人 / 20 消息、200 人 / 500 消息、
   500 人 / 1000 消息三档复验；clean commit `0a1395c` 已解除 1000 人 / 4000 消息 /
   800 msg/s 内的 message outbox relay 瓶颈。push-gateway writer metrics 和 hotgroup
   per-subscriber signal summary 已落地并 redeploy；READ_FANOUT 6000 人 / 1000 消息 /
   400 msg/s / 100 subscriber 诊断 run 已通过。clean commit `01b2a70` 已完成
   6000 人 READ_FANOUT 阶梯复压，目标 400 / 800 / 1200 / 2000 / 4000 /
   8000 msg/s 均通过，最高档产生 500000 条 conversation signal 且 outbox / Kafka 无积压。
   HYBRID 1000 人 / 1000 消息 / 400 msg/s 暴露 delivery outbox ready query 在百万级
   per-user outbox 下退化，当前已改成 per-conversation frontier query；最高档
   READ_FANOUT 已补 Prometheus 低敏时间窗口，核心 target up、SendMessage p99 约
   21ms、delivery_outbox pending 峰值后归零、push writer / Redis route 有数据。
   clean commit `7bff4f3` 的 200 subscriber / 1000000 signal 阶梯和 clean commit
   `233d695` 的 400 subscriber / 2000000 signal 阶梯也已通过，outbox / Kafka
   追平，drain rate 稳定在约 2.83k-2.86k signals/s，继续证明瓶颈是
   online signal drain。Prometheus 窗口工具已补 WebSocket writer / Redis route
   per-event 归因，400 subscriber 窗口显示 writer / delivery notify / Redis subscriber
   error 与 eviction 均为 0，整窗口 writer success 和 Redis subscriber enqueue 约 200 万级，
   当前需要继续区分 push writer flush 批量效率、runner 读取 / JSON decode / accounting
   和网络吞吐。`loadtest/hotgroup` 已新增 `subscriber-only` 多 runner shard 模式；
   clean commit `9e7d4f9` 的 4 runner / 400 subscriber 对照已完成，drain rate 约
   2852 signals/s，与单 runner 400 subscriber baseline 约 2840 signals/s 基本一致。
   后续 registry lock 缩短、payload 预编码和 WebSocket writer duration 复压均未突破
   约 2.85k-2.89k signals/s；clean commit `4f45519` 的 writer duration 复压显示
   `delivery_notify` write p95 / p99 低于 0.5ms，说明单 runner JSON decode、
   registry mutex、重复 JSON marshal 和单次 `conn.Write` 长尾都不是主瓶颈。clean
   commit `6099ecd` 的 Redis subscriber fanout duration 复压显示 400 subscriber
   / 400000 signal drain rate 约 2883.976 signals/s，WebSocket write p99 约
   0.63ms，而 Redis subscriber conversation signal fanout/enqueue p95 / p99 约
   56.14ms / 91.228ms。push-gateway conversation fanout worker / shard queue
   已实现并用 clean commit `93654117` 完成镜像重建、归档、Ubuntu redeploy 和
   400 subscriber coordinator + shard 复压。结果显示 queue handoff 正常：
   queue full / worker error 为 0，queue wait p95 / p99 约 `0.095ms / 0.099ms`；
   但整体 drain rate 仍约 `2876.076 signals/s`，未突破旧曲线。下一步不再优先调
   Redis subscriber receive path，转向 worker 本地 fanout、session outbound queue
   drain、writer goroutine 调度、WebSocket flush / batching 和 runner 读取背压分析。
   已新增
   `tools/analyze-hotgroup-loadtest.ps1`、`tools/analyze-hotgroup-multirunner.ps1` 和
   `tools/record-hotgroup-metrics-window.ps1` 自动汇总压测结果、分类瓶颈、记录
   Prometheus 时间窗口和给出下一步策略；后续每次正式复压都要生成或更新对应低敏分析报告。
   clean commit `fedb5f43` 的 writer queue latency / batch drain 已完成镜像重建 /
   归档 / redeploy 和 400 subscriber coordinator + shard 复压；queue p95 / p99 低、
   write p95 / p99 低，但 worker fanout p95 / p99 仍约 57.759ms / 92.241ms，
   signal drain rate 仍约 2884 signals/s。conversation-local fanout buckets 已实现：
   同一 conversation signal 的本地 subscriber 集合按 stable `session_id` bucket
   并行 fanout，同时保持每个 session 内信号顺序、queue full / slow eviction 和
   durable PullInbox 恢复边界；clean commit `a15e0ad` 已完成镜像重建 / 归档 /
   redeploy 和 400 subscriber coordinator + 4 shard 复压。结果显示 drain rate 约
   2874.378 signals/s，仍在旧区间；fanout p95 / p99 约 54.133ms / 90.827ms，
   queue full / error / slow eviction 为 0。下一步不要继续在单次 fanout 调用里加
   临时 goroutine。clean commit `4be4b2d` 已验证 4 个 push-gateway ws 实例分摊
   同一 conversation 的 400 个 subscriber：4 个实例和 Prometheus target 全部 up，
   但 drain rate 约 2822.479 signals/s，低于单 ws fanout-buckets baseline
   约 2874.378 signals/s。后续不要把简单多开 ws 容器当成主要解法，应评估持久
   per-conversation / per-bucket worker、超大房间 pull-first / sampled online signal
   策略，或用小诊断拆分 server enqueue cost 与 client/network receive cadence。
   显式 `NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY` 已完成 clean commit
   `bac71c65` 镜像重建 / 归档 / redeploy，并用 400 subscriber coordinator + 4 shard
   对照 full-signal 与 sample=10：emitted signal 从 400000 降至 40000，signal span
   从 141.719s 降至 25.243s，SendMessage / PullInbox / ACK 和 message / delivery
   outbox drain 均成立。随后用 clean commit `f5bc0199` 将 sample=10 场景扩大到
   5000 消息：400 subscriber 共读完 200000 条 signal，span 138.555s，SendMessage
   / PullInbox / ACK 和 outbox drain 仍成立，但 Redis subscriber conversation fanout
   p95 / p99 约 54.541ms / 90.908ms，瓶颈仍是 online-signal-drain。已选择
   room policy / adaptive cadence 方向并实现 fanout-mode conversation signal
   policy；clean commit `37b575e5` 已完成镜像重建 / 归档 / redeploy，并以
   default=1、READ_FANOUT/BROADCAST_SIGNAL=10 复压。新 run 在 6000 人 /
   5000 消息 / 8000 msg/s / 400 subscriber 下读完 200000 条 sampled signal，
   span 141.504s，message / delivery outbox pending=0。该结果证明 policy 边界生效，
   但 READ_FANOUT=10 下瓶颈仍是 online-signal-drain。clean commit `9bdf21c5`
   已继续实现并复验 subscriber-aware conversation signal cadence：按 fanout mode
   和每个本机 / 远端 gateway 的 conversation subscriber 数决定更保守的 sample
   cadence；未配置 subscriber threshold 时保留旧采样前置快速路径。同场景
   READ_FANOUT `100:20` 复压读完 100000 条 signal，outbox 和 writer / Redis
   错误路径正常，但 signal span 289.249s、span rate 约 345.723 signals/s，低于
   `37b575e5` baseline。clean commit `304383ea` 的 Redis conversation route cache
   已完成镜像重建 / 归档 / redeploy 和同场景复压：400 subscriber / 5000 消息 /
   READ_FANOUT `100:20` 下，signal span 降至 146.62s，span rate 提升到约
   682.034 signals/s，约 1.97x；message / delivery outbox pending=0，writer /
   Redis subscriber error、queue-full 和 eviction 均为 0。当前已补并 redeploy
   delivery-consumer debug/metrics scrape 配置，并通过
   `hotgroup-policydefaults-400sub-5000msg` 复压确认 route cache hit / miss 可见。
   该轮同时发现 Docker env 漂移为空时会退回全量 remote publish，所以本地 compose
   已固定 READ_FANOUT / BROADCAST_SIGNAL 默认 sample=10、subscriber policy `100:20`。
   同配置 repeat 已确认 corrected policy 曲线稳定：100000 条 signal 的 span 为
   193.012s，和上一轮 193.559s 基本一致。total-subscriber-aware pull-first policy
   已完成代码、镜像重建 / 归档 / redeploy 和可比复压：Redis route 可按整个
   conversation 在线订阅总数触发更强 `sample_every`，并把有效 policy decision
   传给各 ws gateway。本地 Docker 默认已设 per-gateway `100:20`、total `400:50`。
   `hotgroup-totalsubpolicy-400sub-5000msg` 把 signal 数从 100000 降到 40000，
   但 span 仍约 193s，且 5000 条 SendMessage 实际发送耗时 `74.916s`
   （约 `66.741 msg/s`，远低于 target `8000 msg/s`）。已定位旧 runner 是
   单 goroutine 同步发送，现已新增 `--send-concurrency`，默认等于 `sender-count`，
   并把 `achieved_send_rate` 纳入分析脚本。剩余任务：用 clean commit 和新 runner
   重跑同一 total-subscriber 场景，确认 actual send rate 是否提升。并发 sender 首轮
   诊断已经暴露本地 Docker profile 的 PostgreSQL 连接预算不足：`max_connections=100`
   且核心服务 pgx pool 未显式限额，会在 64 concurrency 下触发
   `too many clients already`，需要先 redeploy 已修正的 Postgres max_connections
   和核心服务 PG pool cap，再复验。dirty diagnostic
   `hotgroup-pgpoolcap-200x500-diagnose-20260701-1510` 已证明连接耗尽解除：
   500/500 SendMessage 成功、outbox pending 归零、PostgreSQL 日志无新的
   `too many clients already`；但 SendMessage p95 / p99 为
   `743.89ms / 1024.314ms`，当前瓶颈转为 `send-path-latency`。随后已确认
   6000 人 `READ_FANOUT` 会话仍走 `LOCAL_ROW_LOCK` 是更直接的 seq 行锁瓶颈。
   clean commit `6a4673b` 已把 large group 修正为
   `READ_FANOUT + SEQUENCER_BLOCK`，并完成 Docker 镜像重建 / 归档 / redeploy
   和 6000 人 / 1000 消息 / 256 concurrency clean 复验：
   `conversation_mode=SEQUENCER_BLOCK`、1000/1000 发送成功、SendMessage p95 / p99
   `208.507ms / 220.367ms`、outbox pending=0。message-service 已补
   first-stage recent latency metrics，为 SendMessage、conversation seq allocation
   和 repository 分段输出最近 4096 个样本的 `_recent` operation，hotgroup
   Prometheus 时间窗口脚本也已采集这些 recent p95 / p99。剩余任务是用 clean
   commit 重建 / redeploy message-service 后，用更大消息数做稳态 send-only
   复压。clean commit `d190c35` 的 256 concurrency / 5000 message send-only
   run 已达到约 `2052.125 msg/s`、SendMessage p99 `482.711ms`，outbox pending=0；
   clean commit `1d738f2` 的 512 concurrency / 5000 message 正式复压达到约
   `2356.419 msg/s`、SendMessage p99 `257.893ms`，outbox pending=0；
   `conversation_seq_alloc_recent p99` 约 `0.023ms`、`repository_pool_acquire_recent p99`
   约 `0.538ms`，说明连接池和 seq allocation 仍不是瓶颈。剩余任务是继续尝试
   768 / 1024 concurrency 或扩大 message_count，
   同时观察 recent repository append / insert_outbox / commit p99、PostgreSQL CPU / IO
   和 message-service CPU；之后再回到 total-subscriber-aware policy 的 6000 人 /
   5000 消息 / 400 subscriber 场景，确认 `achieved_send_rate` 与 signal span 新曲线。
   若仍无容量改善，再分析 delivery_outbox signal production cadence、Kafka
   publish / consume cadence 和 push event pacing。在取得复压证据前不要继续只提高
   sample 阈值，也不要把 target rate 当作真实 QPS。
2. Agent action boundary / repair cases：在 provider replay admin / workflow handoff 已落
   的基础上，继续扩更多需要 proposal / approval / workflow / audit 的 action 与 repair 场景。
3. Product-active 服务按需推进：workflow、audit、admin、notification、media、vector、
   model、knowledge、presence、control-plane。
4. 数据平台和中间件 profile 按完整架构逐步补，不抢占 AI / Agent 演示主线。
5. 10 个运行链路服务（9 个既有 IM 服务 + `timeline-service` seq-block allocator）只回补阻塞
   AI / product platform、热点群压测或用户点名项的 P0/P1。
6. 客户端只作为演示入口维护；除非阻塞演示，不继续扩完整产品级客户端。
7. 热点群 / 分区主线已落 conversation-service scale policy、delivery hybrid/read fanout、
   conversation-level delivery signal first-stage runtime、push-gateway conversation
   subscription / signal broadcast、timeline-service seq-block allocator、message-service
   active `SEQUENCER_BLOCK` 写路径、seq block cache、gap marker / repair operator first path
   和 hotgroup runner；2026-06-29 已跑通 61 人 / 20 消息 / 3 WebSocket subscriber 小规模
   smoke；2026-06-30 已定位并修复 `SEQUENCER_BLOCK` 下成员 JOIN 仍未接 timeline
   sequencer 的问题，完成 clean redeploy 三档复验，并完成 message outbox relay
   conversation-sharded batch publish 复验。多 runner 对照已证明单 runner 读取不是唯一瓶颈；
   下一步继续做 push worker 本地 fanout / session writer 调度优化、virtual partition
   mapping、leader ownership audit 和 deeper repair。

## Client Demo Backlog

- Web / PC shell：演示 MVP 已达标；后续只修阻塞 AI / Agent 演示入口的问题。
- Windows PC：只要求本地 shell 能打开并演示；release signing / installer 后置。
- Android：后续切回时重新加载 F 盘 toolchain env 或显式 Docker builder，再跑 APK /
  WebView login smoke。
- 入群审批 / 禁言、复杂群管理、完整媒体 UX、移动端体验、SQLite native store 后置。
- 生产 Web 鉴权后续切 httpOnly cookie / provider-grade session 策略。

## AI / Agent Platform

- `search-service`：真实 OpenSearch 进程 smoke、容量曲线、provider-grade 运维。
- `memory-service`：结构过滤、BM25 / vector、rerank、repair audit、更多 group-memory eval。
- `retrieval-gateway`：真实 OpenSearch、pgvector、Milvus provider smoke 和 coverage 深化。
- `rag-service` / `summary-service`：继续扩展 multi-hop / temporal / profile eval、
  provider-specific regression 和更完整 unsafe output cases。
- `agent-service`：proposal risk policy、instruction approval UI、更多真实业务
  proposal 场景。
- `skill-registry` / `mcp-gateway`：tool contract、risk level、tenant allowlist、adapter、
  rate limit。
- `action-executor`：provider replay admin / workflow handoff、review/readiness/redrive
  operator artifacts、external audit append 和 audit result manifest 已落；后续做更多
  action boundary / repair cases 和 provider-grade replay UI。
- `ai-eval-service`：group-memory asker-bound term ambiguity、visible-chain incomplete
  abstention、missing visibility projection fail-closed、audience-language profile negative
  cases 已进入本地低敏 gate；后续继续扩 provider readiness、Agent action boundary 和
  redrive / repair cases。
- Python AI Worker：继续保持 candidate-only；更多 memory extraction、planner、rerank 和
  eval 候选算法。

## Product-Active Services

- `media-service`：真实 S3-compatible adapter、scanner、thumbnail / transcode provider、
  CDN / download policy、retention / delete proof。
- `notification-service`：SMTP / SMS / APNs / FCM adapter、bounce / suppression、
  provider redrive / audit、tenant template policy。
- `audit-service`：action-executor external audit append operator 已通过公开
  `AppendAuditRecord` 接入第一版低敏 operator 追加路径；后续补更多 Kafka ingestion
  source、checkpoint / rewind、export worker、SIEM forwarding、retention cleanup、
  segment sealing。
- `admin-service`：admin UI、provider-grade provider replay request UI、更多下游公开 API
  adapter、compensation adapter、instruction approval UI。
- `control-plane-service`：outbox relay、drift monitor、expiry / cleanup worker、
  api-gateway quota consumer、provider-grade rollout。
- `presence-service`：push-gateway session consumer、`SubscribePresence`、stale scanner、
  outbox relay、Redis hot-state、privacy / contacts policy。
- `model-gateway`：provider routing、budget、fallback policy as explicit config、audit。
- `knowledge-ingestion-service`：file/web imports、chunking pipeline、PII scan、rebuild jobs。
- `vector-index-service`：real pgvector / OpenSearch vector / Milvus smoke、provider repair。
- `workflow-service`：external callback delivery / redrive、approval queue review / batch
  decision、compensation review / execution artifacts、audit append handoff 和 workflow outbox
  relay first path 已落；后续继续补 workflow outbox relay smoke、更多 compensation adapter、
  callback delivery provider-grade persisted dashboard / provider-grade approval platform。

## 核心 IM 运行链路 P2

- `api-gateway`：legacy observation evidence、provider-grade quota、gray rollout、OTel stack。
- Cross-service loadtest：继续维护 `capacity_summary`，形成容量曲线和瓶颈说明；新增
  `loadtest/hotgroup` 业务压测 runner，覆盖热点群聊 fanout、Kafka lag、delivery
  projection、push notify storm、PullInbox / ACK 追平、成员变更和故障恢复，不用单接口
  QPS 替代真实业务链路；正式压测必须配套 Prometheus / Grafana 趋势图。当前已新增
  `NexusIM Hot Group Loadtest` first-stage dashboard、hotgroup 离线分析器和
  Prometheus 时间窗口记录工具；后续还需补 fanout-mode distribution、Kafka consumer lag、
  delivery timeline item insert rate、
  inbox rows per message 和 PostgreSQL lock / WAL / dead tuple time-series exporter。
- `identity-service`：WebAuthn/passkeys、OIDC、多 issuer、KMS/HSM、完整风控、生产级
  email/SMS provider。
- `message-service`：删除 / 撤回 / 编辑深化、外部 proof workflow、发送链路生产观测。
- `message-service`：`SEQUENCER_BLOCK` 已接 timeline-service seq block active 写路径；
  61 人热点群小规模 smoke 已通过；本轮已补本地 seq block cache、lease safety margin
  和 lease metadata 校验；message outbox relay 已支持 conversation-sharded multi-worker
  batch publish、批量 mark published 和 ready query indexes；1000 人 / 4000 消息 /
  800 msg/s 档位 message outbox 可追平。后续补更完整发送链路生产观测；删除 / 撤回 /
  编辑深化、外部 proof workflow 后置。
- `conversation-service`：群规模策略已进入 domain 层，medium 策略为
  `HYBRID_FANOUT + LOCAL_ROW_LOCK`，large 策略已修正为
  `READ_FANOUT + SEQUENCER_BLOCK`，hot group 策略为
  `BROADCAST_SIGNAL + SEQUENCER_BLOCK`；promotion 已能修正同 version 下的
  `conversation_mode` / `current_seq_shard` 漂移。后续继续补 control-plane rollout、
  群管理深化、历史窗口 / targeted replay repair。
- `timeline-service`：已进入本地运行链路的 seq-block allocator，具备 PostgreSQL
  sequence state / block lease / gap marker、`AllocateSeqBlock` gRPC API、Docker /
  Prometheus / Grafana 观测；message-service 已只在 valid block lease 下取号并支持本地
  block cache；已补 `seq-lease-expire`、`gap-marker-create`、`gap-marker-close`、
  `gap-marker-audit` operator first path。后续补 virtual partition mapping、leader ownership
  audit、operator workflow UI 和更完整 repair smoke。
- `delivery-service`：projection DLQ / repair 深化、更多 delivery event consumer；
  `WRITE_FANOUT`、`HYBRID_FANOUT`、`READ_FANOUT` 和 conversation-level signal 已有
  first-stage runtime，materialized `user_inbox` 已改成批量 insert，`timeline-consumer`
  已支持按 Kafka partition 安全并行的 multi-worker runtime；后续补 timeline item repair、动态 read
  fanout 容量曲线，重点验证 Kafka lag、projection backlog、PullInbox / ACK 追平时间
  和 push notify storm；delivery outbox relay SQL / worker / Kafka batch hardening 已落，
  2026-06-28 hotgroup QPS step 已证明 `delivery_outbox -> Kafka im.delivery.events`
  不再是 100 人群 150 QPS 内的首个瓶颈；下一步转向 delivery timeline projection /
  single hot conversation fanout 策略、projection lag metrics、inbox rows per message metrics 和
  WebSocket notify storm 覆盖。
- `push-gateway`：conversation subscribe / unsubscribe 与 conversation signal fanout
  已进入服务端 first path；hotgroup runner 已验证 3 个 WebSocket subscriber 共收到
  60 条 conversation signal；message relay 复验后高压失败点已迁移到 push conversation
  signal 写出 / runner 读取观测。已补 writer flush 指标和 per-connection signal summary；
  READ_FANOUT 6000 人 / 100 subscriber clean commit 阶梯复压已验证最高 8000 msg/s
  目标档、500000 条 signal 可完整读出；后续继续补趋势图、更高 subscriber / signal
  总量瓶颈曲线、Redis 网络分区 smoke、跨实例 resume、容量测试。
- `receipt-service`：会话列表产品能力、更多摘要策略和容量曲线。
- `contacts-service`：组织级策略、租户默认值、来源策略、隐私例外。
- `policy-service`：provider-grade ReBAC / DSL、moderation / risk scoring、tenant quota、
  external audit pipeline。
