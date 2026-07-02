# NexusIM Agent Platform SDD

日期：2026-07-01

状态：Agent Exploration Mode 的平台级 SDD 草案；不是 ADR、proto、OpenAPI、Kafka
schema、migration、生产服务目录或生产启动路径。

评审状态：本文件的 v0.1 方向正确，但已在
`docs/research/agent-current-design-review-20260701.md` 中被判定为
`REJECTED_FOR_IMPLEMENTATION_PROMOTION`。它不能单独作为实现推广依据。后续评审应同时阅读
本文件和以下重做后的详细 SDD 包：

- `docs/sdd/agent-runtime.md`
- `docs/sdd/agent-memory-admission.md`
- `docs/sdd/agent-context-evidencepack.md`
- `docs/sdd/agent-tool-mcp-boundary.md`
- `docs/sdd/agent-eval-replay-harness.md`
- `docs/sdd/agent-governance-agentops.md`

## 1. 目的和定位

本 SDD 只回答一个问题：如果 NexusIM 要在 2026 年构建完整的企业 IM Agent 层，
它应该由哪些部分组成，各部分拥有什么责任、状态、边界、失败语义、可观测性和
验证门禁。

本文件是 `docs/architecture/agent-plane-initial-design.md` 和
`docs/research/agent-system-complete-scope-20260701.md` 之后的设计收口稿。它
把前期 OpenClaw、Hermes、Claude Code、OpenAI Agents SDK、LangGraph、MCP、A2A、
Microsoft Agent Framework、Google agentic architecture、Databricks 2026 agent
report，以及 2025/2026 agent benchmark / safety / memory 论文输入，整理为 NexusIM
自己的 Agent Platform SDD。

本文件不替代已有服务 SDD。`agent-service.md`、`memory-service.md`、
`retrieval-gateway.md`、`rag-service.md`、`mcp-gateway.md`、`action-executor.md`、
`ai-eval-service.md`、`model-gateway.md` 等仍是各服务边界的当前设计入口。本文件描述
这些服务之上的 Agent 平台组合方式和后续拆分方向。

### 1.1 非目标

- 不冻结 Agent taxonomy、Skill taxonomy、EvidencePack shape、Memory event shape、
  Tool shape、MCP shape、A2A peer contract、ReplayBundle shape。
- 不新增生产服务、生产目录、proto、OpenAPI、Kafka schema、migration 或 runtime
  implementation。
- 不接入真实 NexusIM IM 数据作为第一阶段 Agent 能力验证数据。
- 不把某个开源框架、benchmark 或厂商产品直接照搬为 NexusIM 终局。
- 不允许 Agent 绕过 auth、policy、approval、action-executor、audit 或 service-owned
  business state。

### 1.2 设计原则

1. Agent 是企业 IM 的增强层，不进入消息投递、会话成员变更、回执、推送等 hot path。
2. 读路径必须通过权限过滤后的 EvidencePack / ContextPackage，不直接读业务库。
3. 写路径必须通过 proposal / approval / action-executor / audit，不直接写业务事实。
4. Memory 是一等系统，不是 RAG cache；必须 source-backed、scoped、versioned、reviewed、
   revocable。
5. Tool / MCP 是不可信边界；tool description、tool output、remote MCP server 均不能作为
   权限依据。
6. Python AI Worker 只产出候选、评分、解释或 embedding/rerank 结果；Go 服务拥有最终
   auth、policy、audit、persistence、memory admission、execution state。
7. Eval / replay 是 Agent 平台组成部分，不是上线后的脚本。
8. Workflow-service 只拥有长等待、审批、补偿和恢复状态；Agent Runtime 不成为第二个业务
   长事务引擎。

## 2. 研究输入摘要

### 2.1 开源和产品架构输入

| 输入 | 对 NexusIM 的启发 | 不直接采用的部分 |
| --- | --- | --- |
| OpenClaw | workspace-aware agent、任务过程可恢复、工具/文件/会话边界清晰 | 不把本地开发 agent 的权限模型照搬到企业 IM |
| Hermes | memory prefetch / write hook、MemoryProvider 抽象、对话中持续更新上下文 | 不让模型输出直接进入 ACTIVE memory |
| Claude Code | hooks、subagents、checkpoint、permission gate、MCP lifecycle | 不把 coding agent 的命令权限模型复制到 IM 业务动作 |
| OpenAI Agents SDK | agent / handoff / guardrails / tracing 的轻量组合方式 | 不锁定单一 SDK 或 Python-first runtime |
| LangGraph | long-running workflow、checkpoint、human-in-the-loop、durable execution | 不让图 runtime 拥有业务最终状态 |
| Microsoft Agent Framework | agent 与 workflow 统一视角、observability / tracing 作为默认能力 | 不把单平台 tracing schema 变成硬依赖 |
| Google agentic architecture | model、tool、memory、planning、evaluation、orchestration 的分层 | 不把云厂商组件名写成产品绑定 |
| MCP / A2A | tool interoperability、agent-to-agent 协议化协作 | 不把外部 server 或 peer agent 当可信主体 |
| Databricks 2026 report | 企业落地瓶颈集中在 governance、observability、quality、integration | 不用报告中的 adoption taxonomy 替代系统设计 |

### 2.2 论文和 benchmark 输入

| 方向 | 参考输入 | NexusIM 需要吸收的能力 |
| --- | --- | --- |
| Grounded RAG | BEIR、Natural Questions、HotpotQA、Qasper、MS MARCO | citation、abstain、source coverage、conflict handling |
| Tool / workflow | tau-bench、ToolSandbox、BFCL、MCP-Bench | tool choice、argument correctness、external state validation |
| State diff | Agent-Diff | 不能只看 tool call trace，必须检查业务状态变化 |
| Memory | STATE-Bench、LoCoMo、LongMemEval、EverMemBench、GroupMemBench | long-horizon recall、update、forget、group memory、scope control |
| Multi-agent | MultiAgentBench、MARBLE | bounded delegation、handoff contract、role isolation |
| Security | MCPSecBench、ToolHijacker、MCP tool-selection attack | poisoned tool description、unsafe output、capability confusion |
| Policy adherence | JourneyBench 等 journey / policy benchmark | instruction hierarchy、approval gate、unsafe task refusal |

