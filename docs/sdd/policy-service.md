# policy-service SDD

`policy-service` owns first-stage policy decisions that must not be hard-coded inside message-service. The initial implementation is intentionally small: it exposes a gRPC `CheckMessageAction` endpoint for message send / edit / revoke / delete decisions, and it returns a stable `permission_version`, `classification`, allow/deny flag and public deny reason.

This service is a boundary extraction step. It is not yet a full ReBAC engine, tenant policy engine, content moderation platform, risk scoring system or contacts/conversation projection consumer.

## Boundary

Owns:

- policy decision API contracts;
- message action allow / deny decisions;
- policy version and classification returned to callers;
- future adapters for contacts, conversation, identity, tenant risk and compliance projections.

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

When `NEXUSIM_POLICY_RULES_ENABLED=true`, policy-service checks `policy_message_action_rules` first. A matching row returns its allow / deny decision, `permission_version`, `classification` and public reason. A clean rule miss falls back to the static policy. PostgreSQL lookup errors do not fall back; they return policy unavailable so a broken rule store cannot silently bypass a deny rule.

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

NEXUSIM_POLICY_SERVICE_ADDR=127.0.0.1:10800
NEXUSIM_POLICY_RPC_TIMEOUT=30ms
```

## Contracts

`CheckMessageActionRequest` includes:

- `AuthContext`: tenant, user, device, session and trace/request IDs;
- `conversation_id`;
- `action`: `SEND`, `EDIT`, `REVOKE`, `DELETE`;
- `message_id`: required for edit / revoke / delete.

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

The debug metrics include aggregate gRPC request counts and status codes, aggregate policy decision counts, per-action aggregate decision counts, optional PostgreSQL pool stats and optional exact-rule-store row counts. They intentionally do not expose tenant id, user id, conversation id, message id, device id, session id, request / response payloads, raw rule parameters, deny reason text, classification strings, DSNs or SQL error text.

`allowed=false` is counted as a decision deny, while the gRPC method remains `codes.OK`. Transport errors are counted separately.

This is a local debug surface. It is not a replacement for production OpenTelemetry traces, Prometheus deployment, alert rules or durable policy audit.

## Limitations

- First implementation still supports static environment configuration.
- PostgreSQL rule store is exact-match only; no wildcard / priority rule DSL yet.
- No contacts block projection is consumed yet.
- No conversation role / owner / admin policy is implemented yet.
- No tenant policy, content moderation, risk scoring, rate limiting or audit outbox is implemented yet.
- No mTLS client/server config is implemented for policy-service yet.
- No production OpenTelemetry / Prometheus / alerting rollout is implemented yet.

These are future production hardening steps; the current value is extracting the policy boundary and replacing message-service internal policy rules with an optional real service dependency.
