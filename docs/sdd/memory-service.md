# NexusIM memory-service SDD v0.1

Status: Foundation-active.

This document records the first codable contract for `memory-service`. The
service is a projection, query, and reviewed profile-aggregate service for
structured collaborative memory. It does not run LLM extraction in-process yet
and does not replace search, retrieval, RAG, summary, policy, or business facts.

## 1. Service Position

`memory-service` owns AI-oriented memory read models:

- `memory_structured_events`: versioned collaborative facts with source refs.
- `memory_event_source_refs`: normalized source references for traceability,
  tombstone propagation, and rebuild.
- `memory_membership_projection`: conversation visibility projection used for
  fail-closed memory queries. It is not a member fact source.
- `memory_graph_edges`: lightweight event graph for multi-hop expansion.
- `memory_profile_aggregates`: reviewed profile aggregates derived from several
  supporting memory events.
- `memory_projection_checkpoints`: consumer group + topic + partition next
  offset checkpoint.

Responsibilities:

- Consume conversation timeline and later domain events.
- Project group memory candidates as `StructuredMemoryEvent`.
- Preserve speaker, audience, scope, validity window, supersession,
  contradiction, confidence, review state, and source refs.
- Maintain membership visibility windows so memory queries can be filtered by
  historical membership.
- Provide memory query and profile aggregate query contracts for the future
  `retrieval-gateway`.
- Recompute reviewed profile aggregates from multiple ACTIVE / APPROVED
  `PROFILE_SIGNAL` memories through an explicit service API.

Non-responsibilities:

- It is not a message fact source and must not write `message_log`.
- It is not a member fact source and must not write conversation membership
  facts.
- It does not generate final RAG answers.
- It does not execute Agent tools or write business facts.
- It must not read other services' private tables.
- It must not turn a single group message into a durable personal profile fact.

## 2. Inputs and Outputs

| Direction | Component | Interaction |
| --- | --- | --- |
| Upstream events | Kafka `conversation.timeline.events` | Message and member boundary facts |
| Later upstream events | identity / contacts / receipt / policy events | Optional profile and relationship signals |
| Sync API | `api-gateway` / `retrieval-gateway` | Query memory events and profile aggregates |
| Sync dependency | PostgreSQL | Own projection tables |
| Policy dependency | `policy-service` | Future strict authorization check when visibility is stale |

First version API contract: `api/proto/nexusim/memory/v1/memory_service.proto`.

RPCs:

- `QueryMemoryEvents`
- `GetMemoryEvent`
- `ListProfileAggregates`
- `RecomputeProfileAggregate`

These APIs return only structured, source-backed memory records. They do not
return model answers.

`QueryMemoryEvents.at_conversation_seq` is the runtime "current fact" selector
for conversation-scoped retrieval. When set, the service returns only memory
whose `valid_from_seq` is not after that seq and whose `valid_to_seq` is unset
or still covers that seq. The default status set remains `ACTIVE`; callers may
explicitly request `PENDING` for review/smoke flows.

`RecomputeProfileAggregate` is an explicit profile maintenance API. It derives
one profile aggregate for `subject_user_id + aggregate_type + aggregate_key`
from visible ACTIVE / APPROVED `PROFILE_SIGNAL` memory events. The first version
is self-service only: the caller can recompute its own profile aggregate, and
must not recompute another user's profile without a future policy-controlled
operator path. If the number of visible supporting signals is lower than
`min_support_count` (default 2), the service archives any existing ACTIVE /
PENDING profile aggregate for that key instead of leaving stale profile evidence
visible.

## 3. Core Model

`StructuredMemoryEvent` is not the raw message. It is a source-backed memory
candidate or accepted memory fact.

Required fields:

- `memory_event_id`
- `tenant_id`
- `scope`: conversation, project, personal, or tenant
- `scope_id`
- `conversation_id` when applicable
- `topic`
- `event_type`: task, decision, status, blocker, file, preference signal, role
  signal, profile signal
- `status`: pending, active, superseded, rejected, archived, deleted
- `review_state`: unreviewed, needs_review, approved, rejected
- `fact_text`
- `actor_user_ids`
- `audience_user_ids`
- `source_refs`
- `valid_from_seq` / `valid_to_seq`
- `valid_from_at` / `valid_to_at`
- `supersedes_event_ids`
- `contradicts_event_ids`
- `confidence`
- `visibility_version`
- `extraction_version`

