# NexusIM Development Process

这份文档回答的是：

```text
这个系统应该按什么顺序开发，
每个阶段的目标是什么，
什么时候可以进入下一阶段。
```

它不是详细设计，也不是当前状态快照。

- 当前状态看 `development-progress.md`
- 每轮入口看 `current-brief.md`
- 长期摘要看 `current-goal.md`

## 0. 总原则

NexusIM 的开发顺序不是“先把所有功能铺开”，而是：

```text
先把主链路做对
-> 把 9 个核心服务做成 AI-ready 底座
-> 建 search / memory / retrieval / EvidencePack
-> 建 RAG / summary / Agent / skill-registry / mcp-gateway / action-executor / Python AI Worker
-> 进入 collaborative-memory 算法/eval
-> 分布式可靠性和生产级完整测试按风险持续补强
-> 最后按需要做客户端和产品化展示
```

核心原则：

1. 不为了“微服务数量”而拆服务。
2. 不为了“架构好看”提前抽象。
3. 不在基础链路没收稳前同时铺太多新服务。
4. 功能能跑不等于阶段完成，必须有 smoke / audit / repair / hardening 证据。
5. 代码复杂度要持续治理，不能把单个核心文件继续堆成大文件。
6. 当前面试主线只覆盖后端、分布式可靠性和 AI 应用后端；客户端暂不作为当前开发主线。
7. 短期转进 AI 大模型应用后端不以生产级完整系统测试或生产级 HA 为阻塞，但当前切片仍必须有本地检查、最小 smoke 或等价证据。
8. 可以使用 multi sub-agent 并行开发，但只能拆分互不重叠的服务、文档或测试面；主 agent 负责集成、最终验证和关闭 stale agents。

## 1. 第一阶段：最小 IM 主链路

目标：

```text
把 IM 后端从 demo 提升到可工作的最小链路。
```

这一阶段先做：

- 身份与登录基础能力
- 会话 / 成员 / 权限读取
- 发送消息
- timeline / outbox / Kafka
- durable inbox
- `PullInbox` / `AckDelivery`
- WebSocket 在线通知

这一阶段完成标志：

- 消息从发送到投递、ACK 能串起来
- 核心服务不再是 mock-only
- 至少有真实 PostgreSQL / Kafka / gRPC / WebSocket 链路 smoke

当前这个阶段已经完成。

## 2. 第二阶段：把 9 个核心服务做成 AI-ready IM 底座

目标：

```text
不急着做孤立 LLM demo，
先把已有 9 个核心服务补成 search / memory / Agent 能安全依赖的 IM 后端底座。
```

当前核心服务是：

- `api-gateway`
- `identity-service`
- `message-service`
- `conversation-service`
- `delivery-service`
- `push-gateway`
- `receipt-service`
- `contacts-service`
- `policy-service`

这一阶段重点不只是 hardening，也包括 AI 所需的 IM 产品语义：

- 消息编辑 / 撤回 / 删除 / 合规删除的事件语义；
- 群管理、owner transfer、成员历史可见窗口；
- 回执、未读、会话摘要、联系人分组 / 搜索 / 隐私策略；
- policy 决策来源、moderation、关系门禁和 audit；
- `/healthz` / `/readyz` / `/debug/metrics`
- trusted metadata / TLS / mTLS 边界
- outbox / projection / challenge delivery 的 repair / audit / cleanup
- worker / relay 退避重试
- fail-closed 语义
- 文件拆分、降低耦合、控制复杂度

进入下一阶段前，至少要满足：

- 这 9 个服务都不是“只跑 happy path”
- 各自主要 P2 hardening 已经收掉大头
- search / memory / Agent 会依赖的事实、权限、事件和 tombstone 边界清楚
- 关键文档、service brief、runbook 已经稳定
- 不要求生产级完整系统测试或生产级 HA 全部完成后才启动 `search-service` v0.1

当前阶段已经完成到可支撑 AI 算法切片；后续只在阻塞 AI 主线时回补九服务收口。

当前执行规则：

1. 每次只解决一个服务或一个明确跨服务边界的问题。
2. 优先做 search / memory / Agent 会依赖的 IM 语义和安全边界。
3. 不为了面试说法一次性铺开多个 AI 服务；先让 search / memory / retrieval 有可信数据源。
4. 每个切片完成后更新对应 service brief，并运行本地检查。
5. 生产代码文件接近 2500 行、测试或 runner 接近 3000 行时，优先同 package 拆文件。

