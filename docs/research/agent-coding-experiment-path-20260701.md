# Agent Coding Experiment Path

Date: 2026-07-01
Mode: Agent Exploration Mode
Status: Coding path for the isolated Agent Lab experiment. This is not an ADR,
proto, OpenAPI, Kafka schema, migration, production service directory or
backend integration plan.

## 1. Purpose

This document defines how Agent Lab starts coding while fully isolating Agent
metrics from the backend layer. The first implementation target is an offline
Agent eval harness that consumes public-dataset adapters or synthetic IM-like
fixtures and emits low-sensitive EvalReport / ReplayBundle JSON.

The goal is to prove Agent capabilities before merging with backend services.

## 2. Isolation Rule

First-stage Agent coding must stay isolated:

- no real NexusIM IM data;
- no imports from `services/` production packages;
- no PostgreSQL, Kafka, Redis, OpenSearch, object storage, MCP provider or model
  provider dependency;
- no action-executor, workflow-service or memory-service call;
- no production startup path;
- no fake/mock/fixture fallback consumed by production code.

Allowed paths:

```text
ai/python/nexusim_ai_eval/
ai/python/fixtures/agent_eval/
ai/python/scripts/run_agent_eval_fixture.py
ai/python/scripts/run_agent_dataset_adapter.py
ai/python/scripts/run_agent_eval_regression.py
ai/python/tests/test_agent_eval_*.py
docs/research/
docs/sdd/
docs/runbook/
```

Disallowed paths for this phase:

```text
services/
api/
schemas/
migrations/
deploy/
loadtest/
```

## 3. Implemented Slice 0

Implemented code:

```text
ai/python/nexusim_ai_eval/
  __init__.py
  adapter_runner.py
  adapters.py
  comparison.py
  contracts.py
  evaluator.py
  fixtures.py
  trace.py
ai/python/fixtures/agent_eval/adapter_samples/
ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json
ai/python/fixtures/agent_eval/synthetic_first_trio.json
ai/python/fixtures/agent_eval/synthetic_core_scenarios.json
ai/python/fixtures/agent_eval/synthetic_runtime_control_scenarios.json
ai/python/fixtures/agent_eval/synthetic_mcp_security_scenarios.json
ai/python/fixtures/agent_eval/synthetic_context_evidence_scenarios.json
ai/python/fixtures/agent_eval/synthetic_memory_admission_scenarios.json
ai/python/fixtures/agent_eval/synthetic_state_diff_scenarios.json
ai/python/scripts/run_agent_eval_fixture.py
ai/python/scripts/run_agent_dataset_adapter.py
ai/python/scripts/run_agent_eval_regression.py
```

Implemented tests:

```text
ai/python/tests/test_agent_eval_contracts.py
ai/python/tests/test_agent_eval_evaluator.py
ai/python/tests/test_agent_eval_integration.py
ai/python/tests/test_agent_eval_adapters.py
ai/python/tests/test_agent_eval_adapter_runner.py
ai/python/tests/test_agent_eval_comparison.py
ai/python/tests/test_agent_eval_trace.py
```

Slice 0 covers:

- `EvalCase` validation for low-sensitive synthetic fixtures;
- forbidden field rejection for raw prompt, backend URLs and production payloads;
- deterministic scoring for grounded RAG citation coverage and permission
  leakage;
- deterministic scoring for memory outcome, scope and revocation;
- deterministic scoring for tool poisoning and unsafe output quarantine;
- deterministic scoring for state-diff mismatch;
- ReplayBundle completeness and side-effect reexecution rejection;
- CLI integration using `synthetic_first_trio.json`.
- public dataset adapter skeletons for Qasper/HotpotQA-like RAG,
  ToolSandbox/tau-bench-like tool workflows and STATE-Bench/LoCoMo-like memory;
- AgentRun / AgentStep trace skeleton with EvidencePack, ContextPackage,
  MemoryCandidate and ToolIntent fixture refs;
- `synthetic_core_scenarios.json` for insufficient evidence, permission leakage,
  memory pollution, unsafe output, approval timeout, provider timeout,
  state-diff mismatch and bounded handoff.
- local public-dataset-style sample payloads for Qasper-like RAG,
  ToolSandbox-like tool security and STATE-Bench-like memory;
- batch adapter conversion / run CLI;
- EvalReport baseline fixture, regression delta and blocked promotion reasons.
- fixture-only runtime-control coverage for cancel propagation, checkpointed
  approval resume and replay without side-effect reexecution.
