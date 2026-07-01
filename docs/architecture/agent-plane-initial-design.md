# NexusIM Agent 层初步设计报告

日期：2026-07-01
状态：初步设计报告，供主集成评审；不是 ADR、SDD、proto、OpenAPI、Kafka schema 或 migration。

后续评审状态：本报告方向已被保留，但 `docs/sdd/agent-platform.md` v0.1
已在 `docs/research/agent-current-design-review-20260701.md` 中被判定为
`REJECTED_FOR_IMPLEMENTATION_PROMOTION`。后续不得只凭本初步报告进入实现；需要同时阅读
`docs/sdd/agent-runtime.md`、`docs/sdd/agent-memory-admission.md`、
`docs/sdd/agent-context-evidencepack.md`、`docs/sdd/agent-tool-mcp-boundary.md`、
`docs/sdd/agent-eval-replay-harness.md` 和 `docs/sdd/agent-governance-agentops.md`。

## 1. 背景

NexusIM 已经具备 IM 后端底座、search / memory / retrieval / RAG / summary /
agent / skill / MCP / action-executor / ai-eval 的 first path。当前 Agent 层不应继续按
“固定一种助手形态”推进，而应重新抽象为一个可运行、可治理、可审计、可评测的
Agent Plane。

本报告基于 `docs/research/agent-plane-redesign-20260701.md` 收敛出初步设计。
现有 IM 设计只作为参考，本文不冻结 EvidencePack、memory event、workflow、skill、
tool、MCP 或 agent taxonomy 的最终形态。

## 2. 设计目标

Agent 层的目标不是做一个能聊天的机器人，而是在 IM 协作系统上提供一个受控智能执行层：

- 能理解会话、群组、项目、任务和历史证据；
- 能通过权限过滤后的证据回答、总结、解释和提出行动建议；
- 能把长期群组记忆、项目事实和个人画像分层治理；
- 能调用工具，但不能让模型直接改业务事实；
- 能把复杂任务拆成可追踪的 run / step / checkpoint；
- 能让每次回答、proposal、approval 和 execution 都可审计、可回放、可评测。

核心定位：

```text
IM Core 负责可信通信和业务事实
Agent Plane 负责受控理解、推理、计划、工具意图和任务运行
Policy / Workflow / Executor / Audit 负责权限、审批、执行和追责
```

## 3. 不变量

以下约束应在后续所有 Agent 方案中保持：

1. IM 业务事实仍由 message、conversation、delivery、identity、contacts、policy 等服务拥有。
2. Agent 不进入 IM 消息投递热路径，只通过事件、显式请求或低频触发异步运行。
3. Agent 读取组织上下文必须经过 retrieval-gateway / EvidencePack 或带 policy 的公开服务 API。
4. RAG、summary 和 Agent 不直接读 search、memory、vector、业务数据库或其它服务私表。
5. Agent 写动作必须走 proposal、approval、action-executor、公开业务 API 和 audit。
6. Python AI Worker 只产出候选，不拥有最终状态、审批、持久化、执行状态或业务事实。
7. Memory 必须 source-backed、scoped、versioned、reviewed、可撤销，不能把模型摘要当长期事实。
8. MCP / tool server 不是权限系统，所有工具必须经过 skill、schema、policy、idempotency 和 audit。
9. fake / mock / fixture 只能用于实验或测试隔离，不能接生产启动路径，也不能做真实服务失败 fallback。
10. Eval 必须能区分 retrieval、reasoning、temporal、attribution、permission、tool-policy、memory-pollution 和 execution-safety 失败。

## 4. 总体架构

建议把 Agent 层拆成七个逻辑平面：

