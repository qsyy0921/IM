# Agent ADR Candidate Package

Date: 2026-07-02

Status: research-level ADR candidates. These files are not accepted ADRs and do
not authorize proto, OpenAPI, Kafka schema, migration, production service
directory or runtime implementation.

## Purpose

This package turns the Agent SDDs, isolated skeleton evidence, gap-closure
review and production object model into reviewable decision candidates.

The package exists because the next safe step is not more broad research and not
production integration. The next safe step is to decide ownership, versioning,
replay and governance boundaries.

## Candidate Order

1. `adr-candidate-agent-eval-replay-harness.md`
2. `adr-candidate-agent-runtime-workflow-boundary.md`
3. `adr-candidate-agent-context-evidencepack-boundary.md`
4. `adr-candidate-agent-memory-admission-boundary.md`
5. `adr-candidate-agent-tool-mcp-boundary.md`
6. `adr-candidate-agentops-governance-boundary.md`

Shared appendix:

- `cross-service-versioning-replay-governance-appendix.md`

## Review Rule

A candidate can only become an accepted ADR after main integration review. An
accepted ADR still does not create production schema by itself; it only
authorizes the next explicitly scoped integration design.

## Review Artifacts

- `../agent-architecture-review-loop-20260702.md`
- `../agent-multi-agent-handoff-fixture-evidence-20260702.md`
- `../agent-operator-governance-fixture-evidence-20260702.md`
- `../agent-operational-readiness-fixture-evidence-20260702.md`
- `../agent-controlled-implementation-readiness-fixture-evidence-20260702.md`

## Candidate Acceptance Gate

Before a candidate can be recommended for main integration acceptance, the
review ledger must show:

- no open P0 finding;
- no open P1 finding inside Agent Lab scope;
- explicit owner and non-owner state boundaries;
- object catalog completeness evidence for every production-grade object it
  introduces or consumes;
- contract versioning and replay-reader requirements;
- cross-service preservation refs for every boundary it touches;
- operator inspect-and-act path for high-risk states;
- fixture-only or public-dataset-style evidence, or a recorded blocker;
- rejection rules that block promotion instead of relying on reviewer intent;
- controlled implementation readiness evidence that blocks unaccepted ADRs,
  production contract shortcuts and real-service/Python-owner boundary breaks.