这些输入只作为设计压力测试。NexusIM 的核心必须来自企业 IM 的事实边界：tenant、org、会话、
群组、成员、权限、审计、审批、合规保留、消息时序和业务动作所有权。

## 3. 总体架构

```text
Client / Admin / Bot Surface
  -> Agent Gateway / UX
  -> Agent Identity / Policy / Budget
  -> AgentDefinition / SkillPackage Governance
  -> Agent Runtime / Harness
       -> Model Gateway
       -> Context / EvidencePack / RAG
       -> Memory System
       -> Tool / MCP Gateway
       -> A2A / Peer Agent Boundary
       -> Workflow / HITL
       -> Python AI Worker
  -> Proposal / Approval
  -> Action Executor
  -> Audit / Replay / Eval / AgentOps
```

### 3.1 控制面、认知面、执行面

| 平面 | 包含组件 | 主要责任 | 禁止责任 |
| --- | --- | --- | --- |
| Control Plane | AgentDefinition、SkillPackage、policy、budget、release gate | 谁能运行什么 agent，用哪些模型、工具、memory scope 和预算 | 不执行业务动作，不保存 prompt body |
| Cognitive Plane | Agent Runtime、model gateway、context builder、RAG、memory retrieval、planner | 生成候选答案、计划、tool intent、memory candidate、proposal draft | 不拥有审批等待、业务最终状态、审计归档 |
| Execution Plane | workflow-service、action-executor、业务服务、audit-service | 审批、幂等执行、补偿、结果记录、事实归属 | 不解析 raw prompt，不重跑模型规划 |
| Evaluation Plane | ai-eval-service、ai-eval-harness、replay、dataset adapters | 离线验证、回放、质量门禁、失败分类 | 不接入生产启动路径作为 fallback |

### 3.2 核心运行不变量

- `AgentRun` 可以失败、暂停、取消、重放，但每一步都必须可追溯到低敏引用。
- `ContextPackage` 只包含权限过滤后的内容或低敏 refs；不得把越权资料注入模型上下文。
- `EvidencePack` 是 grounded answer / proposal 的证据基础；引用缺失时必须 abstain 或降级为
  clarification。
- `MemoryCandidate` 进入 review / admission pipeline 后才可能成为 ACTIVE memory。
- `ToolIntent` 只是准备态，不能执行外部副作用。
- `ApprovalWorkflowRef` 由 workflow-service 持有；Agent Runtime 只能等待引用状态变化。
- `ExecutionRef` 由 action-executor / 业务服务持有；Agent 不直接写业务事实。
- `ReplayBundle` 是低敏证据包，服务于调试和 eval，不是原始 prompt / message archive。

## 4. 组件设计

### 4.1 Agent Gateway / UX

Agent Gateway 是用户、群聊、管理员、自动化入口与 Agent 平台之间的入口层。

| 项 | 设计 |
| --- | --- |
| 主要输入 | 用户问题、群聊 @agent、管理台任务、workflow callback、scheduled evaluation |
| 主要输出 | AgentRun request、answer draft、proposal draft、clarification、approval request |
| 拥有状态 | request id、caller identity ref、conversation ref、client correlation id、rate-limit counters |
| 不拥有状态 | raw business facts、ACTIVE memory、workflow long-wait state、tool secret、execution result truth |
| 失败语义 | 鉴权失败直接拒绝；预算不足返回不可执行原因；下游不可用时不伪造结果 |

职责：

- 解析入口来源和用户意图的最小 envelope。
- 绑定 tenant、actor、conversation、device、locale、client capability。
- 调用 policy / budget / AgentDefinition gate。
- 统一返回 grounded answer、proposal、clarification、approval pending、failure envelope。
- 记录低敏 request trace id，便于 audit / replay 关联。

非职责：

- 不拼装 RAG 上下文。
- 不做 memory admission。
- 不调用外部 MCP tool。
- 不直接创建业务写动作。

关键设计点：

- 群聊入口必须区分 `ask in group`、`summarize group`、`propose action from group`、
  `memory admission from group`。
- UI 必须向用户暴露 Agent 的 certainty / source / approval 状态，但不能把内部 planner
  token 或 prompt 原文作为用户可见承诺。
- 对高风险动作，UX 只能展示 proposal 和审批入口，不能展示“已执行”直到 action-executor
  返回 ExecutionRef。

### 4.2 Agent Identity / Policy / Budget

Agent identity 负责把“哪个人/哪个群/哪个系统任务在运行哪个 agent”转成可审计的主体和能力集合。

| 项 | 设计 |
| --- | --- |
| 拥有状态 | Agent principal ref、delegated actor ref、tenant scope、role/capability snapshot ref、budget ledger ref |
| 不拥有状态 | 业务实体最终状态、memory body、tool secret、model transcript |
| 上游 | Agent Gateway、scheduler、workflow callback |
| 下游 | policy-service、model-gateway、retrieval-gateway、mcp-gateway、workflow-service |

职责：

- 生成或解析 Agent principal，区分 user-delegated agent、group-scoped agent、system agent、
  eval agent。
- 计算本次 AgentRun 的 permission envelope：read scopes、write proposal scopes、tool scopes、
  memory scopes、model/provider allowlist。
- 管控 token / cost / tool call / wall-clock / retry budget。
- 维护 instruction hierarchy：system > tenant policy > admin policy > agent definition >
  conversation context > user request > retrieved content > tool output。
- 为每一次 tool prepare、memory candidate、proposal、model call 附带 policy decision ref。

非职责：

- 不存储最终决策解释全文。
- 不在 policy 层调用模型判断权限。
- 不把 external MCP server 声明的 capability 直接并入权限集合。

### 4.3 AgentDefinition / SkillPackage / Release Governance

AgentDefinition 是“可运行 agent”的配置元数据。SkillPackage 是 agent 可使用的任务能力包。
二者都属于发布治理对象，不是单次运行状态。

| 对象 | 草案含义 | 必须包含的元数据方向 |
| --- | --- | --- |
| AgentDefinition | agent 的目标、允许场景、默认模型、memory scope、tool scope、handoff scope | owner、version、risk tier、release stage、eval gate、rollback ref |
| SkillPackage | 一组可组合技能、prompt policy、tool intents、retrieval profile、output validators | semantic version、required scopes、test fixtures、known failure classes |
| AgentRelease | 某版本 AgentDefinition + SkillPackage 的发布记录 | approver、eval result ref、canary policy、rollback policy |

