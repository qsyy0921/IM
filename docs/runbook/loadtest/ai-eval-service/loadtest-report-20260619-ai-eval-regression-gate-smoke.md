# ai-eval-service Multi-Adapter Regression Gate Smoke

Date: 2026-06-19

Scope: first-stage low-sensitive multi-adapter gate smoke. This is a local gate
skeleton, not a production CI gate, model-quality benchmark or long-running eval
platform.

Command:

```powershell
.\tools\validate-ai-eval-gate-policy.ps1
.\tools\run-ai-eval-regression-gate-smoke.ps1 `
  -OptionalAdapter python-ai-worker `
  -Python C:\Users\10495\anaconda3\envs\IM\python.exe
```

Raw result root:

```text
H:\NexusIM\loadtest-results\ai-eval-regression-gate-smoke-20260619-152028
```

Gate policy:

```text
docs/runbook/ai-eval/gate-policy.local.json
```

Verified chain:

```text
profile / Agent safety eval summary
action-executor external HTTP adapter eval summary
python-ai-worker output-safety eval summary
-> ai-eval-service RecordEvalRun for each adapter
-> GetEvalRun / ListEvalRuns for each adapter
-> low-sensitive suite-level gate summary
```

Observed result:

```text
status = passed
adapter_count = 3
selected_optional_adapters = [python-ai-worker]
case_count = 6
passed_count = 6
failed_count = 0
skipped_count = 0

profile-agent-safety:
  suite_id = ai-eval-profile-agent-output-safety
  status = PASSED
  case_count = 2

action-external-http-provider:
  suite_id = ai-eval-action-external-http-provider
  status = PASSED
  case_count = 2

python-ai-worker:
  suite_id = ai-eval-python-ai-worker
  status = PASSED
  case_count = 2
```

Boundary:

- Stored catalog data is limited to refs, counters and low-sensitive metadata.
- The gate summary stores no raw prompt, EvidencePack, model output, user
  content, secret, tool input or provider response body.
- Raw smoke JSON remains outside the repository on `H:`.
