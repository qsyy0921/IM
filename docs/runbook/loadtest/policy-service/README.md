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
- Conversation timeline projection consumer through `NEXUSIM_POLICY_SERVICE_MODE=timeline-consumer`, storing member role/status rows in `policy_conversation_members_projection`.
- First-stage conversation role gate through `policy_conversation_role_action_rules`, guarded by `conversation_permission_version` so stale projections fail closed as policy unavailable.
- First-stage message ownership gate for `EDIT` / `REVOKE` / `DELETE` when message-service supplies `message_sender_user_id`; non-sender mutations return `MESSAGE_OWNERSHIP_DENIED` unless an explicit `ADMIN` / `OWNER` ownership override rule and fresh conversation member projection produce typed `ownership_override=true`.
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
- policy-service 还补了只读 `outbox-repair-audit` 运维模式，方便直接审计 decision audit outbox repair 历史；它不直接 redrive，也不会修改当前 outbox 状态。
- policy-service 还补了 `outbox-repair-cleanup` 运维模式，方便按 retention / scope 清理 `policy_decision_audit_outbox_repair_audit` 历史；它只删除 repair audit 记录，不修改当前 outbox 状态。
- policy-service tenant-level message action rule smoke for `tenant_id + action` defaults across `SEND`, `EDIT`, `REVOKE`, and `DELETE`: `loadtest-report-20260613-policy-message-tenant-rule-smoke.md`.
- policy-service conversation role gate Kafka smoke for `conversation.timeline.events -> policy_conversation_members_projection -> CheckMessageAction`: `loadtest-report-20260613-policy-conversation-role-smoke.md`.
- message-service `SendMessage` role gate integration smoke through `policy-service CheckMessageAction`: `loadtest-report-20260613-policy-message-role-gate-smoke.md`.
- message-service `EditMessage` / `RevokeMessage` / `DeleteMessage` sender-only ownership integration smoke through `policy-service CheckMessageAction`: `loadtest-report-20260613-policy-message-ownership-smoke.md`.
- message-service `EditMessage` / `RevokeMessage` / `DeleteMessage` first-stage ownership override smoke for non-sender `ADMIN` allow and `MEMBER` deny: `loadtest-report-20260613-policy-message-ownership-override-smoke.md`.
- policy-service gRPC server and direct policy smoke clients support first-stage optional TLS / mTLS static config. `loadtest/policy`, `loadtest/policycontacts` and `loadtest/policyroles` accept optional CA, server name and client cert/key flags; default remains plaintext. The `loadtest/policyintegration` runner also supports optional message-service client TLS / mTLS flags and `--verified-auth-metadata` for the `message-service -> policy-service` integration smoke.
- policy-service direct mTLS smoke with client DNS SAN allowlist: `loadtest-report-20260613-policy-service-mtls-smoke.md`.

Not yet implemented:

- full ReBAC, a separate `MODERATOR` role and product-grade moderation policy;
- tenant-level policy DSL / quota / risk policy beyond first-stage action defaults;
- content moderation / risk scoring;
- policy audit retention, external sink, poison-payload classification and broad repair workflow;
- certificate issuance / rotation / distribution, dynamic service identity, service mesh rollout, OpenTelemetry, Prometheus deployment and production alerting.

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

Optional TLS / mTLS client flags are available for direct policy smokes:

```powershell
.\loadtest\policy\run-local-smoke.ps1 `
  -PolicyGrpcTlsCertFile .\certs\policy-server.crt `
  -PolicyGrpcTlsKeyFile .\certs\policy-server.key `
  -PolicyGrpcTlsClientCaFile .\certs\ca.pem `
  -PolicyGrpcTlsRequireClientCert true `
  -PolicyGrpcTlsClientAllowedDnsNames message-service.nexusim.local `
  -PolicyTlsCaFile .\certs\ca.pem `
  -PolicyTlsServerName policy-service.nexusim.local `
  -PolicyTlsClientCertFile .\certs\loadtest-client.crt `
  -PolicyTlsClientKeyFile .\certs\loadtest-client.key
```

For direct `loadtest\policy\run-local-smoke.ps1`, `PolicyGrpcTls*` parameters are injected into the policy-service process started by the script, while `PolicyTls*` parameters configure the loadtest client connection. The same client-side `-PolicyTls*` parameters are supported by `loadtest\policycontacts\run-local-smoke.ps1` and `loadtest\policyroles\run-local-smoke.ps1`; their server-side TLS still uses `NEXUSIM_POLICY_GRPC_TLS_*`.

Run message-service integration smoke with:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1
```

Optional message-service client TLS / mTLS flags are available for the integration runner:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 `
  -MessageTlsCaFile .\certs\ca.pem `
  -MessageTlsServerName message-service.nexusim.local `
  -MessageTlsClientCertFile .\certs\loadtest-client.crt `
  -MessageTlsClientKeyFile .\certs\loadtest-client.key
```

These flags only control the loadtest runner connection to message-service. Server-side message-service TLS still uses `NEXUSIM_MESSAGE_GRPC_TLS_*`.

To verify message-service gateway verified metadata auth while running the policy integration smoke:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 -VerifiedAuthMetadata
```

The script starts message-service with `NEXUSIM_MESSAGE_AUTH_MODE=metadata` and passes `--verified-auth-metadata` to the runner. This validates the message-service public entrypoint identity path; direct policy-service smokes remain service-level contract checks and do not use gateway auth mode.

