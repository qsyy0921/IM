# policy-service SDD

`policy-service` owns first-stage policy decisions that must not be hard-coded inside message-service or future Agent / MCP entry points. The initial message implementation exposes a gRPC `CheckMessageAction` endpoint for message send / edit / revoke / delete decisions, and it returns a stable `permission_version`, `classification`, allow/deny flag and public deny reason. The first-stage AI/action closeout also exposes `CheckToolAction` so future Agent / MCP / Skill callers can precheck tool actions through policy-service before creating proposals or executing actions.

This service is a boundary extraction step. It is not yet a full ReBAC engine, tenant policy DSL / quota / risk engine, provider-grade content moderation platform, provider-grade tool policy engine or complete contacts/conversation policy engine. The current contacts projection is consumed only for direct conversation `SEND` hard-deny when the caller supplies safe direct-peer context, and as first-stage relationship evidence when a `DIRECT_CONTACT_ACTIVE` relation rule is configured. The current conversation member projection is consumed only as a first-stage role gate with a permission-version fence, as first-stage `CONVERSATION_MEMBER_ACTIVE` relationship evidence, and as a narrow `ADMIN` / `OWNER` message ownership override for mutations. The current moderation slices are limited to user-level message action restrictions such as tenant-local mute / action deny rules, plus first-stage configurable keyword and HTTP content moderator adapters for `SEND` / `EDIT` message text. The current tool policy slice is a precheck and low-sensitive local audit surface; it is not an Agent service, MCP gateway, approval system or action executor. A separate `MODERATOR` role, richer risk scoring and full product-level moderation policy remain future work.

## Boundary

Owns:

- policy decision API contracts;
- message action allow / deny decisions;
- policy version and classification returned to callers;
- policy-owned contacts edge and conversation member projection read models;
- a policy-owned first-stage decision audit outbox;
- policy-owned first-stage tool action rules and low-sensitive tool decision audit rows;
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

The first slice keeps an explicit message-service `StaticPolicy` default for local smoke. When `NEXUSIM_POLICY_SERVICE_ADDR` is set, message-service calls policy-service over gRPC instead.

The second slice adds an optional policy-service owned PostgreSQL rule table. The first table is exact-match only:

```text
tenant_id + user_id + conversation_id + action
```

The next table adds a tenant action default:

```text
tenant_id + action
```

When `NEXUSIM_POLICY_RULES_ENABLED=true`, policy-service first evaluates hard contact-block denies for direct `SEND` requests that include `direct_peer_user_id`, then evaluates first-stage user-level message action restrictions from `policy_user_message_action_restrictions`. Active user restrictions are hard denies and intentionally override exact / tenant allow rules. It then applies the message ownership gate for mutation requests that include `message_sender_user_id`. Non-sender mutations can pass only through an explicit `policy_message_ownership_override_rules` row backed by a fresh `policy_conversation_members_projection` `ADMIN` / `OWNER` row. After ownership has passed, policy-service applies the conversation role gate, then applies first-stage `policy_rebac_message_action_rules` relationship gates, then tenant action quota, then checks `policy_message_action_rules`, then checks `policy_tenant_message_action_rules`, and finally falls back to the static policy. A matching exact or tenant rule returns its allow / deny decision, `permission_version`, `classification` and public reason. Exact user/conversation rules intentionally override tenant defaults only after the hard gates have passed. The `CheckMessageAction` hot path reads contact, restriction, role/member, ReBAC, quota and exact/tenant rule facts with one combined PostgreSQL facts query, then applies this same precedence in memory.

Policy revisions are service-owned logical versions, not wall-clock timestamps:

```text
policy_revision_state:
tenant_id + scope_type + scope_id + action -> revision
```

PostgreSQL triggers bump the relevant scoped revision in the same transaction as policy rule, restriction, quota, contact projection or conversation member projection writes. The optional Redis decision cache is enabled with `NEXUSIM_POLICY_DECISION_CACHE_BACKEND=redis`; before a cache lookup, policy-service reads the relevant revision vector and hashes it into the cache key along with tenant, user, conversation, action and caller-supplied conversation permission version. TTL is only cleanup for old keys. User restriction decisions are not cached because `expires_at` can pass without a write, and any path with an enabled tenant quota row is not cached because quota depends on current audit counts. Static default decisions are also not cached, because there is no policy fact row to bind to a meaningful policy revision. Cache hits still go through the normal use-case audit write before the gRPC response returns. The local Docker hotgroup profile keeps this cache disabled by default and turns it on only for workloads dominated by cacheable rule/fact decisions.

The first-stage moderation restriction table is deny-only:

```text
policy_user_message_action_restrictions:
tenant_id + user_id + action -> permission_version + classification + public reason + optional expires_at
```

It is intended for low-cardinality tenant-local controls such as temporarily preventing a user from sending messages. Expired rows are ignored by the evaluator. The table does not store message content, risk features, raw moderation provider output, prompt text or model output. PostgreSQL errors fail closed as policy unavailable.

