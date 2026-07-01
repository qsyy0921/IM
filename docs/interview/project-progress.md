# NexusIM 面试版项目进度

本文用于面试时介绍项目进度，重点说明：

- 已经开发了哪些后端能力；
- 当前系统能证明什么；
- 还差哪些生产化和产品化能力；
- 当前如何从 Go 微服务底座进入搜索、记忆、检索、大模型算法/eval、Agent 和客户端产品化 first slice。

它不是每轮 Codex 工作入口，也不是工程待办来源；每轮工作仍先看 `docs/runbook/current-brief.md`，当前未完成工作以 `docs/runbook/remaining-goals.md` 为准。

## 项目定位

NexusIM 是一个以 Go 微服务为主的分布式 IM + AI 应用平台项目。当前目标不是做一个简单聊天 demo，而是逐步实现：

```text
身份认证
-> 会话和成员
-> 消息写入
-> outbox / Kafka timeline
-> durable inbox
-> WebSocket 在线通知
-> ACK / 回执 / 联系人 / 策略权限
-> search-service v0.1
-> memory / retrieval / RAG / summary / Agent / skill-registry / MCP gateway / action-executor
-> Web / PC / Android client platform MVP
-> 业务平台 / 数据平台 / AI Agent 平台 / 中间件平台的完整目标架构
```

语言和运行时边界：生产后端控制面继续以 Go 为主；Python 后续只作为 AI Worker
层，用于 LLM、embedding、rerank、memory extraction、planner prototype 和 eval
candidate，不直接写 IM 业务库，不绕过 policy / approval / audit。浏览器、PC
desktop 和 Android 的共享协议、同步核心和 UI 使用 TypeScript；Rust / Kotlin 只做
薄平台 bridge。

当前可以准确表述为：

```text
本地 / 双机可运行的分布式 IM 后端 + AI / Agent 应用底座 +
Web-first 客户端平台 first slice。
```

不能过度表述为：

```text
生产级完整分布式 IM 平台。
```

## 开发过程主线

面试时建议按阶段讲，而不是按提交流水账讲：

```text
第一阶段：先做 message-service，验证 SendMessage + outbox + Kafka 的最小写入链路。
第二阶段：补 conversation-service，把发送上下文、成员事实和成员事件边界拆出来。
第三阶段：补 delivery-service 和 push-gateway，把 durable inbox、PullInbox、AckDelivery、在线通知和跨实例 route 串起来。
第四阶段：补 receipt-service、contacts-service、policy-service 和 api-gateway，把已读/未读、联系人、权限决策和统一入口补齐。
第五阶段：集中治理分布式可靠性、安全启动门禁、trusted metadata / TLS 边界、repair / audit / cleanup、debug metrics 和代码复杂度。
第六阶段：search / memory / retrieval / RAG / summary / Agent / skill-registry / MCP gateway / action-executor / ai-eval first paths 已落，EvidencePack、proposal / approval / audit、Python AI Worker 候选边界和 optional stack gate 已验证。
第七阶段：按完整目标架构补 product-active 平台服务和 client platform MVP foundation；Web first path、client BFF、push path、本地 / wired smoke 已通过，PC Tauri WebView metadata smoke 已通过，Web / PC shell 已接账号登录、注册、好友、好友私聊、群聊、群成员管理、群资料、消息列表和发送 first path，真实双用户 direct + group client smoke 已通过。
```

当前项目已经进入“完整架构同步后的产品化 + AI 应用底座”阶段：

```text
9 个既有 IM 后端服务已经能跑通主链路；`timeline-service` 作为第 10 个运行链路服务
已进入热点群 seq block allocator first path，并作为 AI 主线前的 Go 底座补强；
短期不以生产级完整系统测试或生产级 HA 作为算法/eval 前置阻塞，验证重点放在低敏 cases、EvidencePack、权限过滤、source refs、时间版本和审计边界；
search / memory / retrieval / RAG / summary / Agent / skill / MCP / executor / ai-eval first paths 已落，RAG / Summary / Agent stack 已通过 cross-group / temporal optional gate；
future platform / product services 已进入 product-active first-stage implementation；
client platform MVP foundation 已启动：Web first path、api-gateway client BFF、push path、本地和 wired 172 clean baseline 已通过，PC Tauri WebView metadata smoke 已通过，Web / PC shell 已接账号登录、注册、好友、好友私聊、群聊、群成员管理、群标题 / 头像 URI read-update、消息列表和发送 first path，真实双用户 direct + group client smoke 已通过；Android APK / 真机 smoke 后置到用户明确切回；
完整目标架构以 docs/architecture/target-architecture-complete.md 为基线，后续按业务平台、数据平台、AI / Agent 平台、客户端平台和中间件平台演进；
后续 AI 按 low-sensitive collaborative-memory 算法/eval 推进，优先 multi-hop / temporal update / profile aggregation；
api-gateway 已补 first-stage tenant-scoped rate limit、静态 tenant plan override、tenant plan 文件热更新、版本化 quota URL source、DB-backed tenant plan snapshot source、本地 tenant quota audit / set operator、tenant quota approval manifest 强制校验、URL bearer token / HTTPS guard、URL source CA / client cert TLS 边界、可选 checksum-required gate、applied quota snapshot stale 观测和 quota snapshot gate；
api-gateway 已补 legacy/facade traffic metrics，以及 legacy observation-window / removal-plan 低敏 evidence manifest，用于旧 descriptor 迁移观察和归档；
legacy descriptor 已收敛为显式 opt-in 默认；
当前 9 个既有 IM 服务已补 first-stage Prometheus text /metrics、本地 Prometheus alert rules 和本地 Grafana dashboard 原型；timeline-service 已纳入本地 Docker / 观测链路；
api-gateway 已补 first-stage OpenTelemetry 入口 server span 和下游 gRPC client span；当前 9 个服务均已纳入 first-stage trace runtime wiring，其中 8 个后端 gRPC 服务使用 server span，push-gateway 使用 WebSocket connection span，并由采样策略和本地 check-local 门禁约束；
本地 OTel collector debug 入口和 policy OTLP smoke 脚本已补，可用于面试演示 OTLP trace 链路，但还不是生产告警平台；
search-service v0.1 / group memory / retrieval / RAG / summary / Agent / skill / MCP / action-executor / ai-eval 已分别完成 first path、smoke 或 eval gate；当前 Go 底座已足够支撑算法切片，下一步进入 low-sensitive collaborative-memory 算法/eval；
后续开发可以使用 multi sub-agent 并行推进，但以服务 / 文档 / 测试面拆分，最终由主 agent 统一集成和验证；
面试叙述仍优先强调后端、分布式和 AI / Agent；客户端作为产品化展示层和端到端验证入口按需讲。
```

## 已完成的后端服务

当前已有 9 个既有真实 IM 后端微服务，另有 `timeline-service` 作为第 10 个运行链路服务承接热点群 sequencer 能力：

