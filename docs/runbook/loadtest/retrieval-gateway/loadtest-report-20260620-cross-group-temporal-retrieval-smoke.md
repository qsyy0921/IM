# retrieval-gateway Cross-Group / Temporal Memory Smoke - 2026-06-20

## Scope

This is a local smoke for the retrieval layer only:

```text
search-service grpc + memory-service grpc
-> retrieval-gateway grpc
-> RetrieveEvidence
-> EvidencePack with SEARCH_MESSAGE + MEMORY_EVENT
```

The runner seeds low-sensitive projection rows directly into PostgreSQL, starts
the three local gRPC processes, and verifies the retrieval contract through the
public `RetrieveEvidence` API. Raw output is stored outside the repo under
`H:\NexusIM\loadtest-results`.

## Command

```powershell
. .\tools\go-env.ps1
.\loadtest\retrieval\run-local-smoke.ps1 -RunName retrieval-gateway-cross-temporal-smoke-20260620
```

## Result

- Result directory: `H:\NexusIM\loadtest-results\retrieval-gateway-cross-temporal-smoke-20260620`
- Query: `phoenix launch decision`
- Evidence items: 2
- Search items: 1
- Memory items: 1
- Retrieval version: `retrieval-gateway.v1`
- Search projection version: 17
- Memory projection version: 23
- Current memory query seq: 7

Verified:

- Search item preserved `message_id`, `source_event_id`, `conversation_seq`, and visibility version.
- Memory item preserved ACTIVE current-only state, source refs, review state, and extraction version.
- Cross-group memory source refs were preserved.
- Cross-group speaker attribution was preserved.
- Expired, superseded, and future memory decoys were excluded by query seq.
- Source counts and projection versions stayed stable.

Summary booleans:

```json
{
  "cross_group_source_refs_preserved": true,
  "cross_group_speaker_attribution_preserved": true,
  "temporal_version_selected_by_query_seq": true,
  "expired_memory_excluded": true,
  "superseded_memory_excluded": true,
  "future_memory_excluded": true
}
```

## Boundary

This smoke proves retrieval-gateway can preserve low-sensitive cross-group source
refs and use current-memory temporal filtering at the EvidencePack boundary. It
does not yet prove full RAG / Summary / Agent service-stack consumption of these
cross-group / temporal cases, nor does it add graph-edge expansion to the
retrieval proto.
