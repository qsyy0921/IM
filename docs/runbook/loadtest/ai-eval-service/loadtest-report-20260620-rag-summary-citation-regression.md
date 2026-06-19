# AI Eval RAG / Summary Citation Source-Ref Regression

Date: 2026-06-20

Scope: first-stage low-sensitive eval catalog and adapter assertion hardening.
This is not a production benchmark and not a live service-stack smoke by itself.

Changed coverage:

- Added active `rag-service` case `rag-citation-source-ref-integrity`.
- Added active `summary-service` case `summary-citation-source-ref-integrity`.
- Added assertion type `must_match_citation_source_ref`.
- Extended RAG / Summary smoke summaries with low-sensitive `citation_refs`.

Verified locally:

```powershell
.\tools\validate-ai-eval-cases.ps1
go test ./loadtest/rag ./loadtest/summary -count=1
go test ./services/rag-service/internal/app ./services/summary-service/internal/app -count=1
.\tools\check-powershell-scripts.ps1
.\tools\check-ai-eval-regression-gate.ps1
```

Result:

- Case catalog active count: 34
- Attribution family count: 7
- RAG / Summary adapters can now verify that a non-empty citation also matches
  the seeded EvidencePack source ref: `source_id`, `source_event_id`,
  `conversation_id`, `conversation_seq`, and non-empty `evidence_id`.

Boundary:

- No raw EvidencePack text, prompt, model output, user text, secret or tool input
  is stored in the new summary fields.
- `citation_refs` are low-sensitive IDs and sequence metadata for regression
  verification.
- A future live service-stack run is still required to refresh end-to-end RAG /
  Summary runtime evidence with these new cases.