| 服务 | 已完成能力 | 面试可讲重点 |
| --- | --- | --- |
| `api-gateway` | 统一 user-facing gRPC 入口，gateway token 验证，verified metadata 注入，下游代理，token / tenant scope rate limit，静态 tenant plan override，tenant plan 文件热更新，版本化 quota URL source，DB-backed tenant plan snapshot source，本地 tenant quota audit / set operator，tenant quota approval manifest 强制校验，URL bearer token / HTTPS guard，URL source CA / client cert TLS 边界，可选 checksum-required gate，applied quota snapshot stale 观测，quota snapshot gate，legacy descriptor 显式 opt-in 默认，legacy/facade traffic metrics，legacy quiet-window gate、observation 归档脚本和 legacy evidence manifest，first-stage OTel 入口 server span 和下游 gRPC client span，debug metrics | 统一入口、安全边界、correlation / trace 传播、facade-only 默认暴露面 |
| `identity-service` | 注册、登录、Refresh Token、MFA TOTP、recovery codes、JWKS、opt-in OIDC discovery、session/device revoke、verification/password reset challenge、webhook / SMTP email challenge sender、production-like key guard，first-stage OTel gRPC server span | 身份认证、MFA、token 轮换、JWKS、公私钥边界、issuer discovery 边界、通知投递可靠性，生产样式启动配置拒绝 local shared key，身份服务已进入 trace rollout |
| `message-service` | `SendMessage`、编辑、撤回、删除，合规删除 external proof manifest verifier，`TEXT` + `IMAGE` / `FILE` / `VOICE` 附件引用消息，`LOCATION` / `CARD` 结构化 payload 消息，message log，outbox，Kafka timeline event，first-stage OTel gRPC server span | 业务事务不直接 publish Kafka，使用 outbox 保证事件传播；合规 proof 只登记低敏 ref/provider/hash，不保存正文；核心写服务已进入 trace rollout；图片 / 文件 / 语音二进制处理后续交给 media 能力 |
| `conversation-service` | 会话成员事实源，`GetSendContext`，成员变更 saga，owner transfer，ACTIVE roster 分页、单 / 多 role 过滤、role-first 管理排序和 `user_id_prefix` 轻量前缀过滤，成员窗口 audit / repair / repair audit（含当前窗口 `join_seq` / `leave_seq` / 版本 floor 保守修复），first-stage Prometheus text `/metrics`、本地 alert rules / Grafana dashboard、first-stage OTel gRPC server span | 会话成员事实边界、成员事件和消息事件共享 timeline seq，成员事实服务已进入观测 rollout |
| `delivery-service` | timeline projection，durable `user_inbox`，`PullInbox`，`AckDelivery`，delivery outbox，projection failure audit / checkpoint rewind / failure resolve / cleanup operator，first-stage Prometheus text `/metrics`、本地 alert rules / Grafana dashboard、first-stage OTel gRPC server span | 断线可恢复，push-gateway 不拥有 durable inbox，投递服务已进入观测和 projection repair rollout |
| `push-gateway` | WebSocket 在线通知，ACK 转发，resume buffer，Redis route，跨实例在线路由，Redis resume negative recovery，Redis Cluster topology、node-stop recovery、六节点 failover smoke 和六节点短容量基线，first-stage Prometheus text `/metrics`、本地 Prometheus scrape / alert rules 原型、本地 Grafana dashboard 原型、first-stage OTel WebSocket connection span | 在线唤醒层和可靠投递层解耦，Redis / resume / Cluster node 故障时 PullInbox 兜底，在线层已进入观测 rollout |
| `receipt-service` | 已读 / 未读，会话列表，archive / pin / mute / tags / 多标签 all-match 过滤 / draft / last-source-event-type 过滤，unread-first 会话排序，receipt projection，receipt outbox，`ListReceiptStates` repository 级批量查询，低敏 `received_device_count` 聚合和 opt-in capped device details，first-stage Prometheus text `/metrics`、本地 Prometheus scrape / alert rules 原型、本地 Grafana dashboard 原型、first-stage OTel gRPC server span | 会话列表和回执从投递事件投影，不跨服务读内部表，设备明细默认隐藏、显式开启且限量返回，回执服务已进入观测 rollout |
| `contacts-service` | 好友申请、申请来源 metadata、租户级来源策略、来源风险标注和 `REVIEW_REQUIRED` operator 审批状态机、申请列表 source / risk / review 过滤、接受、拒绝、取消、删除、拉黑、解除拉黑、备注、分组、联系人搜索、用户 / 租户 / 系统三级申请隐私、first-stage ALLOW-DENY 隐私例外写入 / 查询 / 清理、搜索来源申请 gate、profile visibility 总开关和字段级白名单、租户默认隐私 operator、contacts outbox，first-stage Prometheus text `/metrics`、本地 Prometheus scrape / alert rules 原型、本地 Grafana dashboard 原型、first-stage OTel gRPC server span | 联系人事实源，策略服务通过事件投影使用联系人关系；隐私、来源策略、审批状态和拉黑只影响本服务关系事实，消息权限通过 policy projection 表达 |
| `policy-service` | 权限决策、规则存储、用户级消息动作限制、first-stage ReBAC decision source、first-stage relationship gate + 本地 relation operator、first-stage keyword / HTTP content moderation、first-stage tenant action quota、first-stage tool policy precheck / low-sensitive local audit、conversation role gate、contacts projection、decision audit outbox、低敏 decision audit export / forward、first-stage Prometheus text `/metrics`、本地 Prometheus scrape / alert rules 原型、本地 Grafana dashboard 原型、first-stage OTel gRPC server span | 策略权限独立服务化，不在 message-service 复制权限逻辑；`decision_source` 让 API / audit / Kafka 能解释决策来自 exact rule、tenant rule、关系门禁、联系人投影、quota、ownership 或 moderation；relationship gate 用 policy-service 自有 projection / relation rules 做直接联系人和活跃成员要求，不满足时在 allow 规则前 fail-closed；工具动作通过 `CheckToolAction` 做统一预检，默认 fail-closed，审计只保存低敏 stable key 和 tool/action/resource/risk 元数据，为后续 Agent / skill-registry / MCP gateway / action-executor 接真实业务动作提供权限边界；关系规则、决策审计和 tenant quota 先以本地低敏 operator 形式闭环；内容分类通过 policy provider port 接入，keyword / HTTP adapter 都不持久化正文；decision audit forward 只推低敏审计行到外部 HTTPS sink，provider-grade 外部 audit pipeline、ReBAC graph / DSL、tenant DSL、tool policy operator / approval integration 和 risk scoring 后续深化 |

## 已完成的主链路

当前主链路已经覆盖：

- 注册 / 登录 / Refresh Token / MFA；
- 会话成员创建和发送上下文查询；
- 普通消息发送、编辑、撤回、删除；
- PostgreSQL 事务事实源；
- outbox + Kafka event 传播；
- durable inbox 投递模型；
- `PullInbox` 和 `AckDelivery`；
- WebSocket 在线通知；
- 已读 / 未读 / 会话列表基础能力；
- 联系人关系和拉黑策略；
- policy-service 权限决策。

可以用这句话概括：

```text
消息从客户端入口进入后，可以经过身份、权限、会话、消息、投递、在线通知、ACK 和回执链路闭环。
```

## 热点群聊面试讲法

面试官问“热点群聊怎么处理”时，不要只回答“加机器”或“上消息队列”。
NexusIM 的回答应先把问题拆清楚：热点群聊真正的瓶颈通常不是单个
`SendMessage` RPC，而是消息写入、Kafka timeline、delivery fanout、
`user_inbox` 写放大、在线 push 风暴、慢连接、ACK 追平和成员可见窗口一起叠加。

当前架构的第一原则是：

```text
消息事实只写一次；
投递异步 fanout；
在线通知只做轻量唤醒；
可靠恢复依赖 durable inbox / PullInbox / AckDelivery；
客户端展示顺序以 conversation_seq 为准。
```

### 已有热点群压测证据

当前已经做过第一轮热点群业务压测，并把原始结果保存在
`H:\NexusIM\loadtest-results`，低敏报告见
`docs/runbook/loadtest/hotgroup/loadtest-report-20260628-hotgroup-relay-bottleneck.md`。

这轮压测不是单接口 `wrk`，而是走真实业务链路：

```text
CreateConversation(GROUP)
-> CreateMemberChange(JOIN)
-> SendMessage
-> message_outbox
-> Kafka conversation.timeline.events
-> delivery timeline projection
-> user_inbox fanout
-> delivery_outbox
-> delivery outbox relay
-> Kafka im.delivery.events
-> sampled PullInbox / AckDelivery
```

关键观察：