Run the PostgreSQL exact-rule integration smoke with:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 -UsePolicyRules
```

Run exact-rule mutation action coverage with:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 -UsePolicyRules -Actions edit,revoke,delete
```

Run tenant-level action default coverage with:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 -UseTenantPolicyRules -Actions send,edit,revoke,delete
```

Run contact projection smoke with:

```powershell
.\loadtest\policycontacts\run-local-smoke.ps1
```

Run conversation role gate projection smoke with:

```powershell
.\loadtest\policyroles\run-local-smoke.ps1
```

Run message-service conversation role gate integration smoke with:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 -UseConversationRoleGate -Actions send
```

Run message-service sender-only ownership integration smoke with:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 -UseOwnershipGate -Actions edit,revoke,delete
```

Run message-service ownership override integration smoke with:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 -UseOwnershipOverride -Actions edit,revoke,delete
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
policy-role-summary.json
policy-message-smoke-summary.json
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

The rule-store smoke also sets local static fallback opposite to the seeded PostgreSQL rule. That makes a rule miss visible: allow would become deny, and deny would become an unexpected write. The tenant-rule smoke uses the same guard but seeds tenant-scoped `tenant_id + action` defaults. For mutation actions, it also seeds a tenant-level `SEND / POLICY_SEND_SEED` allow rule so the baseline message can be created before `EDIT` / `REVOKE` / `DELETE` is tested.

The observability smoke reads `/debug/metrics` after both allow and deny scenarios. Decision metrics are recorded at the final `CheckMessageAction` use-case boundary, so they include normal evaluator decisions plus first-stage message ownership denies and `ownership_override=true` allows. `policy_rule_store` is also a low-cardinality rule inventory: exact rules and tenant rules are grouped by action and allow/deny, while conversation role and ownership override rules are grouped by action and minimum role. `policy_projection` summarizes policy-owned contact edges, conversation member projection rows and Kafka checkpoint topics without listing tenants, users, conversations or partitions. Metrics are aggregate debug snapshots only: they do not expose tenant id, user id, conversation id, message id, policy request bodies, rule parameters, deny reason text or classification strings. Trace id and request id are propagated for structured gRPC logs, not as metrics labels.

The contact projection smoke proves that policy-service can consume `im.contact.events` and maintain a policy-owned edge projection. The contact block decision smoke proves that projected `BLOCKED` edges are consumed for direct `SEND` when `direct_peer_user_id` is present. The tenant-rule smoke proves only first-stage tenant action defaults. The conversation role gate projection smoke proves that policy-service can consume `conversation.timeline.events`, maintain member role/status projection, pass sufficient active roles through to tenant allow, deny insufficient/inactive roles, and fail closed when `conversation_permission_version` is stale. The message role gate integration smoke proves that message-service forwards the conversation permission version to policy-service, accepts `ADMIN/ACTIVE` projections, rejects insufficient `MEMBER/ACTIVE` projections, writes no message/timeline/outbox rows on deny, and records the expected `policy_decision_audit_outbox` decision. The sender-only ownership smoke proves that message-service forwards original sender context for `EDIT` / `REVOKE` / `DELETE`, sender mutations fall through to allow, non-sender mutations are rejected as `MESSAGE_OWNERSHIP_DENIED`, and the target mutation audit row carries message context. The ownership override smoke proves only the first-stage role override shape: non-sender `ADMIN/ACTIVE` with fresh `conversation_permission_version` can mutate through typed `ownership_override=true`, while `MEMBER/ACTIVE` remains denied. Full ReBAC, a separate `MODERATOR` role, tenant policy DSL, tenant quota/risk policy and risk scoring remain future work.

The decision audit outbox smoke proves that public policy decisions are staged as low-sensitive `policy_decision_audit_outbox` rows when PostgreSQL rules mode is enabled. The decision audit relay smoke proves those rows can be published to `im.policy.events` as protobuf `PolicyEvent` records and marked `PUBLISHED`. Audit rows and Kafka events store stable object keys, context-present flags, action, allow/deny, permission version, classification, reason code and trace/request ids. They do not store raw session id, raw device id, raw peer id, raw conversation id, raw message content, rule parameters, SQL errors or free-text deny/provider bodies. Explicit DLQ event-id repair is available; broad repair workflow, poison-payload classification, retention and external audit sinks remain future work.
The decision audit repair smoke proves the first-stage operator path for explicit DLQ event IDs:

```powershell
.\loadtest\policycontacts\run-local-smoke.ps1 -RunName policy-decision-audit-repair-smoke-20260613-clean -ExerciseAuditRepair
```

`NEXUSIM_POLICY_SERVICE_MODE=outbox-repair` only resets explicitly supplied DLQ rows back to `PENDING`, clears retry state, and writes `policy_decision_audit_outbox_repair_audit`. Before redrive, it validates the event through the same policy-event builder used by the relay. Invalid envelope or payload rows stay in `DLQ`, get a `SKIPPED / validation_failed` repair audit row, and make the operator exit non-zero. The repair mode does not publish Kafka directly, skip ordered blockers, rewrite payloads, repair all DLQ rows, implement retention, or send audit data to an external sink. After repair, the normal outbox relay remains responsible for publishing to `im.policy.events`.

`NEXUSIM_POLICY_SERVICE_MODE=outbox-repair-cleanup` is the local retention operator for `policy_decision_audit_outbox_repair_audit`. It deletes oldest repair audit rows before `now - retention`, supports optional `event_id / tenant_id / repair_operator / repair_outcome` filters for scoped cleanup, and never mutates live `policy_decision_audit_outbox` rows.