```text
Agent Gateway
  -> 接收 IM 触发、私聊、@agent、卡片按钮、定时任务和 webhook

Agent Runtime / Harness
  -> 管理 AgentRun、AgentStep、checkpoint、budget、retry、pause、resume、replay

Context Plane
  -> 通过 EvidencePack、ContextPackage、citation、source coverage 和 conflict marker
     构造模型可用上下文

Memory Plane
  -> 管理 run memory、conversation memory、project memory、profile aggregate 和
     memory admission

Tool / MCP Plane
  -> 管理 skill contract、tool intent、prepare、schema、policy、approval 和 executor handoff

Skill Plane
  -> 管理 versioned capability package，不提前冻结 agent 类型

Eval / Replay Plane
  -> 管理 agent run replay、回归评测、失败分类和发布门禁
```

这七个平面不要求一开始拆成七个服务。第一阶段可以复用现有服务边界，用文档、fixture
和小型 prototype 先验证运行模型。

## 5. 可借鉴系统与研究输入

本节记录可借鉴点，不把任何外部项目原样照搬进 NexusIM。外部系统更多提供设计约束：
哪些能力必须系统化，哪些能力不能只靠 prompt。

### 5.1 OpenClaw

OpenClaw 的核心启发是 Gateway 和协议边界：

- 单个长生命周期 Gateway 统一多消息通道，控制端、节点和 UI 都通过 typed WebSocket
  协议进入；
- 请求、响应、事件分离，并带 `seq` / `stateVersion`；
- 首帧 handshake、设备身份、pairing、auth 和 device token 是协议一部分；
- side-effecting method 要求 idempotency key；
- core 保持 plugin-agnostic，插件通过 SDK、manifest、runtime helper 进入；
- runtime state 默认进入 SQLite，不把状态散落成 JSON sidecar。

对 NexusIM 的落点：

- Agent Gateway 应该是 typed ingress，不是把 `@agent` 直接塞进某个 handler；
- AgentRun 接收后应立即返回 accepted / rejected，再通过事件或卡片流式更新；
- 所有可能产生 side effect 的 tool intent / action proposal / execution request 都必须带
  idempotency key；
- Agent 插件、skill、tool 不能把 owner-specific command tree 写进 IM channel；
- 运行状态应该有明确 owner，不把 checkpoint、queue、run cache 写成临时文件。

### 5.2 Hermes

Hermes 的核心启发是 memory 运行时生命周期，而不是单纯向量库：

- MemoryProvider 有 initialize、system prompt block、prefetch、queue prefetch、
  sync turn、tool schema、tool call、shutdown；
- 还有 turn start、session end、session switch、pre-compress、memory write、
  delegation 等 hook；
- MemoryManager 限制同一时间最多一个外部 memory provider，避免工具 schema 膨胀和
  多后端冲突；
- memory context 通过 `<memory-context>` 围栏注入，并在 streaming 输出中 scrub，避免
  内部 memory context 泄漏给用户。

对 NexusIM 的落点：

- Memory Plane 应该是 runtime protocol：prefetch、admission、sync、session switch、
  pre-compact、delegation 都要有 owner；
- 不能把长期 memory 简化为“写一条摘要到向量库”；
- 同一个 AgentRun 不应同时挂多个会互相冲突的 memory provider；
- ContextPackage 需要显式区分“用户输入”“检索证据”“回忆上下文”“工具结果”，并且输出前要有
  scrub / citation verify；
- subagent 的结果应作为 parent run 的 delegation observation，而不是自动污染长期 memory。

### 5.3 Claude Code

Claude Code 的核心启发是 agentic loop 周围的确定性控制面：

- hooks 覆盖 session、turn、tool use、permission、subagent、task、compact、file/worktree
  等生命周期点；
- `PreToolUse` / `PostToolUse` / `PermissionRequest` 等事件说明权限控制不能只靠模型自觉；
- subagents 可以拥有独立 prompt、tool restriction、permission mode、hooks 和 skills；
- checkpoint / compaction / permission / hook 组合让长任务可恢复、可收口、可治理。

对 NexusIM 的落点：