The first-stage ReBAC relationship gate is deny-only. It is not a complete graph engine, but it lets policy-service express a small set of relationship requirements using only policy-owned projections:

```text
policy_rebac_message_action_rules:
tenant_id + action + relation_type + conversation_scope
-> permission_version + classification + public reason + priority + enabled

relation_type:
  DIRECT_CONTACT_ACTIVE
  CONVERSATION_MEMBER_ACTIVE

conversation_scope:
  ANY
  DIRECT
  GROUP
```

When an enabled relation rule matches the request action and scope, the required relation must be proven before tenant quota, exact allow rules or tenant allow rules can apply. `DIRECT_CONTACT_ACTIVE` requires a direct peer context and two directed `ACTIVE` rows in `policy_contact_edges_projection`; if either edge is missing, policy-service returns `allowed=false` with `decision_source=REBAC_RELATION`. `CONVERSATION_MEMBER_ACTIVE` requires a fresh `policy_conversation_members_projection` row whose `permission_version` matches the caller-supplied `conversation_permission_version` and whose status is `ACTIVE`; missing or stale projection remains policy unavailable, while an inactive member is denied. A satisfied relationship gate only allows evaluation to continue; it never grants permission by itself.

The first-stage content moderation adapter is configured with:

```text
NEXUSIM_POLICY_MODERATION_MODE=keyword
NEXUSIM_POLICY_MODERATION_DENY_TERMS=term1,term2
NEXUSIM_POLICY_MODERATION_PERMISSION_VERSION=1
NEXUSIM_POLICY_MODERATION_CLASSIFICATION=CONTENT_MODERATION_DENIED
NEXUSIM_POLICY_MODERATION_DENY_REASON=content moderation policy denied
```

HTTP provider mode is configured with:

```text
NEXUSIM_POLICY_MODERATION_MODE=http
NEXUSIM_POLICY_MODERATION_HTTP_ENDPOINT=https://moderation.example/check
NEXUSIM_POLICY_MODERATION_HTTP_BEARER_TOKEN=...
NEXUSIM_POLICY_MODERATION_HTTP_TIMEOUT=1s
NEXUSIM_POLICY_MODERATION_PERMISSION_VERSION=1
NEXUSIM_POLICY_MODERATION_CLASSIFICATION=CONTENT_PROVIDER_DENIED
NEXUSIM_POLICY_MODERATION_DENY_REASON=content moderation provider denied
```

`http://` provider endpoints are rejected unless `NEXUSIM_POLICY_MODERATION_HTTP_ALLOW_INSECURE=true` is explicitly set for local smoke. message-service forwards only `payload.text` for `SEND` / `EDIT`; it does not forward the whole message payload for policy classification. policy-service does not persist the text, does not write raw content or provider response bodies into `policy_decision_audit_outbox`, and returns only stable `classification` / public reason fields. Empty text, missing text or disabled moderation mode fall through to the normal contact / user restriction / role / exact / tenant / static policy path. Keyword mode is a local first-stage adapter; HTTP mode is a first-stage external provider adapter. Neither one is a complete risk score platform, prompt audit model or tenant policy DSL.

The first-stage tool policy precheck is intentionally separate from message action policy:

```text
CheckToolAction:
tenant_id + user_id + device_id
tool_name + action(CALL / APPROVE / EXECUTE)
resource_type + optional resource_id + risk_level
-> allowed + requires_approval + permission_version + classification + public reason + decision_source

policy_tool_action_rules:
tenant_id + tool_name(or *) + action + resource_type(or *) + risk_level(or ANY)
-> allowed + requires_approval + permission_version + classification + public reason + priority + enabled
```

`CheckToolAction` defaults to fail-closed static deny when rules are disabled or no matching rule exists. When PostgreSQL rules mode is enabled, the evaluator picks the most specific rule by exact tool, exact resource type and exact risk level before wildcard rows, then uses priority / updated time for deterministic tie-breaking. `requires_approval=true` means the caller may create or continue an approval/proposal flow, not that the action executor may mutate business facts immediately. Future Agent, MCP gateway and skill registry code must call this endpoint through a policy port; they must not read `policy_tool_action_rules` directly or hard-code tool allowlists in Agent code.

Successful `CheckToolAction` decisions can be written to `policy_tool_decision_audit` before the response is returned. Audit failure fails closed as policy unavailable. Tool audit rows are local first-stage audit rows, not Kafka outbox rows in this slice. They store only low-sensitive fields: stable actor/device keys, tool name, action, resource type, `resource_id_present`, risk level, allow / requires-approval flags, permission version, classification, reason code, decision source and trace/request id. They must not store tool payloads, `intent` text, business resource IDs, prompt text, model output, raw user/device/session identifiers, tokens, credentials, provider body or SQL error text.

