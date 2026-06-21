# NexusIM Complete Architecture Blueprint

This is the executable target architecture for NexusIM after the current IM
backend is expanded into a complete messaging product, data platform and AI /
Agent application platform. It is not a frozen list of services or middleware.
It defines stable boundaries, ownership rules, data flows and evolution gates.

## 1. Architecture Goal

NexusIM is designed as:

```text
high-concurrency IM backend
  + reusable business platform
  + governed data platform
  + AI / RAG / Agent application platform
  + platform-engineered middleware and operations layer
```

The system must support:

- real-time messaging, group chat, receipts, durable inbox and online wakeup;
- Web, Windows PC and Android clients through stable BFF / push boundaries;
- business reuse through identity, policy, contacts, media, notification,
  admin, audit, workflow and control-plane capabilities;
- AI use cases such as group memory, retrieval, RAG, summary and controlled
  Agent actions;
- local / interview-grade distributed demos now, and production-like HA,
  observability and governance later without redesigning service boundaries.

## 2. External Design Inputs

The design follows these sources and architecture traditions:

- Microservices around business capabilities and independently owned data:
  <https://martinfowler.com/articles/microservices.html>.
- SLO / SLI driven reliability, not vague production claims:
  <https://sre.google/sre-book/service-level-objectives/>.
- Platform engineering as curated internal platform capabilities:
  <https://tag-app-delivery.cncf.io/whitepapers/platform-eng-maturity-model/>.
- Zero Trust, least privilege and explicit verification:
  <https://csrc.nist.gov/pubs/sp/800/207/final>.
- API authorization and object / property access safety:
  <https://owasp.org/API-Security/editions/2023/en/0x11-t10/>.
- Data mesh: domain-owned data products on shared self-service data
  infrastructure: <https://martinfowler.com/articles/data-mesh-principles.html>.
- Lakehouse / open table formats for governed BI and ML data:
  <https://www.cidrdb.org/cidr2021/papers/cidr2021_paper17.pdf>,
  <https://iceberg.apache.org/spec/>.
- Transactional outbox and Saga patterns for microservice consistency:
  <https://microservices.io/patterns/data/transactional-outbox.html>.
- CloudEvents / AsyncAPI for long-term event contract governance:
  <https://cloudevents.io/>, <https://www.asyncapi.com/>.
- OpenTelemetry for vendor-neutral traces, metrics and logs:
  <https://opentelemetry.io/docs/>.
- MCP as a standard boundary for AI tools, resources and prompts:
  <https://modelcontextprotocol.io/docs/getting-started/intro>.
- Long-horizon collaborative memory: multi-party, multi-group, temporally
  evolving facts need attribution and versioning, not only vector recall:
  <https://arxiv.org/abs/2602.01313>.
- RAG and multi-Agent systems require retrieval quality, grounding,
  coordination, tool safety and eval:
  <https://arxiv.org/abs/2506.00054>,
  <https://arxiv.org/abs/2501.06322>.

## 3. System Layers

```text
Clients
  Web / Windows PC / Android / future iOS

Access Layer
  api-gateway / client BFF / push-gateway / auth / rate limit / trusted metadata

IM Core Platform
  identity / policy / contacts / conversation / message / delivery / receipt

Product Business Platform
  media / notification / presence / admin / audit / control-plane / workflow

Event and Fact Layer
  per-service PostgreSQL / outbox / Kafka / schema contracts / DLQ / repair

Data Platform
  CDC / ingestion / lakehouse / OLAP / data catalog / metrics / feature store

AI and Agent Platform
  search / vector-index / memory / retrieval / RAG / summary / agent
  skill-registry / MCP gateway / model-gateway / action-executor / ai-eval

Middleware Platform
  Redis / Kafka / PostgreSQL / OpenSearch / vector store / MinIO / Vault
  Keycloak / OpenFGA / Temporal / OTel / Prometheus / Grafana / future options
```

The service layer owns business semantics. The middleware layer provides
capabilities. A middleware product can be replaced without changing domain
ownership.