- fixture-only MCP security coverage for poisoned tool descriptions, unsafe MCP
  output instructions, provider provenance mismatch and sandbox-only providers.
- fixture-only ContextPackage / EvidencePack coverage for source coverage,
  conflict markers, stale evidence avoidance and permission abstain.
- fixture-only richer memory admission coverage for group speaker/audience,
  project supersedes, profile aggregate review, revoked memory blocking, stale
  memory blocking and overgeneralization prevention.
- fixture-only state-diff report coverage for approved action outcome refs,
  expected-vs-actual state changes, missing execution refs, incomplete reports
  and unauthorized mutation detection.

## 4. Code Architecture

### 4.1 Contracts

`nexusim_ai_eval.contracts` owns:

- allowed capability families;
- allowed failure classes;
- forbidden eval/backend fields;
- `EvalCase`, `EvalResult`, `EvalReport`, `ReplayBundle` dataclasses;
- low-sensitive suite validation;
- stable hash/ref helpers.

It cannot own:

- production schema;
- backend service connectivity;
- model/provider requests;
- durable memory or business state.

### 4.2 Evaluator

`nexusim_ai_eval.evaluator` owns deterministic scoring:

- grounded RAG;
- permission leakage;
- memory admission;
- tool security;
- state diff;
- HITL policy match;
- bounded handoff scope;
- replay completeness.

The evaluator does not call models or services. Inputs represent expected and
actual low-sensitive refs from fixtures. Later dataset adapters can generate
those refs from public datasets.

### 4.3 Adapters

`nexusim_ai_eval.adapters` owns public-dataset-style adapter skeletons:

- `QasperLikeRagAdapter`;
- `ToolSandboxLikeAdapter`;
- `StateBenchLikeMemoryAdapter`;
- `suite_from_adapter_cases`.

Adapters convert already-local, low-sensitive dict payloads into EvalCase JSON.
They do not download public datasets, call providers or import backend code.

`nexusim_ai_eval.adapter_runner` owns batch conversion from local adapter sample
payloads to validated EvalSuite JSON and optional immediate EvalReport runs.

### 4.4 Baseline Comparison

`nexusim_ai_eval.comparison` owns low-sensitive EvalReport comparison:

- suite/status/count deltas;
- aggregate score deltas;
- failure distribution deltas;
- case-level score/status deltas;
- blocked promotion reasons.

It accepts EvalReport-like JSON only. It does not read production data, execute
fixtures, call models or connect to backend services.

### 4.5 AgentRun Trace

`nexusim_ai_eval.trace` owns deterministic trace skeletons:

- AgentRunTrace;
- AgentStep;
- EvidencePackFixture;
- ContextPackageFixture;
- MemoryCandidateFixture;
- ToolIntentFixture.
- RuntimeControlFixture.
- ContextPackage source coverage, conflict, stale-source and permission-abstain
  metadata.
- MemoryCandidate source, speaker, audience, supersedes, stale-memory and
  review metadata.
- StateDiffReport execution, approval, prepare, state-change and audit refs.

The trace is low-sensitive and fixture-only. It records refs, hashes and failure
classes, not raw prompt, message body or provider output.

### 4.6 Fixtures

`nexusim_ai_eval.fixtures` loads and validates JSON suites. The fixture kind must
be `synthetic_im_like`.

Current fixture:

```text
ai/python/fixtures/agent_eval/synthetic_first_trio.json
ai/python/fixtures/agent_eval/synthetic_core_scenarios.json
ai/python/fixtures/agent_eval/synthetic_runtime_control_scenarios.json
ai/python/fixtures/agent_eval/synthetic_mcp_security_scenarios.json
ai/python/fixtures/agent_eval/synthetic_context_evidence_scenarios.json
ai/python/fixtures/agent_eval/synthetic_memory_admission_scenarios.json
ai/python/fixtures/agent_eval/synthetic_state_diff_scenarios.json
ai/python/fixtures/agent_eval/adapter_samples/
ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json
```

These fixtures are intentionally synthetic. They prove harness mechanics before
adding larger public dataset adapters.

### 4.7 CLI

`ai/python/scripts/run_agent_eval_fixture.py` runs one suite and emits a
low-sensitive report:

```powershell
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_first_trio.json
```

`ai/python/scripts/run_agent_dataset_adapter.py` converts local adapter samples
or runs the converted suite:

