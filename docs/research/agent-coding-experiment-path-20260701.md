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
ai/python/scripts/run_agent_eval_current_report.py
ai/python/scripts/run_agent_eval_report_matrix.py
ai/python/scripts/run_agent_memory_calibration.py
ai/python/scripts/run_agent_dataset_adapter.py
ai/python/scripts/run_agent_eval_regression.py
ai/python/tests/test_agent_eval_*.py
ai/python/tests/test_agent_memory_calibration.py
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
  cross_service_preservation.py
  dataset_reproducibility.py
  comparison.py
  contracts.py
  evaluator.py
  fixtures.py
  memory_calibration.py
  reporting.py
  trace.py
ai/python/fixtures/agent_eval/adapter_samples/
ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json
ai/python/fixtures/agent_eval/report_matrix_sample.json
ai/python/fixtures/agent_eval/synthetic_first_trio.json
ai/python/fixtures/agent_eval/synthetic_core_scenarios.json
ai/python/fixtures/agent_eval/synthetic_runtime_control_scenarios.json
ai/python/fixtures/agent_eval/synthetic_runtime_control_negative_scenarios.json
ai/python/fixtures/agent_eval/synthetic_runtime_control_deeper_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_mcp_security_scenarios.json
ai/python/fixtures/agent_eval/synthetic_mcp_security_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_context_evidence_scenarios.json
ai/python/fixtures/agent_eval/synthetic_context_evidence_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_context_evidence_deeper_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_memory_admission_scenarios.json
ai/python/fixtures/agent_eval/synthetic_memory_admission_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_memory_admission_deeper_hardening_scenarios.json
ai/python/fixtures/agent_eval/memory_calibration_sample.json
ai/python/fixtures/agent_eval/memory_calibration_public_export.json
ai/python/fixtures/agent_eval/cross_service_preservation_rehearsal.json
ai/python/fixtures/agent_eval/dataset_reproducibility_rehearsal.json
ai/python/fixtures/agent_eval/synthetic_state_diff_scenarios.json
ai/python/fixtures/agent_eval/synthetic_state_diff_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_state_diff_deeper_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_replay_observability_scenarios.json
ai/python/scripts/run_agent_eval_fixture.py
ai/python/scripts/run_agent_eval_current_report.py
ai/python/scripts/run_agent_eval_report_matrix.py
ai/python/scripts/run_agent_memory_calibration.py
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
ai/python/tests/test_agent_eval_reporting.py
ai/python/tests/test_agent_memory_calibration.py
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
- Current EvalReport generation and baseline refresh review artifacts.
- Multi-suite current-report matrix, baseline refresh approval manifest and
  low-sensitive report retention metadata.
- fixture-only runtime-control coverage for cancel propagation, checkpointed
  approval resume and replay without side-effect reexecution.
- fixture-only runtime-control negative coverage for missing checkpoint,
  incomplete cancel propagation and incomplete replay event detection.
- fixture-only runtime-control deeper hardening coverage for checkpoint version
  drift detection, workflow wakeup race dedupe and ReplayBundle lineage
  completeness.
- fixture-only ReplayBundle observability skeleton for low-sensitive
  observability refs, hash refs, version metadata refs, failure taxonomy refs
  and trace linkage refs in EvalReport / ReplayBundle / AgentRunTrace.
- fixture-only MCP security coverage for poisoned tool descriptions, unsafe MCP
  output instructions, provider provenance mismatch and sandbox-only providers.
- fixture-only MCP security hardening coverage for tool argument schema mismatch,
  tool-selection attack blocking, prepare expiry detection and multi-candidate
  provider selection.
- ToolSandbox/MCP-Bench-like tool adapter alignment coverage for fixture-only
  capability lease refs, capability scope refs and provider attestation refs.
- fixture-only ContextPackage / EvidencePack coverage for source coverage,
  conflict markers, stale evidence avoidance and permission abstain.
- fixture-only ContextPackage / EvidencePack hardening coverage for
  memory-vs-current-source precedence, unsafe tool output quarantine,
  deterministic context-budget retention and unavailable retrieval lane gap
  reporting.
- fixture-only ContextPackage / EvidencePack deeper hardening coverage for
  source ranking, lane redrive, snippet-level citation repair, denied-lane
  handling and provider/tool/peer-agent taint propagation.
- fixture-only ContextPackage / EvidencePack adapter alignment coverage for
  Qasper / HotpotQA / BEIR-like public RAG samples, rerank confidence threshold
  refs, rerank explanation refs, denied-lane audit refs and taint vocabulary
  refs.