职责：

- 让 agent 能力发布可审计、可回滚、可灰度。
- 在生产实现前定义“技能必须带 eval gate”的原则。
- 将 high-risk skill 与 approval / action-executor 绑定。
- 允许不同 tenant 或 workspace 选择不同 release channel。

非职责：

- 不在 SkillPackage 内保存 provider secret。
- 不让 skill 自行声明绕过 approval。
- 不让 prompt text 成为唯一的安全边界。

### 4.4 Model Gateway / Provider Boundary

Model Gateway 是模型供应商隔离层，已有 `docs/sdd/model-gateway.md`。Agent Platform 对它的
要求是：模型调用必须被 budget、policy、trace、eval 和 privacy 共同包裹。

| 项 | 设计 |
| --- | --- |
| 拥有状态 | provider config ref、model policy、request metadata、rate/cost accounting |
| 不拥有状态 | AgentRun planner state、ACTIVE memory、business execution state |
| 关键输出 | model response candidate、usage metrics、safety metadata、provider error class |

职责：

- 对 Agent Runtime 提供统一 model call / embedding / rerank / classification boundary。
- 屏蔽 provider-specific API，保留 provider error taxonomy。
- 执行 prompt/data retention policy、PII redaction policy、model allowlist。
- 输出低敏 trace metadata：model、version、latency、token/cost、safety outcome。

非职责：

- 不解释 tool result 为业务事实。
- 不保存完整 prompt 作为永久审计。
- 不直接写 eval result；eval-service 消费 trace/ref 后生成报告。

### 4.5 Agent Runtime / Harness

Agent Runtime 是认知编排层，负责把一次 AgentRun 分解为 context build、model call、tool
prepare、memory candidate、proposal draft、handoff、clarification 等步骤。它不拥有业务长事务。

| 项 | 设计 |
| --- | --- |
| 拥有状态 | ephemeral planner state、step trace、retry state、candidate outputs、pause reason |
| 可持久化状态 | AgentRun summary、step refs、low-sensitive hashes、failure class、ReplayBundle ref |
| 不拥有状态 | approval wait state、execution final state、ACTIVE memory、business fact、workflow compensation |
| 上游 | Agent Gateway、workflow callback、eval harness |
| 下游 | model-gateway、retrieval-gateway、memory-service、mcp-gateway、workflow-service、audit-service |

职责：

- 驱动单次 AgentRun 的短周期认知过程。
- 生成 structured step trace：context built、model called、tool prepared、memory candidate created、
  proposal drafted、approval requested、handoff emitted。
- 管理 retry/redrive 的认知边界：provider timeout 可重试，policy deny 不重试，unsafe tool output
  转 failure class。
- 允许 cancel/resume/replay，但 replay 不重新执行外部副作用。
- 对长等待 approval 返回 pause，交由 workflow-service 持有等待状态。

非职责：

- 不在内存中长时间等待用户审批。
- 不把 planner state 写成事实源。
- 不直接执行 MCP tool 的副作用动作。
- 不把失败后的 fake/mock 结果返回生产用户。

Runtime 需要支持的最小步骤类型：

| Step | 描述 | 必须记录 |
| --- | --- | --- |
| intake | 入口请求归一化 | caller ref、agent definition ref、risk tier |
| policy_check | 权限、预算、模型、tool gate | decision ref、deny reason class |
| context_build | 构建 EvidencePack / ContextPackage | source refs、coverage、abstain flag |
| model_call | 调用模型或 Python candidate worker | model ref、usage、latency、candidate hash |
| memory_retrieve | 读取 scoped memory | memory refs、scope、version |
| memory_candidate | 生成待入库记忆 | source refs、scope、confidence、review reason |
| tool_prepare | 准备工具调用 | tool ref、args hash、policy ref、dry-run output |
| proposal | 生成业务动作提案 | evidence refs、approval requirement |
| workflow_wait | 交给 workflow-service 等待 | workflow ref、timeout policy |
| handoff | 交给其他 agent / peer | recipient ref、task envelope、budget |
| finalize | 返回答案或最终状态 | output class、failure class、audit ref |

### 4.6 Context / EvidencePack / RAG

Context / RAG 层把 search、retrieval、memory 和 domain refs 组合成模型可用的上下文。已有
`retrieval-gateway.md`、`rag-service.md`、`search-service.md`、`vector-index-service.md`。

| 项 | 设计 |
| --- | --- |
| 拥有状态 | query plan ref、retrieval result refs、citation map、source coverage metrics |
| 不拥有状态 | raw message truth、memory admission state、execution state |
| 关键输出 | ContextPackage、EvidencePackRef、abstain reason、conflict marker |

职责：

- 根据 AgentRun permission envelope 构造检索范围。
- 支持 conversation、group、project、knowledge base、memory、tool documentation 的分层 retrieval。
- 输出 source-backed ContextPackage：每个可引用事实都能回到 source ref。
- 标注冲突、时效性、权限不足、source coverage 不足。
- 对不可回答问题输出 abstain 或 clarification，不生成“看似合理”的答案。

非职责：

- 不把 retrieved text 写入 memory。
- 不让模型凭空补 EvidencePack。
- 不把 search result 排名当权限。

关键质量指标：

- citation coverage：答案关键断言有证据比例。
- abstain precision：证据不足时是否拒答。
- permission leakage：越权 source ref 为零。
- conflict handling：冲突证据是否标注并要求确认。
- freshness：旧事实被 superseded 后是否还能被错误引用。

### 4.7 Memory System

Memory 是 Agent 平台的核心系统，不是 prompt cache。NexusIM 需要同时支持个人记忆、群组记忆、
项目记忆、组织政策记忆和短期运行记忆，但第一阶段只做公开数据集和 synthetic IM-like fixture。

| Memory 类别 | 作用 | 默认 admission |
| --- | --- | --- |
| Episodic memory | 某次交互或事件的低敏摘要和 source refs | 自动候选，review/admission 后 ACTIVE |
| Semantic memory | 稳定事实、偏好、项目决策、术语 | 必须 source-backed，冲突时 human review |
| Procedural memory | agent/skill 的操作偏好和步骤经验 | 必须绑定 SkillPackage version |
| Group memory | 群组共同决策、长期上下文、共享偏好 | 必须有 audience scope 和 speaker attribution |
| Project memory | 项目目标、约束、ADR-like 决策 | 必须支持 supersedes / revocation |
| Short-term run memory | 当前 AgentRun 的 planner scratchpad | 只属于 runtime，不进入 ACTIVE memory |