- Agent Runtime 应显式提供 lifecycle hook vocabulary，例如：
  `BeforeContextBuild`、`BeforeModelCall`、`PreToolPrepare`、`PostToolPrepare`、
  `BeforeWorkflowRequest`、`AfterWorkflowDecision`、`BeforeMemoryCandidate`、
  `BeforeSendMessage`、`PreCompact`、`PostCompact`；
- hook 只能做确定性控制、审计、阻断或上下文补充，不能成为隐藏业务 fallback；
- subagent 初期只能作为隔离候选生成单元；主 Agent 仍对最终回答和 proposal 负责；
- permission / policy / approval 必须是系统事件和决策，不是 prompt 里的自然语言规则。

### 5.4 2025/2026 论文和 benchmark

几个方向对 Agent Plane 特别关键：

- tau-bench 强调真实业务 agent 要同时处理用户交互、domain policy、API tools 和最终状态；
- ToolSandbox 强调 stateful tool execution、隐式状态依赖、交互式轨迹和中间里程碑评估；
- MultiAgentBench 强调多 agent 系统不能只看单 agent task completion，还要看协作、竞争、
  通信拓扑和 milestone KPI；
- JourneyBench 强调客服/业务流程 agent 的 policy adherence，动态流程控制比静态大 prompt
  更接近真实业务；
- 2026 企业报告普遍强调 governance、evaluation 和从单 chatbot 走向 multi-agent workflow。

对 NexusIM 的落点：

- Eval 不应只测“回答对不对”，还要测最终业务状态、policy adherence、tool state、approval
  bypass、memory pollution 和 replay consistency；
- AgentRun / AgentStep / ReplayBundle 必须能记录中间里程碑，否则无法定位失败；
- 多 Agent 不应先做“聊天式互相讨论”，应先做 bounded delegation：固定角色、固定工具权限、
  固定预算、主 Agent 负责最终输出；
- 客服、项目、运维等 IM Agent 应优先建 workflow-aware skill，而不是一个万能 prompt。

### 5.5 大厂协作产品信号

飞书、钉钉、Slack / Teams 类协作产品的共同方向是把 Agent 放进协作入口，而不是另起一个孤岛：

- Agent 作为 IM 联系人、群内成员、workflow node、任务卡片或审批卡片出现；
- 企业知识、业务系统工具、权限、审批、审计和工作流需要统一治理；
- Agent 能力需要可运营：版本、灰度、禁用、效果评估、管理员控制台。

对 NexusIM 的落点：

- Agent UI 应优先使用任务卡片、审批卡片、引用展开、进度状态、转人工和重试，而不是长文本刷屏；
- 群内 Agent 默认不应主动刷屏，复杂任务进入 thread / card / run view；
- Agent 发布要支持 owner、version、release channel、shadow / beta / production；
- 管理员必须能查看 Agent 记住了什么、执行了什么、为什么被拒绝、如何回放。

## 6. 推荐路线

### 6.1 保留现有安全 baseline

当前可继续保留：

```text
EvidencePack
-> RAG grounded answer / Agent proposal
-> approval
-> action-executor
-> public business API
-> audit
```

这条链路已经证明了一个重要原则：Agent 不直接写业务事实，业务 mutation 必须经过
skill、policy、approval、executor 和 audit。

### 6.2 探索 Agent Runtime / Harness

下一阶段应重点探索一个独立的 Agent Runtime / Harness。它不一定立刻成为新服务，但需要
先形成清晰的运行模型。

Runtime 应拥有：

- `AgentRun`：一次 agent 任务实例；
- `AgentStep`：检索、计划、工具准备、审批等待、执行观察、memory candidate 等步骤；
- `ContextPackage`：从 EvidencePack 派生出的模型输入包；
- `Checkpoint`：关键边界前的可回放状态；
- `RunBudget`：时间、token、工具调用、成本和 step 数限制；
- `ReplayBundle`：重建 run 所需的低敏引用、hash、版本和决策链。