- fixture-only richer memory admission coverage for group speaker/audience,
  project supersedes, profile aggregate review, revoked memory blocking, stale
  memory blocking and overgeneralization prevention.
- fixture-only memory admission hardening coverage for duplicate dedupe,
  low-confidence rejection, procedural skill binding, policy-like memory
  rejection and review timeout metadata.
- fixture-only memory admission deeper hardening coverage for multi-source
  duplicate clustering, confidence calibration, procedural memory
  migration/invalidation, governed policy source allowlist/revocation and review
  retry/escalation/redrive.
- STATE-Bench/LoCoMO-like memory adapter alignment coverage for duplicate
  cluster representative selection and tie-break refs, confidence threshold refs
  and governed policy revocation window refs.
- fixture-only memory admission calibration coverage for confidence threshold
  recommendation, governed policy revocation-window retention selection and
  review backoff/operator queue recommendation.
- fixture-only public-export memory calibration coverage for dataset-source
  refs, per-dataset case counts, 15 memory gate cases, 8 policy-window cases
  and 12 review-backoff cases.
- fixture-only dataset reproducibility coverage for dataset manifest refs,
  license refs, snapshot hashes, split manifests, import hashes, adapter
  versions, deterministic report hashes and promotion blocking for changed or
  non-reproducible dataset evidence.
- fixture-only cross-service preservation coverage for retrieval, memory, MCP,
  workflow, executor and audit boundary refs, scope/version/taint/audit lineage
  and promotion blocking when required preservation evidence is dropped.
- fixture-only state-diff report coverage for approved action outcome refs,
  expected-vs-actual state changes, missing execution refs, incomplete reports
  and unauthorized mutation detection.
- fixture-only state-diff hardening coverage for repair/redrive lineage,
  partial execution detection, idempotency-preserved replay and compensating
  action refs.
- fixture-only state-diff deeper hardening coverage for state dependency graph,
  cross-action compensation chain and operator redrive review refs.

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

### 4.4 Baseline Comparison and Report Lifecycle

`nexusim_ai_eval.comparison` owns low-sensitive EvalReport comparison:

- suite/status/count deltas;
- aggregate score deltas;
- failure distribution deltas;
- case-level score/status deltas;
- blocked promotion reasons.

It accepts EvalReport-like JSON only. It does not read production data, execute
fixtures, call models or connect to backend services.

`nexusim_ai_eval.reporting` owns fixture-only report lifecycle artifacts:

- current EvalReport generation;
- baseline refresh review payloads;
- multi-suite current-report matrices;
- baseline refresh approval manifests;
- retention metadata for low-sensitive eval artifacts.

It never overwrites baselines directly. Refresh decisions remain manual
approval inputs until a later ADR promotes service integration.

`nexusim_ai_eval.memory_calibration` owns offline memory admission calibration:

- confidence threshold candidate scoring;
- governed policy revocation-window retention candidate scoring;
- review backoff/operator queue candidate scoring;
- recommendation refs and blocked promotion reasons.

It consumes only local low-sensitive calibration samples. It does not admit
ACTIVE memory, update baselines, call workflow-service or connect to a backend
queue.

### 4.5 AgentRun Trace

`nexusim_ai_eval.trace` owns deterministic trace skeletons:

- AgentRunTrace;
- AgentStep;
- EvidencePackFixture;
- ContextPackageFixture;
- MemoryCandidateFixture;
- ToolIntentFixture.
- ToolIntentFixture hardening metadata for argument schema mismatch,
  tool-selection attacks, expired prepare refs and multi-provider selection.
- ToolIntentFixture adapter alignment metadata for capability lease refs,
  capability scope refs and provider attestation refs.
- RuntimeControlFixture.
- RuntimeControlFixture deeper hardening metadata for checkpoint version refs,
  drift refs, workflow wakeup refs, wakeup race refs and replay lineage refs.
- ContextPackage source coverage, conflict, stale-source and permission-abstain
  metadata.
- ContextPackage hardening metadata for memory/source precedence, unsafe context
  blocks, context-budget retention and retrieval lane gaps.
- ContextPackage deeper hardening metadata for source ranking, lane redrive,
  snippet-level citation repair, denied lanes and taint propagation.
- MemoryCandidate source, speaker, audience, supersedes, stale-memory and
  review metadata.
- MemoryCandidate hardening metadata for duplicate dedupe, low confidence,
  skill binding, policy-like memory rejection and review timeout.
