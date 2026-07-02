# Agent SDD Interview Readiness Review

Date: 2026-07-02

Status: SDD review for interview-ready architecture closure. This is not an
ADR, proto, OpenAPI, Kafka schema, migration, service directory, production
runtime, release pipeline or request to expand the Agent architecture.

## Verdict

Pass for interview-ready completeness with moderate production-grade awareness.

The current SDD package is broad enough to explain a full Agent layer and
constrained enough to avoid becoming an unrealistic production build. No P0/P1
gap was found that requires reopening broad architecture design.

The SDDs should now be treated as a stable explanation and implementation guide
for a backend-isolated demo. Further work should not add new Agent planes,
service candidates, object families, taxonomies or benchmark surveys unless a
reviewer returns a concrete P0/P1.

## Review Standard

The SDD package is judged by four criteria:

| Criterion | Expected Bar |
| --- | --- |
| Interview completeness | Covers the full story: request, context, memory, tool intent, approval, eval, replay and governance |
| Moderate production awareness | Shows owners, failure modes, audit, rollback and production blockers without requiring full production integration |
| Boundary clarity | Keeps Agent outside SendMessage hot path and prevents Python/LLM from owning final business state |
| Demo readiness | Can guide a small backend-isolated command that outputs low-sensitive refs and an EvalReport / ReplayBundle |

## External Alignment

| External signal | Relevant lesson | Current architecture alignment |
| --- | --- | --- |
| OpenAI practical agent guide | Agent systems need model, tools, instructions, orchestration, guardrails and evals; complexity should be introduced incrementally | Runtime, ToolIntent, guardrails through policy/approval, Eval/Replay and scope closure align |
| Anthropic effective agents guidance | Prefer simple, composable workflows before over-building multi-agent systems | Scope closure and bounded multi-agent handoff align |
| Microsoft Agent Framework | Production agents need orchestration, state, workflows and observability | Runtime checkpoints, workflow boundary, replay and AgentOps align |
| Databricks State of AI Agents 2026 | Governance and evaluation materially affect enterprise production adoption | Eval/Replay gates and AgentOps release governance align |
| MCP specification and NSA MCP security guidance | MCP/tool surfaces need explicit security boundaries; tool metadata and provider behavior cannot be treated as authority | Tool/MCP SDD treats provider, description, prompt/resource and output as untrusted |
| tau-bench and ToolSandbox | Tool agents must be evaluated on interaction, policy, stateful tool use and final state change | Tool/MCP, state-diff and approval/action fixture evidence align |
| LongMemEval and LoCoMo | Long-term memory needs extraction, update, temporal reasoning, abstention and retrieval quality, not only vector recall | Memory Admission SDD covers source-backed candidate, review, supersession, revocation and retrieval eligibility |

## SDD Review Matrix

| SDD | Interview Role | Review Result | Keep / Fix |
| --- | --- | --- | --- |
| `agent-platform.md` | Big-picture map of the Agent plane and how all parts fit together | Good as overview, too broad as implementation authority | Keep as orientation only; do not use it to authorize production implementation |
| `agent-runtime.md` | Explains AgentRun, AgentStep, checkpoint, budget, cancel/resume/replay and workflow split | Strong. It prevents Runtime from becoming workflow-service or executor | Use as the main runtime story for the demo |
| `agent-context-evidencepack.md` | Explains how Agent reads context without private table access | Strong. EvidencePack as AI read boundary is a clean interview point | Demo should show denied/unavailable lanes and citation refs |
| `agent-memory-admission.md` | Explains why memory is governed state, not prompt/vector cache | Strong. Source-backed, scoped, reviewable and revocable memory is the right story | Demo should create MemoryCandidate only, not ACTIVE memory |
| `agent-tool-mcp-boundary.md` | Explains safe tool use, MCP distrust, prepare and action-executor handoff | Strong. Capability lease, attestation, taint and stale-prepare rejection are interview-worthy | Demo should include safe tool intent and one blocked unsafe provider/output case |
| `agent-eval-replay-harness.md` | Explains how the Agent is tested, scored and replayed before production integration | Strong. EvalReport and ReplayBundle make the project credible | Demo should end by writing both artifacts |
| `agent-governance-agentops.md` | Explains release gate, owner, kill switch, rollback and incident control | Good production-awareness layer; too much for a first demo if implemented literally | Keep as roadmap / release story; implement only lightweight fixture refs in demo |

## Per-SDD Interview Notes

### Agent Platform

Use it to answer: "What is the overall architecture?"

Best talking point:

```text
The Agent plane is not one monolithic service. It is a set of boundaries:
runtime, context, memory, tool, workflow, executor, eval/replay and governance.
```

Limit:

`agent-platform.md` is intentionally broad. In an interview, do not walk through
every object. Use it as a map, then go into the six focused SDDs.

### Runtime / Harness

Use it to answer: "Who owns an Agent run?"

Best talking point:

```text
Runtime owns cognitive run state and checkpoints. workflow-service owns long
approval waits. action-executor owns side effects.
```

Demo obligation:

Show `AgentRun` / `AgentStep` refs, a checkpoint before approval/action, and a
ReplayBundle that does not re-execute side effects.

### Context / EvidencePack

Use it to answer: "How does the Agent read IM context safely?"

Best talking point:

```text
Agent does not read message tables. It consumes EvidencePack refs produced by
retrieval/RAG with policy, visibility and source coverage.
```

