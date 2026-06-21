# vector-index-service knowledge chunk consumer smoke - 2026-06-21

## Scope

This was a local multi-process smoke for the knowledge event handoff into the
vector embedding task queue:

```text
knowledge-ingestion CreateKnowledgeSource / SubmitIngestionJob
-> knowledge_outbox
-> knowledge-ingestion outbox-relay
-> Kafka im.knowledge.events.<run>
-> vector-index chunk-consumer
-> knowledge-ingestion ListKnowledgeChunks
-> vector_embedding_tasks
```

The business handoff used public gRPC APIs and Kafka events. PostgreSQL access
in the runner was limited to applying migrations, cleaning the smoke tenant, and
verifying the vector embedding queue.

Raw summary:

```text
H:\NexusIM\loadtest-results\vector-chunk-consumer-smoke-20260621-085235\vector-embedding-producer-summary.json
```

## Runtime

```text
knowledge-ingestion-service grpc: dynamic localhost port
knowledge-ingestion-service outbox-relay: im.knowledge.events.vector-chunk-consumer-smoke-20260621-085235
vector-index-service chunk-consumer: nexusim-vector-chunk-smoke-vector-chunk-consumer-smoke-20260621-085235
PostgreSQL: postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable
Kafka: localhost:9092
```

The summary was generated from clean commit `7340f069` with `git_dirty=false`.

## Result

```text
success=true
knowledge_source_id=ksrc_f630b9fa37cf05707b62104b
document_id=kdoc_a8df7fe98d67d68a937615a8
chunk_count=2
expected_count=2
embedding_task_count=2
embedding_task_pending=2
embedding_task_running=0
embedding_task_completed=0
embedding_model_ref=deterministic-embedding-v1
```

## Judgment

This proves the first-stage `knowledge_outbox -> im.knowledge.events ->
vector-index chunk-consumer -> vector_embedding_tasks` path works with real
PostgreSQL and Kafka. The vector consumer skips known non-chunk knowledge events
from the shared topic and only enqueues `knowledge.chunk.ready.v1` refs after
resolving redacted previews through `ListKnowledgeChunks`.

It does not prove embedding worker execution, vector search, Milvus / pgvector /
OpenSearch backends, provider-grade parser / connector behavior, or production
Kafka HA.