| 阶段 | 结果 | 说明 |
| --- | --- | --- |
| 优化前 | 20 人群 10 QPS 通过；50 / 100 / 150 QPS 均卡在 `delivery_outbox` drain | `SendMessage` 和 `user_inbox` 已完成，瓶颈在 `delivery_outbox -> Kafka im.delivery.events`。 |
| 第一版优化 | 多 worker 但按 outbox row id 分片，效果不好 | 这证明不能只“加 worker”；分片边界必须和业务顺序边界一致。 |
| 修正后 | 100 人群 50 / 100 / 150 QPS 均能完成 `user_inbox` 和 `delivery_outbox` drain | relay 改成按 `tenant_id + conversation_id` 分片，单 conversation 顺序推进，跨 conversation 并行。 |
| 继续探测 | 100 人群 200 QPS 时发送 1000 / 1000 成功，后续 DB 证明 `user_inbox` 和 `delivery_outbox` 最终追平，但超过 runner 等待窗口 | 新瓶颈从 relay 转移到 delivery timeline projection / `user_inbox` fanout。 |
| 策略切换 smoke | 61 人热点群、20 条消息、3 个 WebSocket conversation subscriber，`SEQUENCER_BLOCK + BROADCAST_SIGNAL` 通过 | `send_p95_ms=19.03`、`user_inbox_rows=0`、`delivery_timeline_rows=20`、3 个订阅者共收到 60 条 conversation signal，`delivery_outbox_pending=0`，证明热点群 first-stage 不再走小群全员写扩散路径。 |
| 成员边界诊断 | 200 人 / 500 消息 dirty-run 暴露并修复 `SEQUENCER_BLOCK` 下成员 JOIN 未接 timeline sequencer 的问题 | 修复后成员边界通过 timeline-service 分配单 seq lease，中等规模诊断可完成 `BROADCAST_SIGNAL`、`delivery_outbox_pending=0`、Kafka lag=0；正式报告仍需 clean commit 重跑。 |
| clean redeploy 复验 | clean commit `d13bff6c` 重建 / redeploy 后跑通 61 人 / 20 消息、200 人 / 500 消息、500 人 / 1000 消息三档 | 最大档 500 人 / 1000 消息 / 50 subscriber 产生 50000 条 conversation signal，`send_p95_ms=10.633`、`send_p99_ms=13.013`、`user_inbox_rows=0`、`delivery_outbox_pending=0`、Kafka lag=0。 |
| message relay 优化 | clean commit `0a1395c` 后，message-service outbox relay 支持 conversation-sharded multi-worker batch publish | 1000 人 / 4000 消息 / 800 msg/s / 100 subscriber 通过，message / delivery outbox 均无积压；更高档位把瓶颈暴露到 push conversation signal 观测。 |
| delivery outbox frontier 优化 | clean commit `01b2a70` 后，delivery-service outbox ready query 改为 per-conversation frontier，并用 8 worker relay 复压 | 6000 人 READ_FANOUT、100 subscriber 下，400 / 800 / 1200 / 2000 / 4000 / 8000 msg/s 目标档均通过；最高档 5000 条消息、500000 条 signal，`send_p95_ms=18.54`、`send_p99_ms=22.41`、`delivery_outbox_pending=0`、Kafka lag=0。 |
| 压测自动分析 | `tools/analyze-hotgroup-loadtest.ps1` 离线读取 H 盘 `hotgroup-summary.json`，生成 run matrix、瓶颈分类和下一步策略 | clean commit `01b2a70` 的 6 档 READ_FANOUT 被分类为 `online-signal-drain`：outbox / Kafka 已追平，但 500000 条 signal 最慢读完约 176s；下一步应提高 subscriber / signal 总量并补 Grafana / Prometheus 时间窗口。 |
| Prometheus 时间窗口 | `tools/record-hotgroup-metrics-window.ps1` 读取 H 盘压测目录并查询 Prometheus range API，输出低敏窗口报告 | 最高档 `hotgroup-readfanout-6000-8000qps-clean-01b2a70e-20260630-2336` 已补窗口：核心 4 个 scrape target 全部 up，`SendMessage p99` 约 21ms，`delivery_outbox_pending` 峰值 2258 后归零，push writer / Redis route 指标有数据，slow eviction 为 0。 |
| 200 subscriber 阶梯 | clean commit `7bff4f3` 跑 6000 人 / 5000 消息 / 8000 msg/s / 256 sender / 200 subscriber | 产生 1000000 条 conversation signal，`send_p95_ms=18.315`、`send_p99_ms=21.808`、`PullInbox p95=26.326ms`、outbox / Kafka 追平；最慢 drain 349.903s，drain rate 约 2857.934 signals/s，和 100 subscriber 的约 2831.995 signals/s 接近，说明瓶颈沿 online signal drain 线性放大。 |
| 400 subscriber 阶梯 | clean commit `233d695` 跑 6000 人 / 5000 消息 / 8000 msg/s / 256 sender / 400 subscriber | 产生 2000000 条 conversation signal，`send_p95_ms=19.724`、`send_p99_ms=25.668`、`PullInbox p95=25.341ms`、outbox / Kafka 追平；最慢 drain 704.631s，drain rate 约 2838.365 signals/s，和 100 / 200 subscriber 档保持同一量级。 |
| push attribution 归因 | 400 subscriber Prometheus 窗口已补 writer / Redis route per-event 分解和整窗口计数 | 整窗口 `frame_write_success` 约 200.97 万、`delivery_notify_write_success` 约 200.89 万、`redis subscriber_enqueued` 约 200.89 万，writer / delivery notify / Redis subscriber error 与 eviction 均为 0；这说明问题不是 Redis 路由失败或 WebSocket 写失败，下一步应比较 push writer flush、runner 读取 / JSON decode / accounting 和网络吞吐。 |
| 多 runner 读取验证 | clean commit `9e7d4f9` 跑 1 个 coordinator + 4 个 `subscriber-only` shard | 6000 人 / 1000 消息 / 8000 msg/s / 400 subscriber 产生 400000 条 signal，4 个 shard 全部读完；按首帧到末帧计算 drain rate 约 2852 signals/s，和单 runner 400 subscriber baseline 约 2840 signals/s 基本一致，说明瓶颈不只是单 runner JSON decode / accounting。 |
| push registry 锁优化复压 | clean commit `4bc4a30` 把 push-gateway memory registry fanout 改为锁内快照、锁外写出后，重建 / redeploy 并跑 1 coordinator + 4 shard 复压 | 6000 人 / 1000 消息 / 8000 msg/s / 400 subscriber 产生 400000 条 signal，drain rate 约 2891.8 signals/s；比单 runner 400 subscriber baseline 约 2839.888 signals/s 只高约 1.8%，说明 registry 全局锁持有不是主瓶颈。 |
| push WebSocket payload 预编码复压 | clean commit `d8d78fd` 在 registry fanout 时预编码 delivery / conversation notify JSON，WebSocket writer 写 cached payload | 同样的 6000 人 / 1000 消息 / 8000 msg/s / 400 subscriber + 4 shard 场景产生 400000 条 signal，drain rate 约 2863.092 signals/s；相比单 runner baseline 只高约 0.8%，也未超过上一轮 2891.8 signals/s，说明重复 JSON marshal 也不是主瓶颈。下一步应看 WebSocket flush / per-connection scheduling / nhooyr 写入 / 网络或客户端读取背压。 |
| WebSocket writer duration 观测 | push-gateway 已新增 `frame_write` / `delivery_notify` 写耗时 histogram，hotgroup Prometheus 窗口会记录 p95 / p99 / avg / max | 这是下一轮复压的定位工具：如果 `conn.Write` 长尾高，优先优化 WebSocket flush / scheduling / 网络；如果写耗时低但 signal drain 仍慢，则继续查 subscriber read loop、runner decode / accounting 或链路吞吐。 |
| WebSocket writer duration 复压 | clean commit `4f45519` 重建 / redeploy 后，同一 400 subscriber coordinator + 4 shard 场景复压 | 400000 条 signal 全部读完，drain rate 约 2876.698 signals/s，仍未离开旧区间；Prometheus 窗口显示 `delivery_notify` write p95 / p99 约 0.345ms / 0.499ms、avg 约 0.125ms、max 约 10.056ms，说明单次 `conn.Write` 长尾不是主瓶颈，下一步应看 Redis subscriber 本地 fanout / enqueue 和 writer goroutine 调度。 |
| Redis subscriber fanout duration 复压 | clean commit `6099ecd` 重建 / redeploy 后，同一 400 subscriber coordinator + 4 shard 场景复压 | 400000 条 signal 全部读完，drain rate 约 2883.976 signals/s，仍未离开旧区间；Prometheus 窗口显示 WebSocket `delivery_notify` write p95 / p99 约 0.406ms / 0.63ms，但 Redis subscriber conversation signal fanout/enqueue p95 / p99 约 56.14ms / 91.228ms，说明瓶颈已收窄到本地 fanout/enqueue 调度，下一步应做 conversation fanout worker / shard queue。 |
| Redis subscriber signal queue 复压 | clean commit `93654117` 把 conversation signal 从 Redis subscriber receive path 切到 bounded worker / shard queue 后重建 / 归档 / redeploy | 400000 条 signal 全部读完，drain rate 约 2876.076 signals/s；queue full / worker error 为 0，queue wait p95 / p99 约 0.095ms / 0.099ms，说明快速 handoff 已成立；但 worker fanout p95 / p99 仍约 38.636ms / 87.5ms，总 drain 未突破旧曲线，下一步应看 worker 本地 fanout、session queue drain、writer 调度、flush / batching 和客户端读取背压。 |
| WebSocket writer queue / batch drain 复压 | clean commit `fedb5f43` 增加 outbound queue latency 指标和默认 16 帧 batch drain 后重建 / 归档 / redeploy | 400000 条 signal 全部读完，drain rate 约 2884.066 signals/s，仍未突破旧区间；Prometheus 窗口显示 `delivery_notify` queue p95 / p99 约 4.665ms / 4.942ms、write p95 / p99 约 0.383ms / 0.587ms，但 worker fanout p95 / p99 仍约 57.759ms / 92.241ms。结论是 writer queue / 单次 write 不是主瓶颈，下一步应做 conversation-local fanout buckets。 |
| Conversation-local fanout buckets | clean commit `a15e0ad` 已把 registry 内部 stable `session_id` bucket fanout 部署到 Ubuntu Docker，`push-gateway-ws` 配置 8 bucket，并完成同一 400 subscriber coordinator + 4 shard 复压 | 400000 条 signal 全部读完，drain rate 约 2874.378 signals/s，未突破旧区间；`delivery_notify` queue p95 / p99 约 4.616ms / 4.931ms，write p95 / p99 约 0.383ms / 0.574ms，Redis subscriber fanout p95 / p99 约 54.133ms / 90.827ms。结论：per-event bucket goroutine 不是决定性优化，下一步应评估持久 bucket worker、跨 push 实例分摊订阅或超大房间 pull-first 策略。 |
| 多 push-gateway ws 拓扑复压 | clean commit `4be4b2d` 增加 4 个 ws 实例，400 个 subscriber 按 100 / 100 / 100 / 100 分散到 10498 / 11001 / 11002 / 11003 四个 ws 端口 | 400000 条 signal 全部读完，drain rate 约 2822.479 signals/s，低于单 ws fanout-buckets baseline 约 2874.378 signals/s；4 个 Prometheus push target 均 up，writer / Redis error、queue-full 和 slow eviction 为 0。结论：简单多开 push-gateway ws 容器不是当前瓶颈解，不能把“加机器”当成热点群优化答案。 |
| Pull-first 采样式在线唤醒 | clean commit `bac71c65` 将 delivery-consumer 和 4 个 ws 实例统一配置 `NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY=10`，并用同一 400 subscriber coordinator + 4 shard 场景复压 | 6000 人 / 1000 消息 / 8000 msg/s 下 emitted signal 从 full-signal baseline 的 400000 降至 40000，signal span 从 141.719s 降至 25.243s；SendMessage / PullInbox / ACK 成立，message / delivery outbox pending=0。结论：减少在线 frame 总量能显著改善 drain，但 durable 展示仍靠 PullInbox，不能把采样 signal 当可靠投递。 |
| Sampled signal 扩大消息数复压 | clean commit `f5bc0199` 在 sample=10、400 subscriber、8000 msg/s 下把消息数扩大到 5000 | 产生并读完 200000 条 sampled signal，span 138.555s，SendMessage p95 / p99 为 18.103ms / 20.914ms，PullInbox p95 为 23.874ms，message / delivery outbox pending=0；Prometheus 显示 `delivery_notify` write p95 / p99 低于 1ms，但 Redis subscriber fanout p95 / p99 约 54.541ms / 90.908ms。结论：采样可以降压，但扩大消息数后仍是 online-signal-drain，下一步应做 room policy / adaptive cadence 或本地 fanout 持久 worker。 |
| Fanout-mode online signal policy | clean commit `37b575e5` 已把 `delivery.conversation.signal.v1` 的 `fanout_mode` 传入 push-gateway 内部 notification，并支持 `WRITE_FANOUT` / `HYBRID_FANOUT` / `READ_FANOUT` / `BROADCAST_SIGNAL` 各自配置在线 signal cadence；镜像已重建、归档、部署 | 复压 default=1、READ_FANOUT=10、BROADCAST_SIGNAL=10：6000 人 / 5000 消息 / 8000 msg/s / 400 subscriber 共读完 200000 条 sampled signal，span 141.504s，message / delivery outbox pending=0。结论：策略边界已从全局 knob 收敛到 room policy，但 READ_FANOUT=10 下瓶颈仍是 online-signal-drain。下一步应做 adaptive cadence 控制面或持久 fanout worker。 |
| Subscriber-aware signal cadence | clean commit `9bdf21c5` 已在 fanout-mode policy 之上增加 subscriber-aware threshold：可按本机 / 每个远端 gateway 的 conversation subscriber 数提高 sample_every；未配置 threshold 时保留旧采样前置快速路径，镜像已重建、归档、redeploy | 用 READ_FANOUT `100:20` 复压 6000 人 / 5000 消息 / 8000 msg/s / 400 subscriber：signal 总量从 `37b575e5` baseline 的 200000 降到 100000，message / delivery outbox pending=0，writer / Redis error=0；但 span 从 141.504s 变成 289.249s，span rate 从 1413.391 降到 345.723 signals/s。结论：这个 first-stage 静态 threshold 没有形成有效吞吐提升，后续应转 dynamic cadence、持久 fanout worker 或更强 pull-first。 |
| Redis conversation route cache | clean commit `304383ea` 在 subscriber-aware cadence 的 Redis route lookup 前增加短 TTL conversation route cache；缓存只减少重复 `SMembers + GET`，Redis 仍是权威 route 状态，订阅 / 退订 / unregister 显式失效，失败仍计错 | 同一 READ_FANOUT `100:20` 复压下，400 subscriber / 5000 消息共读完 100000 条 signal，span 从 289.249s 降到 146.62s，span rate 从 345.723 提升到 682.034 signals/s，约 1.97x；message / delivery outbox pending=0，writer / Redis subscriber error、queue-full 和 eviction 均为 0。结论：重复 route lookup 是 subscriber-aware 策略的重要成本，但仍未回到 fanout-mode policy baseline；下一步需要补 delivery-consumer debug scrape 让 cache hit / miss 可见，并转向 dynamic cadence / 持久 fanout worker / 更强 pull-first。 |
| Delivery-consumer route cache metrics | clean commit `b119716d` 为 push-gateway delivery-consumer 暴露独立 debug endpoint 和 Prometheus target；随后发现容器 env 中 READ_FANOUT policy 漂移为空，会使 subscriber-aware 场景退回全量 remote publish | 负例 run `hotgroup-routecache-metrics-400sub-5000msg` 中，5000 条消息触发约 20021 次 remote publish，100000 条 expected signal drain span 为 486.339s。它不是容量结论，而是一次配置漂移诊断：热点群压测必须绑定 clean commit、compose env 和 Prometheus target。 |
| Docker policy defaults 固定后复压 | 已在本地 compose 固定 READ_FANOUT / BROADCAST_SIGNAL 默认 sample=10、subscriber policy `100:20`，并用 `hotgroup-policydefaults-400sub-5000msg` 复压 | 4 个 shard 读完 100000 条 signal，span 193.559s，SendMessage p95 / p99 为 17.021ms / 19.347ms，message / delivery outbox pending=0；Prometheus 已看到 delivery-consumer route cache hit / miss 约 4415 / 731，remote publish window last 约 1029。结论：观测缺口已补，配置漂移已收口；但该 run 比旧 routecache baseline 慢，后续先做同配置重复复验，再决定是否优化 dynamic cadence / 持久 fanout worker。 |

