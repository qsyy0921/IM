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
9 个后端服务已经能跑通主链路，并作为 AI 主线的 Go 底座；
短期不以生产级完整系统测试或生产级 HA 作为算法/eval 前置阻塞，验证重点放在低敏 cases、EvidencePack、权限过滤、source refs、时间版本和审计边界；
search / memory / retrieval / RAG / summary / Agent / skill / MCP / executor / ai-eval first paths 已落，RAG / Summary / Agent stack 已通过 cross-group / temporal optional gate；
future platform / product services 已进入 product-active first-stage implementation；
client platform MVP foundation 已启动：Web first path、api-gateway client BFF、push path、本地和 wired 172 clean baseline 已通过，PC Tauri WebView metadata smoke 已通过，Web / PC shell 已接账号登录、注册、好友、好友私聊、群聊、群成员管理、群标题 / 头像 URI read-update、消息列表和发送 first path，真实双用户 direct + group client smoke 已通过；Android APK / 真机 smoke 后置到用户明确切回；
完整目标架构以 docs/architecture/target-architecture-complete.md 为基线，后续按业务平台、数据平台、AI / Agent 平台、客户端平台和中间件平台演进；
后续 AI 按 low-sensitive collaborative-memory 算法/eval 推进，优先 multi-hop / temporal update / profile aggregation；
api-gateway 已补 first-stage tenant-scoped rate limit、静态 tenant plan override、tenant plan 文件热更新、版本化 quota URL source、DB-backed tenant plan snapshot source、本地 tenant quota audit / set operator、tenant quota approval manifest 强制校验、URL bearer token / HTTPS guard、URL source CA / client cert TLS 边界、可选 checksum-required gate、applied quota snapshot stale 观测和 quota snapshot gate；
api-gateway 已补 legacy/facade traffic metrics，以及 legacy observation-window / removal-plan 低敏 evidence manifest，用于旧 descriptor 迁移观察和归档；
legacy descriptor 已收敛为显式 opt-in 默认；
当前 9 个服务已补 first-stage Prometheus text /metrics、本地 Prometheus alert rules 和本地 Grafana dashboard 原型；
api-gateway 已补 first-stage OpenTelemetry 入口 server span 和下游 gRPC client span；当前 9 个服务均已纳入 first-stage trace runtime wiring，其中 8 个后端 gRPC 服务使用 server span，push-gateway 使用 WebSocket connection span，并由采样策略和本地 check-local 门禁约束；
本地 OTel collector debug 入口和 policy OTLP smoke 脚本已补，可用于面试演示 OTLP trace 链路，但还不是生产告警平台；
search-service v0.1 / group memory / retrieval / RAG / summary / Agent / skill / MCP / action-executor / ai-eval 已分别完成 first path、smoke 或 eval gate；当前 Go 底座已足够支撑算法切片，下一步进入 low-sensitive collaborative-memory 算法/eval；
后续开发可以使用 multi sub-agent 并行推进，但以服务 / 文档 / 测试面拆分，最终由主 agent 统一集成和验证；
面试叙述仍优先强调后端、分布式和 AI / Agent；客户端作为产品化展示层和端到端验证入口按需讲。
```

## 已完成的后端服务

当前已有 9 个真实后端微服务：

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
| `workflow-service` | Agent / repair / retention 的长事务、审批等待、补偿和 operator workflow；当前已支撑 action / repair / admin operation approval first path、显式 approval timeout -> `TIMED_OUT`、provider replay queue 低敏队列视图和 external decision manifest binding |
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
