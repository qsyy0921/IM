# policy-service Loadtest Reports

This directory is the entry point for `policy-service` smoke reports. The current implementation is not a capacity-tested policy engine; it is a first-stage service boundary for message action decisions.

## Current Status

Implemented:

- `PolicyService.CheckMessageAction` gRPC contract.
- Static first-stage message action decision configured by environment.
- Optional message-service RPC adapter through `NEXUSIM_POLICY_SERVICE_ADDR`.
- message-service fallback to legacy `StaticPolicy` when no policy-service address is configured.

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
-> message-service NEXUSIM_POLICY_SERVICE_ADDR
-> SendMessage / EditMessage / RevokeMessage / DeleteMessage
-> policy-service CheckMessageAction
-> message-service normal transaction / public deny
```

The next useful smoke should run both allow and deny cases, and should keep the policy permission version aligned with conversation permission version to avoid expected dependency-version mismatch.
