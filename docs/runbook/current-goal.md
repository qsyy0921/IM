# NexusIM Current Goal

本文件维护 Codex 每轮要执行的具体项目目标。目标框只放 `prompt.md` 里的短 Prompt，不把本页长目标复制进去。

## 短 Goal Prompt

```text
持续推进 E:\development\IM 的 NexusIM 项目。

【当前主线 / 不可偏移】本阶段不是继续堆九个 IM 服务的长期 P2 hardening，也不是跑完整生产级 HA / 长压 / sizing；当前默认任务是：在已可运行的后端 + 分布式 IM 底座上，继续建设 AI 大模型应用底座。

默认开发方向：群组 memory、跨群 / 跨时间 EvidencePack、RAG、summary、multi-agent、skill registry、MCP/tool gateway、action-executor、proposal / approval / audit 和 ai-eval。每轮“继续开发”都优先推进这条链，除非用户明确点名别的任务，或发现阻塞该主线的 P0/P1。

九个既有 IM 服务已经作为可运行基础；只处理阻塞 AI 主线的 P0/P1 或用户点名任务。不要把默认任务切回长期 P2 hardening、生产级长压、完整 HA、sizing 或 provider-grade 运维。

每轮开始：
1. 执行 git status --short --branch --untracked-files=all。
2. 读取 prompt.md 和 agent.md。
3. 读取 docs/runbook/current-goal.md 的当前 active slice；再按需读取 current-brief、remaining-goals、相关 service brief 或 SDD；不要全文扫长历史文档。

当前 active slice：skill-registry first catalog path、mcp-gateway first prepare path 和 action-executor first execution audit path 已落；agent-service proposal 前已改为调用 `mcp-gateway.PrepareToolCall`，返回 `skill_id` / `prepared_audit_id`，但仍不执行外部工具；mcp-gateway 只做 skill contract 校验、policy precheck 和低敏 audit；action-executor 第一版只记录 approved execution boundary，`executed=false`，不连接外部 MCP/tool provider。下一步默认跑新的 `retrieval-gateway -> agent-service -> mcp-gateway` adapter smoke，并推进 proposal store / approval / real tool adapter 串接；如果该 smoke 已完成，则按本页优先级继续下一项 AI 主线。RAG / summary / Agent 只能消费 EvidencePack，不直接读 message / conversation / private tables；真实写动作必须继续走 policy、proposal / approval / executor / audit。

可用多个 sub-agent 做互不重叠任务；主 agent 负责集成、检查和文档同步。不回滚用户已有修改。新发现的待办写入 docs/runbook/remaining-goals.md。
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
-> retrieval-gateway / EvidencePack 第一版边界（真实 smoke 已通过）
-> rag-service first read-only answer path（已落地）
-> loadtest/rag + RAG eval execution adapter（已落地）
-> 真实 RAG adapter smoke（已通过）
-> provider boundary / citation verifier first pass（已落地）
-> summary-service first read-only summary path（已落地）
-> summary-service adapter smoke（已通过）
-> agent-service first proposal-only path（已落）
-> agent-service adapter smoke（已通过）
-> skill-registry first catalog path（已落）
-> mcp-gateway first prepare path（已落）
-> action-executor first execution audit path（已落）
-> agent-service -> mcp-gateway prepare adapter smoke
-> proposal store / approval / real tool adapter follow-up
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
4. `retrieval-gateway`：第一版真实 smoke 已通过，统一 EvidencePack、权限过滤、引用来源和 temporal version；第一版只通过 search / memory 公开 gRPC 契约聚合，不直接读业务库；policy-service retrieval precheck 已有 first-stage 可选接入；EvidencePack 字段 hardening first pass 已补 source coverage、rerank score、dedupe reason。
5. `rag-service`：first-stage 只读 answer path、`loadtest/rag`、RAG eval execution adapter、真实 RAG adapter smoke、provider boundary 和 citation verifier first pass 已落；只消费 EvidencePack，返回 citations 和 `generated_by_llm=false`，无 evidence 必须拒答。
6. AI eval harness：first-stage 低敏 case schema / validator 已落；RAG adapter 已落；后续补 Agent execution adapter。
7. `summary-service`：first-stage 只读 summary path 和真实 adapter smoke 已落；只消费受控 EvidencePack，返回 citations 和 `generated_by_llm=false`，无 evidence 必须拒绝摘要。
8. Agent / `skill-registry` / `mcp-gateway` / `action-executor`：只消费受控 EvidencePack 和 tool policy，不直接读业务库；`agent-service` first proposal-only path 和真实 adapter smoke 已落，且 proposal 前已改为调用 `mcp-gateway.PrepareToolCall`；`skill-registry` first catalog path 已落；`mcp-gateway` first prepare path 已落；`action-executor` first execution audit path 已落，后续补新的 Agent -> mcp-gateway adapter smoke 并推进 approval / real tool adapter 串接。
9. 生产级观测、HA、长压和 sizing：继续作为 hardening backlog 推进，但不阻塞当前 AI 底座路线启动。

新发现的待完成工作必须写入 `docs/runbook/remaining-goals.md`；已完成的工作从该文档移除，并同步到对应 service brief / progress 文档。

## 分层路线

| 层级 | 内容 | 当前状态 |
| --- | --- | --- |
| 第一层 | 最小 IM 主链路：发消息、会话、投递、在线通知、ACK | 已完成最小闭环 |
| 第二层 | 分布式与可靠性：outbox、Kafka、durable inbox、Redis route、多实例、故障 smoke | 已有本地 / 双机 smoke，不等于生产 HA |
| 第三层 | 完整 IM 后端产品能力：回执、撤回/编辑/删除、会话列表、联系人、群管理、鉴权、api-gateway | 做必要收口，不以生产级测试深水区阻塞 AI 底座 |
| 第四层 | AI 大模型应用底座：search、memory、retrieval、RAG、summary、Agent、skill registry、MCP gateway、action executor | `search-service v0.1` 是第一步 |

## 文档路由

具体文档路由看 `docs/runbook/README.md`。默认不要读 archive 全文，只在查历史证据时按关键词读取相关段落；常用入口是 `development-progress.md`、`remaining-goals.md`、`service-briefs/<service>.md` 和 `docs/interview/project-progress.md`。
