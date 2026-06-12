# NexusIM Current Goal

本文是 NexusIM 的短目标索引。每轮默认只读 `docs/runbook/current-brief.md`；只有需要长期原则、路线图、评审规则或历史索引时，才读取本文。历史流水已归档到 `docs/runbook/history/current-goal-archive-20260611.md`，不要每轮全文读取归档。

## 0. 可复制短 Goal Prompt

```text
持续推进 E:\development\IM 的 NexusIM 项目。

每轮开始：
1. 执行 git status --short --branch。
2. 读取 docs/runbook/current-brief.md。
3. 不要每轮全文读取 current-goal.md、SDD、压测报告或历史文档。
4. 需要哪类信息，就按关键词读取对应文档的相关片段：长期目标查 current-goal，服务设计查 docs/sdd/<service>.md，压测证据查 docs/runbook/loadtest/<service>/，实现细节用 rg 定位代码。
5. 按 brief 和按需读取到的当前目标、硬边界、下一步优先级继续工作。
6. 不回滚用户已有修改。

工作原则：
1. 优先把系统链路做完整，不把主要时间消耗在重型压测矩阵上。
2. 除非用户明确要求，不再把流量诊断、代理用量归因、外网消耗排查列为当前任务；日常开发只保留“少下载、用已有镜像/依赖、服务间走本地有线”的约束。
3. 每个微服务独立使用六层 DDD：api / app / domain / infrastructure / types / trigger。
4. Kafka 事件只能通过 outbox relay 发布，业务事务不能直接 publish Kafka。
5. 优先降低微服务耦合、控制代码复杂度：不跨服务读取内部表，不引入网状依赖，不为短期功能增加不必要同步 RPC、公共包或抽象层。
6. 单个切片保持小闭环：契约 / migration / 本地事务 / consumer 或 relay / smoke 分阶段推进，不一次性横跨多个产品能力。
7. 开发过程中主动使用可用 sub-agent 做设计、实现、测试、文档或风险复核；任务完成后及时关闭 sub-agent。
8. 有意义的切片完成后运行必要检查，更新 current-brief.md；阶段状态变化时同步 current-goal.md、对应 SDD 和 runbook/loadtest 报告。
9. 批量提交和推送 GitHub，不为低风险小改动频繁推送。
```

## 1. 当前目标

持续推进 `E:\development\IM` 的 NexusIM 项目落地。

当前系统已完成本地多进程 + Win/Mac 双机 Docker 的最小分布式 IM 后端证据。主线不再停留在重型基础设施矩阵，优先推进第三层 IM 产品能力和必要可靠性补强。

当前具体下一步以 `docs/runbook/current-brief.md` 的“当前优先级”为准。

## 2. 四层路线图

### 第一层：最小可运行 IM 主链路

目标：证明 IM 后端主链路真实可跑。

范围：
- 发消息。
- 会话上下文。
- PostgreSQL 本地事务。
- outbox。
- Kafka timeline。
- durable inbox。
- PullInbox。
- AckDelivery。
- WebSocket online notify。

状态：已完成最小真实链路。

### 第二层：分布式与可靠性

目标：证明不是单机 demo，而是可解释的最小分布式后端。

范围：
- outbox relay。
- Kafka 事件流。
- delivery read model。
- Redis route。
- 多实例 push-gateway。
- Win/Mac 双机 smoke。
- Redis Sentinel discovery / failover smoke。
- 基础观测和 smoke 报告。

状态：面试讲解证据已够；Kafka HA、PostgreSQL failover、Redis quorum / 网络分区仍是生产化后续项，不阻塞当前产品能力主线。

### 第三层：完整 IM 产品能力

目标：补齐真正 IM 产品会被问到的核心功能。

候选能力：
- 送达 / 已读回执。
- 会话列表 / 未读数 / 置顶 / 归档 / 静音。
- 编辑 / 撤回 / 删除。
- 群成员列表、owner transfer、群管理规则。
- 联系人 / 好友关系 / 申请列表 / 取消申请 / 拉黑 / 删除 / 备注。
- 真实鉴权、设备绑定、session revoke：当前已完成短期 gateway token、push-gateway 本地验签、legacy HMAC 与标准三段 JWT HS256 gateway token 兼容、identity debug JWKS、device/session revoke event consumer、device revoke deny-list smoke、device revoke active close smoke 和 session revoke active close smoke；完整 Login / refresh token / 非对称 JWK key ring / 多 issuer 仍是后续项。
- 客户端 UI 或最小端到端演示。

推进原则：优先选择能复用已有事实流、read model 和 outbox 的小闭环；不要为了产品功能让服务间耦合变高。

### 第四层：智能化扩展

