# Complete Agent System Scope for NexusIM

Date: 2026-07-01
Status: Agent Exploration Mode research note; not ADR / SDD / proto / schema /
runtime implementation.

## 1. Purpose

This note answers one question:

> In 2026, what should a complete enterprise Agent system include?

For NexusIM, "complete" does not mean "build every service now". It means the
architecture must know which capability planes exist, who owns state, how the
system is evaluated, and which parts can remain offline experiments until the
evidence is strong.

This document keeps the current exploration boundary:

- no proto, OpenAPI, Kafka schema, migration or production service directory;
- no frozen agent / skill / tool / memory event taxonomy;
- no real IM data in the first evaluation stage;
- Python AI Worker remains candidate-only;
- fake / mock / fixture data is allowed only in research or test isolation.

## 2. 2026 External Signal

The 2026 agent ecosystem converges on several points:

- Agents are no longer only chatbots. They plan, use tools, collaborate, keep
  state and execute multi-step work.
- Runtime is becoming explicit. OpenAI Agents SDK, LangGraph, Google ADK and
  Microsoft Agent Framework all expose state, handoff, tools, guardrails,
  persistence, tracing or orchestration surfaces.
- Tool protocols and peer-agent protocols are separate concerns. MCP is mainly
  a tool/context protocol. A2A-style designs are about peer agent identity and
  inter-agent communication.
- Governance and evaluation decide whether agents reach production. Databricks
  2026 enterprise signals, Foundry / Agent Framework material and agent eval
  papers all point to governance, eval, traceability and safety as platform
  primitives.
- Benchmarks are moving from answer matching to stateful outcomes. tau-bench,
  ToolSandbox, JourneyBench, Agent-Diff, STATE-Bench and MCPSecBench all test
  pieces that simple RAG answer scoring misses.

The implication for NexusIM: the Agent layer should be designed as a governed
execution system with RAG, memory, tools, workflow, audit and eval, not as a
single assistant endpoint.

## 3. Complete System Capability Model

### 3.1 Product Entry and Agent UX

A complete system needs stable user surfaces:

- private chat with an agent;
- group mention / quiet group assistant;
- thread or card-based long task view;
- approval cards;
- progress cards;
- retry, cancel, resume and handoff controls;
- admin console for owner, version, risk, disable and rollout state;
- user-facing "why did the agent say/do this" view.

For NexusIM, group agents should be quiet by default. Complex work should move
into card / thread / run view instead of flooding a group conversation.

### 3.2 Identity, Tenant, Policy and Budget Plane

Production agents need their own identity model:

- agent identity;
- human `on_behalf_of` identity;
- tenant / workspace scope;
- conversation / project / group scope;
- delegated subject and allowed actions;
- risk level;
- token, tool, latency and spend budget;
- policy decision refs.

No agent should borrow a normal user identity silently. High-risk work must be
represented as delegated action with explicit policy and audit lineage.

### 3.3 Agent Definition and Skill Registry

An Agent system needs versioned definitions:

- AgentDefinition: name, owner, purpose, model policy, allowed skills, memory
  scope, release channel, risk profile, disable switch.
- SkillPackage: task class, required evidence, allowed tool intents, approval
  policy, input/output contract, eval suite, rollout rule.
- Prompt / instruction versions: refs and hashes, not untracked strings.
- Tool and memory access grants bound to skill version.

NexusIM should avoid freezing final agent taxonomy now. Skills are the safer
unit to version and evaluate first.

### 3.4 Model Gateway and Provider Plane

The model plane should not be a raw SDK call from each feature. It needs:

- provider routing and model selection;
- structured output validation;
- token / cost / latency controls;
- retry and timeout policy;
- prompt / response low-sensitive hashing;
- provider capability catalog;
- model version and safety policy refs;
- batch eval and offline replay support.

Simple read-only flows can call lower-level model APIs through a controlled
gateway. Long-running agentic work should use a runtime / harness path.

### 3.5 Agent Runtime / Harness Plane

The runtime owns cognitive execution state:

- AgentRun;
- AgentStep;
- plan / critique / verify steps;
- ContextPackage refs;
- checkpoint refs;
- cancellation and resume intent;
- run budgets;
- tool intent before prepare;
- memory candidate refs;
- ReplayBundle refs;
- failure taxonomy.

The runtime should expose deterministic lifecycle hook points, for example:

```text
BeforeContextBuild
BeforeModelCall
AfterModelCandidate
PreToolPrepare
PostToolPrepare
BeforeWorkflowRequest
AfterWorkflowDecision
BeforeMemoryCandidate
BeforeSendMessage
PreCompact
PostCompact
```

Hooks are control points, not hidden business fallback paths.

