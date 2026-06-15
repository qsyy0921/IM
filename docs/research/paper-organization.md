# NexusIM Paper Organization

本文件维护 NexusIM 相关论文的分类规则。后续新增论文时，先放入 Zotero 的 `NexusIM` collection，再按主题放入一个或多个子 collection。

## 存放位置

- Zotero collection：`NexusIM`
- 本地 PDF 原始批次：`H:\NexusIM\papers\im-ai-agent-rag-2026`
- 本地 PDF 分类副本：`H:\NexusIM\papers\nexusim`
- Zotero PDF 附件：复制到 Zotero storage，作为每篇论文的 `Full Text PDF` 子附件。
- 仓库内只放分类规则、引用说明和读论文后形成的设计结论。

## Zotero Collection

当前 Zotero 结构：

- `NexusIM`
- `NexusIM / 00 Inbox - To Classify`
- `NexusIM / 00 Source Set - AI-RAG-Agent 2026 Top Conference`
- `NexusIM / 01 Memory and Long-Horizon Context`
- `NexusIM / 02 Retrieval Search and Evidence`
- `NexusIM / 03 Multi-Agent Orchestration and Workflow`
- `NexusIM / 04 Safety Security Audit and Governance`
- `NexusIM / 05 Evaluation Benchmarks and Datasets`
- `NexusIM / 06 Personalization Proactivity and Mobile Agents`

`00 Source Set` 用来保留论文来源批次；主题 collection 用来服务架构设计和后续阅读。

## Tags

每篇 NexusIM 论文必须至少包含：

- `NexusIM`
- `AI-RAG-Agent`

按主题追加：

- `NexusIM:memory`
- `NexusIM:retrieval-search`
- `NexusIM:multi-agent`
- `NexusIM:safety-audit`
- `NexusIM:evaluation`
- `NexusIM:personalization`

## 分类口径

### Memory and Long-Horizon Context

用于支撑 `memory-service`、长期群聊记忆、可见窗口、时间版本和用户画像聚合。

放入条件：

- 长期记忆、增量记忆、遗忘、profile、long-horizon dialogue。
- 讨论 memory state、memory update、memory retrieval 或 conversation memory benchmark。

### Retrieval Search and Evidence

用于支撑 `search-service`、`retrieval-gateway`、EvidencePack、RAG 可追溯证据。

放入条件：

- RAG、deep search、multi-hop search、graph retrieval、evidence tracking。
- 讨论检索失败归因、query decomposition、rerank、source grounding。

### Multi-Agent Orchestration and Workflow

用于支撑 `agent-service`、multi-agent planner/router、tool workflow、action execution。

放入条件：

- 多 Agent 协作、agent routing、planner/executor/summarizer 分工。
- DAG/workflow synthesis、tool execution trace、agent-to-agent protocol。

### Safety Security Audit and Governance

用于支撑 Agent 权限、审批、审计、风控和协议安全。

放入条件：

- Agent 安全、A2A 协议安全、failure attribution、self-evolving agent risk。
- 讨论 action audit、approval、tool policy、attack surface 或 rollback。

### Evaluation Benchmarks and Datasets

用于支撑 `ai-eval`、回归评测、offline benchmark 和面试演示。

放入条件：

- benchmark、dataset、factorized metrics、agent trace dataset、eval harness。
- 论文主要贡献是评测框架或训练数据协议。

### Personalization Proactivity and Mobile Agents

用于支撑个性化、主动提醒、移动端 Agent 和用户偏好建模。

放入条件：

- proactive agent、personalized assistant、mobile GUI agent、user preference。
- 讨论长期行为聚合、个性化执行轨迹或主动任务建议。

## 新论文处理流程

1. 下载 PDF 到 `H:\NexusIM\papers\<topic-or-batch>`，不要放入 E 盘仓库。
2. 导入 Zotero，放入 `NexusIM / 00 Inbox - To Classify`。
3. 添加基础 tags：`NexusIM`、`AI-RAG-Agent`。
4. 按本文规则加入一个或多个主题 collection，并添加对应 `NexusIM:*` tag。
5. 在 `H:\NexusIM\papers\nexusim` 下对应主题目录复制 PDF；论文不大，不使用硬链接作为默认策略。
6. 将 PDF 复制到 Zotero storage，并作为论文条目的 `Full Text PDF` 子附件。
7. 如果论文来自一个明确批次，另建或复用 `00 Source Set - ...` collection。
8. 更新本地批次目录的 `manifest.json` 或 README，记录标题、来源、URL、PDF 文件名和与 NexusIM 的关系。
9. 读完后只把设计结论写入架构/SDD，不把长论文笔记塞进入口文档。

## 当前 2026 Top Conference 分类

| 论文 | 主题分类 |
| --- | --- |
| Evaluating Memory in LLM Agents via Incremental Multi-Turn Interactions | Memory, Evaluation |
| MemAgent: Reshaping Long-Context LLM with Multi-Conv RL-based Memory Agent | Memory |
| GraphPlanner: Graph Memory-Augmented Agentic Routing for Multi-Agent LLMs | Multi-Agent |
| AgenTracer: Who Is Inducing Failure in the LLM Agentic Systems? | Safety, Multi-Agent, Evaluation |
| A2ASecBench: A Protocol-Aware Security Benchmark for Agent-to-Agent Multi-Agent Systems | Safety, Multi-Agent, Evaluation |
| Agent Data Protocol: Unifying Datasets for Diverse, Effective Fine-tuning of LLM Agents | Multi-Agent, Evaluation |
| Your Agent May Misevolve: Emergent Risks in Self-evolving LLM Agents | Safety, Memory |
| FlowSearcher: Synthesizing Memory-Guided Agentic Workflows for Web Information Seeking | Retrieval, Multi-Agent |
| Fathom-DeepResearch: Unlocking Long Horizon Information Retrieval and Synthesis for SLMs | Retrieval, Evaluation |
| Demystifying Deep Search: A Holistic Evaluation with Hint-free Multi-Hop Questions and Factorised Metrics | Retrieval, Evaluation |
| BrowseNet: Graph-Based Associative Memory for Contextual Information Retrieval | Retrieval, Memory |
| Flash-Searcher: Fast and Effective Web Agents via DAG-Based Parallel Execution | Multi-Agent, Retrieval |
| AMemGym: Interactive Memory Benchmarking for Assistants in Long-Horizon Conversations | Memory, Evaluation, Personalization |
| WideSearch: Benchmarking Agentic Broad Info-Seeking | Retrieval, Evaluation |
| FingerTip 20K: A Benchmark for Proactive and Personalized Mobile LLM Agents | Personalization, Evaluation |