Memory pipeline：

```text
Source event / EvidencePack / AgentRun trace
  -> MemoryCandidate extraction
  -> policy / scope / PII / confidence gate
  -> conflict and supersedes check
  -> human or automated review
  -> ACTIVE memory
  -> retrieval with version and scope
  -> correction / revocation / expiry
```

Memory 必须拥有：

- source refs，而不是裸模型总结。
- scope：个人、群组、项目、组织、tenant、eval fixture。
- subject 和 audience：谁说的、适用于谁、谁可以读取。
- version / supersedes / expiry / revocation。
- confidence、review outcome、admission reason、conflict marker。
- read audit 和 use audit，至少保留低敏引用。

Memory 不能拥有：

- 业务服务最终状态。
- 隐式越权的群聊内容。
- 未经 admission 的模型推断。
- provider prompt transcript 的长期原文。
- action-executor 的 execution truth。

Memory 需要支持的评估：

| Eval 类别 | 覆盖问题 |
| --- | --- |
| recall | 能否在长上下文后找回真实、仍有效、权限允许的记忆 |
| update | 新事实是否 supersede 旧事实 |
| forget | revocation / expiry 后是否不再使用 |
| scope | group memory 是否泄露到个人或其他群 |
| attribution | 群组中多 speaker 事实是否归属正确 |
| overgeneralization | 一次对话偏好是否被错误升级为长期偏好 |
| conflict | 冲突事实是否触发确认或 review |
| pollution | 错误候选是否被 admission gate 拦下 |

### 4.8 Tool / MCP Gateway

Tool / MCP Gateway 管理外部工具发现、注册、prepare、sandbox、risk classification 和 provider
provenance。它不是权限源，不能替代 policy-service。

| 项 | 设计 |
| --- | --- |
| 拥有状态 | tool registry ref、provider provenance、capability metadata、prepare result、sandbox policy |
| 不拥有状态 | actor permission truth、approval state、execution final state |
| 上游 | Agent Runtime、SkillPackage governance |
| 下游 | external MCP server、internal tool adapter、action-executor |

职责：

- 管理 tool provider 的注册、版本、owner、risk tier、allowed tenants。
- 对 tool description、tool schema、provider output 做不可信输入处理。
- 提供 prepare / dry-run / validation，不执行高风险业务副作用。
- 绑定 policy decision、approval requirement、audit trace。
- 将生产执行交给 action-executor 或业务服务 adapter。

非职责：

- 不根据 MCP server 自称的权限决定可执行范围。
- 不把 tool output 注入 prompt 后直接执行下一步高风险动作。
- 不保存 provider secret 明文。

安全要求：

- Tool description injection 检测。
- Tool selection attack eval。
- Provider provenance 校验。
- Output taint tracking：外部输出进入模型上下文时降级为 untrusted evidence。
- High-risk tool 必须 prepare-only，执行必须走 approval / action-executor。

### 4.9 A2A / Peer-Agent Boundary

A2A / peer-agent boundary 用于与其他 agent 或系统协作。它不是信任扩张机制，只是任务委托协议。

| 项 | 设计 |
| --- | --- |
| 拥有状态 | peer registry ref、handoff envelope、delegation budget、result ref |
| 不拥有状态 | peer internal chain-of-thought、peer permission truth、business execution truth |

职责：

- 定义 bounded delegation：任务、scope、budget、deadline、allowed tools、expected artifact。
- 记录 handoff trace 和 peer result provenance。
- 对 peer output 进行 source/citation/policy validation。
- 支持失败、超时、partial result、reject、cancel。

非职责：

- 不把 peer agent 的回答当作 NexusIM 内部事实。
- 不跨 tenant 传递未授权上下文。
- 不把 peer 的 tool capability 并入当前 agent 权限。

当前 fixture-only evidence：

- `ai/python/nexusim_ai_eval/multi_agent_handoff.py`；
- `ai/python/fixtures/agent_eval/multi_agent_handoff_rehearsal.json`；
- `docs/research/agent-multi-agent-handoff-fixture-evidence-20260702.md`。

该证据只验证 bounded delegation 的 owner、scope、budget、deadline、taint、
audit、replay 和 verifier 边界；不冻结生产 A2A protocol、peer-agent identity
contract 或 integration path。

### 4.10 Workflow / Human-in-the-loop

Workflow-service 拥有长等待、审批、补偿、resume、timeout、escalation。Agent Runtime 只发起或
监听 workflow ref。

| 场景 | Agent Runtime | workflow-service |
| --- | --- | --- |
| 只读问答 | 生成 grounded answer，不进入 workflow | 无状态 |
| 需要审批的动作 | 生成 proposal，提交 workflow request | 持有 approval state |
| 长时间等待用户 | 暂停 run，等待 workflow callback | 持有 timer、reminder、timeout |
| 审批通过 | 接收 callback，继续生成执行请求 | 输出 approved decision |
| 审批拒绝 | final failure / explain | 输出 rejected decision |
| 补偿 / redrive | 只重新做认知步骤 | 持有补偿策略和业务状态机 |

Workflow-service 不应该理解：

- raw prompt。
- EvidencePack body。
- planner scratchpad。
- model output token。
- memory candidate body。

Workflow-service 必须理解：

- approval request ref。
- approver identity / decision。
- timeout / escalation policy。
- execution precondition refs。
- compensation / retry policy。

### 4.11 Action Executor Handoff

Action-executor 是所有真实业务写动作的执行入口。Agent 只能提交已审批、已 prepare、可审计的
ExecutionRequest ref。

| 项 | 设计 |
| --- | --- |
| 拥有状态 | execution request、idempotency key、execution result、compensation ref、business outcome refs |
| 不拥有状态 | raw prompt、planner state、memory candidate、tool description trust |

职责：

- 校验 approval、policy、prepare lineage、idempotency、target service contract。
- 执行或转发给业务服务。
- 记录 execution result 和 audit event。
- 支持 state-diff eval：期望状态变化与实际变化可比对。

