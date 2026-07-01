# Agent Context / EvidencePack SDD

Date: 2026-07-01
Status: Detailed SDD draft for Agent Exploration Mode. This is not an ADR,
proto, OpenAPI, Kafka schema, migration, production service directory or
EvidencePack contract.

## 1. Goal

Define how Agent Runtime obtains grounded, permission-filtered context from
retrieval-gateway, RAG and memory without directly reading business state.

EvidencePack remains the AI read boundary. ContextPackage is a runtime-facing
model input package derived from EvidencePack and other authorized refs. Neither
is a business fact source.

## 2. Non-Goals

- Do not freeze EvidencePack or ContextPackage schema.
- Do not bypass retrieval-gateway, RAG or service-owned APIs.
- Do not let Agent Runtime directly query search, vector, memory or business
  service private tables.
- Do not use retrieved content as trusted instruction.
- Do not use real NexusIM IM data in first-stage eval.

## 3. Component Responsibilities

`retrieval-gateway` owns:

- query lane orchestration across search, memory, vector and allowed sources;
- permission and temporal filtering;
- EvidencePack construction;
- source coverage metadata;
- visibility, projection and source version refs;
- fail-closed behavior on policy or lane uncertainty.

`rag-service` owns:

- grounded read-only answer path;
- citation verifier and extractive/default answer behavior;
- abstain and conflict handling for simple QA;
- optional verifier lane for Runtime-generated candidates.

Agent Runtime owns:

- ContextPackage request profile;
- task-specific context selection from authorized refs;
- model-facing context layout;
- conflict and missing-evidence decision handling;
- candidate verification before answer/proposal.

`memory-service` owns:

- memory retrieval eligibility;
- memory scope/version/revocation checks;
- memory refs and supporting source refs.

## 4. State Ownership

| State | Owner | Notes |
| --- | --- | --- |
| Source truth | Owning business/search/memory service | Not Agent Runtime |
| EvidencePack | retrieval-gateway | AI read boundary |
| ContextPackage | Agent Runtime | Derived model input package |
| Citation map | retrieval-gateway / RAG / Runtime verifier | Must map claims to source refs |
| Conflict markers | retrieval/RAG/Runtime | Used to abstain or clarify |
| Source coverage metrics | retrieval-gateway | Missing lanes must be visible |
| Memory refs | memory-service | Runtime can consume eligible refs |
| Replay refs | Runtime / eval | Low-sensitive refs and hashes |

Context / Evidence layer cannot own:

- business final state;
- memory admission outcome;
- tool execution result;
- workflow approvals;
- provider secrets;
- raw prompt archive as audit source.

## 5. ContextPackage Concept

These are conceptual fields, not schema:

```text
ContextPackage
  context_package_ref
  agent_run_ref
  evidence_pack_ref
  retrieval_profile_ref
  selected_source_refs
  selected_snippet_refs
  memory_record_refs
  tool_result_refs
  source_coverage
  citation_candidates
  conflict_markers
  temporal_notes
  permission_notes
  abstain_recommendation
  taint_labels
  token_budget_summary
```

The package should distinguish:

- user request;
- system / tenant / admin instruction;
- retrieved evidence;
- memory context;
- tool output;
- peer-agent output;
- prior checkpoint refs.

## 6. Retrieval Lanes

| Lane | Purpose | Required Guard |
| --- | --- | --- |
| Conversation / message refs | Source-backed IM context | Membership window and retention policy |
| Project / artifact refs | Project knowledge | Project permission and temporal version |
| Knowledge base | Docs, policies, manuals | Document ACL and freshness |
| Memory | Personal/group/project memory | Scope, revocation, source lineage |
| Tool documentation | Tool schema/docs | Provenance and taint label |
| Prior run refs | Continuity/replay | Same actor/scope and retention |

If a lane is unavailable or denied, ContextPackage must expose coverage gap. It
must not pretend the lane had no relevant facts.

## 7. Key Flows

### 7.1 Read-Only Answer

```text
AgentRun request
-> retrieval profile
-> EvidencePack
-> ContextPackage
-> model or RAG answer candidate
-> citation verifier
-> answer, abstain or clarification
```

No answer should be finalized unless key claims are grounded or the response is
explicitly non-factual.

### 7.2 Proposal Draft

```text
request
-> EvidencePack
-> ContextPackage with action-relevant facts
-> proposal candidate
-> tool prepare refs
-> approval requirement
```

Proposal must carry evidence refs and cannot rely on ungrounded model inference
for facts that drive side effects.

### 7.3 Memory-Aware Context

```text
EvidencePack
-> memory retrieval by scope/version
-> ContextPackage labels memory as memory, not current source truth
-> model answer/proposal
-> verifier checks source and memory lineage
```

Memory can guide context, but current source-of-truth facts override stale
memory.

### 7.4 Tool Output In Context

Tool output enters context only after:

- source/provenance label;
- schema validation;
- output tainting;
- prompt-injection quarantine where needed;
- policy check for reuse.

Tool output is never a system instruction.

## 8. Failure Semantics