## 4. Deployment View

```text
                 +-----------------------------+
                 | Web / PC / Android clients  |
                 +--------------+--------------+
                                |
                 +--------------v--------------+
                 | api-gateway / client BFF    |
                 | auth, quota, trusted meta   |
                 +--------------+--------------+
                                |
          +---------------------+----------------------+
          |                                            |
+---------v----------+                      +----------v---------+
| IM business APIs   |                      | push-gateway       |
| gRPC / HTTP BFF    |                      | WebSocket wakeup   |
+---------+----------+                      +----------+---------+
          |                                            |
          +---------------------+----------------------+
                                |
                 +--------------v--------------+
                 | service-owned PostgreSQL    |
                 | outbox -> Kafka -> workers  |
                 +--------------+--------------+
                                |
          +---------------------+----------------------+
          |                                            |
+---------v----------+                      +----------v---------+
| data platform      |                      | AI / Agent platform|
| analytics / BI     |                      | retrieval / action |
+--------------------+                      +--------------------+
```

Local development uses Docker profiles. Production can later map the same
boundaries to Kubernetes, managed databases and managed middleware.

## 5. Ownership Invariants

1. A service owns its database schema.
2. No production code reads another service's private tables.
3. Cross-service sync calls must use public APIs or explicit ports.
4. Cross-service async integration uses public events with schema versioning.
5. Kafka is an event propagation surface, not the authoritative fact store.
6. Data platform consumes facts; it does not write business commands.
7. AI / Agent services cannot bypass policy, EvidencePack, approval or audit.
8. Python workers return candidates only; Go owns control, facts and audit.
9. Client local storage is cache / offline queue, not server-side fact source.
10. Middleware is introduced as a capability with adapter and runtime profile,
    not as service-private sprawl.

## 6. Domain Map

### 6.1 Access Domain

| Component | Responsibility |
| --- | --- |
| `api-gateway` | Public API facade, client BFF, auth, quota, trusted metadata, low-sensitive observability. |
| `push-gateway` | Online WebSocket wakeup, Redis route, session lifecycle, best-effort online notification. |

The access layer terminates public trust. Downstream services trust only
gateway-minted metadata, not arbitrary client headers.

### 6.2 IM Core Domain

| Service | Owned facts |
| --- | --- |
| `identity-service` | users, credentials, sessions, refresh tokens, MFA, OIDC/JWKS/challenges. |
| `policy-service` | authorization policies, ReBAC edges, policy decisions, risk/moderation policy. |
| `contacts-service` | contact relationships, privacy settings, contact groups. |
| `conversation-service` | conversations, group membership, roles, member boundaries, owner transfer. |
| `message-service` | message log, edits, revoke/delete, attachments by reference, message outbox. |
| `delivery-service` | durable user inbox, device cursors, delivery events, projection checkpoints. |
| `receipt-service` | read/delivered receipts, unread foundations, conversation list summaries. |

These services are the current foundation for IM product behavior.

### 6.3 Product Business Domain

| Service | Responsibility |
| --- | --- |
| `media-service` | Upload, object storage, thumbnails, virus scan, transcode, download policy. |
| `notification-service` | Email, SMS, APNs/FCM, templates, provider routing, bounce/suppression. |
| `presence-service` | Online state, typing, last-seen, device status, privacy-aware presence. |
| `admin-service` | Tenant/admin APIs, repair approval, user moderation, operator actions. |
| `audit-service` | Login, security, admin, Agent action and repair audit; export and retention. |
| `control-plane-service` | Tenant config, feature flags, rollout, quota, policy/config publication. |
| `workflow-service` | Long-running approvals, compensation, timers, external callbacks. |

These are business platform services. They are promoted only when product scope
needs them, not because the architecture wants more services.

### 6.4 AI and Agent Domain

