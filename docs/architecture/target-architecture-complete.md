# NexusIM Complete Target Architecture

This document defines the complete target architecture after NexusIM is
expanded beyond the current IM backend into business platform, data platform,
AI / Agent platform and middleware platform capabilities. It is intentionally
not a frozen service count. New services and middleware remain subject to ADR,
evidence and migration rules.

## Design Inputs

The design follows these external architecture references:

- Microservices should be organized around business capabilities and own their
  data boundaries: <https://martinfowler.com/articles/microservices.html>.
- SLOs / SLIs should drive reliability decisions rather than vague "production
  grade" claims: <https://sre.google/sre-book/service-level-objectives/>.
- Platform engineering should expose curated capabilities as an internal
  platform product: <https://tag-app-delivery.cncf.io/whitepapers/platform-eng-maturity-model/>.
- Zero Trust requires explicit verification and least privilege instead of
  implicit network trust: <https://csrc.nist.gov/pubs/sp/800/207/final>.
- API design must protect object-level and property-level authorization:
  <https://owasp.org/API-Security/editions/2023/en/0x11-t10/>.
- Data mesh treats analytical data as domain-owned data products on top of
  shared self-service data infrastructure:
  <https://martinfowler.com/articles/data-mesh-principles.html>.
- Lakehouse architecture and open table formats support BI / ML over shared
  governed data: <https://www.cidrdb.org/cidr2021/papers/cidr2021_paper17.pdf>,
  <https://iceberg.apache.org/spec/>.
- Transactional outbox remains the preferred first-stage boundary between local
  database transactions and event publication:
  <https://microservices.io/patterns/data/transactional-outbox.html>.
- CloudEvents / AsyncAPI shape long-term event interoperability:
  <https://cloudevents.io/>, <https://www.asyncapi.com/>.
- OpenTelemetry is the vendor-neutral telemetry baseline:
  <https://opentelemetry.io/docs/>.
- MCP standardizes AI access to tools, resources and prompts:
  <https://modelcontextprotocol.io/docs/getting-started/intro>.
- Long-horizon collaborative memory must model multi-party, multi-group,
  temporally evolving facts, not only vector snippets:
  <https://arxiv.org/abs/2602.01313>.
- Modern RAG / multi-agent systems need retrieval quality, grounding,
  coordination, tool safety and eval:
  <https://arxiv.org/abs/2506.00054>,
  <https://arxiv.org/abs/2501.06322>.

## Layered Target

```text
Clients
  Web / Windows PC / Android / future iOS

Access and edge
  api-gateway / client BFF / push-gateway / auth / rate limit / trusted metadata

Business platform
  identity / policy / contacts / conversation / message / delivery / receipt
  media / notification / presence / admin / audit / control-plane / workflow

Event and transactional facts
  PostgreSQL per service / transactional outbox / Kafka / schema registry
  DLQ / repair / audit / public event contracts

Data platform
  Kafka / CDC ingestion / lakehouse / OLAP / metrics / feature store
  data catalog / data quality / BI / low-sensitive data products

AI and Agent platform
  search / vector-index / memory / retrieval / RAG / summary / agent
  skill-registry / MCP gateway / model-gateway / action-executor / ai-eval

Middleware platform
  Redis / Kafka / PostgreSQL / OpenSearch / vector store / MinIO / Vault
  Keycloak / OpenFGA / Temporal / OTel / Prometheus / Grafana / future options
```

## Business Platform

The business platform is a set of reusable business capabilities. It is not one
large "middle platform" service.

| Capability | Owning service boundary |
| --- | --- |
| Identity, MFA, sessions, OIDC and gateway token keys | `identity-service` |
| Authorization, ReBAC and policy decisions | `policy-service` |
| Contacts, privacy and relationship graph | `contacts-service` |
| Conversations, groups, members and owner transfer | `conversation-service` |
| Message facts, edits, revoke, delete and attachment refs | `message-service` |
| Durable inbox, PullInbox and device ACK | `delivery-service` |
| Read / delivered receipts and unread foundations | `receipt-service` |
| Online WebSocket wakeup and route fanout | `push-gateway` |
| Media upload, object storage, thumbnails and transcoding | `media-service` |
| Email, SMS, APNs / FCM and notification templates | `notification-service` |
| Online status, typing and last-seen | `presence-service` |
| Tenant config, feature flags, quota and rollout control | `control-plane-service` |
| Admin APIs, repair approval and operator control | `admin-service` |
| Security, admin and Agent action audit | `audit-service` |
| Long workflows, approvals and compensation | `workflow-service` |

Business services own transactional facts. They do not read other services'
private tables. Cross-service communication uses public APIs, public events or
explicit ports.

## Data Platform

The data platform consumes events and CDC. It must not become a hidden write path
for business facts.

```text
business outbox / Kafka / CDC
  -> ingestion jobs
  -> ODS raw events
  -> DWD domain facts
  -> DWS subject aggregates
  -> ADS metrics / BI / feature / RAG data products
```

First-stage data platform services may include:

| Service | Responsibility |
| --- | --- |
| `data-ingestion-service` | Consume public events / CDC and write governed analytical data. |
| `data-catalog-service` | Register data products, owners, schemas, lineage and retention. |
| `analytics-service` | Serve product and ops metrics from curated aggregates. |
| `feature-store-service` | Serve low-sensitive features for risk, ranking and Agent decisions. |
| `data-quality-service` | Monitor schema drift, delay, missing events and quality checks. |

Data products must declare owner, source events, schema version, freshness,
retention, privacy class, downstream consumers and repair procedure.

## AI and Agent Platform

AI services are split by control responsibility:

| Service | Responsibility |
| --- | --- |
| `search-service` | Full-text indexing and query API. It is the only writer to its search index. |
| `vector-index-service` | Vector index writes, rebuilds and provider adapters. |
| `memory-service` | Long-horizon personal, group and project memory state with source refs. |
| `retrieval-gateway` | Policy-aware hybrid retrieval and EvidencePack generation. |
| `rag-service` | Answers only from EvidencePack and returns citations / uncertainty. |
| `summary-service` | Conversation, unread and project summaries with source references. |
| `agent-service` | Planning, role coordination and Agent run state. |
| `skill-registry` | Tool / skill metadata, risk level and capability catalog. |
| `mcp-gateway` | MCP tools/resources/prompts boundary and consent enforcement. |
| `model-gateway` | LLM / embedding / rerank provider routing, cost, fallback and audit. |
| `action-executor` | Executes approved real business actions through public APIs only. |
| `ai-eval-service` | Eval datasets, regression runs, Agent/RAG/memory scoring. |

### Collaborative Memory Invariants

Group memory must model:

```text
source_ref
tenant_id / conversation_id / group_id / project_id
speaker_id / actor_ids
scope: personal | group | project | tenant
fact_type: task | decision | preference | blocker | status | file | policy
status: draft | active | superseded | archived | deleted
valid_from / valid_to
supersedes / related_events
visibility_window
confidence
review_state
evidence_refs
```

It must not upgrade a group fact into a personal preference without review, and
must not return facts after revoke / delete / membership-window invalidation.

### Controlled Agent Actions

Agent write actions follow this path:

```text
Agent plan
  -> retrieval-gateway EvidencePack
  -> policy-service authorization
  -> workflow-service proposal
  -> human or policy approval
  -> action-executor public API call
  -> audit-service record
  -> outbox event
```

No Python worker, MCP server or model provider may directly mutate business
facts. Python returns candidates, rankings or draft plans; Go services own
authorization, workflow state, durable facts and audit.

## Middleware Platform

Middleware is managed as platform capability, not as service-private sprawl.

```text
deploy/local/
  docker-compose.core.yml
  docker-compose.observability.yml
  docker-compose.search.yml
  docker-compose.vector.yml
  docker-compose.media.yml
  docker-compose.workflow.yml
  docker-compose.security.yml
  docker-compose.data-platform.yml
  docker-compose.ai-runtime.yml
```

Service code contains only adapters under `internal/infrastructure/<adapter>`.
Deployment, port, data directory and local profile ownership belong in
`deploy/` and `docs/platform/`.

## Security and Governance

- Public listeners must not use mock auth or plaintext secrets.
- Trusted metadata must be minted only at the gateway / verified boundary.
- Object-level and property-level authorization is mandatory for public APIs.
- mTLS, OIDC, KMS/HSM, OpenFGA/OPA and Vault remain replaceable capabilities,
  not hard-coded one-off choices.
- AI tool execution requires capability metadata, consent, policy check,
  approval for risky writes and audit.
- Sensitive values are not emitted in logs, metrics, reports, manifests or
  review pages.

## Observability and Reliability

- Use SLIs / SLOs for product-critical paths only after enough operational
  evidence exists.
- Local Prometheus / Grafana / OTel remains development and interview evidence,
  not production SLO proof.
- Every new service should expose health, readiness, low-sensitive metrics,
  structured logs and trace context.
- Repair / DLQ / replay paths are part of service design, not afterthoughts.

## Evolution Rules

Add a service or middleware only when at least one condition is true:

1. Independent data model.
2. Independent scaling profile.
3. Independent failure or security boundary.
4. Multiple services need the same capability.
5. It significantly reduces existing service complexity.

Every addition needs:

- ADR or SDD section.
- Public API / event contract.
- Local run profile or explicit deferred-runtime note.
- Focused checks.
- Rollback / migration / compatibility statement.

## Near-Term Roadmap

1. Finish client platform MVP foundation: Web, PC shell and Android shell on the
   same client core.
2. Add middleware catalog and runtime profiles before introducing more
   middleware.
3. Continue search / memory / retrieval / vector-index as the AI data boundary.
4. Build RAG / summary only on EvidencePack and visibility-filtered retrieval.
5. Expand Agent actions through workflow approval and audit.
6. Add data platform MVP from public events to low-sensitive metrics and feature
   products.
7. Promote media / notification / presence / control-plane / admin as product
   needs become active, one service slice at a time.
