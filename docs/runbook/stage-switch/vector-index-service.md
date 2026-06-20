# vector-index-service Stage-Switch Review

Date: 2026-06-21

## Result

`vector-index-service` SDD v0.1 is ready to enter the implementation stage. No
P0/P1 blocker was found in the stage-switch review.

This review does not create `services/vector-index-service` yet. The
implementation slice must switch `vector-index-service` out of `future` in
`service-registry.json` and create the service directory in the same coherent
change.

## Why Promotion Is Justified

- Independent data model: vector collections, vector item metadata, index jobs,
  tombstones, rebuild checkpoints and vector outbox are not owned by
  knowledge-ingestion, memory, search, retrieval or model-gateway.
- Independent scale profile: embedding, vector backend upsert/delete, rebuild,
  backfill and tombstone repair scale differently from retrieval fan-in,
  message projection, memory projection and model provider routing.
- Independent failure boundary: vector backend failure or rebuild backlog must
  not block PullInbox, search projection, group memory state, RAG generation or
  Agent proposal creation.
- Security boundary: vector search must stay refs-only and post-filtered by
  retrieval / policy / tombstone metadata; raw text and vector arrays are not a
  public serving contract.
- Complexity reduction: keeping vector write, tombstone, rebuild and backend
  repair logic inside retrieval-gateway or memory-service would mix query fan-in
  with index mutation / repair concerns.

## Boundary Checks

- `retrieval-gateway` remains the only retrieval entry used by RAG, summary and
  Agent.
- `vector-index-service` must not return EvidencePack, raw text, message body,
  memory text, source URI, object key, prompt or model output.
- `model-gateway` owns embedding provider route, budget and provider metadata;
  vector-index consumes authorized embedding refs or calls the model boundary.
- `knowledge-ingestion-service`, `memory-service` and `search-service` own their
  source facts; vector-index stores rebuildable vector metadata and backend refs.
- Tombstone / delete-proof metadata must fail closed. Stale or incomplete vector
  metadata must not relax visibility.
- Events, metrics and debug snapshots must not contain raw text, embedding vector
  arrays, source URI, object key, backend credential, tenant label, source id,
  vector id, trace id or request id.

## Gate Impact For Next Slice

The implementation slice is broader than docs-only. It must update:

- `docs/runbook/service-registry.json`: switch `vector-index-service` from
  `future` to `product-active` with local process / debug / observability
  metadata.
- `api/proto/nexusim/vector/v1/vector_index_service.proto`.
- `migrations/postgres/vector-index/000001_vector_index_core.sql`.
- `services/vector-index-service` six-layer skeleton and
  `cmd/vector-index-service`.
- Docker runtime, local compose, Prometheus rules and Grafana dashboard.

Because this touches registry, proto, migration, Docker/compose and public API,
the implementation slice should run the expanded gate set or full `check-local`
before push.

## First Implementation Scope

Keep the first code slice narrow:

```text
UpsertVectorItem
TombstoneVectorItem
SearchVectors
GetVectorIndexJob
```

Use a local / PostgreSQL-backed test vector adapter first. Milvus, pgvector,
OpenSearch vector, embedding workers, rebuild workers and outbox relay are
future slices unless needed to prove the first contract.

## Focused Acceptance For First Smoke

- `UpsertVectorItem` is idempotent by tenant + source + source version + model
  ref / idempotency key and persists only low-sensitive metadata.
- `SearchVectors` is restricted to retrieval-gateway or explicitly allowlisted
  service identity and returns refs / scores only.
- Tombstoned items and delete-proof-marked items do not appear in search
  results.
- Event payloads and metrics do not contain raw text, embedding vectors, source
  URI, object key, backend vector id, provider body or secrets.
- PostgreSQL metadata remains the rebuildable source for vector index state;
  vector backend state is not treated as authoritative.
