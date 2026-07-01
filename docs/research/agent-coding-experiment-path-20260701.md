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
  contracts.py
  evaluator.py
  fixtures.py
ai/python/fixtures/agent_eval/synthetic_first_trio.json
ai/python/scripts/run_agent_eval_fixture.py
```

Implemented tests:

```text
ai/python/tests/test_agent_eval_contracts.py
ai/python/tests/test_agent_eval_evaluator.py
ai/python/tests/test_agent_eval_integration.py
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

### 4.3 Fixtures

`nexusim_ai_eval.fixtures` loads and validates JSON suites. The fixture kind must
be `synthetic_im_like`.

Current fixture:

```text
ai/python/fixtures/agent_eval/synthetic_first_trio.json
```

This fixture is intentionally tiny. It proves harness mechanics before adding
larger public dataset adapters.

### 4.4 CLI

`ai/python/scripts/run_agent_eval_fixture.py` runs one suite and emits a
low-sensitive report:

```powershell
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_first_trio.json
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

### 5.2 Integration Tests

Integration tests:

- load `synthetic_first_trio.json`;
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

1. Add public dataset adapter skeletons for Qasper/HotpotQA-like RAG cases.
2. Add ToolSandbox/tau-bench-style tool adapter skeleton with fake state.
3. Add STATE-Bench/LoCoMo-like memory adapter skeleton.
4. Add approval wait and timeout fixture.
5. Add state-diff report section for fake action execution.
6. Add malicious MCP/tool description fixture pack.
7. Add baseline comparison between EvalReports.

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
