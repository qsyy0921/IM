# AI Eval Summary Negative Smoke

Date: 2026-06-20

Scope: first-stage low-sensitive `summary-service` eval adapter. This is not a
production benchmark and not a model-quality claim.

Command shape:

```powershell
.\tools\run-ai-eval-summary-adapter.ps1 `
  -RunName ai-eval-summary-negative-20260620 `
  -OutputPath H:\NexusIM\loadtest-results\ai-eval-summary-negative-20260620\summary-eval-adapter-summary.json
```

Runtime note: `summary-service` was started locally in `grpc` mode with the
extractive provider; existing local retrieval / search / memory / PostgreSQL
endpoints were used.

Raw summary:

```text
H:\NexusIM\loadtest-results\ai-eval-summary-negative-20260620\summary-eval-adapter-summary.json
```

Result:

- Adapter: `summary-service`
- Case count: 2
- Passed cases:
  - `summary-grounded-citations`
  - `summary-insufficient-evidence-abstains`
- Negative path: no visible evidence returns `INSUFFICIENT_EVIDENCE`, an empty
  EvidencePack, no citations and `generated_by_llm=false`.

Boundary:

- No raw prompt, raw EvidencePack, raw model output or user content is stored in
  this repo report.
- Raw execution JSON remains under `H:\NexusIM\loadtest-results`.
