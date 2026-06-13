# policy-service Loadtest Reports

This directory is the entry point for `policy-service` smoke reports. The current implementation is not a capacity-tested policy engine; it is a first-stage service boundary for message action decisions.

## Current Status

Implemented:

- `PolicyService.CheckMessageAction` gRPC contract.
- Static first-stage message action decision configured by environment.
- Optional exact-match PostgreSQL message action rule store through `NEXUSIM_POLICY_RULES_ENABLED=true`.
- Optional message-service RPC adapter through `NEXUSIM_POLICY_SERVICE_ADDR`.
- message-service fallback to legacy `StaticPolicy` when no policy-service address is configured.
- Optional debug server through `NEXUSIM_POLICY_DEBUG_ADDR` with `/healthz`, `/readyz`, `/debug/metrics`, aggregate gRPC metrics, aggregate decision metrics and PostgreSQL rule-store summaries.
- message-service policy RPC trace / request metadata propagation for policy-service structured gRPC logs.
- Contacts event projection consumer through `NEXUSIM_POLICY_SERVICE_MODE=contact-consumer`, storing directed contact edges in `policy_contact_edges_projection`.
- Direct gRPC allow/deny smoke for `SEND`, `EDIT`, `REVOKE`, and `DELETE`: `loadtest-report-20260613-policy-service-smoke.md`.
- message-service `SendMessage` allow/deny integration smoke through `NEXUSIM_POLICY_SERVICE_ADDR`: `loadtest-report-20260613-policy-message-integration-smoke.md`.
- message-service `SendMessage` allow/deny integration smoke through PostgreSQL-backed exact policy rules: `loadtest-report-20260613-policy-message-rule-store-smoke.md`.
- message-service `EditMessage` / `RevokeMessage` / `DeleteMessage` allow/deny integration smoke through PostgreSQL-backed exact policy rules: `loadtest-report-20260613-policy-message-actions-rule-store-smoke.md`.
- policy-service observability smoke for gRPC and decision metrics: `loadtest-report-20260613-policy-service-observability-smoke.md`.
- policy-service contact projection smoke for accepted / blocked / unblocked contact events: `loadtest-report-20260613-policy-contact-projection-smoke.md`.
- policy-service contact block decision smoke for direct `SEND` hard-deny through `direct_peer_user_id`: `loadtest-report-20260613-policy-contact-block-decision-smoke.md`.
- policy-service first-stage decision audit outbox smoke: `loadtest-report-20260613-policy-decision-audit-outbox-smoke.md`.
- policy-service decision audit relay smoke for `policy_decision_audit_outbox -> im.policy.events`: `loadtest-report-20260613-policy-decision-audit-relay-smoke.md`.
- policy-service decision audit DLQ repair smoke for explicit event-id redrive: `loadtest-report-20260613-policy-decision-audit-repair-smoke.md`.
- policy-service decision audit repair validation smoke after preflight gate: `loadtest-report-20260613-policy-decision-audit-repair-validated-smoke.md`.

Not yet implemented:

- group / role policy based on contacts or conversation membership;
- conversation role policy;
- tenant-level policy;
- content moderation / risk scoring;
- policy audit retention, external sink, poison-payload classification and broad repair workflow;
- policy-service mTLS, OpenTelemetry, Prometheus deployment and production alerting.

## Local Smoke Shape

```text
policy-service grpc
-> CheckMessageAction(SEND / EDIT / REVOKE / DELETE)
-> allow / deny response echo + permission_version + classification + reason
```

The first smoke is intentionally direct against `policy-service` public gRPC. It proves the service process and contract are runnable without adding PostgreSQL or Kafka noise.

Run direct policy-service gRPC smoke with:

```powershell
.\loadtest\policy\run-local-smoke.ps1
```

Run message-service integration smoke with:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1
```

Run the PostgreSQL exact-rule integration smoke with:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 -UsePolicyRules
```