面试时可以把这个结果讲成一次真实性能定位过程：

```text
我先用业务压测证明瓶颈在 delivery_outbox relay，而不是 SendMessage。
然后做 batch publish、ready index 和 worker sharding。第一次按 row id 分片失败，
因为 outbox 的顺序边界是 conversation。修正为 conversation-sharded relay 后，
100 人群 150 QPS 可以完整追平。随后我把 message outbox relay 改成 conversation-sharded
multi-worker batch publish，把 delivery outbox ready query 改成 per-conversation frontier。
再用 READ_FANOUT 路径做 6000 人 / 100 subscriber 阶梯复压，最高目标 8000 msg/s、
500000 条 online signal 也能追平。随后把 subscriber 提到 200 和 400，signal 到
1000000 和 2000000，drain rate 仍约 2.83k-2.86k signals/s。这个结果说明当前上限不在 SendMessage、outbox、
delivery projection 或 Kafka，而更接近 online signal drain / 压测端读取能力。
为了避免靠人工感觉判断，我补了一个 hotgroup 离线分析器，后续每轮压测都先从
原始 summary 生成 run matrix、瓶颈分类、证据链和下一步策略，再决定优化哪个模块；
每一轮改进策略和结果都写回 runbook / 面试文档。随后又把 Prometheus 窗口工具加上
writer / Redis route 的 per-event 归因，避免只看到
`push_writer_events_total` 这种总数而不知道是写成功、写失败、入队失败还是 subscriber
读取慢。最后我把 subscriber 拆到 4 个 runner 进程复压，结果总 drain rate 仍约
2.85k signals/s，所以后续优化重心从压测端单进程读取转到 push-gateway 写出 / flush /
per-connection 调度和网络吞吐。进一步把 400 个 subscriber 分散到 4 个
push-gateway ws 进程后，drain rate 仍没有提升，这说明热点群优化不能停留在
“多开容器”。随后我把超大房间在线唤醒改成显式 pull-first sampled signal，
sample=10 的复压把在线 signal 从 40 万降到 4 万，drain span 从 141.719s 降到
25.243s，同时 PullInbox / ACK 仍追平 durable timeline。继续把 sample=10 消息数扩大
到 5000 后，系统读完 20 万条 sampled signal，SendMessage 和 outbox 仍追平，但
Redis subscriber fanout p95 / p99 又回到约 54ms / 91ms。这个结果说明热点群不能把
WebSocket signal 当可靠投递，真正可靠展示仍要靠 PullInbox。当前已经把全局采样推进到
fanout-mode online signal policy：小群可以保持默认全量 signal，READ_FANOUT /
BROADCAST_SIGNAL 超大房间可以单独配置更低在线唤醒 cadence。clean Docker redeploy 后
复压证明策略按房间类型生效，但 READ_FANOUT=10 下性能与 global sample=10 同量级，
所以后续不能继续只调 sample knob。随后我把策略推进到 subscriber-aware cadence：
同一个 fanout mode 下，还要看实际在线订阅压力，本机或远端 gateway 的 subscriber 数越大，
online signal cadence 越保守；但 PullInbox / ACK 仍是 durable truth。复压结果说明这个
first-stage 静态 threshold 能降低 emitted signal 数，但没有改善 drain span，甚至把 span rate
拉低了。因此下一步不能继续堆静态阈值，而应做消息速率 / 在线人数感知 dynamic cadence、
持久 per-conversation / per-bucket fanout worker，或对超大房间采用更强 pull-first 策略。
```