## 3. 第三阶段：分布式与可靠性

目标：

```text
证明它不是单机 WebSocket demo，
而是有明确故障边界和恢复路径的分布式 IM 后端。
```

这一阶段重点做：

- 多进程 / 多实例 smoke
- Win / Mac 双机 smoke
- Redis route / resume / fallback
- PostgreSQL failover smoke
- Kafka failover smoke
- Redis Sentinel / quorum / 网络故障 smoke

这一阶段的关键判断标准：

- 在线通知可以失败，但 durable 路径不能丢
- Redis、Kafka、PostgreSQL 局部故障后，最小链路仍可恢复
- 恢复路径要有真实证据，不靠口头描述

注意：

- 本地 failover smoke 不等于生产 HA
- 单 broker / 单 primary 切换成功，不等于多故障场景完成
- 生产级 HA 和完整系统测试是持续补强目标，不作为 `search-service` v0.1 的短期阻塞

这一阶段已经有可面试讲述的最小证据，但还没到生产级 HA。

## 4. 第四阶段：完整产品能力

目标：

```text
从“后端链路能跑”推进到“IM 产品基本完整”。
```

这阶段再做的内容包括：

- 已读 / 送达 / 未读数
- 编辑 / 撤回 / 删除
- 会话列表
- 群管理
- 联系人完善
- 真实鉴权闭环
- 更完整的管理与运营入口

这阶段的重点是：

- 补齐产品语义
- 继续保证分布式边界不被打穿
- 不把新能力直接塞回已有大文件

当前已有 9 个服务已经覆盖这阶段的一部分能力。现在的重点不是继续铺更多功能，而是把这些能力做干净：

- `api-gateway`：入口配额、tenant quota 文件 / URL snapshot、URL bearer token / HTTPS guard、applied quota snapshot stale 观测、OTel server/client trace、legacy/facade traffic metrics 和 legacy opt-in 迁移观察；
- `contacts-service`：first-stage OTel gRPC server span，作为 9 服务 first-stage trace runtime wiring 的服务侧样例；
- `identity-service`：first-stage OTel gRPC server span，覆盖登录 / Refresh / challenge / MFA 管理服务侧请求；
- `message-service`：first-stage OTel gRPC server span，开始覆盖核心写链路服务侧请求；
- `conversation-service`：first-stage OTel gRPC server span，覆盖会话成员事实服务侧请求；
- `delivery-service`：first-stage OTel gRPC server span，覆盖 durable inbox / ACK 服务侧请求；
- `receipt-service`：first-stage OTel gRPC server span，覆盖已读 / 会话列表服务侧请求；
- `policy-service`：first-stage OTel gRPC server span，覆盖权限决策服务侧请求；
- `deploy/local`：本地 OTel collector debug 入口和 policy OTLP smoke 脚本，覆盖 OTLP gRPC / HTTP 接收、本地 debug exporter 与一条真实 gRPC span 到达验证；
- `identity-service`：身份安全、通知投递、key / issuer 治理继续 hardening；
- `message / conversation / delivery / push / receipt / contacts / policy`：继续清 repair、观测、故障语义和容量边界。

## 5. 第五阶段：搜索与群组记忆底座

目标：

```text
以 search-service v0.1 作为第一步，
为聊天记录搜索、群组 memory、retrieval 和后续 RAG / summary / Agent / skill-registry / mcp-gateway / action-executor / Python AI Worker 建立可控的检索事实层。
```

进入这一阶段前，不要求所有生产级 HA 或生产级完整系统测试都完成，但 message / conversation / policy / delivery 的事实边界必须足够稳定，尤其是编辑、撤回、删除、成员窗口和权限策略。

`search-service` v0.1 应该只做后端：

- 消费 message / member / revoke / delete 事件；
- 维护 search index projection；
- 强制 tenant / conversation / member visibility 过滤；
- 支持撤回 / 删除 tombstone；
- 提供 `SearchMessages` 最小查询和 EvidencePack 前置字段；
- 不让 RAG 或 Agent 直接读业务库。

`memory-service` / memory projection 第一阶段应该关注：

