# Agent Context / EvidencePack L2 Scoped Implementation Design

Date: 2026-07-02

Status: L2 scoped implementation design draft for the Context / EvidencePack /
RAG boundary. This is not an accepted production ADR, proto, OpenAPI, Kafka
schema, migration, service directory, startup path, EvidencePack schema,
ContextPackage schema, retrieval contract or runtime implementation.

## Verdict

Conditionally passed as the third L2 scoped design draft.

Rejected for implementation until main integration, retrieval-gateway, RAG,
Agent Runtime, memory-service, mcp-gateway, audit/security, product/operator
and SRE/incident owners review the design and approve the required L3
real-service smoke plan.

Reason: L1 accepted the Context / EvidencePack candidate for reviewability
only. The L2 question is narrower: how can a future controlled implementation
prove that Agent answers, proposals and tool decisions are grounded in
authorized, replayable and inspectable evidence without freezing body schemas or
letting Agent Runtime bypass source owners.

## Scope

The scoped slice is the AI read boundary for Context / EvidencePack / RAG.

It covers:

- EvidencePackRef and ContextPackageRef ownership boundaries;
- SourceVisibilityVersion, EvidenceCoverageReport, DeniedLane, ConflictSet,
  TaintLabel, CitationMap and CitationVerifierResult responsibilities;
- retrieval lane coverage, denied/unavailable/expired lane behavior and
  fail-closed semantics;
- memory, tool output, peer-agent output and provider text reuse rules;
- redaction, retention and replay-reader policy refs;
- operator inspect-and-act surfaces for evidence failures;
- L3 real-service smoke requirements.

It does not cover:

- final EvidencePack or ContextPackage field shape;
- retrieval-gateway, RAG, memory, MCP or audit API design;
- search, vector, memory, object store or message-service query plans;
- production database tables or indexes;
- production data migration;
- production model/provider integration;
- real NexusIM IM data;
- large public benchmark threshold setting;
- production UI copy for denied-lane messages.

## Boundary Thesis

EvidencePack is the AI read boundary. ContextPackage is a derived runtime input
package. Neither is a business fact source.

```text
retrieval-gateway owns:
  source/lane orchestration, permission and temporal filtering, source
  visibility refs, denied/unavailable/expired lane refs, coverage reports and
  EvidencePack refs.

Agent Runtime owns:
  task-specific ContextPackage selection from authorized refs, context layout,
  taint-aware model input construction, candidate verification orchestration and
  replay refs.

RAG owns:
  grounded read-only answer/verifier behavior and citation verification refs for
  supported, unsupported, stale, conflicting or insufficient claims.
```

The integration rule is:

```text
Agent Runtime may request evidence.
retrieval-gateway may return low-sensitive evidence refs and authorized content
refs.
Runtime must not read private source tables directly.
Missing, denied or expired lanes must be visible as low-sensitive refs.
Grounded final answers and action-driving proposals require verifier refs.
```

## Proposed Ownership

| Object / State | Owner | Cannot Be Owned By |
| --- | --- | --- |
| Source truth | Owning business/search/memory service | Agent Runtime, Python worker |
| RetrievalProfileRef | retrieval-gateway plus requesting Runtime policy | Python worker |
| EvidencePackRef | retrieval-gateway | Agent Runtime as source truth |
| ContextPackageRef | Agent Runtime | retrieval-gateway as model input owner |
| SourceVisibilityVersion | retrieval-gateway / policy-integrated source service | model output, Python worker |
| EvidenceCoverageReport | retrieval-gateway / ai-eval reader | Agent Runtime alone |
| DeniedLane / UnavailableLane / ExpiredLane refs | retrieval-gateway | ContextPackage body |
| CitationMap | retrieval-gateway, RAG or Runtime verifier | model self-certification |
| CitationVerifierResult | RAG / Runtime verifier boundary | model output, Python worker |
| ConflictSet | retrieval-gateway, RAG or Runtime verifier | final answer without review |
| TaintLabel / vocabulary version | retrieval-gateway, mcp-gateway, Runtime | unversioned free text |
| Memory eligibility refs | memory-service | Agent Runtime |
| Tool output provenance refs | mcp-gateway / action-executor owner | Agent Runtime as trusted source |
| RedactionPolicyRef | audit/security owner | retrieval fixture |
| ReplayReaderPolicyRef | audit/security plus Runtime/eval reader owner | raw payload archive |
| Operator EvidenceInspectView | product/operator plus audit/security owner | Python worker |