2026-06-29 的小规模 smoke 进一步证明了策略切换链路：

```text
conversation-service promoted fanout_mode=BROADCAST_SIGNAL
-> message-service uses timeline-service AllocateSeqBlock with local seq block cache
-> delivery-service writes delivery_timeline_items instead of per-user user_inbox
-> delivery_outbox publishes conversation-level signal
-> push-gateway sends delivery.notify to conversation subscribers
-> sampled receivers still recover through PullInbox / AckDelivery
```

本轮数据：

```text
group_size=61
message_count=20
conversation_mode=SEQUENCER_BLOCK
fanout_mode=BROADCAST_SIGNAL
conversation_subscribers=3
conversation_signal_count=60
user_inbox_rows=0
delivery_timeline_rows=20
delivery_outbox_pending=0
message_outbox_dlq=0
delivery_outbox_dlq=0
```

这仍然是小规模 smoke，不是容量上限。下一步要扩大三机压测规模，并记录 Kafka lag、
delivery projection lag、push signal、PullInbox / ACK 和 PostgreSQL 指标曲线。

2026-06-30 的中等规模诊断还暴露了一个关键边界：热点会话不仅消息写入要走
timeline-service sequencer，成员边界事件也要走同一个 sequencer。否则群被提升到
`SEQUENCER_BLOCK` 后，继续 JOIN 成员时会 fail-closed。当前修复后的正式结论仍要用
clean commit 重跑后再写入容量报告。

随后 clean commit `d13bff6c` 已重建镜像并 redeploy，通过三档复验。当前可以讲成：

```text
热点群 first-stage 已经从“普通群写扩散”推进到“timeline sequencer + conversation signal”。
500 人 / 1000 消息 / 50 在线订阅者下，系统产生 5 万条 conversation signal，
但不再写 50 万级 user_inbox 行；delivery_outbox、Kafka lag 和 PullInbox / ACK 抽样均追平。
```

后续 clean commit `01b2a70` 进一步把 READ_FANOUT 路径放大到 6000 人、100 个
WebSocket subscriber，并跑 400 / 800 / 1200 / 2000 / 4000 / 8000 msg/s 目标档。
最高档发送 5000 条消息、产生 500000 条 conversation signal，全部 subscriber 读完，
`delivery_outbox_pending=0`、Kafka lag=0。这个结果仍不是生产容量上限；下一步要继续
提高 subscriber 数、在线比例和总 signal 数，并把 Prometheus 时间窗口、自动分析报告和
PostgreSQL / Kafka / projection 指标一起归档；最高档已补一轮低敏 Prometheus 窗口，
后续继续扩大曲线时必须同样记录窗口。

2026-07-01 的 200 / 400 subscriber 阶梯把同一 READ_FANOUT 压测固定在 6000 人、
5000 消息、目标 8000 msg/s 和 256 sender，只改变在线订阅者数量。三档结果的
drain rate 稳定在约 2.83k-2.86k signals/s，因此下一步面试叙事应从“继续加压”
转为“如何定位并优化 online signal drain”：区分 push-gateway writer flush、Redis
route fanout、session queue 和压测 runner 读取能力。新的 attribution 窗口已经显示
Redis subscriber enqueue 与 WebSocket writer success 都能到约 200 万级，并且没有
write error / eviction，所以后续重点不应再讲“消息队列不够”，而应讲如何进一步验证
客户端读取侧、JSON 编解码、writer flush 策略和网络吞吐。为此 runner 已补
subscriber-only shard 模式，并完成 4 shard 对照：drain rate 没有明显超过单 runner
baseline，因此下一步应进入 push-gateway conversation signal 写出路径、WebSocket flush
cadence、Redis subscriber fanout、per-connection write scheduling 和有线网络吞吐优化。
第一轮代码级优化先收缩 memory registry fanout 的锁持有范围，复压后 drain rate
约 2891.8 signals/s，仅比旧 baseline 高约 1.8%，所以它不是主瓶颈。第二轮优化
改为 WebSocket payload 预编码，复压后 drain rate 约 2863.092 signals/s，也没有离开
约 2.85k-2.89k signals/s 的旧区间。因此当前讲法应明确：瓶颈不在 SendMessage、
message outbox、delivery projection、Kafka、Redis route、registry mutex 或重复 JSON
marshal，而更接近 WebSocket writer flush / per-connection scheduling / nhooyr 写入 /
网络吞吐 / 客户端读取背压。为此 push-gateway 已补 WebSocket writer duration
histogram，下一轮 400 subscriber coordinator + shard 复压要把 `delivery_notify`
write p95 / p99 / avg / max 也纳入报告，避免继续用 CPU 空闲这类间接现象判断瓶颈。
复压结果显示写耗时 p99 低于 1ms，但总 drain rate 仍约 2.88k signals/s；随后
Redis subscriber fanout duration 复压显示 conversation signal fanout/enqueue p95 / p99
约 56ms / 91ms。再把 Redis subscriber receive path 改成 bounded worker / shard queue
后，queue wait p99 约 0.099ms、queue full / worker error 为 0，但总 drain rate
仍约 2.876k signals/s。因此当前讲法应进一步收窄：瓶颈已经不是 Redis Pub/Sub
receive path。最新的 writer queue / batch drain 复压显示 queue p99 约 4.94ms、
单次 write p99 约 0.59ms，但 worker fanout p99 仍约 92ms，整体 drain rate 仍约
2.884k signals/s。因此瓶颈更接近“一个 conversation signal 串行 fanout 到本机
400 个 session”的本地 fanout 模型。下一步可讲的优化方向是 conversation-local
fanout buckets：把订阅者按稳定 bucket 拆分并行 fanout，但每个 session 内仍保持顺序，
慢连接仍通过 queue full / eviction 和 PullInbox 恢复。

