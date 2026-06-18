# NexusIM AI Eval Runbook

This folder stores first-stage, low-sensitive eval cases for retrieval, RAG,
summary, Agent and tool/action boundaries.

## Current Scope

- Case file: `retrieval-eval-cases.json`
- Validator: `tools/validate-ai-eval-cases.ps1`
- Scope: schema and assertion coverage only; not a production benchmark, not a
  model-quality claim, and not a long-running eval platform.

## Case Rules

- Use synthetic or low-sensitive text only.
- Every case must have a stable `id`, `family`, `stage`, `query`, `risk` and
  at least one `required_assertions` entry.
- Cases must test failure classes that matter to RAG / Agent safety:
  retrieval miss, temporal version, attribution, permission leak, profile
  overgeneralization, tool policy violation and action execution safety.
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

Future RAG / Agent slices should add execution adapters that evaluate these
cases against real EvidencePack outputs before making model-quality claims.
