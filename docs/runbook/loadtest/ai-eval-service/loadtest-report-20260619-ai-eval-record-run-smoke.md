# ai-eval-service RecordEvalRun Smoke

Date: 2026-06-19

Scope: first-stage low-sensitive recorder smoke. This is not a production eval
platform, model-quality benchmark or CI gate.

Command:

```powershell
.\tools\run-ai-eval-record-run-smoke.ps1
```

Raw result root:

```text
H:\NexusIM\loadtest-results\ai-eval-record-run-smoke-20260619-143529
```

Verified chain:

```text
profile / Agent safety eval summary
-> ai-eval-service RecordEvalRun
-> GetEvalRun
-> ListEvalRuns
```

Observed result:

```text
run_id = ai-eval-record-run-smoke-20260619-143529-profile-agent-safety
suite_id = ai-eval-profile-agent-output-safety
stage = memory-profile-safety
adapter = profile-agent-output-safety
status = PASSED
case_count = 2
passed_count = 2
failed_count = 0
skipped_count = 0
get_run_matched = true
list_contains_run = true
```

Boundary:

- Stored catalog data is limited to summary refs, counts and low-sensitive
  metadata.
- Raw prompt, EvidencePack, model output, user content, secrets and tool input
  are not stored in `ai_eval_runs`.
- Raw smoke JSON remains outside the repository on `H:`.