First-slice invariants:

- A memory event must have at least one source ref before it can become ACTIVE.
- Profile-like signals must not become ACTIVE profile facts without aggregation
  and review.
- Recomputed profile aggregates must keep all supporting memory ids, and must
  archive stale aggregates when supporting memory is deleted, rejected, or no
  longer visible.
- Supersession must be explicit; old and new facts must not sit side by side as
  equally active when a newer event overrides the older one.
- Message revoke / delete / retention cleanup must be able to tombstone or
  hide derived memory.
- Query results must be filtered by tenant and historical visibility.
- If visibility is stale or uncertain, fail closed instead of returning memory.

## 4. Visibility

For conversation-scoped memory, the memory query must apply the same historical
membership semantics as search:

```text
source_ref.conversation_seq >= membership.join_seq
AND (membership.leave_seq IS NULL OR source_ref.conversation_seq <= membership.leave_seq)
AND (query.at_conversation_seq is unset OR memory.valid_from_seq <= query.at_conversation_seq)
AND (query.at_conversation_seq is unset OR memory.valid_to_seq is null OR memory.valid_to_seq >= query.at_conversation_seq)
AND memory_event.status = ACTIVE
AND review_state != REJECTED
```

If a memory event has multiple source refs from several conversations, the first
version can return it only when at least one cited source is visible and all
non-visible cited source snippets are suppressed. The retrieval layer may later
split mixed-scope evidence into separate EvidencePack items.

## 5. Projection Events

First version supports the same essential timeline event families as search:

| Event | Memory behavior |
| --- | --- |
| `message.persisted.v1` | Creates PENDING structured memory candidate only when extraction rules identify a durable task / decision / status signal |
| `message.edited.v1` | Updates source ref projection and may supersede candidate memory |
| `message.revoked.v1` | Hides or tombstones memory derived solely from the revoked message |
| `message.deleted.v1` | Deletes or tombstones memory derived from retention-cleaned messages |
| `conversation.member.joined.v1` | Opens visibility window |
| `conversation.member.left.v1` / `removed.v1` | Closes visibility window |
| `conversation.member.role_changed.v1` | Updates role / permission version without changing join_seq |
| `conversation.member.owner_transferred.v1` | Updates role projection |

Malformed or unsupported event handling:

- Do not write memory rows.
- Do not advance checkpoint.
- Fail closed for the partition until repair / DLQ exists.

## 6. Database Contract

Migration: `migrations/postgres/memory/000001_memory_core.sql`.

Tables:

- `memory_structured_events`
- `memory_event_source_refs`
- `memory_membership_projection`
- `memory_graph_edges`
- `memory_profile_aggregates`
- `memory_projection_checkpoints`

PostgreSQL remains the first projection store. A vector store or graph backend
may be added behind ports later, but it must be rebuildable from these facts and
must not become the source of truth.

## 7. API Errors

| Internal error | gRPC code | Public message |
| --- | --- | --- |
| `INVALID_ARGUMENT` | InvalidArgument | invalid memory request |
| `PERMISSION_DENIED` | PermissionDenied | permission denied |
| `MEMORY_UNAVAILABLE` | Unavailable | memory unavailable |
| `PROJECTION_STALE` | Unavailable | memory projection stale |
| `MEMORY_NOT_FOUND` | NotFound | memory not found |

Public errors must not expose SQL, provider body, model prompt, raw source text
from non-visible evidence, or internal repair details.

## 8. Acceptance for Current First Paths

Current first paths are accepted when:

- `memory_service.proto` exposes query contracts, structured memory DTOs, graph
  edge DTOs, profile aggregate DTOs, and `RecomputeProfileAggregate`.
- `000001_memory_core.sql` defines the projection schema and invariants.
- `services/memory-service` provides six-layer `grpc` and `timeline-consumer`
  runtime modes.
- PostgreSQL integration tests cover projection visibility, source refs,
  current-window queries, graph edges, profile aggregate visibility, and
  recompute / archive behavior.
- `loadtest/memory` proves that the runtime path can query current memory,
  preserve graph edges, call `RecomputeProfileAggregate`, and exclude stale
  profile aggregates whose support was deleted.

Next hardening should add repair / operator flows and richer extraction worker
integration; it must still avoid turning a single group message into an ACTIVE
personal profile fact.