建议状态词汇：

```text
CREATED
ACCEPTED
CONTEXT_BUILDING
PLANNING
TOOL_PREPARING
WAITING_APPROVAL
EXECUTION_REQUESTED
OBSERVING_RESULT
WRITING_MEMORY_CANDIDATE
COMPLETED
FAILED
CANCELLED
EXPIRED
```

这些名称目前只是设计词汇，不是 schema。

### 6.3 明确 Runtime 与 Workflow 的边界

Agent Runtime 与 workflow-service 不应互相替代：

| 关注点 | Owner 建议 |
| --- | --- |
| 模型调用、上下文包、planner candidate、tool intent、checkpoint、run replay | Agent Runtime |
| 人工审批等待、repair workflow、provider replay handoff、compensation、外部 callback | workflow-service |
| 最终业务写入、幂等执行、tool result projection、provider failure projection | action-executor |
| 权限判断、工具动作预检、risk / approval decision | policy-service / mcp-gateway |
| 低敏长期审计和导出 | audit-service |

这样可以避免 workflow-service 被迫理解模型上下文，也避免 agent-service 自己实现完整审批平台。

## 7. 关键数据边界

### 7.1 EvidencePack

EvidencePack 仍是 AI 读路径硬边界。RAG、summary 和 Agent 只能基于 EvidencePack 或其
派生包回答与规划。

要求：

- 每条 evidence 有 source ref、visibility / ACL version、projection version；
- profile aggregate 必须能追溯 supporting memory events；
- 无证据、证据冲突或权限不确定时必须 abstain 或 clarification；
- 删除、撤回、retention、legal hold 和历史成员窗口必须影响 evidence 可见性。

### 7.2 ContextPackage

ContextPackage 是建议新增的运行时概念，不是事实源。它用于把 EvidencePack 组织成模型可用输入：

```text
ContextPackage =
  evidence refs
  selected snippets
  source coverage
  conflict markers
  temporal version notes
  memory graph hints
  tool result refs
  prior checkpoint refs
```

ContextPackage 可以被 replay 和 eval 使用，但不能替代 EvidencePack 的审计地位。

### 7.3 MemoryCandidate

模型、Python worker 或 planner 可以提出 MemoryCandidate，但不能直接写 ACTIVE memory。

推荐准入链：

```text
candidate
-> source visibility check
-> classification
-> dedupe
-> conflict / supersedes check
-> profile overgeneralization check
-> review or auto-admit policy
-> ACTIVE / REJECTED / NEEDS_REVIEW
```

长期个人画像必须来自多证据聚合或用户确认，不能从单条群聊消息直接升级。

## 8. 工具与 MCP 设计

Agent 工具集成需要分层暴露，避免把所有 tool schema 一次性塞给模型：

```text
L0: 只暴露工具名和短描述
L1: 按任务搜索候选工具
L2: 加载少量候选 schema
L3: 通过 mcp-gateway prepare tool intent
L4: approval 后由 action-executor execute
```

工具调用必须满足：

- skill 已注册且 ACTIVE；
- tool/action 在 skill allowlist 内；
- input schema 校验通过；
- policy-service precheck 允许；
- risk / approval 条件满足；
- idempotency key 存在；
- audit 写入成功或执行失败也能记录稳定失败分类。

MCP server 只作为外部工具/数据连接端，不拥有 NexusIM 权限判断。未来如果引入 A2A
式 peer agent，应与 tool call 分开建模：peer agent 有自己的身份、能力边界和 audit lineage，
不能被简化为普通 function call。

## 9. Skill 设计

当前不建议冻结 agent taxonomy。应先冻结 skill 作为可版本化能力包的原则。

一个 skill 至少需要描述：

- owner、version、release channel；
- 适用任务和禁止任务；
- 必需 evidence source；
- 允许 tool intent；
- risk level 和 approval policy；
- 输入/输出契约；
- eval suite 绑定；
- rollout、shadow、disable 策略。

