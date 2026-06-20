# notification-service outbox relay smoke - 2026-06-20

## Scope

This smoke verifies the first real-process notification event publish path:

```text
CreateNotificationRequest -> notification_outbox
-> notification-service outbox-relay -> Kafka im.notification.events
-> typed NotificationEvent readback
```

It also validates the no-secret-payload request path and keeps the current low
sensitivity boundary: outbox / Kafka payloads must not expose raw destination,
destination hash, secret payload, provider body, SMTP transcript, reset token,
TOTP code or recovery code fields.

This is still a small local smoke. It does not validate provider worker,
provider-grade email / SMS / APNs / FCM, bounce / suppression, Kafka HA,
capacity, or long-running retry behavior.

## Command

```powershell
.\loadtest\notification\run-local-smoke.ps1
```

Raw summary:

```text
H:\NexusIM\loadtest-results\notification-service-outbox-relay-smoke-20260620-202253\notification-outbox-relay-summary.json
```

## Environment

| Field | Value |
| --- | --- |
| Commit | `26534bf1` |
| Full commit | `26534bf125466731336ce8884e28b24c25e91b62` |
| Git dirty | `false` |
| PostgreSQL | `nexusim-postgres` local Docker container |
| Kafka | `nexusim-kafka` local Docker container |
| Notification target | `127.0.0.1:54500` |
| Kafka brokers | `localhost:9092` |
| Kafka topic | `im.notification.events.notification-service-outbox-relay-smoke-20260620-202253` |

## Result

| Check | Result |
| --- | --- |
| Overall success | `true` |
| Request id | `notif_640c664b4367e11877dbfb59f5007f56` |
| Request status | `NOTIFICATION_REQUEST_STATUS_ACCEPTED` |
| Request channel | `EMAIL` |

## Outbox / Kafka

| Metric | Value |
| --- | --- |
| `notification_outbox total` | `1` |
| `notification_outbox accepted` | `1` |
| `notification_outbox PUBLISHED` | `1` |
| `notification_outbox PENDING` | `0` |
| `notification_outbox DLQ` | `0` |
| Kafka `NotificationEvent` read count | `1` |

Kafka event read back from the smoke topic:

| Event type | Event id | Payload kind | Channel | Status |
| --- | --- | --- | --- | --- |
| `notification.request.accepted.v1` | `notif_640c664b4367e11877dbfb59f5007f56:accepted` | `request_accepted` | `EMAIL` | `ACCEPTED` |

## Conclusion

`notification_outbox -> im.notification.events` is now verified by a real local
process smoke: the relay published the expected notification event, marked the
outbox row `PUBLISHED`, and the runner read back a typed protobuf
`NotificationEvent` record from Kafka.

Next notification-service work should move to provider worker / provider adapter
boundaries. Do not describe the service as provider-grade email / SMS / APNs /
FCM until delivery worker, provider retry / DLQ, bounce and suppression paths are
implemented and smoked.
