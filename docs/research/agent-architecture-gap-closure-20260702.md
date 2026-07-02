# Agent Architecture Gap Closure

Date: 2026-07-02

Status: architecture hardening package and ADR candidate map. This is not an
ADR, proto, OpenAPI, Kafka schema, migration, production service directory or
runtime implementation.

## Executive Verdict

The Agent Plane direction is correct, but it must not jump from the isolated
skeleton to production contracts. The missing layer is an explicit architecture
decision package that turns the current SDD and fixture evidence into reviewable
ownership, versioning, promotion and rejection rules.

This package closes the design gaps found in the senior-architect review by
defining:

- which boundaries can become ADR candidates now;
- which gaps still block production promotion;
- which state each plane may own;
- which evidence is required before real service integration;
- which follow-up work stays fixture-only.

The first ADR candidate package created from this map lives under
`docs/research/adr-candidates/`. It is review material only; it does not promote
any contract, schema or runtime implementation.

## Non-Negotiable Boundary

Agent Lab remains an isolated Agent / RAG / memory / Python AI Worker /
EvidencePack / eval-gate workspace.

This document does not authorize:

- production service directories;
- proto, OpenAPI or Kafka schema changes;
- migrations;
- Docker/runtime profile changes;
- real backend, model, MCP, database, Kafka, Redis or OpenSearch integration;
- final EvidencePack, MemoryEvent, ToolIntent, Workflow or A2A contract freeze.

## Gap Closure Matrix

| Gap | Closure Decision | Evidence Needed Before Production |
| --- | --- | --- |
| No ADR layer | Draft ADR candidates in a package, not one broad platform ADR | Main integration review and explicit promotion approval |
| No contract versioning policy | Every future contract candidate must carry schema version, semantic version, compatibility window, replay reader policy and deprecation owner | ADR must show old-run replay behavior across one version bump |
| No real service integration proof | Use a preservation matrix for retrieval, memory, MCP, workflow, executor and audit refs | Integration smoke must prove refs, scope, version, taint and audit lineage survive service boundaries |
| Runtime service shape undecided | Start with Agent Runtime as module/harness boundary; do not create `agent-runtime-service` until queue/wakeup/operator needs are proven | Queue ownership, wakeup durability, checkpoint storage and operator control-plane review |
| Governance weak | Treat AgentOps as release-control candidate, not implementation boundary yet | Owner, kill switch, rollback, failure-class lifecycle and release-blocking UX review |
| Memory semantics incomplete | Promote candidate-only memory boundary; keep ACTIVE admission in Go memory-service | Group/project/procedural fixtures plus retrieval/audit proof for scope, version and revocation |
| EvidencePack body not production-ready | Promote lineage and verifier requirements only; keep body shape out of first ADR | Retrieval-gateway proof for source labels, denied lanes, taint and citation verification |
| MCP provider governance missing | Promote prepare/lease/attestation/provenance requirements, not provider schemas | Fixture proof exists; production still needs capability lease matrix, provider attestation governance, action-executor stale prepare rejection review and provider capacity / timeout evidence |
| Dataset pipeline incomplete | Keep sample adapters as skeleton; add dataset manifest/reproducibility requirement before gate use | License refs, snapshot hash, split manifest, import hash and deterministic report reproduction |
| Operator/product UX incomplete | Add UX as production-promotion prerequisite for high-risk skills | Admin can inspect memory, evidence, failures, approval, replay, kill switch and rollback |

## ADR Candidate Package

### ADR 1: Agent Eval / Replay Harness

Candidate decision:

- Eval / Replay becomes the first promotion gate for all future Agent slices.
- `EvalReport`, `ReplayBundle`, failure taxonomy, blocked promotion reasons and
  baseline refresh review are release evidence, not optional debug output.
- Replay must use low-sensitive refs, hashes, versions and decision lineage; raw
  prompt, raw provider body and real IM payloads are not normal replay inputs.

What this solves:

- prevents unreviewed Agent changes from moving toward production;
- creates a stable evidence language across runtime, memory, tool and action
  slices;
- makes failure classes operational instead of anecdotal.

Production blockers:

- no agreed owner for failure-class lifecycle;
- no release-blocking UX;
- no retention and redaction policy for replay artifacts.

### ADR 2: Agent Runtime / Workflow Boundary

Candidate decision:

- Agent Runtime owns cognitive run state: AgentRun, AgentStep, budget,
  checkpoint refs, cancel, resume, retry and replay lineage.
- `workflow-service` owns durable human wait, approval timeout, external
  callback, repair workflow and long-running compensation.
- Runtime starts as a module/harness boundary. A separate service is rejected
  until queue ownership, wakeup durability and operator needs justify it.

What this solves:

- avoids two long-transaction engines;
- keeps workflow-service from understanding raw prompts, EvidencePack body or
  planner state;
- keeps agent-service from becoming a giant workflow monolith.

Production blockers:

- checkpoint storage owner not reviewed;
- wakeup race and dedupe strategy not proven with real workflow callbacks;
- operator controls for cancel/resume/replay not designed.

### ADR 3: Context / EvidencePack Boundary

Candidate decision:

- EvidencePack remains the AI read boundary.
- ContextPackage is a derived runtime input package, not a fact source.
- First ADR freezes lineage requirements, not the final body schema:
  source refs, source visibility versions, denied lanes, taint labels, citation
  candidates, conflict markers and replay refs.

What this solves:

- prevents Agent/RAG from reading service private tables;
- makes permission leakage and missing retrieval lanes visible;
- keeps tool/provider/peer-agent content tainted until verified.

Production blockers:

- retrieval-gateway must preserve taint/source labels through real responses;
- citation verifier must be enforceable;
- denied-lane reporting must be accepted by product UX.

### ADR 4: Memory Admission Boundary

Candidate decision:

- Python emits MemoryCandidate only.
- Go-owned memory-service owns ACTIVE admission, rejection, review state,
  supersession, revocation, expiry and audit.
- Memory must be source-backed, scoped, versioned, reviewed when risky and
  revocable.
- Personal, group, project and procedural memory share the admission machinery
  but require different evidence thresholds.

Group memory hardening:

- group memory must carry group, speaker, subject and audience refs;
- group consensus is not the same as one user statement;
- "someone in the group said X" is not "the group knows or agrees X";
- membership changes, deletion, revocation and cross-group isolation are
  production blockers before real IM use.

What this solves:

- prevents Python from owning long-term truth;
- prevents memory pollution from becoming a hidden fallback;
- turns Akashic/SocialMemBench lessons into explicit attribution and version
  requirements without shrinking the whole Agent architecture to group memory.

Production blockers:

- no real memory-service scope/version/revocation retrieval proof;
- no audit explanation path for why a memory became ACTIVE;
- no UX for review, correction or forget requests.

### ADR 5: Tool / MCP Boundary

Candidate decision:

- Tool and MCP providers are untrusted inputs.
- Agent Runtime may produce ToolIntent and proposal refs, but high-risk actions
  require prepare, policy, approval and action-executor execution.
- mcp-gateway owns provider provenance, schema hash, capability lease,
  attestation refs, output tainting and prepare result refs.
- action-executor remains sole side-effect owner.

What this solves:

- blocks MCP tool descriptions from becoming authority;
- makes provider trust explicit;
- ensures replay can connect tool prepare, approval, execution and audit.

Production blockers:

- capability lease matrix not reviewed;
- provider attestation governance not reviewed;
- production action-executor stale PreparedToolRef rejection not reviewed;
- provider capacity and timeout budgets not reviewed.

Fixture-only update:

- lease denial, attestation downgrade, sandbox onboarding, prepare re-prepare,
  tool-output taint and executor stale-prepare rejection are recorded in
  `docs/research/agent-tool-mcp-fixture-evidence-20260702.md`.

