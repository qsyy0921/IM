# Identity Stack Capacity Baseline

## Scope

This report records a local short capacity baseline for `identity-service`. It proves that the identity stack runner can exercise:

```text
identity-service gRPC
-> verification challenge
-> identity_challenge_delivery_outbox
-> challenge-delivery-worker
-> webhook fixture
-> verification confirm
-> login / refresh
-> password reset
-> MFA TOTP and recovery-code management
```

This is a local development / interview evidence run. It is not a production SLO, HA proof, email/SMS provider proof, or sizing claim.

## Environment

| Item | Value |
| --- | --- |
| Commit | `e9c4beee` |
| Raw result root | `H:\NexusIM\loadtest-results\capacity-baseline-identity-stack-20260616` |
| Service target | temporary `127.0.0.1:57759` |
| Webhook fixture | temporary `http://127.0.0.1:57761/challenge` |
| PostgreSQL DSN | `postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable` |
| Runner | `.\loadtest\identity\run-local-smoke.ps1 -RunName capacity-baseline-identity-stack-20260616` |

The smoke wrapper started temporary identity gRPC, webhook fixture and `challenge-delivery-worker` processes, then stopped them at the end of the run.

## Result

| Metric | Value |
| --- | ---: |
| Summary success | `true` |
| Operation count | `17` |
| Token issue count | `5` |
| Expected error count | `2` |
| `challenge_delivery_outbox_total` | `2` |
| `challenge_delivery_outbox_pending` | `0` |
| `challenge_delivery_outbox_delivered` | `2` |
| `challenge_delivery_outbox_dlq` | `0` |
| `challenge_delivery_attempt_count` | `1` |
| `operations_per_second` | `14.092562103227023` |
| `latency_p95_ms` | `89.67` |
| `latency_p99_ms` | `89.67` |
| `mfa_recovery_code_count` | `20` |

The run also verified:

- verification challenge token was delivered through webhook, not returned as a dev token;
- password reset token was delivered through webhook;
- refresh without MFA returned the expected step-up error;
- login without MFA returned the expected step-up error;
- refresh with MFA and MFA login succeeded;
- recovery code regenerate and revoke succeeded;
- MFA factor disable succeeded.

## Limits

- This is a single short local run, not a long-duration capacity curve.
- It uses a local webhook fixture, not a production email/SMS provider.
- It does not validate provider bounce handling, tenant templates, KMS/HSM, OIDC, WebAuthn, or production risk policy.
- It does not prove HA behavior for PostgreSQL, Redis, Kafka, or the challenge delivery worker.
