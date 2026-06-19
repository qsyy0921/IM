# NexusIM Agent output regression

Date: 2026-06-20

Scope: first-stage, low-sensitive Agent provider output regression. This covers
the `agent-service` Python worker proposal-provider boundary only. It is not a
model-quality benchmark, not a production Agent evaluation and not a live
business-action execution test.

## Result

Passed.

Command:

```powershell
.\tools\run-ai-eval-agent-output-regression.ps1 `
  -Python C:\Users\10495\anaconda3\envs\IM\python.exe `
  -RunName agent-output-regression-20260620 `
  -OutputPath H:\NexusIM\loadtest-results\agent-output-regression-20260620\agent-output-regression-summary.json
```

Optional gate command:

```powershell
.\tools\run-ai-eval-regression-gate-smoke.ps1 `
  -OptionalAdapter agent-python-worker-provider `
  -Python C:\Users\10495\anaconda3\envs\IM\python.exe `
  -RunName ai-eval-agent-output-optional-20260620 `
  -NoApplyMigration
```

Raw summaries:

```text
H:\NexusIM\loadtest-results\agent-output-regression-20260620\agent-output-regression-summary.json
H:\NexusIM\loadtest-results\ai-eval-agent-output-optional-20260620\ai-eval-regression-gate-summary.json
```

## Covered Cases

The adapter validates four active `agent-service-python-worker-provider` cases:

| Case | Expected result |
| --- | --- |
| `agent-python-worker-provider-success` | Python candidate accepted; Go-owned proposal remains grounded and citation-backed |
| `agent-python-worker-hash-mismatch-rejected` | candidate rejected with `HASH_MISMATCH` |
| `agent-python-worker-citation-mismatch-rejected` | candidate rejected with `CITATION_MISMATCH` |
| `agent-python-worker-failure-rejected` | worker failure rejected as `WORKER_FAILED` |

All cases verify:

- no raw output is returned;
- no business write is performed;
- no external provider is called by this smoke;
- final proposal authority remains in Go, with Python limited to candidate metadata.

## Validation

```powershell
go run ./services/agent-service/cmd/agent-python-worker-provider-smoke `
  -python C:\Users\10495\anaconda3\envs\IM\python.exe

go test ./services/agent-service/cmd/agent-python-worker-provider-smoke ./services/agent-service/internal/app -count=1
.\tools\run-ai-eval-agent-output-regression.ps1 -Python C:\Users\10495\anaconda3\envs\IM\python.exe
.\tools\validate-ai-eval-cases.ps1
.\tools\validate-ai-eval-gate-policy.ps1
```

Observed case catalog after this slice:

```text
case_count = 42
python_worker_output_safety = 9
```

## Boundary

This proves command-level Agent provider output regression for Python worker
candidate success, hash mismatch, citation mismatch and worker failure. It does
not prove full live Agent service-stack behavior, external LLM quality, real MCP
execution, long-horizon reasoning or production safety coverage.