### 当前系统如何承接热点群

| 压力点 | 当前边界 |
| --- | --- |
| 消息写入 | `message-service` 只写 `message_log` 和 outbox，不同步写所有群成员 inbox。 |
| 消息顺序 | 普通群由 message 写入链路维持 conversation scope 的 seq 语义；热点 `SEQUENCER_BLOCK` 会由 message-service 通过 timeline-service `AllocateSeqBlock` 和本地 seq block cache 取号，只在 valid block lease、epoch、lease_id 和未过期 lease 下写入。 |
| 事件传播 | `message_outbox -> Kafka conversation.timeline.events`，业务事务不直接 publish Kafka。 |
| 成员可见性 | `conversation-service` 是成员事实源；`delivery-service` 使用 membership projection，不实时回查当前成员来改写历史可见性。 |
| 投递 fanout | `delivery-service` 异步生成 `user_inbox`，失败可 fail-closed、repair / redrive，不阻塞发送主路径。 |
| 在线通知 | `push-gateway` 只发轻量 `delivery.notify` / conversation signal notify，不保存消息事实，不拥有 durable inbox。 |
| 慢连接 | slow session close / resume hint / PullInbox 兜底，不能让慢 WebSocket 卡住 Kafka commit。 |
| 多实例在线路由 | Redis route 支撑 first-stage cross-instance notify；Redis 故障时在线唤醒可降级，消息不丢。 |
| 幂等 | outbox event id、projection unique key、inbox unique constraint 和 ACK cursor 事务语义保证重复消费不重复生成事实。 |

### 热点群的演进策略

普通群可以继续使用 write fanout：

```text
SendMessage
-> message_log + message_outbox
-> Kafka timeline
-> delivery-service fanout user_inbox
-> push-gateway online notify
-> client PullInbox / AckDelivery
```

当群规模和消息速率继续上升时，优先优化模型，而不是马上堆新中间件：

1. `delivery-service` 按 `conversation_id + user_bucket` 做 fanout 分桶，避免一个 worker 串行处理所有成员。
2. `user_inbox` 使用批量 insert / 分批事务 / 幂等 unique key，减少 PostgreSQL roundtrip 和锁竞争；当前 materialized inbox 已先把 per-recipient insert 合并成批量 SQL。
3. delivery timeline consumer 使用同 consumer group 多 worker，由 Kafka 按 partition 分配任务；这提升跨 conversation / partition projection 并行度，但不破坏单 conversation 顺序。
4. 在线 push 只向订阅 conversation 的在线 session 发轻量 notify，离线和慢客户端只依赖 PullInbox 补拉；push-gateway 已有 conversation subscribe / unsubscribe 和 conversation signal fanout 服务端 first path。
5. 超大群可切 lazy inbox：不为全部离线冷用户即时物化 inbox，而是在 PullInbox 时结合 timeline、visibility window 和 cursor 计算可见消息。
6. 对活跃用户、`@我`、置顶会话、在线设备优先 materialize；冷用户延迟或按需 materialize。
7. Kafka 上保留 timeline 的顺序语义，但 delivery fanout 可以拆成 fanout shard topic / user bucket，不把展示顺序和投递并行度绑定死。
8. 当单会话写入热点超过本地 row lock 能力时，引入 `timeline-service` 的 seq block allocator；allocator 已有 lease status、gap marker 和 repair operator first path，message-service 已接 active `SEQUENCER_BLOCK` seq block cache 写路径，拿不到 valid lease 或 lease 过期时仍必须 fail-closed，不能悄悄回退成普通 row-lock。

只有压测证明当前模型和拓扑已经成为硬瓶颈时，才考虑新增或升级中间件：

| 触发条件 | 可能动作 |
| --- | --- |
| Redis route / online session 查询成为瓶颈 | 从 Redis single / Sentinel 升级 Redis Cluster，或为 presence/push 做更细分热状态缓存。 |
| PostgreSQL `user_inbox` 写放大不可接受 | 先做 fanout bucket / lazy inbox；仍不足时评估 Citus、CockroachDB、ScyllaDB / Cassandra 类宽表存储。 |
| Kafka 单 conversation partition 热点过高 | timeline 保序不变，delivery fanout 拆 bucket；必要时增加 fanout shard topic。 |
| 历史搜索 / 大群检索压力上升 | 引入或强化 OpenSearch / vector backend，但搜索索引不替代消息事实源。 |
| 大规模在线状态聚合 | presence-service + Redis Cluster / 专用热状态存储，而不是让 push-gateway 变成事实源。 |

### 热点群压测应该证明什么

热点群不能只用 `wrk` 或 `k6` 打单个 HTTP 接口。NexusIM 后续应补
`loadtest/hotgroup` 业务 runner，至少覆盖：

```text
group_size
online_ratio
sender_count
message_rate
duration
SendMessage p95 / p99
conversation_seq 是否连续
Kafka timeline lag
delivery projection lag
user_inbox inserted rows/s
PullInbox visible latency p95 / p99
AckDelivery p95 / p99
push notify received / dropped / slow evicted
outbox pending / DLQ count
PostgreSQL pool / lock / WAL 指标
```

面试时可以这样收束：

```text
我不会把热点群聊简单理解成 SendMessage QPS。我的方案是先用 outbox、Kafka、
delivery fanout、durable inbox 和 push wakeup 拆开写入事实、投递和在线通知；
再通过热点群业务压测观察 fanout、Kafka lag、user_inbox 写放大、push 风暴和 ACK
追平能力。只有压测证明瓶颈不能靠 fanout 分桶、批量写、lazy inbox 和 Redis/Kafka
拓扑优化解决时，才引入新的中间件。
```

## 已完成的分布式与可靠性能力

当前已经做过的关键验证：

- 本地多进程 smoke；
- Win / Mac 双机 Docker smoke；
- Redis route / Redis-backed resume；
- Redis stop / start recovery；
- Redis Sentinel discovery / failover / master-stop / quorum-loss recovery；
- Redis Cluster 本地三节点 topology smoke；
- Redis Cluster node-stop recovery smoke；
- Redis Cluster 六节点自动 failover smoke；
- Redis Cluster 六节点短容量基线；
- PostgreSQL `repmgr + pgpool` local failover smoke；
- Kafka KRaft 3 broker local leader failover / controller-switch / ISR observation smoke；
- Kafka KRaft repeated ISR flapping smoke：2 轮 broker stop/start 均观察到 ISR 收缩 / 恢复和 `acks=all` probe 写入成功；
- Kafka producer hardening evaluation：7 个 producer package 固定 `acks=all`、禁自动建 topic、bounded retry/backoff，并明确当前 `kafka-go` 不声明 idempotent / transactional producer 语义，业务可靠性边界仍是 outbox / event_id 幂等。
- Kafka producer fault observation：本地 `kafka-go` producer 在 broker stop/restore 窗口内写入 120 条 records，消费侧观察到 unique 120、missing ack 0、duplicate 0；这只是本地观察，不是 exactly-once 证明。
- Kafka consumer group rebalance smoke：两个 push-gateway delivery-consumer 进入同一 group，停止一个后，`im.delivery.events` 的 3 个 partition 被重新分配给剩余 consumer。
- Kafka consumer churn smoke：2 个 push-gateway delivery-consumer 在同一 group 中反复 leave / rejoin，2 轮 8 个 transition 均回到 Stable，且 3 个 partition 都已分配。
- Kafka consumer churn probe smoke：8 个 transition 后共写入 24 条合法 `delivery.inbox_item.created.v1` probe，producer ack 24，consumer group 每次 post-probe lag 回到 0。