非职责：

- 不重新规划 agent 任务。
- 不自动修正模型参数。
- 不执行无法审计或未审批的动作。

### 4.12 Multi-agent Bounded Delegation

Multi-agent 不应被定义为“多个模型互相聊天”。NexusIM 的 multi-agent 需要可审计、可限制、可取消。

| 角色 | 说明 | 边界 |
| --- | --- | --- |
| Coordinator | 拆分任务、分配 budget、整合 artifact | 不越权扩大 scope |
| Specialist | 在受限 scope 内处理子任务 | 不调用未授权工具 |
| Critic / Verifier | 检查 citation、policy、state diff、risk | 不拥有最终业务执行 |
| Memory Reviewer | 对 MemoryCandidate 评分和冲突检查 | 不直接提升 ACTIVE memory |
| Eval Judge | 离线评估输出质量 | 不进入生产决策路径 |

必须具备：

- delegation envelope。
- per-agent budget。
- cancellation propagation。
- per-agent trace。
- result validation。
- failure containment。

### 4.13 Python AI Worker

Python AI Worker 是 candidate-only intelligence plane，适合承载模型生态、embedding、rerank、judge、
offline eval、prototype 算法。Go 服务仍是生产控制和最终状态边界。

| 可做 | 不可做 |
| --- | --- |
| candidate answer / proposal draft | 持久化 final answer state |
| memory candidate extraction | 直接写 ACTIVE memory |
| rerank / embedding / classifier | 绕过 retrieval permission |
| eval judge / report generation | 修改 production policy |
| model-specific adapter | 保存 approval / execution / audit truth |
| fixture-only prototype | 作为真实服务失败 fallback |

Worker 调用必须带：

- request envelope。
- data classification。
- allowed model / tool profile。
- timeout / retry budget。
- output validator。
- trace correlation id。

Worker 输出必须是候选：

- candidate text。
- structured scores。
- source refs。
- uncertainty / failure reason。
- validation metadata。
- low-sensitive hashes。

### 4.14 Eval / Replay / Open Dataset Harness

第一阶段不使用真实 NexusIM IM 数据。Agent 能力先用公开数据集和 synthetic IM-like fixture 验证。

| 能力 | 首批数据集 / fixture | 通过标准方向 |
| --- | --- | --- |
| Grounded RAG | BEIR、NQ、HotpotQA、Qasper、MS MARCO | citation coverage、answer correctness、abstain |
| Tool choice | tau-bench、ToolSandbox、BFCL、MCP-Bench | tool selection、arg validity、failure handling |
| State diff | Agent-Diff + synthetic enterprise state | final state matches expectation |
| Memory | STATE-Bench、LoCoMo、LongMemEval、EverMemBench、GroupMemBench | recall/update/forget/scope |
| Multi-agent | MultiAgentBench、MARBLE + bounded handoff fixture | handoff correctness、budget containment |
| MCP security | MCPSecBench + poisoned tool fixture | malicious tool/output blocked |
| Policy | JourneyBench + approval fixture | unsafe task refusal、approval wait |

Eval harness 需要产出：

- dataset version。
- adapter version。
- AgentDefinition / SkillPackage version。
- model/provider version。
- run trace refs。
- pass/fail and graded metrics。
- failure taxonomy。
- examples with low-sensitive references。
- regression comparison against previous baseline。

ReplayBundle 需要支持：

- 不重新执行外部副作用。
- 不保留过量 raw prompt / message body。
- 记录 context source refs、model/provider metadata、tool prepare refs、approval refs、execution refs。
- 能重放 read-only answer、proposal draft、memory admission decision、tool selection decision。

### 4.15 Observability / Audit / AgentOps

AgentOps 是 Agent 平台的运维和治理能力，覆盖 tracing、quality、cost、safety、release、incident。

| 维度 | 指标 |
| --- | --- |
| Quality | grounded correctness、citation coverage、abstain rate、memory admission precision |
| Safety | policy deny、approval bypass attempt、MCP poisoning block、PII redaction |
| Reliability | run success、provider timeout、retry/redrive、workflow wait timeout |
| Cost | token、model cost、tool call cost、per-agent budget burn |
| Latency | intake、context build、model call、tool prepare、approval wait、execute |
| Memory | candidate volume、active admission rate、revocation rate、pollution findings |
| Eval | dataset score、regression delta、failure class distribution |
| Release | version adoption、canary failure、rollback |

当前 isolated evidence 已补 operational readiness budget rehearsal，用
fixture-only refs 验证 runtime step、model spend、tool timeout、retrieval latency、
eval retention、canary telemetry 和 incident escalation budget 的 owner、limit、
measurement、operator view、audit、release gate 和 rejection refs。它不代表生产
SLO、真实容量或 on-call 流程已批准。

Trace 需要分层：

- User-visible status：pending、needs approval、answered、failed、cancelled。
- Operator trace：step refs、latency、failure class、model/tool/provider metadata。
- Audit archive：actor, agent, policy decision, approval, execution, memory admission refs。
- Debug replay：低敏 ReplayBundle，不等同于永久 raw transcript。

### 4.16 Security / Privacy / Compliance

Agent 平台的安全目标是 fail closed、least privilege、source-backed、auditable。

必须覆盖：

- Prompt injection：retrieved content、tool output、MCP description、peer-agent response 均是不可信输入。
- Tool poisoning：外部 tool schema / description / output 必须 validation 和 taint tracking。
- Data leakage：ContextPackage 构建前必须完成 permission filtering。
- Memory leakage：group memory 不得泄漏到个人或其他群组。
- Secret handling：Agent Runtime 和 Python Worker 不接触 provider secret 明文。
- PII / retention：模型调用前执行 data classification、redaction、retention policy。
- Approval bypass：高风险动作必须有 approval ref 和 action-executor validation。
- Replay privacy：ReplayBundle 只保留可调试所需的低敏引用和 hash。
- Tenant isolation：AgentDefinition、SkillPackage、Memory、Eval artifact 都必须 tenant-scoped 或明确 public dataset。
- Compliance hold：审计保留由 audit-service 拥有，不由 Agent Runtime 自行归档。

## 5. 草案领域词汇

以下是设计词汇，不是 schema。

