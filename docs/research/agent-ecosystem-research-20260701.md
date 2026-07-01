# Agent Ecosystem Research Notes for NexusIM

Date: 2026-07-01
Status: Research appendix for Agent Exploration Mode; not ADR / SDD / proto / schema.

## 1. Purpose

This appendix records a broad scan of 2025/2026 agent frameworks, open-source
systems, benchmarks, security work and enterprise reports. It supports the
initial Agent Plane design in `docs/architecture/agent-plane-initial-design.md`.

The goal is not to choose one framework or vendor. The goal is to extract design
pressure that applies to NexusIM:

- what must be a runtime primitive;
- what can remain a document / fixture exploration;
- what belongs behind policy / workflow / executor boundaries;
- what should be evaluated before production.

## 2. Framework / Project Scan

| Project / Platform | Useful Pattern | NexusIM Design Implication | Do Not Copy Blindly |
| --- | --- | --- | --- |
| OpenClaw / MaxClaw | Typed Gateway, request/response/event protocol, device pairing, idempotency, plugin-agnostic core, durable local state | Agent Gateway should be typed and evented; side effects require idempotency; skills/tools must not leak owner-specific command trees into IM channels | Do not copy single-host local state assumptions into distributed NexusIM services |
| Hermes | MemoryProvider lifecycle, prefetch/sync/session switch/pre-compress/delegation hooks, one external provider limit, memory-context fencing | Memory Plane needs runtime lifecycle hooks and context fencing; subagent results should be parent observations, not automatic durable memory | Do not treat provider recall text as trusted current fact |
| Claude Code | Lifecycle hooks, subagents with scoped tools/permissions, checkpoint/compaction, deterministic PreToolUse/PermissionRequest controls | Agent Runtime needs hook points, permission events, checkpoint and bounded subagent delegation | Do not rely on prompt rules where policy/approval must be deterministic |
| OpenAI Agents SDK / Responses API | Managed turns, tools, guardrails, handoffs, sessions, traces and lower-level Responses API for simpler paths | NexusIM can split simple EvidencePack answer paths from heavier runtime/harness paths; traces should flow into eval loops | Do not force every read-only path into heavy multi-agent runtime |
| LangGraph | Graph/state-machine style orchestration with durable nodes and explicit transitions | Good mental model for AgentRun / AgentStep / checkpoint, especially if runtime grows beyond proposal path | Do not turn every business workflow into model graph logic |
| AutoGen / Magentic-One | Orchestrator delegates to specialist agents and revises plan as progress changes | Useful for future bounded multi-agent handoff; primary agent must keep final responsibility | Do not start with free-form agent group chat as production design |
| CrewAI | Role/task/crew abstraction for structured collaboration | Useful as product mental model for skill packages and specialist roles | Roles are not enough; NexusIM needs policy, evidence, approval, audit |
| Semantic Kernel / Microsoft Agent Framework | Enterprise-friendly skills/plugins/connectors and planner patterns | Skill and tool contracts should be versioned, governed and observable | Do not let connector convenience bypass policy-service |
| LlamaIndex Workflows | Data/RAG-centered workflows and retrieval abstractions | Useful for ContextPackage and retrieval lane thinking | Do not let RAG framework own IM visibility or membership facts |
| Dify / Flowise | Visual workflow and app-builder surfaces | Useful inspiration for future admin/operator Agent builder | Do not make visual flow the source of truth before backend boundaries are stable |
| Google ADK | Model/deployment-agnostic agent framework, graph workflows, session state, subagents, built-in eval, MCP/A2A integration | Confirms the need to separate tools via MCP from peer agents via A2A-style identity | Do not conflate MCP tools and A2A peer agents |
| A2A | Cross-agent communication protocol across languages/deployments | Future peer-agent handoff should include identity, capability, budget and audit lineage | Do not expose internal AgentRun state as public peer-agent protocol too early |
| MCP | Standard way to expose tools/resources/prompts to agents | Keep external tools behind mcp-gateway, skill registry, policy, provenance and output safety | MCP server descriptions and outputs are untrusted input |

## 3. Benchmark / Paper Scan

| Paper / Benchmark | What It Tests | NexusIM Design Implication |
| --- | --- | --- |
| tau-bench | Dynamic user-agent conversations with domain APIs and policies | Evaluate rule following, user interaction and final business state, not only answer text |
| ToolSandbox | Stateful tool execution, implicit state dependencies, interactive milestones | Agent eval needs intermediate milestones and tool state checks |
| MultiAgentBench | Collaboration / competition across multi-agent topologies | Defer multi-agent until single-agent replay and policy boundaries are strong; evaluate delegation quality |
| JourneyBench | Business-policy adherence in customer support journeys | Workflow-aware skills beat static prompts for policy-heavy domains |
| Agent-Diff | Enterprise API tasks with sandboxed execution and state-diff success criteria | Add state-diff fixtures for tool/action paths |
| STATE-Bench | Whether agent memory improves realistic enterprise tasks | Memory eval should measure task improvement and pollution, not raw recall |
| MCPSecBench | Secure MCP formalization and MCP attack surface | Treat MCP as a security boundary; evaluate tool poisoning and malicious server behavior |
| ToolHijacker / tool-selection attacks | Prompt injection manipulates tool selection via malicious tool docs | Tool descriptions are untrusted; tool selection must be allowlisted and policy-checked |
| Long-term memory benchmarks such as LoCoMo / LongMemEval / BEAM | Multi-session memory, temporal reasoning, multi-hop recall | Profile / group / project memory need separate temporal and multi-hop eval |