| Service | Responsibility |
| --- | --- |
| `search-service` | Search projection writes and keyword retrieval. |
| `vector-index-service` | Vector projection writes, rebuild, pgvector/Milvus/OpenSearch adapters. |
| `memory-service` | Personal/group/project memory state with attribution and lifecycle. |
| `retrieval-gateway` | Hybrid retrieval, visibility filtering, EvidencePack construction. |
| `rag-service` | Grounded answers from EvidencePack with citations and uncertainty. |
| `summary-service` | Conversation, unread and project summaries with source references. |
| `agent-service` | Planning, multi-Agent coordination and Agent run state. |
| `skill-registry` | Tool/skill capability catalog, risk level and invocation metadata. |
| `mcp-gateway` | MCP tool/resource/prompt boundary and consent enforcement. |
| `model-gateway` | LLM, embedding and rerank provider routing, budget, fallback and audit. |
| `action-executor` | Executes approved business actions through public APIs only. |
| `ai-eval-service` | Regression datasets, RAG/memory/Agent eval, safety gates. |

AI services consume projections and EvidencePack. They do not become alternate
business fact sources.

### 6.5 Data Platform Domain

Future data platform services are introduced when analytics/RAG/ops needs exceed
service-local debug metrics.

| Service | Responsibility |
| --- | --- |
| `data-ingestion-service` | Consume public events / CDC and write governed analytical records. |
| `data-catalog-service` | Register data products, owners, schemas, lineage, retention, privacy class. |
| `analytics-service` | Serve curated product, ops and business metrics. |
| `feature-store-service` | Serve low-sensitive risk/ranking/Agent features. |
| `data-quality-service` | Track freshness, missing events, schema drift and quality checks. |

Data platform is read/analysis oriented. Commands still go through business
services.

## 7. Core IM Flow

### 7.1 Send Message

```text
client
  -> api-gateway BFF / GatewayService
  -> message-service.SendMessage
  -> conversation-service.GetSendContext
  -> message_log + message_outbox in local transaction
  -> outbox relay -> Kafka conversation.timeline.events
  -> delivery-service projection -> user_inbox + delivery_outbox
  -> delivery outbox relay -> Kafka im.delivery.events
  -> push-gateway -> WebSocket delivery.notify
  -> client PullInbox -> AckDelivery
```

Message display is based on `delivery-service.PullInbox`, not WebSocket payload
alone. WebSocket is wakeup, not durable data.

### 7.2 Group Membership

```text
client/admin
  -> conversation-service.CreateMemberChange
  -> local saga + membership state + timeline event + outbox
  -> Kafka member boundary event
  -> delivery membership projection
  -> search/memory visibility projection
  -> audit / analytics / policy projections
```

Membership windows must affect delivery, search, retrieval and memory. Current
member state cannot be used to rewrite historical visibility.

### 7.3 Read / Delivered Receipts

```text
client AckDelivery / MarkRead
  -> delivery-service / receipt-service
  -> device cursor / receipt facts
  -> receipt events
  -> conversation list / unread projection
  -> notification / analytics / AI summary consumers
```

## 8. Client Architecture

Clients share TypeScript protocol and sync core:

```text
clients/packages/protocol
clients/packages/client-core
clients/web
clients/desktop
clients/android
```

Rules:

- Clients call only `api-gateway` BFF and `push-gateway`.
- `client-core` owns local sync state, offline queue semantics and API models.
- Browser uses Web APIs; PC uses Tauri as a thin shell; Android uses a thin
  platform bridge.
- Native bridges do not implement business decisions.
- Local storage is cache and pending operation state only.

## 9. Event Architecture

Every event-producing service uses local transaction + outbox for first-stage
reliability.

Event envelope guidance:

```text
event_id
tenant_id
producer
aggregate_type
aggregate_id
aggregate_version
event_type
event_version
partition_key
occurred_at
correlation_id
causation_id
trace_id
payload_json/protobuf
```

Long-term:

- use protobuf schemas for Kafka payloads;
- register event docs with AsyncAPI-style channel descriptions;
- keep CloudEvents-compatible metadata where useful;
- support DLQ / retry / repair / replay for each event family;
- keep sensitive raw payloads out of reports, metrics and review pages.

