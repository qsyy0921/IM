# Agent ADR Promotion Readiness Review

Date: 2026-07-02

Scope: review whether the isolated Agent Lab skeleton is ready to produce ADR
candidates. This is not an ADR and does not freeze proto, OpenAPI, Kafka schema,
runtime service directories, memory events, EvidencePack shape, tool/MCP
contract or agent taxonomy.

## Verdict

The isolated skeleton is ready for ADR candidate drafting, but not ready for
production contract promotion.

Reason:

- The harness now runs end-to-end with synthetic and public-dataset-style
  fixtures.
- Core eval artifacts exist: `EvalCase`, `EvalRun`, `EvalResult`,
  `EvalReport`, `ReplayBundle`, `AgentRunTrace` and fixture models for
  EvidencePack, ContextPackage, MemoryCandidate, ToolIntent, RuntimeControl,
  StateDiffReport and ReplayObservability.
- Unit, integration and boundary tests cover the first-stage Agent-layer
  skeleton without backend services or real IM data.
- Remaining gaps are contract governance and production ownership decisions,
  not missing first-stage skeleton mechanics.

## Evidence Snapshot

Current fixture families under `ai/python/fixtures/agent_eval/`:

- first trio and core scenarios;
- runtime-control positive, negative and deeper hardening scenarios;
- MCP security and MCP security hardening scenarios;
- context/evidence, hardening and deeper hardening scenarios;
- memory admission, hardening and deeper hardening scenarios;
- memory calibration sample and public-dataset-style export;
- state-diff, hardening and deeper hardening scenarios;
- ReplayBundle observability scenarios;
- report matrix sample and public-dataset-style adapter samples.

Current tests under `ai/python/tests/` cover:

- contract validation and low-sensitive payload rejection;
- dataset adapters and adapter runner;
- evaluator scoring and failure taxonomy;
- AgentRun trace construction;
- CLI integration over fixture suites;
- report lifecycle, baseline review and matrix generation;
- memory calibration recommendation, blocked reasons and CLI behavior;
- worker and memory extraction candidate boundaries.

Latest module verification:

- `python -m pytest ai/python/tests -q` passed with 196 tests.
- `python -m ruff check ai/python` passed.
- `python -m mypy ai/python/nexusim_ai_common ai/python/nexusim_ai_memory ai/python/nexusim_ai_eval ai/python/scripts` passed.
- `.\tools\check-python-ai-worker-boundary.ps1` passed.
- `git diff --check` and `git diff --cached --check` passed before commit.

## Candidate Readiness Matrix

| Candidate | Readiness | Evidence | ADR Direction | Do Not Promote Yet |
| --- | --- | --- | --- | --- |
| Agent eval / replay harness | Ready for ADR candidate | EvalReport, ReplayBundle, reporting, regression, matrix and CLI tests | Define release-gate semantics, report retention, blocked promotion reasons and failure taxonomy governance | Do not make it a required production gate until governance owner, failure-class lifecycle and release blocking UX are agreed |
| Agent Runtime / Harness module | Ready for ADR candidate | RuntimeControl fixture covers cancel, resume, checkpoint, replay, version drift, wakeup race and replay lineage | Prefer runtime module plus workflow-service boundary before a separate service | Do not create `agent-runtime-service` until queue ownership, wakeup durability and operator control-plane needs are proven |
| ContextPackage / EvidencePack | Ready for ADR candidate | Source coverage, conflict, stale evidence, permission abstain, ranking, redrive, citation repair, denied-lane and taint fixtures | Define low-sensitive context lineage and taint vocabulary as reviewable contract candidates | Do not freeze EvidencePack body shape or retrieval service API from fixture refs alone |
| MemoryCandidate / admission | Ready for ADR candidate | Source/speaker/audience, supersedes, revocation, stale blocking, dedupe, confidence, procedural migration, policy governance and calibration fixtures | Define candidate-only Python boundary and Go-owned ACTIVE admission decision | Do not promote calibrated thresholds as production policy without memory-service, policy-service and audit review |
| ToolIntent / MCP boundary | Ready for ADR candidate | Tool poisoning, unsafe output, provider provenance, selection attack, schema mismatch, prepare expiry, lease and attestation refs | Define tool prepare and MCP provider trust boundaries as ADR candidates | Do not treat MCP provider output, tool descriptions or fixture provider refs as trusted production inputs |
| State-diff evaluator | Ready for ADR candidate | State diff, execution/audit refs, repair/redrive, partial execution, idempotency, compensation, dependency graph and operator review fixtures | Define expected-vs-actual state report requirements for approved actions | Do not let Python own business state or execute side effects |
| ReplayBundle observability | Ready for ADR candidate | Observability refs, hash refs, version refs, failure taxonomy refs and trace linkage refs in EvalReport, ReplayBundle and AgentRunTrace | Define replay observability minimums and low-sensitive retention policy | Do not require raw prompt, raw provider body or production payload archive for replay |
| Governance / AgentOps | Not ADR-ready as an implementation boundary | SDD exists, but current code mainly proves report metadata and blocked promotion reasons | Draft governance ADR only after release blocking UX and ownership are reviewed | Do not add production kill switch, release registry or service directory from this skeleton |