## 4. Enterprise Report / Product Practice Scan

| Source Type | Common Signal | NexusIM Implication |
| --- | --- | --- |
| Databricks State of AI Agents 2026 | Enterprises move from chatbot pilots toward governed multi-agent systems; evaluation/governance increases production success | Eval and governance are architecture, not post-launch dashboards |
| OpenAI agent guide / platform docs | Agents need tools, guardrails, handoffs, tracing and clear use-case selection | Use heavy runtime only where orchestration is needed; keep simple paths simple |
| Claude Code docs | Hooks, permissions, checkpointing and subagents are explicit surfaces | Runtime lifecycle needs deterministic control points |
| Google ADK / A2A material | Production systems span languages, teams and deployment targets | Peer-agent protocol must be explicit and audited |
| Microsoft AutoGen / Magentic-One / STATE-Bench | Orchestrators delegate, memory should improve tasks, agent frameworks need scale primitives | Bounded orchestration + memory eval before open multi-agent |
| Feishu Aily / workflow AI nodes | Agents are embedded in enterprise workflow and collaboration UX | NexusIM Agent UX should use cards, approvals, progress and workflow nodes |
| DingTalk Agent OS / collaboration reports | Enterprise agents need unified entrance, enterprise search, workflow, admin and operations | Agent Control / Governance plane is a product requirement |
| Slack / Teams / Copilot-style products | Agents appear in channels, threads and work surfaces | Group agents should be quiet by default and use thread/card progress |
| Observability / governance reports | Production agents require traceability across data, tools, decisions and actions | AgentRun traces must connect retrieval, model, MCP, workflow, executor and audit |

## 5. Cross-Cutting Findings

### 5.1 Runtime Is a Product Boundary

Across frameworks, a real agent runtime tends to own:

- turn/session state;
- tool execution boundary;
- guardrails;
- handoff / delegation;
- tracing;
- checkpoint / resume;
- cost and budget.

NexusIM should not hide this inside prompt templates. If the runtime remains
inside `agent-service`, it needs a clear module boundary and promotion trigger.

### 5.2 Workflow Is Not the Same as Agent Runtime

Workflow engines are good at waiting, approvals, callbacks, timers and
compensation. Agent runtimes are good at context, planning, tool-intent,
candidate generation and replay. Combining both into one service creates either
a model-aware workflow engine or a workflow-aware agent monolith.

NexusIM should keep:

```text
Agent Runtime = cognitive execution state
workflow-service = human/operational long-wait state
action-executor = final mutation attempts
```

### 5.3 Tool Use Needs Provenance, Not Just Schema

Tool schema alone is insufficient. Tool selection is vulnerable to malicious
descriptions, polluted docs and unsafe provider output. NexusIM should track:

- tool source;
- owner service;
- schema version;
- description/document version;
- risk label;
- policy decision;
- output safety status;
- audit lineage.

### 5.4 Memory Must Be Evaluated by Downstream Behavior

Memory value is not "how much context was recalled". Value is whether the agent
does better on later tasks without leaking, overgeneralizing or using stale
facts. NexusIM should distinguish:

- run memory;
- conversation memory;
- project memory;
- profile aggregate;
- policy facts.

Each should have separate admission and eval rules.

### 5.5 Multi-Agent Should Start as Bounded Delegation

Open multi-agent discussion is expensive and hard to debug. Initial multi-agent
support should be:

- primary agent remains accountable;
- specialists have limited tools and context;
- handoff has budget, scope and trace id;
- specialist output is candidate evidence, not final action authority.

## 6. Recommended Updates to NexusIM Design

1. Keep EvidencePack-first baseline for read-only and proposal-only flows.
2. Promote `ReplayBundle` and state-diff eval as early exploration targets.
3. Treat `mcp-gateway` as both prepare boundary and tool provenance/security
   boundary.
4. Keep `workflow-service` focused on long-wait operational state.
5. Use Agent Runtime / Harness for context, planning, checkpoint, replay and
   bounded delegation.
6. Keep Python AI Worker as candidate-only, including planner/memory/rerank/eval
   candidates.
7. Defer real A2A-style peer agents until identity, audit and budget contracts
   are clear.

## 7. Proposed Research Backlog

These are exploration items only:

- fixture-only AgentRun trace model;
- ContextPackage builder from fake EvidencePack;
- state-diff eval fixtures for approved conversation note creation;
- MCP poisoning fixtures for malicious tool description and unsafe tool output;
- memory before/after task suite for project and group memory;
- bounded subagent handoff fixture with primary-agent accountability;
- governance matrix for AgentDefinition / SkillPackage owner, version,
  release channel, risk profile and disable switch.

## 8. Source Notes

Inputs reviewed or rechecked on 2026-07-01 include:

- local OpenClaw architecture and AGENTS policy files;
- local Hermes memory provider and manager source;
- Claude Code hooks and subagent docs;
- OpenAI Agents SDK / agent guide materials;
- Google ADK / MCP / A2A codelabs and posts;
- Microsoft AutoGen / Magentic-One / STATE-Bench materials;
- LangChain and Langfuse framework comparisons;
- tau-bench, ToolSandbox, MultiAgentBench, JourneyBench, Agent-Diff,
  STATE-Bench, MCPSecBench and tool-selection attack materials;
- Databricks 2026 State of AI Agents and enterprise observability/governance
  reports;
- Feishu Aily / AI Agent workflow node and collaboration product practices.