The conversation role gate is a hard deny / freshness gate, not a complete role allow engine. `policy-service` consumes member boundary events from `conversation.timeline.events` into `policy_conversation_members_projection`. `message-service` forwards `conversation_permission_version` from the `ConversationSendContext` it already read from conversation-service. If a role gate rule exists in `policy_conversation_role_action_rules`, the caller's projected member row must exist, be at the same `permission_version`, be `ACTIVE`, and have a role rank greater than or equal to `min_role`. Missing projection, stale version or PostgreSQL lookup failure returns policy unavailable; insufficient role returns business deny with the projected `permission_version`. If the role gate passes, the request continues to exact / tenant / static decision logic.

The message ownership gate is a hard deny for first-stage mutation safety, with one narrow role override. It is not a complete moderation model. For `EDIT`, `REVOKE` and `DELETE`, message-service reads the message sender from its own message repository and forwards `message_sender_user_id` to policy-service. policy-service does not read message-service tables or synchronously query message-service. If sender context is present and differs from the actor, policy-service first checks `policy_message_ownership_override_rules`. If no matching override rule exists, the caller's member projection is missing / stale / inactive, or the caller role rank is below the rule `min_role`, policy-service returns:

```text
allowed=false
classification=MESSAGE_OWNERSHIP_DENIED
reason=message ownership policy denied
permission_version=<conversation_permission_version>
```

If sender context is absent, policy-service treats the call as legacy-compatible and continues to the role / exact / tenant / static decision path. If sender context is present and the actor is not the sender, `conversation_permission_version` is required; missing version returns policy unavailable so audit outbox rows cannot be written with invalid metadata. message-service still keeps the transactional sender check in its mutation repositories as the final defense.

If an ownership override rule exists for the tenant/action, the caller's projected member row must exist, match `conversation_permission_version`, be `ACTIVE`, and have role `ADMIN` or `OWNER` at or above the configured `min_role`. A successful override returns:

```text
allowed=true
classification=MESSAGE_OWNERSHIP_ROLE_OVERRIDE
ownership_override=true
permission_version=<conversation_permission_version>
```

The `ownership_override` response field is a typed internal proof from policy-service to message-service. message-service rejects ownership override on `SEND`, on denied decisions, or on mismatched responses; mutation repositories use this boolean, not the classification string, to allow a non-sender transaction. Static default, exact rules and tenant rules cannot by themselves override message ownership.

PostgreSQL lookup errors return policy unavailable so a broken rule store cannot silently bypass a deny rule. When rules mode is enabled, policy migrations are required before serving traffic; missing policy tables or a missing revision table are deployment errors, not an implicit allow path.

The contacts projection slice consumes `im.contact.events` into policy-service owned tables:

```text
im.contact.events
-> policy-service contact-consumer
-> policy_contact_edges_projection
-> policy_kafka_checkpoints
```

`contact.request.accepted.v1` writes both directed edges as `ACTIVE`. `contact.edge.blocked.v1`, `contact.edge.unblocked.v1`, `contact.edge.deleted.v1` and `contact.edge.remark_updated.v1` update only the owner-scoped directed edge. Updates apply only when the incoming `edge_version` is newer than the stored version, so duplicate or stale contact events are no-ops. This projection does not read contacts-service internal tables and remains rebuildable from `im.contact.events`.

For direct conversations, `conversation-service` derives `direct_peer_user_id` from its own membership facts, `message-service` forwards that context to `policy-service`, and `CheckMessageAction(SEND)` checks the contact projection before exact message rules, tenant action defaults or static default. If either directed edge between sender and direct peer is `BLOCKED`, policy-service returns:

```text
allowed=false
classification=CONTACT_BLOCKED
reason=contact blocked
permission_version=<blocked edge_version>
```

Policy-service does not guess direct peers and does not synchronously query contacts-service or conversation-service. If no `direct_peer_user_id` is supplied, contacts block enforcement is skipped and the request continues to the exact rule / tenant rule / static decision path.

When PostgreSQL rules mode is enabled, successful `CheckMessageAction` decisions are audited before the response is returned. The default sink is `NEXUSIM_POLICY_DECISION_AUDIT_SINK=postgres`, which stages rows into `policy_decision_audit_outbox`; audit write failure fails closed as policy unavailable. The gRPC runtime opens an isolated audit PostgreSQL pool, controlled by `NEXUSIM_POLICY_AUDIT_PG_DSN` and `NEXUSIM_POLICY_AUDIT_PG_MAX_CONNS`, so synchronous audit inserts do not compete with policy facts reads for the same pgx pool. The default audit pool size is 32 after the hotgroup pressure runs showed 16 connections queueing under 768 sender concurrency. `NEXUSIM_POLICY_SERVICE_MODE=outbox-relay` publishes these rows to `im.policy.events` as protobuf `PolicyEvent` records and marks successful rows `PUBLISHED`; local Docker starts this relay as `policy-service-outbox-relay` so audit rows do not accumulate indefinitely as `PENDING`. Policy decision audit events are immutable audit facts, not a state-rebuild timeline, so this relay uses unordered partition publishing; a lower-version `PENDING` / `DLQ` audit row does not block later audit facts for the same conversation.

