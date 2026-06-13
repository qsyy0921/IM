# policy-service SDD

`policy-service` owns first-stage policy decisions that must not be hard-coded inside message-service. The initial implementation is intentionally small: it exposes a gRPC `CheckMessageAction` endpoint for message send / edit / revoke / delete decisions, and it returns a stable `permission_version`, `classification`, allow/deny flag and public deny reason.

This service is a boundary extraction step. It is not yet a full ReBAC engine, tenant policy engine, content moderation platform, risk scoring system or complete contacts/conversation policy engine. The current contacts projection is consumed only for direct conversation `SEND` hard-deny when the caller supplies safe direct-peer context.

## Boundary

Owns:

- policy decision API contracts;
- message action allow / deny decisions;
- policy version and classification returned to callers;
- a policy-owned contacts edge projection read model;
- a policy-owned first-stage decision audit outbox;
- future adapters for conversation, identity, tenant risk and compliance projections.

Does not own:

- message facts or message mutation transactions;
- conversation membership facts;
- contacts facts;
- identity credentials or sessions;
- outbox publication for message timeline events.

## First Slice

```text
message-service
-> PolicyCheckPort
-> policy-service CheckMessageAction
-> static first-stage policy decision
```

The first slice keeps the legacy message-service `StaticPolicy` fallback for local smoke. When `NEXUSIM_POLICY_SERVICE_ADDR` is set, message-service calls policy-service over gRPC instead.

The second slice adds an optional policy-service owned PostgreSQL rule table. It is exact-match only:

```text
tenant_id + user_id + conversation_id + action
```

When `NEXUSIM_POLICY_RULES_ENABLED=true`, policy-service first evaluates hard contact-block denies for direct `SEND` requests that include `direct_peer_user_id`, then checks `policy_message_action_rules`. A matching exact rule returns its allow / deny decision, `permission_version`, `classification` and public reason. A clean rule miss falls back to the static policy. PostgreSQL lookup errors do not fall back; they return policy unavailable so a broken rule store cannot silently bypass a deny rule.

The contacts projection slice consumes `im.contact.events` into policy-service owned tables:

```text
im.contact.events
-> policy-service contact-consumer
-> policy_contact_edges_projection
-> policy_kafka_checkpoints
```

`contact.request.accepted.v1` writes both directed edges as `ACTIVE`. `contact.edge.blocked.v1`, `contact.edge.unblocked.v1`, `contact.edge.deleted.v1` and `contact.edge.remark_updated.v1` update only the owner-scoped directed edge. Updates apply only when the incoming `edge_version` is newer than the stored version, so duplicate or stale contact events are no-ops. This projection does not read contacts-service internal tables and remains rebuildable from `im.contact.events`.

For direct conversations, `conversation-service` derives `direct_peer_user_id` from its own membership facts, `message-service` forwards that context to `policy-service`, and `CheckMessageAction(SEND)` checks the contact projection before exact message rules or static fallback. If either directed edge between sender and direct peer is `BLOCKED`, policy-service returns:

```text
allowed=false
classification=CONTACT_BLOCKED
reason=contact blocked
permission_version=<blocked edge_version>
```

Policy-service does not guess direct peers and does not synchronously query contacts-service or conversation-service. If no `direct_peer_user_id` is supplied, contacts block enforcement is skipped and the request continues to the exact rule / static decision path.

When PostgreSQL rules mode is enabled, successful `CheckMessageAction` decisions are staged into `policy_decision_audit_outbox` before the response is returned. Audit write failure fails closed as policy unavailable. This outbox is local policy-service audit staging only; it is not yet relayed to Kafka.

Audit rows intentionally store low-sensitive decision metadata:

- stable object keys for actor user, device, conversation, message and direct peer context;
- context-present booleans such as `message_id_present` and `direct_peer_context_present`;
- action, allowed, permission version, classification and bounded reason code;
- trace id and request id for correlation.

Audit rows must not store message content, raw session id, raw device id, raw direct peer id, raw conversation id, raw message id, raw rule parameters, SQL error text, DSNs, tokens, credentials or free-text provider/body data.

Configuration:

```text
NEXUSIM_POLICY_SERVICE_MODE=grpc
NEXUSIM_POLICY_GRPC_ADDR=0.0.0.0:10800
NEXUSIM_POLICY_MESSAGE_ALLOWED=true
NEXUSIM_POLICY_PERMISSION_VERSION=1
NEXUSIM_POLICY_CLASSIFICATION=INTERNAL
NEXUSIM_POLICY_DENY_REASON=
NEXUSIM_POLICY_RULES_ENABLED=false
NEXUSIM_PG_DSN=
NEXUSIM_POLICY_PG_MAX_CONNS=
NEXUSIM_POLICY_DEBUG_ADDR=
NEXUSIM_DEBUG_ADDR=

NEXUSIM_POLICY_SERVICE_MODE=contact-consumer
NEXUSIM_KAFKA_BROKERS=localhost:9092
NEXUSIM_CONTACT_EVENTS_TOPIC=im.contact.events
NEXUSIM_POLICY_CONTACT_CONSUMER_GROUP=nexusim-policy-contacts

NEXUSIM_POLICY_SERVICE_ADDR=127.0.0.1:10800
NEXUSIM_POLICY_RPC_TIMEOUT=30ms
```

## Contracts

`CheckMessageActionRequest` includes:

- `AuthContext`: tenant, user, device, session and trace/request IDs;
- `conversation_id`;
- `action`: `SEND`, `EDIT`, `REVOKE`, `DELETE`;
- `message_id`: required for edit / revoke / delete;
- `direct_peer_user_id`: optional direct conversation peer context used only for `SEND` contact-block decisions.

`CheckMessageActionResponse` includes:

- `allowed`;
- `permission_version`;
- `classification`;
- `reason`;
- `message_id` echo for edit / revoke / delete.

The response is a decision record. `allowed=false` is returned as a successful gRPC response so callers can preserve public deny semantics and use `reason` without relying on transport errors. Transport errors are reserved for invalid request, unavailable policy dependency or implementation failures.

## Message-Service Integration

message-service continues to call only its `PolicyCheckPort`. It does not import policy-service internals. The RPC adapter lives in `message-service/internal/infrastructure/rpc`.

The adapter validates that policy-service response tenant, user, conversation, message id and action match the request, and rejects empty `classification` or non-positive `permission_version` as dependency errors. This prevents a mixed-version or buggy policy-service from silently corrupting message timeline metadata. Transport-level `PermissionDenied` from the policy RPC is treated as dependency failure; business deny must use gRPC OK with `allowed=false`.

The adapter forwards trace id and request id as gRPC metadata when they are present in the message-service auth context. policy-service uses them only for structured request logs. They are not metrics labels and are not part of the policy decision contract.

## Observability

When `NEXUSIM_POLICY_DEBUG_ADDR` is set, policy-service exposes:

```text
/healthz
/readyz
/debug/metrics
```

The debug metrics include aggregate gRPC request counts and status codes, aggregate policy decision counts, per-action aggregate decision counts, optional PostgreSQL pool stats, optional exact-rule-store row counts and optional decision audit outbox status counts. They intentionally do not expose tenant id, user id, conversation id, message id, device id, session id, request / response payloads, raw rule parameters, deny reason text, classification strings, DSNs or SQL error text.

`allowed=false` is counted as a decision deny, while the gRPC method remains `codes.OK`. Transport errors are counted separately.

This is a local debug surface. It is not a replacement for production OpenTelemetry traces, Prometheus deployment, alert rules or external policy audit.

## Limitations

- First implementation still supports static environment configuration.
- PostgreSQL rule store is exact-match only; no wildcard / priority rule DSL yet.
- Contacts block / unblock events are consumed only for direct `SEND` when safe `direct_peer_user_id` context is supplied. Group, role and tenant policy remain future work.
- No conversation role / owner / admin policy is implemented yet.
- No tenant policy, content moderation, risk scoring or rate limiting is implemented yet.
- Decision audit outbox rows remain local `PENDING` rows; no Kafka audit schema, relay, retry/DLQ smoke, repair operator, retention policy or external sink is implemented yet.
- No mTLS client/server config is implemented for policy-service yet.
- No production OpenTelemetry / Prometheus / alerting rollout is implemented yet.

These are future production hardening steps; the current value is extracting the policy boundary and replacing message-service internal policy rules with an optional real service dependency.
