# Agent Architecture Review Loop

Date: 2026-07-02

Status: first review-and-closure pass for Agent ADR candidates. This is not an
accepted ADR, production contract, service directory, schema or runtime
implementation.

## Verdict

Initial review verdict: rejected for production promotion.

Reason: the Agent plane has the right major components, object model and ADR
candidates, but the candidate package did not yet contain a reusable pass/fail
review ledger. Without that ledger, future review could accept a candidate by
reading intent instead of checking evidence.

After this pass: conditionally passed for research-level ADR-candidate review.

This does not mean the overall architecture goal is complete. Full completion
still requires main integration review, accepted ADRs and later explicitly
scoped integration design. Agent Lab remains backend-isolated.

## Evidence Read

- `prompt.md`
- `agent.md`
- `docs/runbook/current-goal.md`
- `docs/runbook/current-brief.md`
- `docs/runbook/remaining-goals.md`
- `docs/research/agent-architecture-gap-closure-20260702.md`
- `docs/research/agent-production-object-model-20260702.md`
- `docs/research/adr-candidates/`
- `docs/research/agent-multi-agent-handoff-fixture-evidence-20260702.md`
- `docs/research/agent-operator-governance-fixture-evidence-20260702.md`
- `docs/research/agent-operational-readiness-fixture-evidence-20260702.md`
- `docs/research/agent-controlled-implementation-readiness-fixture-evidence-20260702.md`
- `docs/research/agent-architecture-coverage-fixture-evidence-20260702.md`
- `docs/research/agent-contract-version-compatibility-fixture-evidence-20260702.md`
- `docs/sdd/agent-runtime.md`
- `docs/sdd/agent-eval-replay-harness.md`
- `docs/sdd/agent-memory-admission.md`
- `docs/sdd/agent-context-evidencepack.md`
- `docs/sdd/agent-tool-mcp-boundary.md`
- `docs/sdd/agent-governance-agentops.md`

## Review Findings Before Fix

| Severity | Finding | Impact | Fix In This Pass |
| --- | --- | --- | --- |
| P0 | None found inside current research boundary | No immediate production-safety contradiction found because production integration is still blocked | Keep hard boundary unchanged |
| P1 | ADR candidates lacked one shared review ledger and candidate-acceptance gate | Review could become subjective and skip owner/version/replay/operator/eval proof | Added shared review ledger requirements and acceptance gates |
| P1 | Eval/Replay and Runtime/Workflow were prioritized but not tied to a reusable review sequence | Later candidates could be reviewed before the platform gate and long-running ownership boundary are stable | Recorded required review order and gate dependencies |
| P1 | Cross-service preservation matrix existed but did not define the minimum proof ladder before integration design | Real-service integration could start before refs, versions, taint and audit lineage are proven to survive boundaries | Added fixture-only verification ladder and preservation gate |
| P1 | Operator governance existed as objects, but acceptance did not require inspect-and-act UX evidence | AgentOps could become a release label without practical control | Added operator acceptance gate for memory, evidence, replay, approval, release, failure, kill switch and rollback |
| P1 | Production object catalog completeness was prose-only | ADR acceptance could miss an object without owner/lifecycle/version/audit/replay/rejection evidence | Added fixture-only object completeness rehearsal and promotion gate |
| P2 | Capacity, cost, retention and latency budgets were conceptual | Does not block research-level ADR review, but will block production readiness without real owner approval | Added fixture-only operational readiness budget rehearsal; production SLOs remain blocked |

## Updated Candidate Acceptance Gates

An ADR candidate can only be recommended for main integration acceptance when it
has all of the following:

1. Ownership: owner, non-owner and state boundary are explicit.
2. Lifecycle: object states or run states are named at concept level.
3. Versioning: contract version, compatibility window and replay reader policy
   are required before any future production contract.
4. Replay: normal replay works from low-sensitive refs, hashes, versions and
   lineage, not raw prompts or raw provider payloads.
5. Preservation: boundary refs for source, memory, tool, workflow, action and
   audit are listed and cannot be silently dropped.
6. Eval evidence: fixture-only or public-dataset-style proof exists, or the gap
   is marked as a blocker.
7. Operator governance: authorized operators have a future inspect-and-act path
   for the affected object.