Python remains fixture/eval/candidate-only. It can generate synthetic reports
and public-dataset-style candidates in isolated tests, but it cannot own source
truth, visibility decisions, citation verifier authority, ContextPackage
production state, redaction policy, operator override or audit archive.

## Non-Owner State

Agent Runtime must not own:

- private source table reads;
- source-of-truth visibility decisions;
- permission or projection truth;
- EvidencePack source truth;
- denied-lane body content;
- source deletion/expiry truth;
- memory ACTIVE state;
- tool output trust decision;
- audit archive of record;
- raw prompt, full IM message archive, raw provider body or raw MCP payload as
  normal replay material.

retrieval-gateway must not own:

- model prompt construction beyond allowed evidence packaging;
- final answer or proposal approval;
- action execution;
- memory admission;
- workflow approval;
- model/provider output truth;
- operator release decision.

RAG / verifier must not own:

- raw source truth;
- visibility policy truth;
- business mutation;
- memory admission;
- audit archive;
- release gates.

workflow-service and action-executor must not use ContextPackage content as
authorization. They may consume approved refs only through their own owner
boundaries.

## Candidate L2 Flow

```text
AgentRun request
-> Runtime selects retrieval profile refs and risk profile
-> retrieval-gateway performs policy, projection and temporal filtering
-> retrieval-gateway records searched, used, denied, unavailable and expired lane refs
-> retrieval-gateway returns EvidencePack refs and authorized evidence refs
-> Runtime builds ContextPackage from allowed refs, memory eligibility refs and taint labels
-> model/RAG creates answer or proposal candidate
-> CitationVerifierResult checks claims, citations, conflicts, staleness and coverage
-> Runtime finalizes answer, abstains, asks clarification or blocks proposal
-> audit-service records low-sensitive lineage refs
-> ReplayReader reconstructs why sources were used, denied or rejected without raw archives
```

The L2 flow is a design path only. It does not create production fields,
tables, APIs, retrieval plans, queues or services.

## Scenario Rules

### Read-Only Grounded QA

- Runtime asks retrieval-gateway for an EvidencePack through a retrieval profile.
- retrieval-gateway records lane coverage, source visibility, freshness and
  denied/unavailable lane refs.
- Runtime builds ContextPackage from authorized refs only.
- RAG or Runtime verifier must produce CitationVerifierResult before finalizing
  factual claims.
- Unsupported, stale, conflicting or insufficient high-risk claims force
  abstain, clarification or repair.

### Proposal Draft For A Business Action

- Proposal-driving facts must come from EvidencePack / ContextPackage refs.
- ToolIntent arguments must cite source refs or mark missing evidence.
- Grounded proposal text requires verifier refs before workflow approval.
- workflow-service approval cannot compensate for missing evidence; it only
  records human decision over an inspectable proposal.
- action-executor must verify prepared, approved and auditable refs before any
  side effect.

### Memory-Aware Context

- memory-service owns memory retrieval eligibility, scope, version, revocation
  and source lineage.
- Runtime may include memory refs in ContextPackage only with memory labels and
  source lineage refs.
- Current source truth overrides stale or conflicting memory.
- Memory cannot become hidden fallback when source lanes fail.

### Denied, Unavailable Or Expired Lanes

- Denied lanes must be visible as low-sensitive refs to Runtime, eval and
  authorized operator review.
- Denied lane bodies must not enter ContextPackage.
- Unavailable or expired lanes must remain visible as coverage gaps.
- If a required lane is denied, unavailable or expired, the answer must abstain,
  clarify or mark insufficient coverage; it must not pretend the lane was empty.

### Tool Output In Context

- Tool/MCP output enters context only after schema/provenance checks and taint
  labeling.
- Tainted tool output can be quoted or summarized only under reuse policy.
- Tool output cannot become trusted instruction, authorization evidence or
  ACTIVE memory by appearing in ContextPackage.
- Unsafe tool output is quarantined and audit-linked.

### Peer-Agent Or Provider Text