`NEXUSIM_POLICY_DECISION_AUDIT_SINK=kafka` is the first-stage reliable async-audit pressure profile. It builds the same low-sensitive `PolicyEvent` payload in the gRPC request path, publishes one record to `NEXUSIM_POLICY_AUDIT_EVENTS_TOPIC`, and waits for the Kafka producer ACK before returning the policy decision. Publish / marshal / configuration failure still fails closed as policy unavailable. This mode removes the synchronous `policy_decision_audit_outbox` PostgreSQL insert from the SendMessage critical path; it does not by itself write a PostgreSQL audit row, so downstream audit consumers must dedupe by `event_id` and persist or export the Kafka event stream according to the target audit retention policy. The current Go adapter uses `kafka-go` with `acks=all`, bounded retries, no auto topic creation, and event-id idempotency at the consumer/sink layer; `kafka-go` does not expose Kafka's producer idempotence flag, so product-grade exactly-once sink semantics still require a dedicated idempotent sink or a lower-level Kafka producer.

`NEXUSIM_POLICY_SERVICE_MODE=outbox-audit` is a read-only operator view over `policy_decision_audit_outbox`. It supports `outbox_id / event_id / tenant_id / aggregate_id / status / event_type / created_at RFC3339 window` filters, returns newest rows first, and never mutates relay state, retry state or repair history.

`NEXUSIM_POLICY_SERVICE_MODE=outbox-repair` is the first-stage repair operator for policy decision audit rows. It accepts an explicit comma-separated list of DLQ `event_id` values, validates each DLQ row through the same policy-event builder used by the relay, resets only valid rows to `PENDING`, clears retry state, and writes `policy_decision_audit_outbox_repair_audit`. Invalid envelope or payload rows stay in `DLQ`, write a `SKIPPED / validation_failed` audit row, and make the operator return a non-zero error so automation cannot mistake a poison row for a clean repair. It does not publish Kafka directly, skip ordered blockers, rewrite payloads, repair all rows, implement retention or export audit data to an external sink. After repair, the normal outbox relay is still responsible for publishing to `im.policy.events`.

`NEXUSIM_POLICY_SERVICE_MODE=outbox-repair-audit` is a read-only operator view over `policy_decision_audit_outbox_repair_audit`. It supports `event_id / tenant_id / repair_operator / repair_outcome` filters, returns newest rows first, and never mutates outbox state.

`NEXUSIM_POLICY_SERVICE_MODE=outbox-repair-cleanup` is a retention operator for `policy_decision_audit_outbox_repair_audit`. It deletes oldest repair audit rows before `now - retention`, supports optional `event_id / tenant_id / repair_operator / repair_outcome` filters for scoped cleanup, supports `NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_DRY_RUN=true` to count matching rows without deleting, records `dry_run` in the low-sensitive JSON summary, and never mutates the live outbox rows themselves.

`NEXUSIM_POLICY_SERVICE_MODE=decision-audit-export` is a read-only local handoff over `policy_decision_audit_outbox`. It supports tenant / event / action / allowed / classification / reason_code / decision_source / status / created_at RFC3339 window filters and can write a low-sensitive JSON artifact via `NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_OUTPUT`. The export never includes `payload_json`, message content, provider body, raw identifiers, tokens, credentials or SQL error text.

`NEXUSIM_POLICY_SERVICE_MODE=decision-audit-forward` is the first-stage external audit sink handoff. It uses the same low-sensitive export row shape, POSTs a bounded batch to `NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ENDPOINT`, requires HTTPS by default, and supports `NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_DRY_RUN=true` plus a low-sensitive summary at `NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_OUTPUT`. Non-2xx responses fail the operator with a stable error class and do not persist provider response bodies. This mode is a local/provider-integration handoff, not a provider-grade streaming audit pipeline: it does not keep an external delivery checkpoint, retry queue, DLQ, tenant routing policy or exactly-once external sink semantics.

`NEXUSIM_POLICY_SERVICE_MODE=rebac-relation-audit` is a read-only operator view over `policy_rebac_message_action_rules`. It supports tenant / action / relation type / conversation scope / enabled filters, returns newest rows first, and can write a low-sensitive JSON result via `NEXUSIM_POLICY_REBAC_RELATION_AUDIT_OUTPUT`. The JSON output contains only rule metadata and `reason_present`; it does not output the operator reason text.