这些验证证明：

- 在线通知可以跨实例工作；
- 在线通知失败时，durable `PullInbox + AckDelivery` 能兜底；
- Redis、Kafka、PostgreSQL 单点切换后，最小链路可以恢复；
- Kafka 在本地 RF=3 / min.insync.replicas=2 下，一 broker down 仍可写，两 broker down 会按 `NOT_ENOUGH_REPLICAS` fail-closed；
- Kafka repeated ISR flapping 下，本地 broker stop/start 后 ISR 能在 2 / 3 之间按预期收缩和恢复；
- Kafka producer fault observation 下，本地已 ack records 可以在消费侧全部找到，同时继续保留 outbox / event_id 幂等作为业务可靠性边界；
- Kafka consumer group 可以完成第一阶段本地 rebalance；
- Kafka consumer group 可以完成第一阶段 repeated leave / rejoin churn；
- Kafka consumer group 在本地 churn 后仍可继续消费合法 delivery event 并提交到 zero lag；
- 多个 worker / relay 已具备退避重试和 fail-closed 行为；
- outbox / projection / challenge delivery 具备第一阶段 audit / repair / cleanup。

## 已完成的安全与运维基础

当前已经落地：

- 各核心服务的 `/healthz`、`/readyz`、`/debug/metrics`，以及当前 9 个服务第一阶段 Prometheus text `/metrics`、本地 scrape / alert rules 原型、本地 Grafana dashboard 原型和 first-stage trace sampling policy / check；
- gRPC / WebSocket 公网监听下的弱鉴权 / 明文入口启动门禁；
- trusted metadata 和 mTLS 边界的第一阶段收口；
- gateway token、JWKS、RS256 key overlap；
- identity MFA / recovery code / refresh step-up；
- challenge delivery outbox、retry、DLQ、repair audit；
- outbox / projection repair 和 cleanup operator；
- 低敏 debug metrics，不暴露 token、secret、TOTP、recovery code、用户敏感标识。

## 待开发功能清单

这里按面试表达分层。当前没有已知 P0 / P1 阻塞；下面主要是还没完成的产品能力、生产化能力和大模型应用能力。

### 当前：客户端平台 first slice + AI / Agent 底座继续演进

当前 9 个服务作为 Go 底座，默认只回补阻塞 client / AI / product platform 主线的
P0/P1；不以生产级完整系统测试或生产级 HA 作为当前切片阻塞：

统一推进顺序：

1. 当前先完成 client platform MVP foundation：Web -> Windows PC 优先，Android 后置；
   三端共用 `protocol` / `client-core`，客户端只连 `api-gateway` / `push-gateway`。
2. AI 继续扩展 low-sensitive collaborative-memory eval cases，优先 multi-hop /
   temporal update / profile aggregation。
3. 让 Python AI Worker 输出算法候选，Go 继续做权限、状态、审批、审计和 eval。
4. 保持 EvidencePack、source refs、visibility、review state 和 proposal / approval / audit 不回退。
5. 生产级完整系统测试和生产级 HA 深水区继续后置。

| 服务 | 待开发 / 待完善功能 |
| --- | --- |
| `api-gateway` | 保持 facade-only 默认、trusted metadata、TLS / mTLS、quota snapshot gate 和 legacy opt-in / observation evidence 不回退；为 search / retrieval 后续入口预留安全 header、trace 和 tenant context 边界。provider-grade 配置中心、灰度治理、多环境发布审计和生产观测平台归入后置 hardening |
| `identity-service` | WebAuthn / passkeys、外部 OIDC federation / OAuth client flows、多 issuer 治理、真正的 KMS / HSM-backed key management、完整登录风控、SMS provider、bounce handling、多租户通知模板 |
| `message-service` | 收紧编辑 / 撤回 / 删除 / 合规删除对 timeline、outbox、tombstone 和 search / memory 消费的事件语义；图片 / 文件 / 语音二进制上传处理后续交给 media 能力。provider-grade proof 工作流、容量曲线和生产观测归入后置 hardening |
| `conversation-service` | 继续收紧 owner transfer、群管理、成员边界事件、成员可见窗口和窗口 repair，保证 search / memory / EvidencePack 能按历史窗口过滤。完整 targeted replay 和生产观测归入后置 hardening |
| `delivery-service` | 保持 durable inbox / PullInbox / AckDelivery / projection repair 边界，为 retrieval / Agent 的可见性兜底提供可靠投递事实；更多 delivery event 消费方、容量曲线和生产观测归入后置 hardening |
| `push-gateway` | 保持在线通知、Redis route、resume recovery 和 PullInbox 兜底边界，不让在线层承担 durable inbox；生产级 Redis HA、跨实例 resume 生产化、长时间容量曲线和生产 sizing 归入后置 hardening |
| `receipt-service` | 补齐 AI 需要的 unread / receipt / conversation summary 低敏聚合语义，避免 retrieval 或 summary 直接读投递内部表；更多会话列表产品化摘要策略归入后续产品能力 |
| `contacts-service` | 收紧联系人隐私、字段级 profile visibility、联系人搜索、分组和来源审批对 memory / profile projection / retrieval 的影响；组织级策略和 admin/config 正式权限面归入后续平台能力 |
| `policy-service` | first-stage tool policy precheck 已落地；继续收紧 decision_source、relationship gate、contacts projection 和 decision audit 对 retrieval / Agent 的可解释权限边界；provider-grade ReBAC graph / DSL、moderation / risk scoring、tool policy operator / approval integration 和外部 audit pipeline 归入后置 hardening |

### AI 大模型应用底座

AI 能力按依赖逐步进入。`search-service` / group memory / retrieval 是前置基础，
先把搜索、可见性、证据和版本语义做好，再进入 RAG 和 Agent。服务数量不写死，
只有满足独立数据模型、独立伸缩需求、独立故障边界或能明显降低现有服务复杂度时才拆。

