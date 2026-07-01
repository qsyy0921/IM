# NexusIM Python AI Worker Foundation

This directory is the Python side of ADR-036.

Python workers are candidate generators only. Go services keep authority over
tenant identity, policy, approval, audit, persistence, outbox and repair.

## Allowed Work

- LLM provider adapter candidates.
- Embedding and rerank candidates.
- Memory extraction and profile aggregation candidates.
- Planner / critic / verifier prototypes.
- Offline eval and benchmark processing.

## Forbidden Work

- Direct reads or writes of NexusIM business PostgreSQL tables.
- Final approval, execution, memory, summary, answer or proposal state.
- High-risk business actions.
- Persisting raw prompts, raw provider bodies, secrets, tokens or sensitive
  payloads.

## First-Stage Layout

```text
ai/python/
  environment.yml
  pyproject.toml
  nexusim_ai_common/
    contracts.py
    safety.py
  nexusim_ai_memory/
    extractor.py
  nexusim_ai_eval/
    adapters.py
    adapter_runner.py
    comparison.py
    contracts.py
    evaluator.py
    fixtures.py
    trace.py
  fixtures/
    agent_eval/
      adapter_samples/
      baselines/
      synthetic_first_trio.json
      synthetic_core_scenarios.json
      synthetic_runtime_control_scenarios.json
      synthetic_runtime_control_negative_scenarios.json
      synthetic_mcp_security_scenarios.json
      synthetic_context_evidence_scenarios.json
      synthetic_context_evidence_hardening_scenarios.json
      synthetic_memory_admission_scenarios.json
      synthetic_memory_admission_hardening_scenarios.json
      synthetic_state_diff_scenarios.json
      synthetic_state_diff_hardening_scenarios.json
  contracts/
    worker-candidate.schema.json
  scripts/
    run_candidate_worker.py
    run_memory_extraction_candidate.py
    run_agent_eval_fixture.py
    run_agent_dataset_adapter.py
    run_agent_eval_regression.py
    validate_contracts.py
  tests/
```

## Local Toolchain

All Python tools for this repo should run inside the `IM` conda environment:

```powershell
conda env create -f ai/python/environment.yml
conda activate IM
python -m pytest ai/python/tests -q
python ai/python/scripts/validate_contracts.py
python ai/python/scripts/run_candidate_worker.py <low-sensitive-request.json>
python ai/python/scripts/run_memory_extraction_candidate.py <low-sensitive-message-batch.json>
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_first_trio.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_runtime_control_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_runtime_control_negative_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_mcp_security_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_context_evidence_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_context_evidence_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_memory_admission_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_memory_admission_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_state_diff_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_state_diff_hardening_scenarios.json
python ai/python/scripts/run_agent_dataset_adapter.py --run ai/python/fixtures/agent_eval/adapter_samples/qasper_like_rag_samples.json
python ai/python/scripts/run_agent_eval_regression.py ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json
python -m ruff check ai/python
python -m mypy ai/python/nexusim_ai_common ai/python/nexusim_ai_memory ai/python/nexusim_ai_eval ai/python/scripts
```

If the environment already exists, update it instead:

```powershell
conda env update -n IM -f ai/python/environment.yml
```

The environment installs `nexusim-ai-workers` in editable mode and keeps
Python tooling out of the system Python installation.

## Boundary Guard

Run this repo-level guard when changing Python worker foundations:

```powershell
.\tools\check-python-ai-worker-boundary.ps1
.\tools\run-python-ai-worker-smoke.ps1 -Python C:\Users\10495\anaconda3\envs\IM\python.exe
.\tools\run-python-memory-extraction-smoke.ps1 -Python C:\Users\10495\anaconda3\envs\IM\python.exe
.\tools\run-ai-eval-python-worker-adapter.ps1 -Python C:\Users\10495\anaconda3\envs\IM\python.exe
.\tools\run-ai-eval-memory-extraction-candidate-adapter.ps1 -Python C:\Users\10495\anaconda3\envs\IM\python.exe
go run ./tools/python-worker-go-adapter-smoke -python C:\Users\10495\anaconda3\envs\IM\python.exe
go run ./tools/memory-extraction-go-adapter-smoke -python C:\Users\10495\anaconda3\envs\IM\python.exe
go run ./services/rag-service/cmd/rag-python-worker-provider-smoke -python C:\Users\10495\anaconda3\envs\IM\python.exe
go run ./services/summary-service/cmd/summary-python-worker-provider-smoke -python C:\Users\10495\anaconda3\envs\IM\python.exe
go run ./services/agent-service/cmd/agent-python-worker-provider-smoke -python C:\Users\10495\anaconda3\envs\IM\python.exe
```

The guard is intentionally small. It protects the first-stage boundary:
candidate-only Python, Go-owned control plane, no direct IM PostgreSQL table
access from Python workers, and a reproducible `IM` conda environment.

The worker smoke, eval adapter and Go-side adapter smoke prove only local
candidate contract safety: malformed / unsafe inputs fail closed, successful
candidates return hashes and source refs rather than raw output text, and Go
consumes candidate metadata rather than delegating control to Python. The
Python eval adapter also covers Go-side rejection of forbidden `raw_output`,
sensitive citation metadata and malformed output hashes.

The memory extraction candidate Go adapter smoke proves Go can invoke the
batch memory extraction CLI, reject unsafe input before worker execution, accept
only hash-only `MEMORY_EVENT_CANDIDATE` metadata, keep ordinary chat at zero
candidates and require review for group-scoped profile signals. The matching
ai-eval adapter records those four low-sensitive cases without databases,
providers or business writes.