`NEXUSIM_POLICY_SERVICE_MODE=rebac-relation-set` upserts one first-stage relation gate rule by tenant / action / relation type / conversation scope. It requires positive `permission_version`, non-empty `classification`, non-negative `priority`, and supports disabling rules with `NEXUSIM_POLICY_REBAC_RELATION_SET_ENABLED=false`. `NEXUSIM_POLICY_REBAC_RELATION_SET_REASON_FILE` may be used to read the operator reason from a local file so the reason does not appear in shell history or operator plans. The optional `NEXUSIM_POLICY_REBAC_RELATION_SET_OUTPUT` JSON output contains only rule metadata and `reason_present`. This remains a local first-stage operator, not a provider-grade ReBAC graph engine or tenant policy DSL.

Audit rows intentionally store low-sensitive decision metadata:

- stable object keys for actor user, device, conversation, message and direct peer context;
- context-present booleans such as `message_id_present` and `direct_peer_context_present`;
- action, allowed, permission version, classification, stable `decision_source` and bounded reason code;
- trace id and request id for correlation.

Audit rows must not store message content, raw session id, raw device id, raw direct peer id, raw conversation id, raw message id, raw rule parameters, SQL error text, DSNs, tokens, credentials or free-text provider/body data.

For the first-stage ownership gate, audit rows identify ownership denies by `classification=MESSAGE_OWNERSHIP_DENIED` / `reason_code=MESSAGE_OWNERSHIP_DENIED` and ownership overrides by `classification=MESSAGE_OWNERSHIP_ROLE_OVERRIDE`. Audit rows keep the stable `message_key` and intentionally do not store raw message sender id in this slice. A future audit schema can add a low-sensitive `message_sender_context_present` flag, ownership override flag or sender stable key if operations needs more ownership-specific attribution.

`decision_source` is low-sensitive path metadata. Current values include `STATIC_DEFAULT`, `EXACT_RULE`, `TENANT_RULE`, `USER_RESTRICTION`, `TENANT_QUOTA`, `REBAC_RELATION`, `CONVERSATION_ROLE`, `CONTACT_PROJECTION`, `MESSAGE_OWNERSHIP`, `OWNERSHIP_OVERRIDE` and `CONTENT_MODERATION`. It is not a free-text reason and must not contain provider body, message content, raw rule parameters or raw user identifiers.

Configuration:

```text
NEXUSIM_POLICY_SERVICE_MODE=grpc
NEXUSIM_POLICY_GRPC_ADDR=0.0.0.0:10800
NEXUSIM_POLICY_GRPC_TLS_CERT_FILE=
NEXUSIM_POLICY_GRPC_TLS_KEY_FILE=
NEXUSIM_POLICY_GRPC_TLS_CLIENT_CA_FILE=
NEXUSIM_POLICY_GRPC_TLS_REQUIRE_CLIENT_CERT=false
NEXUSIM_POLICY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES=
NEXUSIM_POLICY_GRPC_TLS_CLIENT_ALLOWED_URIS=
NEXUSIM_POLICY_MESSAGE_ALLOWED=true
NEXUSIM_POLICY_PERMISSION_VERSION=1
NEXUSIM_POLICY_CLASSIFICATION=INTERNAL
NEXUSIM_POLICY_DENY_REASON=
NEXUSIM_POLICY_RULES_ENABLED=false
NEXUSIM_PG_DSN=
NEXUSIM_POLICY_PG_MAX_CONNS=
NEXUSIM_POLICY_AUDIT_PG_DSN=
NEXUSIM_POLICY_AUDIT_PG_MAX_CONNS=32
NEXUSIM_KAFKA_BROKERS=localhost:9092
NEXUSIM_POLICY_DECISION_AUDIT_SINK=postgres
NEXUSIM_POLICY_AUDIT_EVENTS_TOPIC=im.policy.events
NEXUSIM_POLICY_DECISION_AUDIT_KAFKA_BATCH_SIZE=1
NEXUSIM_POLICY_DECISION_AUDIT_KAFKA_BATCH_TIMEOUT=1ms
NEXUSIM_POLICY_DECISION_CACHE_BACKEND=
NEXUSIM_POLICY_DECISION_CACHE_ENABLED=false
NEXUSIM_POLICY_DECISION_CACHE_REDIS_MODE=single
NEXUSIM_POLICY_DECISION_CACHE_REDIS_ADDR=
NEXUSIM_POLICY_DECISION_CACHE_REDIS_USERNAME=
NEXUSIM_POLICY_DECISION_CACHE_REDIS_PASSWORD=
NEXUSIM_POLICY_DECISION_CACHE_REDIS_DB=0
NEXUSIM_POLICY_DECISION_CACHE_KEY_PREFIX=nexusim:policy
NEXUSIM_POLICY_DECISION_CACHE_TTL=30s
NEXUSIM_POLICY_DEBUG_ADDR=
NEXUSIM_DEBUG_ADDR=
NEXUSIM_POLICY_OTEL_TRACES_ENABLED=false
NEXUSIM_POLICY_OTEL_SERVICE_NAME=policy-service
NEXUSIM_POLICY_OTEL_TRACES_EXPORTER=stdout
NEXUSIM_POLICY_OTEL_TRACES_OTLP_ENDPOINT=
NEXUSIM_POLICY_OTEL_TRACES_OTLP_INSECURE=false
NEXUSIM_POLICY_OTEL_TRACES_SAMPLING_RATIO=1

NEXUSIM_POLICY_SERVICE_MODE=contact-consumer
NEXUSIM_KAFKA_BROKERS=localhost:9092
NEXUSIM_CONTACT_EVENTS_TOPIC=im.contact.events
NEXUSIM_POLICY_CONTACT_CONSUMER_GROUP=nexusim-policy-contacts

NEXUSIM_POLICY_SERVICE_MODE=timeline-consumer
NEXUSIM_KAFKA_BROKERS=localhost:9092
NEXUSIM_CONVERSATION_TIMELINE_TOPIC=conversation.timeline.events
NEXUSIM_POLICY_TIMELINE_CONSUMER_GROUP=nexusim-policy-timeline

NEXUSIM_POLICY_SERVICE_MODE=outbox-relay
NEXUSIM_KAFKA_BROKERS=localhost:9092
NEXUSIM_POLICY_AUDIT_EVENTS_TOPIC=im.policy.events
NEXUSIM_POLICY_OUTBOX_BATCH_SIZE=500
NEXUSIM_POLICY_OUTBOX_POLL_INTERVAL=1s
NEXUSIM_POLICY_OUTBOX_MAX_ATTEMPTS=5
NEXUSIM_POLICY_OUTBOX_RETRY_BASE_DELAY=1s

NEXUSIM_POLICY_SERVICE_MODE=outbox-audit
NEXUSIM_POLICY_OUTBOX_AUDIT_OUTBOX_ID=
NEXUSIM_POLICY_OUTBOX_AUDIT_EVENT_ID=
NEXUSIM_POLICY_OUTBOX_AUDIT_TENANT_ID=
NEXUSIM_POLICY_OUTBOX_AUDIT_AGGREGATE_ID=
NEXUSIM_POLICY_OUTBOX_AUDIT_STATUS=
NEXUSIM_POLICY_OUTBOX_AUDIT_EVENT_TYPE=
NEXUSIM_POLICY_OUTBOX_AUDIT_CREATED_AFTER=
NEXUSIM_POLICY_OUTBOX_AUDIT_CREATED_BEFORE=
NEXUSIM_POLICY_OUTBOX_AUDIT_LIMIT=20

NEXUSIM_POLICY_SERVICE_MODE=outbox-repair
NEXUSIM_POLICY_OUTBOX_REPAIR_EVENT_IDS=
NEXUSIM_POLICY_OUTBOX_REPAIR_OPERATOR=local-operator
NEXUSIM_POLICY_OUTBOX_REPAIR_REASON=manual policy audit outbox repair

NEXUSIM_POLICY_SERVICE_MODE=outbox-repair-audit
NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_EVENT_ID=
NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_TENANT_ID=
NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_OPERATOR=
NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_OUTCOME=
NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_LIMIT=20

NEXUSIM_POLICY_SERVICE_MODE=outbox-repair-cleanup
NEXUSIM_POLICY_OUTBOX_REPAIR_RETENTION=168h
NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_BATCH_SIZE=5000
NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_EVENT_ID=
NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_TENANT_ID=
NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OPERATOR=
NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OUTCOME=

NEXUSIM_POLICY_SERVICE_ADDR=127.0.0.1:10800
NEXUSIM_POLICY_RPC_TIMEOUT=30ms
NEXUSIM_POLICY_SERVICE_TLS_CA_FILE=
NEXUSIM_POLICY_SERVICE_TLS_SERVER_NAME=
NEXUSIM_POLICY_SERVICE_TLS_CLIENT_CERT_FILE=
NEXUSIM_POLICY_SERVICE_TLS_CLIENT_KEY_FILE=
```

`policy-service` defaults to plaintext for existing local smoke. If `NEXUSIM_POLICY_GRPC_TLS_CERT_FILE` or `NEXUSIM_POLICY_GRPC_TLS_KEY_FILE` is configured, both must be present and the gRPC server uses TLS 1.2 or newer. Supplying `NEXUSIM_POLICY_GRPC_TLS_CLIENT_CA_FILE`, `NEXUSIM_POLICY_GRPC_TLS_REQUIRE_CLIENT_CERT=true`, or a client DNS / URI allowlist enables mTLS. The allowlists are exact-match checks against verified client certificate SANs, intended for static local profiles such as `message-service.nexusim.local` or `spiffe://nexusim/message-service`. When `NEXUSIM_POLICY_GRPC_ADDR` is not loopback / RFC1918 private, policy-service must refuse to start unless entry gRPC TLS is enabled; first-stage plaintext is only allowed on private listeners.

