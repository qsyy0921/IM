# NexusIM Current Goal

本文件维护 Codex 每轮要执行的具体项目目标。目标框只放 `prompt.md` 里的短 Prompt，不把本页长目标复制进去。

## 短 Goal Prompt

```text
持续推进 E:\development\IM 的 NexusIM 项目。

当前主线必须放在第一位：面试导向的后端 + 分布式 + AI 大模型应用底座；默认围绕 AI/RAG/Agent 主链路推进，不要漂回无限生产化 hardening。

当前 active slice：完成 RAG adapter smoke 后，设计 LLM provider 边界和 citation verifier，再进入 `summary-service`。

已完成基线：9 个 IM 后端服务主链路、search-service projection smoke、memory-service group memory / StructuredMemoryEvent projection smoke、retrieval-gateway search + memory -> EvidencePack smoke、EvidencePack field hardening first pass、AI eval harness first pass、rag-service first read-only answer path 和真实 RAG adapter smoke。

当前开发规则：1. 只做阻塞 AI 底座的 9-service closeout：mutation/tombstone、visibility window、contacts privacy、policy/audit/security 边界；2. EvidencePack 必须保持 source refs、temporal version、visibility / policy boundary，不直接读 message/conversation/private tables；3. 后续依次推进 summary-service -> agent-service -> skill-registry -> mcp-gateway/tool-gateway -> action-executor -> ai-eval execution adapters，且 RAG / summary / Agent 只能消费 EvidencePack。不要把继续开发理解成无限生产级长压、完整 HA、sizing 或 provider-grade 运维；这些进入 hardening backlog，除非用户明确点名。本轮只做能推进这条主线的工作。每轮先运行 git status --short --branch --untracked-files=all，读取 prompt.md 和 agent.md，再按需读取 current-brief / remaining-goals / 相关 service brief；可用多个 sub-agent 做互不重叠任务；不全文扫长历史文档，不回滚用户已有修改。新发现的待办写入 docs/runbook/remaining-goals.md。
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
-> LLM provider 边界 / citation verifier
-> summary-service
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
5. `rag-service`：first-stage 只读 answer path、`loadtest/rag`、RAG eval execution adapter 和真实 RAG adapter smoke 已落；只消费 EvidencePack，返回 citations 和 `generated_by_llm=false`，无 evidence 必须拒答。
6. AI eval harness：first-stage 低敏 case schema / validator 已落；RAG adapter 已落；后续补 Agent execution adapter。
7. `summary-service` / Agent / `skill-registry` / `mcp-gateway` / `action-executor`：只消费受控 EvidencePack 和 tool policy，不直接读业务库。
8. 生产级观测、HA、长压和 sizing：继续作为 hardening backlog 推进，但不阻塞当前 AI 底座路线启动。

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