```powershell
python ai/python/scripts/run_agent_dataset_adapter.py --run ai/python/fixtures/agent_eval/adapter_samples/qasper_like_rag_samples.json
```

`ai/python/scripts/run_agent_eval_regression.py` compares baseline and current
EvalReport-like JSON:

```powershell
python ai/python/scripts/run_agent_eval_regression.py ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json
```

Exit codes:

- `0`: report status `PASS`;
- `1`: report status `FAIL`;
- `2`: malformed or unsafe input.

## 5. Test Plan

### 5.1 Unit Tests

Contract tests:

- accepts valid synthetic suite;
- rejects non-synthetic fixture kind;
- rejects raw prompt-like fields;
- rejects backend connectivity fields;
- rejects duplicate case ids.

Evaluator tests:

- grounded RAG passes with visible citation;
- permission leakage fails;
- expected abstain passes;
- memory scope violation fails;
- unsafe tool output fails;
- state-diff mismatch fails;
- replay fails if side effect is reexecuted.
- expected negative scenarios pass when the expected failure is detected.
- adapter skeletons generate valid EvalCase suites.
- adapter runner converts local sample payloads and rejects sensitive fields.
- baseline comparison blocks aggregate and case-level regressions.
- MCP security scoring rejects provenance mismatch, unblocked poisoned
  descriptions and unquarantined output instructions.
- AgentRun trace includes context, memory, tool, workflow, runtime-control and
  failure steps.

### 5.2 Integration Tests

Integration tests:

- load `synthetic_first_trio.json`;
- load `synthetic_core_scenarios.json`;
- load `synthetic_runtime_control_scenarios.json`;
- load `synthetic_mcp_security_scenarios.json`;
- load `synthetic_context_evidence_scenarios.json`;
- load `synthetic_memory_admission_scenarios.json`;
- load `synthetic_state_diff_scenarios.json`;
- run `run_agent_eval_fixture.py`;
- verify report status, case count, failure distribution and `raw_payload_returned=false`.

### 5.3 Boundary Tests

Boundary is enforced by:

- `assert_no_forbidden_fields`;
- low-sensitive scanner in `nexusim_ai_common.safety`;
- Agent eval-specific forbidden fields such as `backend_url`,
  `production_endpoint`, `postgres_dsn`, `kafka_broker`, `redis_url`,
  `business_payload`, `execution_payload` and `provider_request_body`;
- fixture-only path and no backend imports.

## 6. Verification Commands

Focused gate:

```powershell
python -m pytest ai/python/tests/test_agent_eval_contracts.py ai/python/tests/test_agent_eval_evaluator.py ai/python/tests/test_agent_eval_integration.py -q
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_first_trio.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_core_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_runtime_control_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_mcp_security_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_context_evidence_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_memory_admission_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_state_diff_scenarios.json
python ai/python/scripts/run_agent_dataset_adapter.py --run ai/python/fixtures/agent_eval/adapter_samples/qasper_like_rag_samples.json
python ai/python/scripts/run_agent_eval_regression.py ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json
```

Full Python gate for this workspace:

```powershell
python -m pytest ai/python/tests -q
python -m ruff check ai/python
python -m mypy ai/python/nexusim_ai_common ai/python/nexusim_ai_memory ai/python/nexusim_ai_eval ai/python/scripts
```

Repo hygiene:

```powershell
git diff --check
git diff --cached --check
git status --short --branch --untracked-files=all
```

## 7. Next Coding Slices

Recommended next slices:

1. Harden memory admission with duplicate/dedupe, low-confidence, procedural
   skill-bound and policy-memory rejection cases.
2. Harden ContextPackage / EvidencePack with memory-vs-source precedence,
   unsafe tool output in context, token-budget truncation and unavailable
   retrieval lane cases.
3. Harden state-diff with repair/redrive, partial execution and idempotency
   cases.
4. Add current-report generation script for baseline refresh review.
5. Add runtime-control negative fixture pack for missing checkpoints and
   incomplete cancel propagation.

Each slice must keep the same isolation rule until an ADR explicitly promotes a
backend integration boundary.

## 8. Rejection Conditions

Reject further coding if a change:

- imports production backend service packages into the harness;
- requires real IM data to pass tests;
- opens network connections to production-like services;
- uses model output as eval ground truth;
- persists ACTIVE memory or business state;
- reexecutes side effects during replay;
- stores raw prompts, raw provider bodies, secrets or private payloads in reports.