message-service only enables TLS for policy RPC when `NEXUSIM_POLICY_SERVICE_TLS_CA_FILE` is configured. `NEXUSIM_POLICY_SERVICE_TLS_SERVER_NAME` is optional and overrides certificate hostname verification. Client certificate and key must be configured together for mTLS. A partial TLS configuration fails fast instead of silently falling back to plaintext.

Direct policy smoke clients use the same static transport model through runner flags:

```text
--policy-tls-ca-file
--policy-tls-server-name
--policy-tls-client-cert-file
--policy-tls-client-key-file
```

These flags are available in `loadtest/policy`, `loadtest/policycontacts`, and `loadtest/policyroles`. They only control the loadtest client connection to `policy-service`; server-side TLS remains configured through `NEXUSIM_POLICY_GRPC_TLS_*`.

## Contracts

`CheckMessageActionRequest` includes:

- `AuthContext`: tenant, user, device, session and trace/request IDs;
- `conversation_id`;
- `action`: `SEND`, `EDIT`, `REVOKE`, `DELETE`;
- `message_id`: required for edit / revoke / delete;
- `direct_peer_user_id`: optional direct conversation peer context used only for `SEND` contact-block decisions;
- `message_sender_user_id`: optional message sender context used only for `EDIT` / `REVOKE` / `DELETE` ownership decisions;
- `conversation_permission_version`: optional for legacy exact / tenant / static policy, required when a conversation role gate rule is configured or when a non-sender ownership deny is produced.

`CheckMessageActionResponse` includes:

- `allowed`;
- `permission_version`;
- `classification`;
- `decision_source`;
- `reason`;
- `message_id` echo for edit / revoke / delete;
- `ownership_override`: true only when policy-service explicitly authorizes a non-sender `EDIT` / `REVOKE` / `DELETE` via the ownership override rule and fresh member projection.

The response is a decision record. `allowed=false` is returned as a successful gRPC response so callers can preserve public deny semantics and use `reason` without relying on transport errors. Transport errors are reserved for invalid request, unavailable policy dependency or implementation failures.

`CheckToolActionRequest` includes:

- `AuthContext`: tenant, user, device, session and trace/request IDs;
- `tool_name`: stable tool identifier, such as a skill or MCP tool name;
- `action`: `CALL`, `APPROVE` or `EXECUTE`;
- `resource_type`: stable resource class such as `conversation`, `message`, `contact`, `tenant_config` or `external_tool`;
- `resource_id`: optional caller resource id; response may echo it, but low-sensitive audit stores only `resource_id_present`;
- `risk_level`: `LOW`, `MEDIUM`, `HIGH` or `CRITICAL`;
- `intent`: optional bounded caller intent summary. It is not persisted in policy audit tables.

`CheckToolActionResponse` includes:

- `allowed`;
- `requires_approval`;
- `permission_version`;
- `classification`;
- public `reason`;
- `decision_source`, currently `TOOL_RULE` or `STATIC_DEFAULT`.

## Message-Service Integration

message-service continues to call only its `PolicyCheckPort`. It does not import policy-service internals. The RPC adapter lives in `message-service/internal/infrastructure/rpc`.

For message mutations, message-service reads the sender from its own `message_log` through the message repository before calling policy-service. This keeps ownership evidence in the service that owns message facts, while moving the policy decision and audit surface to policy-service. The mutation repositories still validate original sender inside the write transaction, so a bypassed or stale policy call cannot mutate another user's message.

The adapter validates that policy-service response tenant, user, conversation, message id and action match the request, and rejects empty `classification` or non-positive `permission_version` as dependency errors. It also rejects `ownership_override=true` unless the decision is allowed and the action is `EDIT`, `REVOKE` or `DELETE`. This prevents a mixed-version or buggy policy-service from silently corrupting message timeline metadata or widening mutation ownership. Transport-level `PermissionDenied` from the policy RPC is treated as dependency failure; business deny must use gRPC OK with `allowed=false`.

The adapter forwards trace id and request id as gRPC metadata when they are present in the message-service auth context. policy-service uses them only for structured request logs. They are not metrics labels and are not part of the policy decision contract.

The policy RPC adapter can use static TLS / mTLS credentials configured by the message-service process. This is transport hardening only: the business decision contract remains the same, and transport-level `PermissionDenied` from mTLS or server policy is still mapped to dependency unavailable rather than a business deny.

## Observability

When `NEXUSIM_POLICY_DEBUG_ADDR` is set, policy-service exposes:

```text
/healthz
/readyz
/debug/metrics
/metrics
```