- MemoryCandidate deeper hardening metadata for duplicate clusters, calibrated
  confidence, procedural migration/invalidation, policy-source governance and
  review retry/escalation/redrive.
- MemoryCandidate adapter alignment metadata for cluster representative refs,
  deterministic tie-break refs, confidence threshold refs and policy revocation
  window refs.
- StateDiffReport execution, approval, prepare, state-change and audit refs.
- StateDiffReport hardening metadata for repair/redrive, partial execution,
  idempotency and compensating action refs.
- StateDiffReport deeper hardening metadata for state dependency graph,
  compensation chain and operator redrive review refs.

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
ai/python/fixtures/agent_eval/synthetic_runtime_control_negative_scenarios.json
ai/python/fixtures/agent_eval/synthetic_runtime_control_deeper_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_mcp_security_scenarios.json
ai/python/fixtures/agent_eval/synthetic_mcp_security_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_context_evidence_scenarios.json
ai/python/fixtures/agent_eval/synthetic_context_evidence_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_context_evidence_deeper_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_memory_admission_scenarios.json
ai/python/fixtures/agent_eval/synthetic_memory_admission_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_memory_admission_deeper_hardening_scenarios.json
ai/python/fixtures/agent_eval/memory_calibration_sample.json
ai/python/fixtures/agent_eval/memory_calibration_public_export.json
ai/python/fixtures/agent_eval/synthetic_state_diff_scenarios.json
ai/python/fixtures/agent_eval/synthetic_state_diff_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_state_diff_deeper_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_replay_observability_scenarios.json
ai/python/fixtures/agent_eval/adapter_samples/
ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json
```

These fixtures are intentionally synthetic or public-dataset-style exports.
They prove harness mechanics before any backend integration.

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
python ai/python/scripts/run_agent_dataset_adapter.py --run ai/python/fixtures/agent_eval/adapter_samples/toolsandbox_like_tool_samples.json
```

`ai/python/scripts/run_agent_eval_regression.py` compares baseline and current
EvalReport-like JSON:

```powershell
python ai/python/scripts/run_agent_eval_regression.py ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json
```

`ai/python/scripts/run_agent_eval_current_report.py` generates a current
EvalReport artifact and optional baseline refresh review without modifying the
baseline:

```powershell
python ai/python/scripts/run_agent_eval_current_report.py ai/python/fixtures/agent_eval/synthetic_core_scenarios.json --report-out .tmp-agent-current-report.json --baseline ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json --review-out .tmp-agent-baseline-review.json --force
```

`ai/python/scripts/run_agent_eval_report_matrix.py` runs a local report matrix
plan and writes per-suite current reports, baseline reviews, a multi-suite
matrix and a baseline refresh approval manifest:

```powershell
python ai/python/scripts/run_agent_eval_report_matrix.py ai/python/fixtures/agent_eval/report_matrix_sample.json --matrix-out .tmp-agent-eval-matrix/matrix.json --approval-manifest-out .tmp-agent-eval-matrix/approval-manifest.json --force
```

`ai/python/scripts/run_agent_memory_calibration.py` runs local low-sensitive
memory calibration samples and writes a recommendation report:

