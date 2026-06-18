# NexusIM Current Goal

本文件维护 Codex 每轮要执行的具体项目目标。目标框只放 `prompt.md` 里的短 Prompt，不把本页长目标复制进去。

## 短 Goal Prompt

```text
持续推进 E:\development\IM 的 NexusIM 项目。当前唯一主线：9 个已成型后端服务只做阻塞 AI 底座的必要收口，然后转向 AI 大模型应用后端。执行顺序：search-service v0.1 projection smoke 已通过 -> memory-service foundation-active projection smoke 已通过 -> retrieval-gateway / EvidencePack -> RAG / summary-service -> agent-service -> skill-registry / mcp-gateway / action-executor。本轮只优先做能推进这条主线的工作；生产级 HA、长压、sizing、完整 provider 运维后置为 hardening backlog，除非用户明确点名。每轮先运行 git status --short --branch --untracked-files=all，读取 prompt.md 和 agent.md，再按需读取 current-brief / remaining-goals / 相关 service brief；可用多个 sub-agent 做互不重叠任务；不全文扫长历史文档，不回滚用户已有修改。
```

## 当前具体执行目标

持续推进 NexusIM 后端项目。当前主线调整为：9 个现有服务做必要收口，不再以短期生产级测试为阻塞；向 AI 大模型应用底座转进。必要收口只处理会阻塞 search / memory / retrieval / Agent 的 IM 语义、权限、tombstone、audit 和安全边界。完整生产级 HA、长时间容量、跨环境 SLO 和 provider-grade 运维仍保留在 backlog，不作为当前转进 AI 底座的短期前置条件。

当前 active slice：

```text
search-service v0.1（第一步，已完成第一轮 smoke）
-> SDD 收敛
-> proto / migration / 六层 skeleton（第一切片已落地）
-> PostgreSQL repository / SearchMessages / grpc runtime（已落地）
-> timeline decoder / consumer（已落地）
-> timeline projection smoke: persisted / edited / revoked / deleted + member boundary（clean commit f2a57516 已通过）
-> memory-service v0.1 SDD / proto / migration / implementation / clean projection smoke（已落地）
-> 当前 active：memory hardening 或 retrieval-gateway / EvidencePack
-> 不做孤立 LLM demo
```

当前可以采用 multi sub-agent 方式推进，但只用于互不重叠的服务、文档或验证范围；禁止多个 agent 同时改同一架构文档、service brief、proto 或 migration，最终由主 agent 合并验证。

每个有意义切片都要尽量闭环：

```text
选定明确缺口 -> 读对应短文档和必要源码 -> 实现代码 -> 补测试 -> 更新对应进度文档 -> 运行 focused checks + .\tools\check-local.ps1 -> 提交 focused commit
```

当前优先级：

1. 9 个现有服务必要收口：只补 search / memory / retrieval / Agent 必须依赖的消息 mutation、成员可见窗口、联系人隐私、policy decision source、tool policy precheck、audit 和安全边界。
2. `search-service v0.1`：第一切片已落 proto / migration / 六层 skeleton / PG repository / `SearchMessages` / grpc runtime / timeline decoder / consumer，并已跑通 projection smoke，作为第一步 AI 数据入口。
3. `memory-service`：SDD / proto / migration、六层 skeleton、repository first pass、projection usecase、runtime wiring、PG integration、timeline worker 单测和 clean projection smoke 已落。group memory / StructuredMemoryEvent / Memory Graph / profile aggregate 必须带 source refs、speaker / audience scope、valid_from / valid_to、supersedes、confidence 和 review state，不能把群聊事实直接升级成个人长期偏好。
4. `retrieval-gateway`：统一 EvidencePack、权限过滤、引用来源和 temporal version。
5. RAG / `summary-service` / Agent / `skill-registry` / `mcp-gateway` / `action-executor`：只消费受控 EvidencePack 和 tool policy，不直接读业务库。
6. 生产级观测、HA、长压和 sizing：继续作为 hardening backlog 推进，但不阻塞当前 AI 底座路线启动。

新发现的待完成工作必须写入 `docs/runbook/remaining-goals.md`；已完成的工作从该文档移除，并同步到对应 service brief / progress 文档。

## 长期目标

把 NexusIM 做成可讲清楚工程边界、分布式可靠性和 AI 应用后端扩展路线的 IM 后端项目。当前阶段：

```text
9 个现有服务做必要收口
-> search-service v0.1 第一实现切片已跑通 projection smoke
-> memory-service foundation-active projection smoke 已通过 / retrieval-gateway
-> RAG / summary-service / Agent
-> skill-registry / mcp-gateway / action-executor
```

Web / App / 桌面端是后续产品化展示层，不是当前后端主线。

## 分层路线

| 层级 | 内容 | 当前状态 |
| --- | --- | --- |
| 第一层 | 最小 IM 主链路：发消息、会话、投递、在线通知、ACK | 已完成最小闭环 |
| 第二层 | 分布式与可靠性：outbox、Kafka、durable inbox、Redis route、多实例、故障 smoke | 已有本地 / 双机 smoke，不等于生产 HA |
| 第三层 | 完整 IM 后端产品能力：回执、撤回/编辑/删除、会话列表、联系人、群管理、鉴权、api-gateway | 做必要收口，不以生产级测试深水区阻塞 AI 底座 |
| 第四层 | AI 大模型应用底座：search、memory、retrieval、RAG、summary、Agent、skill registry、MCP gateway、action executor | `search-service v0.1` 是第一步 |

## 文档路由

具体文档路由看 `docs/runbook/README.md`。默认不要读 archive 全文，只在查历史证据时按关键词读取相关段落；常用入口是 `development-progress.md`、`remaining-goals.md`、`service-briefs/<service>.md` 和 `docs/interview/project-progress.md`。
