# policy-service Loadtest Reports

This directory is the entry point for `policy-service` smoke reports. The current implementation is not a capacity-tested policy engine; it is a first-stage service boundary for message action decisions.

## Current Status

Implemented:

- `PolicyService.CheckMessageAction` gRPC contract.
- Static first-stage message action decision configured by environment.
- Optional message-service RPC adapter through `NEXUSIM_POLICY_SERVICE_ADDR`.
- message-service fallback to legacy `StaticPolicy` when no policy-service address is configured.
- Direct gRPC allow/deny smoke for `SEND`, `EDIT`, `REVOKE`, and `DELETE`: `loadtest-report-20260613-policy-service-smoke.md`.
- message-service `SendMessage` allow/deny integration smoke through `NEXUSIM_POLICY_SERVICE_ADDR`: `loadtest-report-20260613-policy-message-integration-smoke.md`.

Not yet implemented:

- contacts block / unblock projection;
- conversation role policy;
- tenant-level policy;
- content moderation / risk scoring;
- policy audit outbox;
- policy-service mTLS and production observability.

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

Raw summaries are written under `H:\NexusIM\loadtest-results\<run-name>`:

```text
allow\policy-summary.json
deny\policy-summary.json
policy-smoke-summary.json
```

The heavier integration shape for edit / revoke / delete is still:

```text
policy-service grpc
-> message-service NEXUSIM_POLICY_SERVICE_ADDR
-> SendMessage / EditMessage / RevokeMessage / DeleteMessage
-> policy-service CheckMessageAction
-> message-service normal transaction / public deny
```

When testing through `message-service`, keep the policy permission version aligned with conversation permission version to avoid expected dependency-version mismatch. The integration smoke intentionally sets local mock policy opposite to remote policy decision so fallback cannot produce a false positive. Do not treat these smokes as proof of contacts / role / tenant / risk policy behavior.