### ADR 6: AgentOps / Governance

Candidate decision:

- AgentOps is a release-control boundary first, not a production implementation
  service yet.
- Every AgentDefinition and SkillPackage must have owner, purpose, risk tier,
  release channel, eval suite, tool/memory grants, rollback ref and disable
  switch before production.
- P0/P1 eval failures, replay gaps and audit gaps block release.

What this solves:

- prevents unmanaged prompt/tool bundles;
- ties release state to eval and replay evidence;
- gives operators a clear disable/rollback path.

Production blockers:

- kill switch owner not agreed with control plane;
- no admin UX for release pinning, baseline refresh approval or failure review;
- no canary/shadow result comparison against offline baseline.

## Cross-Cutting Versioning Policy

Every future Agent contract candidate must define:

- `contract_name`;
- `schema_version`;
- `semantic_version`;
- `producer_owner`;
- `consumer_owners`;
- `compatibility_window`;
- `replay_reader_policy`;
- `redaction_policy`;
- `deprecation_policy`;
- `migration_or_backfill_policy`, if persistent;
- `rejection_conditions`.

Do not promote any contract whose old artifacts cannot be replayed or explained
after a version bump.

## Integration Preservation Matrix

Before backend integration, each real service boundary must prove these refs are
preserved:

| Boundary | Must Preserve |
| --- | --- |
| retrieval-gateway -> Agent Runtime | EvidencePack ref, source refs, visibility versions, denied lanes, taint labels |
| memory-service -> Agent Runtime | memory refs, scope, version, state, revocation/supersession refs |
| mcp-gateway -> Agent Runtime | prepared ref, provider ref, schema hash, lease, attestation, taint labels |
| workflow-service -> Agent Runtime | approval decision ref, timeout ref, wakeup id, resume/cancel correlation |
| action-executor -> Eval/Replay | execution ref, idempotency ref, state-diff ref, audit ref, repair/redrive refs |
| audit-service -> AgentOps | low-sensitive archive refs, actor refs, policy refs, retention refs |

If a boundary drops any required ref, promotion is rejected even if the happy-path
answer looks correct.

## Operator And Product UX Requirements

Production Agent UX must let authorized users inspect:

- which sources supported an answer;
- why the Agent abstained or refused;
- what memory candidates were proposed;
- which memory is ACTIVE, superseded, revoked or under review;
- which tool/action was prepared, approved, executed or rejected;
- which replay bundle explains a failed run;
- who owns an AgentDefinition or SkillPackage;
- how to disable, roll back or pin a release.

Without this UX, AgentOps remains a design concept instead of an operational
control plane.

## Open Dataset Pipeline Requirements

The current adapter skeleton is enough for Phase 1, but not enough for release
gates. A real dataset pipeline must include:

- dataset name, source URL and license ref;
- snapshot hash and import script version;
- train/dev/eval split manifest;
- case id stability rules;
- source and evidence ref generation rules;
- deterministic report reproduction command;
- blocked promotion reason if licensing or reproducibility is incomplete.

## Recommended Execution Order

1. Write the six ADR candidates from this package, starting with Eval / Replay
   and Runtime / Workflow.
2. Add a contract-versioning appendix to each candidate ADR.
3. Add a real-service preservation checklist before any integration work.
4. Keep memory, group memory, MCP and EvidencePack hardening fixture-only until
   ADR review accepts their boundaries.
5. Defer production service directories, schema and migration until main
   integration approves a specific ADR.

## Rejection Rules

Reject any follow-up design that:

- treats fixture fields as final production schema;
- lets Python own ACTIVE memory, approval, execution, audit archive or business
  facts;
- lets workflow-service consume raw prompt, EvidencePack body or planner state;
- lets Agent Runtime wait durably for human approval without workflow-service;
- treats MCP tool descriptions or provider output as trusted instructions;
- bypasses action-executor for side effects;
- uses real IM data in isolated eval;
- hides missing retrieval, policy or provider state behind fallback summaries.
