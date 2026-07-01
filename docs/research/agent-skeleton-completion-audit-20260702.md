# Agent Skeleton Completion Audit

Date: 2026-07-02

Scope: audit the immutable Agent Lab goal against the current backend-isolated
Python skeleton and runbook state. This document is not an ADR. It does not
freeze proto, OpenAPI, Kafka schema, production service directories, EvidencePack
shape, memory event shape, tool/MCP contract, workflow contract, agent taxonomy
or skill taxonomy.

## Verdict

The Phase 1 isolated Agent-layer skeleton is complete enough to serve as the
current executable baseline.

It can run synthetic and public-dataset-style evals end to end, produce
low-sensitive EvalReport / ReplayBundle outputs, and validate the Agent layer
without backend services, production databases, Kafka, Redis, OpenSearch, real
MCP providers, real model providers or real NexusIM IM data.

This does not mean production integration is ready. The next architectural step
is still an explicit ADR candidate drafting decision, as recorded in
`agent-adr-promotion-readiness-20260702.md`.

## Evidence Snapshot

Current code and fixture surface:

- 24 fixture files under `ai/python/fixtures/agent_eval/`.
- 13 Python test files under `ai/python/tests/`.
- 196 test functions/classes matched by the current test tree.
- Core package: `ai/python/nexusim_ai_eval/`.
- CLI scripts: `ai/python/scripts/run_agent_eval_fixture.py`,
  `run_agent_dataset_adapter.py`, `run_agent_eval_regression.py`,
  `run_agent_eval_current_report.py`, `run_agent_eval_report_matrix.py` and
  `run_agent_memory_calibration.py`.
- Runbook control plane: `docs/runbook/current-goal.md`,
  `current-brief.md`, `remaining-goals.md` and `development-progress.md`.

## Requirement Matrix

| Immutable goal requirement | Status | Evidence |
| --- | --- | --- |
| Agent eval / replay harness | Complete for Phase 1 | `evaluator.py`, `reporting.py`, `comparison.py`, `run_agent_eval_fixture.py`, `run_agent_eval_regression.py`, `test_agent_eval_evaluator.py`, `test_agent_eval_reporting.py`, `test_agent_eval_comparison.py` |
| Public dataset adapter skeleton | Complete for Phase 1 | `adapters.py`, `adapter_runner.py`, `adapter_samples/`, `run_agent_dataset_adapter.py`, `test_agent_eval_adapters.py`, `test_agent_eval_adapter_runner.py` |
| Synthetic IM-like fixture system | Complete for Phase 1 | `fixtures.py`, `synthetic_first_trio.json`, `synthetic_core_scenarios.json`, runtime, context, memory, MCP, state-diff and replay fixture suites |
| AgentRun / AgentStep trace model | Complete for Phase 1 | `trace.py` defines `AgentRunTrace` and `AgentStep`; `test_agent_eval_trace.py` covers trace construction |
| ContextPackage / EvidencePack fixture model | Complete for Phase 1 | `EvidencePackFixture`, `ContextPackageFixture`, context/evidence fixtures, context hardening fixtures and integration tests |
| MemoryCandidate / memory admission fixture model | Complete for Phase 1 | `MemoryCandidateFixture`, memory admission fixtures, memory calibration fixtures, public export and memory calibration tests |
| ToolIntent / MCP security fixture model | Complete for Phase 1 | `ToolIntentFixture`, MCP security fixtures, MCP hardening fixtures and adapter alignment samples |
| Approval / HITL / timeout / cancel / resume / replay fixture | Complete for Phase 1 | Runtime-control fixtures, state-diff approval refs, failure classes for approval and provider timeout, runtime event tests |
| State-diff evaluator | Complete for Phase 1 | State-diff fields in `EvalCase`, state-diff scoring in `evaluator.py`, state-diff fixtures and evaluator tests |
| Bounded multi-agent handoff fixture | Complete for Phase 1 | `MULTI_AGENT_HANDOFF` capability family, handoff scoring in `evaluator.py`, `synthetic_core_scenarios.json` |
| EvalCase / EvalRun / EvalResult / EvalReport | Complete for Phase 1 | `contracts.py` dataclasses, contract validation tests and report CLI tests |
| ReplayBundle | Complete for Phase 1 | `ReplayBundle` in `contracts.py`, `_replay_bundle` in `evaluator.py`, observability fixtures and integration tests |
| CLI runner | Complete for Phase 1 | Fixture, adapter, regression, current-report, report-matrix and memory-calibration CLI scripts |
| Unit tests | Complete for Phase 1 | Contract, evaluator, trace, adapter, reporting, comparison and memory calibration unit coverage |
| Integration tests | Complete for Phase 1 | `test_agent_eval_integration.py` exercises fixture CLI flows and reports |
| Boundary tests | Complete for Phase 1 | `test_agent_eval_contracts.py`, low-sensitive validation, forbidden field checks and `check-python-ai-worker-boundary.ps1` |
| Maintainable docs and runbook progress tracking | Complete with caveat | Runbook and SDD entries are current; repo-wide doc entrypoint line-budget checks still need a separate cleanup decision because some affected docs are outside the Agent Lab slice |

## Isolation Review

The current skeleton stays inside the allowed first-stage paths:

- `ai/python/nexusim_ai_eval/`
- `ai/python/fixtures/agent_eval/`
- `ai/python/scripts/`
- `ai/python/tests/`
- `docs/research/`
- `docs/sdd/`
- `docs/runbook/`

It does not create or modify production service directories, proto/OpenAPI/Kafka
schemas, migrations, Docker/runtime profiles, loadtest paths or production
startup paths. The eval contracts reject backend URLs, database DSNs, service
URLs, provider request bodies, business payloads and raw IM message text.

## What This Means For Development

The immutable Codex goal should remain stable. Ongoing scope changes should be
made through runbook and SDD documents, not by rewriting the goal.

The next coding work should not restart broad research. It should choose one
runbook item, add a small backend-isolated code slice, add focused tests, update
the relevant docs, run gates, commit and handoff.

## Not Ready For Production Promotion

The current skeleton must be rejected as a production contract if a proposal:

- creates production schemas, migrations, services or startup paths from the
  fixture contracts;
- treats fixture fields as final EvidencePack, MemoryEvent, ToolIntent,
  Workflow or A2A shapes;
- lets Python own ACTIVE memory, approval, execution, audit archive, final
  business state or IM facts;
- requires raw prompts, raw provider payloads or real IM messages for normal
  replay;
- treats MCP provider output or tool descriptions as trusted input;
- connects the harness to PostgreSQL, Kafka, Redis, OpenSearch, real MCP
  providers, real action-executor, workflow-service, memory-service or model
  providers during isolated eval.

## Current Risks

- ADR candidate drafting still needs an explicit user / integration decision.
- Governance / AgentOps implementation boundaries remain less proven than eval,
  runtime, context, memory, tool and state-diff skeleton mechanics.
- Public-dataset adapters are intentionally metadata and sample oriented; a real
  import pipeline still needs licensing, reproducibility and dataset manifests.
- Repo-wide doc entrypoint line-budget checks still fail on existing broad
  documentation; this audit does not clean those broader docs because some are
  outside the Agent Lab boundary.

## Recommended Next Step

If the user wants architecture progression, draft ADR candidates from
`agent-adr-promotion-readiness-20260702.md`, starting with the Agent eval /
replay harness boundary.

If the user wants more coding before ADRs, continue only fixture-only hardening
in one bounded area, preferably ReplayBundle observability taxonomy or memory
calibration export reproducibility.