Demo obligation:

Show visible evidence refs, denied lane refs, citation refs and a
ContextPackage derived from those refs.

### Memory Admission

Use it to answer: "How do you prevent memory pollution?"

Best talking point:

```text
Python can propose memory, but memory-service owns ACTIVE admission, review,
revocation and retrieval eligibility.
```

Demo obligation:

Show a source-backed `MemoryCandidate`, a rejected/needs-review reason, and no
ACTIVE memory write.

### Tool / MCP

Use it to answer: "Why not let the LLM directly call tools?"

Best talking point:

```text
Tools are capabilities, not authority. MCP provider descriptions and tool
outputs are untrusted until prepared, scoped, tainted and audited.
```

Demo obligation:

Show `ToolIntent`, `PreparedToolRef` or blocked prepare refs, approval
requirement and no direct Runtime execution.

### Eval / Replay

Use it to answer: "How do you know the Agent is safe and improving?"

Best talking point:

```text
Eval and replay are platform capabilities, not scripts after launch. A run must
produce low-sensitive evidence that explains quality, permission, memory, tool
and state-diff behavior.
```

Demo obligation:

One command should output `EvalReport` and `ReplayBundle` with failure classes
and blocked promotion reasons where appropriate.

### AgentOps / Governance

Use it to answer: "How would this be operated in production?"

Best talking point:

```text
AgentOps controls release, rollback, kill switch, baseline approval and failure
owners. It does not own runtime, workflow, execution, memory or audit truth.
```

Demo obligation:

Keep it lightweight: include release-gate fixture refs and a blocked-promotion
reason, not a full admin console.

## Cross-Cutting Fit

| Concern | Current Coverage | Interview Judgment |
| --- | --- | --- |
| SendMessage hot path isolation | Agent consumes async refs; no Agent work in `repository_append` path | Good and important to mention |
| Python boundary | Python produces candidates and eval artifacts only | Good |
| Real data isolation | Synthetic/public-dataset-style fixtures only | Good for demo |
| Tool safety | Prepare, attestation, taint, approval and executor owner are present | Good |
| Memory safety | Admission, review, revocation and retrieval eligibility are present | Good |
| Replayability | Runtime, Eval and L2 docs preserve replay refs | Good |
| Production honesty | 12 production gap categories are documented | Good |
| Scope control | Scope-closure doc prevents architecture sprawl | Good |

## Findings

| Severity | Finding | Action |
| --- | --- | --- |
| P0 | None | No broad architecture reopen needed |
| P1 | None inside interview / fixture scope | Continue to demo implementation |
| P2 | SDD package is too large to narrate linearly in an interview | Use this review as the spoken outline |
| P2 | AgentOps is production-oriented and could distract from demo | Keep AgentOps in demo as release-gate refs only |
| P2 | Platform SDD remains too broad to be implementation authority | Continue relying on focused SDDs and scope closure |

## Recommended Interview Story

The interview version should be told in this order:

1. IM core stays responsible for trusted messaging and SendMessage performance.
2. Agent runs asynchronously from low-sensitive refs after message commit.
3. EvidencePack gives the Agent safe context without private-table reads.
4. Runtime builds ContextPackage, produces answer/proposal and tracks replay.
5. Memory is candidate-only until memory-service admission.
6. Tools are prepared and approved before action-executor can execute.
7. EvalReport and ReplayBundle prove quality, safety and debuggability.
8. AgentOps explains how production would release, rollback and kill the Agent.
9. Production integration gaps are documented as future work, not hidden debt.

## Demo Acceptance Criteria

The next demo is sufficient when one command can show:

- synthetic IM-like request;
- SendMessage / MessageCommitted refs, but no Agent work in hot path;
- EvidencePack and ContextPackage refs;
- one MemoryCandidate with admission reason;
- one ToolIntent/proposal with approval/action fixture refs;
- one unsafe or insufficient-evidence case blocked;
- EvalReport with metrics and failure classes;
- ReplayBundle with low-sensitive reconstruction refs;
- README explaining boundaries and production roadmap.

## Decision

Do not deepen the SDDs before the demo.

The current SDD package is complete enough for an interview-ready project and
moderate production-grade discussion. The next high-value step is to implement
the backend-isolated demo runner and a concise demo README.

## References

- OpenAI, "A practical guide to building agents":
  <https://openai.com/business/guides-and-resources/a-practical-guide-to-building-ai-agents/>
- Anthropic, "Building Effective AI Agents":
  <https://www.anthropic.com/engineering/building-effective-agents>
- Microsoft Agent Framework overview:
  <https://learn.microsoft.com/en-us/agent-framework/overview/>
- Databricks, "State of AI Agents" 2026:
  <https://www.databricks.com/resources/ebook/state-of-ai-agents>
- Model Context Protocol specification:
  <https://modelcontextprotocol.io/specification/2025-06-18>
- NSA, "Model Context Protocol (MCP): Security Design Considerations for
  AI-Driven Automation":
  <https://www.nsa.gov/Portals/75/documents/Cybersecurity/CSI_MCP_SECURITY.pdf>
- tau-bench:
  <https://arxiv.org/abs/2406.12045>
- ToolSandbox:
  <https://machinelearning.apple.com/research/toolsandbox-stateful-conversational-llm-benchmark>
- LongMemEval:
  <https://arxiv.org/abs/2410.10813>
- LoCoMo:
  <https://arxiv.org/abs/2402.17753>