| Failure | Behavior |
| --- | --- |
| Permission uncertainty | Fail closed; no source exposure |
| Lane unavailable | Record coverage gap; abstain if needed |
| Evidence conflict | Mark conflict and ask clarification/review |
| Evidence stale | Prefer current source; mark temporal note |
| Citation missing | Reject factual candidate or repair |
| Source revoked | Remove from eligible context |
| Memory revoked | Do not include as active memory |
| Tool output unsafe | Quarantine and block as context |
| Token budget exceeded | Deterministic truncation by source priority, not arbitrary cut |

## 9. Security Boundary

ContextPackage must enforce:

- permission filtering before model context;
- source labels on all retrieved content;
- instruction hierarchy so retrieved text/tool output cannot override system or
  policy instructions;
- taint tracking for tool output and peer-agent output;
- redaction before model call when policy requires it;
- no raw secret or provider credential in context;
- no hidden fallback from stale local cache.

Prompt injection defenses apply to every external or retrieved text segment.

## 10. Eval / Replay

Required eval:

- citation coverage;
- answer correctness;
- abstain correctness;
- permission leakage;
- source coverage gap reporting;
- conflict handling;
- temporal/superseded fact handling;
- memory-vs-source precedence;
- unsafe tool output quarantine;
- ContextPackage replay completeness.

Public datasets:

- Qasper, HotpotQA, BEIR, Natural Questions and MS MARCO for grounded RAG;
- synthetic permission windows and source coverage fixtures;
- memory fixtures from LoCoMo/STATE-Bench/GroupMemBench style cases.

Replay should reconstruct why a source was selected, rejected, hidden or marked
as conflict without loading raw private archives.

## 11. Observability / Audit

Metrics:

- retrieval lane success/failure;
- source coverage by lane;
- citation coverage;
- abstain rate and correctness;
- permission denial count;
- stale/superseded source hits;
- conflict marker count;
- unsafe context blocks;
- token budget truncation decisions;
- replay completeness.

Audit refs:

- retrieval request ref;
- EvidencePack ref;
- ContextPackage ref;
- source refs and visibility versions;
- memory refs and versions;
- verifier result;
- final answer/proposal refs.

## 12. Risks / Rejection Conditions

Reject Context/EvidencePack promotion if:

- Runtime can directly query private source tables;
- ContextPackage can include unauthorized evidence;
- answer candidates can bypass citation verification;
- missing retrieval lanes are invisible;
- tool output is inserted as trusted instruction;
- revoked memory/source refs remain usable;
- replay needs raw prompt or full message archive.

## 13. Promotion Conditions

Promote only after:

- grounded RAG eval passes citation/abstain thresholds;
- permission leakage is zero in synthetic fixtures;
- conflict and stale source cases produce correct behavior;
- ContextPackage can be replayed from low-sensitive refs;
- integration with memory-service and mcp-gateway preserves taint/source labels.

## 14. Current Isolated Fixture Coverage

Current first-stage code is fixture-only and lives under
`ai/python/nexusim_ai_eval/`,
`ai/python/fixtures/agent_eval/synthetic_context_evidence_scenarios.json` and
`ai/python/fixtures/agent_eval/synthetic_context_evidence_hardening_scenarios.json`
and
`ai/python/fixtures/agent_eval/synthetic_context_evidence_deeper_hardening_scenarios.json`.
It does not freeze a production EvidencePack or ContextPackage schema.

Implemented checks:

- source coverage refs must include required evidence refs;
- conflicting evidence refs must be marked as detected;
- stale evidence refs must not be used or cited;
- permission-driven abstain can pass without exposing forbidden refs;
- memory-vs-current-source precedence is scored when memory conflicts with
  fresher source truth;
- unsafe tool output refs must be quarantined before context reuse;
- context-budget retention must preserve required high-priority refs;
- unavailable retrieval lanes must be reported as explicit coverage gaps;
- source ranking and deterministic tie-break refs must match expected order;
- retrieval lane redrive refs must exist when lane repair is required;
- snippet-level citation repair must keep precise snippet refs and reject
  partial-source ambiguity;
- denied lanes, including cross-tenant lanes, must be reported without exposing
  denied source refs;
- provider, tool and peer-agent taint labels must propagate through context
  assembly;
- public RAG adapter samples preserve ContextPackage / EvidencePack alignment
  refs for ranking/citation repair across Qasper / HotpotQA / BEIR-like
  fixture cases;
- rerank confidence threshold refs and rerank explanation refs must be recorded
  when public RAG adapter samples require them;
- denied-lane audit refs must be recorded without exposing denied source refs;
- taint vocabulary refs must align with provider / tool / peer-agent labels;
- trace metadata includes low-sensitive refs only.

Remaining production hardening:

- audit-retention policy interaction for denied-lane refs after audit-service
  contract promotion;
- larger public RAG datasets before any production EvidencePack /
  ContextPackage schema is frozen;
- integration proof that memory-service, mcp-gateway and peer-agent adapters
  preserve the same taint vocabulary through real service boundaries.

## 15. References

- `docs/sdd/retrieval-gateway.md`
- `docs/sdd/rag-service.md`
- `docs/sdd/memory-service.md`
- `docs/sdd/agent-runtime.md`
- `docs/research/agent-open-dataset-eval-plan-20260701.md`
- ToolSandbox: <https://aclanthology.org/2025.findings-naacl.65/>