- Peer-agent and external provider text is untrusted candidate material.
- It must carry taint labels, source lane refs, budget refs and verifier refs.
- The primary Runtime remains responsible for final integration.
- Future A2A contracts require separate identity, policy, audit, replay and
  compatibility review.

### Conflict, Stale Source And Citation Repair

- ConflictSet records competing source refs and freshness refs.
- Stale evidence must not be cited as current fact unless the answer explicitly
  discusses history.
- Citation repair may adjust snippet refs, but cannot invent sources or upgrade
  denied lanes.
- If repair cannot prove support, the final path is abstain, clarification or
  blocked proposal.

### Replay And Deletion

- Replay reconstructs source selection, denial, conflict, taint and citation
  decisions from refs, hashes, versions and audit lineage.
- Expired, deleted or redacted sources fail closed under ReplayReaderPolicy.
- Normal replay must not require raw prompts, full IM message archives, raw
  provider bodies, raw MCP payloads, secrets or private service rows.

## Version And Compatibility Rules

Every future controlled implementation design must carry low-sensitive refs for:

- retrieval profile version;
- source visibility version;
- projection and temporal window version;
- EvidencePack reader policy;
- ContextPackage reader policy;
- taint vocabulary version;
- CitationVerifierResult version;
- redaction and retention policy;
- compatibility window;
- deprecation and backfill policy;
- audit lineage and ReplayReaderPolicy refs.

Compatibility must be fail-closed. If a future reader cannot interpret the
visibility, taint, citation or redaction refs, it must block finalization or
replay rather than silently treating the context as complete.

## Redaction And Privacy Rules

The Context / Evidence boundary must prefer refs and hashes over retained raw
payloads.

Redaction rules:

- denied source content is not exposed to Runtime or operator views unless the
  operator is explicitly authorized by source owner policy;
- secrets, credentials and raw provider payloads are never context material;
- deleted or expired source refs cannot be restored from local Agent archives;
- audit exports must use redaction policy refs and low-sensitive lineage;
- public dataset or synthetic fixture records must remain separated from product
  facts.

## Operator Surfaces

Before any implementation, owners must approve low-sensitive inspect-and-act
surfaces for:

- EvidencePackRef and ContextPackageRef;
- retrieval profile and source visibility refs;
- searched, selected, denied, unavailable and expired lane refs;
- EvidenceCoverageReport;
- CitationMap and CitationVerifierResult;
- ConflictSet;
- TaintLabel and vocabulary version;
- memory eligibility and revocation refs;
- tool output provenance and quarantine refs;
- redaction and ReplayReaderPolicy refs;
- final abstain, clarification or blocked-proposal reason.

Operators must be able to inspect why evidence was used, hidden, denied,
quarantined, conflicted, stale or insufficient without seeing content they are
not authorized to view.

## L3 Real-Service Smoke Plan

Before implementation can start, owners must approve a smoke plan that proves
the following with low-sensitive records only:

| Smoke | Boundary | Required Proof |
| --- | --- | --- |
| Evidence request dry run | Runtime -> retrieval-gateway | Runtime uses retrieval API only; no private table reads |
| Source visibility preservation | retrieval-gateway -> Runtime -> audit | source, scope, projection, temporal and visibility refs survive |
| Denied-lane preservation | retrieval-gateway -> Runtime/eval/operator | denied body is hidden, but denied-lane refs remain visible |
| Unavailable lane handling | retrieval-gateway -> Runtime | coverage gap is explicit; no silent empty-lane fallback |
| Citation verifier blocking | RAG/verifier -> Runtime | unsupported, stale, denied or insufficient claims cannot finalize |
| Taint propagation | MCP/peer/provider -> Runtime -> verifier | taint vocabulary and reuse policy refs survive context reuse |
| Memory precedence | memory-service -> Runtime -> verifier | memory labels remain separate and current source truth wins conflicts |
| Redaction and deletion | audit/security -> replay reader | expired/deleted source refs fail closed without raw archive fallback |
| Operator inspect-and-act | operator surface -> audit/security | authorized operator can inspect refs and reasons without body leakage |
| Public/synthetic eval separation | ai-eval -> governance | benchmark truth does not become product fact or fallback data |