Run exact-rule mutation action coverage with:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 -UsePolicyRules -Actions edit,revoke,delete
```

Run contact projection smoke with:

```powershell
.\loadtest\policycontacts\run-local-smoke.ps1
```

The same runner now also starts `policy-service grpc` and validates direct conversation block enforcement when policy receives `direct_peer_user_id`:

```text
im.contact.events
-> policy_contact_edges_projection
-> CheckMessageAction(SEND, direct_peer_user_id)
-> CONTACT_BLOCKED hard deny / allow after unblock
-> policy_decision_audit_outbox PENDING rows
-> policy decision audit outbox relay
-> im.policy.events readback
-> optional explicit DLQ event repair
```

Raw summaries are written under `H:\NexusIM\loadtest-results\<run-name>`:

```text
allow\policy-summary.json
deny\policy-summary.json
policy-smoke-summary.json
policy-contact-summary.json
```

The message mutation integration shape is:

```text
policy-service grpc
-> message-service NEXUSIM_POLICY_SERVICE_ADDR
-> SendMessage / EditMessage / RevokeMessage / DeleteMessage
-> policy-service CheckMessageAction
-> message-service normal transaction / public deny
```

When testing through `message-service`, keep the policy permission version aligned with conversation permission version to avoid expected dependency-version mismatch. The integration smoke intentionally sets local mock policy opposite to remote policy decision so fallback cannot produce a false positive. Do not treat these smokes as proof of contacts / role / tenant / risk policy behavior.

The rule-store smoke also sets local static fallback opposite to the seeded PostgreSQL rule. That makes a rule miss visible: allow would become deny, and deny would become an unexpected write.

The observability smoke reads `/debug/metrics` after both allow and deny scenarios. Metrics are aggregate debug snapshots only: they do not expose tenant id, user id, conversation id, message id, policy request bodies, rule parameters, deny reason text or classification strings. Trace id and request id are propagated for structured gRPC logs, not as metrics labels.

The contact projection smoke proves that policy-service can consume `im.contact.events` and maintain a policy-owned edge projection. The contact block decision smoke proves that projected `BLOCKED` edges are consumed for direct `SEND` when `direct_peer_user_id` is present. It does not prove group conversation role policy, tenant policy, risk scoring or full ReBAC behavior.

The decision audit outbox smoke proves that public policy decisions are staged as low-sensitive `policy_decision_audit_outbox` rows when PostgreSQL rules mode is enabled. The decision audit relay smoke proves those rows can be published to `im.policy.events` as protobuf `PolicyEvent` records and marked `PUBLISHED`. Audit rows and Kafka events store stable object keys, context-present flags, action, allow/deny, permission version, classification, reason code and trace/request ids. They do not store raw session id, raw device id, raw peer id, raw conversation id, raw message content, rule parameters, SQL errors or free-text deny/provider bodies. Explicit DLQ event-id repair is available; broad repair workflow, poison-payload classification, retention and external audit sinks remain future work.
The decision audit repair smoke proves the first-stage operator path for explicit DLQ event IDs:

```powershell
.\loadtest\policycontacts\run-local-smoke.ps1 -RunName policy-decision-audit-repair-smoke-20260613-clean -ExerciseAuditRepair
```

`NEXUSIM_POLICY_SERVICE_MODE=outbox-repair` only resets explicitly supplied DLQ rows back to `PENDING`, clears retry state, and writes `policy_decision_audit_outbox_repair_audit`. Before redrive, it validates the event through the same policy-event builder used by the relay. Invalid envelope or payload rows stay in `DLQ`, get a `SKIPPED / validation_failed` repair audit row, and make the operator exit non-zero. The repair mode does not publish Kafka directly, skip ordered blockers, rewrite payloads, repair all DLQ rows, implement retention, or send audit data to an external sink. After repair, the normal outbox relay remains responsible for publishing to `im.policy.events`.
