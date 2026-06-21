# NexusIM Middleware Catalog

This document defines how NexusIM adopts and organizes middleware as the system
expands. It is a capability catalog, not a frozen list of mandatory components.

## Placement Rules

```text
deploy/                 Runtime wiring, compose files, future Kubernetes files.
docs/platform/          Middleware capability catalog and runtime profiles.
services/*/internal/    Service-specific infrastructure adapters.
internal/platform/      Only stable shared helpers with at least two real callers.
```

Business code must not depend directly on middleware clients outside
`internal/infrastructure` adapters. Domain and app layers depend on ports.

## Capability Groups

| Capability | Current / candidate middleware | Typical consumers | Notes |
| --- | --- | --- | --- |
| Transactional store | PostgreSQL, future cloud PostgreSQL | All fact-owning services | One service owns its schema; no cross-service private-table reads. |
| Event bus | Kafka, Schema Registry, future Pulsar | Outbox relays, projections, data platform | Public event contracts only; prefer AsyncAPI / CloudEvents-compatible metadata over time. |
| Cache / route / ephemeral state | Redis single, Sentinel, Cluster | push-gateway, api-gateway quota, presence | Cache is never source of durable business facts. |
| Search | OpenSearch, Elasticsearch, Meilisearch | search-service, retrieval-gateway | Only search-service writes search indexes. |
| Vector store | pgvector, Milvus, Qdrant, Weaviate | vector-index, retrieval, memory | Vector indexes are projections; rebuildable from source events/data products. |
| Object storage | MinIO, S3, Ceph | media, ingestion, data lake | Store binary payloads outside message-service. |
| Workflow engine | Internal workflow, Temporal, Cadence, Argo | workflow-service, admin, agent actions | Workflow state and approval audit remain service-owned. |
| Identity / federation | Built-in identity, Keycloak, OIDC providers | identity-service, api-gateway | OIDC provider is replaceable; gateway still enforces trusted metadata boundary. |
| Policy engine | policy-service, OpenFGA, OPA | policy, retrieval, agent, admin | External engines are adapters, not a reason to bypass service API. |
| Secrets / keys | env for local, Vault, KMS/HSM | identity, media, notification, model-gateway | Production secrets must not be embedded in docs, metrics or reports. |
| Observability | Prometheus, Grafana, Alertmanager, OTel, Loki, Tempo | All services | Local stack is development evidence, not production SLO proof. |
| Data platform | Debezium, Flink, Iceberg, Delta, Trino, ClickHouse/Doris | analytics, data catalog, feature store | Consumes public events / CDC; does not write business facts. |
| AI runtime | OpenAI/Claude/local model APIs, vLLM, Ollama, Triton, LiteLLM | model-gateway, Python workers, ai-eval | Model providers are behind model-gateway and audit/cost controls. |
| Graph / knowledge | PostgreSQL graph tables, Neo4j, graph projections | memory, retrieval, agent | Use only when relationship queries justify a separate projection. |
| MCP / tools | MCP gateway, tool servers, skill registry | agent, action-executor, AI clients | Tool invocation requires capability metadata, policy, approval and audit. |

## Runtime Profiles

Profiles are additive. Do not start every middleware component by default.

| Profile | Intended components |
| --- | --- |
| `core` | PostgreSQL, Kafka, Redis, base services needed for IM smoke. |
| `client-demo` | core + api-gateway BFF + push-gateway + client smoke dependencies. |
| `observability` | Prometheus, Grafana, Alertmanager, OTel collector. |
| `search-rag` | OpenSearch / vector store / retrieval / RAG / model-gateway. |
| `media` | MinIO / media-service / optional thumbnail and transcoding workers. |
| `workflow-agent` | workflow-service / agent-service / skill-registry / MCP gateway / action-executor. |
| `security` | OIDC provider, Vault/KMS emulator, OpenFGA/OPA adapters. |
| `data-platform` | CDC / ingestion / lakehouse / OLAP / analytics services. |
| `ai-runtime` | local model runtime or provider proxy used by model-gateway. |

## Adoption Checklist

Before adding middleware:

1. Name the capability it provides, not only the product name.
2. Identify owning service(s) and whether it is required for IM core path.
3. Decide if the middleware is source of truth, cache, projection or tool.
4. Add or update service port/adapters; keep domain/app free of concrete client
   imports.
5. Add runtime profile entry under `deploy/local/` or explicitly defer runtime.
6. Add low-sensitive health/metrics expectations.
7. Add focused smoke or a clear "not yet runtime tested" note.
8. Add rollback / migration note if it stores durable data.

## Registration Template

```text
Name:
Capability:
Status: candidate | local-profile | active | deprecated
Source of truth: yes | no
Owning service(s):
Consumers:
Local profile / compose:
Default local ports:
Data directory:
Health check:
Minimum smoke:
Production alternative:
Security notes:
Replacement / migration notes:
```

## Current Guidance

- Keep the current IM path on PostgreSQL + Kafka + Redis unless a feature
  clearly needs another middleware.
- Add search / vector / object storage / workflow / security middleware only
  when its service slice becomes active.
- Treat data-platform middleware as analytical infrastructure. It cannot become
  a hidden command path for business facts.
- Treat AI runtime middleware as provider infrastructure behind model-gateway.
  Business services do not call model providers directly.