候选 first skills 仅供探索：

- 会话证据助手；
- 群组记忆 reviewer；
- 项目决策助手；
- 客服/支持 triage 助手；
- note / proposal 助手；
- policy explanation 助手。

## 10. Eval 与 Replay

Agent 层必须把 eval 当成架构组成部分，而不是上线前脚本。

最小评测族：

| Eval family | 目标 |
| --- | --- |
| evidence grounding | 回答/proposal 只能引用可见 evidence |
| source coverage | 缺少检索 lane 时显式暴露，不假装事实不存在 |
| temporal version | 区分当前事实、历史事实、superseded fact |
| memory admission | 阻止单消息画像泛化和无来源长期记忆 |
| group ambiguity | 保留 asker / speaker / audience / group scope |
| tool policy | 不允许绕过 prepare / policy / approval |
| execution safety | 高风险动作未审批不能执行 |
| provider failure | malformed / unsafe / timeout candidate fail closed |
| replay | 同一 run 输入能重建 evidence 和决策链 |

ReplayBundle 至少应记录低敏引用、hash、版本和决策链，不保存 raw prompt、raw provider
body、secret、完整敏感 payload。

## 11. 自主研究更新：2026 Agent 工程重点

本节基于后续自主研究补充，聚焦生产级 Agent 的薄弱点。它不改变本文“不冻结契约”的状态。

### 11.1 从 trace matching 转向 state-diff eval

Agent-Diff 的关键启发是：真实企业 API 任务不应只看调用参数是否像预期，也不应只看
模型轨迹是否合理，而要看外部环境最终状态是否发生了预期变化。它用 sandbox 统一执行层，
再用 state-diff contract 判断 Slack、Box、Linear、Google Calendar 等企业 API 任务是否真正完成。

对 NexusIM 的落点：

- Agent eval 要增加 `expected_state_diff` 思路：例如 conversation note 是否真的创建、
  成员是否没有被误改、审批状态是否正确推进；
- `ReplayBundle` 不只保存 agent trace，还要能关联 action-executor result、业务 request ref、
  audit record 和可验证的低敏 state diff；
- tool eval 不能只测“是否调用了某个 tool”，必须验证最终业务状态、幂等行为和副作用边界；
- 对外部 API / MCP provider，应优先在 sandbox / fixture contract 中验证，再进入真实 adapter。

### 11.2 Agent memory 要按“经验是否改善任务”评测

STATE-Bench 的关键启发是：memory 系统不是为了“能召回更多文本”，而是为了让 Agent 在后续
企业任务中利用经验、减少重复错误并保持正确上下文。memory-agnostic benchmark 更适合比较
不同 memory 结构，而不是绑定某个向量库。

对 NexusIM 的落点：

- Memory eval 应增加 before/after task suite：同一用户/群/项目在有经验与无经验下的任务成功率；
- MemoryCandidate admission 不能只看抽取准确率，还要看长期污染率、过期事实误用率、个人画像过度泛化率；
- run memory、conversation memory、project memory 和 profile aggregate 必须分开评测；
- memory recall 应记录 why-used / why-not-used，避免模型把历史回忆当成当前事实。

### 11.3 MCP 安全必须进入架构，而不是工具接入后补

MCPSecBench、ToolHijacker 和 prompt-injection / tool-poisoning 相关研究说明：MCP 把 agent
能力扩展到外部数据和工具，也显著扩大攻击面。攻击可以发生在 tool 描述、tool selection、
MCP sampling、外部返回内容或恶意 server 行为中。

对 NexusIM 的落点：

- MCP / tool registry 中的 tool description、schema、examples、doc chunks 都应视为不可信输入；
- tool selection 不能只由模型在长 tool list 中自由选择，必须经过 skill allowlist、risk policy、
  tenant policy 和 explicit prepare；