- 消费 message / conversation / contacts / receipt / policy 事件；
- 抽取 StructuredMemoryEvent；
- 维护 group memory 的 actor / group / topic / time / status / supersedes 语义；
- 记录 source refs、speaker / audience scope、valid_from / valid_to、confidence 和 review state；
- 不把群事实直接升级成个人长期偏好；
- 不替代 search-service 或 policy-service。

2025/2026 memory benchmark 和论文暴露的主要风险必须进入第一版测试：多人归因错误、旧事实覆盖失败、群组共识和个人偏好混淆、退群后可见性泄漏、无证据 memory active、无证据回答和需要拒答时未拒答。

## 6. 第六阶段：AI 应用后端

目标：

```text
在 search-service、group memory 和 retrieval-gateway 稳定后，
再开发 RAG、summary、agent 等智能化后端能力。
```

这一阶段更合理的顺序：

1. `retrieval-gateway` 和 EvidencePack
2. `rag-service`
3. `summary-service`
4. `agent-service` read-only / proposal-only
5. `skill-registry`
6. `mcp-gateway/tool-gateway`
7. `action-executor` + proposal / approval / audit
8. Python AI Worker foundation + external LLM adapter boundary

必须遵守的边界：

- AI 不直接读业务库；
- 检索必须带权限过滤；
- 撤回 / 删除 / 成员可见窗口必须影响搜索和 RAG；
- AI 输出必须带 source message id、conversation seq 和 evidence pack；
- Agent 写动作必须可审计、可审批、可回放。
- `skill-registry`、`mcp-gateway/tool-gateway` 和 `action-executor` 只能封装已通过权限、证据和审计约束的后端能力。
- Python AI Worker 只返回模型 / 算法 / eval 候选结果；Go 服务继续负责权限、审批、审计、outbox 和持久化。

## 7. 第七阶段：其它产品后端服务

目标：

```text
在核心后端和 AI 主线稳定后，
按真实边界继续拆 media、notification、audit、admin、config 等服务。
```

新增服务不写死，必须满足至少一个条件：

- 有独立数据模型；
- 有独立伸缩需求；
- 有独立故障边界；
- 能明显降低现有服务复杂度。

否则优先留在原服务里。

候选服务包括：

- `media-service`
- `notification-service`
- `audit-service`
- `admin-service`
- `control-plane-service`
- `presence-service`
- `model-gateway`
- `workflow-service`
- `knowledge-ingestion-service`
- `vector-index-service`

## 8. 暂不纳入当前面试主线：客户端

```text
Web / App / 桌面端是后续产品化展示层，
不是当前后端开发和面试主线。
```

进入客户端主线前，后端至少要做到：

- 主链路稳定；
- 鉴权边界稳定；
- 投递 / ACK / notify 契约稳定；
- search / RAG 后端边界清楚；
- 不再频繁改核心 API 语义。

## 9. 阶段切换规则

什么时候可以切下一阶段：

1. 当前阶段主目标已经有真实证据，不只是代码存在。
2. 当前阶段的关键 P0/P1 已清掉。
3. P2 hardening 不要求全部为零，但不能继续失控积压。
4. 文档、runbook、service brief 能准确描述当前事实。
5. 对 `search-service` v0.1 这类 AI 后端转进切片，生产级完整系统测试和生产级 HA 不作为硬门槛；本地检查、最小 smoke、权限过滤和证据边界必须闭环。

什么时候不能切下一阶段：

- 只是写了代码，还没 smoke
- 只是 happy path 通了，故障恢复没证据
- 当前服务还在频繁返工，契约不稳定
- 文件复杂度已经明显失控，却还在继续堆功能

## 10. 当前实际顺序

按目前项目状态，最合理的顺序已经调整为：

```text
保留 9 个 IM 服务作为可运行底座
-> 在已落 search / memory / retrieval / RAG / summary / Agent / tool / eval 链路上
   扩展 collaborative-memory 算法/eval
-> 生产级 HA、生产级完整系统测试和客户端产品化继续后置
-> 后续再按需要补 media / notification / audit / admin / 客户端
```

这条顺序的关键点只有一个：

```text
先用低敏 eval 把 AI 会依赖的数据、权限、证据和审计边界证明清楚，再迭代算法；短期不等待生产级完整系统测试全部完成。
```