```powershell
python ai/python/scripts/run_agent_memory_calibration.py ai/python/fixtures/agent_eval/memory_calibration_sample.json --report-out .tmp-agent-memory-calibration-report.json --force
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
- state-diff deeper hardening scoring rejects missing dependency graph,
  compensation chain and operator redrive review refs;
- replay fails if side effect is reexecuted.
- expected negative scenarios pass when the expected failure is detected.
- adapter skeletons generate valid EvalCase suites.
- adapter runner converts local sample payloads and rejects sensitive fields.
- baseline comparison blocks aggregate and case-level regressions.
- MCP security scoring rejects provenance mismatch, unblocked poisoned
  descriptions and unquarantined output instructions.
- MCP security hardening scoring rejects undetected argument schema mismatch,
  undetected prepare expiry, unblocked tool-selection attack and bad provider
  selection.
- Tool / MCP adapter alignment scoring rejects missing capability leases and
  missing provider attestation refs.
- ContextPackage / EvidencePack deeper hardening scoring rejects missing source
  ranking, missing lane redrive refs, missing snippet citation repair, exposed
  denied lanes and missing taint propagation.
- ContextPackage / EvidencePack adapter alignment scoring rejects missing
  rerank confidence thresholds, missing rerank explanations, missing
  denied-lane audit refs and missing taint vocabulary refs.
- memory admission deeper hardening scoring rejects missing duplicate clusters,
  confidence calibration mismatch, missing procedural migration, revoked policy
  sources and missing review redrive refs.
- memory admission adapter alignment scoring rejects missing duplicate cluster
  representative refs, missing confidence threshold refs and missing policy
  revocation window refs.
- memory calibration scoring rejects confidence threshold, revocation-window
  and review backoff candidates that do not meet acceptance.
- dataset reproducibility scoring rejects production-data manifests, missing
  split manifests, backend-connected runs, non-deterministic reports, mismatched
  calibration export counts and changed snapshots allowed through promotion.
- cross-service preservation scoring rejects missing required boundaries,
  dropped role refs, scope widening, raw payload exposure and failed boundary
  promotion allowed through release gates.
- AgentRun trace includes context, memory, tool, workflow, runtime-control and
  failure steps.
- runtime-control deeper hardening scoring rejects stale checkpoint versions,
  unresolved duplicate workflow wakeups and incomplete replay lineage refs.

### 5.2 Integration Tests

Integration tests:

- load `synthetic_first_trio.json`;
- load `synthetic_core_scenarios.json`;
- load `synthetic_runtime_control_scenarios.json`;
- load `synthetic_runtime_control_negative_scenarios.json`;
- load `synthetic_runtime_control_deeper_hardening_scenarios.json`;
- load `synthetic_mcp_security_scenarios.json`;
- load `synthetic_mcp_security_hardening_scenarios.json`;
- load `synthetic_context_evidence_scenarios.json`;
- load `synthetic_context_evidence_hardening_scenarios.json`;
- load `synthetic_context_evidence_deeper_hardening_scenarios.json`;
- load `synthetic_memory_admission_scenarios.json`;
- load `synthetic_memory_admission_hardening_scenarios.json`;
- load `synthetic_memory_admission_deeper_hardening_scenarios.json`;
- load `synthetic_state_diff_scenarios.json`;
- load `synthetic_state_diff_hardening_scenarios.json`;
- load `synthetic_state_diff_deeper_hardening_scenarios.json`;
- run `run_agent_eval_fixture.py`;
- run `run_agent_eval_current_report.py`;
- run `run_agent_eval_report_matrix.py`;
- run `run_agent_memory_calibration.py`;
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
python -m pytest ai/python/tests/test_agent_eval_contracts.py ai/python/tests/test_agent_eval_evaluator.py ai/python/tests/test_agent_eval_trace.py ai/python/tests/test_agent_eval_integration.py ai/python/tests/test_agent_eval_reporting.py ai/python/tests/test_agent_memory_calibration.py -q
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_first_trio.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_core_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_runtime_control_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_runtime_control_negative_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_runtime_control_deeper_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_mcp_security_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_mcp_security_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_context_evidence_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_context_evidence_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_context_evidence_deeper_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_memory_admission_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_memory_admission_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_memory_admission_deeper_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_state_diff_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_state_diff_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_state_diff_deeper_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_replay_observability_scenarios.json
python ai/python/scripts/run_agent_eval_current_report.py ai/python/fixtures/agent_eval/synthetic_core_scenarios.json --report-out .tmp-agent-current-report.json --baseline ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json --review-out .tmp-agent-baseline-review.json --force
python ai/python/scripts/run_agent_eval_report_matrix.py ai/python/fixtures/agent_eval/report_matrix_sample.json --matrix-out .tmp-agent-eval-matrix/matrix.json --approval-manifest-out .tmp-agent-eval-matrix/approval-manifest.json --force
python ai/python/scripts/run_agent_memory_calibration.py ai/python/fixtures/agent_eval/memory_calibration_sample.json --report-out .tmp-agent-memory-calibration-report.json --force
python ai/python/scripts/run_agent_memory_calibration.py ai/python/fixtures/agent_eval/memory_calibration_public_export.json --report-out .tmp-agent-memory-calibration-public-export-report.json --force
python ai/python/scripts/run_agent_dataset_adapter.py --run ai/python/fixtures/agent_eval/adapter_samples/qasper_like_rag_samples.json
python ai/python/scripts/run_agent_dataset_adapter.py --run ai/python/fixtures/agent_eval/adapter_samples/toolsandbox_like_tool_samples.json
python ai/python/scripts/run_agent_dataset_adapter.py --run ai/python/fixtures/agent_eval/adapter_samples/statebench_like_memory_samples.json
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

1. Review whether the isolated Agent eval / replay / memory calibration
   skeleton is ready for ADR promotion decisions.
2. Review ReplayBundle observability hardening only if the current skeleton
   shows missing taxonomy / trace evidence in fixture-only gates.

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
