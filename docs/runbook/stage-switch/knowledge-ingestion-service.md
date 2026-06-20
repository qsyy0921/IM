# knowledge-ingestion-service Stage-Switch Review

Date: 2026-06-21

## Result

`knowledge-ingestion-service` SDD v0.1 is ready to enter the implementation
stage. No P0/P1 blocker was found in the stage-switch review.

This review does not create `services/knowledge-ingestion-service` yet. The
implementation slice must switch `knowledge-ingestion-service` out of `future`
in `service-registry.json` and create the service directory in the same coherent
change.

## Why Promotion Is Justified

- Independent data model: knowledge sources, documents, chunks, ingestion jobs,
  delete proofs and ingestion outbox are not owned by media-service,
  retrieval-gateway, vector-index-service, memory-service or model-gateway.
- Independent scale profile: parsing, chunking, metadata extraction, embedding
  candidate generation and rebuild jobs scale differently from media upload,
  vector writes, retrieval query latency and IM message projection.
- Independent failure boundary: parser failure, bad document format, embedding
  provider outage or rebuild lag must not block IM delivery, media download,
  retrieval queries, RAG answers or Agent proposals.
- Security boundary: raw documents, source refs, object keys, data class,
  tenant policy and delete proof handling need a dedicated owner with strict
  low-sensitive event and metrics rules.
- Complexity reduction: keeping ingestion pipelines inside media-service,
  retrieval-gateway or vector-index-service would mix storage, query and
  indexing responsibilities and make deletion / rebuild semantics fragile.

## Boundary Checks

- media-service still owns object storage, upload sessions, scanner refs and
  media download policy.
- retrieval-gateway remains the only query-time EvidencePack assembly boundary.
- vector-index-service owns vector writes / rebuilds when it is promoted; this
  service emits ingestion manifests and chunk refs, not direct vector-store
  writes in the first slice.
- model-gateway owns provider calls for embedding, rerank or extraction. Python
  parser / embedding workers can return candidates only; Go owns state,
  permission metadata, audit and persistence.
- policy-service / control-plane-service remain decision sources for tenant
  policy, data class, retention and quota.
- Events, metrics and debug snapshots must not contain raw file content, chunk
  text, source URI with secrets, object key, provider body, parser raw error,
  private URL query, credential, token, DSN or raw identifiers.

## Gate Impact For Next Slice

The implementation slice is broader than a docs-only change. It must update:

- `docs/runbook/service-registry.json`: switch `knowledge-ingestion-service`
  from `future` to `product-active` with local process / debug / observability
  metadata.
- `api/proto/nexusim/knowledge/v1/knowledge_ingestion_service.proto`.
- `migrations/postgres/knowledge-ingestion/000001_knowledge_ingestion_core.sql`.
- `services/knowledge-ingestion-service` six-layer skeleton and
  `cmd/knowledge-ingestion-service`.
- Docker runtime, local compose, Prometheus rules and Grafana dashboard.

Because this touches registry, proto, migration, Docker/compose and public API,
the implementation slice should run the expanded gate set or full `check-local`
before push.

## First Implementation Scope

Keep the first code slice narrow:

```text
CreateKnowledgeSource
SubmitIngestionJob
GetIngestionJob
ListKnowledgeChunks
```

Use local document metadata plus a chunk manifest for the first smoke. Do not
connect real provider parsing, embedding, web crawler, object-store download or
vector-store write in the first slice unless the test uses a deterministic local
adapter with low-sensitive fixtures.

## Focused Acceptance For First Smoke

- `CreateKnowledgeSource` records only source refs, data class, visibility,
  owner / tenant policy refs and content hash metadata.
- `SubmitIngestionJob` is idempotent by tenant + source + source version +
  idempotency key.
- chunk records include source ref, version, visibility, policy version,
  chunk hash and delete proof refs; they do not store raw chunk text in events,
  metrics or public responses.
- failed parser / manifest validation produces stable public errors and a
  low-sensitive failure class without provider body or raw parser error.
- output events are low-sensitive ingestion state events for downstream
  vector-index / retrieval consumers.
- no production code reads media-service, retrieval-gateway, model-gateway,
  vector-index or policy-service private tables.