The RAG, summary and Agent provider smokes prove services can wrap Go-owned
providers with a Python worker candidate guard while final state, citations,
approval, audit and failure handling stay in Go.

## Agent Eval Harness

`nexusim_ai_eval` is the first isolated Agent-layer coding experiment. It is a
deterministic offline harness for public-dataset adapters and synthetic IM-like
fixtures. It does not call NexusIM backend services, model providers, databases,
Kafka, Redis, OpenSearch, MCP providers or action executors.

The current fixture is:

```text
ai/python/fixtures/agent_eval/synthetic_first_trio.json
ai/python/fixtures/agent_eval/synthetic_core_scenarios.json
ai/python/fixtures/agent_eval/synthetic_runtime_control_scenarios.json
ai/python/fixtures/agent_eval/synthetic_runtime_control_negative_scenarios.json
ai/python/fixtures/agent_eval/synthetic_mcp_security_scenarios.json
ai/python/fixtures/agent_eval/synthetic_context_evidence_scenarios.json
ai/python/fixtures/agent_eval/synthetic_context_evidence_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_memory_admission_scenarios.json
ai/python/fixtures/agent_eval/synthetic_memory_admission_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_state_diff_scenarios.json
ai/python/fixtures/agent_eval/synthetic_state_diff_hardening_scenarios.json
ai/python/fixtures/agent_eval/adapter_samples/
ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json
```

The first trio covers:

- grounded RAG citation / permission check;
- group memory admission scope check;
- tool poisoning / unsafe output quarantine check.

The core scenarios fixture extends this to insufficient evidence abstain,
permission leakage detection, memory pollution/revocation, unsafe tool output,
approval timeout, provider timeout, state-diff mismatch and bounded handoff.
The runtime-control fixture covers cancel propagation, approval resume from a
checkpoint and replay without side-effect reexecution.
The runtime-control negative fixture covers missing checkpoint, incomplete
cancel propagation and incomplete replay event detection.
The MCP security fixture covers poisoned tool descriptions, unsafe MCP output
instructions, provider provenance mismatch and sandbox-only provider handling.
The context/evidence fixture covers source coverage, conflict marker detection,
stale evidence avoidance and permission-driven abstain recommendation.
The context/evidence hardening fixture covers memory-vs-current-source
precedence, unsafe tool output quarantine before context reuse, deterministic
context-budget retention and unavailable retrieval lane gap reporting.
The memory admission fixture covers group speaker/audience attribution, project
supersedes lineage, profile aggregate review, revoked memory blocking, stale
memory blocking and overgeneralization prevention.
The memory admission hardening fixture covers duplicate/near-duplicate dedupe,
low-confidence rejection, procedural skill binding, policy-like memory rejection
and review timeout metadata.
The state-diff fixture covers approved action outcome reports, expected-vs-actual
state changes, execution refs, audit refs, incomplete reports and unauthorized
mutation detection.
The state-diff hardening fixture covers repair/redrive lineage, partial
execution detection, idempotency-preserved replay and compensating action refs.
`nexusim_ai_eval.adapters` also includes low-sensitive skeleton adapters for
Qasper/HotpotQA-like RAG, ToolSandbox/tau-bench-like tool cases and
STATE-Bench/LoCoMo-like memory cases. `nexusim_ai_eval.trace` builds a
deterministic AgentRun / AgentStep trace skeleton with ContextPackage,
EvidencePack, MemoryCandidate and ToolIntent fixture refs.
`nexusim_ai_eval.adapter_runner` converts local public-dataset-style sample
payloads into validated EvalSuite JSON, and `nexusim_ai_eval.comparison`
compares low-sensitive EvalReports against a baseline for regression blocking.

Run it directly:

```powershell
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_first_trio.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_core_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_runtime_control_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_runtime_control_negative_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_mcp_security_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_context_evidence_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_context_evidence_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_memory_admission_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_memory_admission_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_state_diff_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_state_diff_hardening_scenarios.json
python ai/python/scripts/run_agent_dataset_adapter.py --run ai/python/fixtures/agent_eval/adapter_samples/qasper_like_rag_samples.json
python ai/python/scripts/run_agent_dataset_adapter.py --run ai/python/fixtures/agent_eval/adapter_samples/toolsandbox_like_tool_samples.json
python ai/python/scripts/run_agent_dataset_adapter.py --run ai/python/fixtures/agent_eval/adapter_samples/statebench_like_memory_samples.json
python ai/python/scripts/run_agent_eval_regression.py ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json
python -m pytest ai/python/tests/test_agent_eval_contracts.py ai/python/tests/test_agent_eval_evaluator.py ai/python/tests/test_agent_eval_trace.py ai/python/tests/test_agent_eval_integration.py -q
```

The CLI output is a low-sensitive `EvalReport` with per-case `ReplayBundle`
metadata. It includes refs and hashes only; it must not include raw prompts,
raw provider bodies, secrets, real IM messages or production endpoint fields.

## Memory Extraction Candidate

`nexusim_ai_memory.extractor` is the first concrete memory extraction candidate
module. It accepts an explicit low-sensitive message batch and only extracts
messages with clear memory cues:

```text
decision:
task:
status:
blocker:
file:
profile_signal:
```

Ordinary chat produces zero candidates. `profile_signal` candidates are marked
`NEEDS_REVIEW` and `GROUP_SCOPE_PROFILE_SIGNAL`; they must not become active
profile facts without Go-side validation and review. The CLI output is
hash-only: it includes candidate hashes, source refs, citation refs, speaker and
message hashes, event type metadata and low-sensitive counts, but it does not
return raw message text or persist memory facts.