## Recommended ADR Package

Draft ADRs in this order:

1. `ADR-Agent-Eval-Replay-Harness`
   - Promote the offline harness as the required evidence model for future Agent
     implementation slices.
   - Include failure taxonomy lifecycle, report retention metadata and blocked
     promotion reasons.

2. `ADR-Agent-Runtime-Workflow-Boundary`
   - Keep workflow-service as the durable human-wait and external callback owner.
   - Keep Agent Runtime / Harness as cognitive run, checkpoint, cancel, resume
     and replay owner.
   - Start as runtime module unless service isolation becomes necessary.

3. `ADR-Agent-Memory-Admission-Boundary`
   - Python emits MemoryCandidate only.
   - Go-owned memory-service controls ACTIVE admission, revocation, audit and
     policy integration.
   - Calibration refs are evidence, not production policy constants.

4. `ADR-Agent-Context-EvidencePack-Boundary`
   - Define source-backed context lineage, denied-lane handling and taint
     propagation.
   - Keep body/schema details out of the first ADR unless integration is
     explicitly approved.

5. `ADR-Agent-Tool-MCP-Boundary`
   - Define prepare/lease/attestation/provenance requirements.
   - Confirm action-executor remains sole side-effect owner.

## Blocking Gaps Before Production Promotion

These gaps do not block ADR candidate drafting, but they block production
contract promotion:

- No production owner review for failure-class lifecycle and release blocking.
- No agreed versioning policy for EvidencePack, MemoryCandidate, ToolIntent or
  ReplayBundle beyond low-sensitive fixture refs.
- No integration proof that retrieval-gateway, memory-service, mcp-gateway,
  workflow-service, action-executor and audit-service preserve the same refs.
- No operator UX for redrive, baseline refresh approval, release pinning,
  kill-switch or rollback.
- No real public dataset import pipeline with licensing and reproducibility
  manifests; current public export is deliberately fixture metadata only.
- No load or runtime profile for a future runtime service; current work is
  backend-isolated by design.

## Rejection Conditions

Reject promotion if the proposed ADR:

- creates production service directories, schemas, migrations or startup paths
  from the fixture skeleton;
- lets Python own ACTIVE memory, approval, execution, audit archive or business
  facts;
- requires raw prompts, raw provider bodies or real IM messages for normal
  replay;
- treats MCP provider output or tool descriptions as trusted input;
- makes workflow-service understand raw prompt, EvidencePack body or planner
  state;
- bypasses action-executor for side effects;
- turns synthetic/public-dataset-style fixture refs into final product taxonomy.

## Next Step

Ask the main integration session to review whether to draft ADR candidates from
this package. If review does not approve ADR drafting, continue fixture-only
hardening only in the specific boundary that failed review.
