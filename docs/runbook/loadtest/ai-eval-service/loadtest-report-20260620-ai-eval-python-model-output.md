# AI Eval Python Model Output Negative Smoke

Date: 2026-06-20

Scope: first-stage low-sensitive `python-ai-worker` eval adapter. This is not a
production benchmark and not a model-quality claim.

Command shape:

```powershell
.\tools\run-ai-eval-python-worker-adapter.ps1 `
  -Python python `
  -RunName ai-eval-python-model-output-20260620 `
  -OutputPath H:\NexusIM\loadtest-results\ai-eval-python-model-output-20260620\python-worker-eval-summary.json
```

Raw summary:

```text
H:\NexusIM\loadtest-results\ai-eval-python-model-output-20260620\python-worker-eval-summary.json
```

Result:

- Adapter: `python-ai-worker`
- Case count: 5
- Passed cases:
  - `python-worker-malformed-output-fails-closed`
  - `python-worker-unsafe-output-fails-closed`
  - `python-worker-raw-output-field-rejected`
  - `python-worker-sensitive-citation-output-rejected`
  - `python-worker-malformed-hash-output-rejected`

Boundary:

- The CLI still returns low-sensitive `FAILED` candidates for malformed or
  unsafe inputs.
- Go-side Python runner rejects candidate outputs that include forbidden raw
  model bodies, sensitive citation metadata, or malformed output hashes.
- This smoke does not call an external provider, database, business table or
  service stack.