### 3.6 Context, Retrieval and RAG Plane

RAG must be a governed data-access plane:

- ingestion adapters;
- parsing / OCR / multimodal extraction;
- chunking and source references;
- embedding and sparse / dense indexes;
- retrieval lane selection;
- permission and temporal filtering;
- EvidencePack construction;
- ContextPackage construction;
- citation verification;
- conflict markers;
- abstain / clarification behavior when evidence is missing.

EvidencePack remains the AI read boundary. ContextPackage is only a model input
package derived from authorized evidence.

### 3.7 Memory Plane

Memory is not just vector search. A complete Agent memory plane should separate:

- run memory: ephemeral task state;
- session memory: current interaction history;
- episodic memory: source-backed events;
- semantic memory: consolidated facts;
- procedural memory: user / group preferences and repeated task patterns;
- group memory: conversation or team-level knowledge;
- project memory: durable project facts and decisions;
- profile aggregate: user-level preferences with strict provenance;
- policy facts: governed rules, not model-generated memories.

Every durable memory should pass admission:

```text
candidate
-> source visibility check
-> classification
-> dedupe
-> conflict / supersedes check
-> temporal validity check
-> privacy / profile overgeneralization check
-> review or auto-admit policy
-> ACTIVE / REJECTED / NEEDS_REVIEW
```

Memory eval should measure downstream task improvement and pollution, not only
recall quantity.

### 3.8 Tool, MCP and External Capability Plane

A complete tool plane includes:

- tool registry;
- MCP server registry;
- skill allowlist;
- schema version;
- tool description provenance;
- risk label;
- prepare / dry-run boundary;
- input validation;
- policy precheck;
- output safety gate;
- timeout / retry / circuit breaker;
- sandbox for unknown or high-risk providers;
- audit lineage.

MCP server descriptions, resources and tool outputs must be treated as
untrusted inputs. MCP is not an authorization system.

### 3.9 A2A / Peer Agent Boundary

Peer agents are different from tools. Future A2A-style integration should model:

- peer agent identity;
- capability advertisement;
- trust level;
- tenant and scope;
- budget;
- handoff contract;
- trace lineage;
- result provenance;
- refusal / escalation behavior.

NexusIM should not expose internal AgentRun state as public peer-agent protocol
until identity, policy, audit and budget contracts are clear.

### 3.10 Workflow and Human-in-the-Loop Plane

Agent Runtime should not become a second workflow engine. Workflow owns:

- approval wait;
- user / operator decision;
- timeout;
- external callback;
- repair approval;
- compensation workflow;
- redrive approval;
- operator queue.

The runtime may request a workflow and resume from a workflow decision ref. It
should not store approval decisions locally.

### 3.11 Action Execution Plane

Actions must be executed by a controlled executor, not by the model:

- approved execution request;
- idempotency ledger;
- public business API adapter;
- provider timeout / failure projection;
- bounded retry;
- controlled redrive;
- result refs and hashes;
- audit handoff.

For NexusIM, high-risk actions include sending messages on behalf of a user,
changing group settings, admitting memory, modifying tasks, invoking external
systems and writing business records.

### 3.12 Multi-Agent Coordination Plane

Multi-agent should begin as bounded delegation:

- primary agent remains accountable;
- specialist agents receive scoped evidence only;
- each specialist has limited tools;
- delegation has budget and deadline;
- specialist output is a candidate observation;
- final response / proposal remains owned by the primary agent.

Open-ended group chat between agents should not be a first production shape. It
is hard to audit, hard to replay and expensive to evaluate.

### 3.13 Evaluation, Replay and Benchmark Plane

Eval is part of the architecture:

- offline public-dataset harness;
- scenario fixture adapters;
- model / prompt / skill regression;
- grounded answer eval;
- tool selection eval;
- state-diff eval;
- workflow policy adherence eval;
- memory before/after eval;
- security / prompt-injection eval;
- replay consistency eval;
- failure taxonomy and triage reports.

Every AgentRun should be replayable from low-sensitive refs, hashes and version
metadata. Raw prompts, secrets, raw provider bodies and full private payloads
should not be required for normal replay.

### 3.14 Observability and Audit Plane

A complete system needs traces across:

```text
agent trigger
-> retrieval
-> context build
-> model/provider call
-> tool prepare
-> workflow approval
-> action execution
-> memory admission
-> final response
-> audit
```

Minimum metrics:

- run count by skill, tenant and status;
- failure class;
- provider latency and cost;
- retrieval coverage;
- citation verifier failures;
- tool prepare allow / deny;
- approval approve / reject / timeout;
- execution success / failure;
- memory candidate accepted / rejected;
- replay availability;
- security gate blocks.

Audit should store low-sensitive refs, hashes, decisions and actor lineage. It
should not become a raw prompt archive.

