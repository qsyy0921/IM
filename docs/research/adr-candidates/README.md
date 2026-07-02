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