The debug metrics include aggregate gRPC request counts and status codes, aggregate final `CheckMessageAction` decision counts, per-action aggregate decision counts, optional policy PostgreSQL pool stats, optional isolated audit PostgreSQL pool stats, optional rule-store row counts, optional projection row counts / checkpoints and optional decision audit outbox status counts. Decision metrics are recorded at the use-case boundary, so they include static / exact / user-restriction / tenant / role / relationship decisions as well as first-stage ownership denies and `ownership_override=true` allows. The `policy_rule_store` snapshot includes low-cardinality row counts for exact message action rules, user message action restrictions, tenant action defaults, conversation role gate rules, ownership override rules and ReBAC relationship rules, grouped only by action / allow-deny / min-role / relation type / conversation scope where applicable. The `policy_projection` snapshot includes contact edge status counts, conversation member status / role counts and Kafka checkpoint topic aggregates. `/metrics` renders the same low-sensitive aggregate snapshot as Prometheus text for local scrape / dashboard prototypes, including `nexusim_policy_pg_pool_*`, `nexusim_policy_audit_pg_pool_*`, `nexusim_policy_evaluator_stage_*` and `nexusim_policy_decision_stage_*`. The local scrape target is `host.docker.internal:11916` when the service debug listener runs as `NEXUSIM_POLICY_DEBUG_ADDR=127.0.0.1:11916`. They intentionally do not expose tenant id, user id, conversation id, message id, device id, session id, request / response payloads, raw rule parameters, deny reason text, classification strings, DSNs or SQL error text.

The debug HTTP listener is intended for local smoke and private operational access. `NEXUSIM_POLICY_DEBUG_ADDR` is the service-specific setting; if it is unset, the shared `NEXUSIM_DEBUG_ADDR` value is used explicitly by the startup config. By default the listener only allows loopback / RFC1918 private addresses. Binding the debug listener to a public or unspecified address requires explicit `NEXUSIM_POLICY_DEBUG_ALLOW_PUBLIC=true`.

`allowed=false` is counted as a decision deny, while the gRPC method remains `codes.OK`. Transport errors are counted separately.

First-stage OpenTelemetry gRPC server spans are available in `grpc` mode and are disabled by default. When enabled, policy-service can export spans to stdout or an OTLP gRPC endpoint and extracts W3C `traceparent` from incoming gRPC metadata. Spans record only low-sensitive, low-cardinality transport fields: full gRPC method name, gRPC status code and latency. They must not record tokens, tenant id, user id, device id, session id, trace id, request id, conversation id, message id, direct peer id, sender id, payloads, rule parameters, classification, deny reason or SQL/provider error text. `x-nexusim-trace-id` / `x-nexusim-request-id` remain available for metadata / access-log correlation, but are not exported as span attributes.

This is a local debug surface plus first-stage Prometheus text and trace emission. It is not a replacement for production Prometheus deployment, collector-managed sampling, alert rules, SLO dashboard or external policy audit.

## Limitations

- First implementation still supports static environment configuration.
- PostgreSQL rule store supports exact user/conversation rules, first-stage user action restrictions, first-stage tenant action defaults, a first-stage conversation role gate, first-stage relationship gates and first-stage tool action rules; no provider-grade graph policy DSL yet.
- Contacts block / unblock events are consumed only for direct `SEND` when safe `direct_peer_user_id` context is supplied.
- ReBAC relationship policy is only a first-stage action-level gate backed by local contacts / conversation member projections and `policy_rebac_message_action_rules`. It is not a general graph traversal engine, Zanzibar/OpenFGA replacement, policy DSL, or synchronous query path into contacts-service / conversation-service.
- Conversation role policy is only an action-level role gate backed by a local projection and permission-version fence. It is not complete ReBAC and does not synchronously query conversation-service.
- Message ownership policy supports sender mutation and first-stage `ADMIN` / `OWNER` override for edit / revoke / delete when message-service supplies sender context. It does not implement a separate `MODERATOR` role, full ReBAC, owner transfer semantics, retention policy, compliance delete or user-private delete.
- No provider-grade tenant policy DSL, tool policy operator / approval integration, external provider-backed moderation, risk scoring or rate limiting is implemented yet. First-stage keyword content moderation exists only as a local adapter and does not store message text. First-stage tool policy precheck is a local rules and audit surface for future Agent / MCP / Skill callers; it is not an Agent service, MCP gateway, action executor or approval workflow.
- Decision audit outbox rows can be relayed to `im.policy.events`, and explicit DLQ event IDs can be redriven through the repair operator after relay-equivalent validation. Broad repair workflow, poison-payload classification beyond fail-closed validation, retention policy and external sink remain future work.
- First-stage static TLS / mTLS config exists for policy-service, the message-service policy RPC client and direct policy smoke clients, but there is no certificate issuance, rotation, dynamic service identity registry, service mesh policy, or all-service mTLS rollout yet.
- First-stage OpenTelemetry gRPC server spans exist, but there is no shared collector deployment, fleet-wide sampling policy, dashboard or alerting rollout yet.

These are future production hardening steps; the current value is extracting the policy boundary and replacing message-service internal policy rules with an optional real service dependency.