### 3.15 Governance, Release and AgentOps Plane

Agents need product lifecycle management:

- owner and oncall;
- release channel: shadow, beta, production;
- version pinning;
- eval gate requirement;
- rollout percentage;
- kill switch;
- tenant allowlist;
- risk label;
- spend and rate limits;
- incident review;
- rollback;
- deprecation.

In 2026, "AgentOps" should be treated like service operations plus model /
tool / memory operations.

### 3.16 Security, Privacy and Compliance Plane

Security must be systemic:

- prompt injection defense;
- tool poisoning defense;
- MCP server allowlist and attestation;
- output quarantine for untrusted tools;
- sandbox for code / browser / external tools;
- secret isolation;
- least privilege;
- cross-tenant isolation;
- privacy and retention rules;
- legal hold and deletion propagation;
- policy-based redaction;
- audit export.

Security review should happen before external tools or peer agents become
production paths.

### 3.17 Developer Platform and Simulation Plane

The Agent Lab needs developer tooling:

- offline harness;
- dataset adapters;
- scenario simulator;
- fake tool providers;
- fake workflow approvals;
- run trace viewer;
- eval report generator;
- fixture pack;
- reproducible model/provider configs;
- source-to-invariant traceability table.

This is the right place to use open-source datasets before touching real IM
data.

## 4. Open Dataset First Development Process

The first Agent development loop should not use real NexusIM data.

Recommended loop:

```text
open dataset
-> dataset adapter
-> synthetic IM-like scenario fixture
-> ContextPackage / MemoryCandidate / ToolIntent candidate
-> offline Agent Harness
-> trace recorder
-> eval gate
-> report
```

The output of each experiment should include:

- dataset version;
- task id;
- visible evidence;
- generated context package;
- model/provider config;
- agent steps;
- tool intents;
- memory candidates;
- final answer or proposal;
- state diff, if an action is involved;
- eval score;
- failure class.

### 4.1 Dataset Families

| Capability | Candidate Public Datasets / Benchmarks | What To Learn |
| --- | --- | --- |
| Grounded RAG | BEIR, Natural Questions, HotpotQA, Qasper, MS MARCO | evidence selection, citation, abstain and conflict handling |
| Stateful tool use | tau-bench, ToolSandbox, BFCL, MCP-Bench | tool selection, policy, implicit state, milestones |
| Workflow / policy adherence | JourneyBench, tau-bench policy domains | business SOP adherence and dynamic user interaction |
| Enterprise action outcome | Agent-Diff | state-diff based success criteria |
| Memory | STATE-Bench, LoCoMo, LongMemEval, GroupMemBench | whether memory improves later tasks without pollution |
| Multi-agent | MultiAgentBench / MARBLE | bounded delegation, coordination cost, milestone quality |
| MCP / tool security | MCPSecBench, MCP security papers, tool-selection attack fixtures | poisoned tool descriptions, malicious outputs, unsafe server behavior |
| Coding-agent style long work | SWE-bench variants, OpenHands / coding-agent tasks | checkpoint, repair, test loop and run trace quality |

These datasets should be adapted into IM-like scenarios only after the raw task
is understood. For example, tau-bench customer interactions can become "support
thread with policy tools", but the benchmark ground truth should remain
separate from NexusIM product assumptions.

### 4.2 Phase Gates

Phase 0: research scope

- produce capability map and source-to-invariant table;
- no code except small scripts for source cataloging.

Phase 1: offline harness

- fixture-only AgentRun trace;
- dataset adapter interface;
- no production package import;
- no real IM data.

Phase 2: capability baselines

- RAG grounded answer baseline;
- memory admission baseline;
- tool prepare baseline;
- workflow approval baseline;
- replay bundle baseline.

Phase 3: adversarial and stateful eval

- prompt injection / tool poisoning;
- state-diff action tests;
- memory pollution tests;
- cancel / resume / replay tests;
- multi-agent bounded delegation tests.

Phase 4: synthetic IM mapping

- map public tasks into synthetic conversations, groups, tasks and approvals;
- use fake users, fake groups and fake policies;
- still no production service path.

Phase 5: architecture promotion decision

- decide whether Agent Runtime stays as module, becomes service, or remains
  offline harness;
- only then draft ADR / SDD / proto / schema if the user explicitly asks.

## 5. Minimum Complete NexusIM Agent Platform

For NexusIM, the smallest "complete enough" platform is:

```text
Agent Gateway
  -> Agent Runtime / Harness
  -> Context / RAG / EvidencePack
  -> Memory Candidate / Admission
  -> Skill / Tool / MCP Prepare
  -> Workflow Approval
  -> Action Executor
  -> Audit
  -> Eval / Replay
  -> Governance / AgentOps
```