目标：在核心 IM 事实、权限和消息变更语义稳定后，做智能化能力。

候选能力：
- 聊天记录搜索。
- RAG 问答。
- 智能总结。
- 群聊问答 Agent。
- 客服机器人。
- 推荐 / 风控辅助。

推进原则：第四层必须遵守成员可见窗口、撤回 / 编辑 / 删除语义、ACL 过滤、审计和失败降级。不得绕过 IM 事实源。

## 3. 硬边界

- 项目统一命名为 `NexusIM`。
- 每个微服务独立六层 DDD：`api / app / domain / infrastructure / types / trigger`。
- 根目录 `api/` 只放全局接口契约；`services/<service>/internal/api/` 才是服务内部接口适配实现。
- Kafka 事件只能通过 outbox relay 发布，业务事务不能直接 publish Kafka。
- push-gateway 只做在线唤醒和 ACK 转发；可靠事实在 `delivery-service user_inbox`。
- 不跨服务读取内部表，不引入网状依赖，不为短期功能增加不必要同步 RPC、公共包或抽象层。
- 单个切片如果需要同时改多个服务、多条 Kafka 事件、多张核心表或多种用户语义，必须拆小。
- 公共包、共享 helper、跨服务接口和统一框架必须有两个以上真实调用方或明确降低复杂度，否则保持在单服务内。
- 服务间同步调用只用于查询当前请求必须依赖的权限 / 上下文；状态传播优先走 Kafka 事实事件和本服务 projection。
- 压测原始数据放到 `H:\NexusIM\loadtest-results`；E 盘仓库只放报告和文档。
- Win/Mac 服务间通信优先使用有线 `172.31.50.*`，不要把服务间流量走外网或代理。
- 除非用户明确要求，不再把流量诊断、代理用量归因、外网消耗排查列为当前任务。
- 不回滚用户已有修改。

## 4. 按需读取索引

### 当前入口

- `docs/runbook/current-brief.md`：每轮默认唯一必读入口。

### 长历史

- `docs/runbook/history/current-goal-archive-20260611.md`：旧版完整历史、长事实表、逐日流水。只在需要追溯历史证据时按关键词查询。

### 架构 / 设计

- `docs/README.md`：文档入口。
- `docs/architecture/target-architecture.md`：目标架构。
- `docs/architecture/tadd.md`：技术架构决策。
- `docs/sdd/README.md`：服务设计索引。
- `docs/sdd/message-service.md`
- `docs/sdd/conversation-service.md`
- `docs/sdd/delivery-service.md`
- `docs/sdd/push-gateway.md`
- `docs/sdd/receipt-service.md`
- `docs/sdd/contacts-service.md`

### 压测 / smoke 报告

- `docs/runbook/loadtest/message-service/README.md`
- `docs/runbook/loadtest/conversation-service/README.md`
- `docs/runbook/loadtest/delivery-service/README.md`
- `docs/runbook/loadtest/push-gateway/README.md`
- `docs/runbook/loadtest/contacts-service/README.md`

### 本地 / 分布式运行

- `docs/runbook/distributed-local.md`
- `tools/local-distributed-smoke.ps1`
- `tools/sync-mac-distributed-smoke.ps1`
- `tools/check-mac-docker-desktop.ps1`

## 5. 读取规则

每轮默认：

```powershell
git status --short --branch
Get-Content docs\runbook\current-brief.md -Raw
```

需要细节时：

```powershell
Select-String -Path docs\runbook\current-goal.md -Pattern "关键词" -Context 2,4
Select-String -Path docs\runbook\history\current-goal-archive-20260611.md -Pattern "关键词" -Context 2,4
Select-String -Path docs\sdd\<service>.md -Pattern "关键词" -Context 2,4
Select-String -Path docs\runbook\loadtest\<service>\README.md -Pattern "关键词" -Context 2,4
```

实现细节优先：

```powershell
rg -n "SymbolOrKeyword" services api schemas migrations loadtest tools
```

不要为了“了解项目”全文读取 `current-goal.md`、历史归档、SDD 或压测报告。只有在做完整审计、文档重构或用户明确要求时，才允许全文读取长文档。

## 6. 评审与提交

- 公共契约、migration、事务、幂等、消息顺序、错误码、可运行链路完成时，邀请独立评审或 sub-agent 复核。
- sub-agent 任务结束后及时关闭。
- 有意义的切片完成后运行必要检查。
- 每轮结束更新 `docs/runbook/current-brief.md`。
- 阶段状态、风险或历史证据变化时，同步更新本文、对应 SDD 和 runbook/loadtest 报告。
- 批量提交和推送 GitHub，不为低风险小改动频繁推送。