| 待开发服务 / 能力 | 目标 |
| --- | --- |
| `search-service` | 聊天记录搜索、索引、成员可见窗口过滤、编辑 / 撤回 / 删除 tombstone；第一切片已落 proto / migration / skeleton / PG repository / SearchMessages / grpc runtime / timeline consumer，并已跑通 projection smoke |
| `memory-service` / group memory projection | 已从 SDD / proto / migration contract 推进到 foundation-active implementation，并跑通 clean projection smoke；多人、多群、多时间版本的 StructuredMemoryEvent、Memory Graph、ProfileAggregate；memory 必须有 source refs、speaker / audience、valid_from / valid_to、supersedes、confidence 和 review state；状态至少区分 PENDING / ACTIVE / SUPERSEDED / REJECTED / ARCHIVED，单条群消息不能直接升级成长期个人画像 |
| `retrieval-gateway` | 统一结构过滤、BM25 / 向量 / 图扩展、policy check 和 EvidencePack |
| `rag-service` | 第一版只读问答路径、真实本地 adapter smoke、provider boundary、citation verifier first pass 和 guarded external HTTP LLM boundary 已落；基于权限过滤后的 EvidencePack 生成 deterministic extractive answer，保留 citations，无 evidence 时拒答，可选 external-http 输出必须通过 EvidencePack prompt guard 和 citation verifier |
| `summary-service` | 第一版只读 EvidencePack 会话摘要路径、真实 adapter smoke 和 guarded external HTTP LLM boundary 已落；后续补未读摘要和日报 |
| `agent-service` | 第一版 proposal-only path、真实本地 adapter smoke、MCP prepare、proposal store、approval preflight、approval outbox relay 和 approval operator 已落；当前 proposal 前调用 `mcp-gateway.PrepareToolCall`，通过 skill catalog / policy precheck / prepare audit 后再消费 EvidencePack 生成可引用 proposal，审批时同事务写 `agent.proposal.approved.v1` 低敏 outbox，并由 relay 发布低敏 `im.agent.events`，同时通过 `VerifyApprovedAgentProposal` 给 executor 做公开校验；本地 operator 支持低敏审计和默认 dry-run 审批，不执行真实动作 |
| `skill-registry` | 第一版技能合约目录已落；把可复用的 IM / knowledge 工作流沉淀为可版本化、可审计的 Skill metadata，但不执行工具 |
| `mcp-gateway/tool-gateway` | 第一版 prepare 边界已落；把 skill-registry 技能合约、policy-service `CheckToolAction` 和低敏 audit 串起来，但不执行外部 MCP tool |
| `action-executor` | 第一版 approved execution audit、low-sensitive tool result projection 和本地安全 `nexusim.local.echo` adapter 已落；强制 proposal / approval / prepare audit 关联，重新做 skill execute contract check 和 policy execute precheck；业务 tool 仍只记录低敏审计和结果引用，`executed=false`，本地安全 echo tool 可 `SUCCEEDED` 并只记录 output hash |
| Python AI Worker | ADR-036 已固定边界，`ai/python` 目录、`IM` conda toolchain、candidate contract helpers、低敏 safety guard、contract validator、candidate-only worker CLI、output-safety eval adapter、第一条 worker smoke、Go-side adapter smoke 和 RAG / Summary 服务级 candidate guard 已落；后续只作为 LLM / embedding / rerank / memory extraction / planner / eval 候选层接入，最终校验、权限、审批、审计和持久化仍由 Go 服务完成 |
| evidence pack | AI 输出必须携带 source message id、conversation seq、conversation id |
| Agent 写动作链路 | Proposal -> Approval -> Executor -> Audit，避免 Agent 直接改业务事实 |

### 后续：完整 IM 产品和平台服务

产品级后端按完整目标架构拆分，不阻塞当前 client platform MVP 或 AI 底座继续推进。

| 待开发服务 / 能力 | 目标 |
| --- | --- |
| `media-service` | 图片、语音、视频、文件上传下载、对象存储、缩略图、病毒扫描、语音转码 / 时长探测 |
| `notification-service` | 邮件、短信、APNs / FCM、系统通知、模板、bounce handling |
| `audit-service` | 登录审计、安全审计、管理操作审计、策略决策归档 |
| `admin-service` | 租户管理、封禁、配置、运维操作、repair 工作台；第一版已把 repair 和 CRITICAL 管理操作接入 workflow 长审批，提供公开 gRPC operator CLI 做创建 / 审批 / 查询，并打通非 critical `CONFIG_PUBLISH -> control-plane-service` 第一条真实下游 adapter |
| `control-plane-service` | 多租户配置、功能开关、限流策略、灰度、配额、动态策略发布和 applied-version ACK |
| `presence-service` | 在线状态、输入中、最后在线时间；当前 push-gateway session registry 还不是完整 presence 服务 |
| `model-gateway` | 统一模型 provider、embedding、rerank、成本、recovery、prompt policy 和低敏审计 |
| `workflow-service` | Agent / repair / retention 的长事务、审批等待、补偿和 operator workflow；当前已支撑 action / repair / admin operation approval first path、显式 approval timeout -> `TIMED_OUT`、provider replay queue / operator queues 低敏队列视图、external decision manifest binding、external callback wait、compensation review page 和 execution readiness manifest |
| `knowledge-ingestion-service` | 文件解析、网页导入、企业知识库导入、chunking、embedding pipeline 和导入审计 |
| `vector-index-service` | 向量索引写入、重建、backfill；满足独立伸缩 / 重建边界后再拆 |

### 后置 hardening：生产级分布式平台能力

当前已经做了本地 / 双机 smoke，但还没完整证明生产级 HA。这些工作是上线加固，不作为进入 search / memory / retrieval 的短期阻塞。后续待开发 / 待验证：

- Redis Cluster 容量验证；
- 生产级 Redis HA；
- PostgreSQL split-brain / quorum / 跨机存储故障；
- Kafka multi-failure / controller failover / ISR 抖动；
- 完整服务发现；
- 统一 OpenTelemetry trace / alert / dashboard；
- 结构化日志和统一告警；
- 灰度发布、部署编排、配置治理；
- 运维 UI / repair approval workflow。

## 客户端和产品化展示

Web / PC / Android 已进入当前工程切片；面试时是否重点讲，取决于岗位侧重点。
如果岗位偏后端 / 大模型应用，客户端只作为端到端展示入口简述即可。

面试时优先讲下面能力：

```text
后端微服务主链路；
分布式可靠性；
安全、观测、repair 和运维 hardening；
search / memory / retrieval / RAG / Agent 后端能力；
完整目标架构下的业务平台、数据平台、AI / Agent 平台和中间件平台边界。
```

## 当前开发阶段

当前阶段是：

```text
Go 微服务底座已支撑 AI / Agent 和客户端平台 first slice；
当前推进 client platform MVP foundation，同时保持 collaborative-memory 算法/eval
和完整目标架构作为后续主线。
```

短期优先级：

1. client platform：接 PC desktop Tauri runner 和 Android runtime shell；
2. AI / Agent：继续扩展 collaborative-memory eval，优先 multi-hop / temporal
   update / profile aggregation；
3. 平台服务：按完整架构推进 product-active 服务的 worker / relay / provider
   adapter / smoke；
4. 对 9 个既有服务只回补阻塞 client / AI / product platform 主线的 P0/P1；
5. 生产级完整系统测试、长压和 sizing 后置。

## 面试讲述线

可以这样介绍：

```text
我实现了一个事件驱动的分布式 IM 后端。系统用 PostgreSQL 作为交易事实源，用 outbox 保证业务事务和事件发布之间的一致性，用 Kafka 传播 timeline 和投递事件，用 delivery-service 构建 durable inbox，push-gateway 只负责在线唤醒，不承担可靠投递。这样即使 WebSocket、Redis route 或 push-gateway 出问题，客户端仍可以通过 PullInbox 和 AckDelivery 恢复状态。

在身份侧，我实现了登录、Refresh Token、MFA、recovery code、JWKS、challenge delivery outbox、SMTP / webhook challenge sender 和启动安全门禁。系统也补了 health、ready、debug metrics、repair、audit、cleanup、worker retry 和多种本地故障 smoke。

现在 Go 微服务底座已经支撑 AI 应用链路：search、memory、retrieval、RAG、summary、Agent、skill-registry、mcp-gateway、action-executor 和 ai-eval 都有 first path 或 smoke，RAG / Summary / Agent 已通过 cross-group / temporal optional stack gate。完整目标架构已经扩展到业务平台、数据平台、AI / Agent 平台、客户端平台和中间件平台；当前工程切片是客户端平台 MVP，Web first path、api-gateway client BFF、push path、本地和 wired 172 clean baseline 已通过，Web / PC 已具备好友私聊、群聊、群成员管理、群资料和消息 first path，PC / Android runtime skeleton 已落。大模型只能通过权限过滤后的 EvidencePack 访问聊天记录，Agent 写动作必须走 proposal、approval、executor 和 audit，Python AI Worker 只返回候选，Go 继续拥有权限、状态、审计和持久化。
```

## 维护规则

- 这个文档只在阶段变化时更新。
- 不记录每个提交的流水账。
- 新服务完成真实链路后，更新“已完成的后端服务”。
- 新的 smoke 证据仍写入 `docs/runbook/loadtest/<service>/`。
- 新的详细设计仍写入 `docs/sdd/`。