| 术语 | 草案含义 | 归属方向 |
| --- | --- | --- |
| AgentDefinition | 可运行 agent 的版本化定义 | agent-service / governance |
| SkillPackage | 可组合技能和工具/检索/prompt policy 包 | skill-registry |
| AgentPrincipal | agent 本次运行的身份主体 | identity / policy |
| AgentSession | 用户或系统与 agent 的交互会话引用 | agent-service |
| AgentRun | 一次可追踪的 agent 运行 | Agent Runtime |
| AgentStep | AgentRun 内的一步低敏 trace | Agent Runtime |
| ContextPackage | 模型输入上下文的权限过滤和引用集合 | retrieval / RAG |
| EvidencePackRef | 可引用证据包引用 | retrieval-gateway / rag-service |
| MemoryCandidate | 待评审记忆候选 | memory-service |
| MemoryRecordRef | ACTIVE memory 的引用 | memory-service |
| ToolIntent | prepare 阶段的工具意图 | mcp-gateway |
| PreparedToolCallRef | 已校验但未必执行的工具准备结果 | mcp-gateway |
| ApprovalWorkflowRef | 审批等待状态引用 | workflow-service |
| ExecutionRef | 已执行或失败的业务动作引用 | action-executor |
| DelegationRequest | bounded multi-agent / A2A 委托 envelope | agent-service / runtime |
| ReplayBundle | 低敏回放包 | ai-eval-service / audit refs |
| EvalCase | 单个公开数据集或 fixture case | ai-eval-harness |
| EvalRun | 一次批量评估运行 | ai-eval-service |
| FailureClass | 统一失败分类 | agent-service / eval |

## 6. 概念性命令

以下命令只描述行为，不是 API 契约。

| 命令 | 行为 | 禁止副作用 |
| --- | --- | --- |
| StartAgentRun | 开始一次 agent run | 不执行业务写动作 |
| CancelAgentRun | 取消未完成认知步骤并传播取消 | 不撤销已执行业务事实 |
| ResumeAgentRun | 从 workflow callback 或 replay point 恢复 | 不绕过审批 |
| ReplayAgentRun | 用 ReplayBundle 重放认知路径 | 不重新执行外部副作用 |
| BuildContextPackage | 构造权限过滤上下文 | 不读取越权数据 |
| SubmitMemoryCandidate | 提交记忆候选 | 不直接 ACTIVE |
| ReviewMemoryCandidate | admission / reject / supersede | 不改变业务事实 |
| PrepareToolIntent | 校验工具意图和参数 | 不执行高风险副作用 |
| RequestApproval | 创建审批请求 | 不隐式批准 |
| ExecuteApprovedAction | 执行已审批动作 | 不重新规划任务 |
| RecordEvalRun | 记录离线评估结果 | 不影响生产 fallback |

## 7. 核心流程

### 7.1 只读问答

```text
Gateway
  -> policy / budget
  -> build ContextPackage / EvidencePack
  -> optional memory retrieve
  -> model answer candidate
  -> citation verifier
  -> final answer or abstain
  -> audit low-sensitive refs
```

通过条件：

- 关键断言有引用。
- evidence 不足时 abstain。
- 不创建 approval、execution 或 ACTIVE memory。

### 7.2 群组 Memory Admission

```text
Group source refs
  -> candidate extraction
  -> speaker attribution
  -> scope / audience check
  -> conflict / supersedes check
  -> review
  -> ACTIVE group memory or rejected candidate
```

通过条件：

- 不能把一个人的偏好升级为群组事实。
- 冲突事实不能静默覆盖。
- revocation 后 retrieval 不再返回旧记忆。

### 7.3 需要审批的业务动作

```text
User request
  -> policy / context
  -> proposal draft
  -> tool prepare / state precheck
  -> approval workflow
  -> action-executor
  -> business service
  -> execution result
  -> audit / user-visible status
```

通过条件：

- proposal 与 EvidencePack 关联。
- approval ref 存在且未过期。
- action-executor 校验 prepare lineage 和 idempotency。

### 7.4 长时间等待审批

```text
Agent Runtime creates proposal
  -> workflow-service owns waiting state
  -> AgentRun pauses with workflow ref
  -> user approves / rejects / times out
  -> callback resumes AgentRun or finalizes failure
```

通过条件：

- Agent Runtime 不持有长 timer。
- 等待期间可 cancel。
- resume 不重新生成不同 proposal，除非显式 redrive。

### 7.5 外部 Tool Provider 超时

```text
Tool prepare request
  -> provider timeout
  -> classify retriable / non-retriable
  -> retry within budget or produce partial failure
  -> no fake fallback
```

通过条件：

- 超时不伪造工具结果。
- retry 受预算约束。
- failure class 进入 eval 和 observability。

### 7.6 Repair / Redrive

Repair 是修复认知或准备阶段，Redrive 是按已知安全边界重跑。二者不能重放外部副作用。

| 场景 | 可 redrive | 不可 redrive |
| --- | --- | --- |
| provider timeout before output | 是 | 无 |
| invalid JSON candidate | 是 | 不能执行工具 |
| policy deny | 否 | 不能重试绕过 |
| approval timeout | 需要新 workflow | 不能自动批准 |
| execution partial success | 由 action-executor 补偿 | Agent 不直接重试 |

### 7.7 Cancel / Resume / Replay

- Cancel：停止未完成 Agent steps，传播到下游 prepare/model call；已进入 workflow / executor 的状态由
  对应服务处理。
- Resume：只能从保存的 safe checkpoint 或 workflow callback 恢复。
- Replay：用于 debug/eval，不执行外部副作用，不向用户发送新业务通知。

### 7.8 Multi-agent Handoff

```text
Coordinator
  -> create DelegationRequest(scope, budget, deadline, artifact)
  -> Specialist produces candidate artifact
  -> Verifier checks source / policy / state diff
  -> Coordinator integrates or rejects
```

通过条件：

- 每个子 agent 有独立 trace。
- 子 agent 不能扩大权限。
- peer 输出必须 source-backed 或被标记为 untrusted。

## 8. 状态所有权矩阵