## 10. Data Platform Architecture

The data platform is built from public events and CDC, not from direct joins over
business private tables.

```text
Public events / CDC
  -> ingestion validation
  -> raw event store
  -> domain normalized tables
  -> aggregate data products
  -> metrics / BI / risk / feature / RAG use cases
```

Data product contract:

```text
name
owner
source events / source systems
schema version
freshness target
privacy class
retention
quality checks
consumer list
repair / backfill procedure
```

Do not build AI retrieval directly on arbitrary operational tables. Build
search/vector/memory projections with explicit visibility and delete semantics.

## 11. AI / Memory / RAG Architecture

### 11.1 Group Memory Model

Memory records must preserve context:

```text
source_ref
tenant_id
conversation_id / group_id / project_id
speaker_id
audience_scope
fact_type
fact_text
status: draft | active | superseded | archived | deleted
valid_from / valid_to
supersedes
related_events
visibility_window
confidence
review_state
evidence_refs
```

Rules:

- A group statement is not automatically a user preference.
- A stale decision must be superseded, not merely appended.
- Delete/revoke/member-leave events must affect search, vector and memory
  visibility.
- Retrieval must return source refs and reason for inclusion.

### 11.2 Retrieval and EvidencePack

```text
request
  -> policy check
  -> structural filters: tenant / group / user / time / visibility
  -> keyword search
  -> vector search
  -> graph / related-event expansion
  -> rerank
  -> EvidencePack
  -> RAG / summary / Agent
```

EvidencePack includes:

```text
evidence_id
source_refs
conversation_seq / message_id / object refs
visibility proof
retrieval score
version / valid time
redaction profile
```

RAG and summary services cannot call search/vector stores directly when a
policy-aware retrieval gateway is available.

### 11.3 Agent Architecture

```text
Agent request
  -> intent classification
  -> retrieval EvidencePack
  -> plan candidate
  -> policy precheck
  -> proposal
  -> approval if needed
  -> action-executor
  -> audit
  -> event
```

Agent roles may be multi-agent, but coordination is a service concern:

- planner;
- retrieval specialist;
- risk/policy reviewer;
- tool/action executor;
- evaluator/critic.

Only approved actions can mutate business state.

## 12. Middleware Platform

Middleware is organized by capability. See
`../platform/middleware-catalog.md` for the full catalog and adoption checklist.

Runtime profiles:

| Profile | Scope |
| --- | --- |
| `core` | PostgreSQL, Kafka, Redis and core services. |
| `client-demo` | Client BFF, push and client smoke path. |
| `observability` | Prometheus, Grafana, Alertmanager, OTel collector. |
| `search-rag` | OpenSearch/vector store/retrieval/RAG/model gateway. |
| `media` | MinIO and media processing dependencies. |
| `workflow-agent` | Workflow, Agent, skill, MCP and action execution. |
| `security` | OIDC provider, Vault/KMS emulator, OpenFGA/OPA. |
| `data-platform` | CDC, lakehouse, OLAP and analytics. |
| `ai-runtime` | Local or remote model provider proxies. |

Do not default-start all middleware. Each active slice chooses the smallest
runtime profile needed for its smoke.

## 13. Security Architecture

Security boundaries:

- Public network boundary: `api-gateway`, `push-gateway`, client assets.
- Identity boundary: `identity-service`, OIDC/JWKS, session/MFA.
- Authorization boundary: `policy-service`, gateway metadata and per-service
  ownership checks.
- Data boundary: service-owned PostgreSQL, event contracts, data product privacy.
- AI action boundary: retrieval, policy, workflow approval, action executor,
  audit.

Security rules:

1. Public listeners must reject mock auth or plaintext secrets unless explicitly
   local/private.
2. Trusted metadata is minted only by gateway or verified internal boundary.
3. Every public API must check object-level authorization.
4. Sensitive values are not logged, exported, embedded in metrics or committed
   into reports.