8. Rejection rules: P0/P1 failure classes block promotion.
9. Review ledger: open P0/P1 findings are either closed or explicitly rejected
   from scope by the hard Agent Lab boundary.

## Requirement Review Matrix

| Required Area | Current Evidence | Review Result |
| --- | --- | --- |
| Agent Runtime / Harness | SDD plus Runtime/Workflow ADR candidate | Conditionally passes candidate review; checkpoint owner and wakeup proof remain future evidence |
| Eval / Replay Harness | SDD plus Eval/Replay ADR candidate | Conditionally passes candidate review; first accepted ADR should be this gate |
| Context / EvidencePack / RAG | SDD plus Context/EvidencePack ADR candidate | Conditionally passes; body schema remains unfrozen |
| Memory admission | SDD plus Memory Admission ADR candidate | Conditionally passes; ACTIVE memory remains Go-owned |
| Tool / MCP boundary | SDD plus Tool/MCP ADR candidate | Conditionally passes; provider trust remains untrusted by default |
| Multi-agent / A2A boundary | Complete-scope note, Runtime SDD and fixture evidence | Conditionally passes; bounded delegation evidence exists and production A2A remains unfrozen |
| Workflow / approval | Runtime/Workflow candidate plus workflow ownership matrix | Conditionally passes; workflow cannot read planner internals |
| Action executor handoff | Object model and preservation appendix | Conditionally passes; executor remains sole side-effect owner |
| AgentOps / governance | SDD plus AgentOps candidate and fixture evidence | Conditionally passes; production operator UX remains unimplemented |
| Production object completeness | Object model plus fixture evidence | Conditionally passes; production field/schema design remains unfrozen |
| Operator governance surfaces | Operator governance fixture evidence | Conditionally passes; memory, evidence, replay, approval, release, failure-class, kill-switch and rollback inspect-and-act surfaces have low-sensitive evidence |
| Operational readiness budgets | Operational readiness fixture evidence | Conditionally passes as fixture-only proof; real telemetry, capacity, SLO and on-call contracts remain future owner review |
| Controlled implementation readiness | Controlled implementation readiness fixture evidence | Conditionally passes as a fail-closed gate; fixture-only hardening may continue, but controlled implementation and production contracts remain blocked without accepted ADRs and owner review |
| Required architecture surface coverage | Architecture coverage fixture evidence | Conditionally passes; 13 required surfaces have owner, SDD, research, ADR, fixture, lifecycle, version, replay, preservation, audit, operator, eval and rejection refs |
| Contract versioning | Production object model, shared appendix and fixture evidence | Conditionally passes; compatibility matrix and version-bump rehearsals have fixture evidence |
| Cross-service ref preservation | Shared appendix and fixture evidence | Conditionally passes; real-service preservation smoke is required before production design |
| Security / privacy / audit | SDD rejection rules plus appendix | Conditionally passes; raw prompt/provider replay remains rejected |
| Open dataset / synthetic eval | Eval SDD and current Python fixture harness | Passes Phase 1 boundary; not a production release gate yet |

## Re-Review Result

After adding the review ledger requirement, the current package has no known P0
inside Agent Lab scope and no unresolved P1 that can be closed without main
integration review or prohibited production integration.

The architecture is therefore conditionally ready for main integration review of
the first two ADR candidates:

1. Agent Eval / Replay Harness.
2. Agent Runtime / Workflow Boundary.

Do not mark the overall goal complete yet. The following blockers remain
outside this isolated pass:

- no accepted ADR exists yet;
- main integration has not reviewed this pass;
- no real-service preservation smoke has been approved;
- no production operator UX has been implemented;
- no production schema or runtime contract is authorized.

## Next Review Loop

Next loop should start from the Eval/Replay candidate and verify:

- release-gate semantics are sufficient;
- P0/P1 failure classes block promotion;
- contract-version compatibility, version-bump replay and cross-service
  preservation rehearsals have fixture evidence;
- object catalog completeness has fixture evidence;
- operator governance surface completeness has fixture evidence;
- operational readiness budget coverage has fixture evidence;
- controlled implementation readiness gate blocks unaccepted ADRs and production-path shortcuts;
- required architecture surface coverage has fixture evidence;
- report retention and redaction policy are acceptable;
- baseline approval UX has an owner.

If any P0/P1 is found, update the candidate and rerun this review loop before
moving to Runtime/Workflow.
