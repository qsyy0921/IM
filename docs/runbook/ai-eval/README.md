# NexusIM AI Eval Runbook

This folder stores first-stage, low-sensitive eval cases for retrieval, RAG,
summary, Agent and tool/action boundaries.

## Current Scope

- Case file: `retrieval-eval-cases.json`
- Validator: `tools/validate-ai-eval-cases.ps1`
- RAG execution adapter: `tools/run-ai-eval-rag-adapter.ps1`
- Agent execution adapter: `tools/run-ai-eval-agent-adapter.ps1`
- Profile / Agent output safety adapter:
  `tools/run-ai-eval-profile-agent-safety.ps1`
- action-executor external HTTP adapter eval:
  `tools/run-ai-eval-action-external-adapter.ps1`
- Python worker output-safety adapter: `tools/run-ai-eval-python-worker-adapter.ps1`
- ai-eval-service recorder smoke: `tools/run-ai-eval-record-run-smoke.ps1`
- ai-eval-service gate policy manifest:
  `docs/runbook/ai-eval/gate-policy.local.json`
- ai-eval-service gate policy validator:
  `tools/validate-ai-eval-gate-policy.ps1`
- ai-eval-service multi-adapter gate smoke:
  `tools/run-ai-eval-regression-gate-smoke.ps1`
- Go-side Python worker adapter smoke: `tools/python-worker-go-adapter-smoke`
- rag-service service-level Python worker provider smoke:
  `services/rag-service/cmd/rag-python-worker-provider-smoke`
- summary-service service-level Python worker provider smoke:
  `services/summary-service/cmd/summary-python-worker-provider-smoke`
- agent-service service-level Python worker provider smoke:
  `services/agent-service/cmd/agent-python-worker-provider-smoke`
- Scope: first-stage schema + local execution coverage only; not a production
  benchmark, not a model-quality claim, and not a long-running eval platform.

## Case Rules

- Use synthetic or low-sensitive text only.
- Every case must have a stable `id`, `family`, `stage`, `query`, `risk` and
  at least one `required_assertions` entry.
- Cases must test failure classes that matter to RAG / Agent safety:
  retrieval miss, temporal version, attribution, permission leak, profile
  overgeneralization, LLM output safety, Python worker output safety,
  tool policy violation and action execution safety.
- Do not include raw message bodies, secrets, bearer tokens, emails or phone
  numbers.

## Validation

```powershell
.\tools\validate-ai-eval-cases.ps1
```

Optional report:

```powershell
.\tools\validate-ai-eval-cases.ps1 `
  -MarkdownPath H:\NexusIM\loadtest-results\ai-eval\ai-eval-cases.md
```

First-stage RAG execution adapter:

```powershell
.\tools\run-ai-eval-rag-adapter.ps1 `
  -PGDSN postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable `
  -RAGTarget 127.0.0.1:10610
```

This adapter runs `loadtest/rag`, which seeds low-sensitive search / memory
projection rows, calls real `rag-service AnswerQuestion`, and validates active
`rag-service` cases against the returned answer, citations and EvidencePack.
It requires `rag-service`, `retrieval-gateway`, `search-service` and
`memory-service` runtime processes to be reachable. Raw execution summaries
stay under `H:\NexusIM\loadtest-results`.

Future Agent slices should add execution adapters that evaluate tool policy,
proposal / approval and action safety against real EvidencePack outputs before
making model-quality or agent-safety claims.

First-stage Agent execution adapter:

```powershell
.\tools\run-ai-eval-agent-adapter.ps1 `
  -PGDSN postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable `
  -AgentTarget 127.0.0.1:10630 `
  -ActionExecutorTarget 127.0.0.1:10660
```

This adapter runs `loadtest/agent`, which seeds low-sensitive search / memory /
skill / policy rows, calls real `agent-service CreateAgentProposal` and
`ApproveAgentProposal`, calls real `action-executor ExecuteApprovedAction`, and
validates active Agent / action-executor cases against the returned proposal,
approval, execution response, low-sensitive audit rows and low-sensitive tool
result projection. For cases that require `must_execute_safe_local_tool`, it
runs a second low-sensitive `nexusim.local.echo` path and verifies `SUCCEEDED`
plus output hash only. It proves the proposal / approval / executor / audit /
result-projection boundary and local safe tool output path only. It still does
not execute external MCP/provider tools.

First-stage profile overgeneralization / Agent output safety adapter:

```powershell
.\tools\run-ai-eval-profile-agent-safety.ps1
```

This adapter uses a low-sensitive local fixture to verify that a single
group-scoped memory fact is not promoted into an ACTIVE personal profile, that
profile candidates stay `PENDING_REVIEW` until multi-source support and review
exist, and that Agent output rejects raw EvidencePack text, secret-like content
and unapproved business actions. It does not call models, databases or business
services.

First-stage action-executor external HTTP adapter eval:

```powershell
.\tools\run-ai-eval-action-external-adapter.ps1
```

This adapter runs a local `httptest` provider fixture through the real
action-executor app usecase and external HTTP adapter. It verifies allowlisted
LOW-risk success, stable provider failure classification, unsafe output
suppression, raw input non-disclosure and output-hash-only projection. It does
not call real external networks, arbitrary MCP servers or production tools.

First-stage Python worker output-safety adapter:

```powershell
.\tools\run-ai-eval-python-worker-adapter.ps1 `
  -Python C:\Users\10495\anaconda3\envs\IM\python.exe
```

This adapter calls the local candidate-only Python worker CLI and verifies that
malformed and unsafe inputs return low-sensitive `FAILED` candidates. It does
not call external providers, databases or Go services.

First-stage `ai-eval-service` RecordEvalRun recorder smoke:

```powershell
.\tools\run-ai-eval-record-run-smoke.ps1
```

By default this smoke runs the low-sensitive profile / Agent output safety
adapter, writes the resulting summary under `H:\NexusIM\loadtest-results`, then
records only the summary reference, counters and low-sensitive metadata through
`ai-eval-service` `RecordEvalRun`. It verifies the run can be read back through
`GetEvalRun` and `ListEvalRuns`. It applies the first ai-eval migration locally
if needed and requires PostgreSQL through `-PGDSN` / `NEXUSIM_PG_DSN`.

To record any existing low-sensitive adapter summary:

```powershell
.\tools\run-ai-eval-record-run-smoke.ps1 `
  -SummaryPath H:\NexusIM\loadtest-results\<run>\adapter-summary.json
```

First-stage multi-adapter regression gate smoke:

```powershell
.\tools\validate-ai-eval-gate-policy.ps1
.\tools\run-ai-eval-regression-gate-smoke.ps1
```

The gate policy manifest declares required adapters, minimum case count, maximum
failure count, `GetEvalRun` / `ListEvalRuns` readback requirements, forbidden
persisted fields, and optional service-stack adapters for later RAG / Agent /
Python worker coverage. The smoke runs the required profile / Agent output
safety and action-executor external HTTP adapter evals, records both summaries
into `ai-eval-service`, then writes a low-sensitive suite-level gate summary. It
is a local regression gate skeleton, not a production CI gate and not a
model-quality benchmark.

Optional adapters are opt-in so normal local gates do not require a full service
stack:

```powershell
.\tools\run-ai-eval-regression-gate-smoke.ps1 `
  -OptionalAdapter python-ai-worker `
  -Python C:\Users\10495\anaconda3\envs\IM\python.exe
```

`rag-service` and `agent-action-executor` can also be selected through
`-OptionalAdapter`, but they require their listed service stacks and targets to
already be running. The gate runner records any selected optional adapter through
`ai-eval-service` with the same low-sensitive summary-only boundary.

RAG / Agent service-stack gate wrapper:

```powershell
.\tools\run-ai-eval-service-stack-gate-smoke.ps1 -PreflightOnly -AllowMissing
.\tools\run-ai-eval-service-stack-gate-smoke.ps1
```

The preflight writes endpoint readiness only. It does not prove live RAG / Agent
adapter behavior. Remove `-PreflightOnly` and `-AllowMissing` only after the
RAG / Agent service stack is running.
The first local live run passed with `profile-agent-safety`,
`action-external-http-provider`, `rag-service` and `agent-action-executor`
selected; see `docs/runbook/loadtest/ai-eval-service/`.

First-stage Go-side Python worker adapter smoke:

```powershell
go run ./tools/python-worker-go-adapter-smoke `
  -python C:\Users\10495\anaconda3\envs\IM\python.exe
```

This adapter proves Go can invoke the Python candidate CLI and consume only the
validated candidate metadata / output hash. It still does not call external
providers, databases or business services.

First-stage `rag-service` service-level Python provider smoke:

```powershell
go run ./services/rag-service/cmd/rag-python-worker-provider-smoke `
  -python C:\Users\10495\anaconda3\envs\IM\python.exe
```

This smoke proves `rag-service` can wrap its Go-owned answer provider with the
Python worker candidate guard while final answer state and citation checks stay
in Go.

First-stage `summary-service` service-level Python provider smoke:

```powershell
go run ./services/summary-service/cmd/summary-python-worker-provider-smoke `
  -python C:\Users\10495\anaconda3\envs\IM\python.exe
```

This smoke proves `summary-service` can wrap its Go-owned summary provider with
the Python worker candidate guard while final summary state and citation checks
stay in Go.

First-stage `agent-service` service-level Python provider smoke:

```powershell
go run ./services/agent-service/cmd/agent-python-worker-provider-smoke `
  -python C:\Users\10495\anaconda3\envs\IM\python.exe
```

This smoke proves `agent-service` can wrap its Go-owned proposal provider with
the Python worker candidate guard while final proposal state, citations,
approval and audit stay in Go.