This platform can start as documents and offline experiments. It should not
start as a large production `agent-service`.

## 6. Build / Defer / Avoid

Build first in research:

- complete capability map;
- public dataset selection;
- fixture-only run trace;
- ContextPackage experiment;
- memory admission eval;
- tool prepare and MCP poisoning fixtures;
- state-diff eval;
- replay bundle shape as research data.

Defer:

- production agent-runtime-service;
- production A2A peer agent;
- low-code agent builder;
- full admin console;
- real group memory admission from NexusIM production data;
- automatic external tool execution.

Avoid:

- one giant agent-service owning runtime, workflow, tool execution, memory and
  audit;
- direct model writes to business facts;
- real IM data in early eval;
- MCP server as trust boundary;
- open-ended multi-agent group chat as first production design;
- raw prompt / raw provider body as the replay source of truth;
- memory writes without source-backed admission.

## 7. Source-to-Invariant Summary

| Source Signal | Invariant for NexusIM |
| --- | --- |
| OpenAI Agents SDK / guide separates lower-level API use from managed agent workflows | Keep simple EvidencePack answer path separate from heavy runtime path |
| LangGraph persistence / interrupt / human-in-the-loop patterns | Pause / resume needs durable checkpoint and explicit owner |
| Google ADK with MCP and A2A material | Tools and peer agents are different protocol boundaries |
| Microsoft Agent Framework / Foundry material | Enterprise agent platforms need session state, type safety, telemetry and managed deployment concerns |
| Databricks 2026 State of AI Agents | Governance and eval are production multipliers, not optional dashboards |
| Claude Code hooks / checkpoints / subagents | Lifecycle hooks, scoped subagents and checkpointing are runtime primitives |
| tau-bench / ToolSandbox / JourneyBench | Eval must include user interaction, domain policy, stateful tools and intermediate milestones |
| Agent-Diff | Action success should be validated by state diff, not tool-call similarity |
| STATE-Bench / GroupMemBench | Memory should be evaluated by downstream task improvement and multi-party recall quality |
| MCPSecBench / MCP security analysis | MCP descriptions, resources and outputs are untrusted; provenance and sandboxing are required |

## 8. Immediate Next Report / Experiment

The next safe artifact should be:

```text
docs/research/agent-open-dataset-eval-plan-20260701.md
```

It should choose a first dataset trio:

1. grounded RAG: BEIR or Qasper;
2. tool / workflow: tau-bench or ToolSandbox;
3. memory: STATE-Bench or LoCoMo.

Then it should define a fixture-only harness output format and acceptance
criteria. It should still not use real NexusIM data.

## 9. Reference Links

- OpenAI Agents SDK: <https://openai.github.io/openai-agents-python/>
- OpenAI Agents guide: <https://developers.openai.com/api/docs/guides/agents>
- OpenAI practical guide: <https://openai.com/business/guides-and-resources/a-practical-guide-to-building-ai-agents/>
- LangGraph overview: <https://docs.langchain.com/oss/python/langgraph/overview>
- LangGraph persistence: <https://docs.langchain.com/oss/python/langgraph/persistence>
- LangGraph human-in-the-loop: <https://docs.langchain.com/oss/python/langchain/human-in-the-loop>
- Google ADK: <https://adk.dev/>
- Google MCP / ADK / A2A codelab: <https://codelabs.developers.google.com/codelabs/currency-agent>
- MCP specification: <https://modelcontextprotocol.io/specification/2025-11-25>
- Microsoft Agent Framework: <https://learn.microsoft.com/en-us/agent-framework/overview/>
- Microsoft Foundry Agent Service: <https://learn.microsoft.com/en-us/azure/foundry/agents/overview>
- Databricks 2026 State of AI Agents: <https://www.databricks.com/resources/ebook/state-of-ai-agents>
- Claude Code hooks: <https://code.claude.com/docs/en/hooks>
- tau-bench: <https://arxiv.org/abs/2406.12045>
- ToolSandbox: <https://aclanthology.org/2025.findings-naacl.65/>
- MultiAgentBench: <https://aclanthology.org/2025.acl-long.421/>
- JourneyBench: <https://aclanthology.org/2026.eacl-industry.15/>
- Agent-Diff: <https://arxiv.org/abs/2602.11224>
- STATE-Bench: <https://opensource.microsoft.com/blog/2026/05/19/introducing-state-bench-a-benchmark-for-ai-agent-memory/>
- MCPSecBench: <https://arxiv.org/abs/2508.13220>
- Feishu AI Agent workflow node: <https://www.feishu.cn/hc/en-US/articles/643175485940-use-the-ai-agent-node-in-workflow>