| 组件 | 拥有什么状态 | 不能拥有什么状态 |
| --- | --- | --- |
| Agent Gateway | request envelope、correlation id、rate-limit counters | EvidencePack body、memory、execution truth |
| Agent Service | AgentDefinition ref、AgentRun metadata、release channel、run summary | raw business facts、tool secret、workflow wait timer |
| Agent Runtime / Harness | ephemeral planner state、step trace、candidate outputs、failure class | approval state、ACTIVE memory、business final state |
| Model Gateway | provider metadata、usage/cost、model policy | planner state、business facts、memory admission |
| Retrieval / RAG | query plan refs、citation map、source coverage | source-of-truth data、ACTIVE memory |
| Memory Service | MemoryCandidate、MemoryRecordRef、scope/version/review state | business final state、unreviewed model inference as fact |
| MCP Gateway | tool registry、provider provenance、prepare refs | actor permission truth、approved execution state |
| Workflow Service | approval wait、timer、timeout、escalation、compensation state | raw prompt、EvidencePack body、planner state |
| Action Executor | execution request/result、idempotency、business outcome refs | model planning、memory extraction |
| Audit Service | immutable low-sensitive audit refs and decisions | arbitrary debug transcript beyond policy |
| AI Eval Service | dataset/eval run/report/replay refs | production fallback decision |
| Python AI Worker | transient candidates and scores | final state、ACTIVE memory、approval、execution、audit archive |

## 9. 开发过程

用户当前要求先不使用真实 IM 数据，而用开源数据集测试 Agent 能力。推荐过程如下：

### Phase 0：文档和边界收口

- 完成本 SDD。
- 明确当前不冻结契约。
- 列出必须验证的能力和风险。

### Phase 1：Open dataset eval harness

- 为 RAG、memory、tool/workflow、state-diff、security、multi-agent 各选 1-2 个公开数据集。
- 写 dataset adapter 草案，只输出统一 EvalCase，不接生产服务。
- 建 synthetic IM-like fixture，用来模拟群组、项目、审批、工具和 memory scope。

### Phase 2：Fixture-only AgentRun trace prototype

- 只在 `ai/experiments/` 或 `docs/research/` 标注实验目录。
- 输出 AgentRun trace、ContextPackage draft、MemoryCandidate draft、ToolIntent draft、ReplayBundle draft。
- 不接生产启动路径。

### Phase 3：Eval gates

- 建立 baseline score 和 failure taxonomy。
- 每个 SkillPackage 必须有最低 eval gate。
- 引入 regression report，防止模型或 prompt 更新破坏记忆、引用或工具安全。

### Phase 4：ADR promotion

只有当 fixture 和 open dataset 能证明边界有效，才把某个方向提升为 ADR：

- Agent Runtime 是否独立服务。
- Memory admission 的最终事件和状态模型。
- EvidencePack / ContextPackage 契约。
- Tool prepare / action execution 契约。
- ReplayBundle / eval report 契约。

当前 isolated evidence 还包含 controlled implementation readiness gate：在缺少
accepted ADR、main integration review、owner review、preservation evidence 或 operator
gate 时，只允许继续 fixture-only hardening，不能进入 controlled implementation 或
production contract 设计。

Architecture coverage rehearsal 还验证所有必需 Agent 架构面都有 owner、SDD /
research / ADR ref、fixture evidence、lifecycle、version、replay、preservation、
audit、operator、eval gate 和 rejection refs；这只是受控实现前的 coverage gate，
不是生产实现授权。

Contract-version compatibility rehearsal 还验证 EvidencePack、ContextPackage、
MemoryCandidate、MemoryClaim、ToolIntent、PreparedToolRef、ApprovalDecision、
ExecutionReceipt、EvalReport 和 ReplayBundle 都有 compatibility window、
replay-reader、redaction、deprecation、migration、preservation、audit、operator、
eval gate 和 rejection refs；这只是生产契约前的治理证据，不冻结字段或 schema。

### Phase 5：最小生产切片

- 从 read-only grounded QA 开始。
- 再做 memory candidate，不直接 ACTIVE。
- 再做 proposal / approval / executor 闭环。
- 最后考虑 bounded multi-agent 和外部 MCP。

## 10. 测试和验证策略

| 测试层 | 目标 | 示例 |
| --- | --- | --- |
| Unit | policy decision、context filter、memory admission rule、tool taint rule | deterministic fixtures |
| Contract | AgentDefinition / SkillPackage / tool registry / eval report 草案一致性 | schema-like validation without production schema |
| Integration | read-only QA、approval wait、tool prepare、memory candidate | synthetic IM-like fixture |
| Eval | RAG、memory、tool、state-diff、security、multi-agent | public datasets |
| Replay | 失败 case 能否低敏重放 | ReplayBundle |
| Security | prompt injection、MCP poisoning、tool-selection attack | malicious fixtures |
| Governance | release / rollback / eval gate | canary-like docs/test flow |

必须优先落地的检查：

- Citation verifier。
- Permission leakage detector。
- Memory pollution detector。
- Tool poisoning detector。
- Capability lease / provider attestation detector。
- Stale PreparedToolRef rejection detector。
- Approval bypass detector。
- State-diff checker。
- Replay completeness checker。

## 11. 失败分类

| FailureClass | 含义 | 默认处理 |
| --- | --- | --- |
| `POLICY_DENIED` | 权限、租户、数据或工具策略拒绝 | fail closed |
| `BUDGET_EXCEEDED` | token、cost、tool call、time 超预算 | fail closed / ask user |
| `INSUFFICIENT_EVIDENCE` | 证据不足 | abstain / clarification |
| `CONFLICTING_EVIDENCE` | 证据冲突 | clarification / human review |
| `PROVIDER_TIMEOUT` | model/tool provider 超时 | bounded retry |
| `PROVIDER_UNSAFE_OUTPUT` | provider 输出不可信或违反格式 | reject / repair if safe |
| `TOOL_POISONING_DETECTED` | tool 描述或输出疑似攻击 | block / security audit |
| `MEMORY_SCOPE_VIOLATION` | memory scope 不合法 | reject candidate |
| `MEMORY_CONFLICT` | 新记忆与旧记忆冲突 | review |
| `APPROVAL_REQUIRED` | 需要审批 | workflow wait |
| `APPROVAL_REJECTED` | 审批拒绝 | finalize rejected |
| `EXECUTION_FAILED` | 执行层失败 | action-executor owns repair |
| `REPLAY_INCOMPLETE` | 回放信息不足 | block promotion |

## 12. SLI / SLO 草案

这些不是生产 SLO，只是设计和离线 gate 草案。

