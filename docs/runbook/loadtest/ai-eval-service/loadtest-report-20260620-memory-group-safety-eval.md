# NexusIM Memory Group Safety Eval

Date: 2026-06-20

## Scope

This is a low-sensitive CI-safe fixture expansion for collaborative group memory
safety. It does not call models, databases, business services, external
providers, or production tools.

## Command

```powershell
.\tools\validate-ai-eval-cases.ps1

.\tools\run-ai-eval-profile-agent-safety.ps1 `
  -RunName ai-eval-memory-group-safety-20260620 `
  -OutputPath H:\NexusIM\loadtest-results\ai-eval-memory-group-safety-20260620\profile-agent-safety-eval-summary.json
```

Raw result:

```text
H:\NexusIM\loadtest-results\ai-eval-memory-group-safety-20260620\profile-agent-safety-eval-summary.json
```

## Result

```text
case_catalog_count = 56
collaborative_memory_cases = 3
adapter = profile-agent-output-safety
adapter_case_count = 9
failed_count = 0
```

New covered cases:

- `memory-source-ref-required-before-active`
- `memory-validity-window-current-only`
- `memory-supersession-chain-current-only`

## Verified Boundary

- ACTIVE collaborative memory requires source refs.
- Memory validity windows are preserved and out-of-window facts are not treated
  as current evidence.
- Supersession links are explicit, and old superseded memory is not returned as
  the current fact.

## Limits

This proves fixture-level regression coverage only. It is not a live
`memory-service` projection smoke, model-quality benchmark, long-running eval
platform, production SLO, or proof of targeted repair behavior.
