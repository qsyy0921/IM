# ai-eval group-memory ambiguity safety expansion

Date: 2026-06-25

Scope: low-sensitive local `profile-agent-output-safety` adapter expansion.
This is not a production benchmark and does not call models, databases or
business services.

## Architecture Analysis

Research signal:

- GroupMemBench frames group memory as more than concatenated one-on-one chat:
  evaluation must bind answers to a specific asker, preserve group dynamics,
  speaker-grounded beliefs, audience-adapted language and abstention behavior.
- EverMemBench highlights the same production-shaped failures: multi-party /
  multi-group attribution, temporal updates and profile understanding fail when
  retrieval flattens structure or uses similarity-only evidence.

NexusIM placement:

- Case owner: `ai-eval-service` catalog plus local
  `profile-agent-output-safety` adapter.
- Source chain owner: `memory-service` / `retrieval-gateway`; this slice only
  adds CI-safe contract cases and fixture checks.
- Temporal / visibility boundary: missing visibility projection must fail
  closed instead of returning stale local cache or fake evidence.
- Profile boundary: audience-adapted wording in one group remains review-bound
  and must not become a global style profile.
- Middleware impact: none. Existing local eval harness is enough.

## Added Cases

The active local fixture suite increased from 20 to 24 cases:

- `memory-asker-term-ambiguity-scope`
- `memory-visible-chain-incomplete-abstains`
- `memory-missing-visibility-projection-fails-closed`
- `profile-audience-language-not-global-style`

These cases cover asker-bound term disambiguation, incomplete visible-chain
abstention, visibility-projection fail-closed behavior, unsupported memory
fallback rejection, raw prompt non-persistence and audience-language profile
overgeneralization.

## Verification

Command:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tools\validate-ai-eval-cases.ps1 `
  -OutputPath $env:TEMP\nexusim-ai-eval-cases-check.json

powershell -NoProfile -ExecutionPolicy Bypass -File tools\run-ai-eval-profile-agent-safety.ps1 `
  -ResultRoot H:\NexusIM\loadtest-results `
  -RunName ai-eval-group-memory-ambiguity-safety-dev `
  -OutputPath H:\NexusIM\loadtest-results\ai-eval-group-memory-ambiguity-safety-dev\profile-agent-safety-eval-summary.json
```

Result:

```text
validate-ai-eval-cases.ps1: OK
profile-agent-output-safety: 24 cases, 24 passed, 0 failed
```

Raw summary:

```text
H:\NexusIM\loadtest-results\ai-eval-group-memory-ambiguity-safety-dev\profile-agent-safety-eval-summary.json
```

## Boundary

- This proves low-sensitive contract coverage, not live service-stack quality.
- It does not run LLM providers, retrieval providers or PostgreSQL.
- Later service-stack adapters must prove the same contract against
  `memory-service`, `retrieval-gateway`, RAG, Summary and Agent runtime paths.