| 指标 | 草案目标 |
| --- | --- |
| Permission leakage | eval 中为 0 |
| Approval bypass | eval 中为 0 |
| Citation coverage | grounded answer 关键断言达到阈值 |
| Abstain correctness | evidence 不足时拒答率达到阈值 |
| Memory admission precision | 错误长期记忆候选拦截率达到阈值 |
| Memory revocation correctness | revocation 后不再检索到 |
| Tool poisoning block rate | malicious fixture 阻断率达到阈值 |
| Replay completeness | failed run 可重放到 failure class |
| State-diff correctness | action outcome 与预期状态变化匹配 |

Operational readiness gate 在第一阶段只检查 fixture evidence：每类 budget 必须
有 owner、limit、measurement、operator view、audit、release gate 和 rejection refs；
missing measurement、over-limit continuation、raw body retention、Python override、
unreviewed capacity 或生产 SLO 授权声明都必须阻断 promotion。生产 SLO 数值、真实
provider capacity、on-call escalation 和 canary telemetry 自动化必须由后续 owner
review 单独批准。

## 13. 风险和反对条件

### 13.1 不应扩展为生产 Agent Runtime 的情况

- 只读 RAG citation / abstain 未通过。
- Memory admission 仍会把错误或越权事实升级为 ACTIVE。
- Tool prepare 与 action execution 只有 fixture 证据，尚未获得
  mcp-gateway / policy / action-executor owner 的生产接受。
- Workflow ownership 与 Runtime ownership 仍重叠。
- ReplayBundle 无法支持调试。
- Eval harness 无法在公开数据集和 synthetic fixture 上稳定复现。

### 13.2 不应新增 agent-runtime-service 的情况

- Agent run 仍只是轻量 read-only QA。
- 还没有高频暂停/恢复/replay需求。
- 没有独立 scaling、failure isolation 或 governance 需求。
- `agent-service` 内模块化足够且不会吸收 workflow/executor 职责。

### 13.3 必须拆分或强化边界的信号

- Runtime 开始持有长等待审批状态。
- `agent-service` 开始包含 tool provider adapter、memory admission、workflow compensation、
  eval runner 的大量实现。
- Python Worker 开始保存最终状态或作为服务 fallback。
- MCP gateway 直接执行高风险副作用。
- Audit 只能看到最终 answer，看不到 policy、tool、memory、approval lineage。

## 14. 推荐推进顺序

1. 继续保留 `agent-service` + runtime module 的探索形态，不急于新增生产服务。
2. 先做 open-dataset eval harness 和 synthetic IM-like fixture。
3. 用 fixture-only AgentRun trace 验证 context、memory、tool、approval、replay。
4. 如果 runtime state、pause/resume/replay、tool/memory orchestration 明显膨胀，再写 ADR
   评估 `agent-runtime-service`。
5. 最小生产切片从 read-only grounded QA 开始，再进入 memory candidate 和 approved action。

## 15. SDD 验收条件

本 SDD 被后续提升为 ADR / implementation 前，至少需要满足：

- Agent / workflow / executor / memory / eval 状态所有权无冲突。
- 每个高风险动作都有 approval / executor / audit 路径。
- Memory 系统有 admission、scope、version、revocation、eval gate。
- MCP / tool 安全边界明确，外部 provider 不被信任。
- Python AI Worker candidate-only 边界未被破坏。
- Open dataset eval plan 能覆盖 RAG、tool/workflow、memory、state-diff、security、multi-agent。
- Replay 能解释失败，而不是只保留最终 answer。

## 16. 参考

- `docs/architecture/agent-plane-initial-design.md`
- `docs/research/agent-plane-redesign-20260701.md`
- `docs/research/agent-current-design-review-20260701.md`
- `docs/research/agent-current-to-target-matrix-20260701.md`
- `docs/research/agent-open-dataset-eval-plan-20260701.md`
- `docs/research/agent-runtime-workflow-ownership-20260701.md`
- `docs/research/agent-ecosystem-research-20260701.md`
- `docs/research/agent-system-complete-scope-20260701.md`
- `docs/sdd/agent-runtime.md`
- `docs/sdd/agent-memory-admission.md`
- `docs/sdd/agent-context-evidencepack.md`
- `docs/sdd/agent-tool-mcp-boundary.md`
- `docs/sdd/agent-eval-replay-harness.md`
- `docs/sdd/agent-governance-agentops.md`
- `docs/sdd/agent-service.md`
- `docs/sdd/memory-service.md`
- `docs/sdd/retrieval-gateway.md`
- `docs/sdd/rag-service.md`
- `docs/sdd/mcp-gateway.md`
- `docs/sdd/action-executor.md`
- `docs/sdd/ai-eval-service.md`
- `docs/sdd/model-gateway.md`
- OpenAI Agents SDK: <https://openai.github.io/openai-agents-python/>
- Google Cloud agentic architecture components:
  <https://docs.cloud.google.com/architecture/choose-agentic-ai-architecture-components>
- LangGraph overview: <https://docs.langchain.com/oss/python/langgraph/overview>
- Model Context Protocol tools specification:
  <https://modelcontextprotocol.io/specification/2025-11-25/server/tools>
- Google Agent2Agent announcement:
  <https://developers.googleblog.com/en/a2a-a-new-era-of-agent-interoperability/>
- Microsoft Agent Framework overview:
  <https://learn.microsoft.com/en-us/agent-framework/overview/>
- Microsoft Foundry tracing:
  <https://learn.microsoft.com/en-us/azure/foundry-classic/how-to/develop/trace-agents-sdk>
- STATE-Bench:
  <https://opensource.microsoft.com/blog/2026/05/19/introducing-state-bench-a-benchmark-for-ai-agent-memory/>
- Agent-Diff: <https://arxiv.org/abs/2602.11224>
- ToolSandbox: <https://aclanthology.org/2025.findings-naacl.65/>
- GroupMemBench:
  <https://www.microsoft.com/en-us/research/publication/groupmembench-benchmarking-llm-agent-memory-in-multi-party-conversations/>
- Databricks State of AI Agents:
  <https://www.databricks.com/resources/ebook/state-of-ai-agents>
- EverMemBench: <https://arxiv.org/abs/2602.01313>
- MCPSecBench: <https://arxiv.org/abs/2508.13220>
