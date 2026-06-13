# policy-service Observability Smoke - 2026-06-13

## Scope

This smoke verifies the first-stage `policy-service` observability surface:

- `/healthz`, `/readyz` and `/debug/metrics` through `NEXUSIM_POLICY_DEBUG_ADDR`;
- aggregate gRPC request metrics for `CheckMessageAction`;
- aggregate policy decision metrics split by action and allow / deny result;
- message-service policy RPC trace / request metadata propagation into policy-service structured gRPC logs.

This is not a capacity test and not a full production observability stack. It does not prove OpenTelemetry tracing, alerting, Prometheus deployment, audit outbox, contacts / conversation projection, tenant policy or risk scoring.

## Run

```text
command: .\loadtest\policy\run-local-smoke.ps1 -RunName policy-service-observability-smoke-20260613
raw results: H:\NexusIM\loadtest-results\policy-service-observability-smoke-20260613
commit: 1f98e20 feat: add policy service debug metrics
git_dirty: false
```

The runner starts `policy-service` twice:

- allow scenario: `NEXUSIM_POLICY_MESSAGE_ALLOWED=true`;
- deny scenario: `NEXUSIM_POLICY_MESSAGE_ALLOWED=false`.

Each scenario sends `SEND`, `EDIT`, `REVOKE` and `DELETE` through public gRPC, then reads `/debug/metrics` and validates the aggregate counters.

## Results

Allow scenario:

```text
grpc.total_requests = 4
grpc.total_errors = 0
decisions.total = 4
decisions.allowed = 4
decisions.denied = 0
decisions.errors = 0
```

Deny scenario:

```text
grpc.total_requests = 4
grpc.total_errors = 0
decisions.total = 4
decisions.allowed = 0
decisions.denied = 4
decisions.errors = 0
```

In the deny scenario, `allowed=false` remains a successful gRPC response (`codes.OK`) and is counted as a business decision deny, not a transport error.

## Safety Notes

The debug metrics intentionally avoid high-cardinality identifiers and sensitive request context. They do not expose tenant id, user id, conversation id, message id, device id, session id, policy request/response bodies, rule parameters, SQL errors, DSNs, deny reason text or classification strings.

Trace id and request id are propagated as gRPC metadata from message-service to policy-service for structured logs. They are not used as metrics labels.

## Remaining Work

- Metrics are in-process debug snapshots and reset on restart.
- Rule-store metrics are aggregate only.
- Production OpenTelemetry traces, Prometheus deployment, alerts and audit outbox are still future work.
- contacts block projection, conversation role policy, tenant policy and risk scoring are still future policy-service slices.