- `mcp-gateway` 应记录 tool-source provenance、schema version、risk label、owner service 和
  policy decision ref；
- external MCP output 进入 ContextPackage 前必须经过 output safety gate、source labeling 和
  prompt-injection quarantine；
- 高风险或未知 MCP server 默认不能进入 auto-execute，只能 proposal-only 或 sandbox-only。

### 11.4 Observability 要覆盖 agent 行为，而不只是模型延迟

2026 企业报告和 observability 文章都强调，agent 上线后最难的是看清楚它在生产中做了什么、
为什么这么做、用了哪些数据和工具、是否仍在 policy 边界内。传统模型指标不足以覆盖多步、
多工具、记忆和执行链。

对 NexusIM 的落点：

- 每个 AgentRun 需要统一 trace id，贯穿 retrieval、model/provider、mcp prepare、workflow、
  executor 和 audit；
- 指标至少覆盖：run success / failure class、tool prepare deny、approval reject、memory candidate
  reject、citation verifier failure、provider timeout、cost、latency、replay availability；
- 管理面应能按 agent、skill、tenant、conversation、tool、risk level 聚合观察；
- eval 结果要能回连生产 failure class，避免“离线评测一套，线上事故另一套”。

### 11.5 生产治理重点从“模型治理”转到“行动治理”

2026 企业 Agent 报告反复指出：agent 治理和传统模型治理不同，因为 agent 会代表组织行动。
治理对象不仅是 prompt 和 model，还包括 agent identity、tool permission、memory、approval、
execution、cost、release channel 和 rollback。

对 NexusIM 的落点：

- 每个 AgentDefinition / SkillPackage 必须有 owner、allowed scope、risk profile、release channel
  和 disable switch；
- agent identity 不能复用普通 user identity，必须表达 `on_behalf_of` 和 delegated subject；
- high-risk action 的关键门禁是 policy + approval + executor，而不是让模型“承诺不乱做”；
- 灰度 / shadow / rollback 应成为 Agent Plane 的 release 能力，而不是部署脚本外置约定；
- audit 应保存低敏 refs / hashes / decisions，而不是 raw prompt 或 provider body。

### 11.6 对当前设计的修正

基于以上研究，当前报告的设计倾向做三点修正：

1. `ReplayBundle` 的优先级上调。没有 replay / state diff，就无法证明 Agent 是可靠执行体。
2. `mcp-gateway` 的安全职责上调。它不只是 prepare 边界，还应成为 MCP provenance / poisoning
   防护 / tool source audit 的入口。
3. `ai-eval-service` 后续不应只是 run catalog。它需要逐步承接 retrieval、memory、tool、
   state-diff 和 replay consistency 的 suite aggregation。

这些修正仍然只属于探索结论；是否进入 ADR / SDD 需要后续主集成或用户明确推进。

## 12. 服务演进选项

当前不建议立即新增生产服务。后续如果 Agent Runtime 方向通过主集成评审，有三个演进选项：

1. 在 `agent-service` 内扩展 runtime/harness 模块。
2. 新增 `agent-runtime-service`，让 `agent-service` 保持 public proposal API。
3. 保留 `workflow-service` 作为长等待/审批执行状态机，同时新增轻量 `agent-runner`
   负责认知 runtime。

初步判断：

- 选项 1 最省服务数量，但容易让 `agent-service` 变大。
- 选项 2 边界最清楚，但需要新增服务、门禁和运行成本。
- 选项 3 适合作为过渡，但必须防止 runner 与 workflow 两套状态机交叉污染。

建议先用 fixture-only prototype 验证 run trace、ContextPackage 和 replay，再决定是否写 ADR。

## 13. 初步落地实验

建议下一步只做探索实验，不做生产契约：

