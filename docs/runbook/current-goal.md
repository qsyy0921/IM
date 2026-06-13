# NexusIM Current Goal

## 短 Goal Prompt

持续推进 `E:\development\IM` 的 NexusIM 项目。

每轮开始：
1. 运行 `git status --short --branch`。
2. 读取 `docs/runbook/current-brief.md`。
3. 需要找文档时先读 `docs/runbook/README.md`。
4. 需要服务状态时先读 `docs/runbook/service-briefs/README.md` 索引，再只读相关服务文件。
5. 不要全文读取长历史文档；只用 `Select-String` / `rg` 按关键词读取相关片段。
6. 不回滚用户已有修改。

工作原则：
1. 优先开发完整系统能力，不长期停留在单点压测或基础设施实验。
2. 控制耦合和复杂度：不跨服务读内部表，不引入网状同步 RPC，不为了短期功能抽公共包。
3. 单个切片保持小闭环：契约 / migration / 本地事务 / consumer 或 relay / smoke 分阶段推进。
4. 只在有价值时使用 sub-agent，专项结束后及时关闭。
5. 压测原始数据放 `H:\NexusIM\loadtest-results`；E 盘仓库只放报告和文档。
6. 及时拆文件：生产手写文件接近 2500 行、测试/runner 接近 3000 行时优先同 package 拆分，不继续堆大文件。
7. 每轮结束运行 `.\tools\check-local.ps1`，避免入口文档重新变长、代码文件失控，并捕获基础 whitespace / PowerShell 语法问题。

## 当前主线

先继续第三层产品能力和身份安全 hardening，把系统做成可面试展示的“本地/双机可运行分布式 IM 后端”。

当前优先级：
1. identity-service TOTP / recovery-code proof 的最终事务内 lock recheck 已完成，继续按小切片推进安全 hardening。
2. 保持 api-gateway facade、verified metadata、mTLS、push / delivery / policy / contacts / receipt 已有链路稳定。
3. 后续再补更完整的产品能力：会话列表完善、群管理、联系人关系、回执扩展、客户端 UI。
4. Kafka HA、PostgreSQL failover、Redis quorum / 网络分区属于生产化后续项，不阻塞当前功能推进。

## 分层路线

第一层：最小 IM 主链路
发消息、会话、投递、在线通知、ACK。已完成最小闭环。

第二层：分布式与可靠性
outbox、Kafka、durable inbox、Redis route、多实例、Win/Mac Docker smoke、基础观测。已有小规模 smoke 证据，不是生产 HA。

第三层：完整 IM 产品能力
已读/送达回执、撤回/编辑/删除、会话列表、未读数、联系人、群管理、真实鉴权、api-gateway。当前主线。

第四层：智能化扩展
RAG、Agent、聊天记录搜索、智能总结、群聊问答、客服机器人、推荐、风控辅助。等权限和消息事实更稳定后再做。

## 文档路由

- 当前入口：`docs/runbook/current-brief.md`
- 文档总入口：`docs/README.md`
- runbook 路由：`docs/runbook/README.md`
- 服务状态索引：`docs/runbook/service-briefs/README.md`
- 单服务短状态：`docs/runbook/service-briefs/<service>.md`
- 历史长目标：`docs/runbook/archive/current-goal-20260614-long.md`
- 历史长 brief：`docs/runbook/archive/current-brief-20260614-long.md`
- 服务设计：`docs/sdd/<service>.md`
- 压测 / smoke 证据：`docs/runbook/loadtest/<service>/`

默认不要读 archive 全文。只在查历史证据时按关键词读取相关段落。

## 硬边界

- 项目名统一为 `NexusIM`。
- 每个微服务独立六层 DDD：`api / app / domain / infrastructure / types / trigger`。
- Kafka 事件只能通过 outbox relay 发布，业务事务不能直接 publish Kafka。
- push-gateway 只做在线唤醒和 ACK 转发；可靠事实在 delivery-service。
- 服务间同步调用只用于当前请求必须依赖的权限 / 上下文；状态传播优先走 Kafka 事实事件和本服务 projection。
- 新能力优先复用已有事实流、outbox、projection、read model 和端口。
