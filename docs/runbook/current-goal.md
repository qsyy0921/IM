# NexusIM Current Goal

## 短 Goal Prompt

可复制到 Codex 目标框：

```text
持续推进 E:\development\IM 的 NexusIM 项目。每轮先运行 git status --short --branch --untracked-files=all，然后读取仓库根目录 prompt.md；只按 prompt.md 的路由继续读取必要短文档并执行。不要全文读取长历史文档；不要回滚用户已有修改。
```

维护真实 prompt 内容时只改仓库根目录 `prompt.md`，不要把完整 prompt 复制回本文件。

## 当前主线

当前面试主线是后端、分布式可靠性和 AI 应用后端。先把 9 个已有后端服务收干净，再进入 `search-service`，然后在搜索和权限边界之上推进 `rag-service` / `summary-service` / `agent-service`。Web / App / 桌面端属于后续产品化展示层，暂不作为当前主线。

当前优先级：
1. 先治理已有 9 个微服务，新增服务后置；`search-service` 只保留 SDD draft，不进入 proto / migration / skeleton。
2. 保持入口文档短；总体进度看 `docs/runbook/development-progress.md`，单服务状态看 `docs/runbook/service-briefs/<service>.md`。
3. 继续清各服务 P2 hardening：生产观测、容量验证、故障演练、repair workflow、权限模型深化和代码复杂度治理。
4. 当前 9 个服务已补 first-stage 本地 Prometheus / Grafana 观测原型；它只用于本地开发和面试展示，不等于生产 SLO / Alertmanager / 统一观测平台。
5. 保持 api-gateway、identity、message、conversation、delivery、push、receipt、contacts、policy 已有链路稳定。
6. 本地 PostgreSQL / Kafka / Redis Sentinel 故障 smoke 已补；完整 Redis 网络分区、服务发现、统一观测和部署编排仍属于生产化后续项。

演进原则：当前 9 个服务够支撑 IM 后端主链路；后续服务和中间件都不写死。只有当能力有独立数据模型、独立伸缩需求、独立故障边界，或会明显降低复杂度时才新增服务；替换中间件必须说明兼容、迁移、回滚和压测证据，并通过 ADR。

## 分层路线

第一层：最小 IM 主链路
发消息、会话、投递、在线通知、ACK。已完成最小闭环。

第二层：分布式与可靠性
outbox、Kafka、durable inbox、Redis route、多实例、Win/Mac Docker smoke、基础观测，以及本地 PostgreSQL `repmgr + pgpool` failover smoke 和本地 Kafka KRaft 三 broker failover smoke。已有小规模 smoke 证据，不是生产 HA。

第三层：完整 IM 后端产品能力
已读/送达回执、撤回/编辑/删除、会话列表、未读数、联系人、群管理、真实鉴权、api-gateway。当前主线是把这些已有后端服务收干净，不急着补客户端。

第四层：搜索与智能化后端
先做 `search-service`，再做 RAG、Agent、聊天记录搜索、智能总结、群聊问答、客服机器人、推荐、风控辅助。AI 只能通过权限过滤后的检索层访问消息事实。

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