1. 在 `ai/experiments/agent-runtime-trace/` 做 fixture-only run trace prototype。
2. 做 ContextPackage builder prototype：输入 fake EvidencePack，输出 citation-preserving context。
3. 扩展 memory admission eval：覆盖群组歧义、项目决策 supersedes、个人画像过度泛化。
4. 做 tool prepare vs execute lineage replay：串起 skill、mcp prepare audit、proposal、executor result。
5. 写 Runtime vs Workflow ownership matrix，作为后续 ADR 的前置材料。
6. 补 state-diff eval fixture：用低敏 fixture 验证 action-executor result 与预期业务状态差异。
7. 补 MCP security fixture：模拟 poisoned tool description、unsafe provider output 和 tool-selection attack。

## 14. 风险

- 过早拆服务会增加本地运行和门禁成本。
- 过晚拆 runtime 会让 agent-service 吸收过多编排、workflow、tool、memory 和 eval 逻辑。
- MCP 外部生态安全风险高，不能把外部 MCP server 当可信权限边界。
- Memory 污染会长期影响 RAG 和 Agent，必须优先做 admission / review / eval。
- 没有 ReplayBundle 的 Agent 很难定位线上问题。
- 只做 pass/fail eval 不够，必须保留 failure taxonomy。
- 只做 trace matching 的 eval 不够，必须补最终状态变化和副作用验证。
- MCP tool description / output 本身可能被污染，不能当作可信上下文。

## 15. 结论

NexusIM Agent 层建议采用“安全 baseline + Runtime 探索”的路线：

```text
短期 baseline:
  EvidencePack -> grounded answer / proposal -> approval -> executor -> audit

中期目标:
  Agent Gateway -> Agent Runtime / Harness -> Context / Memory / Tool / Eval

长期能力:
  versioned skills、受控 MCP、candidate-only Python worker、group memory、
  replayable long-running agent、可审计多 agent 协作
```

下一步不应立刻冻结 proto 或新增生产服务，而应先完成 run trace、ContextPackage、
memory admission eval 和 runtime/workflow ownership 的小型实验。主集成评审后，再决定是否将
Agent Runtime / Harness Plane 提升为 ADR / SDD / runtime 实现。

## 16. 参考输入

本报告综合了本地源码/文档与 2026-07-01 可访问的公开资料：

- 研究附录：`docs/research/agent-ecosystem-research-20260701.md`，汇总 2025/2026
  Agent 框架、开源项目、benchmark、安全论文和企业产品报告输入。
- 完整系统范围探索：`docs/research/agent-system-complete-scope-20260701.md`，整理
  2026 年企业 Agent 系统应包含的能力平面与离线公开数据集验证流程。
- 平台级 SDD 草案：`docs/sdd/agent-platform.md`，将 Agent Gateway、identity /
  policy / budget、AgentDefinition、SkillPackage、Runtime / Harness、Context /
  RAG、Memory、Tool / MCP、A2A、Workflow、Action Executor handoff、Python AI
  Worker、Eval / Replay、Observability / Audit、AgentOps 和 Security 收口为可评审设计。
- 本地 OpenClaw：`E:\agent\openclaw\docs\concepts\architecture.md`、
  `E:\agent\openclaw\AGENTS.md`。
- 本地 Hermes：`E:\agent\Hermes\agent\memory_provider.py`、
  `E:\agent\Hermes\agent\memory_manager.py`。
- Claude Code official docs：hooks、subagents、checkpoint、permission / MCP lifecycle。
- OpenAI：A Practical Guide to Building Agents。
- Anthropic / MCP：Model Context Protocol 公开说明与规范。
- Google A2A：Agent2Agent 协议公开说明。
- 论文 / benchmark：tau-bench、ToolSandbox、MultiAgentBench、JourneyBench、
  Agent-Diff、STATE-Bench、MCPSecBench、ToolHijacker / MCP tool-selection attack。
- 企业报告 / 产品信号：Databricks 2026 State of AI Agents、飞书 Aily / workflow
  AI Agent node、钉钉 Agent OS 相关公开材料、AI agent observability / governance
  相关 2026 报告。
