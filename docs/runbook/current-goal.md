# NexusIM Current Goal

本文件维护 Codex 每轮要执行的具体项目目标。目标框只放 `prompt.md` 里的短 Prompt，不把本页长目标复制进去。

## 短 Goal Prompt

```text
持续推进 E:\development\IM 的 NexusIM 项目。每轮先运行 git status --short --branch --untracked-files=all，然后读取仓库根目录 prompt.md 和 agent.md，并按文档指向的项目目标继续执行；不要把具体目标写在目标框里，不全文读取长历史文档，不回滚用户已有修改。
```

## 当前具体执行目标

持续推进 NexusIM 后端项目。当前主线是补完整 IM 后端产品语义，并为 search / group memory / RAG / Agent 建立数据、权限和审计底座；不要为了演示直接做孤立 LLM demo。

当前 active slice：

```text
search-service v0.1
-> SDD 收敛
-> proto / migration / 六层 skeleton
-> timeline projection: persisted / edited / revoked / deleted + member boundary
-> SearchMessages: tenant / conversation / keyword / visibility / tombstone
-> focused tests + search smoke
```

每个有意义切片都要尽量闭环：

```text
选定一个明确缺口
-> 读对应短文档和必要源码
-> 实现代码
-> 补测试
-> 更新对应进度文档
-> 运行 focused checks + .\tools\check-local.ps1
-> 提交 focused commit
```

当前优先级：

1. IM 语义补齐：消息编辑 / 撤回 / 删除、群管理、成员可见窗口、回执、联系人、策略决策。
2. AI 数据底座：search projection、tombstone、visibility filter、EvidencePack、group memory 结构化事件。
3. 安全边界：public listener、mock auth、trusted metadata、TLS / mTLS、敏感字段外泄。
4. Repair / DLQ / audit：operator 安全、批量 repair、错误脱敏、审计证据。
5. 观测和故障 smoke：本地 / 双机 / Docker 可验证，不能夸大为生产 SLO。
6. 容量和复杂度治理：长时间容量曲线、生产 sizing、及时拆分大文件。

新发现的待完成工作必须写入 `docs/runbook/remaining-goals.md`；已完成的工作从该文档移除，并同步到对应 service brief / progress 文档。

## 长期目标

把 NexusIM 做成可讲清楚工程边界、分布式可靠性和 AI 应用后端扩展路线的 IM 后端项目。

当前阶段：

```text
补完整 IM 后端产品语义
-> 启动 search-service / group memory / retrieval foundation
-> 再做 RAG / summary / agent 后端
```

Web / App / 桌面端是后续产品化展示层，不是当前后端主线。

## 分层路线

| 层级 | 内容 | 当前状态 |
| --- | --- | --- |
| 第一层 | 最小 IM 主链路：发消息、会话、投递、在线通知、ACK | 已完成最小闭环 |
| 第二层 | 分布式与可靠性：outbox、Kafka、durable inbox、Redis route、多实例、故障 smoke | 已有本地 / 双机 smoke，不等于生产 HA |
| 第三层 | 完整 IM 后端产品能力：回执、撤回/编辑/删除、会话列表、联系人、群管理、鉴权、api-gateway | 正在补齐 AI 所需语义 |
| 第四层 | 搜索与智能化后端：search、group memory、retrieval、RAG、summary、agent | search / memory foundation 进入规划优先级 |

## 文档路由

具体文档路由看 `docs/runbook/README.md`。默认不要读 archive 全文，只在查历史证据时按关键词读取相关段落。

- 当前进度总览：`docs/runbook/development-progress.md`
- 当前未完成工作：`docs/runbook/remaining-goals.md`
- 单服务状态：`docs/runbook/service-briefs/<service>.md`
- 面试讲述进度：`docs/interview/project-progress.md`