5. Tool / MCP / Agent actions require capability metadata, policy and audit.
6. KMS/HSM/Vault are capabilities behind adapters, not assumptions baked into
   domain logic.

## 14. Reliability and Operations

First-stage local evidence is not production SLO proof. The path to production
requires:

- SLIs for user-visible paths such as login, send, pull inbox, push wakeup,
  search retrieval and Agent action latency;
- SLOs only after enough operational data exists;
- dashboards, alerts, runbooks and error budgets for critical paths;
- DLQ, repair, replay and audit for every event-driven workflow;
- backup, restore, failover and data integrity drills for each durable store;
- canary / rollout / rollback gates for config and service changes.

## 15. Code Organization

```text
services/<service>/
  cmd/
  internal/api/
  internal/app/
  internal/domain/
  internal/infrastructure/
  internal/trigger/
  internal/types/

clients/
  packages/protocol/
  packages/client-core/
  web/
  desktop/
  android/

ai/python/
  workers/
  eval/
  algorithms/
  tools/

deploy/
  local/
  docker/

docs/
  architecture/
  platform/
  sdd/
  runbook/
```

Shared packages require at least two real callers and a stable contract. Do not
extract abstractions merely to make diagrams symmetrical.

## 16. Language Boundary

| Area | Language |
| --- | --- |
| Business services, BFF, control, audit, durable facts | Go |
| Web / PC / Android shared client core and UI | TypeScript |
| Tauri desktop bridge | Rust, thin bridge only |
| Android platform bridge | Kotlin, thin bridge only |
| iOS future bridge | Swift, thin bridge only |
| AI workers, model algorithms, offline eval | Python |

Python cannot own business facts, security decisions or audit truth.

## 17. Evolution Roadmap

### Phase A: Stable IM and Client MVP

- Keep current IM services as the working backend.
- Finish Web / PC / Android client shell on shared client core.
- Keep BFF and push as the only client-facing backend surfaces.

### Phase B: Product Platform Completion

- Promote media, notification, presence, admin, audit, control-plane and
  workflow by product need.
- Keep each promotion service-by-service with SDD, migration, API, smoke and
  docs.

### Phase C: AI Data Boundary

- Strengthen search, vector-index, memory and retrieval.
- Enforce visibility, deletion, supersession and EvidencePack contracts.

### Phase D: RAG and Agent Applications

- Build RAG and summary on EvidencePack only.
- Build Agent actions through workflow approval and action-executor.
- Expand ai-eval before expanding autonomous actions.

### Phase E: Data Platform

- Build ingestion and analytics from public events / CDC.
- Add data catalog, quality checks and feature products.

### Phase F: Production-Like Operations

- Replace local-only middleware with managed or HA profiles where needed.
- Add SLOs, alerting, backup/restore, failover and rollout governance.

## 18. Service / Middleware Addition Rule

Add a service or middleware only when at least one condition is true:

1. It owns an independent data model.
2. It has an independent scaling profile.
3. It has an independent failure or security boundary.
4. Multiple services need the same capability.
5. It significantly reduces existing service complexity.

Every addition needs:

- ADR or SDD section.
- Public API / event contract.
- Data ownership statement.
- Runtime profile or explicit deferred-runtime note.
- Focused validation.
- Rollback / migration / compatibility note.

## 19. What This Architecture Avoids

- One giant "middle platform" service.
- AI services directly reading operational private tables.
- Clients calling internal microservices.
- Data platform becoming a hidden command side.
- Middleware-specific code in domain/app layers.
- Python workers owning durable business state.
- Service count or middleware products frozen as the final answer.

## 20. Interview Narrative

NexusIM can be described as:

```text
A distributed IM backend with durable messaging, delivery and online wakeup,
expanded into a reusable business platform and governed data platform, then
layered with policy-aware retrieval, collaborative memory, RAG and controlled
Agent actions. The architecture keeps business facts in Go services, analytical
data in governed projections, AI evidence in retrieval packs, and real actions
behind approval and audit.
```