These smokes must not use real NexusIM IM data until owner-approved test data
policy exists. Fixture evidence can prepare the plan, but cannot substitute for
L3 real-service smoke.

## Owner Review Checklist

| Owner | Must Approve |
| --- | --- |
| Main integration | Service boundaries, allowed paths and no production shortcut |
| retrieval-gateway owner | EvidencePack ownership, source visibility, lane coverage and denied/unavailable/expired lane refs |
| RAG owner | CitationVerifierResult policy, abstain/clarification behavior and grounded answer path |
| Agent Runtime owner | ContextPackage derivation, model input layout, verifier orchestration and replay refs |
| memory-service owner | memory retrieval eligibility, revocation, scope/version and memory-vs-source precedence |
| mcp-gateway owner | tool output provenance, taint vocabulary, quarantine and reuse policy refs |
| audit/security owner | redaction, retention, deletion, audit lineage and ReplayReaderPolicy refs |
| product/operator owner | denied-lane semantics, evidence inspection UX and incident review workflow |
| SRE/incident owner | retrieval latency, verifier latency, coverage gap metrics and incident escalation refs |

## Test And Gate Plan

Existing Agent Lab gates that must continue to pass:

```powershell
python -m pytest ai/python/tests -q
python -m ruff check ai/python
python -m mypy ai/python/nexusim_ai_common ai/python/nexusim_ai_memory ai/python/nexusim_ai_eval ai/python/scripts
.\tools\check-python-ai-worker-boundary.ps1
.\tools\check-runbook-consistency.ps1
git diff --check
git diff --cached --check
```

Focused fixture gates to rerun for this slice:

```powershell
python -m pytest ai/python/tests/test_agent_eval_context_evidence_preservation.py -q
python -m pytest ai/python/tests/test_agent_eval_architecture_coverage.py -q
python -m pytest ai/python/tests/test_agent_eval_cross_service_preservation.py -q
python -m pytest ai/python/tests/test_agent_eval_contract_version_compatibility.py -q
python -m pytest ai/python/tests/test_agent_eval_operator_governance.py -q
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_context_evidence_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_context_evidence_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_context_evidence_deeper_hardening_scenarios.json
```

## P0 / P1 / P2 Review

| Severity | Finding | Disposition |
| --- | --- | --- |
| P0 | None inside this L2 design draft | No production source access, schema, service connection, real data or runtime implementation is introduced |
| P1 | Owner review is missing | External blocker before implementation |
| P1 | L3 real-service smoke is missing | External blocker before implementation |
| P1 | Production evidence inspection UX is not approved | External blocker before implementation |
| P1 | Denied-lane product/security semantics are not approved | External blocker before implementation |
| P2 | Large public RAG thresholds remain research baselines | Keep for eval/release owner review |
| P2 | Final retention windows and redaction wording are not approved | Keep for audit/security and product review |

## Auto-Reject Rules

Reject any Context / EvidencePack implementation proposal that:

- creates proto, OpenAPI, Kafka schema, migration, service directory, database
  table, startup path or production field shape from this L2 design alone;
- lets Agent Runtime read service private tables directly;
- lets ContextPackage include unauthorized source bodies or denied-lane bodies;
- treats denied, unavailable or expired lanes as empty successful lanes;
- lets model answers self-certify citations;
- finalizes grounded answers or action-driving proposals without
  CitationVerifierResult refs;
- lets tool, MCP, peer-agent or provider output become trusted instruction;
- lets memory override current source truth without memory labels, scope/version
  and source lineage;
- requires raw prompt, raw provider body, raw MCP payload, full IM message
  archive, secret or private service row for normal replay;
- lets Python own source truth, verifier authority, redaction policy, operator
  override, final proposal, ACTIVE memory, execution or audit archive;
- treats fixture evidence or public benchmark data as production smoke or
  product source truth;
- hides source-service failure with stale cache, default success, local memory
  or fallback summaries.

## Decision

This design closes the Agent Lab-side L2 design gap for the third candidate:
Context / EvidencePack / RAG. It does not authorize implementation.

Next safe action after main integration review is either:

- owner review of the first three L2 designs; or
- a fourth L2 scoped design for Memory Admission, Tool / MCP or AgentOps if
  owners want the whole package prepared before any real-service smoke.
